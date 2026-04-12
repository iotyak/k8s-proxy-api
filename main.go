package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

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

type healthResponse struct {
	AppStatus           string `json:"app_status"`
	KubernetesReachable bool   `json:"kubernetes_reachable"`
	KubernetesVersion   string `json:"kubernetes_version,omitempty"`
	KubernetesError     string `json:"kubernetes_error,omitempty"`
}

const (
	routeRestart = "restart"
	routeLogs    = "logs"
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

func namespaceHandler(w http.ResponseWriter, r *http.Request) {
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

		writeJSON(w, http.StatusOK, map[string]any{
			"namespace":  route.namespace,
			"deployment": route.target,
			"action":     "restart",
			"success":    true,
		})

	case routeLogs:
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"namespace": route.namespace,
			"pod":       route.target,
			"action":    "logs",
			"logs":      "placeholder logs",
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
	mux.HandleFunc("/namespaces/", namespaceHandler)

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
