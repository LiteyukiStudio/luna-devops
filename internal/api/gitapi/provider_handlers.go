package gitapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const gitOAuthStateTTL = 10 * time.Minute

func (h *Handlers) ListGitProviders(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	visibility, ok := h.resolveListVisibility(ctx, user)
	if !ok {
		return
	}

	query := h.dbFor(ctx).Model(&model.GitProvider{})
	if user.Role != authz.PlatformRoleAdmin {
		query = query.Where("enabled = ?", true)
	}

	projectID := strings.TrimSpace(ctx.Query("projectId"))
	var visible bool
	query, visible = h.applyScopedResourceListVisibility(ctx, query, scopedResourceGitProvider, user, projectID, visibility)
	if !visible {
		return
	}

	var providers []model.GitProvider
	query = applySearch(ctx, query, "name", "base_url")
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"name": "name", "scope": "scope", "createdAt": "created_at"}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"scope":     "scope",
		"createdAt": "created_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&providers).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	responses, err := h.gitProviderResponsesForUser(user, providers, ctx.Request.Context())
	if err != nil {
		h.host.WriteProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(responses, total, pagination))
}

func (h *Handlers) StartGitOAuth(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		h.debugLog("git.oauth.start current user missing providerId=%s", ctx.Param("providerId"))
		return
	}
	provider, ok := h.findEnabledGitProvider(ctx, ctx.Param("providerId"))
	if !ok {
		h.debugLog("git.oauth.start provider unavailable providerId=%s userId=%s", ctx.Param("providerId"), user.ID)
		return
	}
	h.debugLog("git.oauth.start provider loaded providerId=%s type=%s baseUrl=%s scope=%s ownerRef=%s authType=%s userId=%s", provider.ID, provider.Type, provider.BaseURL, provider.Scope, provider.OwnerRef, provider.AuthType, user.ID)
	if provider.AuthType != "oauth" {
		h.debugLog("git.oauth.start provider auth type rejected providerId=%s authType=%s", provider.ID, provider.AuthType)
		writeError(ctx, http.StatusBadRequest, "git provider does not use oauth")
		return
	}
	if strings.TrimSpace(provider.ClientID) == "" || strings.TrimSpace(provider.ClientSecretRef) == "" {
		h.debugLog("git.oauth.start provider oauth incomplete providerId=%s clientIdSet=%t clientSecretRefSet=%t", provider.ID, strings.TrimSpace(provider.ClientID) != "", strings.TrimSpace(provider.ClientSecretRef) != "")
		writeError(ctx, http.StatusBadRequest, "git provider oauth client is not configured")
		return
	}
	baseURL := strings.TrimSpace(h.externalBaseURL(ctx))
	if baseURL == "" {
		h.debugLog("git.oauth.start public base url missing providerId=%s", provider.ID)
		writeError(ctx, http.StatusInternalServerError, "PUBLIC_BASE_URL is required")
		return
	}
	callbackBaseURL := sanitizeFrontendOrigin(ctx.Query("callbackOrigin"), baseURL)
	h.debugLog("git.oauth.start origins providerId=%s baseURL=%s frontendOriginRaw=%s frontendOrigin=%s callbackOriginRaw=%s callbackOrigin=%s redirectPathRaw=%s", provider.ID, baseURL, ctx.Query("frontendOrigin"), sanitizeFrontendOrigin(ctx.Query("frontendOrigin"), baseURL), ctx.Query("callbackOrigin"), callbackBaseURL, ctx.DefaultQuery("redirect", "/projects"))

	state := "git_" + randomHex(32)
	oauthState := gitOAuthStateValue{
		ProviderID:     provider.ID,
		UserID:         user.ID,
		RedirectPath:   sanitizeRedirectPath(ctx.DefaultQuery("redirect", "/projects")),
		FrontendOrigin: sanitizeFrontendOrigin(ctx.Query("frontendOrigin"), baseURL),
		CallbackOrigin: callbackBaseURL,
	}
	if err := h.oauthStates.SaveGit(ctx.Request.Context(), state, oauthState, gitOAuthStateTTL); err != nil {
		h.debugLog("git.oauth.start state save failed providerId=%s userId=%s stateHash=%s err=%v", provider.ID, user.ID, shortDebugHash(state), err)
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.debugLog("git.oauth.start state saved providerId=%s userId=%s stateHash=%s ttl=%s redirectPath=%s frontendOrigin=%s callbackOrigin=%s", provider.ID, user.ID, shortDebugHash(state), gitOAuthStateTTL, oauthState.RedirectPath, oauthState.FrontendOrigin, oauthState.CallbackOrigin)
	h.auditWithContext(user.ID, "git.oauth.start", provider.ID, true, oauthState.RedirectPath, ctx.Request.Context())

	clientSecret := h.secrets.ResolveContext(ctx.Request.Context(), provider.ClientSecretRef)
	h.debugLog("git.oauth.start secret resolved providerId=%s clientSecretSet=%t clientSecretRefSet=%t", provider.ID, strings.TrimSpace(clientSecret) != "", strings.TrimSpace(provider.ClientSecretRef) != "")
	oauthConfig, err := gitprovider.OAuthConfig(provider, gitOAuthCallbackURL(callbackBaseURL), clientSecret)
	if err != nil {
		h.debugLog("git.oauth.start oauth config failed providerId=%s err=%v", provider.ID, err)
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.debugLog("git.oauth.start oauth config providerId=%s authURL=%s tokenURL=%s redirectURL=%s scopes=%s clientIdSet=%t", provider.ID, oauthConfig.Endpoint.AuthURL, oauthConfig.Endpoint.TokenURL, oauthConfig.RedirectURL, strings.Join(oauthConfig.Scopes, ","), strings.TrimSpace(oauthConfig.ClientID) != "")
	if _, err := h.egressPolicyForUser(user, ctx.Request.Context()).ValidateURL(oauthConfig.Endpoint.AuthURL); err != nil {
		h.debugLog("git.oauth.start auth url blocked providerId=%s authURL=%s err=%v", provider.ID, oauthConfig.Endpoint.AuthURL, err)
		writeError(ctx, http.StatusForbidden, "Git OAuth 授权地址不符合访问策略")
		return
	}
	h.debugLog("git.oauth.start redirecting providerId=%s stateHash=%s authURL=%s redirectURL=%s", provider.ID, shortDebugHash(state), oauthConfig.Endpoint.AuthURL, oauthConfig.RedirectURL)
	ctx.Redirect(http.StatusFound, oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline))
}

