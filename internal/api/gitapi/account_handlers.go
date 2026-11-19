package gitapi

import (
	"context"
	"errors"
	"fmt"

	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handlers) ListGitAccounts(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	visibility, ok := h.resolveListVisibility(ctx, user)
	if !ok {
		return
	}

	projectID := strings.TrimSpace(ctx.Query("projectId"))
	query := h.dbFor(ctx).Model(&model.GitAccount{})
	query, visible := h.applyScopedResourceListVisibility(ctx, query, scopedResourceGitAccount, user, projectID, visibility)
	if !visible {
		return
	}

	var accounts []model.GitAccount
	query = applySearch(ctx, query, "username", "external_user_id")
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"username": "username", "createdAt": "created_at"}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"username":  "username",
		"createdAt": "created_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&accounts).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.attachGitAccountProjects(accounts, ctx.Request.Context())
	h.observeGitAccounts(ctx.Request.Context(), user, accounts)
	ctx.JSON(http.StatusOK, paginatedResponse(gitAccountResponses(accounts), total, pagination))
}

func (h *Handlers) CreateGitAccount(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input gitAccountInput
	if !bindJSON(ctx, &input) {
		return
	}
	provider, ok := h.findEnabledGitProvider(ctx, input.ProviderID)
	if !ok {
		return
	}
	scope := normalizeGitScope(input.Scope)
	scope, ownerRef, projectIDs, scopeOK := h.normalizeCredentialScopeWithinParent(ctx, user, scope, input.ProjectIDs, provider.Scope, provider.ProjectIDs, "只有平台管理员可以创建全局 Git 凭据")
	if !scopeOK {
		return
	}
	account := model.GitAccount{
		ID:             id.New("gita"),
		UserID:         user.ID,
		Scope:          scope,
		OwnerRef:       ownerRef,
		ProjectIDs:     projectIDs,
		ProviderID:     strings.TrimSpace(input.ProviderID),
		ExternalUserID: strings.TrimSpace(input.ExternalUserID),
		Username:       strings.TrimSpace(input.Username),
		AvatarURL:      strings.TrimSpace(input.AvatarURL),
		Scopes:         strings.Join(normalizeList(input.Scopes, false), ","),
	}
	if account.Username == "" {
		writeError(ctx, http.StatusBadRequest, "请输入 Git 账号用户名")
		return
	}

	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedProvider model.GitProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProvider, "id = ? and enabled = ?", account.ProviderID, true).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errGitProviderUnavailable
			}
			return err
		}
		if err := ensureGitAccountFitsProviderScopeTx(tx, lockedProvider, account); err != nil {
			return err
		}
		if strings.TrimSpace(input.AccessToken) != "" && h.secrets.Available() {
			ref, storeErr := h.secrets.StoreContextWithDB(ctx.Request.Context(), tx, input.AccessToken, user.ID, "git_account:"+account.ID+":access")
			if storeErr != nil {
				return storeErr
			}
			account.AccessTokenRef = ref
		}
		if strings.TrimSpace(input.RefreshToken) != "" && h.secrets.Available() {
			ref, storeErr := h.secrets.StoreContextWithDB(ctx.Request.Context(), tx, input.RefreshToken, user.ID, "git_account:"+account.ID+":refresh")
			if storeErr != nil {
				return storeErr
			}
			account.RefreshTokenRef = ref
		}
		if err := tx.Save(&account).Error; err != nil {
			return err
		}
		return h.replaceScopedResourceProjectBindings(tx, scopedResourceGitAccount, account.ID, sortedProjectIDs(account.ProjectIDs), nil)
	})
	if errors.Is(err, errGitProviderUnavailable) {
		writeError(ctx, http.StatusNotFound, "git provider not found")
		return
	}
	if errors.Is(err, errGitCredentialScopeConflict) {
		writeErrorCode(ctx, http.StatusConflict, "git.credential_scope_conflict", "git credential scope exceeds the parent provider scope")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "git_account.create", account.ID, true, account.Scope, ctx.Request.Context())
	h.observeGitAccount(ctx.Request.Context(), user, &account)
	ctx.JSON(http.StatusCreated, gitAccountResponse(account))
}

