package handlers

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestHealthHandler(t *testing.T) {
    req, _ := http.NewRequest("GET", "/health", nil)
    rr := httptest.NewRecorder()
    handler := http.HandlerFunc(HealthHandler)
    handler.ServeHTTP(rr, req)
    if status := rr.Code; status != http.StatusOK {
        t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
    }
    var resp struct {
        App        string `json:"app"`
        Kubernetes string `json:"kubernetes"`
    }
    json.NewDecoder(rr.Body).Decode(&resp)
    if resp.App != "ok" || resp.Kubernetes != "connected" {
        t.Errorf("handler returned unexpected body: got %v want App:ok Kubernetes:connected", resp)
    }
}