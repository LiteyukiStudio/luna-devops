package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListProjects(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	platformAdmin := authz.IsPlatformAdmin(user.Role)
	scope, err := projectservice.ResolveListScope(ctx.Query("scope"), platformAdmin)
	if errors.Is(err, projectservice.ErrListScopeInvalid) {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", err.Error())
		return
	}
	if errors.Is(err, projectservice.ErrListScopeForbidden) {
		writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", err.Error())
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if platformAdmin {
		if _, err := h.ensurePlatformSystemProject(user, ctx.Request.Context()); err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	}

	baseQuery := h.dbFor(ctx).
		Table("projects").
		Select("projects.*, project_members.dashboard_order, project_members.last_used_at, project_members.use_count").
		Joins("left join project_members on project_members.project_id = projects.id and project_members.user_id = ?", user.ID).
		Joins("left join project_pins on project_pins.project_id = projects.id and project_pins.user_id = project_members.user_id").
		Where("projects.deleted_at is null")
	if scope == projectservice.ListScopeRelated {
		baseQuery = baseQuery.Where("project_members.user_id = ?", user.ID)
	}
	baseQuery = applySearch(ctx, baseQuery, "projects.name", "projects.identifier")

	pagination := paginationFromQueryWithSort(ctx, map[string]string{
		"createdAt": "projects.updated_at", "name": "projects.name", "identifier": "projects.identifier",
	}, "createdAt")
	query := baseQuery.Session(&gorm.Session{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	var projects []model.Project
	if err := projectPageQuery(baseQuery.Session(&gorm.Session{}), pagination).Find(&projects).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(projects, total, pagination))
}

func projectPageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return query.Order(projectListOrderClause(pagination.SortBy, pagination.SortOrder)).Limit(pagination.PageSize).Offset(pagination.Offset())
}

func (h *Handlers) CreateProject(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input projectInput
	if !bindJSON(ctx, &input) {
		return
	}
	project, err := projectservice.NewService(h.dbFor(ctx)).Create(ctx.Request.Context(), user.ID, projectservice.CreateInput{
		Identifier: input.Identifier, Name: input.Name, Description: input.Description,
		NamespaceStrategy: input.NamespaceStrategy, MaxConcurrentBuilds: input.MaxConcurrentBuilds,
		WebConsoleEnabled: input.WebConsoleEnabled,
	})
	if errors.Is(err, projectservice.ErrIdentifierInvalid) {
		writeErrorCode(ctx, http.StatusBadRequest, "project.identifier_invalid", err.Error())
		return
	}
	if errors.Is(err, projectservice.ErrIdentifierExists) {
		writeErrorCode(ctx, http.StatusConflict, "project.identifier_exists", "project identifier already exists")
		return
	}
	if errors.Is(err, projectservice.ErrIdentifierDeleteInProgress) {
		writeErrorCode(ctx, http.StatusConflict, "project.identifier_delete_in_progress", "同标识项目空间正在删除，资源清理完成后才能复用")
		return
	}
	if errors.Is(err, projectservice.ErrIdentifierDeleteFailed) {
		writeErrorCode(ctx, http.StatusConflict, "project.identifier_delete_failed", "同标识项目空间上次删除失败，请先完成资源清理")
		return
	}
	if errors.Is(err, projectservice.ErrInputInvalid) {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "project input is invalid")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, h.projectResponse(project, ctx.Request.Context()))
}

