package buildruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/imageref"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/variables"
	"gorm.io/gorm"
)

const scopedResourceBuildVariableSet = "build_variable_set"

type Resolver struct {
	DB      *gorm.DB
	Secrets secret.Store
}

type ResolvedTask struct {
	Task            builder.Task
	SensitiveValues []string
}

func (r Resolver) ResolveBuildTask(tx *gorm.DB, run model.BuildRun, job model.BuildJob) (ResolvedTask, error) {
	if tx == nil {
		tx = r.DB
	}
	if tx == nil {
		return ResolvedTask{}, errors.New("database is required")
	}
	binding, err := r.repositoryBindingForRun(tx, run)
	if err != nil {
		return ResolvedTask{}, err
	}
	var gitAccount model.GitAccount
	if err := tx.First(&gitAccount, "id = ?", binding.GitAccountID).Error; err != nil {
		return ResolvedTask{}, fmt.Errorf("git account not found: %w", err)
	}
	gitToken := r.Secrets.Resolve(gitAccount.AccessTokenRef)
	if strings.TrimSpace(gitToken) == "" && !repositoryBindingLooksPublic(binding) {
		return ResolvedTask{}, errors.New("git access token is missing")
	}
	if strings.TrimSpace(run.TargetRegistryID) == "" {
		return ResolvedTask{}, errors.New("target registry is required")
	}
	var registry model.ArtifactRegistry
	if err := tx.First(&registry, "id = ?", run.TargetRegistryID).Error; err != nil {
		return ResolvedTask{}, fmt.Errorf("target registry not found: %w", err)
	}
	credential, err := r.registryCredentialForBuild(tx, run.CreatedBy, registry)
	if err != nil {
		return ResolvedTask{}, err
	}
	registrySecret := r.Secrets.Resolve(credential.TokenRef)
	if registrySecret == "" {
		registrySecret = r.Secrets.Resolve(credential.PasswordRef)
	}
	if strings.TrimSpace(registrySecret) == "" {
		return ResolvedTask{}, errors.New("registry credential secret is missing")
	}
	var project model.Project
	if err := tx.First(&project, "id = ?", run.ProjectID).Error; err != nil {
		return ResolvedTask{}, fmt.Errorf("project not found: %w", err)
	}
	var application model.Application
	if err := tx.First(&application, "id = ? and project_id = ?", run.ApplicationID, run.ProjectID).Error; err != nil {
		return ResolvedTask{}, fmt.Errorf("application not found: %w", err)
	}
	if strings.TrimSpace(run.TargetRepository) == "" {
		var target model.DeploymentTarget
		if strings.TrimSpace(run.DeploymentTargetID) != "" {
			_ = tx.First(&target, "id = ? and project_id = ? and application_id = ?", run.DeploymentTargetID, run.ProjectID, run.ApplicationID).Error
		}
		run.TargetRepository = BuildTargetImageRepositoryForCredential(registry, credential, project, application, target)
	}
	if strings.TrimSpace(run.TargetTag) == "" {
		run.TargetTag = BuildTargetImageTagTemplateForCredential(credential)
	}
	imageRef := fallback(strings.TrimSpace(run.ImageRef), BuildImageRef(registry, run))
	var actor model.User
	if err := tx.First(&actor, "id = ?", run.CreatedBy).Error; err != nil {
		return ResolvedTask{}, fmt.Errorf("build actor not found: %w", err)
	}
	buildEnv, secretValues, err := r.buildVariablesForRunByIDs(tx, actor, run.ProjectID, BuildVariableSetIDs(run.BuildVariableSetIDs))
	if err != nil {
		return ResolvedTask{}, fmt.Errorf("build variables are unavailable: %w", err)
	}
	hooks, err := r.hookPayloadsForRun(tx, run, job)
	if err != nil {
		return ResolvedTask{}, err
	}
	sensitiveValues := []string{
		gitToken,
		r.Secrets.Resolve(binding.CredentialRef),
		r.Secrets.Resolve(gitAccount.RefreshTokenRef),
		r.Secrets.Resolve(credential.TokenRef),
		r.Secrets.Resolve(credential.PasswordRef),
	}
	sensitiveValues = append(sensitiveValues, secretValues...)
	return ResolvedTask{
		Task: builder.Task{
			JobID:              job.ID,
			BuildRunID:         run.ID,
			ProjectID:          run.ProjectID,
			ApplicationID:      run.ApplicationID,
			DeploymentTargetID: run.DeploymentTargetID,
			Repository: builder.RepositoryPayload{
				CloneURL:     binding.CloneURL,
				Owner:        binding.Owner,
				Repo:         binding.Repo,
				SourceBranch: fallback(run.SourceBranch, binding.DefaultBranch),
				SourceTag:    run.SourceTag,
				SourceCommit: run.SourceCommit,
				AccessToken:  gitToken,
			},
			Build: builder.BuildPayload{
				DockerfilePath: fallback(run.DockerfilePath, "Dockerfile"),
				BuildContext:   fallback(run.BuildContext, "."),
				BuildDirectory: run.BuildDirectory,
				Env:            buildEnv,
				Hooks:          hooks,
			},
			Registry: builder.RegistryPayload{
				Endpoint:         RegistryAuthEndpoint(registry.Endpoint),
				Username:         credential.Username,
				Password:         registrySecret,
				ImageRef:         imageRef,
				ImageNamePrefix:  BuildImageNamePrefix(registry, run.TargetRepository),
				ImageTagTemplate: fallback(strings.TrimSpace(run.TargetTag), "latest"),
			},
		},
		SensitiveValues: normalizedSensitiveValues(sensitiveValues),
	}, nil
}

