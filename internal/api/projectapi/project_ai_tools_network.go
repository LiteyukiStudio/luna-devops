package projectapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type networkAllowlistEntryInput struct {
	CIDR     string `json:"cidr" desc:"公网 CIDR 网段（严禁 /0 与私网/保留地址）"`
	Ports    []int  `json:"ports,omitempty" max:"16" desc:"允许访问的 TCP 端口列表（1..65535，最多 16 个；与 allPorts: true 互斥）"`
	AllPorts *bool  `json:"allPorts,omitempty" desc:"显式放宽已存在 CIDR 规则为允许全部端口"`
}

type workspaceConfigureNetworkPolicyInput struct {
	Project                string                       `json:"project" desc:"项目 ID 或 identifier"`
	IngressMode            *string                      `json:"ingressMode,omitempty" enum:"isolated|dependencies" desc:"入站隔离模式（isolated: 同项目空间内默认互通并隔离跨项目；dependencies: 仅按显式 ServiceBinding 或共享依赖放行即 explicit）"`
	EgressMode             *string                      `json:"egressMode,omitempty" enum:"internet|allowlist|none" desc:"出站隔离模式（internet: 允许公网出站；allowlist: 仅允许 CIDR 白名单；none: 禁止外联）"`
	UpsertAllowlistEntries []networkAllowlistEntryInput `json:"upsertAllowlistEntries,omitempty" max:"64" desc:"新增或增量更新公网出站 CIDR 白名单规则（全项目上限 64 条）"`
	RemoveAllowlistCIDRs   []string                     `json:"removeAllowlistCIDRs,omitempty" desc:"按 CIDR 移除出站白名单规则"`
	ClearAllowlist         *bool                        `json:"clearAllowlist,omitempty" desc:"清空全部出站白名单规则"`
}

type workspaceCreateNetworkShareInput struct {
	Project                   string  `json:"project" desc:"提供方项目 ID 或 identifier"`
	Application               *string `json:"application,omitempty" desc:"应用 ID、identifier 或名称（配合 stage/name 定位部署环境）"`
	Target                    string  `json:"target" desc:"部署目标 ID（dplt_xxx）、stage 或名称"`
	ConsumerProjectIdentifier string  `json:"consumerProjectIdentifier" desc:"消费方项目唯一 identifier"`
}

type workspaceAcceptNetworkShareInput struct {
	Project string `json:"project" desc:"消费方项目 ID 或 identifier"`
	ShareID string `json:"shareId" desc:"跨项目共享记录 ID"`
}

type workspaceDeleteNetworkShareInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	ShareID string `json:"shareId" desc:"跨项目共享记录 ID"`
}

type lockedNetworkShareMutation struct {
	ProjectID                 string `json:"projectId"`
	ResolvedTargetID          string `json:"resolvedTargetId,omitempty"`
	ConsumerProjectIdentifier string `json:"consumerProjectIdentifier,omitempty"`
	ShareID                   string `json:"shareId,omitempty"`
}

func (h *Handler) registerWorkspaceNetworkAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[workspaceConfigureNetworkPolicyInput, map[string]any]{
		Name:           "workspace_configure_network_policy",
		Domain:         aitool.DomainWorkspace,
		Title:          "配置项目网络隔离策略",
		Summary:        "纯增量修改项目空间入站/出站网络策略与公网出站 CIDR 白名单并触发对账",
		Tags:           []string{"network-policy", "security"},
		AliasesZH:      []string{"修改项目网络策略", "配置出站白名单", "限制项目网络访问"},
		AliasesEN:      []string{"configure project network policy", "patch egress allowlist"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "workspace_get_network_policy",
				Family:         "resource",
				IDFrom:         "result.projectId",
				ReadIDArgument: "project",
				Expect:         "present",
			},
		},
		Preview: h.aiPreviewWorkspaceConfigureNetworkPolicy,
		Execute: h.aiExecuteWorkspaceConfigureNetworkPolicy,
	})

	aitool.Register(reg, aitool.Spec[workspaceCreateNetworkShareInput, map[string]any]{
		Name:           "workspace_create_network_share",
		Domain:         aitool.DomainWorkspace,
		Title:          "创建跨项目内网服务共享",
		Summary:        "将本项目的某个部署环境在内网共享给指定消费方项目空间",
		Tags:           []string{"network-share", "cross-project"},
		AliasesZH:      []string{"共享服务给其他项目", "添加跨项目共享"},
		AliasesEN:      []string{"create network share", "share deployment target"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "workspace_list_network_shares",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "shareId",
				StaticArguments: map[string]any{"direction": "provided"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "present",
			},
		},
		Preview: h.aiPreviewWorkspaceCreateNetworkShare,
		Execute: h.aiExecuteWorkspaceCreateNetworkShare,
	})

	aitool.Register(reg, aitool.Spec[workspaceAcceptNetworkShareInput, map[string]any]{
		Name:           "workspace_accept_network_share",
		Domain:         aitool.DomainWorkspace,
		Title:          "确认接受跨项目内网服务共享",
		Summary:        "作为消费方项目管理员确认接受跨项目内网服务共享",
		Tags:           []string{"network-share", "cross-project"},
		AliasesZH:      []string{"接受跨项目共享", "确认共享服务"},
		AliasesEN:      []string{"accept network share"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Idempotent:     true,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "workspace_list_network_shares",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "shareId",
				StaticArguments: map[string]any{"direction": "consumed"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "present",
			},
		},
		Preview: h.aiPreviewWorkspaceAcceptNetworkShare,
		Execute: h.aiExecuteWorkspaceAcceptNetworkShare,
	})

	aitool.Register(reg, aitool.Spec[workspaceDeleteNetworkShareInput, map[string]any]{
		Name:           "workspace_delete_network_share",
		Domain:         aitool.DomainWorkspace,
		Title:          "撤销或退出跨项目内网服务共享",
		Summary:        "提供方撤销或消费方退出跨项目内网服务共享并触发双方网络策略对账",
		Tags:           []string{"network-share", "cross-project"},
		AliasesZH:      []string{"撤销跨项目共享", "退出共享服务", "删除网络共享"},
		AliasesEN:      []string{"delete network share", "revoke network share", "leave network share"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "workspace_list_network_shares",
				Family:         "resource",
				IDFrom:         "result.id",
				ReadIDArgument: "shareId",
				Collection:     true,
				CollectionPath: "items",
				Expect:         "absent",
			},
		},
		Preview: h.aiPreviewWorkspaceDeleteNetworkShare,
		Execute: h.aiExecuteWorkspaceDeleteNetworkShare,
	})
}

