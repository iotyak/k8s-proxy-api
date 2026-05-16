# k8s-proxy-api

Small internal API for a tightly scoped set of Kubernetes operations.

## Local Development

Prereqs:
- Go installed
- Kubernetes access (k3s/k8s)
- `kubectl` configured

Run locally:

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
go run .
```

Quick build-check:

```bash
gofmt -w . && go test ./...
```

Build the binary:

```bash
go build .
```

The service listens on:
- `http://127.0.0.1:8080`

## Project Structure

- Single binary entrypoint (`main.go`)
- Modular packages:
  - `internal/handlers` — endpoint logic (health, restart, logs, status)
  - `internal/auth` — label-based authorization
  - `internal/k8sclient` — Kubernetes client initialization

See `specs.md` and `AGENTS.md` for full API contract and contributor guidelines.

## Routes

Health route:
- `GET /health`

Operational routes (versioned):
- `POST /api/v1/namespaces/{ns}/deployments/{name}/restart`
- `DELETE /api/v1/namespaces/{ns}/deployments/{name}/delete`
- `GET /api/v1/namespaces/{ns}/deployments`
- `GET /api/v1/namespaces/{ns}/pods`
- `GET /api/v1/namespaces/{ns}/pods/{pod}/logs`
- `GET /api/v1/namespaces/{ns}/deployments/{name}/pods/status`

Route descriptions:
- `GET /api/v1/namespaces/{ns}/deployments`: list deployments in a namespace.
- `GET /api/v1/namespaces/{ns}/pods`: list pods in a namespace.
 
Error responses:
- `405`: Wrong HTTP method
- `400`: Malformed path
- `503`: Kubernetes client unavailable or list operation failed

## Curl Examples

Health:

```bash
curl -i http://127.0.0.1:8080/health
```

Restart:

```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/does-not-exist/restart
curl -i -X POST http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/hello/restart
curl -i -X POST http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/hello-allowed/restart
```

Delete deployment:

```bash
curl -i -X DELETE http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/does-not-exist/delete
curl -i -X DELETE http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/hello/delete
curl -i -X DELETE http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/hello-allowed/delete
```

Delete success response format (`200` JSON):

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

Delete error response format (JSON):
- `400` malformed path: `{"error":"malformed path"}`
- `403` label auth fail: includes `required_label` and `deployment_label`
- `404` deployment not found: includes `namespace`, `deployment`, `action`
- `500` Kubernetes/client failure: includes `details`

List deployments in namespace:

```bash
curl -i http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments
```

List deployments success response format (`200` JSON):

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

List pods in namespace:

```bash
curl -i http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods
```

List pods success response format (`200` JSON):

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

Logs:

```bash
curl -i http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/does-not-exist/logs
curl -i http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs

# Default behavior: tailLines=100 (latest 100 lines)
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs"

# Last 500 lines
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs?tailLines=500"

# Logs from last 10 minutes
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs?sinceSeconds=600"

# Stream live logs (follow)
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs?follow=true"

# Cap total bytes returned
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs?limitBytes=1048576"

# Include per-line RFC3339 timestamps
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs?timestamps=true"

# Combine options for high-volume debugging
curl -i "http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs?tailLines=200&sinceSeconds=300&timestamps=true"
```

Logs query parameters:
- `tailLines` (int): number of recent lines to return; default is `100`.
- `sinceSeconds` (int): only return logs newer than N seconds.
- `follow` (bool): stream logs until the client disconnects.
- `limitBytes` (int): cap the number of bytes returned.
- `timestamps` (bool): include timestamps on each log line.

Pod status:

```bash
curl -i http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/does-not-exist/pods/status
curl -i http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/hello/pods/status
curl -i http://127.0.0.1:8080/api/v1/namespaces/proxy-test/deployments/hello-allowed/pods/status
```

## Pilot Deployment Notes

Initial namespace target: `proxy-test`.

Container image tag used by provided manifest:
- `k8s-proxy-api:local`

Build and import image for local k3s:

```bash
docker build -t k8s-proxy-api:local .
docker save k8s-proxy-api:local -o /tmp/k8s-proxy-api-local.tar
sudo k3s ctr images import /tmp/k8s-proxy-api-local.tar
```

Apply manifest:

```bash
kubectl apply -f k8s/proxy-test.yaml
kubectl -n proxy-test rollout status deployment/k8s-proxy-api
```

Port-forward for pilot testing:

```bash
kubectl -n proxy-test port-forward svc/k8s-proxy-api 8080:8080
```

Then run the curl examples above against `http://127.0.0.1:8080`.
```
