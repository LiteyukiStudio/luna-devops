package projectapi

import (
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
)

// projectNetworkPolicy 是项目空间网络模式；只能比集群默认更严格，因此项目管理员即可修改。
type projectNetworkPolicy struct {
	IngressMode     string `json:"ingressMode"`
	EgressMode      string `json:"egressMode"`
	EgressAllowlist string `json:"egressAllowlist"`
}

func (h *Handler) GetProjectNetworkPolicy(ctx *gin.Context) {
	_, project, ok := h.authorizeProject(ctx, authz.ActionProjectRead)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, projectNetworkPolicy{
		IngressMode:     project.NetworkIngressMode,
		EgressMode:      project.NetworkEgressMode,
		EgressAllowlist: project.NetworkEgressAllowlist,
	})
}

func (h *Handler) UpdateProjectNetworkPolicy(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionProjectManage)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var input projectNetworkPolicy
	if !bindJSON(ctx, &input) {
		return
	}
	switch input.IngressMode {
	case kubeprovider.NetworkIngressIsolated, kubeprovider.NetworkIngressDependencies:
	default:
		writeErrorCode(ctx, http.StatusBadRequest, "project.network_ingress_mode_invalid", "ingress mode must be isolated or dependencies")
		return
	}
	switch input.EgressMode {
	case kubeprovider.NetworkEgressInternet, kubeprovider.NetworkEgressAllowlist, kubeprovider.NetworkEgressNone:
	default:
		writeErrorCode(ctx, http.StatusBadRequest, "project.network_egress_mode_invalid", "egress mode must be internet, allowlist or none")
		return
	}
	allowlist, err := normalizeEgressAllowlist(input.EgressAllowlist)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "project.network_egress_allowlist_invalid", err.Error())
		return
	}
	if err := h.dbFor(ctx).Model(&project).Updates(map[string]any{
		"network_ingress_mode":     input.IngressMode,
		"network_egress_mode":      input.EgressMode,
		"network_egress_allowlist": allowlist,
	}).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "project network policy update failed")
		return
	}
	entries := 0
	if allowlist != "" {
		entries = strings.Count(allowlist, "\n") + 1
	}
	// 审计只记录模式与条目数，不记录具体网段。
	h.auditWithContext(user.ID, "project.network_policy.update", project.ID, true,
		fmt.Sprintf("ingress=%s egress=%s allowlistEntries=%d", input.IngressMode, input.EgressMode, entries), ctx.Request.Context())
	h.host.EnqueueNetworkPolicyProjectReconcile(ctx.Request.Context(), project.ID)
	ctx.JSON(http.StatusOK, projectNetworkPolicy{IngressMode: input.IngressMode, EgressMode: input.EgressMode, EgressAllowlist: allowlist})
}

// normalizeEgressAllowlist 校验并规范化出站白名单；整体落在平台始终拒绝的网段内的条目不可能生效，直接拒绝。
func normalizeEgressAllowlist(raw string) (string, error) {
	rules, err := runtimecluster.ParseEgressAllowlist(raw)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(rules))
	for _, rule := range rules {
		if kubeprovider.WithinPlatformDeniedRange(netip.MustParsePrefix(rule.CIDR)) {
			return "", fmt.Errorf("egress allowlist CIDR %s is inside a private or reserved range that is always denied", rule.CIDR)
		}
		line := rule.CIDR
		if len(rule.Ports) > 0 {
			ports := make([]string, 0, len(rule.Ports))
			for _, port := range rule.Ports {
				ports = append(ports, strconv.Itoa(port))
			}
			line += " " + strings.Join(ports, ",")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}
