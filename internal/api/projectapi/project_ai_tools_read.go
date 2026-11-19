package projectapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"gorm.io/gorm"
)

type workspaceListProjectsInput struct {
	Visibility *string `json:"visibility,omitempty" enum:"related|all" default:"related" desc:"可见范围：related（本人关联项目）或 all（仅平台管理员全局会话可用）"`
	Search     *string `json:"search,omitempty" desc:"按项目名称或 identifier 模糊搜索"`
	Identifier *string `json:"identifier,omitempty" desc:"精确匹配项目 identifier"`
	PinnedOnly *bool   `json:"pinnedOnly,omitempty" default:"false" desc:"仅列出当前用户已收藏置顶的项目"`
	SortBy     *string `json:"sortBy,omitempty" enum:"createdAt|updatedAt|lastUsedAt|useCount|dashboardOrder|name|identifier" default:"createdAt" desc:"排序字段"`
	SortOrder  *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page       *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize   *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type workspaceProjectListItem struct {
	model.Project
	ProjectID string     `json:"projectId"`
	Pinned    bool       `json:"pinned"`
	PinnedAt  *time.Time `json:"pinnedAt,omitempty"`
}

type workspaceGetProjectInput struct {
	Project            string `json:"project" desc:"项目 ID 或 identifier"`
	IncludeOverview    *bool  `json:"includeOverview,omitempty" default:"true" desc:"附带应用数、发布数、访问入口状态、部署目标数、成员数与仓库绑定数概览"`
	IncludeDiagnostics *bool  `json:"includeDiagnostics,omitempty" default:"false" desc:"附带近 7 天构建、发布与异常网关入口诊断摘要"`
}

type workspaceOverviewExtended struct {
	projectOverviewResponse
	DeploymentTargetsCount  int64 `json:"deploymentTargetsCount"`
	MembersCount            int64 `json:"membersCount"`
	RepositoryBindingsCount int64 `json:"repositoryBindingsCount"`
}

type workspacePendingBillingTransfer struct {
	RequestID       string     `json:"requestId"`
	RecipientUserID string     `json:"recipientUserId"`
	Status          string     `json:"status"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type workspaceListMembersInput struct {
	Project         string  `json:"project" desc:"项目 ID 或 identifier"`
	Member          *string `json:"member,omitempty" desc:"按成员记录 ID、用户 ID 或邮箱精确过滤单条成员"`
	Search          *string `json:"search,omitempty" desc:"按成员姓名、邮箱或角色搜索"`
	CandidateSearch *string `json:"candidateSearch,omitempty" desc:"搜索尚未加入本项目的平台活跃用户候选列表（需项目 owner/admin 权限）"`
	SortBy          *string `json:"sortBy,omitempty" enum:"createdAt|name|email|role" default:"createdAt" desc:"排序字段"`
	SortOrder       *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"asc" desc:"排序方向"`
	Page            *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize        *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type workspaceGetNetworkPolicyInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
}

type workspaceAllowlistEntryView struct {
	CIDR  string `json:"cidr"`
	Ports []int  `json:"ports,omitempty"`
}

type workspaceListNetworkSharesInput struct {
	Project   string  `json:"project" desc:"项目 ID 或 identifier"`
	Direction string  `json:"direction" enum:"provided|consumed" desc:"共享方向：provided（本项目提供）或 consumed（外部授权给本项目）"`
	ShareID   *string `json:"shareId,omitempty" desc:"按跨项目共享记录 ID 精确过滤"`
	Page      *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize  *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

func (h *Handler) registerWorkspaceReadAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[workspaceListProjectsInput, map[string]any]{
		Name:           "workspace_list_projects",
		Domain:         aitool.DomainWorkspace,
		Title:          "查看项目空间列表",
		Summary:        "分页列出可访问的项目空间及个人收藏置顶状态",
		Tags:           []string{"projects", "dashboard"},
		AliasesZH:      []string{"项目列表", "查看我的项目", "置顶项目列表"},
		AliasesEN:      []string{"list projects", "my projects", "pinned projects"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionProjectRead)},
		Execute:        h.aiWorkspaceListProjects,
	})

	aitool.Register(reg, aitool.Spec[workspaceGetProjectInput, map[string]any]{
		Name:           "workspace_get_project",
		Domain:         aitool.DomainWorkspace,
		Title:          "查看项目空间详情与概览",
		Summary:        "查看单个项目空间配置、资源计数概览、当前用户角色权限及近 7 天交付/网关诊断摘要",
		Tags:           []string{"projects", "overview", "diagnostics"},
		AliasesZH:      []string{"查看项目详情", "项目概览", "项目诊断摘要"},
		AliasesEN:      []string{"get project", "project overview", "project diagnostics"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionProjectRead)},
		Execute:        h.aiWorkspaceGetProject,
	})

	aitool.Register(reg, aitool.Spec[workspaceListMembersInput, map[string]any]{
		Name:           "workspace_list_members",
		Domain:         aitool.DomainWorkspace,
		Title:          "查看项目成员列表",
		Summary:        "分页查看项目成员列表，或搜索未入项的候选平台活跃用户",
		Tags:           []string{"members", "iam"},
		AliasesZH:      []string{"项目成员列表", "查看成员权限", "搜索候选成员"},
		AliasesEN:      []string{"list project members", "search member candidates"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionProjectRead)},
		Execute:        h.aiWorkspaceListMembers,
	})

	aitool.Register(reg, aitool.Spec[workspaceGetNetworkPolicyInput, map[string]any]{
		Name:           "workspace_get_network_policy",
		Domain:         aitool.DomainWorkspace,
		Title:          "查看项目网络隔离策略",
		Summary:        "查看项目空间入站/出站网络隔离模式、出站 CIDR 白名单及只读集群出站例外",
		Tags:           []string{"network-policy", "security"},
		AliasesZH:      []string{"查看项目网络策略", "查看出站白名单", "网络隔离配置"},
		AliasesEN:      []string{"get project network policy", "egress allowlist"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionProjectRead)},
		Execute:        h.aiWorkspaceGetNetworkPolicy,
	})

	aitool.Register(reg, aitool.Spec[workspaceListNetworkSharesInput, map[string]any]{
		Name:           "workspace_list_network_shares",
		Domain:         aitool.DomainWorkspace,
		Title:          "查看跨项目内网服务共享",
		Summary:        "分页列出本项目对外提供（provided）或外部授权给本项目使用（consumed）的跨项目内网服务共享",
		Tags:           []string{"network-share", "cross-project"},
		AliasesZH:      []string{"跨项目共享列表", "查看内网共享服务"},
		AliasesEN:      []string{"list network shares", "cross-project shares"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionProjectRead)},
		Execute:        h.aiWorkspaceListNetworkShares,
	})
}