func (r Resolver) repositoryBindingForRun(tx *gorm.DB, run model.BuildRun) (model.RepositoryBinding, error) {
	var binding model.RepositoryBinding
	query := tx.Where("project_id = ? and application_id = ?", run.ProjectID, run.ApplicationID)
	if strings.TrimSpace(run.DeploymentTargetID) != "" {
		var target model.DeploymentTarget
		if err := tx.First(&target, "id = ? and project_id = ? and application_id = ?", run.DeploymentTargetID, run.ProjectID, run.ApplicationID).Error; err != nil {
			return binding, fmt.Errorf("deployment target not found: %w", err)
		}
		if strings.TrimSpace(target.RepositoryBindingID) != "" {
			query = query.Where("id = ?", target.RepositoryBindingID)
		}
	}
	if err := query.First(&binding).Error; err != nil {
		return binding, fmt.Errorf("repository binding not found: %w", err)
	}
	return binding, nil
}

func repositoryBindingLooksPublic(binding model.RepositoryBinding) bool {
	cloneURL := strings.ToLower(strings.TrimSpace(binding.CloneURL))
	return strings.HasPrefix(cloneURL, "https://github.com/") ||
		strings.HasPrefix(cloneURL, "https://gitea.com/") ||
		strings.HasPrefix(cloneURL, "https://gitlab.com/")
}

func (r Resolver) registryCredentialForBuild(tx *gorm.DB, actorID string, registry model.ArtifactRegistry) (model.RegistryCredential, error) {
	var credential model.RegistryCredential
	if strings.TrimSpace(registry.CredentialRef) != "" {
		err := tx.First(&credential, "id = ? and registry_id = ? and scope in ? and (access_scope = ? or created_by = ?)",
			registry.CredentialRef, registry.ID, []string{"push", "push-pull"}, "registry", actorID).Error
		if err == nil {
			return credential, nil
		}
	}
	err := tx.Where("registry_id = ? and access_scope = ? and created_by = ? and scope in ?",
		registry.ID, "personal", actorID, []string{"push", "push-pull"}).Order("created_at desc").First(&credential).Error
	if err == nil {
		return credential, nil
	}
	if registry.Scope != "global" {
		err = tx.Where("registry_id = ? and access_scope = ? and scope in ?",
			registry.ID, "registry", []string{"push", "push-pull"}).Order("created_at desc").First(&credential).Error
		if err == nil {
			return credential, nil
		}
	}
	return model.RegistryCredential{}, errors.New("usable registry credential not found")
}

