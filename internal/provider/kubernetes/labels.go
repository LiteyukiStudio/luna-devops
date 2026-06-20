package kubernetes

const (
	ManagedByLabel     = "app.kubernetes.io/managed-by"
	ApplicationNameKey = "app.kubernetes.io/name"
	ManagedByValue     = "liteyuki-devops"

	ProjectIDLabel          = "liteyuki.devops/project-id"
	ApplicationIDLabel      = "liteyuki.devops/application-id"
	EnvironmentIDLabel      = "liteyuki.devops/environment-id"
	DeploymentTargetIDLabel = "liteyuki.devops/deployment-target-id"
	ReleaseIDLabel          = "liteyuki.devops/release-id"
	BuildRunIDLabel         = "liteyuki.devops/build-run-id"
	ImageDigestLabel        = "liteyuki.devops/image-digest"
	GatewayRouteIDLabel     = "liteyuki.devops/gateway-route-id"
	HookRunIDLabel          = "liteyuki.devops/hook-run-id"
	HookPhaseLabel          = "liteyuki.devops/hook-phase"
	ScopeLabel              = "liteyuki.devops/scope"
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
