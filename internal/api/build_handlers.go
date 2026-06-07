package api

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"strings"

	builderagent "github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListBuildProviders(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(ctx.Query("projectId"))
	if projectID != "" {
		if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); !ok {
			return
		}
	}

	query := h.db.Order("created_at desc")
	conditions := []string{"scope = 'global'", "(scope = 'user' and owner_ref = ?)"}
	args := []any{user.ID}
	if projectID != "" {
		conditions = append(conditions, "(scope = 'project' and owner_ref = ?)")
		args = append(args, projectID)
	} else if user.Role == "platform_admin" {
		conditions = append(conditions, "scope = 'project'")
	} else {
		projectIDs := h.projectIDsForUser(user.ID)
		if len(projectIDs) > 0 {
			conditions = append(conditions, "(scope = 'project' and owner_ref in ?)")
			args = append(args, projectIDs)
		}
	}
	query = query.Where(strings.Join(conditions, " or "), args...)
	query = applySearch(ctx, query, "name", "type")

	var providers []model.BuildProvider
	if err := query.Find(&providers).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, providers)
}

func (h *Handlers) ListBuilderAgents(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	var builders []model.BuilderAgent
	query := applySearch(ctx, h.db.Model(&model.BuilderAgent{}), "name", "id", "labels", "scopes", "executor", "status")
	if strings.TrimSpace(ctx.Query("includeOffline")) != "true" {
		query = query.Where("status = ?", "online")
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":               "name",
		"status":             "status",
		"executor":           "executor",
		"lastHeartbeatAt":    "last_heartbeat_at",
		"currentConcurrency": "current_concurrency",
		"updatedAt":          "updated_at",
	}, "updated_at")).Find(&builders).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if user.Role == "platform_admin" {
		total := int64(len(builders))
		ctx.JSON(http.StatusOK, paginatedResponse(paginateSlice(builders, pagination), total, pagination))
		return
	}
	projectIDs := h.projectIDsForUser(user.ID)
	visible := make([]model.BuilderAgent, 0, len(builders))
	for _, builder := range builders {
		if builderVisibleToUser(builder.Scopes, user.ID, projectIDs) {
			visible = append(visible, builder)
		}
	}
	total := int64(len(visible))
	ctx.JSON(http.StatusOK, paginatedResponse(paginateSlice(visible, pagination), total, pagination))
}

func (h *Handlers) DeleteBuilderAgent(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != "platform_admin" {
		writeError(ctx, http.StatusForbidden, "只有平台管理员可以删除构建器注册记录")
		return
	}
	var builder model.BuilderAgent
	if err := h.db.First(&builder, "id = ?", ctx.Param("builderId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "builder not found")
		return
	}
	if err := h.db.Delete(&builder).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) CreateBuildProvider(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input buildProviderInput
	if !bindJSON(ctx, &input) {
		return
	}
	provider, ok := h.buildProviderFromInput(ctx, user, input, "")
	if !ok {
		return
	}
	provider.ID = id.New("bldp")
	provider.CreatedBy = user.ID
	if err := h.db.Create(&provider).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, provider)
}

func (h *Handlers) UpdateBuildProvider(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var existing model.BuildProvider
	if err := h.db.First(&existing, "id = ?", ctx.Param("providerId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build provider not found")
		return
	}
	if !h.canManageScopedResource(ctx, user, existing.Scope, existing.OwnerRef, "无权维护该构建提供者") {
		return
	}
	var input buildProviderInput
	if !bindJSON(ctx, &input) {
		return
	}
	next, ok := h.buildProviderFromInput(ctx, user, input, existing.ID)
	if !ok {
		return
	}
	existing.Name = next.Name
	existing.Type = next.Type
	existing.Scope = next.Scope
	existing.OwnerRef = next.OwnerRef
	existing.Config = next.Config
	existing.Enabled = next.Enabled
	if err := h.db.Save(&existing).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, existing)
}

