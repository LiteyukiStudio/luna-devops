package projectapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"gorm.io/gorm"
)

// RegisterAITools registers all 21 AI domain facade tools owned by projectapi
// (4 runtime lifecycle hook tools + 17 workspace tools).
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	h.registerRuntimeHookAITools(reg)
	h.registerWorkspaceReadAITools(reg)
	h.registerWorkspaceWriteAITools(reg)
	h.registerWorkspaceNetworkAITools(reg)
}

func (h *Handler) aiActor(tc *aitool.ToolContext) (model.User, error) {
	if tc == nil {
		return model.User{}, aitool.NewError(http.StatusUnauthorized, "auth.unauthorized", "authentication required")
	}
	if strings.TrimSpace(tc.User.ID) != "" {
		return tc.User, nil
	}
	if tc.Gin != nil {
		if user, ok := h.currentUser(tc.Gin); ok && strings.TrimSpace(user.ID) != "" {
			return user, nil
		}
	}
	return model.User{}, aitool.NewError(http.StatusUnauthorized, "auth.unauthorized", "authentication required")
}

func (h *Handler) aiDB(tc *aitool.ToolContext) *gorm.DB {
	if tc != nil && tc.DB != nil {
		return tc.DB
	}
	if tc != nil && tc.Gin != nil {
		return h.dbFor(tc.Gin)
	}
	return h.dbWithContext(tc.Context())
}

func projectScopeMismatchError(detail string) *aitool.ToolError {
	err := aitool.Forbidden("ai.project_scope_mismatch", detail)
	err.Hint = "Switch to an unbound global conversation when operating across projects or global platform resources."
	return err
}

func (h *Handler) authorizeProjectForAI(tc *aitool.ToolContext, projectRef string, action authz.Action) (model.User, model.Project, string, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return model.User{}, model.Project{}, "", err
	}
	db := h.aiDB(tc)
	projectRef = strings.TrimSpace(projectRef)
	if projectRef == "" {
		return model.User{}, model.Project{}, "", aitool.BadRequest("project.required", "project is required")
	}

	var project model.Project
	if convID := strings.TrimSpace(tc.ConversationProjectID); convID != "" {
		if err := db.First(&project, "id = ? AND deleted_at IS NULL", convID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.User{}, model.Project{}, "", projectScopeMismatchError("bound conversation project no longer exists")
			}
			return model.User{}, model.Project{}, "", aitool.NewError(http.StatusServiceUnavailable, "project.lookup_unavailable", "project storage is unavailable")
		}
		if projectRef != project.ID && projectRef != project.Identifier {
			return model.User{}, model.Project{}, "", projectScopeMismatchError("requested project does not match the bound conversation project")
		}
	} else {
		resolved, resolveErr := aitool.ResolveProject(db, projectRef)
		if resolveErr != nil {
			return model.User{}, model.Project{}, "", resolveErr
		}
		project = resolved
	}

	reqCtx := tc.Context()
	subject := authz.ProjectSubject{UserID: user.ID, PlatformRole: user.Role}
	access, authErr := h.projectAuthorizer(reqCtx).AuthorizeProject(reqCtx, subject, project.ID, action)
	if authErr != nil {
		return model.User{}, model.Project{}, "", mapProjectAuthErrorForAI(authErr)
	}
	return user, project, access.Role, nil
}

func mapProjectAuthErrorForAI(err error) error {
	if errors.Is(err, authz.ErrProjectAuthorizationUnavailable) || errors.Is(err, authz.ErrProjectPolicyUndefined) {
		return aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", "project authorization service is temporarily unavailable")
	}
	return aitool.Forbidden("auth.forbidden", "you do not have permission to perform this project operation")
}

func ensureProjectCanMutateForAI(project model.Project) error {
	if resourceCanMutateDuringDelete(project.DeleteStatus) {
		return nil
	}
	return aitool.Conflict("project.delete_in_progress", "project space is currently being deleted")
}

func (h *Handler) projectMemberOwnerAllowedForAI(tc *aitool.ToolContext, projectID, userID string) (bool, error) {
	allowed, err := h.projectActionAllowed(tc.Context(), authz.ProjectSubject{
		UserID:       userID,
		PlatformRole: authz.PlatformRoleUser,
	}, projectID, authz.ActionProjectOwnerOnly)
	if err != nil {
		return false, mapProjectAuthErrorForAI(err)
	}
	return allowed, nil
}

