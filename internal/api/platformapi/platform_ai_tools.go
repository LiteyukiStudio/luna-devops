package platformapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	dashboardservice "github.com/LiteyukiStudio/devops/internal/dashboard"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/platformevent"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/retention"
	"gorm.io/gorm"
)

type opsSearchGlobalInput struct {
	Query            *string  `json:"query,omitempty" min:"1" max:"160" desc:"搜索关键词（1..160 字符）；省略时仅返回工作台 Dashboard 概览与实时集群/制品库就绪状态"`
	Visibility       *string  `json:"visibility,omitempty" enum:"related|all" default:"related" desc:"可见性范围：related（默认，本人相关项目）或 all（仅平台管理员在全局会话下可用）"`
	Types            []string `json:"types,omitempty" enum:"project|application|deployment_target|gateway_route|repository_binding|project_volume|runtime_config_set|build_variable_set|notification_channel|runtime_cluster|artifact_registry|build_run|release|user" desc:"限定检索的资源类型数组"`
	LimitPerType     *int     `json:"limitPerType,omitempty" min:"1" max:"20" default:"6" desc:"每类资源返回条数上限（默认 6，上限 20）"`
	IncludeDashboard *bool    `json:"includeDashboard,omitempty" default:"false" desc:"传入 query 时是否同时附带 Dashboard 概览"`
}

type opsListEventsInput struct {
	Visibility     *string `json:"visibility,omitempty" enum:"related|all" default:"related" desc:"事件可见性范围：related（默认）或 all（仅平台管理员在全局会话可用）"`
	EventID        *string `json:"eventId,omitempty" desc:"精确查询单条平台事件 ID 并返回完整 detail"`
	Project        *string `json:"project,omitempty" desc:"项目 ID 或唯一 identifier（项目绑定会话省略时自动注入当前项目）"`
	Application    *string `json:"application,omitempty" desc:"应用 ID、唯一 identifier 或唯一名称（需配合 project）"`
	Target         *string `json:"target,omitempty" desc:"部署目标 ID、stage 或唯一名称（需配合 project 与 application）"`
	Category       *string `json:"category,omitempty" desc:"按事件分类过滤"`
	Type           *string `json:"type,omitempty" desc:"按精确事件类型过滤"`
	Severity       *string `json:"severity,omitempty" enum:"info|warning|error" desc:"按严重度过滤"`
	Status         *string `json:"status,omitempty" enum:"in_progress|succeeded|failed|canceled" desc:"按事件状态过滤"`
	DateFrom       *string `json:"dateFrom,omitempty" desc:"起始时间（RFC3339 或 YYYY-MM-DD）"`
	DateTo         *string `json:"dateTo,omitempty" desc:"结束时间（RFC3339 或 YYYY-MM-DD）"`
	Search         *string `json:"search,omitempty" desc:"按事件类型、消息或资源 ID 模糊搜索"`
	IncludeCatalog *bool   `json:"includeCatalog,omitempty" default:"false" desc:"是否附带平台标准事件类型目录"`
	SortBy         *string `json:"sortBy,omitempty" enum:"occurredAt|createdAt|severity|type|category" default:"occurredAt" desc:"排序字段"`
	SortOrder      *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page           *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码（>=1）"`
	PageSize       *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（默认 20，上限 50）"`
}

type adminPreviewDataRetentionInput struct {
	Datasets []string `json:"datasets,omitempty" enum:"platform_events|notification_deliveries|build_logs|release_logs|hook_run_logs|expired_auth_data" desc:"待预览清理条数的数据集列表；省略时仅返回 6 类可清理数据集定义目录"`
	StartAt  *string  `json:"startAt,omitempty" desc:"RFC3339 起始时间（传 datasets 时必填）"`
	EndAt    *string  `json:"endAt,omitempty" desc:"RFC3339 结束时间（传 datasets 时必填，须晚于 startAt）"`
}

