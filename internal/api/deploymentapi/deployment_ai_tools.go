package deploymentapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	aiDNSLabelPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
)

// RegisterAITools registers all 18 deployment, build-pipeline, and runtime diagnostics
// AI Domain Facade Tools owned by deploymentapi.
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	if reg == nil {
		return
	}
	// 1. delivery read tools (3)
	h.registerDeliveryReadAITools(reg)
	// 2. delivery target desired-state write/delete tools (5)
	h.registerDeliveryTargetWriteAITools(reg)
	// 3. delivery workload/release execution tools (5)
	h.registerDeliveryReleaseExecAITools(reg)
	// 4. build pipeline configuration tool (1)
	h.registerBuildTargetPipelineAITool(reg)
	// 5. runtime diagnostics, pods/events, logs, and exec tools (4)
	h.registerRuntimeDiagnosticsAITools(reg)
}

func (h *Handler) aiDB(tc *aitool.ToolContext) *gorm.DB {
	if tc != nil && tc.DB != nil {
		return tc.DB
	}
	return h.dbWithContext(tc.Context())
}

// newScratchGin creates an isolated child *gin.Context + *httptest.ResponseRecorder
// inheriting request context, headers, and Gin keys so reused domain helpers can
// run without polluting tc.Recorder across multiple checks.
func newScratchGin(tc *aitool.ToolContext) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	subGin, _ := gin.CreateTestContext(rec)
	reqCtx := tc.Context()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, "/internal/v1/ai/tools", http.NoBody)
	if tc != nil && tc.Gin != nil && tc.Gin.Request != nil {
		for k, vals := range tc.Gin.Request.Header {
			req.Header[k] = append([]string(nil), vals...)
		}
		req.RemoteAddr = tc.Gin.Request.RemoteAddr
	}
	if tc != nil && strings.TrimSpace(tc.IdempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(tc.IdempotencyKey))
	}
	subGin.Request = req
	if tc != nil && tc.Gin != nil && tc.Gin.Keys != nil {
		subGin.Keys = maps.Clone(tc.Gin.Keys)
	}
	return subGin, rec
}

func toolErrorFromRecorder(rec *httptest.ResponseRecorder, fallbackStatus int, fallbackCode, fallbackMessage string) *aitool.ToolError {
	status := rec.Code
	if status < 400 {
		status = fallbackStatus
	}
	if status < 400 {
		status = http.StatusBadRequest
	}
	raw := bytes.TrimSpace(rec.Body.Bytes())
	if len(raw) == 0 {
		return aitool.NewError(status, fallbackCode, fallbackMessage)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return aitool.NewError(status, fallbackCode, string(raw))
	}
	code, _ := parsed["code"].(string)
	if strings.TrimSpace(code) == "" {
		code = fallbackCode
	}
	msg, _ := parsed["message"].(string)
	if strings.TrimSpace(msg) == "" {
		msg = fallbackMessage
	}
	return &aitool.ToolError{
		Status:  status,
		Code:    code,
		Message: msg,
		RawBody: append(json.RawMessage(nil), raw...),
	}
}

func (h *Handler) authorizeAIProject(tc *aitool.ToolContext, projectRef string, action authz.Action) (model.User, model.Project, error) {
	ref := strings.TrimSpace(projectRef)
	if ref == "" {
		return model.User{}, model.Project{}, aitool.BadRequest("project.required", "project is required")
	}
	scratchCtx, rec := newScratchGin(tc)
	user, project, ok := h.host.AuthorizeProjectRef(scratchCtx, ref, action)
	if !ok {
		return model.User{}, model.Project{}, toolErrorFromRecorder(rec, http.StatusForbidden, "project.access_denied", "project authorization failed")
	}
	if tc != nil && tc.Gin != nil && scratchCtx.Keys != nil {
		if tc.Gin.Keys == nil {
			tc.Gin.Keys = make(map[any]any)
		}
		for k, v := range scratchCtx.Keys {
			tc.Gin.Keys[k] = v
		}
	}
	return user, project, nil
}