func (h *Handlers) UpdateGitAccount(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var account model.GitAccount
	if err := h.dbFor(ctx).First(&account, "id = ?", ctx.Param("accountId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "git account not found")
		return
	}
	if !h.canManageGitAccount(ctx, user, account) {
		return
	}

	var input gitAccountInput
	if !bindJSON(ctx, &input) {
		return
	}
	provider, ok := h.findEnabledGitProvider(ctx, input.ProviderID)
	if !ok {
		return
	}
	scope := normalizeGitScope(input.Scope)
	scope, ownerRef, projectIDs, scopeOK := h.normalizeCredentialScopeWithinParent(ctx, user, scope, input.ProjectIDs, provider.Scope, provider.ProjectIDs, "只有平台管理员可以创建全局 Git 凭据")
	if !scopeOK {
		return
	}
	username := strings.TrimSpace(input.Username)
	if username == "" {
		writeError(ctx, http.StatusBadRequest, "请输入 Git 账号用户名")
		return
	}
	providerID := strings.TrimSpace(input.ProviderID)
	externalUserID := strings.TrimSpace(input.ExternalUserID)
	avatarURL := strings.TrimSpace(input.AvatarURL)
	scopesCSV := strings.Join(normalizeList(input.Scopes, false), ",")

	var updated model.GitAccount
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedProvider model.GitProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProvider, "id = ? and enabled = ?", providerID, true).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errGitProviderUnavailable
			}
			return err
		}
		var locked model.GitAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", account.ID).Error; err != nil {
			return err
		}
		locked.ProviderID = providerID
		locked.Scope = scope
		locked.OwnerRef = ownerRef
		locked.ProjectIDs = projectIDs
		locked.ExternalUserID = externalUserID
		locked.Username = username
		locked.AvatarURL = avatarURL
		locked.Scopes = scopesCSV
		if fitErr := ensureGitAccountFitsProviderScopeTx(tx, lockedProvider, locked); fitErr != nil {
			return fitErr
		}
		if compatErr := ensureGitAccountScopeCompatibleTx(tx, locked); compatErr != nil {
			return compatErr
		}
		if strings.TrimSpace(input.AccessToken) != "" && h.secrets.Available() {
			resource := "git_account:" + locked.ID + ":access"
			newRef, storeErr := h.secrets.StoreContextWithDB(ctx.Request.Context(), tx, input.AccessToken, user.ID, resource)
			if storeErr != nil {
				return storeErr
			}
			if strings.TrimSpace(locked.AccessTokenRef) != "" && locked.AccessTokenRef != newRef {
				if delErr := h.secrets.DeleteRefContextWithDB(ctx.Request.Context(), tx, locked.AccessTokenRef, resource); delErr != nil {
					return delErr
				}
			}
			locked.AccessTokenRef = newRef
		}
		if strings.TrimSpace(input.RefreshToken) != "" && h.secrets.Available() {
			resource := "git_account:" + locked.ID + ":refresh"
			newRef, storeErr := h.secrets.StoreContextWithDB(ctx.Request.Context(), tx, input.RefreshToken, user.ID, resource)
			if storeErr != nil {
				return storeErr
			}
			if strings.TrimSpace(locked.RefreshTokenRef) != "" && locked.RefreshTokenRef != newRef {
				if delErr := h.secrets.DeleteRefContextWithDB(ctx.Request.Context(), tx, locked.RefreshTokenRef, resource); delErr != nil {
					return delErr
				}
			}
			locked.RefreshTokenRef = newRef
		}
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceGitAccount, locked.ID, sortedProjectIDs(locked.ProjectIDs), nil); err != nil {
			return err
		}
		updated = locked
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "git account not found")
		return
	}
	if errors.Is(err, errGitProviderUnavailable) {
		writeError(ctx, http.StatusNotFound, "git provider not found")
		return
	}
	if errors.Is(err, errGitCredentialScopeConflict) {
		writeErrorCode(ctx, http.StatusConflict, "git.credential_scope_conflict", "git credential scope exceeds the parent provider scope")
		return
	}
	if errors.Is(err, errGitAccountInUse) {
		writeErrorCode(ctx, http.StatusConflict, "git.account_in_use", "git credential is referenced by repository bindings outside the target scope")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "git_account.update", updated.ID, true, updated.Scope, ctx.Request.Context())
	h.observeGitAccount(ctx.Request.Context(), user, &updated)
	ctx.JSON(http.StatusOK, gitAccountResponse(updated))
}

func (h *Handlers) attachGitAccountProjects(accounts []model.GitAccount, ctx context.Context) {
	projectMap := h.scopedResourceProjectIDMap(scopedResourceGitAccount, gitAccountIDs(accounts), ctx)
	for index := range accounts {
		accounts[index].ProjectIDs = projectMap[accounts[index].ID]
	}
}

func gitAccountIDs(accounts []model.GitAccount) []string {
	ids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}

var (
	errGitAccountInUse        = errors.New("git credential is referenced by repository bindings")
	errGitProviderUnavailable = errors.New("git provider is unavailable")
)

