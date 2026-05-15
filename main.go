package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/iotyak/k8s-proxy-api/internal/auth"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/iotyak/k8s-proxy-api/internal/handlers"
	"github.com/iotyak/k8s-proxy-api/internal/k8sclient"
)

type namespaceRoute struct {
	namespace string
	target    string
	kind      string
}

type appState struct {
	kubeClient    kubernetes.Interface
	kubeInitErr   string
	openLogStream func(string, string) (io.ReadCloser, error)
}

type routeError struct {
	status  int
	message string
	details map[string]any
}

type podConditionStatus struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type podContainerStatus struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
}

type podStatusItem struct {
	Name              string               `json:"name"`
	Phase             string               `json:"phase"`
	PodIP             string               `json:"pod_ip,omitempty"`
	Conditions        []podConditionStatus `json:"conditions"`
	Containers        []podContainerStatus `json:"containers"`
	ReadyContainers   int                  `json:"ready_containers"`
	TotalContainers   int                  `json:"total_containers"`
	TotalRestartCount int32                `json:"total_restart_count"`
}

type healthResponse struct {
	AppStatus           string `json:"app_status"`
	KubernetesReachable bool   `json:"kubernetes_reachable"`
	KubernetesVersion   string `json:"kubernetes_version,omitempty"`
	KubernetesError     string `json:"kubernetes_error,omitempty"`
}

const (
	RouteRestart = "restart"
	RouteLogs    = "logs"
	RoutePods    = "pods-status"
	RouteDelete  = "delete"
)

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
		log.Printf("completed in %s", time.Since(start))
	})
}

func (a *appState) getDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	if a.kubeClient == nil {
		if a.kubeInitErr != "" {
			return nil, fmt.Errorf("kubernetes client not initialized: %s", a.kubeInitErr)
		}
		return nil, fmt.Errorf("kubernetes client not initialized")
	}

	return a.kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

func writeJSONErrorFields(w http.ResponseWriter, status int, msg string, fields map[string]any) {
	resp := map[string]any{"error": msg}
	for k, v := range fields {
		resp[k] = v
	}
	writeJSON(w, status, resp)
}

func writeRouteError(w http.ResponseWriter, routeErr *routeError, fields map[string]any) {
	resp := map[string]any{"error": routeErr.message}
	for k, v := range fields {
		resp[k] = v
	}
	if routeErr.details != nil {
		for k, v := range routeErr.details {
			resp[k] = v
		}
	}
	writeJSON(w, routeErr.status, resp)
}

func (a *appState) getAuthorizedDeployment(ctx context.Context, namespace, name, action string, extra map[string]any, w http.ResponseWriter) (*appsv1.Deployment, bool) {
	// Restart endpoint walkthrough note:
	// - deployment lookup via Kubernetes client
	// - proxy-access label authorization gate
	// This helper returns endpoint-shaped JSON errors for each failure path.
	dep, err := a.getDeployment(ctx, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			fields := map[string]any{"namespace": namespace, "deployment": name, "action": action}
			for k, v := range extra {
				fields[k] = v
			}
			writeJSONErrorFields(w, http.StatusNotFound, "deployment not found", fields)
			return nil, false
		}

		fields := map[string]any{"namespace": namespace, "deployment": name, "action": action, "details": err.Error()}
		for k, v := range extra {
			fields[k] = v
		}
		writeJSONErrorFields(w, http.StatusServiceUnavailable, "failed to read deployment", fields)
		return nil, false
	}

	deploymentLabel := auth.DeploymentLabelValue(dep)
	if !auth.CheckDeploymentAllowed(dep) {
		fields := map[string]any{
			"namespace":        namespace,
			"deployment":       name,
			"action":           action,
			"required_label":   auth.ProxyAccessLabelKey + "=" + auth.ProxyAccessLabelValue,
			"deployment_label": deploymentLabel,
		}
		for k, v := range extra {
			fields[k] = v
		}
		writeJSONErrorFields(w, http.StatusForbidden, "deployment is not allowed for proxy access", fields)
		return nil, false
	}

	return dep, true
}

func findOwnerReference(ownerRefs []metav1.OwnerReference, kind string) (metav1.OwnerReference, bool) {
	for _, ownerRef := range ownerRefs {
		if ownerRef.Kind == kind {
			return ownerRef, true
		}
	}
	return metav1.OwnerReference{}, false
}

