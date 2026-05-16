package handlers

import (
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type podListItem struct {
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	PodIP     string `json:"pod_ip,omitempty"`
	NodeName  string `json:"node_name,omitempty"`
	HostIP    string `json:"host_ip,omitempty"`
	StartTime string `json:"start_time,omitempty"`
}

func ListPodsHandler(kubeClient kubernetes.Interface, kubeInitErr string, splitPathStrict func(string) ([]string, error)) http.HandlerFunc {
	if splitPathStrict == nil {
		splitPathStrict = splitPathFallback
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		parts, err := splitPathStrict(r.URL.Path)
		if err != nil || len(parts) != 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[4] != "pods" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed path"})
			return
		}

		namespace := parts[3]

		if kubeClient == nil {
			errMsg := "kubernetes client not initialized"
			if kubeInitErr != "" {
				errMsg += ": " + kubeInitErr
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": errMsg, "namespace": namespace, "action": "list-pods"})
			return
		}

		pods, err := kubeClient.CoreV1().Pods(namespace).List(r.Context(), metav1.ListOptions{})
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "failed to list pods", "details": err.Error(), "namespace": namespace, "action": "list-pods"})
			return
		}

		items := make([]podListItem, 0, len(pods.Items))
		for _, pod := range pods.Items {
			item := podListItem{
				Name:     pod.Name,
				Phase:    string(pod.Status.Phase),
				PodIP:    pod.Status.PodIP,
				NodeName: pod.Spec.NodeName,
				HostIP:   pod.Status.HostIP,
			}
			if pod.Status.StartTime != nil {
				item.StartTime = pod.Status.StartTime.UTC().Format("2006-01-02T15:04:05Z")
			}
			items = append(items, item)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"namespace": namespace,
			"action":    "list-pods",
			"count":     len(items),
			"pods":      items,
		})
	}
}
