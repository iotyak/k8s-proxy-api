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

The service listens on:
- `http://127.0.0.1:8080`

## Routes

Health route:
- `GET /health`

Operational routes (versioned):
- `POST /api/v1/namespaces/{ns}/deployments/{name}/restart`
- `GET /api/v1/namespaces/{ns}/pods/{pod}/logs`
- `GET /api/v1/namespaces/{ns}/deployments/{name}/pods/status`

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

Logs:

```bash
curl -i http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/does-not-exist/logs
curl -i http://127.0.0.1:8080/api/v1/namespaces/proxy-test/pods/<POD_NAME>/logs
```

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
