package handlers

import (
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type deploymentListItem struct {
	Name              string `json:"name"`
	ReadyReplicas     int32  `json:"ready_replicas"`
	AvailableReplicas int32  `json:"available_replicas"`
	Replicas          int32  `json:"replicas"`
}

func ListDeploymentsHandler(kubeClient kubernetes.Interface, kubeInitErr string, splitPathStrict func(string) ([]string, error)) http.HandlerFunc {
	if splitPathStrict == nil {
		splitPathStrict = splitPathFallback
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		parts, err := splitPathStrict(r.URL.Path)
		if err != nil || len(parts) != 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[4] != "deployments" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed path"})
			return
		}

		namespace := parts[3]

		if kubeClient == nil {
			errMsg := "kubernetes client not initialized"
			if kubeInitErr != "" {
				errMsg += ": " + kubeInitErr
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": errMsg, "namespace": namespace, "action": "list-deployments"})
			return
		}

		deployments, err := kubeClient.AppsV1().Deployments(namespace).List(r.Context(), metav1.ListOptions{})
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "failed to list deployments", "details": err.Error(), "namespace": namespace, "action": "list-deployments"})
			return
		}

		items := make([]deploymentListItem, 0, len(deployments.Items))
		for _, dep := range deployments.Items {
			items = append(items, deploymentListItem{
				Name:              dep.Name,
				ReadyReplicas:     dep.Status.ReadyReplicas,
				AvailableReplicas: dep.Status.AvailableReplicas,
				Replicas:          dep.Status.Replicas,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"namespace":  namespace,
			"action":     "list-deployments",
			"count":      len(items),
			"deployments": items,
		})
	}
}
