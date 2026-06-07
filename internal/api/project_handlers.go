package api

import (
	"errors"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"strings"
	"time"
)

func (h *Handlers) ListProjects(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	baseQuery := h.db.
		Model(&model.Project{}).
		Joins("join project_members on project_members.project_id = projects.id").
		Where("project_members.user_id = ?", user.ID)
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
		if err := baseQuery.Session(&gorm.Session{}).Order(orderByClause(pagination, map[string]string{
			"createdAt": "projects.created_at",
			"name":      "projects.name",
			"slug":      "projects.slug",
		}, "projects.created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&projects).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, paginatedResponse(projects, total, pagination))
		return
	}

	var projects []model.Project
	err := baseQuery.
		Order("projects.created_at desc").
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
	if !h.ensureProjectSlugAvailable(ctx, input.Slug, "") {
		return
	}

	project := model.Project{
		ID:                id.New("prj"),
		Slug:              input.Slug,
		Name:              input.Name,
		Description:       input.Description,
		NamespaceStrategy: fallback(input.NamespaceStrategy, "project"),
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
	_ = h.db.Create(&member).Error

	ctx.JSON(http.StatusCreated, project)
}

func (h *Handlers) GetProject(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUser(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, project)
}

func (h *Handlers) UpdateProject(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return
	}

	var input projectInput
	if !bindJSON(ctx, &input) {
		return
	}
	input.Slug = strings.TrimSpace(input.Slug)
	if !h.ensureProjectSlugAvailable(ctx, input.Slug, project.ID) {
		return
	}

	project.Slug = input.Slug
	project.Name = input.Name
	project.Description = input.Description
	project.NamespaceStrategy = fallback(input.NamespaceStrategy, "project")

	if err := h.db.Save(&project).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, project)
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
	if err := h.db.Delete(&project).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
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
		Select("projects.id, projects.slug, projects.name, projects.description, projects.namespace_strategy, projects.created_at, project_pins.pinned_at").
		Joins("join projects on projects.id = project_pins.project_id and projects.deleted_at is null").
		Joins("join project_members on project_members.project_id = projects.id and project_members.user_id = project_pins.user_id").
		Where("project_pins.user_id = ?", user.ID).
		Order("project_pins.pinned_at desc").
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
		ctx.JSON(http.StatusOK, projectPinResponseFrom(project, pin.PinnedAt))
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
	ctx.JSON(http.StatusCreated, projectPinResponseFrom(project, pin.PinnedAt))
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
	err := h.db.Table("project_members").
		Select("project_members.id, project_members.project_id, project_members.user_id, project_members.role, users.email, users.name").
		Joins("join users on users.id = project_members.user_id").
		Where("project_members.project_id = ?", ctx.Param("projectId")).
		Order("project_members.created_at asc").
		Scan(&members).Error
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, members)
}

func (h *Handlers) CreateProjectMember(ctx *gin.Context) {
	actor, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return
	}

	var input projectMemberInput
	if !bindJSON(ctx, &input) {
		return
	}

	var targetUser model.User
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if err := h.db.First(&targetUser, "email = ? and disabled = ?", email, false).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "user not found")
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

	var member model.ProjectMember
	err := h.db.First(&member, "project_id = ? and user_id = ?", project.ID, user.ID).Error
	if err != nil {
		writeError(ctx, http.StatusForbidden, "你没有访问该项目的权限")
		return model.Project{}, false
	}

	if !projectRoleAllowed(member.Role, allowedRoles) {
		writeError(ctx, http.StatusForbidden, "你没有执行该项目操作的权限")
		return model.Project{}, false
	}

	return project, true
}

func projectRoleAllowed(role string, allowedRoles []string) bool {
	for _, allowedRole := range allowedRoles {
		if role == allowedRole {
			return true
		}
	}
	return false
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
	Slug              string `json:"slug" binding:"required"`
	Name              string `json:"name" binding:"required"`
	Description       string `json:"description"`
	NamespaceStrategy string `json:"namespaceStrategy"`
}

type projectPinResponse struct {
	ID                string    `json:"id"`
	Slug              string    `json:"slug"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	NamespaceStrategy string    `json:"namespaceStrategy"`
	CreatedAt         time.Time `json:"createdAt"`
	PinnedAt          time.Time `json:"pinnedAt"`
}

func projectPinResponseFrom(project model.Project, pinnedAt time.Time) projectPinResponse {
	return projectPinResponse{
		ID:                project.ID,
		Slug:              project.Slug,
		Name:              project.Name,
		Description:       project.Description,
		NamespaceStrategy: project.NamespaceStrategy,
		CreatedAt:         project.CreatedAt,
		PinnedAt:          pinnedAt,
	}
}

type projectMemberInput struct {
	Email string `json:"email"`
	Role  string `json:"role" binding:"required"`
}

type projectMemberResponse struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	UserID    string `json:"userId"`
	Role      string `json:"role"`
	Email     string `json:"email"`
	Name      string `json:"name"`
}

func normalizeProjectRole(role string) string {
	switch role {
	case "owner", "admin", "developer", "viewer":
		return role
	default:
		return "viewer"
	}
}
