# k8s-proxy-api Modularization and Testing Refactor Plan

Goal: Refactor the single-file main.go into modular packages for handlers, Kubernetes client, and auth logic, while adding unit tests to cover core functionality, improving maintainability without changing external API behavior.

Architecture: Extract inline handler functions from main.go into a handlers package with individual files per endpoint. Create a k8sclient package to wrap client-go interactions (init, get/patch/list). Add an auth package for label checks. Use Go's internal packages (no external deps beyond existing k8s.io/client-go). Tests will use httptest for API simulation and client-go's fake clients for K8s mocking. Keep main.go as the entrypoint (imports, server setup). No runtime changes—preserve strict path parsing, RBAC scope, and endpoints.

Tech Stack: Go 1.21+, net/http (stdlib), gorilla/mux (if used, else stdlib), k8s.io/client-go v0.28+, testing (stdlib), testify/suite (add if needed for assertions; go mod tidy). Tests in _test.go files per package.

## Tasks

### Task 1: Add testify dependency for testing

Objective: Install testify to enable better assertions in tests (e.g., Eventually for async K8s calls).

Files:
- Modify: go.mod

Step 1: No test needed (setup task).

Step 2: Run: go mod tidy github.com/stretchr/testify@v1.9.0
Expected: go.mod updated with require github.com/stretchr/testify v1.9.0

Step 3: Verify: go mod tidy && go list -m github.com/stretchr/testify
Expected: github.com/stretchr/testify v1.9.0

Step 4: Commit: git add go.mod go.sum && git commit -m "chore: add testify for testing"

### Task 2: Create handlers package directory and init module

Objective: Set up internal/handlers package to hold endpoint logic, starting with empty structure.

Files:
- Create: internal/handlers/ (directory)

Step 1: No test needed (structural task).

Step 2: mkdir -p internal/handlers

Step 3: Verify: ls -la internal/

Step 4: Commit: git add internal/handlers && git commit -m "refactor: create internal/handlers package dir"

### Task 3: Extract health handler to handlers package

Objective: Move the GET /health logic from main.go to handlers/health.go, using a simple JSON response for app/K8s status.

Files:
- Create: internal/handlers/health.go
- Create: internal/handlers/health_test.go
- Modify: main.go (remove inline handler, import and wire it)

Step 1: Write failing test in health_test.go
```go
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
```

Step 2: Run test to verify failure
go test ./internal/handlers -v
Expected: FAIL

Step 3: Write minimal implementation in health.go
```go
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
    resp := HealthResponse{App: "ok", Kubernetes: "connected"}  // Stub; later integrate real K8s check
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}
```

Step 4: Run test to verify pass
go test ./internal/handlers -v
Expected: PASS

Step 5: Wire into main.go (e.g., mux.HandleFunc("/health", handlers.HealthHandler))

go build .
Expected: No errors

Step 6: Commit
git add internal/handlers/health.go internal/handlers/health_test.go main.go && git commit -m "refactor: extract health handler to internal/handlers"

### Task 4: Add unit test for strict path parsing function

Objective: Test the existing splitPathStrict function (assumed in main.go) to ensure it rejects invalid paths.

Files:
- Create: main_test.go

Step 1: Write failing test in main_test.go
```go
package main

import (
    "testing"
)

func TestSplitPathStrict(t *testing.T) {
    _, err := splitPathStrict("/api/v1//ns")  // Double slash
    if err == nil {
        t.Error("expected error for double slash")
    }
    _, err = splitPathStrict("/api/v1/ns/")  // Trailing slash
    if err == nil {
        t.Error("expected error for trailing slash")
    }
    parts, err := splitPathStrict("/api/v1/namespaces/ns/deployments/name")
    if err != nil || len(parts) != 5 {
        t.Errorf("expected no error and 5 parts, got %v %v", err, len(parts))
    }
}
```

Step 2: Run test to verify failure
go test . -v
Expected: FAIL

Step 3: Export/adjust splitPathStrict in main.go if needed (make exported).

Step 4: Run test to verify pass
go test . -v
Expected: PASS