func (r Resolver) hookPayloadsForRun(tx *gorm.DB, run model.BuildRun, job model.BuildJob) ([]builder.HookPayload, error) {
	if strings.TrimSpace(run.DeploymentTargetID) == "" {
		return nil, nil
	}
	var existing []model.HookRun
	if err := tx.Where("project_id = ? and build_job_id = ?", run.ProjectID, job.ID).Order("created_at asc").Find(&existing).Error; err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		hooks := make([]builder.HookPayload, 0, len(existing))
		for _, runRecord := range existing {
			hooks = append(hooks, hookPayloadFromRun(runRecord))
		}
		return hooks, nil
	}
	var target model.DeploymentTarget
	if err := tx.First(&target, "id = ? and project_id = ? and application_id = ?", run.DeploymentTargetID, run.ProjectID, run.ApplicationID).Error; err != nil {
		return nil, err
	}
	if !target.BuildHooksEnabled {
		return nil, nil
	}
	var bindings []model.DeploymentTargetHookBinding
	if err := tx.Where("project_id = ? and application_id = ? and target_id = ?", run.ProjectID, run.ApplicationID, run.DeploymentTargetID).
		Order("run_order asc, created_at asc").
		Find(&bindings).Error; err != nil {
		return nil, err
	}
	hookIDs := make([]string, 0, len(bindings))
	buildBindings := make([]model.DeploymentTargetHookBinding, 0, len(bindings))
	for _, binding := range bindings {
		if !builder.IsBuildHookPhase(binding.Phase) {
			continue
		}
		buildBindings = append(buildBindings, binding)
		hookIDs = append(hookIDs, binding.HookConfigID)
	}
	if len(hookIDs) == 0 {
		return nil, nil
	}
	var configs []model.ProjectHookConfig
	if err := tx.Where("project_id = ? and id in ?", run.ProjectID, hookIDs).Find(&configs).Error; err != nil {
		return nil, err
	}
	configsByID := make(map[string]model.ProjectHookConfig, len(configs))
	for _, config := range configs {
		configsByID[config.ID] = config
	}
	hooks := make([]builder.HookPayload, 0, len(configs))
	for _, binding := range buildBindings {
		config, ok := configsByID[binding.HookConfigID]
		if !ok {
			continue
		}
		runRecord := model.HookRun{
			ID:                 id.New("hrun"),
			ProjectID:          run.ProjectID,
			HookConfigID:       config.ID,
			BuildRunID:         run.ID,
			BuildJobID:         job.ID,
			ApplicationID:      run.ApplicationID,
			DeploymentTargetID: run.DeploymentTargetID,
			Name:               config.Name,
			Phase:              binding.Phase,
			Status:             "queued",
			ScriptSnapshot:     config.Script,
			Shell:              config.Shell,
			TimeoutSeconds:     config.TimeoutSeconds,
			FailurePolicy:      config.FailurePolicy,
		}
		if err := tx.Create(&runRecord).Error; err != nil {
			return nil, err
		}
		hooks = append(hooks, hookPayloadFromRun(runRecord))
	}
	return hooks, nil
}

func hookPayloadFromRun(run model.HookRun) builder.HookPayload {
	return builder.HookPayload{
		ID:             run.ID,
		Name:           run.Name,
		Phase:          run.Phase,
		Script:         run.ScriptSnapshot,
		Shell:          run.Shell,
		TimeoutSeconds: run.TimeoutSeconds,
		FailurePolicy:  run.FailurePolicy,
	}
}

func (r Resolver) buildVariablesForRunByIDs(db *gorm.DB, user model.User, projectID string, setIDs []string) (map[string]string, []string, error) {
	output := make(map[string]string)
	var sensitive []string
	sets, err := r.buildVariableSetsForRun(db, user, projectID, setIDs)
	if err != nil {
		return nil, nil, err
	}
	for _, set := range sets {
		setSensitive := applyBuildVariableSetValues(output, set, r.Secrets.Resolve)
		sensitive = append(sensitive, setSensitive...)
	}
	return output, sensitive, nil
}

