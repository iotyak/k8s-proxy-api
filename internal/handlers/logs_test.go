package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func strictLogsSplit(path string) ([]string, error) {
	if path == "/api/v1/namespaces/testns/pods/testpod/logs" {
		return []string{"api", "v1", "namespaces", "testns", "pods", "testpod", "logs"}, nil
	}
	return nil, errInvalidPath
}

func TestLogsHandlerRejectsMalformedPath(t *testing.T) {
	h := LogsHandler(fake.NewSimpleClientset(), "", strictLogsSplit, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces//pods/testpod/logs", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestLogsHandlerRejectsUnauthorizedDeployment(t *testing.T) {
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "testdep"}}
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "testrs", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "testdep"}}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "testpod", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "testrs"}}}}

	client := fake.NewSimpleClientset(dep, rs, pod)
	h := LogsHandler(client, "", strictLogsSplit, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/pods/testpod/logs", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestLogsHandlerStreamsPodLogs(t *testing.T) {
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "testdep", Labels: map[string]string{"proxy-access": "allowed"}}}
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "testrs", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "testdep"}}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "testns", Name: "testpod", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "testrs"}}}}

	client := fake.NewSimpleClientset(dep, rs, pod)
	openStream := func(_ string, _ string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("line1\nline2\n")), nil
	}
	h := LogsHandler(client, "", strictLogsSplit, openStream)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/pods/testpod/logs", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("expected text content-type, got %q", got)
	}
	if body := rr.Body.String(); body != "line1\nline2\n" {
		t.Fatalf("unexpected logs body %q", body)
	}
}
