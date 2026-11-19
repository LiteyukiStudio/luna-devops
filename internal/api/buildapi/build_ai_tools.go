package buildapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"gorm.io/gorm"
)

// RegisterAITools registers all 14 Build domain AI facade tools (12 build_* + 2 admin_*).
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	h.registerBuildReadAITools(reg)
	h.registerBuildWriteAITools(reg)
	h.registerBuildAdminAITools(reg)
}

type buildListTemplatesInput struct {
	Template      *string           `json:"template,omitempty" desc:"可选，传入构建模板 ID 查看参数 Schema；配合 previewValues 渲染预览 Dockerfile"`
	Version       *string           `json:"version,omitempty" desc:"可选，模板版本"`
	PreviewValues map[string]string `json:"previewValues,omitempty" desc:"可选，预览渲染模板参数键值对"`
}

type buildListVariableSetsInput struct {
	Project     *string `json:"project,omitempty" desc:"可选，按项目 ID 或 identifier 筛选可见变量集"`
	VariableSet *string `json:"variableSet,omitempty" desc:"可选，按构建变量集 ID（bvs_xxx）或精确名称过滤"`
	Search      *string `json:"search,omitempty" desc:"可选，按名称模糊搜索"`
	SortBy      *string `json:"sortBy,omitempty" enum:"createdAt|name|scope" default:"createdAt" desc:"排序字段"`
	SortOrder   *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page        *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码，默认 1"`
	PageSize    *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数，默认 20，最大 50"`
}

type buildGetScopedEnvInput struct {
	Scope       string  `json:"scope" enum:"application|deployment" desc:"必填，作用域：application（应用级）或 deployment（部署目标级）"`
	Project     string  `json:"project" desc:"必填，项目 ID 或 identifier"`
	Application string  `json:"application" desc:"必填，应用 ID、identifier 或唯一名称"`
	Target      *string `json:"target,omitempty" desc:"scope=deployment 时必填；支持 targetId、stage 或唯一名称"`
}

type buildListRunsInput struct {
	Project      string  `json:"project" desc:"必填，项目 ID 或 identifier"`
	Application  *string `json:"application,omitempty" desc:"可选，应用 ID、identifier 或唯一名称"`
	Target       *string `json:"target,omitempty" desc:"可选，传入 application 时支持 targetId/stage/name；未传 application 时支持 targetId 或公开 stage"`
	Status       *string `json:"status,omitempty" enum:"queued|running|succeeded|failed|canceled|timeout" desc:"可选，按构建状态过滤"`
	TriggerType  *string `json:"triggerType,omitempty" enum:"manual|webhook|push|tag|api|retry" desc:"可选，按触发来源过滤"`
	SourceBranch *string `json:"sourceBranch,omitempty" desc:"可选，按构建分支精确过滤"`
	Search       *string `json:"search,omitempty" desc:"可选，按分支、Commit、镜像 Tag 搜索"`
	SortBy       *string `json:"sortBy,omitempty" enum:"createdAt|status|sourceCommit|sourceBranch|triggerType" default:"createdAt" desc:"排序字段"`
	SortOrder    *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page         *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码，默认 1"`
	PageSize     *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数，默认 20，最大 50"`
}

type buildGetRunInput struct {
	Project      string `json:"project" desc:"必填，项目 ID 或 identifier"`
	BuildRunID   string `json:"buildRunId" desc:"必填，构建记录 ID（bldr_xxx）"`
	IncludeLogs  *bool  `json:"includeLogs,omitempty" default:"true" desc:"是否自动聚合关联 BuildJob 的构建日志尾部及 HookRun 状态摘要，默认 true"`
	LogTailBytes *int   `json:"logTailBytes,omitempty" min:"256" max:"32768" default:"8192" desc:"构建日志尾部最大字节数（UTF-8 字符边界截断），默认 8192，上限 32768"`
}

type adminGetGlobalBuildEnvInput struct{}

type aiBuildVariableSetOutput struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Scope               string            `json:"scope"`
	OwnerRef            string            `json:"ownerRef"`
	ProjectIDs          []string          `json:"projectIds"`
	Variables           map[string]string `json:"variables"`
	VariableCount       int               `json:"variableCount"`
	CanInspectVariables bool              `json:"canInspectVariables"`
	SecretKeys          []string          `json:"secretKeys"`
	Secrets             map[string]bool   `json:"secrets"`
	Enabled             bool              `json:"enabled"`
	CreatedBy           string            `json:"createdBy"`
	CreatedAt           time.Time         `json:"createdAt"`
}

