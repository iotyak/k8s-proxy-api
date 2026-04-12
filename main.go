package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type namespaceRoute struct {
	namespace string
	target    string
	kind      string
}

type appState struct {
	kubeClient  kubernetes.Interface
	kubeInitErr string
}

type routeError struct {
	status  int
	message string
	details map[string]any
}

type healthResponse struct {
	AppStatus           string `json:"app_status"`
	KubernetesReachable bool   `json:"kubernetes_reachable"`
	KubernetesVersion   string `json:"kubernetes_version,omitempty"`
	KubernetesError     string `json:"kubernetes_error,omitempty"`
}

const (
	routeRestart = "restart"
	routeLogs    = "logs"

	proxyAccessLabelKey   = "proxy-access"
	proxyAccessLabelValue = "allowed"
)

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
		log.Printf("completed in %s", time.Since(start))
	})
}

func (a *appState) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	resp := healthResponse{
		AppStatus:           "ok",
		KubernetesReachable: false,
	}

	if a.kubeClient == nil {
		if a.kubeInitErr != "" {
			resp.KubernetesError = a.kubeInitErr
		} else {
			resp.KubernetesError = "kubernetes client not initialized"
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	version, err := a.kubeClient.Discovery().ServerVersion()
	if err != nil {
		resp.KubernetesError = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.KubernetesReachable = true
	resp.KubernetesVersion = version.GitVersion
	writeJSON(w, http.StatusOK, resp)
}

func initKubernetesClient() (kubernetes.Interface, string) {
	kubeconfigPath := os.Getenv("KUBECONFIG")

	var cfg *rest.Config
	var err error
	if kubeconfigPath != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Sprintf("failed to load kubeconfig from KUBECONFIG (%s): %v", kubeconfigPath, err)
		}
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Sprintf("failed to load in-cluster config: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Sprintf("failed to create kubernetes clientset: %v", err)
	}

	return clientset, ""
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

func hasProxyAccessAllowed(dep *appsv1.Deployment) bool {
	if dep == nil {
		return false
	}
	return dep.Labels[proxyAccessLabelKey] == proxyAccessLabelValue
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

func buildRestartAnnotationPatch(restartedAt string) ([]byte, error) {
	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]string{
						"kubectl.kubernetes.io/restarted-at": restartedAt,
					},
				},
			},
		},
	}

	return json.Marshal(patch)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func splitPathStrict(path string) ([]string, bool) {
	if path == "" || path[0] != '/' {
		return nil, false
	}
	if strings.Contains(path, "//") {
		return nil, false
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		return nil, false
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return nil, false
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, false
		}
	}

	return parts, true
}

func parseNamespaceRoute(path string) (namespaceRoute, bool) {
	parts, ok := splitPathStrict(path)
	if !ok {
		return namespaceRoute{}, false
	}
	if len(parts) != 5 || parts[0] != "namespaces" {
		return namespaceRoute{}, false
	}

	ns := parts[1]
	resource := parts[2]
	name := parts[3]
	action := parts[4]

	if resource == "deployments" && action == "restart" {
		return namespaceRoute{
			namespace: ns,
			target:    name,
			kind:      routeRestart,
		}, true
	}
	if resource == "pods" && action == "logs" {
		return namespaceRoute{
			namespace: ns,
			target:    name,
			kind:      routeLogs,
		}, true
	}

	return namespaceRoute{
		namespace: ns,
		target:    name,
	}, true
}

func (a *appState) namespaceHandler(w http.ResponseWriter, r *http.Request) {
	route, ok := parseNamespaceRoute(r.URL.Path)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "malformed path")
		return
	}

	switch route.kind {
	case routeRestart:
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		dep, err := a.getDeployment(r.Context(), route.namespace, route.target)
		if err != nil {
			if apierrors.IsNotFound(err) {
				writeJSON(w, http.StatusNotFound, map[string]any{
					"error":      "deployment not found",
					"namespace":  route.namespace,
					"deployment": route.target,
					"action":     "restart",
				})
				return
			}

			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error":      "failed to read deployment",
				"namespace":  route.namespace,
				"deployment": route.target,
				"action":     "restart",
				"details":    err.Error(),
			})
			return
		}

		deploymentLabel := ""
		if dep.Labels != nil {
			deploymentLabel = dep.Labels[proxyAccessLabelKey]
		}

		if !hasProxyAccessAllowed(dep) {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":            "deployment is not allowed for proxy access",
				"namespace":        route.namespace,
				"deployment":       route.target,
				"action":           "restart",
				"required_label":   proxyAccessLabelKey + "=" + proxyAccessLabelValue,
				"deployment_label": deploymentLabel,
			})
			return
		}

		restartedAt := time.Now().UTC().Format(time.RFC3339)
		patchBody, err := buildRestartAnnotationPatch(restartedAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error":      "failed to build restart patch",
				"namespace":  route.namespace,
				"deployment": route.target,
				"action":     "restart",
				"details":    err.Error(),
			})
			return
		}

		if _, err := a.kubeClient.AppsV1().Deployments(route.namespace).Patch(
			r.Context(),
			route.target,
			types.MergePatchType,
			patchBody,
			metav1.PatchOptions{},
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error":      "failed to patch deployment restart annotation",
				"namespace":  route.namespace,
				"deployment": route.target,
				"action":     "restart",
				"details":    err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success":        true,
			"authorized":     true,
			"namespace":      route.namespace,
			"deployment":     route.target,
			"action":         "restart",
			"restarted_at":   restartedAt,
			"message":        "deployment restart annotation patched",
			"required_label": proxyAccessLabelKey + "=" + proxyAccessLabelValue,
		})

	case routeLogs:
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		podName, replicaSetName, dep, routeErr := a.resolvePodOwningDeployment(r.Context(), route.namespace, route.target)
		if routeErr != nil {
			resp := map[string]any{
				"error":     routeErr.message,
				"namespace": route.namespace,
				"pod":       route.target,
				"action":    "logs",
			}
			if podName != "" {
				resp["pod"] = podName
			}
			if replicaSetName != "" {
				resp["replicaset"] = replicaSetName
			}
			for k, v := range routeErr.details {
				resp[k] = v
			}
			writeJSON(w, routeErr.status, resp)
			return
		}

		deploymentLabel := ""
		if dep.Labels != nil {
			deploymentLabel = dep.Labels[proxyAccessLabelKey]
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"namespace":            route.namespace,
			"pod":                  podName,
			"replicaset":           replicaSetName,
			"deployment":           dep.Name,
			"proxy_access_allowed": hasProxyAccessAllowed(dep),
			"deployment_label":     deploymentLabel,
			"required_label":       proxyAccessLabelKey + "=" + proxyAccessLabelValue,
			"action":               "logs",
			"logs":                 "not implemented yet",
		})

	default:
		writeJSONError(w, http.StatusNotFound, "endpoint not found")
	}
}

func main() {
	kubeClient, kubeInitErr := initKubernetesClient()
	if kubeInitErr != "" {
		log.Printf("kubernetes initialization warning: %s", kubeInitErr)
	}

	app := &appState{
		kubeClient:  kubeClient,
		kubeInitErr: kubeInitErr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", app.healthHandler)
	mux.HandleFunc("/namespaces/", app.namespaceHandler)

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/namespaces/") {
			if _, ok := splitPathStrict(r.URL.Path); !ok {
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
