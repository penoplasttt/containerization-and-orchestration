package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
)

var (
	memoryHold = make([][]byte, 0)
	memMu      sync.Mutex
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /eat", eatHandler)
	mux.HandleFunc("GET /burn", burnHandler)

	port := ":8080"
	log.Printf("Server listening on port %s...", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok\n"))
}

func eatHandler(w http.ResponseWriter, r *http.Request) {
	mbStr := r.URL.Query().Get("mb")
	if mbStr == "" {
		http.Error(w, "query param 'mb' is required", http.StatusBadRequest)

		return
	}

	mb, err := strconv.Atoi(mbStr)
	if err != nil || mb <= 0 {
		http.Error(w, "query param 'mb' must be a positive integer", http.StatusBadRequest)
		
		return
	}

	chunk := make([]byte, mb*1024*1024)

	for i := 0; i < len(chunk); i += 4096 {
		chunk[i] = 1
	}

	memMu.Lock()
	memoryHold = append(memoryHold, chunk)
	totalChunks := len(memoryHold)
	memMu.Unlock()

	fmt.Fprintf(w, "Allocated and holding %d MB (total chunks: %d)\n", mb, totalChunks)
}

func burnHandler(w http.ResponseWriter, _ *http.Request) {
	go func() {
		for {
		}
	}()

	fmt.Fprintln(w, "Burning 1 CPU core in background")
}

