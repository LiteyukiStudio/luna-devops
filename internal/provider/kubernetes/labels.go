package kubernetes

const (
	ManagedByLabel     = "app.kubernetes.io/managed-by"
	ApplicationNameKey = "app.kubernetes.io/name"
	ManagedByValue     = "luna-devops"

	ProjectIDLabel          = "luna.devops/project-id"
	ApplicationIDLabel      = "luna.devops/application-id"
	EnvironmentIDLabel      = "luna.devops/environment-id"
	DeploymentTargetIDLabel = "luna.devops/deployment-target-id"
	ReleaseIDLabel          = "luna.devops/release-id"
	BuildRunIDLabel         = "luna.devops/build-run-id"
	ImageDigestLabel        = "luna.devops/image-digest"
	GatewayRouteIDLabel     = "luna.devops/gateway-route-id"
	HookRunIDLabel          = "luna.devops/hook-run-id"
	HookPhaseLabel          = "luna.devops/hook-phase"
	ScopeLabel              = "luna.devops/scope"
	SystemComponentLabel    = "luna.devops/system-component"
	SystemResourceLabel     = "luna.devops/system"
	RuntimeClusterIDLabel   = "luna.devops/runtime-cluster-id"
)

func baseManagedLabels(name string) map[string]string {
	labels := map[string]string{
		ManagedByLabel: ManagedByValue,
	}
	if name != "" {
		labels[ApplicationNameKey] = name
	}
	return labels
}

func setLabel(labels map[string]string, key string, value string) {
	if value != "" {
		labels[key] = value
	}
}

func ProjectNamespaceLabels(projectID string) map[string]string {
	labels := baseManagedLabels("")
	labels[ScopeLabel] = "project"
	setLabel(labels, ProjectIDLabel, projectID)
	return labels
}

func SystemComponentLabels(componentID string, clusterID string) map[string]string {
	labels := baseManagedLabels(componentID)
	labels[ScopeLabel] = "system"
	labels[SystemResourceLabel] = "true"
	setLabel(labels, SystemComponentLabel, componentID)
	setLabel(labels, RuntimeClusterIDLabel, clusterID)
	return labels
}
