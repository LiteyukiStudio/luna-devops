package api

import (
	"errors"
	"fmt"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h *Handlers) ListProjects(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	baseQuery := h.db.
		Table("projects").
		Select("projects.*, project_members.dashboard_order, project_members.last_used_at, project_members.use_count").
		Joins("left join project_members on project_members.project_id = projects.id and project_members.user_id = ?", user.ID).
		Joins("left join project_pins on project_pins.project_id = projects.id and project_pins.user_id = project_members.user_id").
		Where("projects.deleted_at is null")
	if user.Role != "platform_admin" || projectListScope(ctx.Query("scope")) == "related" {
		baseQuery = baseQuery.Where("project_members.user_id = ?", user.ID)
	}
	baseQuery = applySearch(ctx, baseQuery, "projects.name", "projects.slug")

	if ctx.Query("page") != "" || ctx.Query("pageSize") != "" {
		pagination := paginationFromQuery(ctx)
		query := baseQuery.Session(&gorm.Session{})
		var total int64
		if err := query.Count(&total).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}

		var projects []model.Project
		if err := baseQuery.Session(&gorm.Session{}).Order(projectListOrderClause(pagination.SortBy, pagination.SortOrder)).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&projects).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, paginatedResponse(projects, total, pagination))
		return
	}

	var projects []model.Project
	err := baseQuery.
		Order("coalesce(project_members.last_used_at, projects.created_at) desc, projects.created_at desc").
		Find(&projects).Error
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, projects)
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
	input.Slug = strings.TrimSpace(input.Slug)
	if len(input.Slug) > projectSlugMaxLength {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("项目空间标识最多 %d 个字符", projectSlugMaxLength))
		return
	}
	if !h.ensureProjectSlugAvailable(ctx, input.Slug, "") {
		return
	}

	project := model.Project{
		ID:                  id.New("prj"),
		Slug:                input.Slug,
		Name:                input.Name,
		Description:         input.Description,
		NamespaceStrategy:   fallback(input.NamespaceStrategy, "project"),
		MaxConcurrentBuilds: normalizeBuildConcurrency(input.MaxConcurrentBuilds, defaultProjectBuildConcurrency),
		BillingOwnerUserID:  user.ID,
	}

	if err := h.db.Create(&project).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	member := model.ProjectMember{
		ID:        id.New("mem"),
		ProjectID: project.ID,
		UserID:    user.ID,
		Role:      "owner",
	}
	if err := h.db.Create(&member).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, h.projectResponse(project))
}

func (h *Handlers) GetProject(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUser(ctx)
	if !ok {
		return
	}
	h.recordProjectUsage(user.ID, project.ID)
	ctx.JSON(http.StatusOK, h.projectResponse(project))
}

func (h *Handlers) UpdateProject(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin")
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
	input.Slug = strings.TrimSpace(input.Slug)
	if len(input.Slug) > projectSlugMaxLength {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("项目空间标识最多 %d 个字符", projectSlugMaxLength))
		return
	}
	if !h.ensureProjectSlugAvailable(ctx, input.Slug, project.ID) {
		return
	}

	project.Slug = input.Slug
	project.Name = input.Name
	project.Description = input.Description
	project.NamespaceStrategy = fallback(input.NamespaceStrategy, "project")
	project.MaxConcurrentBuilds = normalizeBuildConcurrency(input.MaxConcurrentBuilds, defaultProjectBuildConcurrency)

	if err := h.db.Save(&project).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.projectResponse(project))
}

func (h *Handlers) DeleteProject(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner")
	if !ok {
		return
	}
	if !deleteStatusCanStart(project.DeleteStatus) {
		writeErrorCode(ctx, http.StatusConflict, "project.delete_in_progress", "项目空间正在删除中，请等待资源清理完成")
		return
	}
	if err := markResourceDeleting(h.db, &model.Project{}, project.ID); err != nil {
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
		_ = markResourceDeleteFailed(h.db, &model.Project{}, project.ID, "资源清理任务投递失败，请稍后重试")
		writeError(ctx, http.StatusServiceUnavailable, "资源清理任务投递失败，请稍后重试")
		return
	}
	h.audit(user.ID, "project.delete", project.ID, true, project.Name)
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListProjectPins(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var rows []projectPinResponse
	err := h.db.Table("project_pins").
		Select("projects.id, projects.slug, projects.name, projects.description, projects.namespace_strategy, projects.created_at, project_members.dashboard_order, project_members.last_used_at, project_members.use_count, project_pins.pinned_at").
		Joins("join projects on projects.id = project_pins.project_id and projects.deleted_at is null").
		Joins("join project_members on project_members.project_id = projects.id and project_members.user_id = project_pins.user_id").
		Where("project_pins.user_id = ?", user.ID).
		Order("project_members.use_count desc, coalesce(project_members.last_used_at, projects.created_at) desc, project_pins.pinned_at desc").
		Scan(&rows).Error
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, rows)
}

