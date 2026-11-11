package runtimeapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	maxEgressExceptionGrants = 200
	maxEgressExceptionTarget = 50
)

type egressExceptionGrantInput struct {
	ProjectID           string   `json:"projectId"`
	DeploymentTargetIDs []string `json:"deploymentTargetIds"`
	IncludeBuilds       bool     `json:"includeBuilds"`
}

type egressExceptionInput struct {
	Name     string                      `json:"name"`
	CIDR     string                      `json:"cidr"`
	Ports    string                      `json:"ports"`
	Protocol string                      `json:"protocol"`
	Reason   string                      `json:"reason"`
	Grants   []egressExceptionGrantInput `json:"grants"`
}

type egressExceptionGrantResponse struct {
	ProjectID           string   `json:"projectId"`
	DeploymentTargetIDs []string `json:"deploymentTargetIds"`
	IncludeBuilds       bool     `json:"includeBuilds"`
}

type egressExceptionResponse struct {
	ID               string                         `json:"id"`
	RuntimeClusterID string                         `json:"runtimeClusterId"`
	Name             string                         `json:"name"`
	CIDR             string                         `json:"cidr"`
	Ports            string                         `json:"ports"`
	Protocol         string                         `json:"protocol"`
	Reason           string                         `json:"reason"`
	Grants           []egressExceptionGrantResponse `json:"grants"`
	CreatedBy        string                         `json:"createdBy"`
	CreatedAt        time.Time                      `json:"createdAt"`
	UpdatedAt        time.Time                      `json:"updatedAt"`
}

var egressExceptionSortFields = map[string]string{"name": "name", "createdAt": "created_at"}

// managedRuntimeCluster 读取活动集群并要求调用者能维护它；出站例外只能由集群维护者管理。
func (h *Handlers) managedRuntimeCluster(ctx *gin.Context) (model.User, model.RuntimeCluster, bool) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return model.User{}, model.RuntimeCluster{}, false
	}
	var cluster model.RuntimeCluster
	if err := runtimecluster.ActiveScope(h.dbFor(ctx)).Model(&model.RuntimeCluster{}).Where("id = ?", ctx.Param("clusterId")).Take(&cluster).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErrorCode(ctx, http.StatusNotFound, "runtime_cluster.not_found", "runtime cluster not found")
		} else {
			writeError(ctx, http.StatusInternalServerError, "runtime cluster lookup failed")
		}
		return model.User{}, model.RuntimeCluster{}, false
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权维护该运行集群的出站例外") {
		return model.User{}, model.RuntimeCluster{}, false
	}
	return user, cluster, true
}

func (h *Handlers) ListRuntimeClusterEgressExceptions(ctx *gin.Context) {
	_, cluster, ok := h.managedRuntimeCluster(ctx)
	if !ok {
		return
	}
	pagination := paginationFromQueryWithSort(ctx, egressExceptionSortFields, "createdAt")
	query := h.dbFor(ctx).Model(&model.NetworkEgressException{}).Where("runtime_cluster_id = ?", cluster.ID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "egress exception lookup failed")
		return
	}
	var exceptions []model.NetworkEgressException
	if err := query.Preload("Grants").Order(orderByClause(pagination, egressExceptionSortFields, "created_at")).
		Offset(pagination.Offset()).Limit(pagination.PageSize).Find(&exceptions).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "egress exception lookup failed")
		return
	}
	items := make([]egressExceptionResponse, 0, len(exceptions))
	for _, exception := range exceptions {
		items = append(items, egressExceptionResponseFrom(exception))
	}
	ctx.JSON(http.StatusOK, paginatedResponse(items, total, pagination))
}

func (h *Handlers) CreateRuntimeClusterEgressException(ctx *gin.Context) {
	user, cluster, ok := h.managedRuntimeCluster(ctx)
	if !ok {
		return
	}
	var input egressExceptionInput
	if !bindJSON(ctx, &input) {
		return
	}
	now := time.Now().UTC()
	exception := model.NetworkEgressException{ID: id.New("nex"), RuntimeClusterID: cluster.ID, CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now}
	if !h.applyEgressExceptionInput(ctx, cluster, &exception, input) {
		return
	}
	if err := h.dbFor(ctx).Create(&exception).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "egress exception save failed")
		return
	}
	h.finishEgressExceptionMutation(ctx, user, cluster, "create", exception)
	ctx.JSON(http.StatusCreated, egressExceptionResponseFrom(exception))
}

