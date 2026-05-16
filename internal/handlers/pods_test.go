package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func strictPodsSplit(path string) ([]string, error) {
	if path == "/api/v1/namespaces/testns/pods" {
		return []string{"api", "v1", "namespaces", "testns", "pods"}, nil
	}
	return nil, errInvalidPath
}

func TestListPodsHandlerMethodValidation(t *testing.T) {
	h := ListPodsHandler(fake.NewSimpleClientset(), "", strictPodsSplit)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/testns/pods", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestListPodsHandlerPathValidation(t *testing.T) {
	h := ListPodsHandler(fake.NewSimpleClientset(), "", strictPodsSplit)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/pods/", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListPodsHandlerSuccess(t *testing.T) {
	start := metav1.NewTime(time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC))
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "pod-a"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
			Status: corev1.PodStatus{
				Phase:     corev1.PodRunning,
				PodIP:     "10.42.0.10",
				HostIP:    "192.168.1.10",
				StartTime: &start,
			},
		},
	)

	h := ListPodsHandler(client, "", strictPodsSplit)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/pods", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Namespace string `json:"namespace"`
		Action    string `json:"action"`
		Count     int    `json:"count"`
		Pods      []struct {
			Name      string `json:"name"`
			Phase     string `json:"phase"`
			PodIP     string `json:"pod_ip"`
			NodeName  string `json:"node_name"`
			HostIP    string `json:"host_ip"`
			StartTime string `json:"start_time"`
		} `json:"pods"`
	}

	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Namespace != "testns" || resp.Action != "list-pods" {
		t.Fatalf("unexpected metadata: %#v", resp)
	}
	if resp.Count != 1 || len(resp.Pods) != 1 {
		t.Fatalf("expected one pod, got count=%d len=%d", resp.Count, len(resp.Pods))
	}
	if resp.Pods[0].Name != "pod-a" || resp.Pods[0].Phase != "Running" {
		t.Fatalf("unexpected pod identity: %#v", resp.Pods[0])
	}
	if resp.Pods[0].StartTime != "2026-01-02T03:04:05Z" {
		t.Fatalf("unexpected start time: %s", resp.Pods[0].StartTime)
	}
}
