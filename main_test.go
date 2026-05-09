package main

import (
    "testing"
)

func TestSplitPathStrict(t *testing.T) {
    _, err := SplitPathStrict("/api/v1//ns")  // Double slash
    if err == nil {
        t.Error("expected error for double slash")
    }
    _, err = SplitPathStrict("/api/v1/ns/")  // Trailing slash
    if err == nil {
        t.Error("expected error for trailing slash")
    }
    parts, err := SplitPathStrict("/api/v1/namespaces/ns/deployments/name")
    if err != nil || len(parts) != 6 {
        t.Errorf("expected no error and 6 parts, got %v %v", err, len(parts))
    }
}