func (h *Handlers) GetProject(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUser(ctx)
	if !ok {
		return
	}
	h.recordProjectUsage(user.ID, project.ID, ctx.Request.Context())
	response := h.projectResponse(project, ctx.Request.Context())
	if role, exists := ctx.Get(currentProjectRoleContextKey); exists {
		response.CurrentUserRole, _ = role.(string)
	}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handlers) UpdateProject(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}

	var input projectInput
	if !bindJSON(ctx, &input) {
		return
	}
	input.Identifier = strings.TrimSpace(input.Identifier)
	if input.Identifier != project.Identifier {
		writeErrorCode(ctx, http.StatusConflict, "project.identifier_immutable", "project identifier cannot be changed")
		return
	}

	project.Name = input.Name
	project.Description = input.Description
	project.NamespaceStrategy = fallback(input.NamespaceStrategy, "project")
	project.MaxConcurrentBuilds = normalizeBuildConcurrency(input.MaxConcurrentBuilds, defaultProjectBuildConcurrency)
	if input.WebConsoleEnabled != nil {
		project.WebConsoleEnabled = *input.WebConsoleEnabled
	}

	if err := h.dbFor(ctx).Save(&project).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.projectResponse(project, ctx.Request.Context()))
}

func (h *Handlers) DeleteProject(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, authz.ProjectRoleOwner)
	if !ok {
		return
	}
	if !deleteStatusCanStart(project.DeleteStatus) {
		writeErrorCode(ctx, http.StatusConflict, "project.delete_in_progress", "项目空间正在删除中，请等待资源清理完成")
		return
	}
	if isSystemProject(project) {
		writeErrorCode(ctx, http.StatusForbidden, "project.system_protected", "平台系统项目空间不能删除")
		return
	}
	if err := markResourceDeleting(h.dbFor(ctx), &model.Project{}, project.ID); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueResourceCleanup(ctx.Request.Context(), tasks.ResourceCleanupPayload{
		ResourceType: "project",
		ResourceID:   project.ID,
		ProjectID:    project.ID,
		ActorID:      user.ID,
		DeleteData:   true,
	}) {
		_ = markResourceDeleteFailed(h.dbFor(ctx), &model.Project{}, project.ID, "资源清理任务投递失败，请稍后重试")
		writeError(ctx, http.StatusServiceUnavailable, "资源清理任务投递失败，请稍后重试")
		return
	}
	h.auditWithContext(user.ID, "project.delete", project.ID, true, project.Name, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListProjectPins(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	pagination := paginationFromQueryWithSort(ctx, map[string]string{
		"pinnedAt": "project_pins.pinned_at", "lastUsedAt": "project_members.last_used_at",
		"name": "projects.name", "useCount": "project_members.use_count",
	}, "pinnedAt")
	query := h.dbFor(ctx).Table("project_pins").
		Joins("join projects on projects.id = project_pins.project_id and projects.deleted_at is null").
		Joins("join project_members on project_members.project_id = projects.id and project_members.user_id = project_pins.user_id").
		Where("project_pins.user_id = ?", user.ID)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var rows []projectPinResponse
	err := query.
		Select("projects.id, projects.identifier, projects.kubernetes_namespace, projects.name, projects.description, projects.namespace_strategy, projects.created_at, project_members.dashboard_order, project_members.last_used_at, project_members.use_count, project_pins.pinned_at").
		Order(orderByClause(pagination, map[string]string{
			"pinnedAt":   "project_pins.pinned_at",
			"lastUsedAt": "project_members.last_used_at",
			"name":       "projects.name",
			"useCount":   "project_members.use_count",
		}, "project_pins.pinned_at")).
		Limit(pagination.PageSize).
		Offset(pagination.Offset()).
		Scan(&rows).Error
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(rows, total, pagination))
}

func (h *Handlers) PinProject(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUser(ctx)
	if !ok {
		return
	}

	now := time.Now()
	var pin model.ProjectPin
	err := h.dbFor(ctx).First(&pin, "user_id = ? and project_id = ?", user.ID, project.ID).Error
	if err == nil {
		pin.PinnedAt = now
		if err := h.dbFor(ctx).Save(&pin).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, projectPinResponseFrom(project, pin, h.projectDashboardOrder(user.ID, project.ID, ctx.Request.Context())))
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	pin = model.ProjectPin{
		ID:        id.New("ppin"),
		UserID:    user.ID,
		ProjectID: project.ID,
		PinnedAt:  now,
	}
	if err := h.dbFor(ctx).Create(&pin).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, projectPinResponseFrom(project, pin, h.projectDashboardOrder(user.ID, project.ID, ctx.Request.Context())))
}