func (h *Handler) aiWorkspaceListProjects(tc *aitool.ToolContext, in workspaceListProjectsInput) (map[string]any, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	platformAdmin := authz.IsPlatformAdmin(user.Role)
	pinnedOnly := boolOrDefault(in.PinnedOnly, false)

	sortBy := stringOrDefault(in.SortBy, "createdAt")
	switch sortBy {
	case "createdAt", "updatedAt", "lastUsedAt", "useCount", "dashboardOrder", "name", "identifier":
	default:
		return nil, aitool.BadRequest("argument_invalid", "invalid sortBy field")
	}
	sortOrder := strings.ToLower(stringOrDefault(in.SortOrder, "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, aitool.BadRequest("argument_invalid", "sortOrder must be asc or desc")
	}
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	readPolicy, exists := authz.ProjectPolicyForAction(authz.ActionProjectRead)
	if !exists {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", "project authorization service is temporarily unavailable")
	}

	rawVisibility := ""
	if in.Visibility != nil {
		rawVisibility = strings.TrimSpace(*in.Visibility)
	}

	convProjectID := strings.TrimSpace(tc.ConversationProjectID)
	var visibility projectservice.ListVisibility
	if convProjectID != "" {
		if strings.EqualFold(rawVisibility, "all") {
			return nil, projectScopeMismatchError("visibility=all is not allowed in a single-project conversation")
		}
		if rawVisibility != "" && !strings.EqualFold(rawVisibility, "related") {
			return nil, aitool.BadRequest("request.invalid", "visibility must be related or all")
		}
		if _, _, _, authErr := h.authorizeProjectForAI(tc, convProjectID, authz.ActionProjectRead); authErr != nil {
			return nil, authErr
		}
		visibility = projectservice.ListVisibilityRelated
	} else {
		resolvedVis, visErr := projectservice.ResolveListVisibility(rawVisibility, platformAdmin)
		switch {
		case errors.Is(visErr, projectservice.ErrListVisibilityInvalid):
			return nil, aitool.BadRequest("request.invalid", visErr.Error())
		case errors.Is(visErr, projectservice.ErrListVisibilityForbidden):
			return nil, aitool.Forbidden("auth.forbidden", visErr.Error())
		case visErr != nil:
			return nil, aitool.NewError(http.StatusInternalServerError, "project.list_failed", visErr.Error())
		}
		visibility = resolvedVis
		if platformAdmin {
			if _, sysErr := h.ensurePlatformSystemProject(user, tc.Context()); sysErr != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "project.system_project_failed", sysErr.Error())
			}
		}
	}

	baseQuery := db.Table("projects").
		Select("projects.*, project_members.dashboard_order, project_members.last_used_at, project_members.use_count").
		Joins("left join project_members on project_members.project_id = projects.id and project_members.user_id = ? and project_members.role in ?", user.ID, readPolicy.AllowedRoles).
		Joins("left join project_pins on project_pins.project_id = projects.id and project_pins.user_id = ?", user.ID).
		Where("projects.deleted_at is null")

	if convProjectID != "" {
		baseQuery = baseQuery.Where("projects.id = ?", convProjectID)
	} else if visibility == projectservice.ListVisibilityRelated {
		if pinnedOnly && platformAdmin {
			baseQuery = baseQuery.Where("(project_members.user_id = ? OR project_pins.user_id = ?)", user.ID, user.ID)
		} else {
			baseQuery = baseQuery.Where("project_members.user_id = ?", user.ID)
		}
	}

	if pinnedOnly {
		baseQuery = baseQuery.Where("project_pins.user_id = ?", user.ID)
	}
	if in.Identifier != nil && strings.TrimSpace(*in.Identifier) != "" {
		baseQuery = baseQuery.Where("projects.identifier = ?", strings.TrimSpace(*in.Identifier))
	}
	if in.Search != nil && strings.TrimSpace(*in.Search) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(*in.Search)) + "%"
		baseQuery = baseQuery.Where("(lower(projects.name) like ? or lower(projects.identifier) like ?)", like, like)
	}

	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var projects []model.Project
	if err := baseQuery.Session(&gorm.Session{}).
		Order(projectListOrderClause(sortBy, sortOrder)).
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&projects).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	pinsByProject := map[string]time.Time{}
	if len(projects) > 0 {
		ids := make([]string, len(projects))
		for i, p := range projects {
			ids[i] = p.ID
		}
		var pins []model.ProjectPin
		if err := db.Where("user_id = ? AND project_id IN ?", user.ID, ids).Find(&pins).Error; err == nil {
			for _, pin := range pins {
				pinsByProject[pin.ProjectID] = pin.PinnedAt
			}
		}
	}

	items := make([]workspaceProjectListItem, 0, len(projects))
	for _, p := range projects {
		item := workspaceProjectListItem{
			Project:   p,
			ProjectID: p.ID,
		}
		if pinnedAt, ok := pinsByProject[p.ID]; ok {
			t := pinnedAt
			item.Pinned = true
			item.PinnedAt = &t
		}
		items = append(items, item)
	}

	return aitool.PageEnvelope(items, page, pageSize, sortBy, sortOrder, total), nil
}

