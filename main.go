package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
		log.Printf("completed in %s", time.Since(start))
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}