func (h *Handler) resolveProjectHookForAI(db *gorm.DB, projectID, hookRef string) (model.ProjectHookConfig, error) {
	hookRef = strings.TrimSpace(hookRef)
	if hookRef == "" {
		return model.ProjectHookConfig{}, aitool.BadRequest("project_hook.required", "hook is required")
	}
	var hooks []model.ProjectHookConfig
	if err := db.Where("project_id = ?", projectID).Order("created_at asc, id asc").Find(&hooks).Error; err != nil {
		return model.ProjectHookConfig{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		hooks,
		"project_hook",
		"project_hook.not_found",
		hookRef,
		func(item model.ProjectHookConfig) string { return item.ID },
		func(item model.ProjectHookConfig) string { return item.Name },
	)
}

func resolveProjectTargetForShareAI(db *gorm.DB, projectID string, appRef *string, targetRef string) (model.DeploymentTarget, error) {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return model.DeploymentTarget{}, aitool.BadRequest("deployment_target.required", "target is required")
	}
	if appRef != nil && strings.TrimSpace(*appRef) != "" {
		app, err := aitool.ResolveApplication(db, projectID, *appRef)
		if err != nil {
			return model.DeploymentTarget{}, err
		}
		target, err := aitool.ResolveDeploymentTarget(db, projectID, app.ID, targetRef)
		if err != nil {
			return model.DeploymentTarget{}, err
		}
		if !model.DeleteStatusActive(target.DeleteStatus) {
			return model.DeploymentTarget{}, aitool.BadRequest("network_share.target_invalid", "deployment target must be an active target of this project")
		}
		return target, nil
	}

	var byID model.DeploymentTarget
	if err := db.Where("project_id = ? AND id = ? AND deleted_at IS NULL", projectID, targetRef).First(&byID).Error; err == nil {
		if !model.DeleteStatusActive(byID.DeleteStatus) {
			return model.DeploymentTarget{}, aitool.BadRequest("network_share.target_invalid", "deployment target must be an active target of this project")
		}
		return byID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.DeploymentTarget{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var matches []model.DeploymentTarget
	if err := db.Where("project_id = ? AND (stage = ? OR name = ?) AND deleted_at IS NULL", projectID, targetRef, targetRef).
		Order("created_at asc, id asc").
		Find(&matches).Error; err != nil {
		return model.DeploymentTarget{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	activeMatches := make([]model.DeploymentTarget, 0, len(matches))
	for _, m := range matches {
		if model.DeleteStatusActive(m.DeleteStatus) {
			activeMatches = append(activeMatches, m)
		}
	}
	if len(activeMatches) == 1 {
		return activeMatches[0], nil
	}
	if len(activeMatches) > 1 {
		candidates := make([]aitool.AmbiguousCandidate, len(activeMatches))
		for i, item := range activeMatches {
			candidates[i] = aitool.AmbiguousCandidate{ID: item.ID, Name: item.Name, Stage: item.Stage}
		}
		return model.DeploymentTarget{}, aitool.AmbiguousResource("deployment_target", targetRef, candidates)
	}
	return model.DeploymentTarget{}, aitool.NotFound("deployment_target.not_found", "deployment target was not found; pass application along with target if needed")
}

func mapInboxTransferErrorForAI(err error) error {
	switch {
	case errors.Is(err, inbox.ErrInvalidInput), errors.Is(err, inbox.ErrInvalidDeepLink):
		return aitool.BadRequest("inbox.request_invalid", "inbox request is invalid")
	case errors.Is(err, inbox.ErrNotFound):
		return aitool.NotFound("inbox.not_found", "inbox resource was not found")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferInvalid):
		return aitool.BadRequest("inbox.billing_owner_transfer_invalid", "billing owner transfer input is invalid")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferForbidden):
		return aitool.Forbidden("inbox.billing_owner_transfer_forbidden", "billing owner transfer is forbidden; caller must be both current billingOwner and project owner")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferConflict):
		return aitool.Conflict("inbox.billing_owner_transfer_conflict", "billing owner transfer conflicts with current state or a pending transfer already exists")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferStale):
		return aitool.Conflict("inbox.billing_owner_transfer_stale", "billing owner transfer request is stale")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferNotFound):
		return aitool.NotFound("inbox.billing_owner_transfer_not_found", "billing owner transfer request was not found")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferExpired):
		return aitool.NewError(http.StatusGone, "inbox.billing_owner_transfer_expired", "billing owner transfer request expired")
	default:
		return aitool.NewError(http.StatusInternalServerError, "inbox.operation_failed", err.Error())
	}
}

func truncateUTF8Runes(s string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return "", len(s) > 0
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s, false
	}
	runes := []rune(s)
	return string(runes[:maxRunes]), true
}

func truncateUTF8HeadBytes(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", len(s) > 0
	}
	if len(s) <= maxBytes {
		return s, false
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

func truncateUTF8TailBytes(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", len(s) > 0
	}
	if len(s) <= maxBytes {
		return s, false
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:], true
}

func countScriptLines(s string) int {
	if s == "" {
		return 0
	}
	lines := strings.Count(s, "\n") + 1
	if strings.HasSuffix(s, "\n") && lines > 1 {
		lines--
	}
	return lines
}

func boolOrDefault(ptr *bool, def bool) bool {
	if ptr == nil {
		return def
	}
	return *ptr
}

func stringOrDefault(ptr *string, def string) string {
	if ptr == nil {
		return def
	}
	v := strings.TrimSpace(*ptr)
	if v == "" {
		return def
	}
	return v
}