func prepareNetworkPolicyPatch(existing model.Project, in workspaceConfigureNetworkPolicyInput) (string, string, string, []aitool.PreviewChange, []string, error) {
	ingressMode := existing.NetworkIngressMode
	if ingressMode == "" {
		ingressMode = kubeprovider.NetworkIngressIsolated
	}
	if in.IngressMode != nil {
		val := strings.TrimSpace(*in.IngressMode)
		switch val {
		case kubeprovider.NetworkIngressIsolated, kubeprovider.NetworkIngressDependencies:
			ingressMode = val
		default:
			return "", "", "", nil, nil, aitool.BadRequest("project.network_ingress_mode_invalid", "ingressMode must be isolated or dependencies")
		}
	}

	egressMode := existing.NetworkEgressMode
	if egressMode == "" {
		egressMode = kubeprovider.NetworkEgressInternet
	}
	if in.EgressMode != nil {
		val := strings.TrimSpace(*in.EgressMode)
		switch val {
		case kubeprovider.NetworkEgressInternet, kubeprovider.NetworkEgressAllowlist, kubeprovider.NetworkEgressNone:
			egressMode = val
		default:
			return "", "", "", nil, nil, aitool.BadRequest("project.network_egress_mode_invalid", "egressMode must be internet, allowlist, or none")
		}
	}

	clearAllowlist := boolOrDefault(in.ClearAllowlist, false)
	if clearAllowlist && len(in.RemoveAllowlistCIDRs) > 0 {
		return "", "", "", nil, nil, aitool.BadRequest("argument_invalid", "clearAllowlist=true and removeAllowlistCIDRs cannot both be provided")
	}

	var rules []kubeprovider.EgressAllowRule
	if !clearAllowlist {
		parsed, parseErr := runtimecluster.ParseEgressAllowlist(existing.NetworkEgressAllowlist)
		if parseErr != nil {
			return "", "", "", nil, nil, aitool.BadRequest("project.network_egress_allowlist_invalid", parseErr.Error())
		}
		rules = parsed
	}

	if len(in.RemoveAllowlistCIDRs) > 0 {
		removeSet := make(map[string]struct{}, len(in.RemoveAllowlistCIDRs))
		for _, rawCIDR := range in.RemoveAllowlistCIDRs {
			prefix, parseErr := kubeprovider.ParseNetworkCIDR(strings.TrimSpace(rawCIDR))
			if parseErr != nil || prefix.Bits() == 0 {
				return "", "", "", nil, nil, aitool.BadRequest("project.network_egress_allowlist_invalid", fmt.Sprintf("invalid CIDR in removeAllowlistCIDRs: %q", rawCIDR))
			}
			removeSet[prefix.String()] = struct{}{}
		}
		filtered := make([]kubeprovider.EgressAllowRule, 0, len(rules))
		for _, r := range rules {
			if _, drop := removeSet[r.CIDR]; !drop {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}

	indexByCIDR := make(map[string]int, len(rules))
	for i, r := range rules {
		indexByCIDR[r.CIDR] = i
	}

	for _, entry := range in.UpsertAllowlistEntries {
		if len(entry.Ports) > 0 && entry.AllPorts != nil && *entry.AllPorts {
			return "", "", "", nil, nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("ports and allPorts=true are mutually exclusive for CIDR %q", entry.CIDR))
		}
		prefix, parseErr := kubeprovider.ParseNetworkCIDR(strings.TrimSpace(entry.CIDR))
		if parseErr != nil || prefix.Bits() == 0 {
			return "", "", "", nil, nil, aitool.BadRequest("project.network_egress_allowlist_invalid", fmt.Sprintf("egress allowlist CIDR %q is invalid or /0", entry.CIDR))
		}
		if kubeprovider.WithinPlatformDeniedRange(netip.MustParsePrefix(prefix.String())) {
			return "", "", "", nil, nil, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"project.network_egress_allowlist_invalid",
				fmt.Sprintf("egress allowlist CIDR %s is inside a private or reserved range that is always denied", prefix.String()),
				"Private or internal network access requires a cluster maintainer to grant an exception via runtime_save_cluster_egress_exception.",
			)
		}

		var normalizedPorts []int
		if len(entry.Ports) > 0 {
			seenPorts := make(map[int]struct{}, len(entry.Ports))
			for _, p := range entry.Ports {
				if p < 1 || p > 65535 {
					return "", "", "", nil, nil, aitool.BadRequest("project.network_egress_allowlist_invalid", fmt.Sprintf("invalid port %d in CIDR %s; must be 1..65535", p, prefix.String()))
				}
				if _, dup := seenPorts[p]; !dup {
					seenPorts[p] = struct{}{}
					normalizedPorts = append(normalizedPorts, p)
				}
			}
			sort.Ints(normalizedPorts)
			if len(normalizedPorts) > 16 {
				return "", "", "", nil, nil, aitool.BadRequest("project.network_egress_allowlist_invalid", "at most 16 ports per egress allowlist entry")
			}
		}

		canonical := prefix.String()
		if existingIdx, exists := indexByCIDR[canonical]; exists {
			switch {
			case len(normalizedPorts) > 0:
				rules[existingIdx].Ports = normalizedPorts
			case entry.AllPorts != nil && *entry.AllPorts:
				rules[existingIdx].Ports = nil
			default:
				// R7 anti-privilege-escalation: omitting ports on an existing CIDR preserves its existing port restrictions!
			}
		} else {
			indexByCIDR[canonical] = len(rules)
			rules = append(rules, kubeprovider.EgressAllowRule{
				CIDR:  canonical,
				Ports: normalizedPorts,
			})
		}
	}

	if len(rules) > 64 {
		return "", "", "", nil, nil, aitool.BadRequest("project.network_egress_allowlist_invalid", "at most 64 egress allowlist entries are allowed")
	}

	lines := make([]string, 0, len(rules))
	for _, r := range rules {
		line := r.CIDR
		if len(r.Ports) > 0 {
			portStrs := make([]string, 0, len(r.Ports))
			for _, p := range r.Ports {
				portStrs = append(portStrs, strconv.Itoa(p))
			}
			line += " " + strings.Join(portStrs, ",")
		}
		lines = append(lines, line)
	}
	allowlistRaw, normErr := normalizeEgressAllowlist(strings.Join(lines, "\n"))
	if normErr != nil {
		return "", "", "", nil, nil, aitool.BadRequest("project.network_egress_allowlist_invalid", normErr.Error())
	}

	var changes []aitool.PreviewChange
	if ingressMode != existing.NetworkIngressMode {
		changes = append(changes, aitool.PreviewChange{Field: "ingressMode", Before: existing.NetworkIngressMode, After: ingressMode, Action: "update"})
	}
	if egressMode != existing.NetworkEgressMode {
		changes = append(changes, aitool.PreviewChange{Field: "egressMode", Before: existing.NetworkEgressMode, After: egressMode, Action: "update"})
	}
	if allowlistRaw != existing.NetworkEgressAllowlist {
		changes = append(changes, aitool.PreviewChange{Field: "egressAllowlist", Before: existing.NetworkEgressAllowlist, After: allowlistRaw, Action: "update"})
	}

	allowlistMutated := len(in.UpsertAllowlistEntries) > 0 || len(in.RemoveAllowlistCIDRs) > 0 || clearAllowlist
	var warnings []string
	if allowlistMutated && egressMode != kubeprovider.NetworkEgressAllowlist {
		warnings = append(warnings, fmt.Sprintf("egressAllowlist only takes effect when egressMode is \"allowlist\" (current effective egressMode is %q).", egressMode))
	}

	return ingressMode, egressMode, allowlistRaw, changes, warnings, nil
}

