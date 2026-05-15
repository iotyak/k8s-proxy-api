package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func strictStatusSplit(path string) ([]string, error) {
	if path == "/api/v1/namespaces/testns/deployments/testdep/pods/status" {
		return []string{"api", "v1", "namespaces", "testns", "deployments", "testdep", "pods", "status"}, nil
	}
	return nil, errInvalidPath
}

func TestStatusHandlerRejectsMalformedPath(t *testing.T) {
	h := StatusHandler(nil, "", strictStatusSplit)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/deployments/testdep/pods//status", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestStatusHandlerRejectsUnauthorizedDeployment(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "testdep"},
		Spec:       appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}}},
	}

	client := fake.NewSimpleClientset(dep)
	h := StatusHandler(client, "", strictStatusSplit)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/deployments/testdep/pods/status", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestStatusHandlerReturnsPodStatuses(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "testdep", Labels: map[string]string{"proxy-access": "allowed"}},
		Spec:       appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}}},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "demo-pod", Labels: map[string]string{"app": "demo"}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true, RestartCount: 2}},
		},
	}

	client := fake.NewSimpleClientset(dep, pod)
	h := StatusHandler(client, "", strictStatusSplit)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/deployments/testdep/pods/status", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Namespace  string `json:"namespace"`
		Deployment string `json:"deployment"`
		PodCount   int    `json:"pod_count"`
		Pods       []struct {
			Name              string `json:"name"`
			Phase             string `json:"phase"`
			ReadyContainers   int    `json:"ready_containers"`
			TotalContainers   int    `json:"total_containers"`
			TotalRestartCount int32  `json:"total_restart_count"`
		} `json:"pods"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Namespace != "testns" || resp.Deployment != "testdep" {
		t.Fatalf("unexpected identity fields: %#v", resp)
	}
	if resp.PodCount != 1 || len(resp.Pods) != 1 {
		t.Fatalf("expected one pod in response, got pod_count=%d len=%d", resp.PodCount, len(resp.Pods))
	}
	if resp.Pods[0].Name != "demo-pod" || resp.Pods[0].Phase != "Running" {
		t.Fatalf("unexpected pod identity: %#v", resp.Pods[0])
	}
	if resp.Pods[0].ReadyContainers != 1 || resp.Pods[0].TotalContainers != 1 || resp.Pods[0].TotalRestartCount != 2 {
		t.Fatalf("unexpected pod counters: %#v", resp.Pods[0])
	}
}
