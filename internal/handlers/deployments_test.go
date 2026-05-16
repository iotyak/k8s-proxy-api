package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func strictDeploymentsSplit(path string) ([]string, error) {
	if path == "/api/v1/namespaces/testns/deployments" {
		return []string{"api", "v1", "namespaces", "testns", "deployments"}, nil
	}
	return nil, errInvalidPath
}

func TestListDeploymentsHandlerMethodValidation(t *testing.T) {
	h := ListDeploymentsHandler(fake.NewSimpleClientset(), "", strictDeploymentsSplit)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/testns/deployments", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestListDeploymentsHandlerPathValidation(t *testing.T) {
	h := ListDeploymentsHandler(fake.NewSimpleClientset(), "", strictDeploymentsSplit)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/deployments/", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListDeploymentsHandlerSuccess(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "dep-a"},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 1, Replicas: 2},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "dep-b"},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 3, AvailableReplicas: 2, Replicas: 3},
		},
	)

	h := ListDeploymentsHandler(client, "", strictDeploymentsSplit)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/deployments", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Namespace   string `json:"namespace"`
		Action      string `json:"action"`
		Count       int    `json:"count"`
		Deployments []struct {
			Name              string `json:"name"`
			ReadyReplicas     int32  `json:"ready_replicas"`
			AvailableReplicas int32  `json:"available_replicas"`
			Replicas          int32  `json:"replicas"`
		} `json:"deployments"`
	}

	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Namespace != "testns" || resp.Action != "list-deployments" {
		t.Fatalf("unexpected metadata: %#v", resp)
	}
	if resp.Count != 2 || len(resp.Deployments) != 2 {
		t.Fatalf("expected two deployments, got count=%d len=%d", resp.Count, len(resp.Deployments))
	}
}