func (a *appState) resolvePodOwningDeployment(ctx context.Context, namespace, podName string) (string, string, *appsv1.Deployment, *routeError) {
	if a.kubeClient == nil {
		errMsg := "kubernetes client not initialized"
		if a.kubeInitErr != "" {
			errMsg = errMsg + ": " + a.kubeInitErr
		}
		return "", "", nil, &routeError{status: http.StatusServiceUnavailable, message: errMsg}
	}

	pod, err := a.kubeClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", nil, &routeError{status: http.StatusNotFound, message: "pod not found"}
		}
		return "", "", nil, &routeError{status: http.StatusServiceUnavailable, message: "failed to read pod", details: map[string]any{"details": err.Error()}}
	}

	if len(pod.OwnerReferences) == 0 {
		return pod.Name, "", nil, &routeError{status: http.StatusBadRequest, message: "pod has no owner references"}
	}

	rsOwnerRef, ok := findOwnerReference(pod.OwnerReferences, "ReplicaSet")
	if !ok {
		return pod.Name, "", nil, &routeError{status: http.StatusBadRequest, message: "pod is not owned by a ReplicaSet"}
	}

	rs, err := a.kubeClient.AppsV1().ReplicaSets(namespace).Get(ctx, rsOwnerRef.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return pod.Name, rsOwnerRef.Name, nil, &routeError{status: http.StatusNotFound, message: "replicaset owner not found"}
		}
		return pod.Name, rsOwnerRef.Name, nil, &routeError{status: http.StatusServiceUnavailable, message: "failed to read replicaset", details: map[string]any{"details": err.Error()}}
	}

	if len(rs.OwnerReferences) == 0 {
		return pod.Name, rs.Name, nil, &routeError{status: http.StatusBadRequest, message: "replicaset has no Deployment owner"}
	}

	depOwnerRef, ok := findOwnerReference(rs.OwnerReferences, "Deployment")
	if !ok {
		return pod.Name, rs.Name, nil, &routeError{status: http.StatusBadRequest, message: "replicaset has no Deployment owner"}
	}

	dep, err := a.getDeployment(ctx, namespace, depOwnerRef.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return pod.Name, rs.Name, nil, &routeError{status: http.StatusNotFound, message: "deployment owner not found"}
		}
		return pod.Name, rs.Name, nil, &routeError{status: http.StatusServiceUnavailable, message: "failed to read deployment", details: map[string]any{"details": err.Error()}}
	}

	return pod.Name, rs.Name, dep, nil
}

func (a *appState) streamPodLogs(ctx context.Context, namespace, podName string, w http.ResponseWriter) *routeError {
	if a.kubeClient == nil {
		errMsg := "kubernetes client not initialized"
		if a.kubeInitErr != "" {
			errMsg = errMsg + ": " + a.kubeInitErr
		}
		return &routeError{status: http.StatusServiceUnavailable, message: errMsg}
	}

	stream, err := a.kubeClient.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &routeError{status: http.StatusNotFound, message: "pod not found"}
		}
		return &routeError{status: http.StatusServiceUnavailable, message: "failed to open pod logs stream", details: map[string]any{"details": err.Error()}}
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, stream); err != nil {
		log.Printf("failed while streaming pod logs for %s/%s: %v", namespace, podName, err)
	}

	return nil
}

func (a *appState) listDeploymentPodsStatus(ctx context.Context, namespace string, dep *appsv1.Deployment) ([]podStatusItem, string, *routeError) {
	if a.kubeClient == nil {
		errMsg := "kubernetes client not initialized"
		if a.kubeInitErr != "" {
			errMsg = errMsg + ": " + a.kubeInitErr
		}
		return nil, "", &routeError{status: http.StatusServiceUnavailable, message: errMsg}
	}

	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return nil, "", &routeError{
			status:  http.StatusInternalServerError,
			message: "invalid deployment selector",
			details: map[string]any{"details": err.Error()},
		}
	}

	pods, err := a.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, selector.String(), &routeError{
			status:  http.StatusServiceUnavailable,
			message: "failed to list pods for deployment",
			details: map[string]any{"details": err.Error()},
		}
	}

	out := make([]podStatusItem, 0, len(pods.Items))
	for _, p := range pods.Items {
		conditions := make([]podConditionStatus, 0, len(p.Status.Conditions))
		for _, c := range p.Status.Conditions {
			conditions = append(conditions, podConditionStatus{
				Type:    string(c.Type),
				Status:  string(c.Status),
				Reason:  c.Reason,
				Message: c.Message,
			})
		}

		containers := make([]podContainerStatus, 0, len(p.Status.ContainerStatuses))
		readyCount := 0
		var restartCount int32
		for _, cs := range p.Status.ContainerStatuses {
			if cs.Ready {
				readyCount++
			}
			restartCount += cs.RestartCount
			containers = append(containers, podContainerStatus{
				Name:         cs.Name,
				Ready:        cs.Ready,
				RestartCount: cs.RestartCount,
			})
		}

		out = append(out, podStatusItem{
			Name:              p.Name,
			Phase:             string(p.Status.Phase),
			PodIP:             p.Status.PodIP,
			Conditions:        conditions,
			Containers:        containers,
			ReadyContainers:   readyCount,
			TotalContainers:   len(p.Status.ContainerStatuses),
			TotalRestartCount: restartCount,
		})
	}

	return out, selector.String(), nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func SplitPathStrict(path string) ([]string, error) {
	if path == "" || path[0] != '/' {
		return nil, errors.New("invalid path")
	}
	if strings.Contains(path, "//") {
		return nil, errors.New("invalid path")
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		return nil, errors.New("invalid path")
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return nil, errors.New("invalid path")
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, errors.New("invalid path")
		}
	}

	return parts, nil
}