func (h *Handler) aiPreviewWorkspaceConfigureNetworkPolicy(tc *aitool.ToolContext, in workspaceConfigureNetworkPolicyInput) (*aitool.PreviewResult, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}

	ingressMode, egressMode, _, changes, warnings, err := prepareNetworkPolicyPatch(project, in)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Configure network policy for project %q (ingress=%s, egress=%s)", project.Name, ingressMode, egressMode),
			TargetResource: "project_network_policy",
			TargetID:       project.ID,
			ProjectID:      project.ID,
			Changes:        changes,
			Impact:         []string{"Triggers asynchronous Kubernetes NetworkPolicy reconciliation across clusters used by this project."},
			Warnings:       warnings,
		},
		LockedPayload: lockedProjectMutation{ProjectID: project.ID},
	}, nil
}

func (h *Handler) aiExecuteWorkspaceConfigureNetworkPolicy(tc *aitool.ToolContext, in workspaceConfigureNetworkPolicyInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	ingressMode, egressMode, allowlistRaw, _, warnings, err := prepareNetworkPolicyPatch(project, in)
	if err != nil {
		return nil, err
	}

	if err := db.Model(&project).Updates(map[string]any{
		"network_ingress_mode":     ingressMode,
		"network_egress_mode":      egressMode,
		"network_egress_allowlist": allowlistRaw,
	}).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "project.network_policy_update_failed", "project network policy update failed")
	}

	entriesCount := 0
	if allowlistRaw != "" {
		entriesCount = strings.Count(allowlistRaw, "\n") + 1
	}
	h.auditWithContext(user.ID, "project.network_policy.update", project.ID, true,
		fmt.Sprintf("ingress=%s egress=%s allowlistEntries=%d", ingressMode, egressMode, entriesCount), tc.Context())
	h.host.EnqueueNetworkPolicyProjectReconcile(tc.Context(), project.ID)

	exceptions, err := projectEgressExceptions(db, project.ID)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", "egress exception lookup failed")
	}

	out := map[string]any{
		"id":               project.ID,
		"projectId":        project.ID,
		"ingressMode":      ingressMode,
		"egressMode":       egressMode,
		"egressAllowlist":  allowlistRaw,
		"allowlistEntries": allowlistEntryViews(allowlistRaw),
		"egressExceptions": exceptions,
	}
	if len(warnings) > 0 {
		out["warnings"] = warnings
	}
	return out, nil
}

