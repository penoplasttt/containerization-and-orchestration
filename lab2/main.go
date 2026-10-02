package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "endpoint", "status"},
	)
	httpErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_errors_total",
			Help: "Total number of HTTP 5xx errors.",
		},
		[]string{"method", "endpoint"},
	)
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Histogram of response time for handler in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpErrorsTotal)
	prometheus.MustRegister(httpRequestDuration)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
}

func initTracer() *sdktrace.TracerProvider {
	exporter, err := otlptracehttp.New(context.Background(), otlptracehttp.WithInsecure())
	if err != nil {
		slog.Error("failed to initialize exporter", "error", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("lab2-api"),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ctx := r.Context()
		spanCtx := trace.SpanContextFromContext(ctx)
		traceID := spanCtx.TraceID().String()

		slog.InfoContext(ctx, "incoming request",
			slog.String("trace_id", traceID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
		)

		rw := &responseWriter{w, http.StatusOK}
		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()

		httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(rw.statusCode)).Inc()
		httpRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
		if rw.statusCode >= 500 {
			httpErrorsTotal.WithLabelValues(r.Method, r.URL.Path).Inc()
		}

		slog.InfoContext(ctx, "request completed",
			slog.String("trace_id", traceID),
			slog.Int("status", rw.statusCode),
			slog.Float64("duration_sec", duration),
		)
	})
}

func main() {
	tp := initTracer()
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			slog.Error("Error shutting down tracer provider", "error", err)
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error\n"))
	})

	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		sleepTime := time.Duration(rand.Intn(2000)+1000) * time.Millisecond
		time.Sleep(sleepTime)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("slept for %v\n", sleepTime)))
	})

	mux.HandleFunc("/load", func(w http.ResponseWriter, r *http.Request) {
		var wg sync.WaitGroup

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				http.Get("http://localhost:8080/health")
			}()
		}
		wg.Wait()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("load generated\n"))
	})

	mux.Handle("/metrics", promhttp.Handler())

	handler := middleware(mux)
	otelHandler := otelhttp.NewHandler(handler, "lab2-server")

	slog.Info("Server is starting on :8080")
	if err := http.ListenAndServe(":8080", otelHandler); err != nil {
		slog.Error("Server failed", "error", err)
	}
}