// aiHookRunSummary omits HookRun.ScriptSnapshot (up to 64KB) per catalog spec line 490.
type aiHookRunSummary struct {
	ID                 string     `json:"id"`
	HookConfigID       string     `json:"hookConfigId"`
	BuildRunID         string     `json:"buildRunId"`
	BuildJobID         string     `json:"buildJobId"`
	ApplicationID      string     `json:"applicationId"`
	DeploymentTargetID string     `json:"deploymentTargetId"`
	Name               string     `json:"name"`
	Phase              string     `json:"phase"`
	Status             string     `json:"status"`
	Shell              string     `json:"shell"`
	ImageRef           string     `json:"imageRef"`
	TimeoutSeconds     int        `json:"timeoutSeconds"`
	FailurePolicy      string     `json:"failurePolicy"`
	ExitCode           int        `json:"exitCode"`
	Message            string     `json:"message"`
	StartedAt          *time.Time `json:"startedAt"`
	FinishedAt         *time.Time `json:"finishedAt"`
	CreatedAt          time.Time  `json:"createdAt"`
}

func (h *Handler) registerBuildReadAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[buildListTemplatesInput, map[string]any]{
		Name:           "build_list_templates",
		Domain:         aitool.DomainBuild,
		Title:          "查看内置构建模板与预览 Dockerfile",
		Summary:        "列出内置构建模板（语言/框架模板）并支持纯内存渲染预览 Dockerfile",
		Tags:           []string{"build", "template", "dockerfile", "buildpack"},
		AliasesZH:      []string{"构建模板", "预览Dockerfile"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionBuildRead)},
		Execute:        h.aiListBuildTemplates,
	})

	aitool.Register(reg, aitool.Spec[buildListVariableSetsInput, map[string]any]{
		Name:           "build_list_variable_sets",
		Domain:         aitool.DomainBuild,
		Title:          "查看构建变量与密钥集",
		Summary:        "分页列出可见的构建变量与密钥集（普通变量键值按权限脱敏 + 已配置加密密钥键名数组 secretKeys）",
		Tags:           []string{"build", "variable", "secret", "env"},
		AliasesZH:      []string{"构建变量集", "构建密钥集"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionBuildRead)},
		Execute:        h.aiListBuildVariableSets,
	})

	aitool.Register(reg, aitool.Spec[buildGetScopedEnvInput, map[string]any]{
		Name:           "build_get_scoped_env",
		Domain:         aitool.DomainBuild,
		Title:          "读取应用或部署环境专属构建变量",
		Summary:        "读取应用级或部署目标级的专属构建期环境变量与已配置加密密钥键名数组 secretKeys（仅构建阶段注入，运行期环境变量请调 delivery_get_target）",
		Tags:           []string{"build", "environment", "variable", "secret"},
		AliasesZH:      []string{"查看构建环境变量", "应用构建变量"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionBuildRead), string(authz.ActionSecretViewValue)},
		Execute:        h.aiGetScopedBuildEnv,
	})

	aitool.Register(reg, aitool.Spec[buildListRunsInput, map[string]any]{
		Name:           "build_list_runs",
		Domain:         aitool.DomainBuild,
		Title:          "查询构建运行历史记录",
		Summary:        "分页查询项目下的镜像构建运行历史记录，支持按应用、环境、状态、触发类型与分支过滤",
		Tags:           []string{"build", "run", "history", "pipeline"},
		AliasesZH:      []string{"构建历史", "构建记录列表"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionBuildRead)},
		Execute:        h.aiListBuildRuns,
	})

	aitool.Register(reg, aitool.Spec[buildGetRunInput, map[string]any]{
		Name:           "build_get_run",
		Domain:         aitool.DomainBuild,
		Title:          "查看单次构建详情与构建日志",
		Summary:        "查看单次构建详情、BuildJob 步骤进度、UTF-8 尾部截断的权威构建日志及关联 HookRun 执行状态摘要",
		Tags:           []string{"build", "run", "log", "job", "hook"},
		AliasesZH:      []string{"构建详情", "查看构建日志", "构建排障"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionBuildRead)},
		Execute:        h.aiGetBuildRun,
	})
}

func (h *Handler) syncGinUser(tc *aitool.ToolContext) {
	if tc == nil || tc.Gin == nil || tc.User.ID == "" {
		return
	}
	tc.Gin.Set("luna.devops.current_user", tc.User)
}