func (h *Handlers) UnpinProject(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUser(ctx)
	if !ok {
		return
	}

	if err := h.dbFor(ctx).Delete(&model.ProjectPin{}, "user_id = ? and project_id = ?", user.ID, project.ID).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) UpdateProjectOrder(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input projectOrderInput
	if !bindJSON(ctx, &input) {
		return
	}
	projectIDs := normalizedProjectOrderIDs(input.ProjectIDs)
	if len(projectIDs) == 0 {
		writeError(ctx, http.StatusBadRequest, "项目空间排序不能为空")
		return
	}
	if len(projectIDs) > 8 {
		writeError(ctx, http.StatusBadRequest, "看板最多展示 8 个项目空间")
		return
	}

	var accessibleCount int64
	if err := h.dbFor(ctx).Model(&model.ProjectMember{}).Where("user_id = ? and project_id in ?", user.ID, projectIDs).Count(&accessibleCount).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if accessibleCount != int64(len(projectIDs)) {
		writeError(ctx, http.StatusForbidden, "你没有访问部分项目空间的权限")
		return
	}

	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		for index, projectID := range projectIDs {
			if err := tx.Model(&model.ProjectMember{}).
				Where("user_id = ? and project_id = ?", user.ID, projectID).
				Update("dashboard_order", index+1).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projectIds": projectIDs})
}

func (h *Handlers) ListProjectMembers(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	var members []projectMemberResponse
	query := h.dbFor(ctx).Table("project_members").
		Select("project_members.id, project_members.project_id, project_members.user_id, project_members.role, users.email, users.name").
		Joins("join users on users.id = project_members.user_id").
		Where("project_members.project_id = ?", ctx.Param("projectId"))
	query = applySearch(ctx, query, "users.email", "users.name", "project_members.role")
	pagination := paginationFromQueryWithSort(ctx, map[string]string{
		"email": "users.email", "name": "users.name", "role": "project_members.role", "createdAt": "project_members.created_at",
	}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"email":     "users.email",
		"name":      "users.name",
		"role":      "project_members.role",
		"createdAt": "project_members.created_at",
	}, "project_members.created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Scan(&members).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(members, total, pagination))
}

func (h *Handlers) SearchProjectMemberCandidates(ctx *gin.Context) {
	_, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}

	search := strings.TrimSpace(ctx.Query("search"))
	if search == "" {
		ctx.JSON(http.StatusOK, []projectMemberCandidateResponse{})
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(ctx.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = min(max(parsed, 1), 50)
		}
	}

	like := "%" + strings.ToLower(search) + "%"
	var users []projectMemberCandidateResponse
	err := h.dbFor(ctx).Table("users").
		Select("users.id, users.email, users.name, users.avatar_url").
		Where("users.disabled = ?", false).
		Where("(lower(users.email) like ? or lower(users.name) like ?)", like, like).
		Where("not exists (select 1 from project_members where project_members.project_id = ? and project_members.user_id = users.id)", project.ID).
		Order("users.email asc").
		Limit(limit).
		Scan(&users).Error
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, users)
}