func ensureGitAccountFitsProviderScopeTx(tx *gorm.DB, provider model.GitProvider, account model.GitAccount) error {
	parentScope := normalizeGitScope(provider.Scope)
	childScope := normalizeGitScope(account.Scope)
	switch parentScope {
	case "global":
		return nil
	case "user":
		if childScope != "user" || strings.TrimSpace(account.OwnerRef) != strings.TrimSpace(provider.OwnerRef) {
			return errGitCredentialScopeConflict
		}
		return nil
	case "project":
		if childScope == "global" {
			return errGitCredentialScopeConflict
		}
		if childScope == "project" {
			var boundProjects []string
			if err := tx.Model(&model.ScopedResourceProjectBinding{}).
				Where("resource_type = ? and resource_id = ?", scopedResourceGitProvider, provider.ID).
				Pluck("project_id", &boundProjects).Error; err != nil {
				return err
			}
			allowed := make(map[string]bool, len(boundProjects))
			for _, projectID := range boundProjects {
				allowed[strings.TrimSpace(projectID)] = true
			}
			for _, projectID := range account.ProjectIDs {
				if !allowed[strings.TrimSpace(projectID)] {
					return errGitCredentialScopeConflict
				}
			}
		}
		return nil
	default:
		return errGitCredentialScopeConflict
	}
}

func ensureGitAccountScopeCompatibleTx(tx *gorm.DB, account model.GitAccount) error {
	scope := normalizeGitScope(account.Scope)
	if scope == "global" {
		return nil
	}
	bindingQuery := tx.Model(&model.RepositoryBinding{}).
		Joins("join projects on projects.id = repository_bindings.project_id and projects.deleted_at is null").
		Where("repository_bindings.git_account_id = ?", account.ID)
	switch scope {
	case "project":
		if len(account.ProjectIDs) > 0 {
			bindingQuery = bindingQuery.Where("repository_bindings.project_id not in ?", account.ProjectIDs)
		}
	case "user":
		memberProjects := tx.Model(&model.ProjectMember{}).Select("project_id").Where("user_id = ?", account.OwnerRef)
		bindingQuery = bindingQuery.Where("repository_bindings.project_id not in (?)", memberProjects)
	}
	var conflictCount int64
	if err := bindingQuery.Count(&conflictCount).Error; err != nil {
		return err
	}
	if conflictCount > 0 {
		return errGitAccountInUse
	}
	return nil
}

func (h *Handlers) DeleteGitAccount(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var account model.GitAccount
	if err := h.dbFor(ctx).First(&account, "id = ?", ctx.Param("accountId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "git account not found")
		return
	}
	if !h.canManageGitAccount(ctx, user, account) {
		return
	}
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.GitAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", account.ID).Error; err != nil {
			return err
		}
		var bindingCount int64
		if err := tx.Model(&model.RepositoryBinding{}).
			Joins("join projects on projects.id = repository_bindings.project_id and projects.deleted_at is null").
			Where("repository_bindings.git_account_id = ?", locked.ID).
			Count(&bindingCount).Error; err != nil {
			return err
		}
		if bindingCount > 0 {
			var provider model.GitProvider
			providerErr := tx.Select("id").First(&provider, "id = ?", locked.ProviderID).Error
			if providerErr == nil {
				return errGitAccountInUse
			}
			if !errors.Is(providerErr, gorm.ErrRecordNotFound) {
				return providerErr
			}
		}
		if err := h.secrets.DeleteRefContextWithDB(ctx.Request.Context(), tx, locked.AccessTokenRef, "git_account:"+locked.ID+":access"); err != nil {
			return err
		}
		if err := h.secrets.DeleteRefContextWithDB(ctx.Request.Context(), tx, locked.RefreshTokenRef, "git_account:"+locked.ID+":refresh"); err != nil {
			return err
		}
		if err := tx.Where("resource_type = ? and resource_id = ?", scopedResourceGitAccount, locked.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&locked).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "git account not found")
		return
	}
	if errors.Is(err, errGitAccountInUse) {
		writeErrorCode(ctx, http.StatusConflict, "git.account_in_use", "git credential is referenced by repository bindings")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "git_account.delete", account.ID, true, account.Username, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) RefreshGitAccount(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	account, ok := h.findGitAccountForUser(ctx, user.ID, ctx.Param("accountId"))
	if !ok {
		return
	}
	provider, ok := h.findEnabledGitProvider(ctx, account.ProviderID)
	if !ok {
		return
	}
	refreshed, ok := h.refreshGitAccountForUser(ctx, user, account, provider)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, gitAccountResponse(refreshed))
}

