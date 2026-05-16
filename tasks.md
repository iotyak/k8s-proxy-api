`tasks.md`
```md
# k8s-proxy-api Tasks

## Project Workflow
Implementation should stay incremental.
Each task should be completed, tested, and committed before moving to the next one.

## Completed (Core APIs)
- [x] Initialize the Go module
- [x] Create a minimal stdlib HTTP server
- [x] Add `GET /health`
- [x] Return a basic JSON health response
- [x] Add basic request logging
- [x] Confirm the server runs locally and responds on `127.0.0.1:8080`

### Add Kubernetes client-go with practical dev behavior
- [x] Add official `client-go` dependencies
- [x] Load Kubernetes config from `KUBECONFIG` when set
- [x] Fall back to in-cluster config when `KUBECONFIG` is not set
- [x] Create a Kubernetes clientset during startup
- [x] Do not crash the HTTP server just because Kubernetes is unreachable
- [x] Update `GET /health` to return:
  - [x] app status
  - [x] Kubernetes reachable true/false
  - [x] Kubernetes version when reachable
  - [x] clear error string when unreachable
- [x] Keep the server on stdlib `net/http`
- [x] Keep the code small and readable
- [x] Test locally using:
  - [x] `export KUBECONFIG=/etc/rancher/k3s/k3s.yaml`
  - [x] `go run .`
  - [x] `curl http://127.0.0.1:8080/health`

### Placeholder API structure → Full Impl
- [x] Add routes:
  - [x] `POST /api/v1/namespaces/{ns}/deployments/{name}/restart`
  - [x] `GET /api/v1/namespaces/{ns}/pods/{pod}/logs`
  - [x] `GET /api/v1/namespaces/{ns}/deployments/{name}/pods/status`
- [x] Parse path segments explicitly and safely (`SplitPathStrict`)
- [x] Return structured JSON errors for malformed paths/auth/missing

## Refactor: Modularization & Tests (from refactor_plan.md)
Status: Tasks 1-5 done/partial (testify, handlers dir/health, main_test paths, k8sclient).

### Remaining Tasks (TDD: red→green→refactor)
- [x] **Task 6: Extract restart handler** (`internal/handlers/restart.go` +test)
- [x] **Task 7: Extract auth** (`internal/auth/check.go` +test)
- [x] **Task 8: Extract logs & status** (`internal/handlers/logs.go/status.go` +tests)
- [x] **Task 9: Integration test** (main_test.go full mux +fake)
- [x] **Task 10: Full verify**

## Logs Endpoint Improvement (Large Volume Handling)
- [x] **Task 11: Improve logs endpoint for high volume**
  - Add query parameters: `tailLines`, `sinceSeconds`, `follow`, `limitBytes`, `timestamps`
  - Update `LogsHandler` / `streamPodLogs` to use `corev1.PodLogOptions`
  - Update tests in `logs_test.go` (table-driven)
  - Update `README.MD` and `specs.md` documentation
  - Commit: `380c3dc feat: support log tailing, limits, follow and timestamps for large volumes`

## Deployment Delete Endpoint
- [x] **Task 12: Add DELETE deployment endpoint**
  - Update path parsing to support `/deployments/{name}/delete`
  - Create `DeleteDeploymentHandler` in `internal/handlers/delete.go`
  - Add unit tests (success, 404, 403, malformed path)
  - Wire handler into `main.go` `namespaceHandler`
  - Update `README.MD` and `specs.md`
  - Commit: `d20d315 feat: add DELETE /api/v1/namespaces/{ns}/deployments/{name}/delete endpoint`

## Namespace Listing Endpoints
- [x] **Task 13: Add namespace-level listing endpoints**
  - Add route constants (`RouteDeployments`, `RoutePodsList`)
  - Update path parsing in `main.go`
  - Create `ListDeployments