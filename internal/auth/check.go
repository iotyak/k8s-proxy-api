package auth

import appsv1 "k8s.io/api/apps/v1"

const (
	ProxyAccessLabelKey   = "proxy-access"
	ProxyAccessLabelValue = "allowed"
)

func CheckDeploymentAllowed(dep *appsv1.Deployment) bool {
	if dep == nil {
		return false
	}
	return dep.Labels[ProxyAccessLabelKey] == ProxyAccessLabelValue
}

func DeploymentLabelValue(dep *appsv1.Deployment) string {
	if dep == nil || dep.Labels == nil {
		return ""
	}
	return dep.Labels[ProxyAccessLabelKey]
}