func validateNetworkShareCreation(db *gorm.DB, provider model.Project, target model.DeploymentTarget, consumerIdentifier string) (model.Project, model.RuntimeCluster, bool, error) {
	consumerIdentifier = strings.TrimSpace(consumerIdentifier)
	if consumerIdentifier == "" {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.BadRequest("network_share.consumer_invalid", "consumerProjectIdentifier is required")
	}
	cluster, err := runtimecluster.ResolveForTarget(db, target.ClusterID)
	if err != nil {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.BadRequest("network_share.target_invalid", "deployment target has no available runtime cluster")
	}

	var consumer model.Project
	if err := db.Where("identifier = ? AND deleted_at IS NULL", consumerIdentifier).Take(&consumer).Error; err != nil || consumer.ID == provider.ID || !model.DeleteStatusActive(consumer.DeleteStatus) {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.BadRequest("network_share.consumer_invalid", "consumer must be another active project space that can use the cluster")
	}
	usable, err := runtimecluster.ProjectCanDeployToCluster(db, cluster, consumer.ID)
	if err != nil {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if !usable {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.BadRequest("network_share.consumer_invalid", "consumer must be another active project space that can use the cluster")
	}

	var existingCount int64
	if err := db.Model(&model.NetworkServiceShare{}).
		Where("provider_project_id = ? AND deployment_target_id = ? AND consumer_project_id = ?", provider.ID, target.ID, consumer.ID).
		Count(&existingCount).Error; err != nil {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if existingCount > 0 {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.Conflict("network_share.duplicate", "the deployment target is already shared with this project space")
	}

	var perTarget, pending int64
	if err := db.Model(&model.NetworkServiceShare{}).Where("deployment_target_id = ?", target.ID).Count(&perTarget).Error; err != nil {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if err := db.Model(&model.NetworkServiceShare{}).Where("provider_project_id = ? AND accepted_at IS NULL", provider.ID).Count(&pending).Error; err != nil {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if perTarget >= maxNetworkShareConsumers || pending >= maxPendingNetworkShares {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.BadRequest("network_share.limit_exceeded",
			fmt.Sprintf("at most %d project spaces per deployment target and %d unaccepted shares per project space", maxNetworkShareConsumers, maxPendingNetworkShares))
	}

	draftShare := model.NetworkServiceShare{
		ProviderProjectID:  provider.ID,
		DeploymentTargetID: target.ID,
		ConsumerProjectID:  consumer.ID,
	}
	_, approvalRequired, err := runtimecluster.ShareApprovalState(db, draftShare, target.ClusterID)
	if err != nil {
		return model.Project{}, model.RuntimeCluster{}, false, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return consumer, cluster, approvalRequired, nil
}

func shareResponseToMap(resp NetworkServiceShareResponse, projectID, direction string) map[string]any {
	return map[string]any{
		"id":                   resp.ID,
		"projectId":            projectID,
		"direction":            direction,
		"providerProjectId":    resp.ProviderProjectID,
		"providerProjectName":  resp.ProviderProjectName,
		"deploymentTargetId":   resp.DeploymentTargetID,
		"deploymentTargetName": resp.DeploymentTargetName,
		"applicationName":      resp.ApplicationName,
		"runtimeClusterId":     resp.RuntimeClusterID,
		"runtimeClusterName":   resp.RuntimeClusterName,
		"consumerProjectId":    resp.ConsumerProjectID,
		"consumerProjectName":  resp.ConsumerProjectName,
		"address":              resp.Address,
		"status":               resp.Status,
		"approvalRequired":     resp.ApprovalRequired,
		"acceptedAt":           resp.AcceptedAt,
		"approvedAt":           resp.ApprovedAt,
		"createdAt":            resp.CreatedAt,
	}
}

func (h *Handler) aiPreviewWorkspaceCreateNetworkShare(tc *aitool.ToolContext, in workspaceCreateNetworkShareInput) (*aitool.PreviewResult, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	target, err := resolveProjectTargetForShareAI(db, project.ID, in.Application, in.Target)
	if err != nil {
		return nil, err
	}
	_, cluster, approvalRequired, err := validateNetworkShareCreation(db, project, target, in.ConsumerProjectIdentifier)
	if err != nil {
		return nil, err
	}

	impact := []string{
		fmt.Sprintf("Shares target %q (%s) on cluster %q with consumer project identifier %q (pending consumer acceptance)", target.Name, target.ID, cluster.Name, strings.TrimSpace(in.ConsumerProjectIdentifier)),
	}
	var warnings []string
	if approvalRequired {
		warnings = append(warnings, "Provider project holds cluster egress exceptions; a cluster maintainer must also approve this share via runtime_approve_cluster_network_share.")
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Share target %q (%s) from project %q to consumer %q", target.Name, target.ID, project.Identifier, strings.TrimSpace(in.ConsumerProjectIdentifier)),
			TargetResource: "network_service_share",
			TargetID:       target.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "deploymentTargetId", After: target.ID, Action: "create"},
				{Field: "consumerProjectIdentifier", After: strings.TrimSpace(in.ConsumerProjectIdentifier), Action: "create"},
			},
			Impact:   impact,
			Warnings: warnings,
		},
		LockedPayload: lockedNetworkShareMutation{
			ProjectID:                 project.ID,
			ResolvedTargetID:          target.ID,
			ConsumerProjectIdentifier: strings.TrimSpace(in.ConsumerProjectIdentifier),
		},
	}, nil
}

func (h *Handler) aiExecuteWorkspaceCreateNetworkShare(tc *aitool.ToolContext, in workspaceCreateNetworkShareInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var locked lockedNetworkShareMutation
	_ = tc.UnmarshalLockedPayload(&locked)

	targetID := strings.TrimSpace(locked.ResolvedTargetID)
	if targetID == "" {
		resolved, resolveErr := resolveProjectTargetForShareAI(db, project.ID, in.Application, in.Target)
		if resolveErr != nil {
			return nil, resolveErr
		}
		targetID = resolved.ID
	}
	consumerIdent := strings.TrimSpace(in.ConsumerProjectIdentifier)

	var share model.NetworkServiceShare
	txErr := db.Transaction(func(tx *gorm.DB) error {
		lockChain := func() *gorm.DB { return tx.Clauses(clause.Locking{Strength: "UPDATE"}) }
		var provider model.Project
		if err := lockChain().Select("id").Where("id = ?", project.ID).Take(&provider).Error; err != nil {
			return err
		}
		var target model.DeploymentTarget
		if err := lockChain().Select("id", "project_id", "cluster_id", "delete_status").
			Where("id = ? AND project_id = ? AND deleted_at IS NULL", targetID, project.ID).Take(&target).Error; err != nil || !model.DeleteStatusActive(target.DeleteStatus) {
			return networkShareRejection{http.StatusBadRequest, "network_share.target_invalid", "deployment target must be an active target of this project"}
		}
		cluster, err := runtimecluster.ResolveForTarget(tx, target.ClusterID)
		if err != nil {
			return networkShareRejection{http.StatusBadRequest, "network_share.target_invalid", "deployment target has no available runtime cluster"}
		}
		var consumer model.Project
		if err := tx.Select("id", "delete_status").Where("identifier = ? AND deleted_at IS NULL", consumerIdent).Take(&consumer).Error; err != nil || consumer.ID == project.ID || !model.DeleteStatusActive(consumer.DeleteStatus) {
			return networkShareRejection{http.StatusBadRequest, "network_share.consumer_invalid", "consumer must be another active project space that can use the cluster"}
		}
		usable, err := runtimecluster.ProjectCanDeployToCluster(tx, cluster, consumer.ID)
		if err != nil {
			return err
		}
		if !usable {
			return networkShareRejection{http.StatusBadRequest, "network_share.consumer_invalid", "consumer must be another active project space that can use the cluster"}
		}
		var perTarget, pending int64
		if err := tx.Model(&model.NetworkServiceShare{}).Where("deployment_target_id = ?", target.ID).Count(&perTarget).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.NetworkServiceShare{}).Where("provider_project_id = ? AND accepted_at IS NULL", project.ID).Count(&pending).Error; err != nil {
			return err
		}
		if perTarget >= maxNetworkShareConsumers || pending >= maxPendingNetworkShares {
			return networkShareRejection{http.StatusBadRequest, "network_share.limit_exceeded",
				fmt.Sprintf("at most %d project spaces per deployment target and %d unaccepted shares per project space", maxNetworkShareConsumers, maxPendingNetworkShares)}
		}
		now := time.Now().UTC()
		share = model.NetworkServiceShare{
			ID: id.New("nss"), ProviderProjectID: project.ID, DeploymentTargetID: target.ID, ConsumerProjectID: consumer.ID,
			CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&share)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return networkShareRejection{http.StatusConflict, "network_share.duplicate", "the deployment target is already shared with this project space"}
		}
		return nil
	})

	var rejection networkShareRejection
	if errors.As(txErr, &rejection) {
		return nil, aitool.NewError(rejection.status, rejection.code, rejection.detail)
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "network_share.save_failed", "network share save failed")
	}

	items, err := NetworkServiceShareResponses(db, []model.NetworkServiceShare{share})
	if err != nil || len(items) == 0 {
		return nil, aitool.NewError(http.StatusInternalServerError, "network_share.lookup_failed", "network share lookup failed")
	}
	h.auditWithContext(user.ID, "project.network_share.create", share.ID, true,
		fmt.Sprintf("target=%s consumer=%s approvalRequired=%t %s", share.DeploymentTargetID, share.ConsumerProjectID, items[0].ApprovalRequired, runtimecluster.EgressExceptionAuditMarker(db, project.ID)), tc.Context())

	return shareResponseToMap(items[0], project.ID, "provided"), nil
}