func (h *Handlers) PinProject(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUser(ctx)
	if !ok {
		return
	}

	now := time.Now()
	var pin model.ProjectPin
	err := h.db.First(&pin, "user_id = ? and project_id = ?", user.ID, project.ID).Error
	if err == nil {
		pin.PinnedAt = now
		if err := h.db.Save(&pin).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, projectPinResponseFrom(project, pin, h.projectDashboardOrder(user.ID, project.ID)))
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
	if err := h.db.Create(&pin).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, projectPinResponseFrom(project, pin, h.projectDashboardOrder(user.ID, project.ID)))
}

func (h *Handlers) UnpinProject(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUser(ctx)
	if !ok {
		return
	}

	if err := h.db.Delete(&model.ProjectPin{}, "user_id = ? and project_id = ?", user.ID, project.ID).Error; err != nil {
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
	if err := h.db.Model(&model.ProjectMember{}).Where("user_id = ? and project_id in ?", user.ID, projectIDs).Count(&accessibleCount).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if accessibleCount != int64(len(projectIDs)) {
		writeError(ctx, http.StatusForbidden, "你没有访问部分项目空间的权限")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
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

func (h *Handlers) ensureProjectSlugAvailable(ctx *gin.Context, slug string, excludeProjectID string) bool {
	if slug == "" {
		writeError(ctx, http.StatusBadRequest, "项目空间标识不能为空")
		return false
	}
	query := h.db.Model(&model.Project{}).Where("slug = ?", slug)
	if strings.TrimSpace(excludeProjectID) != "" {
		query = query.Where("id <> ?", excludeProjectID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if count > 0 {
		writeError(ctx, http.StatusBadRequest, "项目空间标识已存在")
		return false
	}
	return true
}

func (h *Handlers) ListProjectMembers(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	var members []projectMemberResponse
	query := h.db.Table("project_members").
		Select("project_members.id, project_members.project_id, project_members.user_id, project_members.role, users.email, users.name").
		Joins("join users on users.id = project_members.user_id").
		Where("project_members.project_id = ?", ctx.Param("projectId"))
	query = applySearch(ctx, query, "users.email", "users.name", "project_members.role")
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
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
		return
	}
	if err := query.Order("project_members.created_at asc").Scan(&members).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, members)
}

func (h *Handlers) SearchProjectMemberCandidates(ctx *gin.Context) {
	_, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin")
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
	err := h.db.Table("users").
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
	actor, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin")
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
		if err := h.db.First(&targetUser, "id = ? and disabled = ?", userID, false).Error; err != nil {
			writeError(ctx, http.StatusNotFound, "user not found")
			return
		}
	case email != "":
		if err := h.db.First(&targetUser, "email = ? and disabled = ?", email, false).Error; err != nil {
			writeError(ctx, http.StatusNotFound, "user not found")
			return
		}
	default:
		writeError(ctx, http.StatusBadRequest, "user is required")
		return
	}

	role := normalizeProjectRole(input.Role)
	if role == "owner" && !h.currentProjectRoleAllows(ctx, project.ID, actor.ID, "owner") {
		writeError(ctx, http.StatusForbidden, "只有项目 owner 可以授予 owner 角色")
		return
	}

	member := model.ProjectMember{
		ID:        id.New("mem"),
		ProjectID: ctx.Param("projectId"),
		UserID:    targetUser.ID,
		Role:      role,
	}
	if err := h.db.First(&model.ProjectMember{}, "project_id = ? and user_id = ?", member.ProjectID, member.UserID).Error; err == nil {
		writeError(ctx, http.StatusConflict, "user is already a project member")
		return
	}
	if err := h.db.Create(&member).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(actor.ID, "project_member.create", member.ID, true, member.Role)

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
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}

	var member model.ProjectMember
	if err := h.db.First(&member, "id = ? and project_id = ?", ctx.Param("memberId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "member not found")
		return
	}

	var input projectMemberInput
	if !bindJSON(ctx, &input) {
		return
	}
	nextRole := normalizeProjectRole(input.Role)
	actorIsOwner := h.currentProjectRoleAllows(ctx, project.ID, user.ID, "owner")
	if (member.Role == "owner" || nextRole == "owner") && !actorIsOwner {
		writeError(ctx, http.StatusForbidden, "只有项目 owner 可以修改 owner 角色")
		return
	}
	if member.Role == "owner" && nextRole != "owner" && !h.projectHasAnotherOwner(member.ProjectID, member.ID) {
		writeError(ctx, http.StatusBadRequest, "项目至少需要保留一个 owner")
		return
	}
	member.Role = nextRole
	if err := h.db.Save(&member).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "project_member.update", member.ID, true, member.Role)
	ctx.JSON(http.StatusOK, member)
}

func (h *Handlers) DeleteProjectMember(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}

	var member model.ProjectMember
	if err := h.db.First(&member, "id = ? and project_id = ?", ctx.Param("memberId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "member not found")
		return
	}
	if member.UserID == user.ID {
		writeError(ctx, http.StatusBadRequest, "不能移除当前登录账号")
		return
	}
	if member.Role == "owner" {
		if !h.currentProjectRoleAllows(ctx, project.ID, user.ID, "owner") {
			writeError(ctx, http.StatusForbidden, "只有项目 owner 可以移除 owner 成员")
			return
		}
		if !h.projectHasAnotherOwner(member.ProjectID, member.ID) {
			writeError(ctx, http.StatusBadRequest, "项目至少需要保留一个 owner")
			return
		}
	}
	if err := h.db.Delete(&member).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "project_member.delete", member.ID, true, member.Role)
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) findProject(ctx *gin.Context) (model.Project, bool) {
	var project model.Project
	if err := h.db.First(&project, "id = ?", ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project not found")
		return project, false
	}
	return project, true
}