func parseNamespaceRoute(path string) (namespaceRoute, bool) {
	parts, err := SplitPathStrict(path)
	if err != nil {
		return namespaceRoute{}, false
	}
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "v1" {
		return namespaceRoute{}, false
	}

	parts = parts[2:]
	if len(parts) == 5 && parts[0] == "namespaces" {
		ns := parts[1]
		resource := parts[2]
		name := parts[3]
		action := parts[4]

		if resource == "deployments" && action == RouteRestart {
			// Restart endpoint walkthrough note: exact route shape match
			// /api/v1/namespaces/{ns}/deployments/{name}/restart
			return namespaceRoute{
				namespace: ns,
				target:    name,
				kind:      RouteRestart,
			}, true
		}
		if resource == "deployments" && action == RouteDelete {
			return namespaceRoute{
				namespace: ns,
				target:    name,
				kind:      RouteDelete,
			}, true
		}
		if resource == "pods" && action == RouteLogs {
			return namespaceRoute{
				namespace: ns,
				target:    name,
				kind:      RouteLogs,
			}, true
		}

		return namespaceRoute{
			namespace: ns,
			target:    name,
		}, true
	}

	if len(parts) == 6 && parts[0] == "namespaces" && parts[2] == "deployments" && parts[4] == "pods" && parts[5] == "status" {
		return namespaceRoute{
			namespace: parts[1],
			target:    parts[3],
			kind:      RoutePods,
		}, true
	}

	return namespaceRoute{}, false
}

func (a *appState) namespaceHandler(w http.ResponseWriter, r *http.Request) {
	// Restart endpoint walkthrough note:
	// request -> route match -> namespace/name extraction occurs here.
	route, ok := parseNamespaceRoute(r.URL.Path)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "malformed path")
		return
	}

	switch route.kind {
	case RouteRestart:
		handlers.RestartHandler(a.kubeClient, a.kubeInitErr, SplitPathStrict, time.Now).ServeHTTP(w, r)

	case RouteLogs:
		openLogStream := a.openLogStream
		if openLogStream == nil {
			openLogStream = func(ns, pod string) (io.ReadCloser, error) {
				return a.kubeClient.CoreV1().Pods(ns).GetLogs(pod, &corev1.PodLogOptions{}).Stream(r.Context())
			}
		}
		handlers.LogsHandler(a.kubeClient, a.kubeInitErr, SplitPathStrict, openLogStream).ServeHTTP(w, r)

	case RoutePods:
		handlers.StatusHandler(a.kubeClient, a.kubeInitErr, SplitPathStrict).ServeHTTP(w, r)

	case RouteDelete:
		handlers.DeleteDeploymentHandler(a.kubeClient, a.kubeInitErr, SplitPathStrict).ServeHTTP(w, r)

	default:
		writeJSONError(w, http.StatusNotFound, "endpoint not found")
	}
}

func main() {
	kubeClient, err := k8sclient.NewClient()
	var kubeInitErr string
	if err != nil {
		kubeInitErr = err.Error()
		log.Printf("kubernetes initialization warning: %s", kubeInitErr)
	}
	app := &appState{
		kubeClient:  kubeClient,
		kubeInitErr: kubeInitErr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.HealthHandler)
	mux.HandleFunc("/api/v1/namespaces/", app.namespaceHandler)

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") {
			// Restart endpoint walkthrough note: strict path validation happens
			// before route dispatch so malformed namespace paths fail fast with 400.
			if _, err := SplitPathStrict(r.URL.Path); err != nil {
				writeJSONError(w, http.StatusBadRequest, "malformed path")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})

	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(root)); err != nil {
		log.Fatal(err)
	}
}
