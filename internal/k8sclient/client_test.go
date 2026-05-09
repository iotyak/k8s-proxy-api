package k8sclient

import (
    "testing"
)

func TestNewClient(t *testing.T) {
    client, err := NewClient()
    if err != nil {
        t.Error("expected no error")
    }
    if client == nil {
        t.Error("expected non-nil client")
    }
}