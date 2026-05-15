package handlers

import (
	"io"
	"net/http"

	"github.com/iotyak/k8s-proxy-api/internal/auth"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type logsRouteError struct {
	status  int
	message string
	details map[string]any
}

func LogsHandler(kubeClient kubernetes.Interface, kubeInitErr string, splitPathStrict func(string) ([]string, error), openLogStream func(string, string) (io.ReadCloser, error)) http.HandlerFunc {
	if splitPathStrict == nil {
		splitPathStrict = splitPathFallback
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		parts, err := splitPathStrict(r.URL.Path)
		if err != nil || len(parts) != 7 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[4] != "pods" || parts[6] != "logs" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed path"})
			return
		}

		namespace := parts[3]
		podName := parts[5]

		podName, replicaSetName, dep, routeErr := resolvePodOwningDeployment(r, kubeClient, kubeInitErr, namespace, podName)
		if routeErr != nil {
			fields := map[string]any{"namespace": namespace, "pod": podName, "action": "logs"}
			if replicaSetName != "" {
				fields["replicaset"] = replicaSetName
			}
			writeLogsRouteError(w, routeErr, fields)
			return
		}

		if !auth.CheckDeploymentAllowed(dep) {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":            "deployment is not allowed for proxy access",
				"namespace":        namespace,
				"pod":              podName,
				"replicaset":       replicaSetName,
				"deployment":       dep.Name,
				"action":           "logs",
				"required_label":   auth.ProxyAccessLabelKey + "=" + auth.ProxyAccessLabelValue,
				"deployment_label": auth.DeploymentLabelValue(dep),
			})
			return
		}

		if err := streamPodLogs(r, kubeClient, kubeInitErr, namespace, podName, openLogStream, w); err != nil {
			writeLogsRouteError(w, err, map[string]any{
				"namespace":  namespace,
				"pod":        podName,
				"replicaset": replicaSetName,
				"deployment": dep.Name,
				"action":     "logs",
			})
		}
	}
}

func writeLogsRouteError(w http.ResponseWriter, routeErr *logsRouteError, fields map[string]any) {
	resp := map[string]any{"error": routeErr.message}
	for k, v := range fields {
		resp[k] = v
	}
	for k, v := range routeErr.details {
		resp[k] = v
	}
	writeJSON(w, routeErr.status, resp)
}

func findOwnerReference(ownerRefs []metav1.OwnerReference, kind string) (metav1.OwnerReference, bool) {
	for _, ownerRef := range ownerRefs {
		if ownerRef.Kind == kind {
			return ownerRef, true
		}
	}
	return metav1.OwnerReference{}, false
}

func resolvePodOwningDeployment(r *http.Request, kubeClient kubernetes.Interface, kubeInitErr, namespace, podName string) (string, string, *appsv1.Deployment, *logsRouteError) {
	if kubeClient == nil {
		errMsg := "kubernetes client not initialized"
		if kubeInitErr != "" {
			errMsg += ": " + kubeInitErr
		}
		return podName, "", nil, &logsRouteError{status: http.StatusServiceUnavailable, message: errMsg}
	}

	pod, err := kubeClient.CoreV1().Pods(namespace).Get(r.Context(), podName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return podName, "", nil, &logsRouteError{status: http.StatusNotFound, message: "pod not found"}
		}
		return podName, "", nil, &logsRouteError{status: http.StatusServiceUnavailable, message: "failed to read pod", details: map[string]any{"details": err.Error()}}
	}

	if len(pod.OwnerReferences) == 0 {
		return pod.Name, "", nil, &logsRouteError{status: http.StatusBadRequest, message: "pod has no owner references"}
	}

	rsOwnerRef, ok := findOwnerReference(pod.OwnerReferences, "ReplicaSet")
	if !ok {
		return pod.Name, "", nil, &logsRouteError{status: http.StatusBadRequest, message: "pod is not owned by a ReplicaSet"}
	}

	rs, err := kubeClient.AppsV1().ReplicaSets(namespace).Get(r.Context(), rsOwnerRef.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return pod.Name, rsOwnerRef.Name, nil, &logsRouteError{status: http.StatusNotFound, message: "replicaset owner not found"}
		}
		return pod.Name, rsOwnerRef.Name, nil, &logsRouteError{status: http.StatusServiceUnavailable, message: "failed to read replicaset", details: map[string]any{"details": err.Error()}}
	}

	if len(rs.OwnerReferences) == 0 {
		return pod.Name, rs.Name, nil, &logsRouteError{status: http.StatusBadRequest, message: "replicaset has no Deployment owner"}
	}

	depOwnerRef, ok := findOwnerReference(rs.OwnerReferences, "Deployment")
	if !ok {
		return pod.Name, rs.Name, nil, &logsRouteError{status: http.StatusBadRequest, message: "replicaset has no Deployment owner"}
	}

	dep, err := kubeClient.AppsV1().Deployments(namespace).Get(r.Context(), depOwnerRef.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return pod.Name, rs.Name, nil, &logsRouteError{status: http.StatusNotFound, message: "deployment owner not found"}
		}
		return pod.Name, rs.Name, nil, &logsRouteError{status: http.StatusServiceUnavailable, message: "failed to read deployment", details: map[string]any{"details": err.Error()}}
	}

	return pod.Name, rs.Name, dep, nil
}

func streamPodLogs(r *http.Request, kubeClient kubernetes.Interface, kubeInitErr, namespace, podName string, openLogStream func(string, string) (io.ReadCloser, error), w http.ResponseWriter) *logsRouteError {
	if kubeClient == nil {
		errMsg := "kubernetes client not initialized"
		if kubeInitErr != "" {
			errMsg += ": " + kubeInitErr
		}
		return &logsRouteError{status: http.StatusServiceUnavailable, message: errMsg}
	}

	if openLogStream == nil {
		openLogStream = func(ns, pod string) (io.ReadCloser, error) {
			return kubeClient.CoreV1().Pods(ns).GetLogs(pod, nil).Stream(r.Context())
		}
	}

	stream, err := openLogStream(namespace, podName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &logsRouteError{status: http.StatusNotFound, message: "pod not found"}
		}
		return &logsRouteError{status: http.StatusServiceUnavailable, message: "failed to open pod logs stream", details: map[string]any{"details": err.Error()}}
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, stream)

	return nil
}