func (h *Handlers) CreateProjectMember(ctx *gin.Context) {
	actor, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}

	var input projectMemberInput
	if !bindJSON(ctx, &input) {
		return
	}

	var targetUser model.User
	userID := strings.TrimSpace(input.UserID)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	switch {
	case userID != "":
		if err := h.dbFor(ctx).First(&targetUser, "id = ? and disabled = ?", userID, false).Error; err != nil {
			writeError(ctx, http.StatusNotFound, "user not found")
			return
		}
	case email != "":
		if err := h.dbFor(ctx).First(&targetUser, "email = ? and disabled = ?", email, false).Error; err != nil {
			writeError(ctx, http.StatusNotFound, "user not found")
			return
		}
	default:
		writeError(ctx, http.StatusBadRequest, "user is required")
		return
	}

	role := normalizeProjectRole(input.Role)
	if role == authz.ProjectRoleOwner && !h.currentProjectRoleAllows(ctx, project.ID, actor.ID, authz.ProjectRoleOwner) {
		writeError(ctx, http.StatusForbidden, "只有项目 owner 可以授予 owner 角色")
		return
	}

	member := model.ProjectMember{
		ID:        id.New("mem"),
		ProjectID: ctx.Param("projectId"),
		UserID:    targetUser.ID,
		Role:      role,
	}
	if err := h.dbFor(ctx).First(&model.ProjectMember{}, "project_id = ? and user_id = ?", member.ProjectID, member.UserID).Error; err == nil {
		writeError(ctx, http.StatusConflict, "user is already a project member")
		return
	}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
		return publishProjectMemberInbox(ctx.Request.Context(), tx, projectMemberInboxInput{
			Type: "project.member_added", Priority: "normal", Project: project, Actor: actor,
			RecipientUserID: targetUser.ID, MemberID: member.ID, Role: member.Role,
			DedupKey: "project-member-added:" + member.ID,
		})
	}); err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "inbox.operation_failed", "project member notification failed")
		return
	}
	h.auditWithContext(actor.ID, "project_member.create", member.ID, true, member.Role, ctx.Request.Context())
	defaultInboxBroker.Notify(targetUser.ID, "")

	ctx.JSON(http.StatusCreated, projectMemberResponse{
		ID:        member.ID,
		ProjectID: member.ProjectID,
		UserID:    member.UserID,
		Role:      member.Role,
		Email:     targetUser.Email,
		Name:      targetUser.Name,
	})
}

func (h *Handlers) UpdateProjectMember(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}

	var member model.ProjectMember
	if err := h.dbFor(ctx).First(&member, "id = ? and project_id = ?", ctx.Param("memberId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "member not found")
		return
	}

	var input projectMemberInput
	if !bindJSON(ctx, &input) {
		return
	}
	nextRole := normalizeProjectRole(input.Role)
	actorIsOwner := h.currentProjectRoleAllows(ctx, project.ID, user.ID, authz.ProjectRoleOwner)
	if (member.Role == authz.ProjectRoleOwner || nextRole == authz.ProjectRoleOwner) && !actorIsOwner {
		writeError(ctx, http.StatusForbidden, "只有项目 owner 可以修改 owner 角色")
		return
	}
	if member.Role == authz.ProjectRoleOwner && nextRole != authz.ProjectRoleOwner && !h.projectHasAnotherOwner(ctx.Request.Context(), member.ProjectID, member.ID) {
		writeError(ctx, http.StatusBadRequest, "项目至少需要保留一个 owner")
		return
	}
	previousRole := member.Role
	if previousRole == nextRole {
		ctx.JSON(http.StatusOK, member)
		return
	}
	member.Role = nextRole
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&member).Error; err != nil {
			return err
		}
		return publishProjectMemberInbox(ctx.Request.Context(), tx, projectMemberInboxInput{
			Type: "project.member_role_changed", Priority: "high", Project: project, Actor: user,
			RecipientUserID: member.UserID, MemberID: member.ID, Role: member.Role, PreviousRole: previousRole,
		})
	}); err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "inbox.operation_failed", "project member notification failed")
		return
	}
	h.auditWithContext(user.ID, "project_member.update", member.ID, true, member.Role, ctx.Request.Context())
	defaultInboxBroker.Notify(member.UserID, "")
	ctx.JSON(http.StatusOK, member)
}