func (h *Handlers) CompleteGitOAuth(ctx *gin.Context) {
	plainState := strings.TrimSpace(ctx.Query("state"))
	code := strings.TrimSpace(ctx.Query("code"))
	h.debugLog("git.oauth.complete callback received statePresent=%t stateHash=%s codePresent=%t codeLength=%d error=%s", plainState != "", shortDebugHash(plainState), code != "", len(code), ctx.Query("error"))
	if plainState == "" || code == "" {
		h.debugLog("git.oauth.complete invalid callback statePresent=%t codePresent=%t error=%s", plainState != "", code != "", ctx.Query("error"))
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_callback_invalid")
		return
	}
	baseURL := strings.TrimSpace(h.externalBaseURL(ctx))
	if baseURL == "" {
		h.debugLog("git.oauth.complete public base url missing stateHash=%s", shortDebugHash(plainState))
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_callback_invalid")
		return
	}
	h.debugLog("git.oauth.complete base url stateHash=%s baseURL=%s requestURL=%s", shortDebugHash(plainState), baseURL, ctx.Request.URL.String())

	oauthState, ok, err := h.oauthStates.ConsumeGit(ctx.Request.Context(), plainState)
	if err != nil {
		h.debugLog("git.oauth.complete state consume failed stateHash=%s err=%v", shortDebugHash(plainState), err)
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_state_invalid")
		return
	}
	if !ok {
		h.debugLog("git.oauth.complete state missing stateHash=%s", shortDebugHash(plainState))
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_state_invalid")
		return
	}
	h.debugLog("git.oauth.complete state consumed stateHash=%s providerId=%s userId=%s redirectPath=%s frontendOrigin=%s callbackOrigin=%s", shortDebugHash(plainState), oauthState.ProviderID, oauthState.UserID, oauthState.RedirectPath, oauthState.FrontendOrigin, oauthState.CallbackOrigin)

	var stateUser model.User
	if err := h.dbFor(ctx).First(&stateUser, "id = ? and disabled = ?", oauthState.UserID, false).Error; err != nil {
		h.debugLog("git.oauth.complete state user missing userId=%s providerId=%s err=%v", oauthState.UserID, oauthState.ProviderID, err)
		h.auditWithContext(oauthState.UserID, "git.oauth.complete", oauthState.ProviderID, false, "user disabled or missing", ctx.Request.Context())
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_user_invalid")
		return
	}
	h.debugLog("git.oauth.complete state user loaded userId=%s email=%s role=%s", stateUser.ID, stateUser.Email, stateUser.Role)

	var provider model.GitProvider
	if err := h.dbFor(ctx).First(&provider, "id = ? and enabled = ?", oauthState.ProviderID, true).Error; err != nil {
		h.debugLog("git.oauth.complete provider missing providerId=%s err=%v", oauthState.ProviderID, err)
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_provider_disabled")
		return
	}
	h.debugLog("git.oauth.complete provider loaded providerId=%s type=%s baseUrl=%s scope=%s ownerRef=%s authType=%s", provider.ID, provider.Type, provider.BaseURL, provider.Scope, provider.OwnerRef, provider.AuthType)
	if !h.canUseScopedResourceByID(stateUser, provider.Scope, provider.OwnerRef, scopedResourceGitProvider, provider.ID, ctx.Request.Context()) {
		h.debugLog("git.oauth.complete provider forbidden providerId=%s userId=%s", provider.ID, stateUser.ID)
		h.auditWithContext(oauthState.UserID, "git.oauth.complete", oauthState.ProviderID, false, "provider access denied", ctx.Request.Context())
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_provider_forbidden")
		return
	}
	callbackBaseURL := strings.TrimSpace(oauthState.CallbackOrigin)
	if callbackBaseURL == "" {
		callbackBaseURL = baseURL
	}
	clientSecret := h.secrets.ResolveContext(ctx.Request.Context(), provider.ClientSecretRef)
	h.debugLog("git.oauth.complete secret resolved providerId=%s clientSecretSet=%t clientSecretRefSet=%t", provider.ID, strings.TrimSpace(clientSecret) != "", strings.TrimSpace(provider.ClientSecretRef) != "")
	oauthConfig, err := gitprovider.OAuthConfig(provider, gitOAuthCallbackURL(callbackBaseURL), clientSecret)
	if err != nil {
		h.debugLog("git.oauth.complete oauth config failed providerId=%s err=%v", provider.ID, err)
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_provider_invalid")
		return
	}
	h.debugLog("git.oauth.complete oauth config providerId=%s authURL=%s tokenURL=%s redirectURL=%s scopes=%s", provider.ID, oauthConfig.Endpoint.AuthURL, oauthConfig.Endpoint.TokenURL, oauthConfig.RedirectURL, strings.Join(oauthConfig.Scopes, ","))
	egressCtx := h.egressContextForUser(ctx.Request.Context(), stateUser, 15*time.Second)
	h.debugLog("git.oauth.complete exchanging code providerId=%s tokenURL=%s redirectURL=%s codeLength=%d", provider.ID, oauthConfig.Endpoint.TokenURL, oauthConfig.RedirectURL, len(code))
	token, err := oauthConfig.Exchange(egressCtx, code)
	if err != nil {
		h.debugLog("git.oauth.complete code exchange failed providerId=%s tokenURL=%s redirectURL=%s err=%v", provider.ID, oauthConfig.Endpoint.TokenURL, oauthConfig.RedirectURL, err)
		h.auditWithContext(oauthState.UserID, "git.oauth.complete", provider.ID, false, "code exchange failed", ctx.Request.Context())
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_code_invalid")
		return
	}
	h.debugLog("git.oauth.complete code exchange succeeded providerId=%s accessTokenSet=%t refreshTokenSet=%t expiry=%s tokenType=%s", provider.ID, token.AccessToken != "", token.RefreshToken != "", token.Expiry.Format(time.RFC3339), token.TokenType)

	client := gitprovider.NewClientWithPolicy(provider, token.AccessToken, h.egressPolicyForUser(stateUser, ctx.Request.Context()))
	h.debugLog("git.oauth.complete loading git user providerId=%s type=%s baseUrl=%s", provider.ID, provider.Type, provider.BaseURL)
	gitUser, err := client.CurrentUser(egressCtx)
	if err != nil {
		h.debugLog("git.oauth.complete git user failed providerId=%s err=%v", provider.ID, err)
		h.auditWithContext(oauthState.UserID, "git.oauth.complete", provider.ID, false, "git user failed", ctx.Request.Context())
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_user_failed")
		return
	}
	h.debugLog("git.oauth.complete git user loaded providerId=%s externalUserId=%s username=%s", provider.ID, gitUser.ExternalID(), gitUser.Username())
	account, err := h.upsertGitAccountFromOAuth(oauthState.UserID, provider, gitUser, token, ctx.Request.Context())
	if err != nil {
		h.debugLog("git.oauth.complete account save failed providerId=%s userId=%s externalUserId=%s err=%v", provider.ID, oauthState.UserID, gitUser.ID, err)
		h.auditWithContext(oauthState.UserID, "git.oauth.complete", provider.ID, false, "save failed", ctx.Request.Context())
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_save_failed")
		return
	}
	h.debugLog("git.oauth.complete account saved accountId=%s providerId=%s userId=%s username=%s", account.ID, provider.ID, account.UserID, account.Username)
	h.auditWithContext(stateUser.ID, "git.oauth.complete", provider.ID, true, account.ID, ctx.Request.Context())

	redirectTarget := buildFrontendRedirect(baseURL, oauthState.FrontendOrigin, oauthState.RedirectPath, account.ID)
	h.debugLog("git.oauth.complete redirecting accountId=%s target=%s", account.ID, redirectTarget)
	ctx.Redirect(http.StatusFound, redirectTarget)
}

