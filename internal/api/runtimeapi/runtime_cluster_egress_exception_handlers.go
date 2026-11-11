package runtimeapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

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
	ProjectName         string   `json:"projectName"`
	DeploymentTargetIDs []string `json:"deploymentTargetIds"`
	IncludeBuilds       bool     `json:"includeBuilds"`
	// ProvidedShares 是该项目空间对外提供的跨项目共享数，用于提示例外可能被共享服务当作跳板。
	ProvidedShares int `json:"providedShares"`
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
	ctx.JSON(http.StatusOK, paginatedResponse(h.egressExceptionResponses(ctx, exceptions), total, pagination))
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
	ctx.JSON(http.StatusCreated, h.egressExceptionResponses(ctx, []model.NetworkEgressException{exception})[0])
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
		// 用 Updates 并检查影响行数：并发删除后不能像 Save 那样回退为插入而“复活”例外。
		result := tx.Model(&model.NetworkEgressException{}).Where("id = ? and runtime_cluster_id = ?", exception.ID, cluster.ID).Updates(map[string]any{
			"name": exception.Name, "cidr": exception.CIDR, "ports": exception.Ports, "protocol": exception.Protocol,
			"reason": exception.Reason, "updated_at": exception.UpdatedAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Where("exception_id = ?", exception.ID).Delete(&model.NetworkEgressExceptionGrant{}).Error; err != nil {
			return err
		}
		if len(exception.Grants) == 0 {
			return nil
		}
		return tx.Create(&exception.Grants).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErrorCode(ctx, http.StatusNotFound, "runtime_cluster.egress_exception_not_found", "egress exception not found")
			return
		}
		writeError(ctx, http.StatusInternalServerError, "egress exception save failed")
		return
	}
	h.finishEgressExceptionMutation(ctx, user, cluster, "update", exception)
	ctx.JSON(http.StatusOK, h.egressExceptionResponses(ctx, []model.NetworkEgressException{exception})[0])
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
	// 每类字段使用独立错误码，前端据此把错误标到对应字段上。
	invalid := func(code, detail string) bool {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.egress_exception_"+code+"_invalid", detail)
		return false
	}
	name := strings.TrimSpace(input.Name)
	reason := strings.TrimSpace(input.Reason)
	// 长度按字符计算，与前端校验和文案“1–64 字”一致。
	if count := utf8.RuneCountInString(name); count == 0 || count > 64 {
		return invalid("name", "name must be 1-64 characters")
	}
	if count := utf8.RuneCountInString(reason); count == 0 || count > 500 {
		return invalid("reason", "reason must be 1-500 characters")
	}
	prefix, err := kubeprovider.ParseNetworkCIDR(input.CIDR)
	if err != nil || prefix.Bits() == 0 {
		return invalid("cidr", "cidr must be a valid IPv4 or IPv6 CIDR other than /0")
	}
	for _, metadata := range kubeprovider.CloudMetadataCIDRs() {
		if prefix == netip.MustParsePrefix(metadata) {
			return invalid("cidr", "cidr must not be a cloud metadata address")
		}
	}
	// 未写端口表示该协议的全部端口；写了端口时复用私网镜像站端口的校验规则（最多 16 个）。
	ports := ""
	if strings.TrimSpace(input.Ports) != "" {
		normalized, err := normalizeBuildPrivateEgressPorts(input.Ports)
		if err != nil {
			return invalid("ports", err.Error())
		}
		ports = normalized
	}
	protocol := strings.TrimSpace(input.Protocol)
	switch protocol {
	case kubeprovider.EgressExceptionProtocolTCP, kubeprovider.EgressExceptionProtocolUDP, kubeprovider.EgressExceptionProtocolAny:
	default:
		return invalid("protocol", "protocol must be tcp, udp or any")
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
	// 只能授权给实际能在该集群部署的项目空间：项目级集群看绑定，个人集群要求集群所有者是项目成员。
	// 不存在、已删除与无权使用统一返回同一错误，避免借此探测项目 ID。
	usable := func(projectID string) bool {
		var project model.Project
		if err := h.dbFor(ctx).Select("id", "delete_status").Where("id = ?", projectID).Take(&project).Error; err != nil || !model.DeleteStatusActive(project.DeleteStatus) {
			return false
		}
		switch cluster.Scope {
		case "global":
			return true
		case "project":
			return bound[projectID]
		case "user":
			var members int64
			if err := h.dbFor(ctx).Model(&model.ProjectMember{}).Where("project_id = ? and user_id = ?", projectID, cluster.OwnerRef).Count(&members).Error; err != nil {
				return false
			}
			return members > 0
		}
		return false
	}
	seen := map[string]bool{}
	grants := make([]model.NetworkEgressExceptionGrant, 0, len(inputs))
	for _, input := range inputs {
		projectID := strings.TrimSpace(input.ProjectID)
		if projectID == "" || seen[projectID] {
			return invalid("each grant needs a distinct projectId")
		}
		seen[projectID] = true
		if !usable(projectID) {
			return invalid(fmt.Sprintf("project %s is unavailable for this runtime cluster", projectID))
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

// egressExceptionResponses 附带授权项目空间的名称，界面不必加载全部项目空间来显示授权。
func (h *Handlers) egressExceptionResponses(ctx *gin.Context, exceptions []model.NetworkEgressException) []egressExceptionResponse {
	projectIDs := []string{}
	for _, exception := range exceptions {
		for _, grant := range exception.Grants {
			projectIDs = append(projectIDs, grant.ProjectID)
		}
	}
	names := map[string]string{}
	if len(projectIDs) > 0 {
		var projects []model.Project
		if err := h.dbFor(ctx).Select("id", "name").Where("id in ?", normalizeStringList(projectIDs)).Find(&projects).Error; err == nil {
			for _, project := range projects {
				names[project.ID] = project.Name
			}
		}
	}
	shares := map[string]int{}
	if len(projectIDs) > 0 {
		var counts []struct {
			ProjectID string `gorm:"column:project_id"`
			Count     int    `gorm:"column:count"`
		}
		if err := h.dbFor(ctx).Model(&model.NetworkServiceShare{}).Select("provider_project_id as project_id, count(*) as count").
			Where("provider_project_id in ?", normalizeStringList(projectIDs)).Group("provider_project_id").Scan(&counts).Error; err == nil {
			for _, row := range counts {
				shares[row.ProjectID] = row.Count
			}
		}
	}
	items := make([]egressExceptionResponse, 0, len(exceptions))
	for _, exception := range exceptions {
		item := egressExceptionResponseFrom(exception, names)
		for index := range item.Grants {
			item.Grants[index].ProvidedShares = shares[item.Grants[index].ProjectID]
		}
		items = append(items, item)
	}
	return items
}

func egressExceptionResponseFrom(exception model.NetworkEgressException, projectNames map[string]string) egressExceptionResponse {
	grants := make([]egressExceptionGrantResponse, 0, len(exception.Grants))
	for _, grant := range exception.Grants {
		grants = append(grants, egressExceptionGrantResponse{
			ProjectID: grant.ProjectID, ProjectName: projectNames[grant.ProjectID],
			DeploymentTargetIDs: runtimecluster.SplitIDList(grant.DeploymentTargetIDs), IncludeBuilds: grant.IncludeBuilds,
		})
	}
	return egressExceptionResponse{
		ID: exception.ID, RuntimeClusterID: exception.RuntimeClusterID, Name: exception.Name, CIDR: exception.CIDR,
		Ports: exception.Ports, Protocol: exception.Protocol, Reason: exception.Reason, Grants: grants,
		CreatedBy: exception.CreatedBy, CreatedAt: exception.CreatedAt, UpdatedAt: exception.UpdatedAt,
	}
}
