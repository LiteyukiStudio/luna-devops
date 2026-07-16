package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) GetRegistryImageTemplateDefault(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}
	project, ok := h.findProjectForCurrentUserWithRolesByID(ctx, strings.TrimSpace(ctx.Query("projectId")), "owner", "admin", "developer")
	if !ok {
		return
	}
	var app model.Application
	if err := h.db.First(&app, "id = ? and project_id = ?", strings.TrimSpace(ctx.Query("applicationId")), project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "应用不存在")
		return
	}
	target := model.DeploymentTarget{
		Name:  strings.TrimSpace(ctx.Query("targetName")),
		Stage: normalizeStage(ctx.Query("stage")),
	}
	repository := repositoryWithoutRegistryHost(registry, buildTargetImageRepository(registry, project, app))
	tag := "latest"
	if credential, ok := h.registryPushCredentialForProject(user, registry, project.ID); ok {
		templatedRepository, _ := splitTargetImageRef(buildTargetImageRepositoryForCredential(registry, credential, project, app, target))
		repository = repositoryWithoutRegistryHost(registry, templatedRepository)
		tag = buildStaticTargetImageTagForCredential(registry, credential, project, app, target)
	}
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	tag = fallback(strings.TrimSpace(tag), "latest")
	ctx.JSON(http.StatusOK, registryImageTemplateDefaultOutput{
		TargetImageRef:   repository + ":" + tag,
		TargetRepository: repository,
		TargetTag:        tag,
	})
}