func (h *Handlers) findProjectForCurrentUser(ctx *gin.Context) (model.Project, bool) {
	return h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer", "viewer")
}

func (h *Handlers) projectAndCurrentUser(ctx *gin.Context) (model.User, model.Project, bool) {
	return h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer", "viewer")
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
	err := h.db.First(&member, "project_id = ? and user_id = ?", project.ID, user.ID).Error
	if err != nil {
		writeError(ctx, http.StatusForbidden, "你没有访问该项目的权限")
		return model.Project{}, false
	}

	if !projectUserRoleAllowed(user, member.Role, allowedRoles) {
		writeError(ctx, http.StatusForbidden, "你没有执行该项目操作的权限")
		return model.Project{}, false
	}

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
	if err := h.db.First(&member, "project_id = ? and user_id = ?", projectID, userID).Error; err != nil {
		writeError(ctx, http.StatusForbidden, "你没有访问该项目的权限")
		return false
	}
	return projectRoleAllowed(member.Role, allowedRoles)
}

func (h *Handlers) projectHasAnotherOwner(projectID, memberID string) bool {
	return h.projects.HasAnotherOwner(projectID, memberID)
}

type projectInput struct {
	Slug                string `json:"slug" binding:"required"`
	Name                string `json:"name" binding:"required"`
	Description         string `json:"description"`
	NamespaceStrategy   string `json:"namespaceStrategy"`
	MaxConcurrentBuilds int    `json:"maxConcurrentBuilds"`
}

type projectOrderInput struct {
	ProjectIDs []string `json:"projectIds" binding:"required"`
}

type projectPinResponse struct {
	ID                string     `json:"id"`
	Slug              string     `json:"slug"`
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
	BillingOwner *projectBillingOwnerResponse `json:"billingOwner,omitempty"`
}

type projectBillingOwnerResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

func (h *Handlers) projectResponse(project model.Project) projectResponse {
	response := projectResponse{Project: project}
	if strings.TrimSpace(project.BillingOwnerUserID) == "" {
		return response
	}

	var user model.User
	if err := h.db.Select("id", "email", "name", "avatar_url").First(&user, "id = ?", project.BillingOwnerUserID).Error; err != nil {
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
		Slug:              project.Slug,
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
	case "slug":
		return "projects.slug " + order + ", projects.id asc"
	default:
		return "coalesce(project_members.last_used_at, projects.created_at) " + order + ", projects.created_at desc"
	}
}

func projectListScope(scope string) string {
	if strings.TrimSpace(scope) == "all" {
		return "all"
	}
	return "related"
}

func (h *Handlers) recordProjectUsage(userID string, projectID string) {
	now := time.Now()
	_ = h.db.Model(&model.ProjectMember{}).
		Where("user_id = ? and project_id = ?", userID, projectID).
		Updates(map[string]any{
			"last_used_at": now,
			"use_count":    gorm.Expr("use_count + 1"),
		}).Error
}

func (h *Handlers) projectDashboardOrder(userID string, projectID string) int {
	var member model.ProjectMember
	if err := h.db.Select("dashboard_order").First(&member, "user_id = ? and project_id = ?", userID, projectID).Error; err != nil {
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

type projectMemberInput struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Role   string `json:"role" binding:"required"`
}

type projectMemberResponse struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	UserID    string `json:"userId"`
	Role      string `json:"role"`
	Email     string `json:"email"`
	Name      string `json:"name"`
}

type projectMemberCandidateResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

func normalizeProjectRole(role string) string {
	return authz.NormalizeProjectRole(role)
}