func (r Resolver) buildVariableSetsForRun(db *gorm.DB, user model.User, projectID string, setIDs []string) ([]model.BuildVariableSet, error) {
	sets := make([]model.BuildVariableSet, 0)
	seen := make(map[string]bool)
	var defaultSets []model.BuildVariableSet
	if err := db.Joins(
		"join scoped_resource_project_bindings srpb on srpb.resource_type = ? and srpb.resource_id = build_variable_sets.id and srpb.project_id = ?",
		scopedResourceBuildVariableSet,
		strings.TrimSpace(projectID),
	).Where("build_variable_sets.scope = ? and build_variable_sets.enabled = ?", "project", true).Order("build_variable_sets.created_at asc").Find(&defaultSets).Error; err != nil {
		return nil, err
	}
	for _, set := range defaultSets {
		if !r.buildVariableSetAccessible(db, user, projectID, set) {
			continue
		}
		sets = append(sets, set)
		seen[set.ID] = true
	}
	for _, setID := range normalizeStringList(setIDs) {
		if seen[setID] {
			continue
		}
		seen[setID] = true
		var set model.BuildVariableSet
		if err := db.First(&set, "id = ? and enabled = ?", setID, true).Error; err != nil {
			return nil, errors.New("variables are unavailable")
		}
		if !r.buildVariableSetAccessible(db, user, projectID, set) {
			return nil, errors.New("variables are not allowed")
		}
		sets = append(sets, set)
	}
	return sets, nil
}

func (r Resolver) buildVariableSetAccessible(db *gorm.DB, user model.User, projectID string, set model.BuildVariableSet) bool {
	switch set.Scope {
	case "global":
		return true
	case "user":
		return set.OwnerRef == user.ID
	case "project":
		if user.Role == "platform_admin" {
			return true
		}
		var count int64
		if err := db.Model(&model.ScopedResourceProjectBinding{}).Where("resource_type = ? and resource_id = ? and project_id = ?", scopedResourceBuildVariableSet, set.ID, projectID).Count(&count).Error; err != nil {
			return false
		}
		return count > 0
	default:
		return false
	}
}

func applyBuildVariableSetValues(output map[string]string, set model.BuildVariableSet, resolveSecret func(string) string) []string {
	var values map[string]string
	if err := json.Unmarshal([]byte(fallback(set.Variables, "{}")), &values); err == nil {
		for key, value := range values {
			if isBuildEnvKey(key) {
				output[key] = value
			}
		}
	}
	sensitive := make([]string, 0)
	for key, ref := range decodeSecretRefs(set.SecretRefs) {
		if !isBuildEnvKey(key) {
			continue
		}
		if secretValue := resolveSecret(ref); secretValue != "" {
			output[key] = secretValue
			sensitive = append(sensitive, secretValue)
		}
	}
	return sensitive
}

func BuildVariableSetIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err == nil {
		return normalizeStringList(ids)
	}
	return normalizeStringList(strings.Split(raw, ","))
}

func decodeSecretRefs(raw string) map[string]string {
	refs := map[string]string{}
	if err := json.Unmarshal([]byte(fallback(raw, "{}")), &refs); err != nil {
		return map[string]string{}
	}
	return refs
}

func isBuildEnvKey(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, char := range value {
		if index == 0 {
			if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' {
				continue
			}
			return false
		}
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func RegistryAuthEndpoint(endpoint string) string {
	return imageref.RegistryAuthEndpoint(endpoint)
}

func BuildImageRef(registry model.ArtifactRegistry, run model.BuildRun) string {
	return imageref.BuildImageRef(registry, run)
}

func BuildTargetImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application) string {
	return imageref.BuildTargetImageRepository(registry, project, application)
}

func BuildTargetImageRepositoryForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return imageref.BuildTargetImageRepositoryForCredential(registry, credential, project, application, target)
}

func BuildTargetImageTagTemplateForCredential(credential model.RegistryCredential) string {
	return imageref.BuildTargetImageTagTemplateForCredential(credential)
}

func BuildImageNamePrefix(registry model.ArtifactRegistry, repository string) string {
	return imageref.BuildImageNamePrefix(registry, repository)
}

func RenderBuildTagTemplate(template string, ctx variables.Context) string {
	return imageref.RenderBuildTagTemplate(template, ctx)
}

func sanitizeImageTag(value string) string {
	return imageref.RenderBuildTagTemplate(value, variables.Context{})
}

func isDockerHubRegistry(registry model.ArtifactRegistry) bool {
	return imageref.IsDockerHubRegistry(registry)
}

func hasRegistryHost(repository string) bool {
	first := strings.Split(strings.Trim(repository, "/"), "/")[0]
	return strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost"
}

func registryImageHost(endpoint string) string {
	return imageref.RegistryImageHost(endpoint)
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

func normalizedSensitiveValues(values []string) []string {
	seen := map[string]bool{}
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len(value) < 4 || seen[value] {
			continue
		}
		seen[value] = true
		output = append(output, value)
	}
	return output
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return defaultValue
}
