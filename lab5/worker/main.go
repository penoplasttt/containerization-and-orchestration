package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/shop?sslmode=disable"))
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	interval := 5 * time.Second
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}

	for {
		res, err := db.Exec(`UPDATE orders SET status = 'processed'
			WHERE id IN (SELECT id FROM orders WHERE status = 'pending' ORDER BY id LIMIT 10)`)
		if err != nil {
			log.Printf("failed to process orders: %v", err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			log.Printf("processed %d orders", n)
		}
		time.Sleep(interval)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}