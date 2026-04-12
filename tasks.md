
`tasks.md`
```md
# k8s-proxy-api Tasks

## Project Workflow
Implementation should stay incremental.
Each task should be completed, tested, and committed before moving to the next one.

## Completed
- [x] Initialize the Go module
- [x] Create a minimal stdlib HTTP server
- [x] Add `GET /health`
- [x] Return a basic JSON health response
- [x] Add basic request logging
- [x] Confirm the server runs locally and responds on `127.0.0.1:8080`

## Complted
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

## Completed
### Placeholder API structure
- [x] Add placeholder route:
  - [x] `POST /namespaces/{ns}/deployments/{name}/restart`
- [x] Add placeholder route:
  - [x] `GET /namespaces/{ns}/pods/{pod}/logs`
- [x] Parse path segments explicitly and safely
- [x] Return structured JSON errors for malformed paths

## Current Tasks
### Deployment authorization helper
- [x] Add helper to get a Deployment by namespace/name
- [x] Add helper to verify label `proxy-access=allowed`
- [x] Return `404` when Deployment is missing
- [x] Return `403` when label is missing or not allowed

### Safe Deployment restart
- [x] Implement restart handler using a narrow patch
- [x] Patch `spec.template.metadata.annotations`
- [x] Set `kubectl.kubernetes.io/restarted-at`
- [x] Return clear JSON success/error responses

### Pod ownership resolution
- [x] Add helper to resolve Pod ownership:
  - [x] Pod -> ReplicaSet -> Deployment
- [x] Return clear errors when ownership cannot be resolved

### Pod logs
- [x] Implement `GET /namespaces/{ns}/pods/{pod}/logs`
- [x] Authorize access through owning Deployment label check
- [x] Stream logs directly to the HTTP response
- [x] Avoid buffering the entire log output in memory

### Pod status
- [x] Add deployment-scoped endpoint:
  - [x] `GET /namespaces/{ns}/deployments/{name}/pods/status`
- [x] List pods belonging to the Deployment
- [x] Return:
  - [x] pod name
  - [x] phase
  - [x] pod IP
  - [x] conditions
  - [x] container readiness
  - [x] restart counts

### Packaging and deployment
- [x] Add a minimal Dockerfile
- [x] Prefer a small non-root image
- [x] Generate Kubernetes manifests:
  - [x] ServiceAccount
  - [x] Role
  - [x] RoleBinding
  - [x] Deployment
  - [x] Service

## Guardrails
These should stay true as the project grows:
- [ ] Do not add third-party routers unless there is a strong reason
- [ ] Do not expose general Kubernetes API access
- [ ] Keep RBAC as narrow as possible
- [ ] Keep authorization logic explicit
- [ ] Keep code understandable enough to maintain without a framework
- [ ] Test each phase before moving to the next one

## Suggested Commit Milestones
- [ ] `feat: add kube client initialization and health reporting`
- [ ] `feat: add placeholder restart and logs routes`
- [ ] `feat: add deployment label authorization`
- [ ] `feat: implement deployment restart patch`
- [ ] `feat: implement pod ownership resolution and logs streaming`
- [ ] `feat: add deployment pod status endpoint`
- [ ] `feat: add container build and kubernetes manifests`