func (h *Handlers) DeleteBuildProvider(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var provider model.BuildProvider
	if err := h.db.First(&provider, "id = ?", ctx.Param("providerId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build provider not found")
		return
	}
	if !h.canManageScopedResource(ctx, user, provider.Scope, provider.OwnerRef, "无权维护该构建提供者") {
		return
	}
	if err := h.db.Delete(&provider).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListBuildVariableSets(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(ctx.Query("projectId"))
	if projectID != "" {
		if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); !ok {
			return
		}
	}

	query := h.db.Order("created_at desc")
	conditions := []string{"scope = 'global'", "(scope = 'user' and owner_ref = ?)"}
	args := []any{user.ID}
	if projectID != "" {
		conditions = append(conditions, "(scope = 'project' and owner_ref = ?)")
		args = append(args, projectID)
	} else if user.Role == "platform_admin" {
		conditions = append(conditions, "scope = 'project'")
	} else {
		projectIDs := h.projectIDsForUser(user.ID)
		if len(projectIDs) > 0 {
			conditions = append(conditions, "(scope = 'project' and owner_ref in ?)")
			args = append(args, projectIDs)
		}
	}
	query = query.Where(strings.Join(conditions, " or "), args...)
	query = applySearch(ctx, query, "name")

	var sets []model.BuildVariableSet
	if err := query.Find(&sets).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, sets)
}

func (h *Handlers) CreateBuildVariableSet(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input buildVariableSetInput
	if !bindJSON(ctx, &input) {
		return
	}
	set, ok := h.buildVariableSetFromInput(ctx, user, input, "")
	if !ok {
		return
	}
	set.ID = id.New("bvs")
	set.CreatedBy = user.ID
	if err := h.db.Create(&set).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, set)
}

func (h *Handlers) UpdateBuildVariableSet(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var existing model.BuildVariableSet
	if err := h.db.First(&existing, "id = ?", ctx.Param("setId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build variable set not found")
		return
	}
	if !h.canManageScopedResource(ctx, user, existing.Scope, existing.OwnerRef, "无权维护该构建变量集") {
		return
	}
	var input buildVariableSetInput
	if !bindJSON(ctx, &input) {
		return
	}
	next, ok := h.buildVariableSetFromInput(ctx, user, input, existing.ID)
	if !ok {
		return
	}
	existing.Name = next.Name
	existing.Scope = next.Scope
	existing.OwnerRef = next.OwnerRef
	existing.Variables = next.Variables
	existing.Enabled = next.Enabled
	if err := h.db.Save(&existing).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, existing)
}

func (h *Handlers) DeleteBuildVariableSet(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var set model.BuildVariableSet
	if err := h.db.First(&set, "id = ?", ctx.Param("setId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build variable set not found")
		return
	}
	if !h.canManageScopedResource(ctx, user, set.Scope, set.OwnerRef, "无权维护该构建变量集") {
		return
	}
	if err := h.db.Delete(&set).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListBuildRuns(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.db.Where("project_id = ?", ctx.Param("projectId"))
	query = applySearch(ctx, query, "id", "source_commit", "target_repository", "image_ref")
	var runs []model.BuildRun
	if ctx.Query("page") == "" && ctx.Query("pageSize") == "" {
		if err := query.Order("created_at desc").Find(&runs).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, runs)
		return
	}
	var total int64
	if err := query.Model(&model.BuildRun{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	orderBy := orderByClause(pagination, map[string]string{
		"createdAt":    "created_at",
		"status":       "status",
		"sourceCommit": "source_commit",
	}, "created_at")
	if err := query.Order(orderBy).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&runs).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(runs, total, pagination))
}

func (h *Handlers) GetBuildRun(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	run, ok := h.findBuildRun(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, run)
}

func (h *Handlers) TriggerBuildRun(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	var input buildRunInput
	if !bindJSON(ctx, &input) {
		return
	}
	run := h.buildRunFromInput(ctx.Param("projectId"), user.ID, input)
	run.ID = id.New("bldr")
	run.Status = "queued"
	run.TriggerType = fallback(strings.TrimSpace(input.TriggerType), "manual")
	h.createQueuedBuildRun(ctx, user, run, strings.TrimSpace(input.TargetImageRef), http.StatusCreated)
}