type adminExecuteDataRetentionCleanupInput struct {
	Datasets []string `json:"datasets" required:"true" min:"1" enum:"platform_events|notification_deliveries|build_logs|release_logs|hook_run_logs|expired_auth_data" desc:"待执行不可逆历史清理的数据集列表"`
	StartAt  string   `json:"startAt" required:"true" desc:"RFC3339 起始时间"`
	EndAt    string   `json:"endAt" required:"true" desc:"RFC3339 结束时间（须晚于 startAt）"`
}

type dataRetentionLockedPayload struct {
	Datasets []string `json:"datasets"`
	StartAt  string   `json:"startAt"`
	EndAt    string   `json:"endAt"`
}

func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[opsSearchGlobalInput, map[string]any]{
		Name:           "ops_search_global",
		Domain:         aitool.DomainOps,
		Title:          "全局搜索与工作台概览",
		Summary:        "全局跨项目快速搜索平台资源，或读取工作台 Dashboard 概览统计与集群/制品库实时状态",
		AliasesZH:      []string{"全局搜索", "跨项目检索", "工作台概览", "仪表盘", "Dashboard"},
		AliasesEN:      []string{"global search", "dashboard", "overview"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionDashboardRead)},
		Execute:        h.executeOpsSearchGlobal,
	})

	aitool.Register(reg, aitool.Spec[opsListEventsInput, map[string]any]{
		Name:           "ops_list_events",
		Domain:         aitool.DomainOps,
		Title:          "查询平台事件流",
		Summary:        "分页查询可见范围内的平台事件流，支持精确查看单条事件详情及标准事件类型目录",
		AliasesZH:      []string{"平台事件", "事件日志", "活动记录", "故障事件"},
		AliasesEN:      []string{"platform events", "event stream", "event detail"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionEventRead)},
		Execute:        h.executeOpsListEvents,
	})

	aitool.Register(reg, aitool.Spec[adminPreviewDataRetentionInput, map[string]any]{
		Name:           "admin_preview_data_retention",
		Domain:         aitool.DomainAdmin,
		Title:          "预览历史数据保留与清理范围",
		Summary:        "查看平台 6 类可清理历史数据集定义目录，或零副作用预览指定时间区间将匹配的记录条数",
		AliasesZH:      []string{"数据保留", "数据保留策略", "查看数据保留策略", "清理预览", "历史清理目录"},
		AliasesEN:      []string{"data retention preview", "retention catalog"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionDataRetentionRead)},
		Execute:        h.executeAdminPreviewDataRetention,
	})

	aitool.Register(reg, aitool.Spec[adminExecuteDataRetentionCleanupInput, map[string]any]{
		Name:           "admin_execute_data_retention_cleanup",
		Domain:         aitool.DomainAdmin,
		Title:          "执行历史数据保留清理",
		Summary:        "对指定历史数据集与时间区间执行不可逆清理，并返回实际删除行数",
		AliasesZH:      []string{"执行数据清理", "清理数据保留策略", "数据保留清理", "清理历史日志", "清理过期数据"},
		AliasesEN:      []string{"execute data retention cleanup", "purge historical logs"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectExecute,
		RequiredScopes: []string{string(authz.ActionDataRetentionManage)},
		Preview:        h.previewAdminExecuteDataRetentionCleanup,
		Execute:        h.executeAdminExecuteDataRetentionCleanup,
	})
}

