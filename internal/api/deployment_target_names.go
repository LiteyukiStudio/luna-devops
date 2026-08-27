package api

import (
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourcename"
)

func runtimeProjectNamespace(project model.Project) string {
	return strings.TrimSpace(project.KubernetesNamespace)
}

func deploymentTargetResourceName(target model.DeploymentTarget) string {
	return strings.TrimSpace(target.KubernetesName)
}

func shortResourceID(value string) string {
	return resourcename.ShortID(value)
}

func runtimeIDResourceName(prefix string, value string) string {
	return resourcename.FromID(prefix, value)
}

func runtimeShortID(value string) string {
	return resourcename.ShortID(value)
}

func runtimeDNSLabel(value string) string {
	return resourcename.DNSLabel(value)
}
