package handlers

import (
	"encoding/json"
	"net/http"
)

type HealthResponse struct {
	App        string `json:"app"`
	Kubernetes string `json:"kubernetes"`
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{App: "ok", Kubernetes: "connected"} // Stub; later integrate real K8s check
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
