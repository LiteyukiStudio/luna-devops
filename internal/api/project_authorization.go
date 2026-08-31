package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/gin-gonic/gin"
)

// authorizeProject is the only request-level project RBAC entry point. It
// authenticates the subject, loads the project and delegates the policy
// decision to internal/authz before business work begins.
func (h *Handlers) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return model.User{}, model.Project{}, false
	}
	project, ok := h.findProject(ctx)
	if !ok {
		return model.User{}, model.Project{}, false
	}

	subject := authz.ProjectSubject{UserID: user.ID, PlatformRole: user.Role}
	access, err := h.projectAuthorizer(ctx.Request.Context()).AuthorizeProject(ctx.Request.Context(), subject, project.ID, action)
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return model.User{}, model.Project{}, false
	}
	if access.Role != "" {
		ctx.Set(currentProjectRoleContextKey, access.Role)
	}
	return user, project, true
}

func (h *Handlers) authorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	originalParams := ctx.Params
	ctx.Params = replaceParam(ctx.Params, "projectId", projectID)
	defer func() { ctx.Params = originalParams }()
	user, project, ok := h.authorizeProject(ctx, action)
	return user, project, ok
}

func (h *Handlers) projectActionAllowed(ctx context.Context, subject authz.ProjectSubject, projectID string, action authz.Action) (bool, error) {
	_, err := h.projectAuthorizer(ctx).AuthorizeProject(ctx, subject, projectID, action)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, authz.ErrProjectAccessDenied) {
		return false, nil
	}
	return false, err
}

func (h *Handlers) projectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return h.projectActionAllowed(ctx, authz.ProjectSubject{
		UserID:       user.ID,
		PlatformRole: user.Role,
	}, projectID, action)
}

// projectMemberActionAllowed applies the same policy without the
// platform-admin bypass. The second result distinguishes an ordinary denial
// from an unavailable authorization dependency, whose response is written
// here so callers cannot accidentally downgrade it to a 403.
func (h *Handlers) projectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool) {
	allowed, err := h.projectActionAllowed(ctx.Request.Context(), authz.ProjectSubject{
		UserID:       userID,
		PlatformRole: authz.PlatformRoleUser,
	}, projectID, action)
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return false, false
	}
	return allowed, true
}

func (h *Handlers) projectAuthorizer(ctx context.Context) authz.ProjectAuthorizer {
	return authz.NewProjectAuthorizer(repository.NewProjectRepository(h.dbWithContext(ctx)))
}

func writeProjectAuthorizationError(ctx *gin.Context, err error) {
	if errors.Is(err, authz.ErrProjectAuthorizationUnavailable) || errors.Is(err, authz.ErrProjectPolicyUndefined) {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "auth.project_authorization_unavailable", "项目权限服务暂时不可用")
		ctx.Abort()
		return
	}
	writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "你没有执行该项目操作的权限")
}