func (h *Handlers) RetryBuildRun(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	previous, ok := h.findBuildRun(ctx)
	if !ok {
		return
	}
	run := model.BuildRun{
		ID:                  id.New("bldr"),
		ProjectID:           previous.ProjectID,
		ApplicationID:       previous.ApplicationID,
		BuildProviderID:     previous.BuildProviderID,
		BuildVariableSetIDs: previous.BuildVariableSetIDs,
		Status:              "queued",
		TriggerType:         "retry",
		SourceBranch:        previous.SourceBranch,
		SourceTag:           previous.SourceTag,
		SourceCommit:        previous.SourceCommit,
		DockerfilePath:      previous.DockerfilePath,
		BuildContext:        previous.BuildContext,
		BuildDirectory:      previous.BuildDirectory,
		TargetRegistryID:    previous.TargetRegistryID,
		TargetRepository:    previous.TargetRepository,
		TargetTag:           previous.TargetTag,
		CacheConfig:         previous.CacheConfig,
		CreatedBy:           user.ID,
	}
	h.createQueuedBuildRun(ctx, user, run, "", http.StatusCreated)
}

func (h *Handlers) createQueuedBuildRun(ctx *gin.Context, user model.User, run model.BuildRun, targetImageRef string, statusCode int) {
	if !h.validateBuildRunRequest(ctx, user, &run) {
		return
	}
	builder, ok := h.selectBuilderForRun(ctx, user, run)
	if !ok {
		return
	}
	job := model.BuildJob{
		ID:         id.New("bldj"),
		BuildRunID: run.ID,
		ProjectID:  run.ProjectID,
		Type:       "build",
		Status:     "queued",
		BuilderID:  builder.ID,
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if targetImageRef != "" {
			if err := tx.Model(&model.Application{}).
				Where("id = ? and project_id = ?", run.ApplicationID, run.ProjectID).
				Update("target_image_ref", targetImageRef).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		return tx.Create(&job).Error
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.enqueueRedisBuilderTask(ctx.Request.Context(), run, job, builder.ID); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(statusCode, run)
}

func (h *Handlers) enqueueRedisBuilderTask(ctx context.Context, run model.BuildRun, job model.BuildJob, builderID string) error {
	if h.builderQueue == nil {
		return errors.New("redis builder queue is not configured")
	}
	payload, err := h.builderPayloadForRun(h.db, run, job)
	if err != nil {
		return err
	}
	return builderagent.EnqueueRedisTask(ctx, h.builderQueue, builderagent.Task{
		JobID:         payload.JobID,
		TargetBuilder: builderID,
		BuildRunID:    payload.BuildRunID,
		ProjectID:     payload.ProjectID,
		ApplicationID: payload.ApplicationID,
		Repository: builderagent.RepositoryPayload{
			CloneURL:     payload.Repository.CloneURL,
			Owner:        payload.Repository.Owner,
			Repo:         payload.Repository.Repo,
			SourceBranch: payload.Repository.SourceBranch,
			SourceTag:    payload.Repository.SourceTag,
			SourceCommit: payload.Repository.SourceCommit,
			AccessToken:  payload.Repository.AccessToken,
		},
		Build: builderagent.BuildPayload{
			DockerfilePath: payload.Build.DockerfilePath,
			BuildContext:   payload.Build.BuildContext,
			BuildDirectory: payload.Build.BuildDirectory,
			Env:            payload.Build.Env,
		},
		Registry: builderagent.RegistryPayload{
			Endpoint:         payload.Registry.Endpoint,
			Username:         payload.Registry.Username,
			Password:         payload.Registry.Password,
			ImageRef:         payload.Registry.ImageRef,
			ImageNamePrefix:  payload.Registry.ImageNamePrefix,
			ImageTagTemplate: payload.Registry.ImageTagTemplate,
		},
	})
}

func (h *Handlers) validateBuildRunRequest(ctx *gin.Context, user model.User, run *model.BuildRun) bool {
	var project model.Project
	if err := h.db.First(&project, "id = ?", run.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "项目空间不存在")
		return false
	}
	var app model.Application
	if err := h.db.First(&app, "id = ? and project_id = ?", run.ApplicationID, run.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "应用不存在")
		return false
	}
	run.DockerfilePath = fallback(strings.TrimSpace(app.DockerfilePath), "Dockerfile")
	run.BuildContext = fallback(strings.TrimSpace(app.BuildContext), ".")
	run.BuildDirectory = ""
	run.BuildLabels = strings.Join(normalizeBuildSelectorList(strings.Split(app.BuildLabels, ",")), ",")
	if app.SourceType == "repository" {
		var count int64
		if err := h.db.Model(&model.RepositoryBinding{}).Where("project_id = ? and application_id = ?", run.ProjectID, run.ApplicationID).Count(&count).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return false
		}
		if count == 0 {
			writeError(ctx, http.StatusBadRequest, "应用未绑定代码仓库")
			return false
		}
	}
	if strings.TrimSpace(run.BuildProviderID) != "" {
		var provider model.BuildProvider
		if err := h.db.First(&provider, "id = ? and enabled = ?", run.BuildProviderID, true).Error; err != nil {
			writeError(ctx, http.StatusBadRequest, "构建提供方不可用")
			return false
		}
	}
	if strings.TrimSpace(run.TargetRegistryID) == "" {
		writeError(ctx, http.StatusBadRequest, "目标镜像站不能为空")
		return false
	}
	var registry model.ArtifactRegistry
	if err := h.db.First(&registry, "id = ?", run.TargetRegistryID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "目标镜像站不存在")
		return false
	}
	if strings.TrimSpace(run.TargetRepository) == "" {
		repository, tag := splitTargetImageRef(fallback(strings.TrimSpace(app.TargetImageRef), buildTargetImageRepository(registry, project, app)))
		run.TargetRepository = repository
		run.TargetTag = tag
	}
	run.TargetRepository = strings.Trim(strings.TrimSpace(run.TargetRepository), "/")
	run.TargetTag = fallback(strings.TrimSpace(run.TargetTag), "latest")
	run.ImageRef = fallback(strings.TrimSpace(run.ImageRef), buildImageRef(registry, *run))
	if !h.usableRegistryCredentialExists(user.ID, registry) {
		writeError(ctx, http.StatusBadRequest, "目标镜像站缺少可用推送凭据")
		return false
	}
	if _, ok := h.buildVariablesForRun(ctx, user, run.ProjectID, buildVariableSetIDs(run.BuildVariableSetIDs)); !ok {
		return false
	}
	return true
}

