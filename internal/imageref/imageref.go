package imageref

import (
	"net/url"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/variables"
)

const (
	DefaultRepositoryTemplate = "{registryNamespace}/{projectIdentifier}-{appIdentifier}"
	DefaultTagTemplate        = "latest"
)

func RegistryAuthEndpoint(endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" {
		return strings.TrimSpace(endpoint)
	}
	host := strings.ToLower(parsed.Host)
	if host == "registry-1.docker.io" || host == "docker.io" || host == "index.docker.io" {
		return "https://index.docker.io/v1/"
	}
	return parsed.Host
}

func BuildImageRef(registry model.ArtifactRegistry, run model.BuildRun) string {
	repository := strings.Trim(strings.TrimSpace(run.TargetRepository), "/")
	if repository == "" {
		return ""
	}
	tag := RenderBuildTagTemplate(fallback(strings.TrimSpace(run.TargetTag), DefaultTagTemplate), variables.Context{SourceBranch: run.SourceBranch, SourceTag: run.SourceTag, SourceCommit: run.SourceCommit})
	if hasRegistryHost(repository) || IsDockerHubRegistry(registry) {
		return repository + ":" + tag
	}
	endpoint := RegistryImageHost(registry.Endpoint)
	if endpoint != "" {
		return endpoint + "/" + repository + ":" + tag
	}
	return repository + ":" + tag
}

func BuildTargetImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application) string {
	projectIdentifier := projectIdentifierValue(project)
	appIdentifier := applicationIdentifierValue(application)
	repository := projectIdentifier + "-" + appIdentifier
	namespace := strings.Trim(strings.TrimSpace(registry.Namespace), "/")
	if namespace == "" {
		namespace = projectIdentifier
	}
	if namespace != "" {
		repository = namespace + "/" + repository
	}
	return BuildImageNamePrefix(registry, repository)
}

func BuildTargetImageRepositoryForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	template := NormalizeRepositoryTemplate(credential.RepositoryTemplate)
	repository := renderRepositoryTemplate(template, registry, project, application, target)
	if repository == "" {
		return BuildTargetImageRepository(registry, project, application)
	}
	return BuildImageNamePrefix(registry, repository)
}

func BuildTargetImageTagTemplateForCredential(credential model.RegistryCredential) string {
	return NormalizeTagTemplate(credential.TagTemplate)
}

func BuildStaticTargetImageTagForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	rendered := replaceTemplatePlaceholders(NormalizeTagTemplate(credential.TagTemplate), staticImageTemplateValues(registry, project, application, target))
	if hasUnresolvedPlaceholder(rendered) {
		return DefaultTagTemplate
	}
	return sanitizeImageTag(rendered)
}

func NormalizeRepositoryTemplate(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return DefaultRepositoryTemplate
	}
	return value
}

func NormalizeTagTemplate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultTagTemplate
	}
	return value
}

func IsDefaultRepositoryFor(registry model.ArtifactRegistry, project model.Project, application model.Application, repository string) bool {
	expected, _ := SplitImageRef(BuildTargetImageRepository(registry, project, application))
	normalized := strings.Trim(strings.TrimSpace(repository), "/")
	if normalized == expected {
		return true
	}
	return normalized == RepositoryWithoutRegistryHost(registry, expected)
}

func RepositoryWithoutRegistryHost(registry model.ArtifactRegistry, repository string) string {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	host := RegistryImageHost(registry.Endpoint)
	if host != "" && strings.HasPrefix(repository, host+"/") {
		return strings.TrimPrefix(repository, host+"/")
	}
	return repository
}

func BuildImageNamePrefix(registry model.ArtifactRegistry, repository string) string {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	if repository == "" {
		return ""
	}
	if hasRegistryHost(repository) || IsDockerHubRegistry(registry) {
		return repository
	}
	host := RegistryImageHost(registry.Endpoint)
	if host == "" {
		return repository
	}
	return strings.TrimRight(host, "/") + "/" + repository
}

func IsDockerHubRegistry(registry model.ArtifactRegistry) bool {
	provider := strings.ToLower(strings.TrimSpace(registry.Provider))
	if provider == "dockerhub" || provider == "docker-hub" {
		return true
	}
	host := RegistryImageHost(registry.Endpoint)
	return host == "docker.io" || host == "registry-1.docker.io" || host == "index.docker.io"
}

func RenderBuildTagTemplate(template string, ctx variables.Context) string {
	rendered := variables.Render(fallback(strings.TrimSpace(template), DefaultTagTemplate), ctx)
	rendered = replaceTemplatePlaceholders(rendered, buildVariableValues(ctx))
	return sanitizeImageTag(rendered)
}

