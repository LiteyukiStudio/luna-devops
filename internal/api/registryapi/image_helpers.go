package registryapi

import (
	"strings"

	"github.com/LiteyukiStudio/devops/internal/imageref"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func buildTargetImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application) string {
	return imageref.BuildTargetImageRepository(registry, project, application)
}

func buildTargetImageRepositoryForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return imageref.BuildTargetImageRepositoryForCredential(registry, credential, project, application, target)
}

func buildStaticTargetImageTagForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return imageref.BuildStaticTargetImageTagForCredential(registry, credential, project, application, target)
}

func repositoryWithoutRegistryHost(registry model.ArtifactRegistry, repository string) string {
	return imageref.RepositoryWithoutRegistryHost(registry, repository)
}

func normalizeImageRepositoryTemplate(value string) string {
	return imageref.NormalizeRepositoryTemplate(value)
}

func normalizeImageTagTemplate(value string) string {
	return imageref.NormalizeTagTemplate(value)
}

func splitTargetImageRef(value string) (string, string) {
	return imageref.SplitImageRef(value)
}

func normalizeStage(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "prod", "production":
		return "prod"
	case "dev", "staging", "test":
		return normalized
	case "":
		return model.DefaultDeploymentStage
	default:
		return normalized
	}
}
