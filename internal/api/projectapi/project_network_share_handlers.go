package projectapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// maxNetworkShareConsumers 限制单个部署配置的使用方数量，避免共享策略无限增长。
const maxNetworkShareConsumers = 50

const (
	NetworkShareStatusPendingAcceptance = "pending_acceptance"
	NetworkShareStatusPendingApproval   = "pending_approval"
	NetworkShareStatusActive            = "active"
)

var networkShareSortFields = map[string]string{"createdAt": "created_at"}

type networkShareInput struct {
	DeploymentTargetID        string `json:"deploymentTargetId"`
	ConsumerProjectIdentifier string `json:"consumerProjectIdentifier"`
}

// NetworkServiceShareResponse 是跨项目共享的展示视图；Address 是使用方在集群内访问提供方服务的地址。
type NetworkServiceShareResponse struct {
	ID                   string     `json:"id"`
	ProviderProjectID    string     `json:"providerProjectId"`
	ProviderProjectName  string     `json:"providerProjectName"`
	DeploymentTargetID   string     `json:"deploymentTargetId"`
	DeploymentTargetName string     `json:"deploymentTargetName"`
	ApplicationName      string     `json:"applicationName"`
	ConsumerProjectID    string     `json:"consumerProjectId"`
	ConsumerProjectName  string     `json:"consumerProjectName"`
	Address              string     `json:"address"`
	Status               string     `json:"status"`
	ApprovalRequired     bool       `json:"approvalRequired"`
	AcceptedAt           *time.Time `json:"acceptedAt"`
	ApprovedAt           *time.Time `json:"approvedAt"`
	CreatedAt            time.Time  `json:"createdAt"`
}

// NetworkServiceShareStatus 先看使用方是否确认，再看是否待集群维护者批准。
func NetworkServiceShareStatus(share model.NetworkServiceShare) string {
	switch {
	case share.AcceptedAt == nil:
		return NetworkShareStatusPendingAcceptance
	case share.ApprovalRequired && share.ApprovedAt == nil:
		return NetworkShareStatusPendingApproval
	default:
		return NetworkShareStatusActive
	}
}

// NetworkServiceShareResponses 批量补全项目空间、部署配置与应用名称。
func NetworkServiceShareResponses(db *gorm.DB, shares []model.NetworkServiceShare) ([]NetworkServiceShareResponse, error) {
	projectIDs, targetIDs := []string{}, []string{}
	for _, share := range shares {
		projectIDs = append(projectIDs, share.ProviderProjectID, share.ConsumerProjectID)
		targetIDs = append(targetIDs, share.DeploymentTargetID)
	}
	projects := map[string]model.Project{}
	targets := map[string]model.DeploymentTarget{}
	apps := map[string]string{}
	if len(shares) > 0 {
		var projectRows []model.Project
		if err := db.Select("id", "name", "kubernetes_namespace").Where("id in ?", projectIDs).Find(&projectRows).Error; err != nil {
			return nil, err
		}
		for _, project := range projectRows {
			projects[project.ID] = project
		}
		var targetRows []model.DeploymentTarget
		if err := db.Select("id", "application_id", "name", "kubernetes_name").Where("id in ?", targetIDs).Find(&targetRows).Error; err != nil {
			return nil, err
		}
		appIDs := []string{}
		for _, target := range targetRows {
			targets[target.ID] = target
			appIDs = append(appIDs, target.ApplicationID)
		}
		var appRows []model.Application
		if len(appIDs) > 0 {
			if err := db.Select("id", "name").Where("id in ?", appIDs).Find(&appRows).Error; err != nil {
				return nil, err
			}
		}
		for _, app := range appRows {
			apps[app.ID] = app.Name
		}
	}
	items := make([]NetworkServiceShareResponse, 0, len(shares))
	for _, share := range shares {
		provider := projects[share.ProviderProjectID]
		target := targets[share.DeploymentTargetID]
		address := ""
		if target.KubernetesName != "" && provider.KubernetesNamespace != "" {
			address = target.KubernetesName + "." + provider.KubernetesNamespace + ".svc"
		}
		items = append(items, NetworkServiceShareResponse{
			ID: share.ID, ProviderProjectID: share.ProviderProjectID, ProviderProjectName: provider.Name,
			DeploymentTargetID: share.DeploymentTargetID, DeploymentTargetName: target.Name, ApplicationName: apps[target.ApplicationID],
			ConsumerProjectID: share.ConsumerProjectID, ConsumerProjectName: projects[share.ConsumerProjectID].Name,
			Address: address, Status: NetworkServiceShareStatus(share), ApprovalRequired: share.ApprovalRequired,
			AcceptedAt: share.AcceptedAt, ApprovedAt: share.ApprovedAt, CreatedAt: share.CreatedAt,
		})
	}
	return items, nil
}

