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

func TestPodLogOptionsFromQueryValidCases(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		assertion func(*testing.T, *corev1.PodLogOptions)
	}{
		{
			name:  "defaults",
			query: "",
			assertion: func(t *testing.T, options *corev1.PodLogOptions) {
				t.Helper()
				if options == nil {
					t.Fatal("expected options, got nil")
				}
				if options.TailLines != nil {
					t.Fatalf("expected default tailLines=nil, got %v", *options.TailLines)
				}
				if options.SinceSeconds != nil {
					t.Fatalf("expected default sinceSeconds=nil, got %v", *options.SinceSeconds)
				}
				if options.LimitBytes != nil {
					t.Fatalf("expected default limitBytes=nil, got %v", *options.LimitBytes)
				}
				if options.Follow {
					t.Fatalf("expected default follow=false, got %v", options.Follow)
				}
				if options.Timestamps {
					t.Fatalf("expected default timestamps=false, got %v", options.Timestamps)
				}
			},
		},
		{
			name:  "timestamps only",
			query: "timestamps=true",
			assertion: func(t *testing.T, options *corev1.PodLogOptions) {
				t.Helper()
				if options == nil || !options.Timestamps {
					t.Fatalf("expected timestamps=true, got %#v", options)
				}
			},
		},
		{
			name:  "all values",
			query: "tailLines=10&sinceSeconds=0&follow=true&limitBytes=4096&timestamps=true",
			assertion: func(t *testing.T, options *corev1.PodLogOptions) {
				t.Helper()
				if options == nil {
					t.Fatal("expected options, got nil")
				}
				if options.TailLines == nil || *options.TailLines != 10 {
					t.Fatalf("expected tailLines=10, got %#v", options.TailLines)
				}
				if options.SinceSeconds == nil || *options.SinceSeconds != 0 {
					t.Fatalf("expected sinceSeconds=0, got %#v", options.SinceSeconds)
				}
				if !options.Follow {
					t.Fatalf("expected follow=true, got %v", options.Follow)
				}
				if options.LimitBytes == nil || *options.LimitBytes != 4096 {
					t.Fatalf("expected limitBytes=4096, got %#v", options.LimitBytes)
				}
				if !options.Timestamps {
					t.Fatalf("expected timestamps=true, got %v", options.Timestamps)
				}
			},
		},
		{
			name:  "follow false and timestamps false",
			query: "follow=false&timestamps=false",
			assertion: func(t *testing.T, options *corev1.PodLogOptions) {
				t.Helper()
				if options == nil {
					t.Fatal("expected options, got nil")
				}
				if options.Follow {
					t.Fatalf("expected follow=false, got %v", options.Follow)
				}
				if options.Timestamps {
					t.Fatalf("expected timestamps=false, got %v", options.Timestamps)
				}
			},
		},
		{
			name:  "follow true and timestamps true",
			query: "follow=true&timestamps=true",
			assertion: func(t *testing.T, options *corev1.PodLogOptions) {
				t.Helper()
				if options == nil {
					t.Fatal("expected options, got nil")
				}
				if !options.Follow {
					t.Fatalf("expected follow=true, got %v", options.Follow)
				}
				if !options.Timestamps {
					t.Fatalf("expected timestamps=true, got %v", options.Timestamps)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/namespaces/testns/pods/testpod/logs"
			if tt.query != "" {
				url += "?" + tt.query
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			options, routeErr := podLogOptionsFromQuery(req)
			if routeErr != nil {
				t.Fatalf("expected no error, got %#v", routeErr)
			}

			tt.assertion(t, options)
		})
	}
}

func TestPodLogOptionsFromQueryInvalidCases(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		wantParameter string
	}{
		{name: "tailLines zero", query: "tailLines=0", wantParameter: "tailLines"},
		{name: "tailLines negative", query: "tailLines=-1", wantParameter: "tailLines"},
		{name: "limitBytes zero", query: "limitBytes=0", wantParameter: "limitBytes"},
		{name: "limitBytes negative", query: "limitBytes=-1", wantParameter: "limitBytes"},
		{name: "sinceSeconds negative", query: "sinceSeconds=-1", wantParameter: "sinceSeconds"},
		{name: "follow invalid", query: "follow=maybe", wantParameter: "follow"},
		{name: "timestamps invalid", query: "timestamps=maybe", wantParameter: "timestamps"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/testns/pods/testpod/logs?"+tt.query, nil)

			_, routeErr := podLogOptionsFromQuery(req)
			if routeErr == nil {
				t.Fatal("expected route error")
			}
			if routeErr.status != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", routeErr.status)
			}
			if got := routeErr.details["parameter"]; got != tt.wantParameter {
				t.Fatalf("expected parameter %s, got %v", tt.wantParameter, got)
			}
		})
	}
}