func (h *Handler) aiListBuildTemplates(tc *aitool.ToolContext, in buildListTemplatesInput) (map[string]any, error) {
	tplID := buildTrimPtr(in.Template)
	version := buildTrimPtr(in.Version)

	out := map[string]any{}
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		out["projectId"] = strings.TrimSpace(tc.ConversationProjectID)
	}

	if tplID == "" {
		if len(in.PreviewValues) > 0 {
			return nil, aitool.BadRequest("argument_invalid", "template is required when previewValues is provided")
		}
		list := buildtemplate.List()
		out["items"] = list
		out["total"] = len(list)
		return out, nil
	}

	def, ok := buildtemplate.Find(tplID, version)
	if !ok {
		return nil, aitool.NotFound("build_template.not_found", "build template was not found")
	}
	out["template"] = def
	out["items"] = []buildtemplate.Definition{def}
	out["total"] = 1

	if in.PreviewValues != nil {
		preview, err := buildtemplate.Render(def.ID, def.Version, in.PreviewValues)
		if err != nil {
			return nil, aitool.BadRequest("build_template.invalid", err.Error())
		}
		out["preview"] = preview
	}
	return out, nil
}

func (h *Handler) aiListBuildVariableSets(tc *aitool.ToolContext, in buildListVariableSetsInput) (map[string]any, error) {
	h.syncGinUser(tc)
	var effectiveProjectID string
	projRef := buildTrimPtr(in.Project)
	if projRef == "" && strings.TrimSpace(tc.ConversationProjectID) != "" {
		projRef = strings.TrimSpace(tc.ConversationProjectID)
	}
	if projRef != "" {
		_, project, ok := h.authorizeProjectRef(tc.Gin, projRef, authz.ActionBuildRead)
		if !ok {
			return nil, tc.AbortedError("project.forbidden", "project is not accessible")
		}
		effectiveProjectID = project.ID
	}

	visibility := projectservice.ListVisibilityRelated
	if effectiveProjectID == "" && authz.IsPlatformAdmin(tc.User.Role) {
		visibility = projectservice.ListVisibilityAll
	}

	query := tc.DB.Model(&model.BuildVariableSet{})
	var visible bool
	query, visible = h.applyScopedResourceListVisibility(tc.Gin, query, scopedResourceBuildVariableSet, tc.User, effectiveProjectID, visibility)
	if !visible {
		return nil, tc.AbortedError("build.variable_set_forbidden", "cannot list build variable sets")
	}
	if setFilter := buildTrimPtr(in.VariableSet); setFilter != "" {
		query = query.Where("id = ? OR lower(name) = lower(?)", setFilter, setFilter)
	}
	if search := buildTrimPtr(in.Search); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	sortBy := fallback(buildTrimPtr(in.SortBy), "createdAt")
	sortOrder := strings.ToLower(fallback(buildTrimPtr(in.SortOrder), "desc"))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	sortCols := map[string]string{
		"createdAt": "created_at",
		"name":      "name",
		"scope":     "scope",
	}
	sortCol, ok := sortCols[sortBy]
	if !ok {
		sortBy = "createdAt"
		sortCol = "created_at"
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var sets []model.BuildVariableSet
	if err := query.Order(sortCol + " " + sortOrder).Limit(pageSize).Offset((page - 1) * pageSize).Find(&sets).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if setFilter := buildTrimPtr(in.VariableSet); setFilter != "" && len(sets) > 1 {
		for _, s := range sets {
			if s.ID == setFilter {
				sets = []model.BuildVariableSet{s}
				total = 1
				break
			}
		}
	}
	h.attachBuildVariableSetProjects(sets, tc.Context())

	items := make([]aiBuildVariableSetOutput, 0, len(sets))
	for _, set := range sets {
		formatted, fmtErr := h.formatAIBuildVariableSet(tc.User, set, tc.Context())
		if fmtErr != nil {
			return nil, aitool.NewError(http.StatusForbidden, "build.variable_set_forbidden", fmtErr.Error())
		}
		items = append(items, formatted)
	}

	out := aitool.PageEnvelope(items, page, pageSize, sortBy, sortOrder, total)
	if effectiveProjectID != "" {
		out["projectId"] = effectiveProjectID
	}
	return out, nil
}

func (h *Handler) formatAIBuildVariableSet(user model.User, set model.BuildVariableSet, ctx context.Context) (aiBuildVariableSetOutput, error) {
	secretsMap := map[string]bool{}
	var secretKeys []string
	for key, ref := range decodeSecretRefs(set.SecretRefs) {
		if isBuildEnvKey(key) && strings.TrimSpace(ref) != "" {
			secretsMap[key] = true
			secretKeys = append(secretKeys, key)
		}
	}
	sort.Strings(secretKeys)
	if secretKeys == nil {
		secretKeys = []string{}
	}

	canInspect, err := h.canInspectScopedResourceConfigByID(user, set.Scope, set.OwnerRef, scopedResourceBuildVariableSet, set.ID, ctx)
	if err != nil {
		return aiBuildVariableSetOutput{}, err
	}
	vars := map[string]string{}
	if canInspect {
		vars = buildenv.Decode(set.Variables)
	}
	return aiBuildVariableSetOutput{
		ID:                  set.ID,
		Name:                set.Name,
		Scope:               set.Scope,
		OwnerRef:            set.OwnerRef,
		ProjectIDs:          jsonList(set.ProjectIDs),
		Variables:           vars,
		VariableCount:       buildVariableSetVariableCount(set.Variables),
		CanInspectVariables: canInspect,
		SecretKeys:          secretKeys,
		Secrets:             secretsMap,
		Enabled:             set.Enabled,
		CreatedBy:           set.CreatedBy,
		CreatedAt:           set.CreatedAt,
	}, nil
}

func (h *Handler) resolveScopedBuildEnvTarget(tc *aitool.ToolContext, rawScope, rawProject, rawApp string, rawTarget *string) (model.Project, model.Application, *model.DeploymentTarget, string, string, error) {
	h.syncGinUser(tc)
	scope := strings.ToLower(strings.TrimSpace(rawScope))
	if scope != model.BuildEnvironmentScopeApplication && scope != model.BuildEnvironmentScopeDeployment {
		return model.Project{}, model.Application{}, nil, "", "", aitool.BadRequest("argument_invalid", "scope must be application or deployment")
	}
	project, err := aitool.ResolveProject(tc.DB, rawProject)
	if err != nil {
		return model.Project{}, model.Application{}, nil, "", "", err
	}
	allowed, roleErr := h.projectRoleActionAllowed(tc.Context(), tc.User, project.ID, authz.ActionSecretViewValue)
	if roleErr != nil {
		return model.Project{}, model.Application{}, nil, "", "", aitool.NewError(http.StatusForbidden, "project.forbidden", roleErr.Error())
	}
	if !allowed {
		return model.Project{}, model.Application{}, nil, "", "", aitool.Forbidden("secret.view_value_forbidden", "project owner or admin role (secret:view_value) is required to view or manage scoped build environment variables")
	}
	app, err := aitool.ResolveApplication(tc.DB, project.ID, rawApp)
	if err != nil {
		return model.Project{}, model.Application{}, nil, "", "", err
	}
	if scope == model.BuildEnvironmentScopeApplication {
		if buildTrimPtr(rawTarget) != "" {
			return model.Project{}, model.Application{}, nil, "", "", aitool.BadRequest("argument_invalid", "target must be omitted when scope=application")
		}
		return project, app, nil, scope, app.ID, nil
	}
	targetRef := buildTrimPtr(rawTarget)
	if targetRef == "" {
		return model.Project{}, model.Application{}, nil, "", "", aitool.BadRequest("deployment_target.required", "target is required when scope=deployment")
	}
	target, err := aitool.ResolveDeploymentTarget(tc.DB, project.ID, app.ID, targetRef)
	if err != nil {
		return model.Project{}, model.Application{}, nil, "", "", err
	}
	return project, app, &target, scope, target.ID, nil
}

func (h *Handler) aiGetScopedBuildEnv(tc *aitool.ToolContext, in buildGetScopedEnvInput) (map[string]any, error) {
	project, app, target, scope, scopeRef, err := h.resolveScopedBuildEnvTarget(tc, in.Scope, in.Project, in.Application, in.Target)
	if err != nil {
		return nil, err
	}
	config, findErr := h.findBuildEnvironmentConfig(tc.DB, scope, scopeRef)
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", findErr.Error())
	}
	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		config = model.BuildEnvironmentConfig{Scope: scope, ScopeRef: scopeRef, Variables: "{}", SecretRefs: "{}"}
	}
	out := formatBuildEnvConfigOutput(config)
	out["projectId"] = project.ID
	out["applicationId"] = app.ID
	if target != nil {
		out["deploymentTargetId"] = target.ID
		out["stage"] = target.Stage
	}
	return out, nil
}