// ListProjectNetworkShares 列出项目空间提供（direction=provided）或可使用（direction=consumed）的跨项目共享。
func (h *Handler) ListProjectNetworkShares(ctx *gin.Context) {
	_, project, ok := h.authorizeProject(ctx, authz.ActionProjectRead)
	if !ok {
		return
	}
	column := ""
	switch ctx.Query("direction") {
	case "provided":
		column = "provider_project_id"
	case "consumed":
		column = "consumer_project_id"
	default:
		writeErrorCode(ctx, http.StatusBadRequest, "network_share.direction_invalid", "direction must be provided or consumed")
		return
	}
	pagination := paginationFromQueryWithSort(ctx, networkShareSortFields, "createdAt")
	query := h.dbFor(ctx).Model(&model.NetworkServiceShare{}).Where(column+" = ?", project.ID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	var shares []model.NetworkServiceShare
	if err := query.Order(orderByClause(pagination, networkShareSortFields, "created_at") + ", id asc").
		Offset(pagination.Offset()).Limit(pagination.PageSize).Find(&shares).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	items, err := NetworkServiceShareResponses(h.dbFor(ctx), shares)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(items, total, pagination))
}

// CreateProjectNetworkShare 把本项目空间的一个部署配置开放给另一个项目空间；使用方确认后才生效。
// 提供方在部署配置所在集群持有出站例外时，共享还需该集群维护者批准，防止被当作访问内网的跳板。
func (h *Handler) CreateProjectNetworkShare(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionProjectManage)
	if !ok || !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var input networkShareInput
	if !bindJSON(ctx, &input) {
		return
	}
	db := h.dbFor(ctx)
	var target model.DeploymentTarget
	if err := db.Select("id", "project_id", "cluster_id", "delete_status").
		Where("id = ? and project_id = ?", strings.TrimSpace(input.DeploymentTargetID), project.ID).Take(&target).Error; err != nil || !model.DeleteStatusActive(target.DeleteStatus) {
		writeErrorCode(ctx, http.StatusBadRequest, "network_share.target_invalid", "deployment target must be an active target of this project")
		return
	}
	clusterID, err := runtimecluster.TargetRuntimeClusterID(db, target.ClusterID)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "network_share.target_invalid", "deployment target has no available runtime cluster")
		return
	}
	// 不存在、已删除与提供方自身统一返回同一错误。
	var consumer model.Project
	if err := db.Select("id", "delete_status").Where("identifier = ?", strings.TrimSpace(input.ConsumerProjectIdentifier)).Take(&consumer).Error; err != nil ||
		!model.DeleteStatusActive(consumer.DeleteStatus) || consumer.ID == project.ID {
		writeErrorCode(ctx, http.StatusBadRequest, "network_share.consumer_invalid", "consumer must be another active project space")
		return
	}
	var existing int64
	if err := db.Model(&model.NetworkServiceShare{}).Where("deployment_target_id = ?", target.ID).Count(&existing).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	if existing >= maxNetworkShareConsumers {
		writeErrorCode(ctx, http.StatusBadRequest, "network_share.limit_exceeded", fmt.Sprintf("a deployment target can be shared with at most %d project spaces", maxNetworkShareConsumers))
		return
	}
	approvalRequired, err := runtimecluster.ProjectHoldsEgressExceptions(db, clusterID, project.ID)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "egress exception lookup failed")
		return
	}
	now := time.Now().UTC()
	share := model.NetworkServiceShare{
		ID: id.New("nss"), ProviderProjectID: project.ID, DeploymentTargetID: target.ID, ConsumerProjectID: consumer.ID,
		ApprovalRequired: approvalRequired, CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now,
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&share)
	if result.Error != nil {
		writeError(ctx, http.StatusInternalServerError, "network share save failed")
		return
	}
	if result.RowsAffected == 0 {
		writeErrorCode(ctx, http.StatusConflict, "network_share.duplicate", "the deployment target is already shared with this project space")
		return
	}
	h.auditWithContext(user.ID, "project.network_share.create", share.ID, true,
		fmt.Sprintf("target=%s consumer=%s approvalRequired=%t %s", target.ID, consumer.ID, approvalRequired, runtimecluster.EgressExceptionAuditMarker(db, project.ID)), ctx.Request.Context())
	h.writeNetworkShare(ctx, http.StatusCreated, share)
}

