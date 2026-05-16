package handlers

import (
	"net/http"

	"github.com/iotyak/k8s-proxy-api/internal/auth"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func DeleteDeploymentHandler(kubeClient kubernetes.Interface, kubeInitErr string, splitPathStrict func(string) ([]string, error)) http.HandlerFunc {
	if splitPathStrict == nil {
		splitPathStrict = splitPathFallback
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		parts, err := splitPathStrict(r.URL.Path)
		if err != nil || len(parts) != 7 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[4] != "deployments" || parts[6] != "delete" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed path"})
			return
		}

		namespace := parts[3]
		deploymentName := parts[5]

		if kubeClient == nil {
			errMsg := "kubernetes client not initialized"
			if kubeInitErr != "" {
				errMsg += ": " + kubeInitErr
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to read deployment", "details": errMsg, "namespace": namespace, "deployment": deploymentName, "action": "delete"})
			return
		}

		dep, err := kubeClient.AppsV1().Deployments(namespace).Get(r.Context(), deploymentName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "deployment not found", "namespace": namespace, "deployment": deploymentName, "action": "delete"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to read deployment", "details": err.Error(), "namespace": namespace, "deployment": deploymentName, "action": "delete"})
			return
		}

		if !auth.CheckDeploymentAllowed(dep) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "deployment is not allowed for proxy access", "namespace": namespace, "deployment": deploymentName, "action": "delete", "required_label": auth.ProxyAccessLabelKey + "=" + auth.ProxyAccessLabelValue, "deployment_label": auth.DeploymentLabelValue(dep)})
			return
		}

		if err := kubeClient.AppsV1().Deployments(namespace).Delete(r.Context(), deploymentName, metav1.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "deployment not found", "namespace": namespace, "deployment": deploymentName, "action": "delete"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to delete deployment", "details": err.Error(), "namespace": namespace, "deployment": deploymentName, "action": "delete"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success":          true,
			"authorized":       true,
			"namespace":        namespace,
			"deployment":       deploymentName,
			"action":           "delete",
			"message":          "deployment deleted",
			"required_label":   auth.ProxyAccessLabelKey + "=" + auth.ProxyAccessLabelValue,
			"deployment_label": auth.DeploymentLabelValue(dep),
		})
	}
}
