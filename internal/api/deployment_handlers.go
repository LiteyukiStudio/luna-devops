package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListRuntimeClusters(ctx *gin.Context) {
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

	var clusters []model.RuntimeCluster
	query := h.db.Order("is_default desc, created_at desc")
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
	if err := applySearch(ctx, query, "name", "endpoint").Find(&clusters).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	for index := range clusters {
		clusters[index] = h.runtimeClusterResponseForUser(user, clusters[index])
	}
	ctx.JSON(http.StatusOK, clusters)
}

func (h *Handlers) CreateRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input runtimeClusterInput
	if !bindJSON(ctx, &input) {
		return
	}
	clusterID := id.New("clu")
	cluster, ok := h.runtimeClusterFromInput(ctx, user, input, clusterID)
	if !ok {
		return
	}
	if err := h.saveRuntimeClusterWithDefault(cluster); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, h.runtimeClusterResponseForUser(user, cluster))
}

func (h *Handlers) UpdateRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var existing model.RuntimeCluster
	if err := h.db.First(&existing, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResource(ctx, user, existing.Scope, existing.OwnerRef, "无权维护该运行集群") {
		return
	}
	var input runtimeClusterInput
	if !bindJSON(ctx, &input) {
		return
	}
	if strings.TrimSpace(input.Kubeconfig) != "" && !h.canInspectRuntimeClusterKubeconfig(user, existing) {
		writeError(ctx, http.StatusForbidden, "只有创建者或平台管理员可以编辑 kubeconfig")
		return
	}
	next, ok := h.runtimeClusterFromInput(ctx, user, input, existing.ID)
	if !ok {
		return
	}
	existing.Name = next.Name
	existing.Type = next.Type
	existing.Endpoint = next.Endpoint
	existing.Scope = next.Scope
	existing.OwnerRef = next.OwnerRef
	if next.KubeconfigRef != "" {
		existing.KubeconfigRef = next.KubeconfigRef
	}
	existing.IsDefault = next.IsDefault
	existing.Status = next.Status
	if err := h.saveRuntimeClusterWithDefault(existing); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.runtimeClusterResponseForUser(user, existing))
}

func (h *Handlers) DeleteRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.db.First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResource(ctx, user, cluster.Scope, cluster.OwnerRef, "无权维护该运行集群") {
		return
	}
	if err := h.db.Delete(&cluster).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) TestRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.db.First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResource(ctx, user, cluster.Scope, cluster.OwnerRef, "无权测试该运行集群") {
		return
	}
	now := time.Now()
	cluster.LastCheckedAt = &now
	if cluster.KubeconfigRef == "" {
		cluster.Status = "missing-credential"
		if err := h.db.Save(&cluster).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法测试连接")
		return
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		cluster.Status = "missing-credential"
		if err := h.db.Save(&cluster).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法测试连接")
		return
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		cluster.Status = "unhealthy"
		if saveErr := h.db.Save(&cluster).Error; saveErr != nil {
			writeError(ctx, http.StatusInternalServerError, saveErr.Error())
			return
		}
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return
	}
	pingCtx, cancel := context.WithTimeout(ctx.Request.Context(), 8*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx); err != nil {
		cluster.Status = "unhealthy"
		if saveErr := h.db.Save(&cluster).Error; saveErr != nil {
			writeError(ctx, http.StatusInternalServerError, saveErr.Error())
			return
		}
		writeError(ctx, http.StatusBadGateway, "运行集群连接测试失败，请检查 kubeconfig、集群地址和网络连通性")
		return
	}
	cluster.Status = "ready"
	if err := h.db.Save(&cluster).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.runtimeClusterResponseForUser(user, cluster))
}

func (h *Handlers) ListEnvironments(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	var environments []model.Environment
	query := h.db.Where("project_id = ?", ctx.Param("projectId")).Order("created_at desc")
	query = applySearch(ctx, query, "name", "slug", "stage", "namespace")
	if err := query.Find(&environments).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, environments)
}