func (h *Handler) aiWorkspaceGetProject(tc *aitool.ToolContext, in workspaceGetProjectInput) (map[string]any, error) {
	user, project, currentRole, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectRead)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	reqCtx := tc.Context()

	includeOverview := boolOrDefault(in.IncludeOverview, true)
	includeDiagnostics := boolOrDefault(in.IncludeDiagnostics, false)

	if includeDiagnostics {
		subject := authz.ProjectSubject{UserID: user.ID, PlatformRole: user.Role}
		authorizer := h.projectAuthorizer(reqCtx)
		for _, action := range []authz.Action{authz.ActionBuildRead, authz.ActionDeploymentRead, authz.ActionGatewayRead} {
			if _, authErr := authorizer.AuthorizeProject(reqCtx, subject, project.ID, action); authErr != nil {
				return nil, mapProjectAuthErrorForAI(authErr)
			}
		}
	}

	baseResp := h.projectResponse(project, reqCtx)
	baseResp.CurrentUserRole = currentRole
	allowedActions := authz.ProjectActionsAllowed(user.Role, currentRole)
	actionList := make([]string, 0, len(allowedActions))
	for _, act := range allowedActions {
		actionList = append(actionList, string(act))
	}
	transferEnabled := h.host.VolumeTransferEnabled()

	var pinCount int64
	_ = db.Model(&model.ProjectPin{}).Where("user_id = ? AND project_id = ?", user.ID, project.ID).Count(&pinCount).Error

	out := map[string]any{
		"id":                     project.ID,
		"projectId":              project.ID,
		"identifier":             project.Identifier,
		"kubernetesNamespace":    project.KubernetesNamespace,
		"name":                   project.Name,
		"description":            project.Description,
		"maxConcurrentBuilds":    project.MaxConcurrentBuilds,
		"webConsoleEnabled":      project.WebConsoleEnabled,
		"billingOwnerUserId":     project.BillingOwnerUserID,
		"billingOwner":           baseResp.BillingOwner,
		"systemKey":              project.SystemKey,
		"networkIngressMode":     project.NetworkIngressMode,
		"networkEgressMode":      project.NetworkEgressMode,
		"networkEgressAllowlist": project.NetworkEgressAllowlist,
		"deleteStatus":           project.DeleteStatus,
		"deleteMessage":          project.DeleteMessage,
		"deleteStartedAt":        project.DeleteStartedAt,
		"deleteFinishedAt":       project.DeleteFinishedAt,
		"createdAt":              project.CreatedAt,
		"updatedAt":              project.UpdatedAt,
		"pinned":                 pinCount > 0,
		"currentUserRole":        currentRole,
		"currentUserActions":     actionList,
		"volumeTransferEnabled":  transferEnabled,
	}

	if currentRole == authz.ProjectRoleOwner || (strings.TrimSpace(project.BillingOwnerUserID) != "" && project.BillingOwnerUserID == user.ID) {
		var pendingReq model.InboxActionRequest
		if err := db.Where("type = ? AND project_id = ? AND status IN ?",
			projectservice.BillingOwnerTransferRequestType,
			project.ID,
			[]string{"pending", "processing"},
		).Order("created_at desc").First(&pendingReq).Error; err == nil {
			out["pendingBillingTransfer"] = workspacePendingBillingTransfer{
				RequestID:       pendingReq.ID,
				RecipientUserID: pendingReq.RecipientUserID,
				Status:          pendingReq.Status,
				ExpiresAt:       pendingReq.ExpiresAt,
				CreatedAt:       pendingReq.CreatedAt,
			}
		}
	}

	now := time.Now().UTC()
	if includeOverview || includeDiagnostics {
		tc.SetNoStore()
	}

	if includeOverview {
		overviewCounts, countErr := projectOverviewCounts(db, project.ID, now)
		if countErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", countErr.Error())
		}
		var routes []model.GatewayRoute
		if routeErr := db.Where("project_id = ? AND enabled = ? AND delete_status NOT IN ?", project.ID, true, projectOverviewExcludedRouteDeleteStatuses).
			Order("created_at asc, id asc").
			Find(&routes).Error; routeErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", routeErr.Error())
		}
		overviewCounts.AccessRoutes = summarizeProjectOverviewAccessRoutes(h.host.ObserveGatewayRouteStatuses(reqCtx, routes), time.Now().UTC())

		var targetsCount, membersCount, repoBindingsCount int64
		if err := db.Model(&model.DeploymentTarget{}).
			Where("project_id = ? AND delete_status NOT IN ?", project.ID, projectOverviewExcludedApplicationDeleteStatuses).
			Count(&targetsCount).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if err := db.Model(&model.ProjectMember{}).
			Where("project_id = ?", project.ID).
			Count(&membersCount).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if err := db.Model(&model.RepositoryBinding{}).
			Where("project_id = ?", project.ID).
			Count(&repoBindingsCount).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}

		out["overview"] = workspaceOverviewExtended{
			projectOverviewResponse: overviewCounts,
			DeploymentTargetsCount:  targetsCount,
			MembersCount:            membersCount,
			RepositoryBindingsCount: repoBindingsCount,
		}
	}

	if includeDiagnostics {
		diag, diagErr := loadProjectDiagnosticsSnapshot(db, project.ID, now)
		if diagErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", diagErr.Error())
		}
		var routes []model.GatewayRoute
		if routeErr := db.Where("project_id = ? AND enabled = ? AND delete_status NOT IN ?", project.ID, true, projectOverviewExcludedRouteDeleteStatuses).
			Order("created_at asc, id asc").
			Find(&routes).Error; routeErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", routeErr.Error())
		}
		diag.AccessRoutes = summarizeProjectDiagnosticsAccessRoutes(h.host.ObserveGatewayRouteDiagnostics(reqCtx, routes), time.Now().UTC())
		out["diagnostics"] = diag
	}

	return out, nil
}

