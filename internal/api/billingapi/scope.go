package billingapi

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ScopeModeWallet  = "wallet"
	ScopeModeProject = "project"
)

func (h *Handler) billingScopeForUser(ctx *gin.Context, user model.User) (billingScope, bool) {
	scope := billingScope{}
	requested := make([]string, 0)
	for _, rawProjectIDs := range ctx.QueryArray("projectIds") {
		requested = append(requested, strings.Split(rawProjectIDs, ",")...)
	}
	requested = deduplicateStrings(normalizeStringList(requested))

	scopeMode := strings.ToLower(strings.TrimSpace(ctx.Query("scopeMode")))
	if scopeMode != "" && scopeMode != ScopeModeWallet && scopeMode != ScopeModeProject {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.scope_mode_invalid", "scopeMode must be wallet or project")
		return scope, false
	}
	if scopeMode == ScopeModeProject {
		return h.projectBillingScopeForUser(ctx, user, requested)
	}

	if user.Role == authz.PlatformRoleAdmin {
		selectedUserID := strings.TrimSpace(ctx.Query("userId"))
		scope.SelectedUserID = selectedUserID
		scope.ProjectIDs = requested
		scope.FilterProjectIDs = len(requested) > 0
		if selectedUserID != "" {
			var count int64
			if err := h.dbFor(ctx).Model(&model.User{}).Where("id = ?", selectedUserID).Count(&count).Error; err != nil {
				writeError(ctx, http.StatusInternalServerError, err.Error())
				return scope, false
			}
			if count == 0 {
				writeErrorCode(ctx, http.StatusNotFound, "billing.user_not_found", "billing user not found")
				return scope, false
			}
			scope.UserIDs = []string{selectedUserID}
		} else {
			var wallets []model.UserWallet
			if err := h.dbFor(ctx).Select("user_id").Find(&wallets).Error; err != nil {
				writeError(ctx, http.StatusInternalServerError, err.Error())
				return scope, false
			}
			for _, wallet := range wallets {
				if strings.TrimSpace(wallet.UserID) != "" {
					scope.UserIDs = append(scope.UserIDs, wallet.UserID)
				}
			}
			if len(scope.UserIDs) == 0 {
				var users []model.User
				if err := h.dbFor(ctx).Select("id").Find(&users).Error; err != nil {
					writeError(ctx, http.StatusInternalServerError, err.Error())
					return scope, false
				}
				for _, item := range users {
					scope.UserIDs = append(scope.UserIDs, item.ID)
				}
			}
		}
		if scope.FilterProjectIDs && !h.ensureBillingProjectsExist(ctx, requested) {
			return scope, false
		}
		return scope, true
	}
	scope.UserIDs = []string{user.ID}
	if len(requested) == 0 {
		return scope, true
	}
	scope.FilterProjectIDs = true
	scope.ProjectIDs = requested
	for _, projectID := range requested {
		var count int64
		if err := h.dbFor(ctx).Model(&model.Project{}).Where("id = ? and billing_owner_user_id = ?", projectID, user.ID).Count(&count).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return scope, false
		}
		if count == 0 {
			writeErrorCode(ctx, http.StatusForbidden, "billing.project_forbidden", "current user cannot access the requested project billing")
			return scope, false
		}
	}
	return scope, true
}

func (h *Handler) projectBillingScopeForUser(ctx *gin.Context, user model.User, requested []string) (billingScope, bool) {
	scope := billingScope{
		ProjectScoped:    true,
		FilterProjectIDs: true,
	}
	if strings.TrimSpace(ctx.Query("userId")) != "" || strings.TrimSpace(ctx.Query("accountScope")) != "" {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.argument_invalid", "userId and accountScope cannot be combined with scopeMode=project")
		return scope, false
	}

	if authz.IsPlatformAdmin(user.Role) {
		if len(requested) > 0 {
			if !h.ensureBillingProjectsExist(ctx, requested) {
				return scope, false
			}
			scope.ProjectIDs = requested
			return scope, true
		}
		var allProjectIDs []string
		if err := h.dbFor(ctx).Model(&model.Project{}).Order("id asc").Pluck("id", &allProjectIDs).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return scope, false
		}
		scope.ProjectIDs = allProjectIDs
		return scope, true
	}

	accessibleIDs, err := accessibleBillingProjectIDs(h.dbFor(ctx), user.ID)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return scope, false
	}
	if len(requested) == 0 {
		scope.ProjectIDs = accessibleIDs
		return scope, true
	}

	allowedSet := make(map[string]struct{}, len(accessibleIDs))
	for _, projectID := range accessibleIDs {
		allowedSet[projectID] = struct{}{}
	}
	for _, projectID := range requested {
		if _, ok := allowedSet[projectID]; !ok {
			writeErrorCode(ctx, http.StatusForbidden, "billing.project_forbidden", "current user cannot access the requested project billing")
			return scope, false
		}
	}
	scope.ProjectIDs = requested
	return scope, true
}

func accessibleBillingProjectIDs(db *gorm.DB, userID string) ([]string, error) {
	policy, _ := authz.ProjectPolicyForAction(authz.ActionBillingRead)
	if len(policy.AllowedRoles) == 0 {
		return []string{}, nil
	}
	var accessibleIDs []string
	memberSubQuery := db.Table("project_members").
		Select("1").
		Where("project_members.project_id = projects.id AND project_members.user_id = ? AND project_members.role IN ?", userID, policy.AllowedRoles)
	if err := db.Model(&model.Project{}).
		Where("EXISTS (?)", memberSubQuery).
		Order("projects.id asc").
		Pluck("projects.id", &accessibleIDs).Error; err != nil {
		return nil, err
	}
	return accessibleIDs, nil
}

func deduplicateStrings(values []string) []string {
	if len(values) <= 1 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, item := range values {
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (h *Handler) ensureBillingProjectsExist(ctx *gin.Context, projectIDs []string) bool {
	if len(projectIDs) == 0 {
		return true
	}
	var count int64
	if err := h.dbFor(ctx).Model(&model.Project{}).Where("id in ?", projectIDs).Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if count != int64(len(projectIDs)) {
		writeErrorCode(ctx, http.StatusNotFound, "billing.project_not_found", "project not found")
		return false
	}
	return true
}

type billingScope struct {
	UserIDs          []string
	ProjectIDs       []string
	FilterProjectIDs bool
	SelectedUserID   string
	ProjectScoped    bool
}