func (h *Handlers) refreshGitAccountForUser(ctx *gin.Context, user model.User, account model.GitAccount, provider model.GitProvider) (model.GitAccount, bool) {
	refreshToken := h.secrets.ResolveContext(ctx.Request.Context(), account.RefreshTokenRef)
	if refreshToken == "" {
		writeError(ctx, http.StatusBadRequest, "git account has no refresh token")
		return account, false
	}
	oauthConfig, err := gitprovider.OAuthConfig(provider, h.gitOAuthRedirectURL(ctx), h.secrets.ResolveContext(ctx.Request.Context(), provider.ClientSecretRef))
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "git OAuth provider configuration is invalid")
		return account, false
	}
	egressCtx := h.egressContextForUser(ctx.Request.Context(), user, 15*time.Second)
	tokenSource := oauthConfig.TokenSource(egressCtx, &oauth2.Token{
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	})
	token, err := tokenSource.Token()
	if err != nil {
		h.auditWithContext(user.ID, "git_account.refresh", account.ID, false, "git token refresh failed", ctx.Request.Context())
		writeErrorCode(ctx, http.StatusBadRequest, "git.token_refresh_failed", "git token refresh failed")
		return account, false
	}
	account.AccessTokenRef = h.secrets.StoreContext(ctx.Request.Context(), token.AccessToken, account.UserID, "git_account:"+account.ID+":access")
	if token.RefreshToken != "" {
		account.RefreshTokenRef = h.secrets.StoreContext(ctx.Request.Context(), token.RefreshToken, account.UserID, "git_account:"+account.ID+":refresh")
	}
	if !token.Expiry.IsZero() {
		account.ExpiresAt = &token.Expiry
	}
	if err := h.dbFor(ctx).Save(&account).Error; err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "git.token_refresh_failed", "git token refresh failed")
		return account, false
	}
	h.auditWithContext(user.ID, "git_account.refresh", account.ID, true, account.Username, ctx.Request.Context())
	now := time.Now().UTC()
	account.Status = observation.StatusReady
	account.ObservationCode = "git_account_ready"
	account.ObservedAt = &now
	return account, true
}

func gitAccountNeedsRefresh(account model.GitAccount) bool {
	if account.ExpiresAt == nil {
		return false
	}
	return time.Until(*account.ExpiresAt) <= 5*time.Minute
}

func (h *Handlers) upsertGitAccountFromOAuth(userID string, provider model.GitProvider, gitUser gitprovider.UserResponse, token *oauth2.Token, ctx context.Context) (model.GitAccount, error) {
	externalID := gitUser.ExternalID()
	username := gitUser.Username()
	if externalID == "" || username == "" {
		return model.GitAccount{}, fmt.Errorf("git user identity is incomplete")
	}
	var account model.GitAccount
	err := h.dbWithContext(ctx).First(&account, "user_id = ? and provider_id = ? and external_user_id = ?", userID, provider.ID, externalID).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return account, err
	}
	if err == gorm.ErrRecordNotFound {
		account = model.GitAccount{
			ID:             id.New("gita"),
			UserID:         userID,
			Scope:          "user",
			OwnerRef:       userID,
			ProviderID:     provider.ID,
			ExternalUserID: externalID,
		}
	} else {
		if account.Scope == "" {
			account.Scope = "user"
			account.OwnerRef = userID
		}
	}
	account.Username = username
	account.AvatarURL = strings.TrimSpace(gitUser.AvatarURL)
	account.AccessTokenRef = h.secrets.StoreContext(ctx, token.AccessToken, userID, "git_account:"+account.ID+":access")
	if token.RefreshToken != "" {
		account.RefreshTokenRef = h.secrets.StoreContext(ctx, token.RefreshToken, userID, "git_account:"+account.ID+":refresh")
	}
	account.Scopes = strings.Join(normalizeList(tokenScopes(token), false), ",")
	if !token.Expiry.IsZero() {
		account.ExpiresAt = &token.Expiry
	}
	if err == gorm.ErrRecordNotFound {
		if err := h.dbWithContext(ctx).Create(&account).Error; err != nil {
			return account, err
		}
		h.auditWithContext(userID, "git_account.oauth_upsert", account.ID, true, provider.ID, ctx)
		return account, nil
	}
	if err := h.dbWithContext(ctx).Save(&account).Error; err != nil {
		return account, err
	}
	h.auditWithContext(userID, "git_account.oauth_upsert", account.ID, true, provider.ID, ctx)
	return account, nil
}

func tokenScopes(token *oauth2.Token) []string {
	scope, _ := token.Extra("scope").(string)
	if scope == "" {
		return nil
	}
	return strings.Fields(strings.ReplaceAll(scope, ",", " "))
}
