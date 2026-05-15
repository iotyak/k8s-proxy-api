package handlers

import (
	"context"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type podStatusItem struct {
	Name              string `json:"name"`
	Phase             string `json:"phase"`
	ReadyContainers   int    `json:"ready_containers"`
	TotalContainers   int    `json:"total_containers"`
	TotalRestartCount int32  `json:"total_restart_count"`
}

func StatusHandler(kubeClient kubernetes.Interface, kubeInitErr string, splitPathStrict func(string) ([]string, error)) http.HandlerFunc {
	if splitPathStrict == nil {
		splitPathStrict = splitPathFallback
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		parts, err := splitPathStrict(r.URL.Path)
		if err != nil || len(parts) != 8 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[4] != "deployments" || parts[6] != "pods" || parts[7] != "status" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed path"})
			return
		}

		namespace := parts[3]
		deploymentName := parts[5]

		dep, ok := getAuthorizedDeployment(r.Context(), kubeClient, kubeInitErr, namespace, deploymentName, w)
		if !ok {
			return
		}

		pods, selector, routeErr := listDeploymentPodsStatus(r.Context(), kubeClient, kubeInitErr, namespace, dep)
		if routeErr != nil {
			writeLogsRouteError(w, routeErr, map[string]any{
				"namespace":  namespace,
				"deployment": deploymentName,
				"action":     "pods-status",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"namespace":  namespace,
			"deployment": deploymentName,
			"action":     "pods-status",
			"selector":   selector,
			"pod_count":  len(pods),
			"pods":       pods,
		})
	}
}

func listDeploymentPodsStatus(ctx context.Context, kubeClient kubernetes.Interface, kubeInitErr, namespace string, dep *appsv1.Deployment) ([]podStatusItem, string, *logsRouteError) {
	if kubeClient == nil {
		errMsg := "kubernetes client not initialized"
		if kubeInitErr != "" {
			errMsg += ": " + kubeInitErr
		}
		return nil, "", &logsRouteError{status: http.StatusServiceUnavailable, message: errMsg}
	}

	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return nil, "", &logsRouteError{status: http.StatusInternalServerError, message: "invalid deployment selector", details: map[string]any{"details": err.Error()}}
	}

	pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, selector.String(), &logsRouteError{status: http.StatusServiceUnavailable, message: "failed to list pods for deployment", details: map[string]any{"details": err.Error()}}
	}

	out := make([]podStatusItem, 0, len(pods.Items))
	for _, p := range pods.Items {
		readyCount := 0
		var restartCount int32
		for _, cs := range p.Status.ContainerStatuses {
			if cs.Ready {
				readyCount++
			}
			restartCount += cs.RestartCount
		}

		out = append(out, podStatusItem{
			Name:              p.Name,
			Phase:             string(p.Status.Phase),
			ReadyContainers:   readyCount,
			TotalContainers:   len(p.Status.ContainerStatuses),
			TotalRestartCount: restartCount,
		})
	}

	return out, selector.String(), nil
}