func (h *Handler) executeOpsSearchGlobal(tc *aitool.ToolContext, in opsSearchGlobalInput) (map[string]any, error) {
	tc.SetNoStore()

	vis, err := resolveAIToolVisibility(in.Visibility, tc.User, tc.ConversationProjectID)
	if err != nil {
		return nil, err
	}

	convProjectID := strings.TrimSpace(tc.ConversationProjectID)
	if convProjectID != "" {
		if _, _, ok := h.host.AuthorizeProjectByID(tc.Gin, convProjectID, authz.ActionProjectRead); !ok {
			return nil, tc.AbortedError("project.forbidden", "cannot access bound conversation project")
		}
	}

	rawQuery := strings.TrimSpace(derefPlatformStr(in.Query))
	if rawQuery != "" && utf8.RuneCountInString(rawQuery) > maxSearchQueryRunes {
		return nil, aitool.BadRequest("search.query_too_long", "query must be at most 160 characters")
	}

	limitPerType := defaultSearchLimitPerType
	if in.LimitPerType != nil {
		if *in.LimitPerType < 1 || *in.LimitPerType > maxSearchLimitPerType {
			return nil, aitool.BadRequest("search.invalid_limit", "limitPerType must be between 1 and 20")
		}
		limitPerType = *in.LimitPerType
	}

	explicitTypes, err := validateRequestedSearchTypes(in.Types)
	if err != nil {
		return nil, err
	}

	includeDashboard := rawQuery == "" || (in.IncludeDashboard != nil && *in.IncludeDashboard)
	out := map[string]any{
		"visibility": string(vis),
	}
	if convProjectID != "" {
		out["projectId"] = convProjectID
	}

	if rawQuery != "" {
		items, searchErr := h.runGlobalSearch(tc, rawQuery, vis, convProjectID, explicitTypes, limitPerType)
		if searchErr != nil {
			return nil, searchErr
		}
		out["query"] = rawQuery
		out["items"] = items
	}

	if includeDashboard {
		overview, dashErr := h.buildDashboardOverview(tc, vis, convProjectID)
		if dashErr != nil {
			return nil, dashErr
		}
		out["dashboard"] = overview
	}

	return out, nil
}

func (h *Handler) runGlobalSearch(
	tc *aitool.ToolContext,
	rawQuery string,
	visibility projectservice.ListVisibility,
	convProjectID string,
	explicitTypes []string,
	limitPerType int,
) ([]SearchResultItem, error) {
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}
	parsed := parseSearchQuery(rawQuery)
	if parsed.emptyTerm {
		return []SearchResultItem{}, nil
	}
	execScope, err := h.resolveSearchExecutionScope(tc.Gin, tc.User, visibility, convProjectID, limitPerType)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "search.load_failed", err.Error())
	}

	selectedTypes := resolveCandidateSearchTypes(parsed, explicitTypes)
	branches := make([]searchSQLBranch, 0, len(selectedTypes))
	for _, itemType := range selectedTypes {
		if !h.searchTypeAllowedByRequestScope(tc.Gin, execScope, itemType) {
			continue
		}
		branch, include := buildSearchBranch(itemType, parsed, execScope)
		if include {
			branches = append(branches, branch)
		}
	}
	if len(branches) == 0 {
		return []SearchResultItem{}, nil
	}

	unionParts := make([]string, 0, len(branches))
	unionArgs := make([]any, 0, len(branches)*8)
	for index, branch := range branches {
		unionParts = append(unionParts, fmt.Sprintf("SELECT * FROM (%s) AS search_branch_%d", branch.sql, index))
		unionArgs = append(unionArgs, branch.args...)
	}
	finalSQL := fmt.Sprintf(
		"SELECT * FROM (%s) AS search_union ORDER BY is_related DESC, match_rank ASC, kind_order ASC, updated_at DESC, id ASC",
		strings.Join(unionParts, " UNION ALL "),
	)

	var rows []searchSQLRow
	if err := tc.DB.Raw(finalSQL, unionArgs...).Scan(&rows).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "search.load_failed", err.Error())
	}

	items := make([]SearchResultItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, SearchResultItem{
			Type:              row.ItemType,
			ID:                row.ID,
			Title:             row.Title,
			Subtitle:          row.Subtitle,
			Identifier:        row.Identifier,
			Scope:             row.Scope,
			Status:            row.Status,
			Related:           row.IsRelated,
			ProjectID:         row.ProjectID,
			ProjectName:       row.ProjectName,
			ProjectIdentifier: row.ProjectIdentifier,
			ApplicationID:     row.ApplicationID,
			ApplicationName:   row.ApplicationName,
			UpdatedAt:         row.UpdatedAt,
		})
	}
	return items, nil
}