Step 5: Commit
git add main_test.go && git commit -m "test: add unit tests for splitPathStrict"

### Task 5: Create k8sclient package for client initialization

Objective: Extract K8s client setup from main.go to a reusable client in internal/k8sclient.

Files:
- Create: internal/k8sclient/client.go
- Create: internal/k8sclient/client_test.go
- Modify: main.go

Step 1: Write failing test in client_test.go
```go
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
```

Step 2: Run test to verify failure
go test ./internal/k8sclient -v
Expected: FAIL

Step 3: Write minimal implementation in client.go
```go
package k8sclient

import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/clientcmd"
)

func NewClient() (*kubernetes.Clientset, error) {
    config, err := rest.InClusterConfig()
    if err != nil {
        config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
        if err != nil {
            return nil, err
        }
    }
    return kubernetes.NewForConfig(config)
}
```

Step 4: Run test to verify pass
go test ./internal/k8sclient -v
Expected: PASS

Step 5: Update main.go to use clientset, _ := k8sclient.NewClient()

go build .
Expected: Builds

Step 6: Commit
git add internal/k8sclient/client.go internal/k8sclient/client_test.go main.go && git commit -m "refactor: extract K8s client to internal/k8sclient"

### Task 6: Extract deployment restart handler

Objective: Move POST /api/v1/namespaces/{ns}/deployments/{name}/restart to handlers/restart.go, using k8sclient.

Files:
- Create: internal/handlers/restart.go
- Create: internal/handlers/restart_test.go
- Modify: main.go

Step 1: Write failing test in restart_test.go
```go
package handlers

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "k8s.io/client-go/kubernetes/fake"
    "k8s.io/api/apps/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    "time"
)

func TestRestartDeploymentHandler(t *testing.T) {
    fakeClient := fake.NewSimpleClientset()
    dep := &v1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "testdep", Namespace: "testns", Labels: map[string]string{"proxy-access": "allowed"}}}
    fakeClient.AppsV1().Deployments("testns").Create(context.Background(), dep, metav1.CreateOptions{})
    
    req, _ := http.NewRequest("POST", "/api/v1/namespaces/testns/deployments/testdep/restart", bytes.NewBuffer([]byte("{}")))
    rr := httptest.NewRecorder()
    RestartDeploymentHandler(rr, req, fakeClient)
    if status := rr.Code; status != http.StatusOK {
        t.Errorf("wrong status: %v", status)
    }
    updated, _ := fakeClient.AppsV1().Deployments("testns").Get(context.Background(), "testdep", metav1.GetOptions{})
    if _, ok := updated.Annotations["kubectl.kubernetes.io/restartedAt"]; !ok {
        t.Error("expected restart annotation")
    }
}
```

Step 2: Run test to verify failure
go test ./internal/handlers -v
Expected: FAIL

Step 3: Write minimal implementation in restart.go
```go
package handlers

import (
    "context"
    "encoding/json"
    "net/http"
    "strings"
    "time"
    "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    "k8s.io/client-go/kubernetes"
    "k8s.io/api/apps/v1"
)

func RestartDeploymentHandler(w http.ResponseWriter, r *http.Request, clientset *kubernetes.Clientset) {
    parts := strings.Split(r.URL.Path, "/")[1:]  // Simple split; use splitPathStrict if available
    if len(parts) != 6 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[3] != "deployments" {
        http.Error(w, "Invalid path", http.StatusBadRequest)
        return
    }
    ns, name := parts[4], parts[5]
    
    ctx := context.Background()
    dep, err := clientset.AppsV1().Deployments(ns).Get(ctx, name, v1.GetOptions{})
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    if dep.Labels["proxy-access"] != "allowed" {
        http.Error(w, "Access denied", http.StatusForbidden)
        return
    }
    patch := []byte(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"` + time.Now().Format(time.RFC3339) + `"}}}}}`)
    _, err = clientset.AppsV1().Deployments(ns).Patch(ctx, name, types.StrategicMergePatchType, patch, v1.PatchOptions{})
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "restarted"})
}
```

Step 4: Run test to verify pass
go test ./internal/handlers -v
Expected: PASS

Step 5: Wire in main.go: mux.HandleFunc("/api/v1/namespaces/{ns}/deployments/{name}/restart", func(w, r) { handlers.RestartDeploymentHandler(w, r, clientset) })

go build .
Expected: Builds

Step 6: Commit
git add internal/handlers/restart.go internal/handlers/restart_test.go main.go && git commit -m "refactor: extract deployment restart handler and tests"

### Task 7: Extract auth logic to separate package

Objective: Move label check to internal/auth/check.go for reuse across handlers.

Files:
- Create: internal/auth/check.go
- Create: internal/auth/check_test.go
- Modify: handlers/restart.go

Step 1: Write failing test in check_test.go
```go
package auth