func (h *Handlers) DeleteProjectMember(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}

	var member model.ProjectMember
	if err := h.dbFor(ctx).First(&member, "id = ? and project_id = ?", ctx.Param("memberId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "member not found")
		return
	}
	if member.UserID == user.ID {
		writeError(ctx, http.StatusBadRequest, "不能移除当前登录账号")
		return
	}
	if member.Role == authz.ProjectRoleOwner {
		if !h.currentProjectRoleAllows(ctx, project.ID, user.ID, authz.ProjectRoleOwner) {
			writeError(ctx, http.StatusForbidden, "只有项目 owner 可以移除 owner 成员")
			return
		}
		if !h.projectHasAnotherOwner(ctx.Request.Context(), member.ProjectID, member.ID) {
			writeError(ctx, http.StatusBadRequest, "项目至少需要保留一个 owner")
			return
		}
	}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&member).Error; err != nil {
			return err
		}
		return publishProjectMemberInbox(ctx.Request.Context(), tx, projectMemberInboxInput{
			Type: "project.member_removed", Priority: "high", Project: project, Actor: user,
			RecipientUserID: member.UserID, MemberID: member.ID, Role: member.Role,
			DedupKey: "project-member-removed:" + member.ID,
		})
	}); err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "inbox.operation_failed", "project member notification failed")
		return
	}
	h.auditWithContext(user.ID, "project_member.delete", member.ID, true, member.Role, ctx.Request.Context())
	defaultInboxBroker.Notify(member.UserID, "")
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) findProject(ctx *gin.Context) (model.Project, bool) {
	var project model.Project
	if err := h.dbFor(ctx).First(&project, "id = ?", ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project not found")
		return project, false
	}
	return project, true
}

func (h *Handlers) findProjectForCurrentUser(ctx *gin.Context) (model.Project, bool) {
	return h.findProjectForCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper, authz.ProjectRoleViewer)
}

func (h *Handlers) projectAndCurrentUser(ctx *gin.Context) (model.User, model.Project, bool) {
	return h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper, authz.ProjectRoleViewer)
}

func (h *Handlers) projectAndCurrentUserWithRoles(ctx *gin.Context, allowedRoles ...string) (model.User, model.Project, bool) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return model.User{}, model.Project{}, false
	}
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, allowedRoles...)
	if !ok {
		return model.User{}, model.Project{}, false
	}
	return user, project, true
}

func (h *Handlers) findProjectForCurrentUserWithRoles(ctx *gin.Context, allowedRoles ...string) (model.Project, bool) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return model.Project{}, false
	}

	project, ok := h.findProject(ctx)
	if !ok {
		return project, false
	}

	if authz.IsPlatformAdmin(user.Role) {
		return project, true
	}

	var member model.ProjectMember
	err := h.dbFor(ctx).First(&member, "project_id = ? and user_id = ?", project.ID, user.ID).Error
	if err != nil {
		writeError(ctx, http.StatusForbidden, "你没有访问该项目的权限")
		return model.Project{}, false
	}

	if !projectUserRoleAllowed(user, member.Role, allowedRoles) {
		writeError(ctx, http.StatusForbidden, "你没有执行该项目操作的权限")
		return model.Project{}, false
	}
	ctx.Set(currentProjectRoleContextKey, member.Role)

	return project, true
}

func projectUserRoleAllowed(user model.User, memberRole string, allowedRoles []string) bool {
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	return authz.ProjectRoleAllowsLegacyRoles(memberRole, allowedRoles)
}

func projectRoleAllowed(role string, allowedRoles []string) bool {
	return authz.ProjectRoleAllowsLegacyRoles(role, allowedRoles)
}

func (h *Handlers) currentProjectRoleAllows(ctx *gin.Context, projectID, userID string, allowedRoles ...string) bool {
	var member model.ProjectMember
	if err := h.dbFor(ctx).First(&member, "project_id = ? and user_id = ?", projectID, userID).Error; err != nil {
		writeError(ctx, http.StatusForbidden, "你没有访问该项目的权限")
		return false
	}
	return projectRoleAllowed(member.Role, allowedRoles)
}

func (h *Handlers) projectHasAnotherOwner(ctx context.Context, projectID, memberID string) bool {
	return h.projects.HasAnotherOwnerContext(ctx, projectID, memberID)
}