func (h *Handlers) UpdateRuntimeClusterEgressException(ctx *gin.Context) {
	user, cluster, ok := h.managedRuntimeCluster(ctx)
	if !ok {
		return
	}
	exception, ok := h.findEgressException(ctx, cluster)
	if !ok {
		return
	}
	var input egressExceptionInput
	if !bindJSON(ctx, &input) {
		return
	}
	exception.UpdatedAt = time.Now().UTC()
	if !h.applyEgressExceptionInput(ctx, cluster, &exception, input) {
		return
	}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("exception_id = ?", exception.ID).Delete(&model.NetworkEgressExceptionGrant{}).Error; err != nil {
			return err
		}
		return tx.Session(&gorm.Session{FullSaveAssociations: true}).Save(&exception).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, "egress exception save failed")
		return
	}
	h.finishEgressExceptionMutation(ctx, user, cluster, "update", exception)
	ctx.JSON(http.StatusOK, egressExceptionResponseFrom(exception))
}

func (h *Handlers) DeleteRuntimeClusterEgressException(ctx *gin.Context) {
	user, cluster, ok := h.managedRuntimeCluster(ctx)
	if !ok {
		return
	}
	exception, ok := h.findEgressException(ctx, cluster)
	if !ok {
		return
	}
	if err := h.dbFor(ctx).Delete(&model.NetworkEgressException{}, "id = ?", exception.ID).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "egress exception delete failed")
		return
	}
	h.finishEgressExceptionMutation(ctx, user, cluster, "delete", exception)
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) findEgressException(ctx *gin.Context, cluster model.RuntimeCluster) (model.NetworkEgressException, bool) {
	var exception model.NetworkEgressException
	err := h.dbFor(ctx).Preload("Grants").Where("id = ? and runtime_cluster_id = ?", ctx.Param("exceptionId"), cluster.ID).Take(&exception).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeErrorCode(ctx, http.StatusNotFound, "runtime_cluster.egress_exception_not_found", "egress exception not found")
		return exception, false
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "egress exception lookup failed")
		return exception, false
	}
	return exception, true
}

// finishEgressExceptionMutation 写审计并立即重新下发本集群的网络策略。审计只记录授权范围，不记录网段。
func (h *Handlers) finishEgressExceptionMutation(ctx *gin.Context, user model.User, cluster model.RuntimeCluster, action string, exception model.NetworkEgressException) {
	includeBuilds := false
	for _, grant := range exception.Grants {
		includeBuilds = includeBuilds || grant.IncludeBuilds
	}
	h.auditWithContext(user.ID, "runtime_cluster.egress_exception."+action, exception.ID, true,
		fmt.Sprintf("cluster=%s projects=%d includeBuilds=%t", cluster.ID, len(exception.Grants), includeBuilds), ctx.Request.Context())
	h.host.EnqueueNetworkPolicyReconcile(ctx.Request.Context(), cluster.ID)
}

// applyEgressExceptionInput 校验并写入例外规则与授权：网段不能是 /0、IPv4 映射写法或云元数据地址本身；
// 理由必填；授权的项目空间必须能使用该集群，收窄的部署配置必须属于对应项目空间。
func (h *Handlers) applyEgressExceptionInput(ctx *gin.Context, cluster model.RuntimeCluster, exception *model.NetworkEgressException, input egressExceptionInput) bool {
	invalid := func(detail string) bool {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.egress_exception_invalid", detail)
		return false
	}
	name := strings.TrimSpace(input.Name)
	reason := strings.TrimSpace(input.Reason)
	if name == "" || len(name) > 64 {
		return invalid("name must be 1-64 characters")
	}
	if reason == "" || len(reason) > 500 {
		return invalid("reason must be 1-500 characters")
	}
	prefix, err := kubeprovider.ParseNetworkCIDR(input.CIDR)
	if err != nil || prefix.Bits() == 0 {
		return invalid("cidr must be a valid IPv4 or IPv6 CIDR other than /0")
	}
	for _, metadata := range kubeprovider.CloudMetadataCIDRs() {
		if prefix == netip.MustParsePrefix(metadata) {
			return invalid("cidr must not be a cloud metadata address")
		}
	}
	// 未写端口表示该协议的全部端口；写了端口时复用私网镜像站端口的校验规则（最多 16 个）。
	ports := ""
	if strings.TrimSpace(input.Ports) != "" {
		normalized, err := normalizeBuildPrivateEgressPorts(input.Ports)
		if err != nil {
			return invalid(err.Error())
		}
		ports = normalized
	}
	protocol := strings.TrimSpace(input.Protocol)
	switch protocol {
	case kubeprovider.EgressExceptionProtocolTCP, kubeprovider.EgressExceptionProtocolUDP, kubeprovider.EgressExceptionProtocolAny:
	default:
		return invalid("protocol must be tcp, udp or any")
	}
	grants, ok := h.egressExceptionGrants(ctx, cluster, exception.ID, input.Grants)
	if !ok {
		return false
	}
	exception.Name, exception.Reason, exception.CIDR = name, reason, prefix.String()
	exception.Ports, exception.Protocol, exception.Grants = ports, protocol, grants
	return true
}