func (h *Handler) aiWorkspaceListMembers(tc *aitool.ToolContext, in workspaceListMembersInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectRead)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var candidates []projectMemberCandidateResponse
	hasCandidateSearch := in.CandidateSearch != nil && strings.TrimSpace(*in.CandidateSearch) != ""
	if hasCandidateSearch {
		subject := authz.ProjectSubject{UserID: user.ID, PlatformRole: user.Role}
		if _, authErr := h.projectAuthorizer(tc.Context()).AuthorizeProject(tc.Context(), subject, project.ID, authz.ActionProjectManage); authErr != nil {
			return nil, mapProjectAuthErrorForAI(authErr)
		}
		like := "%" + strings.ToLower(strings.TrimSpace(*in.CandidateSearch)) + "%"
		if err := db.Table("users").
			Select("users.id, users.email, users.name, users.avatar_url").
			Where("users.disabled = ?", false).
			Where("(lower(users.email) like ? or lower(users.name) like ?)", like, like).
			Where("not exists (select 1 from project_members where project_members.project_id = ? and project_members.user_id = users.id)", project.ID).
			Order("users.email asc").
			Limit(50).
			Scan(&candidates).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if candidates == nil {
			candidates = []projectMemberCandidateResponse{}
		}
	}

	sortBy := stringOrDefault(in.SortBy, "createdAt")
	sortColumns := map[string]string{
		"createdAt": "project_members.created_at",
		"name":      "users.name",
		"email":     "users.email",
		"role":      "project_members.role",
	}
	orderCol, ok := sortColumns[sortBy]
	if !ok {
		return nil, aitool.BadRequest("argument_invalid", "sortBy must be createdAt, name, email, or role")
	}
	sortOrder := strings.ToLower(stringOrDefault(in.SortOrder, "asc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, aitool.BadRequest("argument_invalid", "sortOrder must be asc or desc")
	}
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	query := db.Table("project_members").
		Select("project_members.id, project_members.project_id, project_members.user_id, project_members.role, users.email, users.name").
		Joins("join users on users.id = project_members.user_id").
		Where("project_members.project_id = ?", project.ID)

	if in.Member != nil && strings.TrimSpace(*in.Member) != "" {
		ref := strings.TrimSpace(*in.Member)
		query = query.Where("(project_members.id = ? OR project_members.user_id = ? OR lower(users.email) = ?)", ref, ref, strings.ToLower(ref))
	}
	if in.Search != nil && strings.TrimSpace(*in.Search) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(*in.Search)) + "%"
		query = query.Where("(lower(users.email) like ? or lower(users.name) like ? or lower(project_members.role) like ?)", like, like, like)
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var members []projectMemberResponse
	if err := query.Order(orderCol + " " + sortOrder + ", project_members.id asc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Scan(&members).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if members == nil {
		members = []projectMemberResponse{}
	}

	env := aitool.PageEnvelope(members, page, pageSize, sortBy, sortOrder, total)
	env["projectId"] = project.ID
	if hasCandidateSearch {
		env["candidates"] = candidates
	}
	return env, nil
}

