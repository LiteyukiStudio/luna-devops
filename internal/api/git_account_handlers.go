package api

import (
	"errors"
	"fmt"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"net/http"
	"strings"
	"time"
)

func (h *Handlers) ListGitAccounts(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	projectID := strings.TrimSpace(ctx.Query("projectId"))
	query := h.db.Model(&model.GitAccount{})
	conditions := []string{
		"scope = 'global'",
		"(scope = 'user' and owner_ref = ?)",
	}
	args := []any{user.ID}
	if projectID != "" {
		if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); !ok {
			return
		}
		conditions = append(conditions, "(scope = 'project' and exists (select 1 from scoped_resource_project_bindings srpb where srpb.resource_type = ? and srpb.resource_id = git_accounts.id and srpb.project_id = ?))")
		args = append(args, scopedResourceGitAccount, projectID)
	} else if user.Role == "platform_admin" {
		conditions = append(conditions, "scope = 'project'")
	} else {
		projectIDs := h.projectIDsForUser(user.ID)
		if len(projectIDs) > 0 {
			conditions = append(conditions, "(scope = 'project' and exists (select 1 from scoped_resource_project_bindings srpb where srpb.resource_type = ? and srpb.resource_id = git_accounts.id and srpb.project_id in ?))")
			args = append(args, scopedResourceGitAccount, projectIDs)
		}
	}
	query = query.Where(strings.Join(conditions, " or "), args...)

	var accounts []model.GitAccount
	query = applySearch(ctx, query, "username", "external_user_id")
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		if err := query.Order(orderByClause(pagination, map[string]string{
			"username":  "username",
			"status":    "status",
			"createdAt": "created_at",
		}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&accounts).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		h.attachGitAccountProjects(accounts)
		ctx.JSON(http.StatusOK, paginatedResponse(gitAccountResponses(accounts), total, pagination))
		return
	}
	if err := query.Order("created_at desc").Find(&accounts).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.attachGitAccountProjects(accounts)
	ctx.JSON(http.StatusOK, gitAccountResponses(accounts))
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
		Status:         normalizeGitAccountStatus(input.Status),
	}
	if strings.TrimSpace(input.AccessToken) != "" {
		account.AccessTokenRef = h.secrets.Store(input.AccessToken, user.ID, "git_account:"+account.ID+":access")
	}
	if strings.TrimSpace(input.RefreshToken) != "" {
		account.RefreshTokenRef = h.secrets.Store(input.RefreshToken, user.ID, "git_account:"+account.ID+":refresh")
	}
	if account.Username == "" {
		writeError(ctx, http.StatusBadRequest, "请输入 Git 账号用户名")
		return
	}

	if err := h.saveGitAccount(account); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "git_account.create", account.ID, true, account.Scope)
	ctx.JSON(http.StatusCreated, gitAccountResponse(account))
}

func (h *Handlers) UpdateGitAccount(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var account model.GitAccount
	if err := h.db.First(&account, "id = ?", ctx.Param("accountId")).Error; err != nil {
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
	account.ProviderID = strings.TrimSpace(input.ProviderID)
	account.Scope = scope
	account.OwnerRef = ownerRef
	account.ProjectIDs = projectIDs
	account.ExternalUserID = strings.TrimSpace(input.ExternalUserID)
	account.Username = strings.TrimSpace(input.Username)
	account.AvatarURL = strings.TrimSpace(input.AvatarURL)
	if strings.TrimSpace(input.AccessToken) != "" {
		account.AccessTokenRef = h.secrets.Store(input.AccessToken, user.ID, "git_account:"+account.ID+":access")
	}
	if strings.TrimSpace(input.RefreshToken) != "" {
		account.RefreshTokenRef = h.secrets.Store(input.RefreshToken, user.ID, "git_account:"+account.ID+":refresh")
	}
	account.Scopes = strings.Join(normalizeList(input.Scopes, false), ",")
	account.Status = normalizeGitAccountStatus(input.Status)
	if account.Username == "" {
		writeError(ctx, http.StatusBadRequest, "请输入 Git 账号用户名")
		return
	}

	if err := h.saveGitAccount(account); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "git_account.update", account.ID, true, account.Scope)
	ctx.JSON(http.StatusOK, gitAccountResponse(account))
}

func (h *Handlers) saveGitAccount(account model.GitAccount) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&account).Error; err != nil {
			return err
		}
		return h.replaceScopedResourceProjectBindings(tx, scopedResourceGitAccount, account.ID, sortedProjectIDs(account.ProjectIDs), nil)
	})
}

