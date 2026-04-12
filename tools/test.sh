#!/usr/bin/env bash
set -euo pipefail

NS="proxy-test"
BASE="http://127.0.0.1:8080"

ALLOWED_POD="$(kubectl -n "$NS" get pods -l app=hello-allowed -o jsonpath='{.items[0].metadata.name}')"
UNLABELED_POD="$(kubectl -n "$NS" get pods -l app=hello -o jsonpath='{.items[0].metadata.name}')"

echo
echo "== Restart tests =="
curl -i -X POST "$BASE/api/v1/namespaces/$NS/deployments/does-not-exist/restart"
echo
curl -i -X POST "$BASE/api/v1/namespaces/$NS/deployments/hello/restart"
echo
curl -i -X POST "$BASE/api/v1/namespaces/$NS/deployments/hello-allowed/restart"
echo

echo
echo "== Logs tests =="
curl -i "$BASE/api/v1/namespaces/$NS/pods/does-not-exist/logs"
echo
curl -i "$BASE/api/v1/namespaces/$NS/pods/$UNLABELED_POD/logs"
echo
curl -i "$BASE/api/v1/namespaces/$NS/pods/$ALLOWED_POD/logs"
echo

echo
echo "== Pod status tests =="
curl -i "$BASE/api/v1/namespaces/$NS/deployments/does-not-exist/pods/status"
echo
curl -i "$BASE/api/v1/namespaces/$NS/deployments/hello/pods/status"
echo
curl -i "$BASE/api/v1/namespaces/$NS/deployments/hello-allowed/pods/status"
echo