func (h *Handlers) selectBuilderForRun(ctx *gin.Context, user model.User, run model.BuildRun) (model.BuilderAgent, bool) {
	requiredLabels := normalizeBuildSelectorList(strings.Split(run.BuildLabels, ","))
	var builders []model.BuilderAgent
	if err := h.db.Where("status = ?", "online").Find(&builders).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.BuilderAgent{}, false
	}
	candidates := make([]model.BuilderAgent, 0, len(builders))
	for _, builder := range builders {
		if builder.MaxConcurrency > 0 && builder.CurrentConcurrency >= builder.MaxConcurrency {
			continue
		}
		if !builderHasLabels(builder.Labels, requiredLabels) {
			continue
		}
		if !builderAllowsRun(builder.Scopes, run.ProjectID, user.ID) {
			continue
		}
		candidates = append(candidates, builder)
	}
	if len(candidates) == 0 {
		writeError(ctx, http.StatusBadRequest, "没有可用 Builder，请检查 Builder 是否在线、标签是否匹配、scope 是否允许当前项目或用户")
		return model.BuilderAgent{}, false
	}
	return candidates[rand.IntN(len(candidates))], true
}

func (h *Handlers) usableRegistryCredentialExists(userID string, registry model.ArtifactRegistry) bool {
	if strings.TrimSpace(registry.CredentialRef) != "" {
		var count int64
		h.db.Model(&model.RegistryCredential{}).
			Where("registry_id = ? and scope in ?", registry.ID, []string{"push", "push-pull"}).
			Where("id = ? and (access_scope = ? or created_by = ?)", registry.CredentialRef, "registry", userID).
			Count(&count)
		if count > 0 {
			return true
		}
	}
	var count int64
	h.db.Model(&model.RegistryCredential{}).
		Where("registry_id = ? and scope in ?", registry.ID, []string{"push", "push-pull"}).
		Where("(access_scope = ? and created_by = ?) or access_scope = ?", "personal", userID, "registry").
		Count(&count)
	return count > 0
}

