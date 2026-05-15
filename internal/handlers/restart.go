package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	proxyAccessLabelKey   = "proxy-access"
	proxyAccessLabelValue = "allowed"
	restartedAtAnnotation = "kubectl.kubernetes.io/restartedAt"
)

var errInvalidPath = errors.New("invalid path")

func RestartHandler(kubeClient kubernetes.Interface, kubeInitErr string, splitPathStrict func(string) ([]string, error), now func() time.Time) http.HandlerFunc {
	if splitPathStrict == nil {
		splitPathStrict = splitPathFallback
	}
	if now == nil {
		now = time.Now
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		parts, err := splitPathStrict(r.URL.Path)
		if err != nil || len(parts) != 7 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[4] != "deployments" || parts[6] != "restart" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed path"})
			return
		}

		namespace := parts[3]
		deploymentName := parts[5]

		dep, ok := getAuthorizedDeployment(r.Context(), kubeClient, kubeInitErr, namespace, deploymentName, w)
		if !ok {
			return
		}

		restartedAt := now().UTC().Format(time.RFC3339)
		patch, err := json.Marshal(map[string]any{
			"spec": map[string]any{
				"template": map[string]any{
					"metadata": map[string]any{
						"annotations": map[string]string{restartedAtAnnotation: restartedAt},
					},
				},
			},
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to build restart patch", "details": err.Error()})
			return
		}

		if _, err := kubeClient.AppsV1().Deployments(namespace).Patch(r.Context(), deploymentName, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to patch deployment restart annotation", "details": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success":          true,
			"authorized":       true,
			"namespace":        namespace,
			"deployment":       deploymentName,
			"action":           "restart",
			"restarted_at":     restartedAt,
			"message":          "deployment restart annotation patched",
			"required_label":   proxyAccessLabelKey + "=" + proxyAccessLabelValue,
			"deployment_label": deploymentLabelValue(dep),
		})
	}
}

func getAuthorizedDeployment(ctx context.Context, kubeClient kubernetes.Interface, kubeInitErr, namespace, name string, w http.ResponseWriter) (*appsv1.Deployment, bool) {
	if kubeClient == nil {
		errMsg := "kubernetes client not initialized"
		if kubeInitErr != "" {
			errMsg += ": " + kubeInitErr
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "failed to read deployment", "details": errMsg})
		return nil, false
	}

	dep, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "deployment not found", "namespace": namespace, "deployment": name, "action": "restart"})
			return nil, false
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "failed to read deployment", "details": err.Error(), "namespace": namespace, "deployment": name, "action": "restart"})
		return nil, false
	}

	if dep.Labels[proxyAccessLabelKey] != proxyAccessLabelValue {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "deployment is not allowed for proxy access", "namespace": namespace, "deployment": name, "action": "restart", "required_label": proxyAccessLabelKey + "=" + proxyAccessLabelValue, "deployment_label": deploymentLabelValue(dep)})
		return nil, false
	}

	return dep, true
}

func deploymentLabelValue(dep *appsv1.Deployment) string {
	if dep == nil || dep.Labels == nil {
		return ""
	}
	return dep.Labels[proxyAccessLabelKey]
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func splitPathFallback(path string) ([]string, error) {
	if path == "" || path[0] != '/' {
		return nil, errInvalidPath
	}
	parts := make([]string, 0)
	start := 1
	for i := 1; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i == start {
				return nil, errInvalidPath
			}
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	return parts, nil
}