func (h *Handler) buildDashboardOverview(
	tc *aitool.ToolContext,
	visibility projectservice.ListVisibility,
	convProjectID string,
) (dashboardservice.Overview, error) {
	if tc.DB == nil {
		return dashboardservice.Overview{}, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}
	ctx := tc.Context()
	if convProjectID == "" && authz.IsPlatformAdmin(tc.User.Role) {
		if _, err := h.ensurePlatformSystemProject(tc.User, ctx); err != nil {
			return dashboardservice.Overview{}, aitool.NewError(http.StatusInternalServerError, "dashboard.load_failed", err.Error())
		}
	}

	service := dashboardservice.NewService(tc.DB)
	if convProjectID != "" {
		// In a project-bound session, strictly confine events/attention/activities to project_id = convProjectID
		// without mixing in actor_id = user.ID events from other projects.
		overviewScope := dashboardservice.Scope{
			UserID:            "\x00",
			AllProjects:       false,
			VisibleProjectIDs: []string{convProjectID},
		}
		overview, err := service.Overview(ctx, overviewScope)
		if err != nil {
			return dashboardservice.Overview{}, aitool.NewError(http.StatusInternalServerError, "dashboard.load_failed", err.Error())
		}
		if len(overview.Projects) > 0 {
			var pinCount int64
			if err := tc.DB.Table("project_pins").
				Where("project_id = ? AND user_id = ?", convProjectID, tc.User.ID).
				Count(&pinCount).Error; err == nil {
				for i := range overview.Projects {
					if overview.Projects[i].ID == convProjectID {
						overview.Projects[i].Pinned = pinCount > 0
					}
				}
			}
		}
		readinessScope := dashboardservice.Scope{
			UserID:            tc.User.ID,
			AllProjects:       false,
			VisibleProjectIDs: []string{convProjectID},
		}
		clusters, registries, err := service.ReadinessResources(ctx, readinessScope)
		if err != nil {
			return dashboardservice.Overview{}, aitool.NewError(http.StatusInternalServerError, "dashboard.load_failed", err.Error())
		}
		overview.Readiness = h.observeDashboardReadiness(ctx, tc.User, clusters, registries)
		overview.Summary.HealthyClusters = overview.Readiness.Clusters.Available
		overview.Summary.TotalClusters = overview.Readiness.Clusters.Total
		return overview, nil
	}

	allProjects := visibility == projectservice.ListVisibilityAll
	var projectIDs []string
	if !allProjects {
		projectIDs = h.projectIDsForUser(ctx, tc.User.ID)
	}
	scope := dashboardservice.Scope{
		UserID:            tc.User.ID,
		AllProjects:       allProjects,
		VisibleProjectIDs: projectIDs,
	}
	overview, err := service.Overview(ctx, scope)
	if err != nil {
		return dashboardservice.Overview{}, aitool.NewError(http.StatusInternalServerError, "dashboard.load_failed", err.Error())
	}
	clusters, registries, err := service.ReadinessResources(ctx, scope)
	if err != nil {
		return dashboardservice.Overview{}, aitool.NewError(http.StatusInternalServerError, "dashboard.load_failed", err.Error())
	}
	overview.Readiness = h.observeDashboardReadiness(ctx, tc.User, clusters, registries)
	overview.Summary.HealthyClusters = overview.Readiness.Clusters.Available
	overview.Summary.TotalClusters = overview.Readiness.Clusters.Total
	return overview, nil
}