func (h *Handlers) ListBuildJobs(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.db.Where("project_id = ?", ctx.Param("projectId"))
	if runID := strings.TrimSpace(ctx.Query("buildRunId")); runID != "" {
		query = query.Where("build_run_id = ?", runID)
	}
	var jobs []model.BuildJob
	if ctx.Query("page") == "" && ctx.Query("pageSize") == "" {
		if err := query.Order("created_at desc").Find(&jobs).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, jobs)
		return
	}
	var total int64
	if err := query.Model(&model.BuildJob{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	orderBy := orderByClause(pagination, map[string]string{
		"createdAt": "created_at",
		"status":    "status",
		"attempts":  "attempts",
	}, "created_at")
	if err := query.Order(orderBy).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&jobs).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(jobs, total, pagination))
}

func (h *Handlers) GetBuildJob(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	var job model.BuildJob
	if err := h.db.First(&job, "id = ? and project_id = ?", ctx.Param("jobId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build job not found")
		return
	}
	ctx.JSON(http.StatusOK, job)
}

func (h *Handlers) GetBuildJobLogs(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	var log model.BuildLog
	if err := h.db.First(&log, "build_job_id = ? and project_id = ?", ctx.Param("jobId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build log not found")
		return
	}
	ctx.JSON(http.StatusOK, log)
}

func (h *Handlers) findBuildRun(ctx *gin.Context) (model.BuildRun, bool) {
	var run model.BuildRun
	if err := h.db.First(&run, "id = ? and project_id = ?", ctx.Param("runId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build run not found")
		return run, false
	}
	return run, true
}

func (h *Handlers) buildProviderFromInput(ctx *gin.Context, user model.User, input buildProviderInput, providerID string) (model.BuildProvider, bool) {
	scope, ownerRef, ok := h.normalizeScopedOwner(ctx, user, input.Scope, input.OwnerRef, "只有平台管理员可以维护全局构建提供者")
	if !ok {
		return model.BuildProvider{}, false
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入构建提供者名称")
		return model.BuildProvider{}, false
	}
	return model.BuildProvider{
		ID:       providerID,
		Name:     name,
		Type:     normalizeBuildProviderType(input.Type),
		Scope:    scope,
		OwnerRef: ownerRef,
		Config:   strings.TrimSpace(input.Config),
		Enabled:  input.Enabled,
	}, true
}

func (h *Handlers) buildVariableSetFromInput(ctx *gin.Context, user model.User, input buildVariableSetInput, setID string) (model.BuildVariableSet, bool) {
	scope, ownerRef, ok := h.normalizeScopedOwner(ctx, user, input.Scope, input.OwnerRef, "只有平台管理员可以维护全局构建变量集")
	if !ok {
		return model.BuildVariableSet{}, false
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入构建变量集名称")
		return model.BuildVariableSet{}, false
	}
	variables, ok := normalizeBuildVariables(ctx, input.Variables)
	if !ok {
		return model.BuildVariableSet{}, false
	}
	if len(variables) == 0 {
		writeError(ctx, http.StatusBadRequest, "请至少配置一个构建变量")
		return model.BuildVariableSet{}, false
	}
	content, err := json.Marshal(variables)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.BuildVariableSet{}, false
	}
	return model.BuildVariableSet{
		ID:        setID,
		Name:      name,
		Scope:     scope,
		OwnerRef:  ownerRef,
		Variables: string(content),
		Enabled:   input.Enabled,
	}, true
}

