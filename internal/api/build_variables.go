package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func normalizeBuildVariables(ctx *gin.Context, input map[string]string) (map[string]string, bool) {
	output := make(map[string]string)
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" && value == "" {
			continue
		}
		if !isBuildEnvKey(key) {
			writeError(ctx, http.StatusBadRequest, "构建变量名只能使用字母、数字和下划线，且不能以数字开头")
			return nil, false
		}
		if len(value) > 4096 {
			writeError(ctx, http.StatusBadRequest, "构建变量值过长")
			return nil, false
		}
		output[key] = value
	}
	return output, true
}

func normalizeBuildArgsInput(ctx *gin.Context, raw string) (string, bool) {
	values, err := parseBuildArgsInput(raw)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return "", false
	}
	return model.EncodeBuildArgs(values), true
}

func normalizeBuildArgsInputValue(raw string) string {
	values, err := parseBuildArgsInput(raw)
	if err != nil {
		return model.EncodeBuildArgs(model.BuildArgs(raw))
	}
	return model.EncodeBuildArgs(values)
}

func parseBuildArgsInput(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	if len(raw) > 64*1024 {
		return nil, errors.New("Dockerfile Build Args 过长")
	}
	if strings.HasPrefix(raw, "{") {
		values := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, errors.New("Dockerfile Build Args JSON 格式不正确")
		}
		return validateBuildArgs(values)
	}
	values := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			key, value, ok = strings.Cut(line, ":")
		}
		if !ok {
			return nil, errors.New("Dockerfile Build Args 需要按 KEY=value 每行填写一个")
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return validateBuildArgs(values)
}

func validateBuildArgs(values map[string]string) (map[string]string, error) {
	output := make(map[string]string)
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" && value == "" {
			continue
		}
		if !model.IsBuildArgKey(key) {
			return nil, errors.New("Dockerfile Build Args 名称只能使用字母、数字和下划线，且不能以数字开头")
		}
		if len(value) > 4096 {
			return nil, errors.New("Dockerfile Build Args 单项值过长")
		}
		output[key] = value
	}
	return output, nil
}

func isBuildEnvKey(value string) bool {
	return buildenv.IsKey(value)
}

func encodeBuildVariableSetIDs(ids []string) string {
	normalized := normalizeStringList(ids)
	if len(normalized) == 0 {
		return ""
	}
	content, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(content)
}

func buildVariableSetIDs(raw string) []string {
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

func removeBuildVariableSetID(raw string, setID string) string {
	setID = strings.TrimSpace(setID)
	if setID == "" {
		return raw
	}
	next := make([]string, 0)
	for _, id := range buildVariableSetIDs(raw) {
		if id != setID {
			next = append(next, id)
		}
	}
	return encodeBuildVariableSetIDs(next)
}

func normalizeBuildSelectorList(values []string) []string {
	seen := map[string]bool{}
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		output = append(output, value)
	}
	return output
}

func builderHasLabels(rawLabels string, requiredLabels []string) bool {
	if len(requiredLabels) == 0 {
		return true
	}
	labels := map[string]bool{}
	for _, label := range normalizeBuildSelectorList(strings.Split(rawLabels, ",")) {
		labels[label] = true
	}
	for _, required := range requiredLabels {
		if !labels[required] {
			return false
		}
	}
	return true
}

func builderAllowsRun(rawScopes string, projectID string, userID string) bool {
	scopes := normalizeBuildSelectorList(strings.Split(rawScopes, ","))
	if len(scopes) == 0 {
		return true
	}
	for _, scope := range scopes {
		switch {
		case scope == "global":
			return true
		case strings.HasPrefix(scope, "project:") && strings.TrimPrefix(scope, "project:") == strings.ToLower(strings.TrimSpace(projectID)):
			return true
		case strings.HasPrefix(scope, "user:") && strings.TrimPrefix(scope, "user:") == strings.ToLower(strings.TrimSpace(userID)):
			return true
		}
	}
	return false
}

func builderVisibleToUser(rawScopes string, userID string, projectIDs []string) bool {
	scopes := normalizeBuildSelectorList(strings.Split(rawScopes, ","))
	if len(scopes) == 0 {
		return true
	}
	userID = strings.ToLower(strings.TrimSpace(userID))
	projectSet := map[string]bool{}
	for _, projectID := range projectIDs {
		projectSet[strings.ToLower(strings.TrimSpace(projectID))] = true
	}
	for _, scope := range scopes {
		switch {
		case scope == "global":
			return true
		case strings.HasPrefix(scope, "user:") && strings.TrimPrefix(scope, "user:") == userID:
			return true
		case strings.HasPrefix(scope, "project:") && projectSet[strings.TrimPrefix(scope, "project:")]:
			return true
		}
	}
	return false
}

func (h *Handlers) buildVariablesForRun(ctx *gin.Context, user model.User, projectID string, setIDs []string) (map[string]string, bool) {
	variables, err := h.buildVariablesForRunByIDs(h.db, user, projectID, setIDs)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return variables, true
}

func (h *Handlers) buildVariablesForRunByIDs(db *gorm.DB, user model.User, projectID string, setIDs []string) (map[string]string, error) {
	output := make(map[string]string)
	sets, err := h.buildVariableSetsForRun(db, user, projectID, setIDs)
	if err != nil {
		return nil, err
	}
	for _, set := range sets {
		applyBuildVariableSetValues(output, set, h.secrets.Resolve)
	}
	return output, nil
}