func (h *Handler) executeOpsListEvents(tc *aitool.ToolContext, in opsListEventsInput) (map[string]any, error) {
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	vis, err := resolveAIToolVisibility(in.Visibility, tc.User, tc.ConversationProjectID)
	if err != nil {
		return nil, err
	}

	convProjectID := strings.TrimSpace(tc.ConversationProjectID)
	projectRef := strings.TrimSpace(derefPlatformStr(in.Project))
	if projectRef == "" && convProjectID != "" {
		projectRef = convProjectID
	}

	effectiveProjectID := ""
	if projectRef != "" {
		resolvedProj, projErr := aitool.ResolveProject(tc.DB, projectRef)
		if projErr != nil {
			return nil, projErr
		}
		if convProjectID != "" && resolvedProj.ID != convProjectID {
			mismatch := aitool.Forbidden("ai.project_scope_mismatch", "requested project does not match bound conversation project")
			mismatch.Hint = "Switch to an unbound global conversation when querying events across projects."
			return nil, mismatch
		}
		if _, authProj, ok := h.host.AuthorizeProjectByID(tc.Gin, resolvedProj.ID, authz.ActionProjectRead); !ok {
			return nil, tc.AbortedError("project.forbidden", "cannot access requested project")
		} else {
			effectiveProjectID = authProj.ID
		}
	}

	effectiveApplicationID := ""
	if appRef := strings.TrimSpace(derefPlatformStr(in.Application)); appRef != "" {
		if effectiveProjectID == "" {
			return nil, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"project.required",
				"project is required when filtering events by application",
				"Pass the project parameter alongside application.",
			)
		}
		app, appErr := aitool.ResolveApplication(tc.DB, effectiveProjectID, appRef)
		if appErr != nil {
			return nil, appErr
		}
		effectiveApplicationID = app.ID
	}

	effectiveTargetID := ""
	if targetRef := strings.TrimSpace(derefPlatformStr(in.Target)); targetRef != "" {
		if effectiveProjectID == "" || effectiveApplicationID == "" {
			return nil, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"application.required",
				"project and application are required when filtering events by target",
				"Pass both project and application alongside target.",
			)
		}
		tgt, tgtErr := aitool.ResolveDeploymentTarget(tc.DB, effectiveProjectID, effectiveApplicationID, targetRef)
		if tgtErr != nil {
			return nil, tgtErr
		}
		effectiveTargetID = tgt.ID
	}

	var dateFromVal *time.Time
	if rawFrom := strings.TrimSpace(derefPlatformStr(in.DateFrom)); rawFrom != "" {
		parsed, ok := parsePlatformEventTime(rawFrom, false)
		if !ok {
			return nil, aitool.BadRequest("event.date_invalid", "dateFrom must be RFC3339 or YYYY-MM-DD")
		}
		dateFromVal = &parsed
	}
	var dateToVal *time.Time
	if rawTo := strings.TrimSpace(derefPlatformStr(in.DateTo)); rawTo != "" {
		parsed, ok := parsePlatformEventTime(rawTo, true)
		if !ok {
			return nil, aitool.BadRequest("event.date_invalid", "dateTo must be RFC3339 or YYYY-MM-DD")
		}
		dateToVal = &parsed
	}

	if sev := strings.TrimSpace(derefPlatformStr(in.Severity)); sev != "" {
		switch sev {
		case "info", "warning", "error":
		default:
			return nil, aitool.BadRequest("event.severity_invalid", "severity must be info, warning, or error")
		}
	}
	if st := strings.TrimSpace(derefPlatformStr(in.Status)); st != "" {
		switch st {
		case "in_progress", "succeeded", "failed", "canceled":
		default:
			return nil, aitool.BadRequest("event.status_invalid", "status must be in_progress, succeeded, failed, or canceled")
		}
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	sortBy := strings.TrimSpace(derefPlatformStr(in.SortBy))
	if sortBy == "" {
		sortBy = "occurredAt"
	}
	allowedSort := map[string]string{
		"occurredAt": "occurred_at",
		"createdAt":  "created_at",
		"severity":   "severity",
		"type":       "type",
		"category":   "category",
	}
	sortCol, ok := allowedSort[sortBy]
	if !ok {
		return nil, aitool.BadRequest("event.sort_invalid", "unsupported sortBy field")
	}
	sortOrder := strings.ToLower(strings.TrimSpace(derefPlatformStr(in.SortOrder)))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, aitool.BadRequest("event.sort_order_invalid", "sortOrder must be asc or desc")
	}

	includeCatalog := in.IncludeCatalog != nil && *in.IncludeCatalog

	if eventID := strings.TrimSpace(derefPlatformStr(in.EventID)); eventID != "" {
		var event model.PlatformEvent
		if err := tc.DB.First(&event, "id = ?", eventID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("event.not_found", "platform event was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if effectiveProjectID != "" {
			if event.ProjectID != effectiveProjectID {
				if convProjectID != "" {
					mismatch := aitool.Forbidden("ai.project_scope_mismatch", "event does not belong to the bound conversation project")
					mismatch.Hint = "Switch to an unbound global conversation when inspecting events from another project."
					return nil, mismatch
				}
				return nil, aitool.NotFound("event.not_found", "platform event was not found in the specified project")
			}
		} else if !h.canReadPlatformEvent(tc.Context(), tc.User, event) {
			return nil, aitool.Forbidden("event.forbidden", "you cannot access this event")
		}

		var deliveryCount int64
		if authz.IsPlatformAdmin(tc.User.Role) {
			if err := tc.DB.Model(&model.NotificationDelivery{}).Where("event_id = ?", event.ID).Count(&deliveryCount).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
		}
		var resourceNames map[string]string
		var nameErr error
		if effectiveProjectID != "" {
			resourceNames, nameErr = (platformevent.Service{DB: tc.DB}).ResourceNames(tc.Context(), []model.PlatformEvent{event})
		} else {
			resourceNames, nameErr = h.platformEventResourceNames(tc.Context(), tc.User, []model.PlatformEvent{event})
		}
		if nameErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", nameErr.Error())
		}
		item := platformEventResponseFor(event, deliveryCount)
		item.ResourceName = resourceNames[event.ID]

		env := aitool.PageEnvelope([]platformEventResponse{item}, 1, pageSize, sortBy, sortOrder, 1)
		env["event"] = item
		if effectiveProjectID != "" {
			env["projectId"] = effectiveProjectID
		}
		if includeCatalog {
			env["catalog"] = platformevent.Catalog()
		}
		return env, nil
	}

	var query *gorm.DB
	if effectiveProjectID != "" {
		// AuthorizeProjectByID has already verified access (including non-member platform_admin
		// and rejecting removed former members), so filter directly on project_id.
		query = tc.DB.Model(&model.PlatformEvent{}).Where("platform_events.project_id = ?", effectiveProjectID)
	} else {
		query = h.platformEventsVisibleTo(tc.User, vis, tc.Context())
	}

	if effectiveApplicationID != "" {
		query = query.Where("platform_events.application_id = ?", effectiveApplicationID)
	}
	if effectiveTargetID != "" {
		query = query.Where("platform_events.deployment_target_id = ?", effectiveTargetID)
	}
	if cat := strings.TrimSpace(derefPlatformStr(in.Category)); cat != "" {
		query = query.Where("platform_events.category = ?", cat)
	}
	if evtType := strings.TrimSpace(derefPlatformStr(in.Type)); evtType != "" {
		query = query.Where("platform_events.type = ?", evtType)
	}
	if sev := strings.TrimSpace(derefPlatformStr(in.Severity)); sev != "" {
		query = query.Where("platform_events.severity = ?", sev)
	}
	if st := strings.TrimSpace(derefPlatformStr(in.Status)); st != "" {
		query = query.Where("platform_events.status = ?", st)
	}
	if dateFromVal != nil {
		query = query.Where("platform_events.occurred_at >= ?", *dateFromVal)
	}
	if dateToVal != nil {
		query = query.Where("platform_events.occurred_at <= ?", *dateToVal)
	}
	if search := strings.TrimSpace(derefPlatformStr(in.Search)); search != "" {
		likePattern := "%" + escapeSQLLikeLiteral(strings.ToLower(search)) + "%"
		query = query.Where(
			`(LOWER(platform_events.type) LIKE ? ESCAPE '\' OR LOWER(platform_events.message) LIKE ? ESCAPE '\' OR LOWER(platform_events.resource_id) LIKE ? ESCAPE '\')`,
			likePattern, likePattern, likePattern,
		)
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var events []model.PlatformEvent
	if err := query.Session(&gorm.Session{}).
		Order(fmt.Sprintf("%s %s, id desc", sortCol, sortOrder)).
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&events).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var resourceNames map[string]string
	var nameErr error
	if effectiveProjectID != "" {
		resourceNames, nameErr = (platformevent.Service{DB: tc.DB}).ResourceNames(tc.Context(), events)
	} else {
		resourceNames, nameErr = h.platformEventResourceNames(tc.Context(), tc.User, events)
	}
	if nameErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", nameErr.Error())
	}

	responses := make([]platformEventResponse, 0, len(events))
	for _, evt := range events {
		resp := platformEventResponseFor(evt, 0)
		resp.ResourceName = resourceNames[evt.ID]
		responses = append(responses, resp)
	}

	env := aitool.PageEnvelope(responses, page, pageSize, sortBy, sortOrder, total)
	if effectiveProjectID != "" {
		env["projectId"] = effectiveProjectID
	}
	if effectiveApplicationID != "" {
		env["applicationId"] = effectiveApplicationID
	}
	if effectiveTargetID != "" {
		env["deploymentTargetId"] = effectiveTargetID
	}
	if includeCatalog {
		env["catalog"] = platformevent.Catalog()
	}
	return env, nil
}