func (h *Handlers) CreateEnvironment(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	var input environmentInput
	if !bindJSON(ctx, &input) {
		return
	}
	environment := environmentFromInput(ctx.Param("projectId"), user.ID, input, "")
	environment.ID = id.New("env")
	if err := h.db.Create(&environment).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, environment)
}

func (h *Handlers) UpdateEnvironment(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer"); !ok {
		return
	}
	environment, ok := h.findEnvironment(ctx)
	if !ok {
		return
	}
	var input environmentInput
	if !bindJSON(ctx, &input) {
		return
	}
	next := environmentFromInput(ctx.Param("projectId"), environment.CreatedBy, input, environment.ID)
	environment.Name = next.Name
	environment.Slug = next.Slug
	environment.Stage = next.Stage
	environment.ClusterID = next.ClusterID
	environment.Namespace = next.Namespace
	environment.Replicas = next.Replicas
	environment.CPURequest = next.CPURequest
	environment.MemoryRequest = next.MemoryRequest
	environment.EnvVars = next.EnvVars
	environment.ConfigRefs = next.ConfigRefs
	environment.SecretRefs = next.SecretRefs
	if err := h.db.Save(&environment).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, environment)
}

func (h *Handlers) DeleteEnvironment(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin"); !ok {
		return
	}
	environment, ok := h.findEnvironment(ctx)
	if !ok {
		return
	}
	if err := h.db.Delete(&environment).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListReleases(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	query := h.db.Where("project_id = ?", ctx.Param("projectId")).Order("created_at desc")
	if environmentID := strings.TrimSpace(ctx.Query("environmentId")); environmentID != "" {
		query = query.Where("environment_id = ?", environmentID)
	}
	var releases []model.Release
	if err := query.Find(&releases).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, releases)
}

func (h *Handlers) CreateRelease(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	var input releaseInput
	if !bindJSON(ctx, &input) {
		return
	}
	release := releaseFromInput(ctx.Param("projectId"), user.ID, input, "")
	release.ID = id.New("rel")
	if err := h.db.Create(&release).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.enqueueDeployRun(ctx.Request.Context(), release) {
		release.Status = "failed"
		release.Message = "部署任务投递失败，请稍后重试"
		if err := h.db.Save(&release).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(ctx, http.StatusServiceUnavailable, "部署队列暂不可用")
		return
	}
	ctx.JSON(http.StatusCreated, release)
}

func (h *Handlers) RollbackRelease(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	source, ok := h.findRelease(ctx)
	if !ok {
		return
	}
	target, ok := h.findPreviousSuccessfulRelease(ctx, source)
	if !ok {
		return
	}
	revision, err := h.nextReleaseRevision(source)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	release := rollbackReleaseFromTarget(source, target, user.ID, revision)
	release.ID = id.New("rel")
	if err := h.db.Create(&release).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.enqueueDeployRun(ctx.Request.Context(), release) {
		release.Status = "failed"
		release.Message = "部署任务投递失败，请稍后重试"
		if err := h.db.Save(&release).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(ctx, http.StatusServiceUnavailable, "部署队列暂不可用")
		return
	}
	ctx.JSON(http.StatusCreated, release)
}

func (h *Handlers) findPreviousSuccessfulRelease(ctx *gin.Context, source model.Release) (model.Release, bool) {
	var target model.Release
	err := h.db.Where(
		"project_id = ? and application_id = ? and environment_id = ? and status = ? and revision < ?",
		source.ProjectID,
		source.ApplicationID,
		source.EnvironmentID,
		"succeeded",
		source.Revision,
	).Order("revision desc, created_at desc").First(&target).Error
	if err != nil {
		writeError(ctx, http.StatusConflict, "上一成功版本不存在")
		return target, false
	}
	return target, true
}

