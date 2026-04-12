# k8s-proxy-api Specs

## Overview
`k8s-proxy-api` is a small internal Go service that exposes a tightly scoped HTTP API for a limited set of Kubernetes operations.

The goal is **not** to provide general Kubernetes API access. The goal is to provide a narrow control plane that can later be consumed by another internal UI or service.

This project is being built incrementally. The current phase focuses on:
- a minimal stdlib HTTP server
- local development against a kubeconfig
- future support for in-cluster execution
- health reporting for Kubernetes connectivity

## Current Phase Goal
In this phase, the service must:
- start a small HTTP server using Go's standard library
- support `GET /health`
- attempt to initialize Kubernetes client-go using:
  1. `KUBECONFIG` if set
  2. in-cluster config otherwise
- create a Kubernetes clientset during startup
- keep the HTTP server running even if Kubernetes is unreachable
- report Kubernetes connectivity state in the `/health` response

This phase is about proving the application structure and Kubernetes connectivity model before implementing any real cluster actions.

## Design Principles
- Keep the code small and readable
- Use stdlib `net/http`
- Avoid third-party routers
- Prefer small helper functions over broad abstractions
- Do not couple server startup to successful Kubernetes API connectivity
- Return structured JSON responses
- Build incrementally so each phase can be tested independently

## Runtime Configuration
The service should determine Kubernetes configuration as follows:

1. If the `KUBECONFIG` environment variable is set:
   - load config from that file

2. Otherwise:
   - attempt to use in-cluster Kubernetes config

This allows practical local development on a laptop or dev box while also supporting later deployment inside Kubernetes.

### Local Development Example
```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
go run .