func buildFrontendRedirect(defaultOrigin, frontendOrigin, path, accountID string) string {
	targetPath := sanitizeRedirectPath(path)
	targetOrigin := sanitizeFrontendOrigin(frontendOrigin, defaultOrigin)
	separator := "?"
	if strings.Contains(targetPath, "?") {
		separator = "&"
	}
	return targetOrigin + targetPath + separator + "gitAccountId=" + url.QueryEscape(accountID)
}

func gitOAuthCallbackURL(origin string) string {
	return strings.TrimRight(origin, "/") + "/api/v1/git/oauth/callback"
}

func sanitizeFrontendOrigin(raw, defaultOrigin string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return strings.TrimRight(defaultOrigin, "/")
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return strings.TrimRight(defaultOrigin, "/")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return strings.TrimRight(defaultOrigin, "/")
	}

	defaultParsed, defaultErr := url.Parse(defaultOrigin)
	if defaultErr != nil || defaultParsed.Hostname() == "" {
		return strings.TrimRight(parsed.Scheme+"//"+parsed.Host, "/")
	}

	candidateHost := strings.ToLower(parsed.Hostname())
	referenceHost := strings.ToLower(defaultParsed.Hostname())
	if candidateHost == referenceHost || isLoopbackPair(candidateHost, referenceHost) {
		return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/")
	}
	return strings.TrimRight(defaultParsed.Scheme+"://"+defaultParsed.Host, "/")
}