// AcceptProjectNetworkShare 由使用方确认使用共享；确认后（且无需批准或已批准）两侧放行策略生效。
func (h *Handler) AcceptProjectNetworkShare(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionProjectManage)
	if !ok || !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	db := h.dbFor(ctx)
	share, ok := h.findNetworkShare(ctx, db.Where("consumer_project_id = ?", project.ID))
	if !ok {
		return
	}
	if share.AcceptedAt == nil {
		now := time.Now().UTC()
		result := db.Model(&model.NetworkServiceShare{}).Where("id = ? and accepted_at is null", share.ID).
			Updates(map[string]any{"accepted_at": now, "accepted_by": user.ID, "updated_at": now})
		if result.Error != nil {
			writeError(ctx, http.StatusInternalServerError, "network share save failed")
			return
		}
		if result.RowsAffected == 0 {
			writeErrorCode(ctx, http.StatusNotFound, "network_share.not_found", "network share not found")
			return
		}
		share.AcceptedAt, share.AcceptedBy, share.UpdatedAt = &now, user.ID, now
		h.auditWithContext(user.ID, "project.network_share.accept", share.ID, true,
			fmt.Sprintf("target=%s provider=%s", share.DeploymentTargetID, share.ProviderProjectID), ctx.Request.Context())
		h.reconcileNetworkShareProjects(ctx, share)
	}
	h.writeNetworkShare(ctx, http.StatusOK, share)
}

// DeleteProjectNetworkShare 由提供方撤销或使用方退出共享，两侧放行策略随之删除。
func (h *Handler) DeleteProjectNetworkShare(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionProjectManage)
	if !ok || !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	db := h.dbFor(ctx)
	share, ok := h.findNetworkShare(ctx, db.Where("provider_project_id = ? or consumer_project_id = ?", project.ID, project.ID))
	if !ok {
		return
	}
	if err := db.Delete(&model.NetworkServiceShare{}, "id = ?", share.ID).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share delete failed")
		return
	}
	role := "consumer"
	if share.ProviderProjectID == project.ID {
		role = "provider"
	}
	h.auditWithContext(user.ID, "project.network_share.delete", share.ID, true,
		fmt.Sprintf("target=%s provider=%s consumer=%s role=%s", share.DeploymentTargetID, share.ProviderProjectID, share.ConsumerProjectID, role), ctx.Request.Context())
	h.reconcileNetworkShareProjects(ctx, share)
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) findNetworkShare(ctx *gin.Context, scoped *gorm.DB) (model.NetworkServiceShare, bool) {
	var share model.NetworkServiceShare
	err := scoped.Where("id = ?", ctx.Param("shareId")).Take(&share).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeErrorCode(ctx, http.StatusNotFound, "network_share.not_found", "network share not found")
		return share, false
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return share, false
	}
	return share, true
}

func (h *Handler) reconcileNetworkShareProjects(ctx *gin.Context, share model.NetworkServiceShare) {
	h.host.EnqueueNetworkPolicyProjectReconcile(ctx.Request.Context(), share.ProviderProjectID)
	h.host.EnqueueNetworkPolicyProjectReconcile(ctx.Request.Context(), share.ConsumerProjectID)
}

func (h *Handler) writeNetworkShare(ctx *gin.Context, status int, share model.NetworkServiceShare) {
	items, err := NetworkServiceShareResponses(h.dbFor(ctx), []model.NetworkServiceShare{share})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "network share lookup failed")
		return
	}
	ctx.JSON(status, items[0])
}