func (h *Handler) aiPreviewWorkspaceAcceptNetworkShare(tc *aitool.ToolContext, in workspaceAcceptNetworkShareInput) (*aitool.PreviewResult, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	shareID := strings.TrimSpace(in.ShareID)
	if shareID == "" {
		return nil, aitool.BadRequest("argument_invalid", "shareId is required")
	}

	var share model.NetworkServiceShare
	if err := db.Where("id = ? AND consumer_project_id = ?", shareID, project.ID).Take(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("network_share.not_found", "network share was not found for this consumer project")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	items, err := NetworkServiceShareResponses(db, []model.NetworkServiceShare{share})
	if err != nil || len(items) == 0 {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", "network share lookup failed")
	}

	nextStatus := NetworkShareStatusActive
	if items[0].ApprovalRequired && share.ApprovedAt == nil {
		nextStatus = NetworkShareStatusPendingApproval
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Accept network share %s from project %q into consumer project %q", share.ID, items[0].ProviderProjectName, project.Name),
			TargetResource: "network_service_share",
			TargetID:       share.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "status", Before: items[0].Status, After: nextStatus, Action: "accept"},
			},
			Impact: []string{"Reconciles NetworkPolicies on both provider and consumer projects."},
		},
		LockedPayload: lockedNetworkShareMutation{
			ProjectID: project.ID,
			ShareID:   share.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteWorkspaceAcceptNetworkShare(tc *aitool.ToolContext, in workspaceAcceptNetworkShareInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	shareID := strings.TrimSpace(in.ShareID)

	var share model.NetworkServiceShare
	if err := db.Where("id = ? AND consumer_project_id = ?", shareID, project.ID).Take(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("network_share.not_found", "network share was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	if share.AcceptedAt == nil {
		now := time.Now().UTC()
		res := db.Model(&model.NetworkServiceShare{}).
			Where("id = ? AND accepted_at IS NULL", share.ID).
			Updates(map[string]any{"accepted_at": now, "accepted_by": user.ID, "updated_at": now})
		if res.Error != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "network_share.save_failed", "network share save failed")
		}
		if res.RowsAffected > 0 {
			share.AcceptedAt = &now
			share.AcceptedBy = user.ID
			share.UpdatedAt = now
			h.auditWithContext(user.ID, "project.network_share.accept", share.ID, true,
				fmt.Sprintf("target=%s provider=%s", share.DeploymentTargetID, share.ProviderProjectID), tc.Context())
			h.host.EnqueueNetworkPolicyProjectReconcile(tc.Context(), share.ProviderProjectID)
			h.host.EnqueueNetworkPolicyProjectReconcile(tc.Context(), share.ConsumerProjectID)
		} else {
			_ = db.Where("id = ?", share.ID).Take(&share).Error
		}
	}

	items, err := NetworkServiceShareResponses(db, []model.NetworkServiceShare{share})
	if err != nil || len(items) == 0 {
		return nil, aitool.NewError(http.StatusInternalServerError, "network_share.lookup_failed", "network share lookup failed")
	}
	return shareResponseToMap(items[0], project.ID, "consumed"), nil
}

