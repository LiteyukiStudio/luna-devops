package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) ListApplications(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	var applications []model.Application
	query := h.db.Where("project_id = ?", ctx.Param("projectId"))
	query = applySearch(ctx, query, "name", "slug")
	if err := query.Order("created_at desc").Find(&applications).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, applications)
}

func (h *Handlers) CreateApplication(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}

	var input applicationInput
	if !bindJSON(ctx, &input) {
		return
	}
	input.Slug = strings.TrimSpace(input.Slug)
	if !h.ensureApplicationSlugAvailable(ctx, ctx.Param("projectId"), input.Slug, "") {
		return
	}
	gitAccountID, ok := h.applicationGitAccountID(ctx, user.ID, input)
	if !ok {
		return
	}

	app := model.Application{
		ID:             id.New("app"),
		ProjectID:      ctx.Param("projectId"),
		Slug:           input.Slug,
		Name:           input.Name,
		SourceType:     fallback(input.SourceType, "repository"),
		GitAccountID:   gitAccountID,
		RepositoryURL:  input.RepositoryURL,
		ImageReference: input.ImageReference,
		TargetImageRef: input.TargetImageRef,
		DockerfilePath: fallback(input.DockerfilePath, "Dockerfile"),
		BuildContext:   fallback(input.BuildContext, "."),
		BuildLabels:    strings.Join(normalizeBuildSelectorList(strings.Split(input.BuildLabels, ",")), ","),
		ServicePort:    fallbackInt(input.ServicePort, 8080),
	}

	if err := h.db.Create(&app).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, app)
}

func (h *Handlers) GetApplication(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, app)
}

func (h *Handlers) UpdateApplication(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}

	var input applicationInput
	if !bindJSON(ctx, &input) {
		return
	}
	input.Slug = strings.TrimSpace(input.Slug)
	if !h.ensureApplicationSlugAvailable(ctx, ctx.Param("projectId"), input.Slug, app.ID) {
		return
	}
	gitAccountID, ok := h.applicationGitAccountID(ctx, user.ID, input)
	if !ok {
		return
	}

	app.Slug = input.Slug
	app.Name = input.Name
	app.SourceType = fallback(input.SourceType, "repository")
	app.GitAccountID = gitAccountID
	app.RepositoryURL = input.RepositoryURL
	app.ImageReference = input.ImageReference
	app.TargetImageRef = input.TargetImageRef
	app.DockerfilePath = fallback(input.DockerfilePath, "Dockerfile")
	app.BuildContext = fallback(input.BuildContext, ".")
	app.BuildLabels = strings.Join(normalizeBuildSelectorList(strings.Split(input.BuildLabels, ",")), ",")
	app.ServicePort = fallbackInt(input.ServicePort, 8080)

	if err := h.db.Save(&app).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, app)
}

func (h *Handlers) DeleteApplication(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if err := h.db.Delete(&app).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "application.delete", app.ID, true, app.Name)
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) findApplication(ctx *gin.Context) (model.Application, bool) {
	var app model.Application
	err := h.db.First(
		&app,
		"id = ? and project_id = ?",
		ctx.Param("applicationId"),
		ctx.Param("projectId"),
	).Error
	if err != nil {
		writeError(ctx, http.StatusNotFound, "application not found")
		return app, false
	}
	return app, true
}

func (h *Handlers) ensureApplicationSlugAvailable(ctx *gin.Context, projectID string, slug string, excludeApplicationID string) bool {
	if slug == "" {
		writeError(ctx, http.StatusBadRequest, "应用标识不能为空")
		return false
	}
	query := h.db.Model(&model.Application{}).Where("project_id = ? and slug = ?", projectID, slug)
	if strings.TrimSpace(excludeApplicationID) != "" {
		query = query.Where("id <> ?", excludeApplicationID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if count > 0 {
		writeError(ctx, http.StatusBadRequest, "该项目空间内应用标识已存在")
		return false
	}
	return true
}

func (h *Handlers) applicationGitAccountID(ctx *gin.Context, userID string, input applicationInput) (string, bool) {
	if fallback(input.SourceType, "repository") != "repository" {
		return "", true
	}
	gitAccountID := strings.TrimSpace(input.GitAccountID)
	if gitAccountID == "" {
		return "", true
	}
	account, ok := h.findGitAccountForUser(ctx, userID, gitAccountID)
	if !ok {
		return "", false
	}
	return account.ID, true
}

type applicationInput struct {
	Slug           string `json:"slug" binding:"required"`
	Name           string `json:"name" binding:"required"`
	SourceType     string `json:"sourceType"`
	GitAccountID   string `json:"gitAccountId"`
	RepositoryURL  string `json:"repositoryUrl"`
	ImageReference string `json:"imageReference"`
	TargetImageRef string `json:"targetImageRef"`
	DockerfilePath string `json:"dockerfilePath"`
	BuildContext   string `json:"buildContext"`
	BuildLabels    string `json:"buildLabels"`
	ServicePort    int    `json:"servicePort"`
}
