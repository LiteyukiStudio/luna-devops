package api

import (
	"fmt"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

func (h *Handlers) ListEnvironments(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	var environments []model.Environment
	query := h.db.Where("project_id = ?", ctx.Param("projectId")).Order("created_at desc")
	query = applySearch(ctx, query, "name", "slug", "namespace")
	if err := query.Find(&environments).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, environments)
}

func (h *Handlers) CreateEnvironment(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var input environmentInput
	if !bindJSON(ctx, &input) {
		return
	}
	if !validateEnvironmentSlug(ctx, input.Slug) {
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
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
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
	if !validateEnvironmentSlug(ctx, input.Slug) {
		return
	}
	next := environmentFromInput(ctx.Param("projectId"), environment.CreatedBy, input, environment.ID)
	environment.Name = next.Name
	environment.Slug = next.Slug
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
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	environment, ok := h.findEnvironment(ctx)
	if !ok {
		return
	}
	if !h.ensureEnvironmentCanDelete(ctx, environment) {
		return
	}
	if err := h.db.Delete(&environment).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ensureEnvironmentCanDelete(ctx *gin.Context, environment model.Environment) bool {
	checks := []struct {
		model   any
		message string
	}{
		{&model.DeploymentTarget{}, "环境仍被部署配置引用，请先删除相关部署配置"},
		{&model.GatewayRoute{}, "环境仍被访问入口引用，请先删除相关访问入口"},
		{&model.Release{}, "环境仍被发布记录引用，请先删除或归档相关发布记录"},
	}
	for _, check := range checks {
		var count int64
		err := h.db.Model(check.model).
			Where("project_id = ? and environment_id = ?", environment.ProjectID, environment.ID).
			Count(&count).Error
		if err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return false
		}
		if count > 0 {
			writeError(ctx, http.StatusConflict, check.message)
			return false
		}
	}
	return true
}

func (h *Handlers) findEnvironment(ctx *gin.Context) (model.Environment, bool) {
	var environment model.Environment
	if err := h.db.First(&environment, "id = ? and project_id = ?", ctx.Param("environmentId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "environment not found")
		return environment, false
	}
	return environment, true
}

func environmentFromInput(projectID, userID string, input environmentInput, environmentID string) model.Environment {
	slug := strings.TrimSpace(input.Slug)
	return model.Environment{
		ID:            environmentID,
		ProjectID:     projectID,
		Name:          strings.TrimSpace(input.Name),
		Slug:          slug,
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

func defaultProductionEnvironment(projectID, userID string) model.Environment {
	return model.Environment{
		ID:            id.New("env"),
		ProjectID:     projectID,
		Name:          "Production",
		Slug:          "prod",
		Replicas:      1,
		CPURequest:    "500m",
		MemoryRequest: "512Mi",
		EnvVars:       "{}",
		CreatedBy:     userID,
	}
}

func validateEnvironmentSlug(ctx *gin.Context, slug string) bool {
	slug = strings.TrimSpace(slug)
	if len(slug) > environmentSlugMaxLength {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("环境标识最多 %d 个字符", environmentSlugMaxLength))
		return false
	}
	return true
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

type environmentInput struct {
	Name          string `json:"name" binding:"required"`
	Slug          string `json:"slug" binding:"required"`
	ClusterID     string `json:"clusterId"`
	Namespace     string `json:"namespace"`
	Replicas      int    `json:"replicas"`
	CPURequest    string `json:"cpuRequest"`
	MemoryRequest string `json:"memoryRequest"`
	EnvVars       string `json:"envVars"`
	ConfigRefs    string `json:"configRefs"`
	SecretRefs    string `json:"secretRefs"`
}