func (h *Handlers) nextReleaseRevision(source model.Release) (int, error) {
	var maxRevision int
	err := h.db.Model(&model.Release{}).
		Where("project_id = ? and application_id = ? and environment_id = ?", source.ProjectID, source.ApplicationID, source.EnvironmentID).
		Select("coalesce(max(revision), 0)").
		Scan(&maxRevision).Error
	if err != nil {
		return 0, err
	}
	return maxRevision + 1, nil
}

func (h *Handlers) enqueueDeployRun(ctx context.Context, release model.Release) bool {
	if h.taskClient == nil {
		return false
	}
	_, err := h.taskClient.EnqueueDeployRun(ctx, tasks.DeployRunPayload{
		ReleaseID: release.ID,
		ProjectID: release.ProjectID,
		ActorID:   release.CreatedBy,
	})
	return err == nil
}

func (h *Handlers) findEnvironment(ctx *gin.Context) (model.Environment, bool) {
	var environment model.Environment
	if err := h.db.First(&environment, "id = ? and project_id = ?", ctx.Param("environmentId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "environment not found")
		return environment, false
	}
	return environment, true
}

func (h *Handlers) findRelease(ctx *gin.Context) (model.Release, bool) {
	var release model.Release
	if err := h.db.First(&release, "id = ? and project_id = ?", ctx.Param("releaseId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "release not found")
		return release, false
	}
	return release, true
}

func (h *Handlers) runtimeClusterResponseForUser(user model.User, cluster model.RuntimeCluster) model.RuntimeCluster {
	cluster.KubeconfigSet = cluster.KubeconfigRef != ""
	cluster.Kubeconfig = ""
	if !h.canInspectScopedResourceConfig(user, cluster.Scope, cluster.OwnerRef) {
		cluster.Endpoint = ""
	}
	if h.canInspectRuntimeClusterKubeconfig(user, cluster) {
		cluster.Kubeconfig = h.secrets.Resolve(cluster.KubeconfigRef)
	}
	return cluster
}

func (h *Handlers) canInspectRuntimeClusterKubeconfig(user model.User, cluster model.RuntimeCluster) bool {
	return user.Role == "platform_admin" || cluster.CreatedBy == user.ID
}

func (h *Handlers) runtimeClusterFromInput(ctx *gin.Context, user model.User, input runtimeClusterInput, clusterID string) (model.RuntimeCluster, bool) {
	scope, ownerRef, ok := h.normalizeScopedOwner(ctx, user, input.Scope, input.OwnerRef, "只有平台管理员可以维护全局运行集群")
	if !ok {
		return model.RuntimeCluster{}, false
	}
	if input.IsDefault && scope != "global" {
		writeError(ctx, http.StatusBadRequest, "只有全局运行集群可以设为默认集群")
		return model.RuntimeCluster{}, false
	}
	kubeconfigRef := ""
	if strings.TrimSpace(input.Kubeconfig) != "" {
		kubeconfigRef = h.secrets.Store(input.Kubeconfig, user.ID, "runtime_cluster:"+clusterID+":kubeconfig")
	}
	return model.RuntimeCluster{
		ID:            clusterID,
		Name:          strings.TrimSpace(input.Name),
		Type:          normalizeRuntimeClusterType(input.Type),
		Endpoint:      strings.TrimSpace(input.Endpoint),
		Scope:         scope,
		OwnerRef:      ownerRef,
		KubeconfigRef: kubeconfigRef,
		IsDefault:     input.IsDefault,
		Status:        fallback(strings.TrimSpace(input.Status), "unknown"),
		CreatedBy:     user.ID,
	}, true
}

func (h *Handlers) saveRuntimeClusterWithDefault(cluster model.RuntimeCluster) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		if cluster.IsDefault {
			if cluster.Scope != "global" {
				return errors.New("只有全局运行集群可以设为默认集群")
			}
			if err := tx.Model(&model.RuntimeCluster{}).Where("scope = ? and id <> ?", "global", cluster.ID).Update("is_default", false).Error; err != nil {
				return err
			}
		} else if cluster.Scope != "global" {
			cluster.IsDefault = false
		}
		return tx.Save(&cluster).Error
	})
}

