package auth

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckDeploymentAllowed(t *testing.T) {
	allowed := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"proxy-access": "allowed"}}}
	if !CheckDeploymentAllowed(allowed) {
		t.Fatalf("expected deployment to be allowed")
	}

	missing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}}
	if CheckDeploymentAllowed(missing) {
		t.Fatalf("expected deployment without label to be denied")
	}

	wrong := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"proxy-access": "denied"}}}
	if CheckDeploymentAllowed(wrong) {
		t.Fatalf("expected deployment with wrong value to be denied")
	}

	if CheckDeploymentAllowed(nil) {
		t.Fatalf("expected nil deployment to be denied")
	}
}