func (h *Handler) executeAdminPreviewDataRetention(tc *aitool.ToolContext, in adminPreviewDataRetentionInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	rawStart := strings.TrimSpace(derefPlatformStr(in.StartAt))
	rawEnd := strings.TrimSpace(derefPlatformStr(in.EndAt))

	if len(in.Datasets) == 0 {
		if rawStart != "" || rawEnd != "" {
			return nil, aitool.BadRequest("retention.invalid_dataset", "datasets is required when startAt or endAt is provided")
		}
		cat := retention.Catalog()
		return map[string]any{
			"mode":    "catalog",
			"catalog": cat,
			"items":   cat,
		}, nil
	}

	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	parsed, err := parseDataRetentionRange(dataRetentionRequest{
		Datasets: in.Datasets,
		StartAt:  rawStart,
		EndAt:    rawEnd,
	})
	if err != nil {
		return nil, aitool.BadRequest("retention.invalid_range", err.Error())
	}

	items, err := retention.NewService(tc.DB).Preview(tc.Context(), parsed.Datasets, parsed.StartAt, parsed.EndAt, time.Now())
	if err != nil {
		return nil, mapDataRetentionToolError(err)
	}

	var totalMatched int64
	for _, item := range items {
		totalMatched += item.Matched
	}
	return map[string]any{
		"mode":         "preview",
		"startAt":      parsed.StartAt.UTC().Format(time.RFC3339),
		"endAt":        parsed.EndAt.UTC().Format(time.RFC3339),
		"totalMatched": totalMatched,
		"items":        items,
	}, nil
}