func (h *Handlers) buildEnvironmentSnapshotForRun(db *gorm.DB, user model.User, run model.BuildRun) (buildenv.Snapshot, error) {
	snapshot := buildenv.NewSnapshot()
	if config, err := h.findBuildEnvironmentConfig(db, model.BuildEnvironmentScopeGlobal, model.BuildEnvironmentGlobalRef); err == nil {
		buildenv.Apply(&snapshot, config.Variables, config.SecretRefs)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return snapshot, err
	}
	sets, err := h.buildVariableSetsForRun(db, user, run.ProjectID, buildVariableSetIDs(run.BuildVariableSetIDs))
	if err != nil {
		return snapshot, err
	}
	for _, set := range sets {
		buildenv.Apply(&snapshot, set.Variables, set.SecretRefs)
	}
	if config, err := h.findBuildEnvironmentConfig(db, model.BuildEnvironmentScopeApplication, run.ApplicationID); err == nil {
		buildenv.Apply(&snapshot, config.Variables, config.SecretRefs)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return snapshot, err
	}
	if config, err := h.findBuildEnvironmentConfig(db, model.BuildEnvironmentScopeDeployment, run.DeploymentTargetID); err == nil {
		buildenv.Apply(&snapshot, config.Variables, config.SecretRefs)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return snapshot, err
	}
	return snapshot, nil
}

func (h *Handlers) buildVariableSetsForRun(db *gorm.DB, user model.User, projectID string, setIDs []string) ([]model.BuildVariableSet, error) {
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
		if !h.buildVariableSetAccessible(user, projectID, set) {
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
			return nil, errors.New("变量和密钥不可用")
		}
		if !h.buildVariableSetAccessible(user, projectID, set) {
			return nil, errors.New("无权使用该变量和密钥")
		}
		sets = append(sets, set)
	}
	return sets, nil
}

func applyBuildVariableSetValues(output map[string]string, set model.BuildVariableSet, resolveSecret func(string) string) {
	var values map[string]string
	if err := json.Unmarshal([]byte(fallback(set.Variables, "{}")), &values); err == nil {
		for key, value := range values {
			if isBuildEnvKey(key) {
				output[key] = value
			}
		}
	}
	for key, ref := range decodeSecretRefs(set.SecretRefs) {
		if !isBuildEnvKey(key) {
			continue
		}
		if secretValue := resolveSecret(ref); secretValue != "" {
			output[key] = secretValue
		}
	}
}

func decodeSecretRefs(raw string) map[string]string {
	refs := map[string]string{}
	if err := json.Unmarshal([]byte(fallback(raw, "{}")), &refs); err != nil {
		return map[string]string{}
	}
	return refs
}

func (h *Handlers) buildVariableSetAccessible(user model.User, projectID string, set model.BuildVariableSet) bool {
	switch set.Scope {
	case "global":
		return true
	case "user":
		return set.OwnerRef == user.ID
	case "project":
		if user.Role == "platform_admin" {
			return true
		}
		for _, boundProjectID := range h.scopedResourceProjectIDs(scopedResourceBuildVariableSet, set.ID) {
			if boundProjectID == projectID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

type buildVariableSetInput struct {
	Name       string            `json:"name" binding:"required"`
	Scope      string            `json:"scope"`
	OwnerRef   string            `json:"ownerRef"`
	ProjectIDs []string          `json:"projectIds"`
	Variables  map[string]string `json:"variables"`
	Secrets    map[string]string `json:"secrets"`
	Enabled    bool              `json:"enabled"`
}

type buildVariableSetResponse struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Scope               string          `json:"scope"`
	OwnerRef            string          `json:"ownerRef"`
	ProjectIDs          []string        `json:"projectIds"`
	Variables           string          `json:"variables"`
	VariableCount       int             `json:"variableCount"`
	CanInspectVariables bool            `json:"canInspectVariables"`
	Secrets             map[string]bool `json:"secrets"`
	Enabled             bool            `json:"enabled"`
	CreatedBy           string          `json:"createdBy"`
	CreatedAt           time.Time       `json:"createdAt"`
}

func (h *Handlers) buildVariableSetResponsesForUser(user model.User, sets []model.BuildVariableSet) []buildVariableSetResponse {
	output := make([]buildVariableSetResponse, 0, len(sets))
	for _, set := range sets {
		output = append(output, h.buildVariableSetResponseForUser(user, set))
	}
	return output
}

func (h *Handlers) buildVariableSetResponseForUser(user model.User, set model.BuildVariableSet) buildVariableSetResponse {
	secrets := map[string]bool{}
	for key, ref := range decodeSecretRefs(set.SecretRefs) {
		if isBuildEnvKey(key) && strings.TrimSpace(ref) != "" {
			secrets[key] = true
		}
	}
	canInspectVariables := h.canInspectScopedResourceConfigByID(user, set.Scope, set.OwnerRef, scopedResourceBuildVariableSet, set.ID)
	variables := "{}"
	if canInspectVariables {
		variables = set.Variables
	}
	return buildVariableSetResponse{
		ID:                  set.ID,
		Name:                set.Name,
		Scope:               set.Scope,
		OwnerRef:            set.OwnerRef,
		ProjectIDs:          jsonList(set.ProjectIDs),
		Variables:           variables,
		VariableCount:       buildVariableSetVariableCount(set.Variables),
		CanInspectVariables: canInspectVariables,
		Secrets:             secrets,
		Enabled:             set.Enabled,
		CreatedBy:           set.CreatedBy,
		CreatedAt:           set.CreatedAt,
	}
}

func buildVariableSetVariableCount(raw string) int {
	values := map[string]string{}
	if err := json.Unmarshal([]byte(fallback(raw, "{}")), &values); err != nil {
		return 0
	}
	count := 0
	for key := range values {
		if isBuildEnvKey(key) {
			count++
		}
	}
	return count
}