func allowlistEntryViews(raw string) []workspaceAllowlistEntryView {
	rules, err := runtimecluster.ParseEgressAllowlist(raw)
	if err != nil || len(rules) == 0 {
		return []workspaceAllowlistEntryView{}
	}
	out := make([]workspaceAllowlistEntryView, 0, len(rules))
	for _, r := range rules {
		out = append(out, workspaceAllowlistEntryView{
			CIDR:  r.CIDR,
			Ports: r.Ports,
		})
	}
	return out
}

func (h *Handler) aiWorkspaceGetNetworkPolicy(tc *aitool.ToolContext, in workspaceGetNetworkPolicyInput) (map[string]any, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectRead)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	exceptions, err := projectEgressExceptions(db, project.ID)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", "egress exception lookup failed")
	}

	return map[string]any{
		"id":               project.ID,
		"projectId":        project.ID,
		"ingressMode":      project.NetworkIngressMode,
		"egressMode":       project.NetworkEgressMode,
		"egressAllowlist":  project.NetworkEgressAllowlist,
		"allowlistEntries": allowlistEntryViews(project.NetworkEgressAllowlist),
		"egressExceptions": exceptions,
	}, nil
}

func (h *Handler) aiWorkspaceListNetworkShares(tc *aitool.ToolContext, in workspaceListNetworkSharesInput) (map[string]any, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectRead)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	direction := strings.TrimSpace(in.Direction)
	column := ""
	switch direction {
	case "provided":
		column = "provider_project_id"
	case "consumed":
		column = "consumer_project_id"
	default:
		return nil, aitool.BadRequest("network_share.direction_invalid", "direction must be provided or consumed")
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	query := db.Model(&model.NetworkServiceShare{}).Where(column+" = ?", project.ID)
	if in.ShareID != nil && strings.TrimSpace(*in.ShareID) != "" {
		query = query.Where("id = ?", strings.TrimSpace(*in.ShareID))
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", "network share lookup failed")
	}

	var shares []model.NetworkServiceShare
	if err := query.Order("created_at desc, id asc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&shares).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", "network share lookup failed")
	}

	items, err := NetworkServiceShareResponses(db, shares)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", "network share lookup failed")
	}

	env := aitool.PageEnvelope(items, page, pageSize, "createdAt", "desc", total)
	env["projectId"] = project.ID
	env["direction"] = direction
	return env, nil
}
