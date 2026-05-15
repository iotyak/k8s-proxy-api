package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func strictSplit(path string) ([]string, error) {
	if path == "" || path[0] != '/' {
		return nil, errInvalidPath
	}
	parts := []string{"api", "v1", "namespaces", "testns", "deployments", "testdep", "restart"}
	if path == "/api/v1/namespaces/testns/deployments/testdep/restart" {
		return parts, nil
	}
	return nil, errInvalidPath
}

func TestRestartHandlerRejectsMalformedPath(t *testing.T) {
	h := RestartHandler(fake.NewSimpleClientset(), "", strictSplit, time.Now)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces//deployments/testdep/restart", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestRestartHandlerRejectsUnauthorizedDeployment(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testns",
			Name:      "testdep",
		},
	})

	h := RestartHandler(client, "", strictSplit, time.Now)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/testns/deployments/testdep/restart", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRestartHandlerPatchesRestartAnnotation(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testns",
			Name:      "testdep",
			Labels:    map[string]string{"proxy-access": "allowed"},
		},
	})

	fixedNow := func() time.Time {
		return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	}

	h := RestartHandler(client, "", strictSplit, fixedNow)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/testns/deployments/testdep/restart", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	dep, err := client.AppsV1().Deployments("testns").Get(req.Context(), "testdep", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if dep.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatalf("expected restartedAt annotation to be set")
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["authorized"] != true {
		t.Fatalf("expected authorized=true, got %v", resp["authorized"])
	}
}
