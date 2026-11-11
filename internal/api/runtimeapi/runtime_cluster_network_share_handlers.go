package runtimeapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var networkShareApprovalSortFields = map[string]string{"createdAt": "s.created_at"}

// ListRuntimeClusterNetworkShareApprovals 列出提供方部署配置位于本集群、仍待集群维护者批准的跨项目共享。
func (h *Handlers) ListRuntimeClusterNetworkShareApprovals(ctx *gin.Context) {
	_, cluster, ok := h.managedRuntimeCluster(ctx)
	if !ok {
		return
	}
	query, err := runtimecluster.ServiceSharesOnCluster(ctx.Request.Context(), h.dbFor(ctx), cluster)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	query = query.Where("s.approval_required = ? and s.approved_at is null", true)
	pagination := paginationFromQueryWithSort(ctx, networkShareApprovalSortFields, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	var shares []model.NetworkServiceShare
	if err := query.Select("s.*").Order(orderByClause(pagination, networkShareApprovalSortFields, "s.created_at") + ", s.id asc").
		Offset(pagination.Offset()).Limit(pagination.PageSize).Find(&shares).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	items, err := projectapi.NetworkServiceShareResponses(h.dbFor(ctx), shares)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(items, total, pagination))
}

// ApproveRuntimeClusterNetworkShare 由集群维护者批准持有出站例外的项目空间对外共享服务。
func (h *Handlers) ApproveRuntimeClusterNetworkShare(ctx *gin.Context) {
	user, cluster, ok := h.managedRuntimeCluster(ctx)
	if !ok {
		return
	}
	query, err := runtimecluster.ServiceSharesOnCluster(ctx.Request.Context(), h.dbFor(ctx), cluster)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	var share model.NetworkServiceShare
	if err := query.Select("s.*").Where("s.id = ? and s.approval_required = ?", ctx.Param("shareId"), true).Take(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErrorCode(ctx, http.StatusNotFound, "network_share.not_found", "network share not found")
		} else {
			writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		}
		return
	}
	if share.ApprovedAt == nil {
		now := time.Now().UTC()
		result := h.dbFor(ctx).Model(&model.NetworkServiceShare{}).Where("id = ? and approved_at is null", share.ID).
			Updates(map[string]any{"approved_at": now, "approved_by": user.ID, "updated_at": now})
		if result.Error != nil {
			writeError(ctx, http.StatusInternalServerError, "network share save failed")
			return
		}
		if result.RowsAffected == 0 {
			writeErrorCode(ctx, http.StatusNotFound, "network_share.not_found", "network share not found")
			return
		}
		share.ApprovedAt, share.ApprovedBy, share.UpdatedAt = &now, user.ID, now
		h.auditWithContext(user.ID, "runtime_cluster.network_share.approve", share.ID, true,
			fmt.Sprintf("cluster=%s target=%s provider=%s consumer=%s", cluster.ID, share.DeploymentTargetID, share.ProviderProjectID, share.ConsumerProjectID), ctx.Request.Context())
		h.host.EnqueueNetworkPolicyReconcile(ctx.Request.Context(), cluster.ID)
	}
	items, err := projectapi.NetworkServiceShareResponses(h.dbFor(ctx), []model.NetworkServiceShare{share})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	ctx.JSON(http.StatusOK, items[0])
}