func (h *Handler) aiGetGlobalBuildEnvAdmin(tc *aitool.ToolContext, _ adminGetGlobalBuildEnvInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("auth.platform_admin_required", "platform administrator role is required")
	}
	config, err := h.findBuildEnvironmentConfig(tc.DB, model.BuildEnvironmentScopeGlobal, model.BuildEnvironmentGlobalRef)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		config = model.BuildEnvironmentConfig{
			Scope:      model.BuildEnvironmentScopeGlobal,
			ScopeRef:   model.BuildEnvironmentGlobalRef,
			Variables:  "{}",
			SecretRefs: "{}",
		}
	}
	return formatBuildEnvConfigOutput(config), nil
}

func formatBuildEnvConfigOutput(config model.BuildEnvironmentConfig) map[string]any {
	secretKeys := make([]string, 0)
	secretsMap := make(map[string]bool)
	for key, ref := range buildenv.Decode(config.SecretRefs) {
		if buildenv.IsKey(key) && strings.TrimSpace(ref) != "" {
			secretKeys = append(secretKeys, key)
			secretsMap[key] = true
		}
	}
	sort.Strings(secretKeys)
	return map[string]any{
		"scope":      config.Scope,
		"scopeRef":   config.ScopeRef,
		"variables":  buildenv.Decode(config.Variables),
		"secretKeys": secretKeys,
		"secrets":    secretsMap,
	}
}

