package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

type Order struct {
	ID     int64  `json:"id"`
	Item   string `json:"item"`
	Status string `json:"status"`
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

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("HEALTH_FAIL") == "true" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unhealthy\n"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
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
	})

	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
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
	})

	addr := ":" + getenv("PORT", "8080")
	log.Printf("api listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}