func (h *Handlers) egressExceptionGrants(ctx *gin.Context, cluster model.RuntimeCluster, exceptionID string, inputs []egressExceptionGrantInput) ([]model.NetworkEgressExceptionGrant, bool) {
	invalid := func(detail string) ([]model.NetworkEgressExceptionGrant, bool) {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.egress_exception_grant_invalid", detail)
		return nil, false
	}
	if len(inputs) > maxEgressExceptionGrants {
		return invalid(fmt.Sprintf("at most %d grants are allowed", maxEgressExceptionGrants))
	}
	bound := map[string]bool{}
	if cluster.Scope == "project" {
		for _, projectID := range h.scopedResourceProjectIDs(scopedResourceRuntimeCluster, cluster.ID, ctx.Request.Context()) {
			bound[projectID] = true
		}
	}
	seen := map[string]bool{}
	grants := make([]model.NetworkEgressExceptionGrant, 0, len(inputs))
	for _, input := range inputs {
		projectID := strings.TrimSpace(input.ProjectID)
		if projectID == "" || seen[projectID] {
			return invalid("each grant needs a distinct projectId")
		}
		seen[projectID] = true
		var project model.Project
		if err := h.dbFor(ctx).Select("id", "delete_status").Where("id = ?", projectID).Take(&project).Error; err != nil || !model.DeleteStatusActive(project.DeleteStatus) {
			return invalid(fmt.Sprintf("project %s is not an active project space", projectID))
		}
		if cluster.Scope == "project" && !bound[projectID] {
			return invalid(fmt.Sprintf("project %s cannot use this runtime cluster", projectID))
		}
		targets := normalizeStringList(input.DeploymentTargetIDs)
		if len(targets) > maxEgressExceptionTarget {
			return invalid(fmt.Sprintf("at most %d deployment targets per grant", maxEgressExceptionTarget))
		}
		if len(targets) > 0 {
			var count int64
			if err := h.dbFor(ctx).Model(&model.DeploymentTarget{}).Where("project_id = ? and id in ?", projectID, targets).Count(&count).Error; err != nil || int(count) != len(targets) {
				return invalid(fmt.Sprintf("deployment targets must belong to project %s", projectID))
			}
		}
		grants = append(grants, model.NetworkEgressExceptionGrant{
			ID: id.New("nexg"), ExceptionID: exceptionID, ProjectID: projectID,
			DeploymentTargetIDs: strings.Join(targets, ","), IncludeBuilds: input.IncludeBuilds, CreatedAt: time.Now().UTC(),
		})
	}
	return grants, true
}

func egressExceptionResponseFrom(exception model.NetworkEgressException) egressExceptionResponse {
	grants := make([]egressExceptionGrantResponse, 0, len(exception.Grants))
	for _, grant := range exception.Grants {
		grants = append(grants, egressExceptionGrantResponse{
			ProjectID: grant.ProjectID, DeploymentTargetIDs: runtimecluster.SplitIDList(grant.DeploymentTargetIDs), IncludeBuilds: grant.IncludeBuilds,
		})
	}
	return egressExceptionResponse{
		ID: exception.ID, RuntimeClusterID: exception.RuntimeClusterID, Name: exception.Name, CIDR: exception.CIDR,
		Ports: exception.Ports, Protocol: exception.Protocol, Reason: exception.Reason, Grants: grants,
		CreatedBy: exception.CreatedBy, CreatedAt: exception.CreatedAt, UpdatedAt: exception.UpdatedAt,
	}
}
