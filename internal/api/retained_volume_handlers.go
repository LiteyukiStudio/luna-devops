package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (h *Handlers) ListRetainedVolumes(ctx *gin.Context) {
	if _, _, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper, authz.ProjectRoleViewer); !ok {
		return
	}
	query := h.dbFor(ctx).Model(&model.RetainedVolume{}).Where("project_id = ?", ctx.Param("projectId"))
	query = applySearch(ctx, query, "claim_name", "source_application_name", "volume_name")
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"retainedAt": "retained_at", "claimName": "claim_name"}, "retainedAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var items []model.RetainedVolume
	if err := query.Order(orderByClause(pagination, map[string]string{"retainedAt": "retained_at", "claimName": "claim_name"}, "retained_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&items).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(items, total, pagination))
}

func (h *Handlers) DeleteRetainedVolume(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	var volume model.RetainedVolume
	if err := h.dbFor(ctx).First(&volume, "id = ? and project_id = ?", ctx.Param("retainedVolumeId"), project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "retained volume not found")
		return
	}
	if volume.Status == model.RetainedVolumeStatusClaimed || volume.Status == model.RetainedVolumeStatusReserved {
		writeErrorCode(ctx, http.StatusConflict, "retained_volume.already_claimed", "保留数据卷已被新的部署认领")
		return
	}
	if volume.Status != model.RetainedVolumeStatusRetained && volume.Status != model.RetainedVolumeStatusFailed {
		writeErrorCode(ctx, http.StatusConflict, "retained_volume.state_conflict", "保留数据卷当前状态不允许删除")
		return
	}
	cluster, ok := h.runtimeClusterForEnvironment(ctx, model.Environment{ClusterID: volume.ClusterID})
	if !ok {
		return
	}
	kubeconfig := h.secrets.ResolveContext(ctx.Request.Context(), cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "retained_volume.cluster_unavailable", "运行集群连接不可用")
		return
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return
	}
	transition := h.dbFor(ctx).Model(&model.RetainedVolume{}).
		Where("id = ? and status in ?", volume.ID, []string{model.RetainedVolumeStatusRetained, model.RetainedVolumeStatusFailed}).
		Updates(map[string]any{"status": model.RetainedVolumeStatusDeleting, "last_error": ""})
	if transition.Error != nil {
		writeError(ctx, http.StatusInternalServerError, transition.Error.Error())
		return
	}
	if transition.RowsAffected != 1 {
		writeErrorCode(ctx, http.StatusConflict, "retained_volume.state_conflict", "保留数据卷状态已变化，请刷新后重试")
		return
	}
	if err := client.DeleteManagedResource(ctx.Request.Context(), "PersistentVolumeClaim", volume.Namespace, volume.ClaimName); err != nil && !apierrors.IsNotFound(err) {
		_ = h.dbFor(ctx).Model(&volume).Updates(map[string]any{"status": model.RetainedVolumeStatusFailed, "last_error": err.Error()}).Error
		writeErrorCode(ctx, http.StatusBadGateway, "retained_volume.delete_failed", "保留数据卷删除失败")
		return
	}
	if err := h.dbFor(ctx).Delete(&volume).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "retained_volume.delete", volume.ID, true, volume.ClaimName, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func reserveRetainedVolumes(tx *gorm.DB, projectID, applicationID, targetID, clusterID string, volumes []deploymentTargetDataVolumeInput) error {
	now := time.Now()
	if err := tx.Model(&model.RetainedVolume{}).
		Where("claimed_by_target_id = ? and status = ?", targetID, model.RetainedVolumeStatusReserved).
		Updates(map[string]any{
			"status":                    model.RetainedVolumeStatusRetained,
			"claimed_by_application_id": "",
			"claimed_by_target_id":      "",
			"claimed_at":                nil,
		}).Error; err != nil {
		return err
	}
	for _, item := range volumes {
		if normalizeDataVolumeSourceType(item.SourceType) != "retainedClaim" {
			continue
		}
		var current model.RetainedVolume
		if err := tx.First(&current, "id = ? and project_id = ? and cluster_id = ? and claim_name = ?", strings.TrimSpace(item.RetainedVolumeID), projectID, clusterID, strings.TrimSpace(item.ExistingClaimName)).Error; err != nil {
			return err
		}
		if (current.Status == model.RetainedVolumeStatusReserved || current.Status == model.RetainedVolumeStatusClaimed) && current.ClaimedByTargetID == targetID {
			continue
		}
		result := tx.Model(&model.RetainedVolume{}).Where("id = ? and status = ?", current.ID, model.RetainedVolumeStatusRetained).Updates(map[string]any{"status": model.RetainedVolumeStatusReserved, "claimed_by_application_id": applicationID, "claimed_by_target_id": targetID, "claimed_at": &now, "last_error": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrInvalidData
		}
	}
	return nil
}
