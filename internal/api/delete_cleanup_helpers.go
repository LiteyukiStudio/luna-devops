package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func deleteStatusCanStart(status string) bool {
	status = strings.TrimSpace(status)
	return status == "" || status == "active" || status == "delete_failed"
}

func resourceCanMutateDuringDelete(status string) bool {
	status = strings.TrimSpace(status)
	return status == "" || status == "active" || status == "delete_failed"
}

func (h *Handlers) ensureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	if resourceCanMutateDuringDelete(project.DeleteStatus) {
		return true
	}
	writeErrorCode(ctx, http.StatusConflict, "project.delete_in_progress", "项目空间正在删除中，请等待资源清理完成")
	return false
}

func (h *Handlers) ensureDeploymentTargetCanMutate(ctx *gin.Context, target model.DeploymentTarget) bool {
	if resourceCanMutateDuringDelete(target.DeleteStatus) {
		return true
	}
	writeErrorCode(ctx, http.StatusConflict, "deployment_target.delete_in_progress", "部署配置正在删除中，请等待资源清理完成")
	return false
}

func (h *Handlers) ensureGatewayRouteCanMutate(ctx *gin.Context, route model.GatewayRoute) bool {
	if resourceCanMutateDuringDelete(route.DeleteStatus) {
		return true
	}
	writeErrorCode(ctx, http.StatusConflict, "gateway_route.delete_in_progress", "访问入口正在删除中，请等待资源清理完成")
	return false
}

func (h *Handlers) ensureRuntimeConfigSetCanMutate(ctx *gin.Context, set model.ProjectRuntimeConfigSet) bool {
	if resourceCanMutateDuringDelete(set.DeleteStatus) {
		return true
	}
	writeErrorCode(ctx, http.StatusConflict, "runtime_config.delete_in_progress", "运行配置正在删除中，请等待资源清理完成")
	return false
}

func markResourceDeleting(tx *gorm.DB, model any, id string) error {
	startedAt := time.Now()
	return tx.Model(model).Where("id = ?", id).Updates(map[string]any{
		"delete_status":      "deleting",
		"delete_message":     "",
		"delete_started_at":  &startedAt,
		"delete_finished_at": nil,
	}).Error
}

func markDeploymentTargetGatewayRoutesDeleting(tx *gorm.DB, target model.DeploymentTarget) error {
	startedAt := time.Now()
	return tx.Model(&model.GatewayRoute{}).
		Where("project_id = ? and application_id = ? and deployment_target_id = ? and delete_status in ?",
			target.ProjectID,
			target.ApplicationID,
			target.ID,
			[]string{"", "active", "delete_failed"},
		).
		Updates(map[string]any{
			"delete_status":      "deleting",
			"delete_message":     "",
			"delete_started_at":  &startedAt,
			"delete_finished_at": nil,
		}).Error
}

func markResourceDeleteFailed(db *gorm.DB, model any, id string, message string) error {
	finishedAt := time.Now()
	return db.Model(model).Where("id = ?", id).Updates(map[string]any{
		"delete_status":      "delete_failed",
		"delete_message":     strings.TrimSpace(message),
		"delete_finished_at": &finishedAt,
	}).Error
}

func markDeploymentTargetGatewayRoutesDeleteFailed(db *gorm.DB, target model.DeploymentTarget, message string) error {
	finishedAt := time.Now()
	return db.Model(&model.GatewayRoute{}).
		Where("project_id = ? and application_id = ? and deployment_target_id = ? and delete_status = ?",
			target.ProjectID,
			target.ApplicationID,
			target.ID,
			"deleting",
		).
		Updates(map[string]any{
			"delete_status":      "delete_failed",
			"delete_message":     strings.TrimSpace(message),
			"delete_finished_at": &finishedAt,
		}).Error
}

func (h *Handlers) enqueueResourceCleanup(ctx context.Context, payload tasks.ResourceCleanupPayload) bool {
	if h.taskClient == nil {
		return false
	}
	_, err := h.taskClient.EnqueueResourceCleanup(ctx, payload)
	return err == nil
}