func (h *Handler) aiListBuildRuns(tc *aitool.ToolContext, in buildListRunsInput) (map[string]any, error) {
	h.syncGinUser(tc)
	_, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionBuildRead)
	if !ok {
		return nil, tc.AbortedError("project.forbidden", "project is not accessible")
	}

	query := tc.DB.Model(&model.BuildRun{}).Where("project_id = ?", project.ID)
	var applicationID string
	if appRef := buildTrimPtr(in.Application); appRef != "" {
		app, err := aitool.ResolveApplication(tc.DB, project.ID, appRef)
		if err != nil {
			return nil, err
		}
		applicationID = app.ID
		query = query.Where("application_id = ?", app.ID)
		if targetRef := buildTrimPtr(in.Target); targetRef != "" {
			target, tErr := aitool.ResolveDeploymentTarget(tc.DB, project.ID, app.ID, targetRef)
			if tErr != nil {
				return nil, tErr
			}
			query = query.Where("deployment_target_id = ?", target.ID)
		}
	} else if targetRef := buildTrimPtr(in.Target); targetRef != "" {
		normalizedStage := strings.ToLower(targetRef)
		if normalizedStage == "production" {
			normalizedStage = "prod"
		}
		if slices.Contains([]string{"dev", "test", "staging", "prod"}, normalizedStage) {
			sub := tc.DB.Model(&model.DeploymentTarget{}).
				Select("id").
				Where("project_id = ? AND stage = ? AND deleted_at IS NULL", project.ID, normalizedStage)
			query = query.Where("deployment_target_id IN (?)", sub)
		} else {
			query = query.Where("deployment_target_id = ?", targetRef)
		}
	}

	if st := buildTrimPtr(in.Status); st != "" {
		if !buildRunStatusAllowed(st) {
			return nil, aitool.BadRequest("argument_invalid", "invalid build status filter")
		}
		query = query.Where("status = ?", st)
	}
	if tt := buildTrimPtr(in.TriggerType); tt != "" {
		if !buildRunTriggerAllowed(tt) {
			return nil, aitool.BadRequest("argument_invalid", "invalid triggerType filter")
		}
		query = query.Where("trigger_type = ?", tt)
	}
	if br := buildTrimPtr(in.SourceBranch); br != "" {
		query = query.Where("source_branch = ?", br)
	}
	if search := buildTrimPtr(in.Search); search != "" {
		like := "%" + search + "%"
		query = query.Where("id ILIKE ? OR source_branch ILIKE ? OR source_tag ILIKE ? OR source_commit ILIKE ? OR target_repository ILIKE ? OR target_tag ILIKE ? OR image_ref ILIKE ?",
			like, like, like, like, like, like, like)
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	sortBy := fallback(buildTrimPtr(in.SortBy), "createdAt")
	sortOrder := strings.ToLower(fallback(buildTrimPtr(in.SortOrder), "desc"))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	sortCols := map[string]string{
		"createdAt":    "created_at",
		"status":       "status",
		"sourceCommit": "source_commit",
		"sourceBranch": "source_branch",
		"triggerType":  "trigger_type",
	}
	sortCol, ok := sortCols[sortBy]
	if !ok {
		sortBy = "createdAt"
		sortCol = "created_at"
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var runs []model.BuildRun
	if err := query.Order(sortCol + " " + sortOrder).Limit(pageSize).Offset((page - 1) * pageSize).Find(&runs).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	out := aitool.PageEnvelope(runs, page, pageSize, sortBy, sortOrder, total)
	out["projectId"] = project.ID
	if applicationID != "" {
		out["applicationId"] = applicationID
	}
	return out, nil
}

func (h *Handler) aiGetBuildRun(tc *aitool.ToolContext, in buildGetRunInput) (map[string]any, error) {
	h.syncGinUser(tc)
	_, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionBuildRead)
	if !ok {
		return nil, tc.AbortedError("project.forbidden", "project is not accessible")
	}
	runID := strings.TrimSpace(in.BuildRunID)
	if runID == "" {
		return nil, aitool.BadRequest("build_run.required", "buildRunId is required")
	}

	var run model.BuildRun
	if err := tc.DB.First(&run, "id = ? AND project_id = ?", runID, project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("build_run.not_found", "build run was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if !buildRunTerminal(run.Status) {
		tc.SetNoStore()
	}

	var jobs []model.BuildJob
	if err := tc.DB.Where("build_run_id = ? AND project_id = ?", run.ID, project.ID).Order("created_at asc").Find(&jobs).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if jobs == nil {
		jobs = []model.BuildJob{}
	}

	out := buildRunToMap(run)
	out["projectId"] = project.ID
	out["buildRunId"] = run.ID
	out["jobs"] = jobs

	if buildBoolPtrDefault(in.IncludeLogs, true) {
		tailBytes := 8192
		if in.LogTailBytes != nil && *in.LogTailBytes > 0 {
			tailBytes = *in.LogTailBytes
		}
		if tailBytes < 256 {
			tailBytes = 256
		}
		if tailBytes > 32768 {
			tailBytes = 32768
		}

		var logs []model.BuildLog
		if err := tc.DB.Where("build_run_id = ? AND project_id = ?", run.ID, project.ID).Order("created_at asc").Find(&logs).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var combined strings.Builder
		for i, lg := range logs {
			if i > 0 && combined.Len() > 0 && !strings.HasSuffix(combined.String(), "\n") {
				combined.WriteByte('\n')
			}
			combined.WriteString(lg.Content)
		}
		fullLog := combined.String()
		tailContent, truncated := truncateUTF8Tail(fullLog, tailBytes)
		out["log"] = map[string]any{
			"content":    tailContent,
			"totalBytes": len(fullLog),
			"tailBytes":  len(tailContent),
			"truncated":  truncated,
		}

		var hookRuns []model.HookRun
		if err := tc.DB.Where("build_run_id = ? AND project_id = ?", run.ID, project.ID).Order("created_at asc").Find(&hookRuns).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		summaries := make([]aiHookRunSummary, len(hookRuns))
		for i, hr := range hookRuns {
			summaries[i] = aiHookRunSummary{
				ID:                 hr.ID,
				HookConfigID:       hr.HookConfigID,
				BuildRunID:         hr.BuildRunID,
				BuildJobID:         hr.BuildJobID,
				ApplicationID:      hr.ApplicationID,
				DeploymentTargetID: hr.DeploymentTargetID,
				Name:               hr.Name,
				Phase:              hr.Phase,
				Status:             hr.Status,
				Shell:              hr.Shell,
				ImageRef:           hr.ImageRef,
				TimeoutSeconds:     hr.TimeoutSeconds,
				FailurePolicy:      hr.FailurePolicy,
				ExitCode:           hr.ExitCode,
				Message:            hr.Message,
				StartedAt:          hr.StartedAt,
				FinishedAt:         hr.FinishedAt,
				CreatedAt:          hr.CreatedAt,
			}
		}
		out["hookRuns"] = summaries
	}

	return out, nil
}

func buildRunToMap(run model.BuildRun) map[string]any {
	raw, _ := json.Marshal(run)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func truncateUTF8Tail(s string, maxBytes int) (string, bool) {
	if len(s) <= maxBytes {
		return s, false
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:], true
}

func buildTrimPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func buildBoolPtrDefault(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}
