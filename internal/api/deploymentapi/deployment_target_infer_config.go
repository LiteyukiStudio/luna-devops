package deploymentapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	maxInferredSingleFileBytes = 8 * 1024
	maxInferredTotalFilesBytes = 36 * 1024
	maxInferredFileCount       = 8
)

var (
	inferredEnvKeyPattern      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	inferredPortNamePattern    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	sensitiveEnvKeyNamePattern = regexp.MustCompile(`(?i)(SECRET|PASSWORD|PASSWD|TOKEN|PRIVATE_KEY|API_KEY|ACCESS_KEY|CLIENT_SECRET|DSN|DATABASE_URL|REDIS_URL|AUTH|_KEY$)`)
	yamlSensitiveKVPattern     = regexp.MustCompile(`^(\s*(?:-\s*)?['"]?)([A-Za-z_][A-Za-z0-9_]{0,127})(['"]?\s*[:=]\s*)(.+)$`)
	dockerInstructionKVPattern = regexp.MustCompile(`(?i)^(\s*(?:ENV|ARG)\s+)([A-Za-z_][A-Za-z0-9_]{0,127})(\s*(?:=|\s+)\s*)(.+)$`)
	inlineEnvPairPattern       = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]{0,127})(\s*=\s*)("[^"]*"|'[^']*'|[^\s]+)`)
)

type DeploymentTargetConfigInferenceInput struct {
	RepositoryBindingID string `json:"repositoryBindingId"`
	Ref                 string `json:"ref"`
	Subpath             string `json:"subpath"`
	Stage               string `json:"stage"`
}

type InferredBuildArg struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type InferredServicePort struct {
	Name        string `json:"name"`
	Port        int    `json:"port"`
	AppProtocol string `json:"appProtocol"`
}

type InferredEnvironmentVariable struct {
	Key         string `json:"key"`
	Secret      bool   `json:"secret"`
	Placeholder string `json:"placeholder"`
}

type DeploymentTargetConfigInferenceOutput struct {
	Summary              string                        `json:"summary"`
	Confidence           string                        `json:"confidence"`
	BuildDefinitionMode  string                        `json:"buildDefinitionMode"`
	DockerfilePath       string                        `json:"dockerfilePath"`
	BuildContext         string                        `json:"buildContext"`
	BuildDirectory       string                        `json:"buildDirectory"`
	BuildTemplateID      string                        `json:"buildTemplateId"`
	BuildTemplateVersion string                        `json:"buildTemplateVersion"`
	BuildTemplateValues  map[string]string             `json:"buildTemplateValues"`
	BuildArgs            []InferredBuildArg            `json:"buildArgs"`
	ServicePorts         []InferredServicePort         `json:"servicePorts"`
	EnvironmentVariables []InferredEnvironmentVariable `json:"environmentVariables"`
	CPURequest           string                        `json:"cpuRequest"`
	MemoryRequest        string                        `json:"memoryRequest"`
	ContainerCommand     []string                      `json:"containerCommand"`
	ContainerArgs        []string                      `json:"containerArgs"`
	Notes                []string                      `json:"notes"`
	InspectedFiles       []string                      `json:"inspectedFiles"`
}

type repoFileSnippet struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