func isLoopbackPair(left, right string) bool {
	return (left == "localhost" && right == "127.0.0.1") || (left == "127.0.0.1" && right == "localhost")
}

func (h *Handlers) CreateGitProvider(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}

	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input gitProviderInput
	if !bindJSON(ctx, &input) {
		return
	}

	scope := normalizeGitScope(input.Scope)
	projectIDs := normalizeStringList(input.ProjectIDs)
	providerType := normalizeGitProviderType(input.Type)
	if providerType == "github" {
		scope = "global"
		input.OwnerRef = ""
		projectIDs = nil
		if err := h.requireSingleGitHubProvider(ctx, ""); err != nil {
			return
		}
	} else {
		normalizedScope, ownerRef, normalizedProjectIDs, ok := h.normalizeScopedOwnerWithProjects(ctx, user, scope, input.OwnerRef, projectIDs, "只有平台管理员可以维护全局 Git Provider")
		if !ok {
			return
		}
		scope = normalizedScope
		input.OwnerRef = ownerRef
		projectIDs = normalizedProjectIDs
	}

	provider := model.GitProvider{
		ID:         id.New("gitp"),
		Type:       providerType,
		Name:       strings.TrimSpace(input.Name),
		BaseURL:    normalizeGitBaseURL(providerType, input.BaseURL),
		Scope:      scope,
		OwnerRef:   strings.TrimSpace(input.OwnerRef),
		ProjectIDs: projectIDs,
		AuthType:   normalizeGitAuthType(input.AuthType),
		ClientID:   strings.TrimSpace(input.ClientID),
		Enabled:    input.Enabled,
	}
	if provider.Name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入 Git Provider 名称")
		return
	}
	if provider.BaseURL == "" {
		provider.BaseURL = defaultGitBaseURL(provider.Type)
	}

	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if strings.TrimSpace(input.ClientSecret) != "" && h.secrets.Available() {
			ref, storeErr := h.secrets.StoreContextWithDB(ctx.Request.Context(), tx, input.ClientSecret, user.ID, "git_provider:"+provider.ID)
			if storeErr != nil {
				return storeErr
			}
			provider.ClientSecretRef = ref
		}
		if err := tx.Save(&provider).Error; err != nil {
			return err
		}
		return h.replaceScopedResourceProjectBindings(tx, scopedResourceGitProvider, provider.ID, sortedProjectIDs(provider.ProjectIDs), nil)
	})
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "git_provider.create", provider.ID, true, provider.Type, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, gitProviderResponse(provider))
}