func SplitImageRef(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if at := strings.Index(value, "@"); at >= 0 {
		return strings.Trim(value[:at], "/"), value[at+1:]
	}
	lastSlash := strings.LastIndex(value, "/")
	lastColon := strings.LastIndex(value, ":")
	if lastColon > lastSlash {
		return strings.Trim(value[:lastColon], "/"), value[lastColon+1:]
	}
	return strings.Trim(value, "/"), ""
}

func RegistryImageHost(endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" {
		return strings.TrimSpace(endpoint)
	}
	host := strings.ToLower(parsed.Host)
	if host == "registry-1.docker.io" || host == "index.docker.io" {
		return "docker.io"
	}
	return parsed.Host
}

func renderRepositoryTemplate(template string, registry model.ArtifactRegistry, project model.Project, application model.Application, target model.DeploymentTarget) string {
	output := replaceTemplatePlaceholders(template, staticImageTemplateValues(registry, project, application, target))
	output = strings.Trim(strings.TrimSpace(output), "/")
	for strings.Contains(output, "//") {
		output = strings.ReplaceAll(output, "//", "/")
	}
	return output
}

func staticImageTemplateValues(registry model.ArtifactRegistry, project model.Project, application model.Application, target model.DeploymentTarget) map[string]string {
	return map[string]string{
		"registryNamespace":     registryNamespaceValue(registry, project),
		"project":               projectIdentifierValue(project),
		"projectIdentifier":     projectIdentifierValue(project),
		"app":                   applicationIdentifierValue(application),
		"appIdentifier":         applicationIdentifierValue(application),
		"application":           applicationIdentifierValue(application),
		"applicationIdentifier": applicationIdentifierValue(application),
		"stage":                 stageValue(target),
		"target":                targetValue(target),
	}
}

func replaceTemplatePlaceholders(template string, values map[string]string) string {
	output := template
	for key, value := range values {
		output = strings.ReplaceAll(output, "{"+key+"}", value)
		output = strings.ReplaceAll(output, "${"+key+"}", value)
	}
	return output
}

func hasUnresolvedPlaceholder(value string) bool {
	return strings.Contains(value, "{") || strings.Contains(value, "}")
}

func buildVariableValues(ctx variables.Context) map[string]string {
	refName := strings.TrimSpace(ctx.SourceTag)
	if refName == "" {
		refName = strings.TrimSpace(ctx.SourceBranch)
	}
	shortSHA := shortCommit(ctx.SourceCommit)
	return map[string]string{
		"branch":        strings.TrimSpace(ctx.SourceBranch),
		"branchSlug":    dnsSafeSegment(ctx.SourceBranch),
		"tag":           strings.TrimSpace(ctx.SourceTag),
		"tagSlug":       dnsSafeSegment(ctx.SourceTag),
		"ref":           refName,
		"refSlug":       dnsSafeSegment(refName),
		"commit":        strings.TrimSpace(ctx.SourceCommit),
		"commitSha":     strings.TrimSpace(ctx.SourceCommit),
		"shortSha":      shortSHA,
		"shortSHA":      shortSHA,
		"shortCommit":   shortSHA,
		"short_sha":     shortSHA,
		"github.sha":    strings.TrimSpace(ctx.SourceCommit),
		"github.ref":    refName,
		"github.branch": strings.TrimSpace(ctx.SourceBranch),
	}
}

func registryNamespaceValue(registry model.ArtifactRegistry, project model.Project) string {
	namespace := strings.Trim(strings.TrimSpace(registry.Namespace), "/")
	if namespace != "" {
		return namespace
	}
	return projectIdentifierValue(project)
}

func projectIdentifierValue(project model.Project) string {
	return dnsSafeSegment(fallback(project.Identifier, project.Name))
}

func applicationIdentifierValue(application model.Application) string {
	return dnsSafeSegment(fallback(application.Identifier, application.Name))
}

func stageValue(target model.DeploymentTarget) string {
	return dnsSafeSegment(fallback(target.Stage, model.DefaultDeploymentStage))
}

func targetValue(target model.DeploymentTarget) string {
	return dnsSafeSegment(fallback(target.Name, target.Stage))
}

func hasRegistryHost(repository string) bool {
	first := strings.Split(strings.Trim(repository, "/"), "/")[0]
	return strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost"
}

func sanitizeImageTag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultTagTemplate
	}
	var builder strings.Builder
	for _, char := range value {
		if char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '.' || char == '-' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('-')
	}
	output := strings.Trim(builder.String(), ".-")
	if output == "" {
		return DefaultTagTemplate
	}
	if len(output) > 128 {
		output = output[:128]
	}
	return output
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

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