func (h *Handler) aiPreviewWorkspaceDeleteNetworkShare(tc *aitool.ToolContext, in workspaceDeleteNetworkShareInput) (*aitool.PreviewResult, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	shareID := strings.TrimSpace(in.ShareID)
	if shareID == "" {
		return nil, aitool.BadRequest("argument_invalid", "shareId is required")
	}

	var share model.NetworkServiceShare
	if err := db.Where("id = ? AND (provider_project_id = ? OR consumer_project_id = ?)", shareID, project.ID, project.ID).Take(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("network_share.not_found", "network share was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	actionVerb := "Leave"
	if share.ProviderProjectID == project.ID {
		actionVerb = "Revoke"
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("%s cross-project network share %s for project %q", actionVerb, share.ID, project.Name),
			TargetResource: "network_service_share",
			TargetID:       share.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "share", Before: share.ID, Action: "delete"},
			},
			Impact: []string{"Removes cross-project network access and triggers NetworkPolicy reconciliation on both projects."},
		},
		LockedPayload: lockedNetworkShareMutation{
			ProjectID: project.ID,
			ShareID:   share.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteWorkspaceDeleteNetworkShare(tc *aitool.ToolContext, in workspaceDeleteNetworkShareInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	shareID := strings.TrimSpace(in.ShareID)

	var share model.NetworkServiceShare
	if err := db.Where("id = ? AND (provider_project_id = ? OR consumer_project_id = ?)", shareID, project.ID, project.ID).Take(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("network_share.not_found", "network share was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	if err := db.Delete(&model.NetworkServiceShare{}, "id = ?", share.ID).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "network_share.delete_failed", "network share delete failed")
	}

	role := "consumer"
	direction := "consumed"
	if share.ProviderProjectID == project.ID {
		role = "provider"
		direction = "provided"
	}
	h.auditWithContext(user.ID, "project.network_share.delete", share.ID, true,
		fmt.Sprintf("target=%s provider=%s consumer=%s role=%s", share.DeploymentTargetID, share.ProviderProjectID, share.ConsumerProjectID, role), tc.Context())
	h.host.EnqueueNetworkPolicyProjectReconcile(tc.Context(), share.ProviderProjectID)
	h.host.EnqueueNetworkPolicyProjectReconcile(tc.Context(), share.ConsumerProjectID)

	return map[string]any{
		"id":        share.ID,
		"projectId": project.ID,
		"direction": direction,
		"deleted":   true,
	}, nil
}

var _ = slices.Equal[[]int]
