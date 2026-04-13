# AGENTS

## What Matters First
- Single-binary service; all runtime logic is in `main.go` (no package split).
- Keep `GET /health` unversioned; operational APIs are under `/api/v1/...`.
- Path parsing is intentionally strict (`splitPathStrict`): rejects double slashes and trailing slash forms.

## Local Run / Verify
- Local dev assumes kube access via `KUBECONFIG`:
  - `export KUBECONFIG=/etc/rancher/k3s/k3s.yaml`
  - `go run .`
- Fast verification command used in this repo: `gofmt -w main.go && go test ./...`.
- There are currently no Go test files; `go test ./...` is mainly a build check.

## API Behavior You Must Preserve
- `GET /health` returns JSON app/kubernetes connectivity state.
- `POST /api/v1/namespaces/{ns}/deployments/{name}/restart` returns JSON.
- `GET /api/v1/namespaces/{ns}/pods/{pod}/logs` returns plain text log stream on success, JSON on failures.
- `GET /api/v1/namespaces/{ns}/deployments/{name}/pods/status` returns JSON.
- Deployment authorization is label-based and explicit: `proxy-access=allowed`.

## Kubernetes / RBAC Scope
- Keep scope narrow; current code only needs:
  - deployments: `get`, `patch`
  - pods: `get`, `list`
  - pods/log: `get`
  - replicasets: `get`
- Primary manifest for pilot is `k8s/proxy-test.yaml` (namespace `proxy-test`, image `k8s-proxy-api:local`).
- `k8s/deployment.yaml` exists but is older and not the full RBAC/service bundle.

## Existing Script Assumptions
- `tools/test.sh` is the repo's e2e smoke script and assumes:
  - namespace `proxy-test`
  - deployments `hello` and `hello-allowed`
  - matching pods exist for selectors `app=hello` and `app=hello-allowed`.