func (h *Handlers) buildRunFromInput(projectID, userID string, input buildRunInput) model.BuildRun {
	targetRepository, targetTag := splitTargetImageRef(input.TargetImageRef)
	if targetRepository == "" {
		targetRepository = strings.Trim(strings.TrimSpace(input.TargetRepository), "/")
		targetTag = strings.TrimSpace(input.TargetTag)
	}
	return model.BuildRun{
		ProjectID:           projectID,
		ApplicationID:       strings.TrimSpace(input.ApplicationID),
		BuildProviderID:     strings.TrimSpace(input.BuildProviderID),
		BuildVariableSetIDs: encodeBuildVariableSetIDs(input.BuildVariableSetIDs),
		SourceBranch:        strings.TrimSpace(input.SourceBranch),
		SourceTag:           strings.TrimSpace(input.SourceTag),
		SourceCommit:        strings.TrimSpace(input.SourceCommit),
		DockerfilePath:      fallback(strings.TrimSpace(input.DockerfilePath), "Dockerfile"),
		BuildContext:        fallback(strings.TrimSpace(input.BuildContext), "."),
		BuildDirectory:      strings.TrimSpace(input.BuildDirectory),
		TargetRegistryID:    strings.TrimSpace(input.TargetRegistryID),
		TargetRepository:    targetRepository,
		TargetTag:           fallback(targetTag, "latest"),
		ImageRef:            "",
		CacheConfig:         strings.TrimSpace(input.CacheConfig),
		CreatedBy:           userID,
	}
}

func splitTargetImageRef(value string) (string, string) {
	normalized := strings.Trim(strings.TrimSpace(value), "/")
	if normalized == "" {
		return "", ""
	}
	lastSlash := strings.LastIndex(normalized, "/")
	lastColon := strings.LastIndex(normalized, ":")
	if lastColon > lastSlash {
		repository := strings.Trim(strings.TrimSpace(normalized[:lastColon]), "/")
		tag := strings.TrimSpace(normalized[lastColon+1:])
		return repository, tag
	}
	return normalized, "latest"
}

func normalizeBuildProviderType(value string) string {
	return "platform"
}

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
	seen := make(map[string]bool)
	for _, setID := range normalizeStringList(setIDs) {
		if seen[setID] {
			continue
		}
		seen[setID] = true
		var set model.BuildVariableSet
		if err := db.First(&set, "id = ? and enabled = ?", setID, true).Error; err != nil {
			return nil, errors.New("构建变量集不可用")
		}
		if !buildVariableSetAccessible(user, projectID, set) {
			return nil, errors.New("无权使用该构建变量集")
		}
		var values map[string]string
		if err := json.Unmarshal([]byte(fallback(set.Variables, "{}")), &values); err != nil {
			return nil, err
		}
		for key, value := range values {
			if isBuildEnvKey(key) {
				output[key] = value
			}
		}
	}
	return output, nil
}

func buildVariableSetAccessible(user model.User, projectID string, set model.BuildVariableSet) bool {
	switch set.Scope {
	case "global":
		return true
	case "user":
		return set.OwnerRef == user.ID
	case "project":
		return set.OwnerRef == projectID || user.Role == "platform_admin"
	default:
		return false
	}
}

type buildProviderInput struct {
	Name     string `json:"name" binding:"required"`
	Type     string `json:"type"`
	Scope    string `json:"scope"`
	OwnerRef string `json:"ownerRef"`
	Config   string `json:"config"`
	Enabled  bool   `json:"enabled"`
}

type buildVariableSetInput struct {
	Name      string            `json:"name" binding:"required"`
	Scope     string            `json:"scope"`
	OwnerRef  string            `json:"ownerRef"`
	Variables map[string]string `json:"variables"`
	Enabled   bool              `json:"enabled"`
}

type buildRunInput struct {
	ApplicationID       string   `json:"applicationId"`
	BuildProviderID     string   `json:"buildProviderId"`
	BuildVariableSetIDs []string `json:"buildVariableSetIds"`
	TriggerType         string   `json:"triggerType"`
	SourceBranch        string   `json:"sourceBranch"`
	SourceTag           string   `json:"sourceTag"`
	SourceCommit        string   `json:"sourceCommit"`
	DockerfilePath      string   `json:"dockerfilePath"`
	BuildContext        string   `json:"buildContext"`
	BuildDirectory      string   `json:"buildDirectory"`
	TargetRegistryID    string   `json:"targetRegistryId"`
	TargetImageRef      string   `json:"targetImageRef"`
	TargetRepository    string   `json:"targetRepository"`
	TargetTag           string   `json:"targetTag"`
	ImageRef            string   `json:"imageRef"`
	CacheConfig         string   `json:"cacheConfig"`
}
