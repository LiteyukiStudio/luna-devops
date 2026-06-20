package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
)

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
	return nextReleaseRevisionFor(h.db, source.ProjectID, source.ApplicationID, source.EnvironmentID)
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

func (h *Handlers) validateReleaseForCreate(ctx *gin.Context, release *model.Release) bool {
	var application model.Application
	if err := h.db.First(&application, "id = ? and project_id = ?", release.ApplicationID, release.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "应用不存在或不属于当前项目空间")
		return false
	}
	if !applicationCanMutate(application) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能创建发布")
		return false
	}
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ? and application_id = ? and enabled = ?", strings.TrimSpace(release.DeploymentTargetID), release.ProjectID, release.ApplicationID, true).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "部署配置不存在或不可用")
		return false
	}
	if !h.ensureDeploymentTargetCanMutate(ctx, target) {
		return false
	}
	release.EnvironmentID = target.EnvironmentID
	if strings.TrimSpace(release.BuildRunID) == "" {
		if strings.TrimSpace(release.ImageRef) == "" {
			release.ImageRef = strings.TrimSpace(target.ImageRef)
		}
		if strings.TrimSpace(release.ImageRef) == "" {
			writeError(ctx, http.StatusBadRequest, "发布镜像不能为空")
			return false
		}
		return true
	}
	var run model.BuildRun
	if err := h.db.First(&run, "id = ? and project_id = ? and application_id = ?", release.BuildRunID, release.ProjectID, release.ApplicationID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "构建产物不存在或不属于当前应用")
		return false
	}
	if run.Status != "succeeded" {
		writeError(ctx, http.StatusBadRequest, "只能发布成功构建产物")
		return false
	}
	if run.DeploymentTargetID != release.DeploymentTargetID {
		writeError(ctx, http.StatusBadRequest, "构建产物不属于当前部署配置")
		return false
	}
	imageRef := strings.TrimSpace(run.ImageRef)
	if imageRef == "" && strings.TrimSpace(run.TargetRegistryID) != "" {
		var registry model.ArtifactRegistry
		if err := h.db.First(&registry, "id = ?", run.TargetRegistryID).Error; err == nil {
			imageRef = buildImageRef(registry, run)
		}
	}
	if imageRef == "" {
		writeError(ctx, http.StatusBadRequest, "构建产物缺少镜像引用")
		return false
	}
	if strings.TrimSpace(release.ImageRef) != "" && strings.TrimSpace(release.ImageRef) != imageRef {
		writeError(ctx, http.StatusBadRequest, "发布镜像必须与所选构建产物一致")
		return false
	}
	release.ImageRef = imageRef
	return true
}

func (h *Handlers) findRelease(ctx *gin.Context) (model.Release, bool) {
	var release model.Release
	if err := h.db.First(&release, "id = ? and project_id = ?", ctx.Param("releaseId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "release not found")
		return release, false
	}
	return release, true
}

func releaseFromInput(projectID, userID string, input releaseInput, releaseID string) model.Release {
	return model.Release{
		ID:                 releaseID,
		ProjectID:          projectID,
		ApplicationID:      strings.TrimSpace(input.ApplicationID),
		EnvironmentID:      strings.TrimSpace(input.EnvironmentID),
		DeploymentTargetID: strings.TrimSpace(input.DeploymentTargetID),
		BuildRunID:         strings.TrimSpace(input.BuildRunID),
		ImageRef:           strings.TrimSpace(input.ImageRef),
		ForceImagePull:     input.ForceImagePull,
		Type:               normalizeReleaseType(input.Type),
		Status:             fallback(strings.TrimSpace(input.Status), "pending"),
		Revision:           fallbackInt(input.Revision, 1),
		Message:            strings.TrimSpace(input.Message),
		CreatedBy:          userID,
	}
}

func rollbackReleaseFromTarget(source model.Release, target model.Release, userID string, revision int) model.Release {
	return model.Release{
		ProjectID:          source.ProjectID,
		ApplicationID:      source.ApplicationID,
		EnvironmentID:      source.EnvironmentID,
		DeploymentTargetID: source.DeploymentTargetID,
		BuildRunID:         target.BuildRunID,
		ImageRef:           target.ImageRef,
		Type:               "rollback",
		Status:             "pending",
		Revision:           fallbackInt(revision, source.Revision+1),
		RollbackFromID:     target.ID,
		CreatedBy:          userID,
	}
}

func normalizeReleaseType(value string) string {
	if strings.ToLower(strings.TrimSpace(value)) == "rollback" {
		return "rollback"
	}
	return "deploy"
}

type releaseInput struct {
	ApplicationID      string `json:"applicationId" binding:"required"`
	EnvironmentID      string `json:"environmentId"`
	DeploymentTargetID string `json:"deploymentTargetId" binding:"required"`
	BuildRunID         string `json:"buildRunId"`
	ImageRef           string `json:"imageRef"`
	ForceImagePull     bool   `json:"forceImagePull"`
	Type               string `json:"type"`
	Status             string `json:"status"`
	Revision           int    `json:"revision"`
	Message            string `json:"message"`
}