func (h *Handlers) attachGitAccountProjects(accounts []model.GitAccount) {
	projectMap := h.scopedResourceProjectIDMap(scopedResourceGitAccount, gitAccountIDs(accounts))
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

func (h *Handlers) DeleteGitAccount(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var account model.GitAccount
	if err := h.db.First(&account, "id = ?", ctx.Param("accountId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "git account not found")
		return
	}
	if !h.canManageGitAccount(ctx, user, account) {
		return
	}
	var bindingCount int64
	if err := h.db.Model(&model.RepositoryBinding{}).Where("git_account_id = ?", account.ID).Count(&bindingCount).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if bindingCount > 0 {
		var provider model.GitProvider
		err := h.db.Select("id").First(&provider, "id = ?", account.ProviderID).Error
		if err == nil {
			writeError(ctx, http.StatusConflict, "Git 凭据仍被仓库绑定引用，请先解绑")
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("resource_type = ? and resource_id = ?", scopedResourceGitAccount, account.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&account).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "git_account.delete", account.ID, true, account.Username)
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
	refreshToken := h.secrets.Resolve(account.RefreshTokenRef)
	if refreshToken == "" {
		writeError(ctx, http.StatusBadRequest, "git account has no refresh token")
		return account, false
	}
	oauthConfig, err := gitprovider.OAuthConfig(provider, h.gitOAuthRedirectURL(ctx), h.secrets.Resolve(provider.ClientSecretRef))
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
		account.Status = "expired"
		_ = h.db.Save(&account).Error
		h.audit(user.ID, "git_account.refresh", account.ID, false, "git token refresh failed")
		writeErrorCode(ctx, http.StatusBadRequest, "git.token_refresh_failed", "git token refresh failed")
		return account, false
	}
	account.AccessTokenRef = h.secrets.Store(token.AccessToken, account.UserID, "git_account:"+account.ID+":access")
	if token.RefreshToken != "" {
		account.RefreshTokenRef = h.secrets.Store(token.RefreshToken, account.UserID, "git_account:"+account.ID+":refresh")
	}
	if !token.Expiry.IsZero() {
		account.ExpiresAt = &token.Expiry
	}
	account.Status = "connected"
	if err := h.db.Save(&account).Error; err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "git.token_refresh_failed", "git token refresh failed")
		return account, false
	}
	h.audit(user.ID, "git_account.refresh", account.ID, true, account.Username)
	return account, true
}

func gitAccountNeedsRefresh(account model.GitAccount) bool {
	if account.ExpiresAt == nil {
		return false
	}
	return time.Until(*account.ExpiresAt) <= 5*time.Minute
}

func (h *Handlers) upsertGitAccountFromOAuth(userID string, provider model.GitProvider, gitUser gitprovider.UserResponse, token *oauth2.Token) (model.GitAccount, error) {
	externalID := gitUser.ExternalID()
	username := gitUser.Username()
	if externalID == "" || username == "" {
		return model.GitAccount{}, fmt.Errorf("git user identity is incomplete")
	}
	var account model.GitAccount
	err := h.db.First(&account, "user_id = ? and provider_id = ? and external_user_id = ?", userID, provider.ID, externalID).Error
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
	account.AccessTokenRef = h.secrets.Store(token.AccessToken, userID, "git_account:"+account.ID+":access")
	if token.RefreshToken != "" {
		account.RefreshTokenRef = h.secrets.Store(token.RefreshToken, userID, "git_account:"+account.ID+":refresh")
	}
	account.Scopes = strings.Join(normalizeList(tokenScopes(token), false), ",")
	if !token.Expiry.IsZero() {
		account.ExpiresAt = &token.Expiry
	}
	account.Status = "connected"
	if err == gorm.ErrRecordNotFound {
		if err := h.db.Create(&account).Error; err != nil {
			return account, err
		}
		h.audit(userID, "git_account.oauth_upsert", account.ID, true, provider.ID)
		return account, nil
	}
	if err := h.db.Save(&account).Error; err != nil {
		return account, err
	}
	h.audit(userID, "git_account.oauth_upsert", account.ID, true, provider.ID)
	return account, nil
}

func tokenScopes(token *oauth2.Token) []string {
	scope, _ := token.Extra("scope").(string)
	if scope == "" {
		return nil
	}
	return strings.Fields(strings.ReplaceAll(scope, ",", " "))
}