type projectInput struct {
	Identifier          string `json:"identifier" binding:"required"`
	Name                string `json:"name" binding:"required"`
	Description         string `json:"description"`
	NamespaceStrategy   string `json:"namespaceStrategy"`
	MaxConcurrentBuilds int    `json:"maxConcurrentBuilds"`
	WebConsoleEnabled   *bool  `json:"webConsoleEnabled"`
}

type projectOrderInput struct {
	ProjectIDs []string `json:"projectIds" binding:"required"`
}

type projectPinResponse struct {
	ID                string     `json:"id"`
	Identifier        string     `json:"identifier"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	NamespaceStrategy string     `json:"namespaceStrategy"`
	CreatedAt         time.Time  `json:"createdAt"`
	DashboardOrder    int        `json:"dashboardOrder"`
	LastUsedAt        *time.Time `json:"lastUsedAt"`
	UseCount          int        `json:"useCount"`
	PinnedAt          time.Time  `json:"pinnedAt"`
}

type projectResponse struct {
	model.Project
	BillingOwner    *projectBillingOwnerResponse `json:"billingOwner,omitempty"`
	CurrentUserRole string                       `json:"currentUserRole,omitempty"`
}

type projectBillingOwnerResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

func (h *Handlers) projectResponse(project model.Project, ctx context.Context) projectResponse {
	response := projectResponse{Project: project}
	if strings.TrimSpace(project.BillingOwnerUserID) == "" {
		return response
	}

	var user model.User
	if err := h.dbWithContext(ctx).Select("id", "email", "name", "avatar_url").First(&user, "id = ?", project.BillingOwnerUserID).Error; err != nil {
		return response
	}
	response.BillingOwner = &projectBillingOwnerResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
	}
	return response
}

func projectPinResponseFrom(project model.Project, pin model.ProjectPin, dashboardOrder int) projectPinResponse {
	return projectPinResponse{
		ID:                project.ID,
		Identifier:        project.Identifier,
		Name:              project.Name,
		Description:       project.Description,
		NamespaceStrategy: project.NamespaceStrategy,
		CreatedAt:         project.CreatedAt,
		DashboardOrder:    dashboardOrder,
		LastUsedAt:        project.LastUsedAt,
		UseCount:          project.UseCount,
		PinnedAt:          pin.PinnedAt,
	}
}

func projectListOrderClause(sortBy string, sortOrder string) string {
	order := "desc"
	if sortOrder == "asc" {
		order = "asc"
	}

	switch sortBy {
	case "useCount":
		return "case when project_pins.id is null then 1 else 0 end asc, project_members.use_count " + order + ", coalesce(project_members.last_used_at, projects.created_at) desc, projects.created_at desc"
	case "createdAt":
		return "projects.created_at " + order + ", projects.id asc"
	case "updatedAt":
		return "projects.updated_at " + order + ", projects.id asc"
	case "name":
		return "projects.name " + order + ", projects.id asc"
	case "identifier":
		return "projects.identifier " + order + ", projects.id asc"
	default:
		return "coalesce(project_members.last_used_at, projects.created_at) " + order + ", projects.created_at desc"
	}
}

func (h *Handlers) recordProjectUsage(userID string, projectID string, ctx context.Context) {
	now := time.Now()
	_ = h.dbWithContext(ctx).Model(&model.ProjectMember{}).
		Where("user_id = ? and project_id = ?", userID, projectID).
		Updates(map[string]any{
			"last_used_at": now,
			"use_count":    gorm.Expr("use_count + 1"),
		}).Error
}

func (h *Handlers) projectDashboardOrder(userID string, projectID string, ctx context.Context) int {
	var member model.ProjectMember
	if err := h.dbWithContext(ctx).Select("dashboard_order").First(&member, "user_id = ? and project_id = ?", userID, projectID).Error; err != nil {
		return 0
	}
	return member.DashboardOrder
}

func normalizedProjectOrderIDs(values []string) []string {
	seen := map[string]bool{}
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		output = append(output, value)
	}
	return output
}