// InferDeploymentTargetConfig handles POST /api/v1/projects/:projectId/applications/:applicationId/deployment-target-config-inference.
func (h *Handlers) InferDeploymentTargetConfig(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	user, project, ok := h.authorizeProject(ctx, authz.ActionDeploymentUpdate)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "application deletion is in progress")
		return
	}

	var input DeploymentTargetConfigInferenceInput
	if !bindJSON(ctx, &input) {
		return
	}
	bindingID := strings.TrimSpace(input.RepositoryBindingID)
	if bindingID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_target.repository_binding_required", "repositoryBindingId is required")
		return
	}
	subpath, subpathOK := sanitizeInferenceSubpath(input.Subpath)
	if !subpathOK {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_target.subpath_invalid", "subpath format is invalid")
		return
	}

	var binding model.RepositoryBinding
	if err := h.dbFor(ctx).First(&binding, "id = ? and project_id = ? and application_id = ?", bindingID, project.ID, app.ID).Error; err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "deployment_target.repository_binding_not_found", "repository binding not found")
		return
	}

	gitClient, ok := h.host.GitClientForUserBinding(ctx, user, binding)
	if !ok {
		return
	}

	effectiveRef := strings.TrimSpace(input.Ref)
	if effectiveRef == "" {
		effectiveRef = strings.TrimSpace(binding.DefaultBranch)
	}
	if effectiveRef == "" {
		effectiveRef = "HEAD"
	}

	reqCtx := ctx.Request.Context()
	buildOptions, err := gitClient.DiscoverBuildOptions(reqCtx, binding.Owner, binding.Repo, effectiveRef)
	if err != nil {
		h.host.WriteGitUpstreamError(ctx, err)
		return
	}

	snippets := collectRepoInferenceSnippets(reqCtx, gitClient, binding.Owner, binding.Repo, effectiveRef, subpath, buildOptions)
	inspectedFiles := make([]string, 0, len(snippets))
	for _, item := range snippets {
		inspectedFiles = append(inspectedFiles, item.Path)
	}

	recommendedTemplateIDs := buildtemplate.Recommend(buildOptions.DetectedFiles)
	availableTemplates := buildtemplate.List()
	stage := strings.TrimSpace(input.Stage)
	if stage == "" {
		stage = "prod"
	}

	inferenceResp, ok := h.host.RunHeadlessInferenceForUser(ctx, user, aiagent.HeadlessInferenceRequest{
		ActorUserID: user.ID,
		ProjectID:   project.ID,
		TaskType:    aiagent.HeadlessTaskRepoConfigInfer,
		MaxToolTier: aiagent.HeadlessToolTierNone,
		ContextPayload: map[string]any{
			"applicationIdentifier": app.Identifier,
			"applicationName":       app.Name,
			"stage":                 stage,
			"owner":                 binding.Owner,
			"repo":                  binding.Repo,
			"ref":                   effectiveRef,
			"subpath":               subpath,
			"dockerfiles":           buildOptions.Dockerfiles,
			"directories":           buildOptions.Directories,
			"detectedFiles":         buildOptions.DetectedFiles,
			"exposedPorts":          buildOptions.ExposedPorts,
			"recommendedTemplates":  recommendedTemplateIDs,
			"availableTemplates":    availableTemplates,
			"files":                 snippets,
		},
		OutputSchema: repoConfigInferenceOutputSchema(),
	})
	if !ok {
		return
	}

	var rawOut DeploymentTargetConfigInferenceOutput
	if err := json.Unmarshal(inferenceResp.Output, &rawOut); err != nil {
		writeErrorCode(ctx, http.StatusBadGateway, "ai.structured_output_invalid", "AI returned invalid structured configuration output")
		return
	}

	normalized := sanitizeInferredDeploymentConfig(rawOut, buildOptions, subpath, inspectedFiles)
	h.auditWithContext(user.ID, "deployment_target.ai_infer_config", app.ID, true, binding.ID, reqCtx)
	ctx.JSON(http.StatusOK, normalized)
}

