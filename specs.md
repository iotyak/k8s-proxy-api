# k8s-proxy-api Specs

## Overview
`k8s-proxy-api` is a minimal Go HTTP proxy for scoped Kubernetes ops (deploy restart, pod logs/status). Not a full K8s API—tight control plane for internal UIs/services.

Single binary (`go build`), stdlib `net/http`, client-go. Strict paths, label auth (`proxy-access=allowed`).

## Design Principles
- Single entry (`main.go` wires pkgs).
- `/health` unversioned; ops `/api/v1/...`.
- Strict paths: no //, trailing /, ./..
- JSON errors w/ context (ns/name/action).
- Narrow RBAC: deps get/patch, pods get/list/log, rs get.
- Resilient: Runs sans K8s (health reports).
- Incremental: Phases build/test independently.

## Runtime Config
1. `KUBECONFIG` → file config.
2. Else in-cluster.

Local: `export KUBECONFIG=/etc/rancher/k3s/k3s.yaml && go run .`

## APIs (Immutable)
| Method | Path | Response | Notes |
|--------|------|----------|-------|
| GET | `/health` | JSON `{app_status:"ok", kubernetes_reachable:bool, version/error}` | Connectivity. |
| POST | `/api/v1/namespaces/{ns}/deployments/{name}/restart` | 200 JSON or err (404/403/5xx) | Patch restart annot. Label req'd. |
| GET | `/api/v1/namespaces/{ns}/pods/{pod}/logs` | text stream or JSON err | Latest logs. |
| GET | `/api/v1/namespaces/{ns}/deployments/{name}/pods/status` | JSON pod[] `{name,phase,ip,conditions[],containers[],ready/total,restarts}` | Selector pods. Label req'd. |

## K8s Flow
- Client: `internal/k8sclient` (KUBECONFIG/in-cluster).
- Handlers: `internal/handlers` (health/restart/logs/status).
- Auth: Label check.
- Pods→Dep: Traverse pod→rs→dep owners.

## Deploy (proxy-test ns)
`kubectl apply -f k8s/proxy-test.yaml` (SA/Role/Deploy/Svc).

## Verification
- Unit: `go test ./... -cover`
- E2E: `tools/test.sh`
- Smoke: `gofmt -w . && go test ./... && go build .`

## Phases Complete
- Phase 1: Health + K8s connect.
- Phase 2: Restart/logs/status + auth/strict paths.
- Phase 3: Modularize/tests (in-progress).

## Next Phases
- Observability (metrics/logs).
- Validation (ns/name lengths).
- OpenAPI spec.
- Multi-ns authz.
-