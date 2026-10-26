package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	_ "github.com/lib/pq"
)

type Order struct {
	ID     int64  `json:"id"`
	Item   string `json:"item"`
	Status string `json:"status"`
}

var (
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "shop_api_http_requests_total",
			Help: "Total number of HTTP requests handled by the api, by handler and status code.",
		},
		[]string{"handler", "code"},
	)
	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "shop_api_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds, by handler.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"handler"},
	)
	// shuttingDown сигнализирует, что под завершается: readiness-проба
	// начинает возвращать 503, чтобы Kubernetes убрал под из эндпоинтов
	// ДО того, как мы закроем listener.
	shuttingDown atomic.Bool
)

func init() {
	prometheus.MustRegister(httpRequests)
	prometheus.MustRegister(httpDuration)
}

// instrument wraps a handler and records request count and duration.
func instrument(handler string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		httpRequests.WithLabelValues(handler, strconv.Itoa(rec.status)).Inc()
		httpDuration.WithLabelValues(handler).Observe(time.Since(start).Seconds())
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func main() {
	db, err := sql.Open("postgres", getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/shop?sslmode=disable"))
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS orders (
		id BIGSERIAL PRIMARY KEY,
		item TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		log.Fatalf("failed to create table: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", instrument("health", func(w http.ResponseWriter, r *http.Request) {
		if shuttingDown.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("shutting down\n"))
			return
		}
		if os.Getenv("HEALTH_FAIL") == "true" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unhealthy\n"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
	}))

	mux.HandleFunc("/order", instrument("order", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Item string `json:"item"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		var id int64
		if err := db.QueryRow(`INSERT INTO orders (item) VALUES ($1) RETURNING id`, req.Item).Scan(&id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Order{ID: id, Item: req.Item, Status: "pending"})
	}))

	mux.HandleFunc("/orders", instrument("orders", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`SELECT id, item, status FROM orders ORDER BY id`)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		defer rows.Close()

		orders := []Order{}
		for rows.Next() {
			var o Order
			if err := rows.Scan(&o.ID, &o.Item, &o.Status); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}
			orders = append(orders, o)
		}
		json.NewEncoder(w).Encode(orders)
	}))

	// Prometheus metrics endpoint (not instrumented to avoid self-scraping noise).
	mux.Handle("/metrics", promhttp.Handler())

	addr := ":" + getenv("PORT", "8080")
	log.Printf("api listening on %s", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown: ждём SIGTERM/SIGINT.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
		<-quit
		log.Println("получен сигнал завершения")

		// 1) Помечаем под как завершающийся: readiness-проба начнёт отдавать 503,
		//    Kubernetes уберёт под из EndpointSlice.
		shuttingDown.Store(true)
		log.Println("readiness переведён в 503, жду, пока k8s уберёт под из эндпоинтов...")

		// 2) Даём Kubernetes время обновить EndpointSlice (обычно 1-2 сек).
		time.Sleep(3 * time.Second)

		// 3) Теперь можно закрывать listener и ждать завершения активных запросов.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown: %v", err)
		}
		log.Println("сервер остановлен")
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}