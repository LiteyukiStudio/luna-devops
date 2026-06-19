package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListReleases(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	query := h.db.Where("project_id = ?", ctx.Param("projectId")).Order("created_at desc")
	if environmentID := strings.TrimSpace(ctx.Query("environmentId")); environmentID != "" {
		query = query.Where("environment_id = ?", environmentID)
	}
	if targetID := strings.TrimSpace(ctx.Query("deploymentTargetId")); targetID != "" {
		query = query.Where("deployment_target_id = ?", targetID)
	}
	var releases []model.Release
	if err := query.Find(&releases).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, releases)
}

func (h *Handlers) CreateRelease(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var input releaseInput
	if !bindJSON(ctx, &input) {
		return
	}
	release := releaseFromInput(ctx.Param("projectId"), user.ID, input, "")
	if !h.validateReleaseForCreate(ctx, &release) {
		return
	}
	if !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
		return
	}
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
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	source, ok := h.findRelease(ctx)
	if !ok {
		return
	}
	var sourceTarget model.DeploymentTarget
	if err := h.db.First(&sourceTarget, "id = ? and project_id = ? and application_id = ?", source.DeploymentTargetID, source.ProjectID, source.ApplicationID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "部署配置不存在或不可用")
		return
	}
	if !h.ensureDeploymentTargetCanMutate(ctx, sourceTarget) {
		return
	}
	if !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
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

func (h *Handlers) GetReleaseLogs(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	release, ok := h.findRelease(ctx)
	if !ok {
		return
	}
	var log model.ReleaseLog
	err := h.db.First(&log, "release_id = ? and project_id = ?", release.ID, release.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		ctx.JSON(http.StatusOK, model.ReleaseLog{
			ReleaseID: release.ID,
			ProjectID: release.ProjectID,
			Content:   "",
		})
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, log)
}