func (h *Handlers) UpdateGitProvider(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}

	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var provider model.GitProvider
	if err := h.dbFor(ctx).First(&provider, "id = ?", ctx.Param("providerId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "git provider not found")
		return
	}

	var input gitProviderInput
	if !bindJSON(ctx, &input) {
		return
	}

	providerType := normalizeGitProviderType(input.Type)
	scope := normalizeGitScope(input.Scope)
	projectIDs := normalizeStringList(input.ProjectIDs)
	if providerType == "github" {
		scope = "global"
		projectIDs = nil
		if !h.providerIsSingleFor(ctx, provider.ID, providerType) {
			writeError(ctx, http.StatusBadRequest, "GitHub Provider 仅支持单个实例配置")
			return
		}
		input.OwnerRef = ""
	} else {
		normalizedScope, ownerRef, normalizedProjectIDs, ok := h.normalizeScopedOwnerWithProjects(ctx, user, scope, input.OwnerRef, projectIDs, "只有平台管理员可以维护全局 Git Provider")
		if !ok {
			return
		}
		scope = normalizedScope
		input.OwnerRef = ownerRef
		projectIDs = normalizedProjectIDs
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入 Git Provider 名称")
		return
	}
	baseURL := normalizeGitBaseURL(providerType, input.BaseURL)
	ownerRef := strings.TrimSpace(input.OwnerRef)
	authType := normalizeGitAuthType(input.AuthType)
	clientID := strings.TrimSpace(input.ClientID)

	var updated model.GitProvider
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.GitProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", provider.ID).Error; err != nil {
			return err
		}
		locked.Type = providerType
		locked.Name = name
		locked.BaseURL = baseURL
		locked.Scope = scope
		locked.OwnerRef = ownerRef
		locked.ProjectIDs = projectIDs
		locked.AuthType = authType
		locked.ClientID = clientID
		locked.Enabled = input.Enabled
		if compatErr := ensureGitProviderScopeCompatibleTx(tx, locked); compatErr != nil {
			return compatErr
		}
		if strings.TrimSpace(input.ClientSecret) != "" && h.secrets.Available() {
			resource := "git_provider:" + locked.ID
			newRef, storeErr := h.secrets.StoreContextWithDB(ctx.Request.Context(), tx, input.ClientSecret, user.ID, resource)
			if storeErr != nil {
				return storeErr
			}
			if strings.TrimSpace(locked.ClientSecretRef) != "" && locked.ClientSecretRef != newRef {
				if delErr := h.secrets.DeleteRefContextWithDB(ctx.Request.Context(), tx, locked.ClientSecretRef, resource); delErr != nil {
					return delErr
				}
			}
			locked.ClientSecretRef = newRef
		}
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceGitProvider, locked.ID, sortedProjectIDs(locked.ProjectIDs), nil); err != nil {
			return err
		}
		updated = locked
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "git provider not found")
		return
	}
	if errors.Is(err, errGitCredentialScopeConflict) {
		writeErrorCode(ctx, http.StatusConflict, "git.credential_scope_conflict", "child git accounts exceed the target provider scope")
		return
	}
	if errors.Is(err, errGitProviderInUse) {
		writeErrorCode(ctx, http.StatusConflict, "git.provider_in_use", "git provider is referenced by repository bindings outside the target scope")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "git_provider.update", updated.ID, true, updated.Type, ctx.Request.Context())
	ctx.JSON(http.StatusOK, gitProviderResponse(updated))
}