func environmentFromInput(projectID, userID string, input environmentInput, environmentID string) model.Environment {
	return model.Environment{
		ID:            environmentID,
		ProjectID:     projectID,
		Name:          strings.TrimSpace(input.Name),
		Slug:          strings.TrimSpace(input.Slug),
		Stage:         normalizeStage(input.Stage),
		ClusterID:     strings.TrimSpace(input.ClusterID),
		Namespace:     strings.TrimSpace(input.Namespace),
		Replicas:      fallbackInt(input.Replicas, 1),
		CPURequest:    strings.TrimSpace(input.CPURequest),
		MemoryRequest: strings.TrimSpace(input.MemoryRequest),
		EnvVars:       strings.TrimSpace(input.EnvVars),
		ConfigRefs:    strings.TrimSpace(input.ConfigRefs),
		SecretRefs:    strings.TrimSpace(input.SecretRefs),
		CreatedBy:     userID,
	}
}

func releaseFromInput(projectID, userID string, input releaseInput, releaseID string) model.Release {
	return model.Release{
		ID:            releaseID,
		ProjectID:     projectID,
		ApplicationID: strings.TrimSpace(input.ApplicationID),
		EnvironmentID: strings.TrimSpace(input.EnvironmentID),
		BuildRunID:    strings.TrimSpace(input.BuildRunID),
		ImageRef:      strings.TrimSpace(input.ImageRef),
		Type:          normalizeReleaseType(input.Type),
		Status:        fallback(strings.TrimSpace(input.Status), "pending"),
		Revision:      fallbackInt(input.Revision, 1),
		Message:       strings.TrimSpace(input.Message),
		CreatedBy:     userID,
	}
}

func rollbackReleaseFromTarget(source model.Release, target model.Release, userID string, revision int) model.Release {
	return model.Release{
		ProjectID:      source.ProjectID,
		ApplicationID:  source.ApplicationID,
		EnvironmentID:  source.EnvironmentID,
		ImageRef:       target.ImageRef,
		Type:           "rollback",
		Status:         "pending",
		Revision:       fallbackInt(revision, source.Revision+1),
		RollbackFromID: target.ID,
		CreatedBy:      userID,
	}
}

func normalizeRuntimeClusterType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "docker-compose":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "kubernetes"
	}
}

func normalizeStage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "prod", "production":
		return "prod"
	case "staging", "test":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "dev"
	}
}

func normalizeReleaseType(value string) string {
	if strings.ToLower(strings.TrimSpace(value)) == "rollback" {
		return "rollback"
	}
	return "deploy"
}

type runtimeClusterInput struct {
	Name       string `json:"name" binding:"required"`
	Type       string `json:"type"`
	Endpoint   string `json:"endpoint"`
	Scope      string `json:"scope"`
	OwnerRef   string `json:"ownerRef"`
	Kubeconfig string `json:"kubeconfig"`
	IsDefault  bool   `json:"isDefault"`
	Status     string `json:"status"`
}

type environmentInput struct {
	Name          string `json:"name" binding:"required"`
	Slug          string `json:"slug" binding:"required"`
	Stage         string `json:"stage"`
	ClusterID     string `json:"clusterId"`
	Namespace     string `json:"namespace"`
	Replicas      int    `json:"replicas"`
	CPURequest    string `json:"cpuRequest"`
	MemoryRequest string `json:"memoryRequest"`
	EnvVars       string `json:"envVars"`
	ConfigRefs    string `json:"configRefs"`
	SecretRefs    string `json:"secretRefs"`
}

type releaseInput struct {
	ApplicationID string `json:"applicationId" binding:"required"`
	EnvironmentID string `json:"environmentId" binding:"required"`
	BuildRunID    string `json:"buildRunId"`
	ImageRef      string `json:"imageRef" binding:"required"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Revision      int    `json:"revision"`
	Message       string `json:"message"`
}