func sanitizeInferenceSubpath(raw string) (string, bool) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" || trimmed == "." {
		return "", true
	}
	if strings.Contains(trimmed, `\`) {
		return "", false
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", false
		}
	}
	return path.Clean(trimmed), true
}

var priorityManifestNames = []string{
	"package.json",
	"go.mod",
	"pyproject.toml",
	"requirements.txt",
	"Cargo.toml",
	"pom.xml",
	"build.gradle",
	"build.gradle.kts",
	"next.config.ts",
	"next.config.mjs",
	"next.config.js",
	"vite.config.ts",
	"vite.config.js",
	".env.example",
	"compose.yaml",
	"docker-compose.yml",
}

func collectRepoInferenceSnippets(
	ctx context.Context,
	client gitprovider.Client,
	owner, repo, ref, subpath string,
	options gitprovider.BuildOptions,
) []repoFileSnippet {
	candidatePaths := make([]string, 0, 16)
	addCandidate := func(p string) {
		cleaned := strings.Trim(strings.TrimSpace(p), "/")
		if cleaned == "" || isIgnoredSnippetCandidate(cleaned) || slices.Contains(candidatePaths, cleaned) {
			return
		}
		candidatePaths = append(candidatePaths, cleaned)
	}

	// 1. Dockerfiles (preferring subpath if provided)
	for _, df := range options.Dockerfiles {
		if subpath != "" && strings.HasPrefix(df, subpath+"/") {
			addCandidate(df)
		}
	}
	for _, df := range options.Dockerfiles {
		addCandidate(df)
	}

	// 2. Detected manifest files from DiscoverBuildOptions
	detectedSet := make(map[string]bool, len(options.DetectedFiles))
	for _, f := range options.DetectedFiles {
		detectedSet[strings.Trim(f, "/")] = true
	}
	if subpath != "" {
		for _, name := range priorityManifestNames {
			subCandidate := subpath + "/" + name
			if detectedSet[subCandidate] {
				addCandidate(subCandidate)
			}
		}
	}
	for _, name := range priorityManifestNames {
		if detectedSet[name] {
			addCandidate(name)
		}
	}
	for _, f := range options.DetectedFiles {
		if subpath != "" && strings.HasPrefix(f, subpath+"/") {
			addCandidate(f)
		}
	}
	for _, f := range options.DetectedFiles {
		addCandidate(f)
	}

	// 3. Also check directory listing of subpath (or root) for .env.example / compose files not in buildTemplateDetectionFiles.
	if items, err := client.ListContents(ctx, owner, repo, subpath, ref); err == nil {
		for _, item := range items {
			if item.Type != "file" {
				continue
			}
			lower := strings.ToLower(item.Name)
			if lower == ".env.example" || lower == ".env.sample" || lower == "compose.yaml" || lower == "compose.yml" || lower == "docker-compose.yml" || lower == "docker-compose.yaml" || lower == "dockerfile" {
				addCandidate(item.Path)
			}
		}
	}

	snippets := make([]repoFileSnippet, 0, maxInferredFileCount)
	totalBytes := 0
	for _, filePath := range candidatePaths {
		if len(snippets) >= maxInferredFileCount || totalBytes >= maxInferredTotalFilesBytes {
			break
		}
		fc, err := client.ReadFile(ctx, owner, repo, filePath, ref)
		if err != nil || strings.TrimSpace(fc.Content) == "" {
			continue
		}
		if strings.IndexByte(fc.Content, 0) >= 0 || !utf8.ValidString(fc.Content) {
			continue
		}
		sanitized := sanitizeRepoSnippetContent(filePath, fc.Content)
		remaining := maxInferredTotalFilesBytes - totalBytes
		limit := min(maxInferredSingleFileBytes, remaining)
		truncated := false
		if len(sanitized) > limit {
			sanitized = truncateValidUTF8(sanitized, limit)
			truncated = true
		}
		totalBytes += len(sanitized)
		snippets = append(snippets, repoFileSnippet{
			Path:      filePath,
			Content:   sanitized,
			Truncated: truncated,
		})
	}
	return snippets
}

func isIgnoredSnippetCandidate(filePath string) bool {
	base := strings.ToLower(path.Base(filePath))
	if strings.HasSuffix(base, ".lock") || strings.HasSuffix(base, ".lockb") {
		return true
	}
	switch base {
	case "pnpm-lock.yaml", "package-lock.json", "npm-shrinkwrap.json", "gradlew", "gradlew.bat":
		return true
	default:
		return false
	}
}

func sanitizeRepoSnippetContent(filePath, content string) string {
	masked := telemetry.RedactText(content)
	lines := strings.Split(masked, "\n")
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := dockerInstructionKVPattern.FindStringSubmatch(line); len(m) == 5 {
			if strings.Contains(line, "=") {
				lines[idx] = inlineEnvPairPattern.ReplaceAllStringFunc(line, func(pair string) string {
					pm := inlineEnvPairPattern.FindStringSubmatch(pair)
					if len(pm) == 4 && sensitiveEnvKeyNamePattern.MatchString(pm[1]) {
						return pm[1] + "=<redacted>"
					}
					return pair
				})
			} else if sensitiveEnvKeyNamePattern.MatchString(m[2]) && strings.TrimSpace(m[4]) != "" {
				lines[idx] = m[1] + m[2] + "=<redacted>"
			}
			continue
		}
		if m := yamlSensitiveKVPattern.FindStringSubmatch(line); len(m) == 5 {
			key := m[2]
			val := strings.TrimSpace(m[4])
			if val != "" && sensitiveEnvKeyNamePattern.MatchString(key) {
				lines[idx] = m[1] + key + m[3] + "<redacted>"
				continue
			}
		}
		eq := strings.IndexByte(line, '=')
		if eq > 0 {
			key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line[:eq]), "export "))
			val := strings.TrimSpace(line[eq+1:])
			if val != "" && inferredEnvKeyPattern.MatchString(key) && sensitiveEnvKeyNamePattern.MatchString(key) {
				lines[idx] = key + "=<redacted>"
			}
		}
	}
	return strings.Join(lines, "\n")
}

func truncateValidUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return strings.ToValidUTF8(s, "")
	}
	cut := s[:maxBytes]
	for i := 0; i < utf8.UTFMax && len(cut) > 0; i++ {
		r, size := utf8.DecodeLastRuneInString(cut)
		if r != utf8.RuneError || size != 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return strings.ToValidUTF8(cut, "")
}

func sanitizeInferredDeploymentConfig(
	raw DeploymentTargetConfigInferenceOutput,
	options gitprovider.BuildOptions,
	subpath string,
	inspectedFiles []string,
) DeploymentTargetConfigInferenceOutput {
	out := DeploymentTargetConfigInferenceOutput{
		Summary:              strings.TrimSpace(raw.Summary),
		Confidence:           normalizeInferenceConfidence(raw.Confidence),
		BuildArgs:            []InferredBuildArg{},
		ServicePorts:         []InferredServicePort{},
		EnvironmentVariables: []InferredEnvironmentVariable{},
		ContainerCommand:     []string{},
		ContainerArgs:        []string{},
		Notes:                []string{},
		InspectedFiles:       inspectedFiles,
		BuildTemplateValues:  map[string]string{},
	}

	mode := strings.TrimSpace(raw.BuildDefinitionMode)
	if mode != buildtemplate.DefinitionModeTemplate {
		mode = buildtemplate.DefinitionModeRepository
	}

	if mode == buildtemplate.DefinitionModeTemplate {
		tplID := strings.TrimSpace(raw.BuildTemplateID)
		tplVer := strings.TrimSpace(raw.BuildTemplateVersion)
		if def, found := buildtemplate.Find(tplID, tplVer); found {
			out.BuildDefinitionMode = buildtemplate.DefinitionModeTemplate
			out.BuildTemplateID = def.ID
			out.BuildTemplateVersion = def.Version
			allowedParams := make(map[string]buildtemplate.Parameter, len(def.Parameters))
			for _, p := range def.Parameters {
				allowedParams[p.Key] = p
			}
			for k, v := range raw.BuildTemplateValues {
				param, exists := allowedParams[k]
				if !exists {
					continue
				}
				trimmedVal := strings.TrimSpace(v)
				if trimmedVal == "" {
					continue
				}
				if len(param.Options) > 0 && !slices.Contains(param.Options, trimmedVal) {
					continue
				}
				out.BuildTemplateValues[k] = trimmedVal
			}
			for _, p := range def.Parameters {
				if _, exists := out.BuildTemplateValues[p.Key]; !exists && p.DefaultValue != "" {
					out.BuildTemplateValues[p.Key] = p.DefaultValue
				}
			}
		} else if len(options.Dockerfiles) > 0 {
			mode = buildtemplate.DefinitionModeRepository
		} else if recommended := buildtemplate.Recommend(options.DetectedFiles); len(recommended) > 0 {
			if fallbackDef, ok := buildtemplate.Find(recommended[0], ""); ok {
				out.BuildDefinitionMode = buildtemplate.DefinitionModeTemplate
				out.BuildTemplateID = fallbackDef.ID
				out.BuildTemplateVersion = fallbackDef.Version
				for _, p := range fallbackDef.Parameters {
					if p.DefaultValue != "" {
						out.BuildTemplateValues[p.Key] = p.DefaultValue
					}
				}
			}
		}
	}

	if out.BuildDefinitionMode == "" {
		out.BuildDefinitionMode = buildtemplate.DefinitionModeRepository
	}

	defaultCtx := "."
	if subpath != "" {
		defaultCtx = subpath
	}
	out.BuildContext = sanitizeRelativeRepoPath(raw.BuildContext, defaultCtx)
	out.BuildDirectory = sanitizeRelativeRepoPath(raw.BuildDirectory, "")
	if out.BuildDefinitionMode == buildtemplate.DefinitionModeRepository {
		defaultDF := "Dockerfile"
		if len(options.Dockerfiles) > 0 {
			defaultDF = options.Dockerfiles[0]
		}
		out.DockerfilePath = sanitizeRelativeRepoPath(raw.DockerfilePath, defaultDF)
	} else {
		out.DockerfilePath = "Dockerfile"
	}

	seenArgKeys := map[string]bool{}
	for _, arg := range raw.BuildArgs {
		key := strings.TrimSpace(arg.Key)
		if !inferredEnvKeyPattern.MatchString(key) || seenArgKeys[key] || sensitiveEnvKeyNamePattern.MatchString(key) {
			continue
		}
		seenArgKeys[key] = true
		out.BuildArgs = append(out.BuildArgs, InferredBuildArg{
			Key:   key,
			Value: telemetry.RedactText(strings.TrimSpace(arg.Value)),
		})
	}

	seenPorts := map[int]bool{}
	seenPortNames := map[string]bool{}
	for _, sp := range raw.ServicePorts {
		if sp.Port < 1 || sp.Port > 65535 || seenPorts[sp.Port] || len(out.ServicePorts) >= 8 {
			continue
		}
		seenPorts[sp.Port] = true
		name := strings.ToLower(strings.TrimSpace(sp.Name))
		if !inferredPortNamePattern.MatchString(name) {
			if len(out.ServicePorts) == 0 && !seenPortNames["http"] {
				name = "http"
			} else {
				name = "port-" + strings.TrimSpace(jsonNumberString(sp.Port))
			}
		}
		if seenPortNames[name] {
			base := "port-" + strings.TrimSpace(jsonNumberString(sp.Port))
			name = base
			for suffix := 2; seenPortNames[name]; suffix++ {
				name = base + "-" + strings.TrimSpace(jsonNumberString(suffix))
			}
		}
		seenPortNames[name] = true
		proto := strings.ToLower(strings.TrimSpace(sp.AppProtocol))
		if proto == "" {
			proto = "http"
		}
		out.ServicePorts = append(out.ServicePorts, InferredServicePort{
			Name:        name,
			Port:        sp.Port,
			AppProtocol: proto,
		})
	}
	if len(out.ServicePorts) == 0 {
		fallbackPort := 8080
		if out.BuildDefinitionMode == buildtemplate.DefinitionModeTemplate {
			if def, ok := buildtemplate.Find(out.BuildTemplateID, out.BuildTemplateVersion); ok && def.DefaultServicePort > 0 {
				fallbackPort = def.DefaultServicePort
			}
		} else if ports := options.ExposedPorts[out.DockerfilePath]; len(ports) > 0 && ports[0] >= 1 && ports[0] <= 65535 {
			fallbackPort = ports[0]
		}
		out.ServicePorts = []InferredServicePort{{Name: "http", Port: fallbackPort, AppProtocol: "http"}}
	}

	seenEnvKeys := map[string]bool{}
	for _, env := range raw.EnvironmentVariables {
		key := strings.TrimSpace(env.Key)
		if !inferredEnvKeyPattern.MatchString(key) || seenEnvKeys[key] || len(out.EnvironmentVariables) >= 32 {
			continue
		}
		seenEnvKeys[key] = true
		isSecret := env.Secret || sensitiveEnvKeyNamePattern.MatchString(key)
		placeholder := strings.TrimSpace(env.Placeholder)
		if isSecret {
			// Never populate any value/placeholder for secret fields.
			placeholder = ""
		}
		out.EnvironmentVariables = append(out.EnvironmentVariables, InferredEnvironmentVariable{
			Key:         key,
			Secret:      isSecret,
			Placeholder: placeholder,
		})
	}

	out.CPURequest = sanitizeQuantityOrDefault(raw.CPURequest, model.DefaultDeploymentCPURequest)
	out.MemoryRequest = sanitizeQuantityOrDefault(raw.MemoryRequest, model.DefaultDeploymentMemoryRequest)

	for _, item := range raw.ContainerCommand {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out.ContainerCommand = append(out.ContainerCommand, trimmed)
		}
	}
	for _, item := range raw.ContainerArgs {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out.ContainerArgs = append(out.ContainerArgs, trimmed)
		}
	}
	for _, note := range raw.Notes {
		if trimmed := strings.TrimSpace(note); trimmed != "" && len(out.Notes) < 8 {
			out.Notes = append(out.Notes, trimmed)
		}
	}
	return out
}

func normalizeInferenceConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func sanitizeRelativeRepoPath(raw, defaultValue string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultValue
	}
	if trimmed == "." {
		return "."
	}
	cleaned, ok := sanitizeInferenceSubpath(trimmed)
	if !ok || cleaned == "" {
		return defaultValue
	}
	return cleaned
}

func sanitizeQuantityOrDefault(raw, fallbackQty string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallbackQty
	}
	qty, err := resource.ParseQuantity(trimmed)
	if err != nil || qty.Sign() <= 0 {
		return fallbackQty
	}
	return trimmed
}

func jsonNumberString(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func repoConfigInferenceOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary":              map[string]any{"type": "string", "minLength": 1, "maxLength": 800},
			"confidence":           map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
			"buildDefinitionMode":  map[string]any{"type": "string", "enum": []string{"repository_dockerfile", "template"}},
			"dockerfilePath":       map[string]any{"type": "string"},
			"buildContext":         map[string]any{"type": "string"},
			"buildDirectory":       map[string]any{"type": "string"},
			"buildTemplateId":      map[string]any{"type": "string"},
			"buildTemplateVersion": map[string]any{"type": "string"},
			"buildTemplateValues": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
			"buildArgs": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"key":   map[string]any{"type": "string", "minLength": 1},
						"value": map[string]any{"type": "string"},
					},
					"required":             []string{"key", "value"},
					"additionalProperties": false,
				},
			},
			"servicePorts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":        map[string]any{"type": "string"},
						"port":        map[string]any{"type": "integer", "minimum": 1, "maximum": 65535},
						"appProtocol": map[string]any{"type": "string"},
					},
					"required":             []string{"port"},
					"additionalProperties": false,
				},
			},
			"environmentVariables": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"key":         map[string]any{"type": "string", "minLength": 1},
						"secret":      map[string]any{"type": "boolean"},
						"placeholder": map[string]any{"type": "string"},
					},
					"required":             []string{"key", "secret"},
					"additionalProperties": false,
				},
			},
			"cpuRequest":       map[string]any{"type": "string"},
			"memoryRequest":    map[string]any{"type": "string"},
			"containerCommand": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"containerArgs":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"notes":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"summary", "buildDefinitionMode"},
		"additionalProperties": false,
	}
}