import (
    "testing"
    "k8s.io/api/apps/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckDeploymentAllowed(t *testing.T) {
    depAllowed := &v1.Deployment{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"proxy-access": "allowed"}}}
    if !CheckDeploymentAllowed(depAllowed) {
        t.Error("expected allowed")
    }
    depDenied := &v1.Deployment{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}}
    if CheckDeploymentAllowed(depDenied) {
        t.Error("expected denied")
    }
}
```

Step 2: Run test to verify failure
go test ./internal/auth -v
Expected: FAIL

Step 3: Write minimal implementation in check.go
```go
package auth

import "k8s.io/api/apps/v1"

func CheckDeploymentAllowed(dep *v1.Deployment) bool {
    return dep.Labels["proxy-access"] == "allowed"
}
```

Step 4: Run test to verify pass
go test ./internal/auth -v
Expected: PASS

Step 5: Update restart.go: if !auth.CheckDeploymentAllowed(dep) { ... }

go build .
Expected: Builds

Step 6: Commit
git add internal/auth/check.go internal/auth/check_test.go internal/handlers/restart.go && git commit -m "refactor: extract auth label check to internal/auth"

### Task 8: Extract pod logs and status handlers similarly

Objective: Repeat extraction for logs and pods/status endpoints, using k8sclient and auth where applicable.

Files:
- Create: internal/handlers/logs.go and logs_test.go
- Create: internal/handlers/status.go and status_test.go
- Modify: main.go

For logs: GET /api/v1/namespaces/{ns}/pods/{pod}/logs — stream plain text logs using clientset.CoreV1().Pods(ns).GetLogs(pod).DoRaw(ctx)

Test: Mock logs with fake, assert text response or error JSON.

Implementation: Parse path with splitPathStrict, get logs stream, copy to w or error.

For status: GET /api/v1/namespaces/{ns}/deployments/{name}/pods/status — get deployment, list pods by selector, JSON with status.

Test: Fake deployment/pods, assert JSON array.

Follow TDD pattern: failing test, implement, pass, wire mux.HandleFunc, build, commit "refactor: extract logs and status handlers with tests"

### Task 9: Add integration test for full API using httptest

Objective: Test the entire mux in main.go with a fake client injected.

Files:
- Modify: main_test.go

Step 1: Write failing test
Add:
```go
func TestFullAPI(t *testing.T) {
    // Assume global clientset or param; setup mux with fake
    // Test /health: assert JSON
    // Test /restart: assert patch called, response
    // Test /logs: assert stream or error
    // Test /status: assert pod list JSON
}
```

Step 2: Run fail.

Step 3: Adjust main.go to expose clientset for testing (e.g., var Clientset *kubernetes.Clientset; in init set it).

Step 4: Pass.

Step 5: Commit: "test: add integration test for API endpoints"

### Task 10: Run full verification and clean up

Objective: Format, test all, build.

Files:
- All .go

Step 1: gofmt -w . && go test ./... -v && go build .

Expected: All pass, builds.

Step 2: Test local run: export KUBECONFIG=/etc/rancher/k3s/k3s.yaml && go run . (curl /health should return JSON)

Step 3: Commit: "refactor: final modularization cleanup and full tests"

After all tasks, summarize changes (git log --oneline -10), test coverage (go test ./... -cover), any issues or deviations from plan.
