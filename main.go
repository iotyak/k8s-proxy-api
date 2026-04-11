package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
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
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func splitPathStrict(path string) ([]string, bool) {
	if path == "" || path[0] != '/' {
		return nil, false
	}
	if strings.Contains(path, "//") {
		return nil, false
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		return nil, false
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return nil, false
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, false
		}
	}

	return parts, true
}

func namespaceHandler(w http.ResponseWriter, r *http.Request) {
	parts, ok := splitPathStrict(r.URL.Path)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "malformed path")
		return
	}

	if len(parts) != 5 || parts[0] != "namespaces" {
		writeJSONError(w, http.StatusBadRequest, "malformed path")
		return
	}

	ns := parts[1]
	resource := parts[2]
	target := parts[3]
	action := parts[4]

	switch {
	case resource == "deployments" && action == "restart":
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"namespace":  ns,
			"deployment": target,
			"action":     "restart",
			"success":    true,
		})

	case resource == "pods" && action == "logs":
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"namespace": ns,
			"pod":       target,
			"action":    "logs",
			"logs":      "placeholder logs",
		})

	default:
		writeJSONError(w, http.StatusNotFound, "endpoint not found")
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/namespaces/", namespaceHandler)

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/namespaces/") {
			if _, ok := splitPathStrict(r.URL.Path); !ok {
				writeJSONError(w, http.StatusBadRequest, "malformed path")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})

	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(root)); err != nil {
		log.Fatal(err)
	}
}