func (h *Handler) ensureAIProjectCanMutate(tc *aitool.ToolContext, project model.Project) error {
	scratchCtx, rec := newScratchGin(tc)
	if !h.ensureProjectCanMutate(scratchCtx, project) {
		return toolErrorFromRecorder(rec, http.StatusConflict, "project.delete_in_progress", "project cannot be mutated while deletion is in progress")
	}
	return nil
}

func ensureAIApplicationCanMutate(app model.Application) error {
	if !applicationCanMutate(app) {
		return aitool.Conflict("application.delete_in_progress", "application deletion is in progress")
	}
	return nil
}

func (h *Handler) ensureAIDeploymentTargetCanMutate(tc *aitool.ToolContext, target model.DeploymentTarget) error {
	scratchCtx, rec := newScratchGin(tc)
	if !h.ensureDeploymentTargetCanMutate(scratchCtx, target) {
		return toolErrorFromRecorder(rec, http.StatusConflict, "deployment_target.delete_in_progress", "deployment target cannot be mutated while deletion is in progress")
	}
	return nil
}

func (h *Handler) ensureAIBillingAllowsDeployChange(tc *aitool.ToolContext, projectID string) error {
	scratchCtx, rec := newScratchGin(tc)
	if !h.ensureBillingAllowsDeployChange(scratchCtx, projectID) {
		return toolErrorFromRecorder(rec, http.StatusPaymentRequired, "billing.insufficient_balance", "billing policy prevents deployment changes")
	}
	return nil
}

func checkDeploymentStageAvailable(db *gorm.DB, applicationID, stage, excludeTargetID string) error {
	query := db.Select("id", "delete_status").
		Where("application_id = ? AND stage = ? AND deleted_at IS NULL", applicationID, stage)
	if strings.TrimSpace(excludeTargetID) != "" {
		query = query.Where("id <> ?", excludeTargetID)
	}
	var existing model.DeploymentTarget
	if err := query.First(&existing).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	switch strings.TrimSpace(existing.DeleteStatus) {
	case "deleting":
		return aitool.Conflict("deployment.stage_delete_in_progress", "deployment stage is currently being deleted; wait for resource cleanup to finish")
	case "delete_failed":
		return aitool.Conflict("deployment.stage_delete_failed", "previous deletion of this deployment stage failed; complete cleanup first")
	default:
		return aitool.Conflict("deployment.stage_exists", fmt.Sprintf("deployment stage %q already exists in this application", stage))
	}
}