func (h *Handlers) DeleteGitProvider(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var provider model.GitProvider
	if err := h.dbFor(ctx).First(&provider, "id = ?", ctx.Param("providerId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "git provider not found")
		return
	}
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.GitProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", provider.ID).Error; err != nil {
			return err
		}
		var accounts []model.GitAccount
		if err := tx.Where("provider_id = ?", locked.ID).Find(&accounts).Error; err != nil {
			return err
		}
		accountIDs := gitAccountIDs(accounts)
		bindingQuery := tx.Model(&model.RepositoryBinding{}).
			Joins("join projects on projects.id = repository_bindings.project_id and projects.deleted_at is null")
		if len(accountIDs) > 0 {
			bindingQuery = bindingQuery.Where("repository_bindings.git_provider_id = ? or repository_bindings.git_account_id in ?", locked.ID, accountIDs)
		} else {
			bindingQuery = bindingQuery.Where("repository_bindings.git_provider_id = ?", locked.ID)
		}
		var bindingCount int64
		if err := bindingQuery.Count(&bindingCount).Error; err != nil {
			return err
		}
		if bindingCount > 0 {
			return errGitProviderInUse
		}
		for _, account := range accounts {
			if err := h.secrets.DeleteRefContextWithDB(ctx.Request.Context(), tx, account.AccessTokenRef, "git_account:"+account.ID+":access"); err != nil {
				return err
			}
			if err := h.secrets.DeleteRefContextWithDB(ctx.Request.Context(), tx, account.RefreshTokenRef, "git_account:"+account.ID+":refresh"); err != nil {
				return err
			}
		}
		if err := h.secrets.DeleteRefContextWithDB(ctx.Request.Context(), tx, locked.ClientSecretRef, "git_provider:"+locked.ID); err != nil {
			return err
		}
		if err := tx.Where("provider_id = ?", locked.ID).Delete(&model.GitAccount{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&locked).Error; err != nil {
			return err
		}
		if err := tx.Where("resource_type = ? and resource_id = ?", scopedResourceGitProvider, locked.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		if len(accountIDs) > 0 {
			return tx.Where("resource_type = ? and resource_id in ?", scopedResourceGitAccount, accountIDs).Delete(&model.ScopedResourceProjectBinding{}).Error
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "git provider not found")
		return
	}
	if errors.Is(err, errGitProviderInUse) {
		writeErrorCode(ctx, http.StatusConflict, "git.provider_in_use", "git provider is referenced by repository bindings")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "git_provider.delete", provider.ID, true, provider.Type, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

var (
	errGitProviderInUse           = errors.New("git provider is referenced by repository bindings")
	errGitCredentialScopeConflict = errors.New("child git accounts exceed the target provider scope")
)

func ensureGitProviderScopeCompatibleTx(tx *gorm.DB, provider model.GitProvider) error {
	if provider.Scope == "global" {
		return nil
	}
	var accounts []model.GitAccount
	if err := tx.Where("provider_id = ?", provider.ID).Find(&accounts).Error; err != nil {
		return err
	}
	allowedProjects := make(map[string]bool, len(provider.ProjectIDs))
	for _, projectID := range provider.ProjectIDs {
		allowedProjects[strings.TrimSpace(projectID)] = true
	}
	accountIDs := gitAccountIDs(accounts)
	accountProjectMap := make(map[string][]string, len(accountIDs))
	if len(accountIDs) > 0 {
		var bindings []model.ScopedResourceProjectBinding
		if err := tx.Where("resource_type = ? and resource_id in ?", scopedResourceGitAccount, accountIDs).Find(&bindings).Error; err != nil {
			return err
		}
		for _, binding := range bindings {
			accountProjectMap[binding.ResourceID] = append(accountProjectMap[binding.ResourceID], binding.ProjectID)
		}
	}
	for _, account := range accounts {
		accountScope := normalizeGitScope(account.Scope)
		switch provider.Scope {
		case "user":
			if accountScope != "user" || strings.TrimSpace(account.OwnerRef) != strings.TrimSpace(provider.OwnerRef) {
				return errGitCredentialScopeConflict
			}
		case "project":
			if accountScope == "global" {
				return errGitCredentialScopeConflict
			}
			if accountScope == "project" {
				for _, boundProjectID := range accountProjectMap[account.ID] {
					if !allowedProjects[boundProjectID] {
						return errGitCredentialScopeConflict
					}
				}
			}
		}
	}

	bindingQuery := tx.Model(&model.RepositoryBinding{}).
		Joins("join projects on projects.id = repository_bindings.project_id and projects.deleted_at is null").
		Where("repository_bindings.git_provider_id = ?", provider.ID)
	switch provider.Scope {
	case "project":
		if len(provider.ProjectIDs) > 0 {
			bindingQuery = bindingQuery.Where("repository_bindings.project_id not in ?", provider.ProjectIDs)
		}
	case "user":
		memberProjects := tx.Model(&model.ProjectMember{}).Select("project_id").Where("user_id = ?", provider.OwnerRef)
		bindingQuery = bindingQuery.Where("repository_bindings.project_id not in (?)", memberProjects)
	}
	var conflictBindings int64
	if err := bindingQuery.Count(&conflictBindings).Error; err != nil {
		return err
	}
	if conflictBindings > 0 {
		return errGitProviderInUse
	}
	return nil
}
