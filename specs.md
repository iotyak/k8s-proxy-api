# k8s-proxy-api Specs

## Overview
`k8s-proxy-api` is a minimal Go HTTP proxy for scoped Kubernetes ops (deploy restart/delete, pod logs/status). Not a full K8s API; tight control plane for internal UIs/services.

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
| DELETE | `/api/v1/namespaces/{ns}/deployments/{name}/delete` | 200 JSON or err (400/404/403/5xx) | Delete deployment. Label req'd. |
| GET | `/api/v1/namespaces/{ns}/deployments` | JSON list `{namespace,count,deployments[]}` | Namespace deployment listing. |
| GET | `/api/v1/namespaces/{ns}/pods` | JSON list `{namespace,count,pods[]}` | Namespace pod listing. |
| GET | `/api/v1/namespaces/{ns}/pods/{pod}/logs` | text stream or JSON err | Log options supported; default `tailLines=100`. |
| GET | `/api/v1/namespaces/{ns}/deployments/{name}/pods/status` | JSON pod[] `{name,phase,ip,conditions[],containers[],ready/total,restarts}` | Selector pods. Label req'd. |

### Delete Deployment Endpoint
`DELETE /api/v1/namespaces/{ns}/deployments/{name}/delete`

Example:

```bash
curl -i -X DELETE "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/hello-allowed/delete"
```

Success response (`200`, JSON):

```json
{
  "success": true,
  "authorized": true,
  "namespace": "proxy-test",
  "deployment": "hello-allowed",
  "action": "delete",
  "message": "deployment deleted",
  "required_label": "proxy-access=allowed",
  "deployment_label": "allowed"
}
```

Error responses (JSON):
- `400`: `{"error":"malformed path"}`
- `403`: deployment is not allowed for proxy access; includes `required_label` and `deployment_label`
- `404`: deployment not found
- `500`: failed to read/delete deployment with `details`

### Namespace Listing Endpoints

#### List Deployments in Namespace
`GET /api/v1/namespaces/{ns}/deployments`

Description:
- Returns deployments scoped to the requested namespace.

Example:

```bash
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments"
```

Basic success response (`200`, JSON):

```json
{
  "namespace": "proxy-test",
  "count": 2,
  "deployments": [
    {
      "name": "hello",
      "replicas": 1,
      "ready_replicas": 1,
      "available_replicas": 1,
      "updated_replicas": 1
    }
  ]
}
```

#### List Pods in Namespace
`GET /api/v1/namespaces/{ns}/pods`

Description:
- Returns pods scoped to the requested namespace.
 
Error responses:
- `405`: Wrong HTTP method
- `400`: Malformed path
- `503`: Kubernetes client unavailable or list operation failed

Example:

```bash
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods"
```

Basic success response (`200`, JSON):

```json
{
  "namespace": "proxy-test",
  "count": 3,
  "pods": [
    {
      "name": "hello-7f9d7f7f75-abcde",
      "phase": "Running",
      "ip": "10.42.0.15",
      "ready_containers": 1,
      "total_containers": 1,
      "restart_count": 0
    }
  ]
}
```

### Logs Endpoint Query Parameters
`GET /api/v1/namespaces/{ns}/pods/{pod}/logs`

- `tailLines` (int): return the most recent N log lines. Default: `100`.
- `sinceSeconds` (int): return only logs newer than N seconds.
- `follow` (bool): stream logs continuously until client disconnect.
- `limitBytes` (int): cap bytes returned by the log response.
- `timestamps` (bool): include timestamps on each line.

Examples:

```bash
# Default (latest 100 lines)
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs"

# Tail more lines
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs?tailLines=500"

# Last 5 minutes, include timestamps
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs?sinceSeconds=300&timestamps=true"

# Follow stream with 1MiB cap for initial burst
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs?follow=true&limitBytes=1048576"
```

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