// resolveRuntimeClusterForProject resolves an explicit cluster ID/name or (when empty)
// resolves and validates the default active cluster for the project so cluster_id is
// persisted authoritatively instead of an empty string.
func (h *Handler) resolveRuntimeClusterForProject(tc *aitool.ToolContext, user model.User, projectID, clusterRef string) (model.RuntimeCluster, error) {
	db := h.aiDB(tc)
	ref := strings.TrimSpace(clusterRef)
	if ref == "" {
		defaultCluster, err := runtimecluster.ResolveForTarget(db, "")
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.RuntimeCluster{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"runtime_cluster.not_configured",
				"no default runtime cluster is configured",
				"Specify an explicit cluster ID or name via `cluster`, or inspect available clusters with `runtime_list_clusters`.",
			)
		}
		if err != nil {
			return model.RuntimeCluster{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		scratchCtx, rec := newScratchGin(tc)
		validated, ok := h.runtimeClusterForProjectUse(scratchCtx, user, projectID, defaultCluster.ID)
		if !ok {
			return model.RuntimeCluster{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "runtime_cluster.unavailable", "default runtime cluster is unavailable for this project")
		}
		return validated, nil
	}

	var candidates []model.RuntimeCluster
	visQuery := h.applyScopedResourceVisibilityForProject(
		runtimecluster.ActiveScope(db.Model(&model.RuntimeCluster{})),
		scopedResourceRuntimeCluster,
		user,
		projectID,
		tc.Context(),
	)
	if err := visQuery.Order("created_at asc").Find(&candidates).Error; err != nil {
		return model.RuntimeCluster{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	matched, err := aitool.ResolveUniqueByIDOrName(
		candidates,
		"runtime_cluster",
		"runtime_cluster.not_found",
		ref,
		func(c model.RuntimeCluster) string { return c.ID },
		func(c model.RuntimeCluster) string { return c.Name },
	)
	if err != nil {
		return model.RuntimeCluster{}, err
	}
	if matched.Scope == "project" && !scopedResourceBoundToProject(db, scopedResourceRuntimeCluster, matched.ID, projectID) {
		return model.RuntimeCluster{}, aitool.Forbidden("runtime_cluster.project_not_bound", "runtime cluster is not authorized for this project")
	}
	scratchCtx, rec := newScratchGin(tc)
	validated, ok := h.runtimeClusterForProjectUse(scratchCtx, user, projectID, matched.ID)
	if !ok {
		return model.RuntimeCluster{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "runtime_cluster.unavailable", "runtime cluster cannot be used by this project")
	}
	return validated, nil
}

func scopedResourceBoundToProject(db *gorm.DB, resourceType, resourceID, projectID string) bool {
	var count int64
	if err := db.Table("scoped_resource_project_bindings").
		Where("resource_type = ? AND resource_id = ? AND project_id = ?", resourceType, resourceID, projectID).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func resolveRepositoryBindingForApp(db *gorm.DB, projectID, appID, ref string) (model.RepositoryBinding, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.RepositoryBinding{}, aitool.BadRequest("deployment_target.repository_binding_required", "repositoryBinding is required when sourceType=repository")
	}
	var bindings []model.RepositoryBinding
	if err := db.Where("project_id = ? AND application_id = ? AND deleted_at IS NULL", projectID, appID).
		Order("created_at asc").
		Find(&bindings).Error; err != nil {
		return model.RepositoryBinding{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		bindings,
		"repository_binding",
		"deployment_target.repository_binding_not_found",
		ref,
		func(b model.RepositoryBinding) string { return b.ID },
		func(b model.RepositoryBinding) string { return strings.TrimSpace(b.Owner + "/" + b.Repo) },
	)
}

func (h *Handler) resolveArtifactRegistryForProject(tc *aitool.ToolContext, user model.User, projectID, ref string) (model.ArtifactRegistry, error) {
	db := h.aiDB(tc)
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.ArtifactRegistry{}, aitool.BadRequest("deployment_target.target_registry_required", "targetRegistry is required")
	}
	var registries []model.ArtifactRegistry
	visQuery := h.applyScopedResourceVisibilityForProject(
		db.Model(&model.ArtifactRegistry{}).Where("deleted_at IS NULL"),
		scopedResourceArtifactRegistry,
		user,
		projectID,
		tc.Context(),
	)
	if err := visQuery.Order("created_at asc").Find(&registries).Error; err != nil {
		return model.ArtifactRegistry{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	// Filter strictly by (actor, projectID) scope visibility
	filtered := make([]model.ArtifactRegistry, 0, len(registries))
	for _, r := range registries {
		if canUseScopedResourceForTargetProject(db, user, r.Scope, r.OwnerRef, scopedResourceArtifactRegistry, r.ID, projectID) {
			filtered = append(filtered, r)
		}
	}
	return aitool.ResolveUniqueByIDOrName(
		filtered,
		"artifact_registry",
		"artifact_registry.not_found",
		ref,
		func(r model.ArtifactRegistry) string { return r.ID },
		func(r model.ArtifactRegistry) string { return r.Name },
	)
}

func (h *Handler) resolveDefaultArtifactRegistryForProject(tc *aitool.ToolContext, user model.User, projectID string) (model.ArtifactRegistry, bool) {
	db := h.aiDB(tc)
	var projectRegistry model.ArtifactRegistry
	if err := db.Joins(
		"JOIN scoped_resource_project_bindings srpb ON srpb.resource_id = artifact_registries.id AND srpb.resource_type = ? AND srpb.project_id = ? AND srpb.is_default = ?",
		scopedResourceArtifactRegistry, projectID, true,
	).Where("artifact_registries.scope = ? AND artifact_registries.deleted_at IS NULL", "project").
		First(&projectRegistry).Error; err == nil {
		return projectRegistry, true
	}
	for _, candidate := range []struct {
		scope string
		owner string
	}{
		{scope: "user", owner: user.ID},
		{scope: "global", owner: ""},
	} {
		var reg model.ArtifactRegistry
		if err := db.First(&reg, "scope = ? AND owner_ref = ? AND is_default = ? AND deleted_at IS NULL", candidate.scope, candidate.owner, true).Error; err == nil {
			return reg, true
		}
	}
	return model.ArtifactRegistry{}, false
}

func canUseScopedResourceForTargetProject(db *gorm.DB, user model.User, scope, ownerRef, resourceType, resourceID, projectID string) bool {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "global":
		return true
	case "user":
		return strings.TrimSpace(ownerRef) != "" && strings.TrimSpace(ownerRef) == strings.TrimSpace(user.ID)
	case "project":
		return scopedResourceBoundToProject(db, resourceType, resourceID, projectID)
	default:
		return false
	}
}

func (h *Handler) resolveBuildVariableSetForProject(tc *aitool.ToolContext, user model.User, projectID, ref string) (model.BuildVariableSet, error) {
	db := h.aiDB(tc)
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.BuildVariableSet{}, aitool.BadRequest("build_variable_set.required", "variable set identifier is required")
	}
	var sets []model.BuildVariableSet
	visQuery := h.applyScopedResourceVisibilityForProject(
		db.Model(&model.BuildVariableSet{}).Where("deleted_at IS NULL"),
		scopedResourceBuildVariableSet,
		user,
		projectID,
		tc.Context(),
	)
	if err := visQuery.Order("created_at asc").Find(&sets).Error; err != nil {
		return model.BuildVariableSet{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	filtered := make([]model.BuildVariableSet, 0, len(sets))
	for _, s := range sets {
		if canUseScopedResourceForTargetProject(db, user, s.Scope, s.OwnerRef, scopedResourceBuildVariableSet, s.ID, projectID) {
			filtered = append(filtered, s)
		}
	}
	return aitool.ResolveUniqueByIDOrName(
		filtered,
		"build_variable_set",
		"build_variable_set.not_found",
		ref,
		func(s model.BuildVariableSet) string { return s.ID },
		func(s model.BuildVariableSet) string { return s.Name },
	)
}

func resolveProjectConfigSet(db *gorm.DB, projectID, ref string) (model.ProjectRuntimeConfigSet, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.ProjectRuntimeConfigSet{}, aitool.BadRequest("runtime_config.required", "configSet is required")
	}
	var sets []model.ProjectRuntimeConfigSet
	if err := db.Where("project_id = ? AND deleted_at IS NULL", projectID).Order("created_at asc").Find(&sets).Error; err != nil {
		return model.ProjectRuntimeConfigSet{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	matched, err := aitool.ResolveUniqueByIDOrName(
		sets,
		"runtime_config_set",
		"runtime_config.not_found",
		ref,
		func(s model.ProjectRuntimeConfigSet) string { return s.ID },
		func(s model.ProjectRuntimeConfigSet) string { return s.Name },
	)
	if err != nil {
		return model.ProjectRuntimeConfigSet{}, err
	}
	if !model.DeleteStatusActive(matched.DeleteStatus) {
		return model.ProjectRuntimeConfigSet{}, aitool.Conflict("runtime_config.delete_in_progress", "runtime config set is being deleted")
	}
	return matched, nil
}

func resolveProjectVolume(db *gorm.DB, projectID, ref string) (model.ProjectVolume, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.ProjectVolume{}, aitool.BadRequest("volume.required", "projectVolume is required")
	}
	var volumes []model.ProjectVolume
	if err := db.Where("project_id = ? AND deleted_at IS NULL", projectID).Order("created_at asc").Find(&volumes).Error; err != nil {
		return model.ProjectVolume{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		volumes,
		"project_volume",
		"volume.not_found",
		ref,
		func(v model.ProjectVolume) string { return v.ID },
		func(v model.ProjectVolume) string { return v.DisplayName },
	)
}

func resolveProjectHookConfig(db *gorm.DB, projectID, ref string) (model.ProjectHookConfig, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.ProjectHookConfig{}, aitool.BadRequest("hook.required", "hook is required")
	}
	var hooks []model.ProjectHookConfig
	if err := db.Where("project_id = ? AND deleted_at IS NULL", projectID).Order("created_at asc").Find(&hooks).Error; err != nil {
		return model.ProjectHookConfig{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		hooks,
		"project_hook",
		"hook.not_found",
		ref,
		func(hc model.ProjectHookConfig) string { return hc.ID },
		func(hc model.ProjectHookConfig) string { return hc.Name },
	)
}

// validateRelativeRepoPath rejects absolute paths, backslash traversals, and `..` parent escapes.
func validateRelativeRepoPath(raw, fieldName string, allowDot bool) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", aitool.BadRequest("argument_invalid", fieldName+" cannot be empty")
	}
	if strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, `\`) || strings.ContainsRune(trimmed, 0) {
		return "", aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"argument_invalid",
			fmt.Sprintf("%s must be a clean relative path inside the repository (cannot start with `/` or contain `\\`)", fieldName),
			"Provide a repository-relative path such as `Dockerfile`, `.`, or `services/api`.",
		)
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == ".." {
			return "", aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"argument_invalid",
				fmt.Sprintf("%s must not contain `..` parent directory traversal", fieldName),
				"Keep all build paths strictly within the repository root.",
			)
		}
	}
	cleaned := path.Clean(trimmed)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("%s escapes repository root", fieldName))
	}
	if cleaned == "." && !allowDot {
		return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("%s must point to a file path rather than `.`", fieldName))
	}
	return cleaned, nil
}

// truncateUTF8Tail truncates s to at most maxBytes from the end without splitting multi-byte UTF-8 runes.
func truncateUTF8Tail(s string, maxBytes int) (content string, truncated bool, truncatedBytes int) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, false, 0
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	for start < len(s) && !utf8.ValidString(s[start:]) {
		start++
	}
	return s[start:], true, start
}

// truncateUTF8Head truncates s to at most maxBytes from the beginning without splitting multi-byte UTF-8 runes.
func truncateUTF8Head(s string, maxBytes int) (content string, truncated bool, truncatedBytes int) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, false, 0
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	for end > 0 && !utf8.ValidString(s[:end]) {
		end--
	}
	return s[:end], true, len(s) - end
}

func decodeStringListField(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	if strings.HasPrefix(raw, "[") {
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err == nil {
			out := make([]string, 0, len(values))
			for _, v := range values {
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					out = append(out, trimmed)
				}
			}
			return out
		}
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func decodeStringMapField(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	out := map[string]string{}
	if raw == "" {
		return out
	}
	if strings.HasPrefix(raw, "{") {
		_ = json.Unmarshal([]byte(raw), &out)
		return out
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			k = strings.TrimSpace(k)
			if k != "" {
				out[k] = strings.TrimSpace(v)
			}
		}
	}
	return out
}

func decodeConfigFilesList(raw string) []runtimeConfigFileInput {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[]" {
		return []runtimeConfigFileInput{}
	}
	var files []runtimeConfigFileInput
	if err := json.Unmarshal([]byte(trimmed), &files); err != nil {
		return []runtimeConfigFileInput{}
	}
	out := make([]runtimeConfigFileInput, 0, len(files))
	for _, f := range files {
		p := strings.TrimSpace(f.Path)
		if p != "" && strings.HasPrefix(p, "/") {
			out = append(out, runtimeConfigFileInput{Path: path.Clean(p), Content: f.Content})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func encodeConfigFilesList(files []runtimeConfigFileInput) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	sorted := make([]runtimeConfigFileInput, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	data, err := json.Marshal(sorted)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func sortedSecretFilePaths(raw string) []string {
	refs := decodeSecretRefs(raw)
	paths := make([]string, 0, len(refs))
	for p, ref := range refs {
		p = strings.TrimSpace(p)
		if p != "" && strings.HasPrefix(p, "/") && strings.TrimSpace(ref) != "" {
			paths = append(paths, path.Clean(p))
		}
	}
	sort.Strings(paths)
	return paths
}

// extractAllFilePaths parses both JSON array `[{path, content}]` and JSON map `{"/path": "ref"}` formats.
func extractAllFilePaths(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[]" || trimmed == "{}" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		items := decodeConfigFilesList(trimmed)
		out := make([]string, 0, len(items))
		for _, item := range items {
			out = append(out, item.Path)
		}
		return out
	}
	if strings.HasPrefix(trimmed, "{") {
		return sortedSecretFilePaths(trimmed)
	}
	return nil
}

func pathsOverlapOrNest(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a = path.Clean(a)
	b = path.Clean(b)
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func pathsNestStrictly(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a = path.Clean(a)
	b = path.Clean(b)
	return a != b && (strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/"))
}

// outboundServiceBindingEnvKeys returns ALL env keys injected into sourceTargetID by active outbound ServiceBindings,
// including URLEnvVar, HostEnvVar, PortEnvVar, AND SecretMap[].SourceEnvVar.
func outboundServiceBindingEnvKeys(db *gorm.DB, projectID, sourceTargetID string) (map[string]string, error) {
	var bindings []model.ServiceBinding
	if err := db.Where("project_id = ? AND source_deployment_target_id = ? AND enabled = ?", projectID, sourceTargetID, true).
		Find(&bindings).Error; err != nil {
		return nil, err
	}
	injected := make(map[string]string)
	for _, b := range bindings {
		for _, k := range []string{b.URLEnvVar, b.HostEnvVar, b.PortEnvVar} {
			k = strings.TrimSpace(k)
			if k != "" {
				injected[k] = b.ID
			}
		}
		for _, sm := range model.DecodeSecretMap(b.SecretMap) {
			k := strings.TrimSpace(sm.SourceEnvVar)
			if k != "" {
				injected[k] = b.ID
			}
		}
	}
	return injected, nil
}

// inboundServiceBindingSecretUsages returns active inbound ServiceBindings referencing targetSecretKey on targetID.
func inboundServiceBindingSecretUsages(db *gorm.DB, projectID, targetID string) (map[string][]string, error) {
	var bindings []model.ServiceBinding
	if err := db.Where("project_id = ? AND target_deployment_target_id = ? AND enabled = ?", projectID, targetID, true).
		Find(&bindings).Error; err != nil {
		return nil, err
	}
	used := make(map[string][]string)
	for _, b := range bindings {
		for _, sm := range model.DecodeSecretMap(b.SecretMap) {
			k := strings.TrimSpace(sm.TargetSecretKey)
			if k != "" {
				used[k] = append(used[k], b.ID)
			}
		}
	}
	return used, nil
}

// validateTargetEffectiveFilesAndEnv validates layered configFiles, secretFiles, enabled bound ConfigSets,
// dataVolume mountPaths, and active outbound ServiceBinding injected env keys.
func validateTargetEffectiveFilesAndEnv(
	db *gorm.DB,
	projectID string,
	targetID string,
	publicEnv map[string]string,
	secretKeys map[string]struct{},
	targetConfigFilePaths []string,
	targetSecretFilePaths []string,
	configRefs []model.DeploymentRuntimeConfigRef,
	volumeMountPaths []string,
) error {
	injectedByBinding, err := outboundServiceBindingEnvKeys(db, projectID, targetID)
	if err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	checkEnvAgainstBindings := func(key string) error {
		if bindingID, conflict := injectedByBinding[strings.TrimSpace(key)]; conflict {
			return aitool.NewErrorWithHint(
				http.StatusConflict,
				"service_binding.env_conflict",
				fmt.Sprintf("environment key %q conflicts with active outbound service binding %s", key, bindingID),
				"Choose a different variable name or update/delete the conflicting service binding via `network_save_service_binding` / `network_delete_service_binding`.",
			)
		}
		return nil
	}

	for k := range publicEnv {
		if err := checkEnvAgainstBindings(k); err != nil {
			return err
		}
	}
	for k := range secretKeys {
		if err := checkEnvAgainstBindings(k); err != nil {
			return err
		}
	}

	// Collect effective configFile paths and secretFile paths across target and enabled ConfigSets.
	configPathSet := make(map[string]struct{}, len(targetConfigFilePaths))
	secretPathSet := make(map[string]struct{}, len(targetSecretFilePaths))
	for _, p := range targetConfigFilePaths {
		configPathSet[path.Clean(p)] = struct{}{}
	}
	for _, p := range targetSecretFilePaths {
		secretPathSet[path.Clean(p)] = struct{}{}
	}

	liveSetIDs := model.DeploymentRuntimeConfigLiveSetIDs(configRefs)
	liveSetsByID := map[string]model.ProjectRuntimeConfigSet{}
	if len(liveSetIDs) > 0 {
		var sets []model.ProjectRuntimeConfigSet
		if err := db.Where("project_id = ? AND id IN ? AND deleted_at IS NULL", projectID, liveSetIDs).Find(&sets).Error; err != nil {
			return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		for _, s := range sets {
			liveSetsByID[s.ID] = s
		}
	}

	for _, ref := range configRefs {
		var (
			enabled        bool
			envRaw         string
			secretRefsRaw  string
			configFilesRaw string
			secretFilesRaw string
		)
		if model.RuntimeConfigRefMode(ref.Mode) == model.RuntimeConfigRefModeSnapshot {
			if ref.Snapshot != nil {
				enabled = ref.Snapshot.Enabled
				envRaw = ref.Snapshot.EnvVars
				secretRefsRaw = ref.Snapshot.SecretRefs
				configFilesRaw = ref.Snapshot.ConfigFiles
				secretFilesRaw = ref.Snapshot.SecretFiles
			}
		} else if liveSet, ok := liveSetsByID[ref.SetID]; ok && model.DeleteStatusActive(liveSet.DeleteStatus) {
			enabled = liveSet.Enabled
			envRaw = liveSet.EnvVars
			secretRefsRaw = liveSet.SecretRefs
			configFilesRaw = liveSet.ConfigFiles
			secretFilesRaw = liveSet.SecretFiles
		}
		if !enabled {
			continue
		}
		if kv, decErr := runtimeconfig.DecodeKeyValue(envRaw); decErr == nil {
			for k := range kv {
				if err := checkEnvAgainstBindings(k); err != nil {
					return err
				}
			}
		}
		for k, v := range decodeSecretRefs(secretRefsRaw) {
			if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
				if err := checkEnvAgainstBindings(k); err != nil {
					return err
				}
			}
		}
		for _, p := range extractAllFilePaths(configFilesRaw) {
			configPathSet[path.Clean(p)] = struct{}{}
		}
		for _, p := range extractAllFilePaths(secretFilesRaw) {
			secretPathSet[path.Clean(p)] = struct{}{}
		}
	}

	// 1. Cross-kind same-path conflict (configFiles <-> secretFiles)
	for p := range configPathSet {
		if _, exists := secretPathSet[p]; exists {
			return aitool.Conflict("deployment.runtime_path_conflict", fmt.Sprintf("mount path %q cannot be used by both plaintext configFiles and encrypted secretFiles", p))
		}
	}

	// 2. Parent-child path prefix conflict among all mounted files
	allFilePaths := make([]string, 0, len(configPathSet)+len(secretPathSet))
	for p := range configPathSet {
		allFilePaths = append(allFilePaths, p)
	}
	for p := range secretPathSet {
		allFilePaths = append(allFilePaths, p)
	}
	sort.Strings(allFilePaths)
	for i := 0; i < len(allFilePaths); i++ {
		for j := i + 1; j < len(allFilePaths); j++ {
			if pathsNestStrictly(allFilePaths[i], allFilePaths[j]) {
				return aitool.Conflict("deployment.runtime_path_conflict", fmt.Sprintf("mounted file paths %q and %q have a parent-child prefix conflict", allFilePaths[i], allFilePaths[j]))
			}
		}
	}

	// 3. Same-path or parent-child conflict against dataVolumes.mountPath
	for _, mountPath := range volumeMountPaths {
		mountPath = strings.TrimSpace(mountPath)
		if mountPath == "" {
			continue
		}
		for _, fp := range allFilePaths {
			if pathsOverlapOrNest(mountPath, fp) {
				return aitool.Conflict("deployment.runtime_path_conflict", fmt.Sprintf("volume mountPath %q conflicts with mounted file %q", mountPath, fp))
			}
		}
	}

	return nil
}

func boolVal(ptr *bool, def bool) bool {
	if ptr == nil {
		return def
	}
	return *ptr
}

func intVal(ptr *int, def int) int {
	if ptr == nil {
		return def
	}
	return *ptr
}

func contextBackgroundFallback(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
