package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) billingScopeForUser(ctx *gin.Context, user model.User) (billingScope, bool) {
	scope := billingScope{}
	requested := make([]string, 0)
	for _, rawProjectIDs := range ctx.QueryArray("projectIds") {
		requested = append(requested, strings.Split(rawProjectIDs, ",")...)
	}
	requested = normalizeStringList(requested)
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

func (h *Handlers) ensureBillingProjectsExist(ctx *gin.Context, projectIDs []string) bool {
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
}
