package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func strictDeleteSplit(path string) ([]string, error) {
	if path == "/api/v1/namespaces/testns/deployments/testdep/delete" {
		return []string{"api", "v1", "namespaces", "testns", "deployments", "testdep", "delete"}, nil
	}
	return nil, errInvalidPath
}

func TestDeleteDeploymentHandlerCases(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		client     *fake.Clientset
		wantStatus int
	}{
		{
			name:       "wrong method",
			method:     http.MethodGet,
			path:       "/api/v1/namespaces/testns/deployments/testdep/delete",
			client:     fake.NewSimpleClientset(),
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "malformed path",
			method:     http.MethodDelete,
			path:       "/api/v1/namespaces/testns/deployments//delete",
			client:     fake.NewSimpleClientset(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "deployment not found",
			method:     http.MethodDelete,
			path:       "/api/v1/namespaces/testns/deployments/testdep/delete",
			client:     fake.NewSimpleClientset(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "unauthorized deployment",
			method: http.MethodDelete,
			path:   "/api/v1/namespaces/testns/deployments/testdep/delete",
			client: fake.NewSimpleClientset(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "testns",
					Name:      "testdep",
				},
			}),
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := DeleteDeploymentHandler(tt.client, "", strictDeleteSplit)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rr.Code)
			}
		})
	}
}

func TestDeleteDeploymentHandlerDeletesAuthorizedDeployment(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testns",
			Name:      "testdep",
			Labels:    map[string]string{"proxy-access": "allowed"},
		},
	})

	h := DeleteDeploymentHandler(client, "", strictDeleteSplit)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/namespaces/testns/deployments/testdep/delete", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	_, err := client.AppsV1().Deployments("testns").Get(context.Background(), "testdep", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected deployment to be deleted, got err=%v", err)
	}
}