func (h *Handler) previewAdminExecuteDataRetentionCleanup(tc *aitool.ToolContext, in adminExecuteDataRetentionCleanupInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	parsed, err := parseDataRetentionRange(dataRetentionRequest{
		Datasets: in.Datasets,
		StartAt:  in.StartAt,
		EndAt:    in.EndAt,
	})
	if err != nil {
		return nil, aitool.BadRequest("retention.invalid_range", err.Error())
	}

	items, err := retention.NewService(tc.DB).Preview(tc.Context(), parsed.Datasets, parsed.StartAt, parsed.EndAt, time.Now())
	if err != nil {
		return nil, mapDataRetentionToolError(err)
	}

	var totalMatched int64
	changes := make([]aitool.PreviewChange, 0, len(items))
	impact := make([]string, 0, len(items))
	lockedKeys := make([]string, 0, len(items))
	for _, item := range items {
		totalMatched += item.Matched
		lockedKeys = append(lockedKeys, item.Dataset)
		changes = append(changes, aitool.PreviewChange{
			Field:  item.Dataset,
			Before: item.Matched,
			After:  int64(0),
			Action: "delete",
		})
		impact = append(impact, fmt.Sprintf("%s: %d matched rows", item.Dataset, item.Matched))
	}

	startNorm := parsed.StartAt.UTC().Format(time.RFC3339)
	endNorm := parsed.EndAt.UTC().Format(time.RFC3339)
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Clean up %d historical dataset(s) [%s .. %s], matching %d row(s)", len(items), startNorm, endNorm, totalMatched),
			TargetResource: "data_retention",
			Changes:        changes,
			Impact:         impact,
			Warnings:       []string{"Historical data retention cleanup is irreversible."},
		},
		LockedPayload: dataRetentionLockedPayload{
			Datasets: lockedKeys,
			StartAt:  startNorm,
			EndAt:    endNorm,
		},
	}, nil
}

