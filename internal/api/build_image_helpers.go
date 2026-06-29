package api

import (
	"github.com/LiteyukiStudio/devops/internal/imageref"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/variables"
	"strings"
)

func registryAuthEndpointForBuilder(endpoint string) string {
	return imageref.RegistryAuthEndpoint(endpoint)
}

func buildImageRef(registry model.ArtifactRegistry, run model.BuildRun) string {
	return imageref.BuildImageRef(registry, run)
}

func buildTargetImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application) string {
	return imageref.BuildTargetImageRepository(registry, project, application)
}

func buildTargetImageRepositoryForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return imageref.BuildTargetImageRepositoryForCredential(registry, credential, project, application, target)
}

func buildTargetImageTagTemplateForCredential(credential model.RegistryCredential) string {
	return imageref.BuildTargetImageTagTemplateForCredential(credential)
}

func normalizeImageRepositoryTemplate(value string) string {
	return imageref.NormalizeRepositoryTemplate(value)
}

func normalizeImageTagTemplate(value string) string {
	return imageref.NormalizeTagTemplate(value)
}

func isDefaultImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application, repository string) bool {
	return imageref.IsDefaultRepositoryFor(registry, project, application, repository)
}

func buildImageNamePrefix(registry model.ArtifactRegistry, repository string) string {
	return imageref.BuildImageNamePrefix(registry, repository)
}

func isDockerHubRegistry(registry model.ArtifactRegistry) bool {
	return imageref.IsDockerHubRegistry(registry)
}

func hasRegistryHost(repository string) bool {
	first := strings.Split(strings.Trim(repository, "/"), "/")[0]
	return strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost"
}

func renderBuildTagTemplate(template string, ctx variables.Context) string {
	return imageref.RenderBuildTagTemplate(template, ctx)
}

func sanitizeImageTag(value string) string {
	return imageref.RenderBuildTagTemplate(value, variables.Context{})
}

func dnsSafeSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	previousDash := false
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			builder.WriteRune(char)
			previousDash = false
			continue
		}
		if !previousDash {
			builder.WriteByte('-')
			previousDash = true
		}
	}
	output := strings.Trim(builder.String(), "-")
	if output == "" {
		return "app"
	}
	return output
}

func registryImageHost(endpoint string) string {
	return imageref.RegistryImageHost(endpoint)
}

func normalizeStringList(values []string) []string {
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			output = append(output, value)
		}
	}
	return output
}
