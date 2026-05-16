package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iotyak/k8s-proxy-api/internal/handlers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSplitPathStrict(t *testing.T) {
	_, err := SplitPathStrict("/api/v1//ns") // Double slash
	if err == nil {
		t.Error("expected error for double slash")
	}
	_, err = SplitPathStrict("/api/v1/ns/") // Trailing slash
	if err == nil {
		t.Error("expected error for trailing slash")
	}
	parts, err := SplitPathStrict("/api/v1/namespaces/ns/deployments/name")
	if err != nil || len(parts) != 6 {
		t.Errorf("expected no error and 6 parts, got %v %v", err, len(parts))
	}
}

func TestFullAPI(t *testing.T) {
	namespace := "testns"
	deploymentName := "hello-allowed"
	replicaSetName := "hello-allowed-rs"
	podName := "hello-allowed-pod"

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels:    map[string]string{"proxy-access": "allowed"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "hello-allowed"}},
		},
	}

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      replicaSetName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deploymentName,
			}},
			Labels: map[string]string{"app": "hello-allowed"},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    map[string]string{"app": "hello-allowed"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       replicaSetName,
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "app",
				Ready:        true,
				RestartCount: 1,
			}},
		},
	}

	client := fake.NewSimpleClientset([]runtime.Object{dep, rs, pod}...)
	app := &appState{
		kubeClient: client,
		openLogStream: func(ns, pod string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("line1\nline2\n")), nil
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.HealthHandler)
	mux.HandleFunc("/api/v1/namespaces/", app.namespaceHandler)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") {
			if _, err := SplitPathStrict(r.URL.Path); err != nil {
				writeJSONError(w, http.StatusBadRequest, "malformed path")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})

	t.Run("health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}

		var body map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode health body: %v", err)
		}
		if body["app"] != "ok" || body["kubernetes"] != "connected" {
			t.Fatalf("unexpected health response: %#v", body)
		}
	})

	t.Run("restart", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/testns/deployments/hello-allowed/restart", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode restart body: %v", err)
		}
		if body["action"] != "restart" || body["deployment"] != deploymentName {
			t.Fatalf("unexpected restart response: %#v", body)
		}
	})

	t.Run("logs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/pods/hello-allowed-pod/logs", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "line1") {
			t.Fatalf("unexpected logs body: %q", rr.Body.String())
		}
	})

	t.Run("status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/deployments/hello-allowed/pods/status", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode status body: %v", err)
		}
		if body["action"] != "pods-status" {
			t.Fatalf("unexpected status action: %#v", body)
		}
		if body["pod_count"] == nil {
			t.Fatalf("expected pod_count in status response: %#v", body)
		}
	})
}