func (h *Handler) executeAdminExecuteDataRetentionCleanup(tc *aitool.ToolContext, in adminExecuteDataRetentionCleanupInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	parsed, err := parseDataRetentionRange(dataRetentionRequest{
		Datasets: in.Datasets,
		StartAt:  in.StartAt,
		EndAt:    in.EndAt,
	})
	if err != nil {
		h.auditWithContext(tc.User.ID, dataRetentionCleanupAction, "data_retention", false, "invalid cleanup request", tc.Context())
		return nil, aitool.BadRequest("retention.invalid_range", err.Error())
	}

	items, err := retention.NewService(tc.DB).Cleanup(tc.Context(), parsed.Datasets, parsed.StartAt, parsed.EndAt, time.Now())
	if err != nil {
		h.auditWithContext(tc.User.ID, dataRetentionCleanupAction, "data_retention", false, dataRetentionFailureSummary(err), tc.Context())
		return nil, mapDataRetentionToolError(err)
	}
	h.auditWithContext(tc.User.ID, dataRetentionCleanupAction, "data_retention", true, dataRetentionResultSummary(items), tc.Context())

	var totalMatched, totalDeleted int64
	for _, item := range items {
		totalMatched += item.Matched
		totalDeleted += item.Deleted
	}
	return map[string]any{
		"startAt":      parsed.StartAt.UTC().Format(time.RFC3339),
		"endAt":        parsed.EndAt.UTC().Format(time.RFC3339),
		"totalMatched": totalMatched,
		"totalDeleted": totalDeleted,
		"items":        items,
	}, nil
}

func resolveAIToolVisibility(raw *string, user model.User, convProjectID string) (projectservice.ListVisibility, error) {
	v := strings.ToLower(strings.TrimSpace(derefPlatformStr(raw)))
	if v == "" || v == string(projectservice.ListVisibilityRelated) {
		return projectservice.ListVisibilityRelated, nil
	}
	if v != string(projectservice.ListVisibilityAll) {
		return "", aitool.BadRequest("request.invalid_visibility", "visibility must be related or all")
	}
	if strings.TrimSpace(convProjectID) != "" {
		mismatch := aitool.Forbidden("ai.project_scope_mismatch", "visibility=all is not allowed in a single-project conversation")
		mismatch.Hint = "Switch to an unbound global conversation when querying across all projects."
		return "", mismatch
	}
	if !authz.IsPlatformAdmin(user.Role) {
		return "", aitool.Forbidden("auth.forbidden", "visibility=all requires platform_admin role")
	}
	return projectservice.ListVisibilityAll, nil
}

func validateRequestedSearchTypes(rawTypes []string) ([]string, error) {
	if len(rawTypes) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(rawTypes))
	out := make([]string, 0, len(rawTypes))
	for _, raw := range rawTypes {
		normalized := strings.ToLower(strings.TrimSpace(raw))
		if normalized == "" {
			continue
		}
		if _, valid := allowedSearchTypeSet[normalized]; !valid {
			return nil, aitool.BadRequest("search.invalid_type", fmt.Sprintf("unsupported search type %q", normalized))
		}
		if !seen[normalized] {
			seen[normalized] = true
			out = append(out, normalized)
		}
	}
	return out, nil
}

func mapDataRetentionToolError(err error) error {
	switch {
	case errors.Is(err, retention.ErrInvalidRange):
		return aitool.BadRequest("retention.invalid_range", err.Error())
	case errors.Is(err, retention.ErrUnknownDataset), errors.Is(err, retention.ErrNoDatasets):
		return aitool.BadRequest("retention.invalid_dataset", err.Error())
	default:
		return aitool.NewError(http.StatusInternalServerError, "retention.cleanup_failed", "data retention operation failed")
	}
}

func derefPlatformStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
