package api

import (
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/LiteyukiStudio/devops/internal/service"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const gitOAuthStateTTL = 10 * time.Minute

func (h *Handlers) ListGitProviders(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	query := h.db.Order("created_at desc")
	if user.Role != "platform_admin" {
		query = query.Where("enabled = ?", true)
	}

	projectID := strings.TrimSpace(ctx.Query("projectId"))
	conditions := []string{"scope = 'global'", "(scope = 'user' and owner_ref = ?)"}
	args := []any{user.ID}
	if projectID != "" {
		if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); !ok {
			return
		}
		conditions = append(conditions, "(scope = 'project' and owner_ref = ?)")
		args = append(args, projectID)
	} else if user.Role == "platform_admin" {
		conditions = append(conditions, "scope = 'project'")
	} else {
		projectIDs := h.projectIDsForUser(user.ID)
		if len(projectIDs) > 0 {
			conditions = append(conditions, "(scope = 'project' and owner_ref in ?)")
			args = append(args, projectIDs)
		}
	}
	query = query.Where(strings.Join(conditions, " or "), args...)

	var providers []model.GitProvider
	query = applySearch(ctx, query, "name", "base_url")
	if err := query.Find(&providers).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.gitProviderResponsesForUser(user, providers))
}

func (h *Handlers) StartGitOAuth(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		debugLog("git.oauth.start current user missing providerId=%s", ctx.Param("providerId"))
		return
	}
	provider, ok := h.findEnabledGitProvider(ctx, ctx.Param("providerId"))
	if !ok {
		debugLog("git.oauth.start provider unavailable providerId=%s userId=%s", ctx.Param("providerId"), user.ID)
		return
	}
	debugLog("git.oauth.start provider loaded providerId=%s type=%s baseUrl=%s scope=%s ownerRef=%s authType=%s userId=%s", provider.ID, provider.Type, provider.BaseURL, provider.Scope, provider.OwnerRef, provider.AuthType, user.ID)
	if provider.AuthType != "oauth" {
		debugLog("git.oauth.start provider auth type rejected providerId=%s authType=%s", provider.ID, provider.AuthType)
		writeError(ctx, http.StatusBadRequest, "git provider does not use oauth")
		return
	}
	if strings.TrimSpace(provider.ClientID) == "" || strings.TrimSpace(provider.ClientSecretRef) == "" {
		debugLog("git.oauth.start provider oauth incomplete providerId=%s clientIdSet=%t clientSecretRefSet=%t", provider.ID, strings.TrimSpace(provider.ClientID) != "", strings.TrimSpace(provider.ClientSecretRef) != "")
		writeError(ctx, http.StatusBadRequest, "git provider oauth client is not configured")
		return
	}
	baseURL := strings.TrimSpace(h.externalBaseURL(ctx))
	if baseURL == "" {
		debugLog("git.oauth.start public base url missing providerId=%s", provider.ID)
		writeError(ctx, http.StatusInternalServerError, "PUBLIC_BASE_URL is required")
		return
	}
	callbackBaseURL := sanitizeFrontendOrigin(ctx.Query("callbackOrigin"), baseURL)
	debugLog("git.oauth.start origins providerId=%s baseURL=%s frontendOriginRaw=%s frontendOrigin=%s callbackOriginRaw=%s callbackOrigin=%s redirectPathRaw=%s", provider.ID, baseURL, ctx.Query("frontendOrigin"), sanitizeFrontendOrigin(ctx.Query("frontendOrigin"), baseURL), ctx.Query("callbackOrigin"), callbackBaseURL, ctx.DefaultQuery("redirect", "/projects"))

	state := "git_" + randomHex(32)
	oauthState := gitOAuthStateValue{
		ProviderID:     provider.ID,
		UserID:         user.ID,
		RedirectPath:   sanitizeRedirectPath(ctx.DefaultQuery("redirect", "/projects")),
		FrontendOrigin: sanitizeFrontendOrigin(ctx.Query("frontendOrigin"), baseURL),
		CallbackOrigin: callbackBaseURL,
	}
	if err := h.oauthStates.SaveGit(ctx.Request.Context(), state, oauthState, gitOAuthStateTTL); err != nil {
		debugLog("git.oauth.start state save failed providerId=%s userId=%s stateHash=%s err=%v", provider.ID, user.ID, shortDebugHash(state), err)
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	debugLog("git.oauth.start state saved providerId=%s userId=%s stateHash=%s ttl=%s redirectPath=%s frontendOrigin=%s callbackOrigin=%s", provider.ID, user.ID, shortDebugHash(state), gitOAuthStateTTL, oauthState.RedirectPath, oauthState.FrontendOrigin, oauthState.CallbackOrigin)
	h.audit(user.ID, "git.oauth.start", provider.ID, true, oauthState.RedirectPath)

	clientSecret := h.secrets.Resolve(provider.ClientSecretRef)
	debugLog("git.oauth.start secret resolved providerId=%s clientSecretSet=%t clientSecretRefSet=%t", provider.ID, strings.TrimSpace(clientSecret) != "", strings.TrimSpace(provider.ClientSecretRef) != "")
	oauthConfig, err := gitprovider.OAuthConfig(provider, gitOAuthCallbackURL(callbackBaseURL), clientSecret)
	if err != nil {
		debugLog("git.oauth.start oauth config failed providerId=%s err=%v", provider.ID, err)
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	debugLog("git.oauth.start oauth config providerId=%s authURL=%s tokenURL=%s redirectURL=%s scopes=%s clientIdSet=%t", provider.ID, oauthConfig.Endpoint.AuthURL, oauthConfig.Endpoint.TokenURL, oauthConfig.RedirectURL, strings.Join(oauthConfig.Scopes, ","), strings.TrimSpace(oauthConfig.ClientID) != "")
	if _, err := h.egressPolicyForUser(user).ValidateURL(oauthConfig.Endpoint.AuthURL); err != nil {
		debugLog("git.oauth.start auth url blocked providerId=%s authURL=%s err=%v", provider.ID, oauthConfig.Endpoint.AuthURL, err)
		writeError(ctx, http.StatusForbidden, "Git OAuth 授权地址不符合访问策略")
		return
	}
	debugLog("git.oauth.start redirecting providerId=%s stateHash=%s authURL=%s redirectURL=%s", provider.ID, shortDebugHash(state), oauthConfig.Endpoint.AuthURL, oauthConfig.RedirectURL)
	ctx.Redirect(http.StatusFound, oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline))
}

func (h *Handlers) CompleteGitOAuth(ctx *gin.Context) {
	plainState := strings.TrimSpace(ctx.Query("state"))
	code := strings.TrimSpace(ctx.Query("code"))
	debugLog("git.oauth.complete callback received statePresent=%t stateHash=%s codePresent=%t codeLength=%d error=%s", plainState != "", shortDebugHash(plainState), code != "", len(code), ctx.Query("error"))
	if plainState == "" || code == "" {
		debugLog("git.oauth.complete invalid callback statePresent=%t codePresent=%t error=%s", plainState != "", code != "", ctx.Query("error"))
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_callback_invalid")
		return
	}
	baseURL := strings.TrimSpace(h.externalBaseURL(ctx))
	if baseURL == "" {
		debugLog("git.oauth.complete public base url missing stateHash=%s", shortDebugHash(plainState))
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_callback_invalid")
		return
	}
	debugLog("git.oauth.complete base url stateHash=%s baseURL=%s requestURL=%s", shortDebugHash(plainState), baseURL, ctx.Request.URL.String())

	oauthState, ok, err := h.oauthStates.ConsumeGit(ctx.Request.Context(), plainState)
	if err != nil {
		debugLog("git.oauth.complete state consume failed stateHash=%s err=%v", shortDebugHash(plainState), err)
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_state_invalid")
		return
	}
	if !ok {
		debugLog("git.oauth.complete state missing stateHash=%s", shortDebugHash(plainState))
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_state_invalid")
		return
	}
	debugLog("git.oauth.complete state consumed stateHash=%s providerId=%s userId=%s redirectPath=%s frontendOrigin=%s callbackOrigin=%s", shortDebugHash(plainState), oauthState.ProviderID, oauthState.UserID, oauthState.RedirectPath, oauthState.FrontendOrigin, oauthState.CallbackOrigin)

	var stateUser model.User
	if err := h.db.First(&stateUser, "id = ? and disabled = ?", oauthState.UserID, false).Error; err != nil {
		debugLog("git.oauth.complete state user missing userId=%s providerId=%s err=%v", oauthState.UserID, oauthState.ProviderID, err)
		h.audit(oauthState.UserID, "git.oauth.complete", oauthState.ProviderID, false, "user disabled or missing")
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_user_invalid")
		return
	}
	debugLog("git.oauth.complete state user loaded userId=%s email=%s role=%s", stateUser.ID, stateUser.Email, stateUser.Role)

	var provider model.GitProvider
	if err := h.db.First(&provider, "id = ? and enabled = ?", oauthState.ProviderID, true).Error; err != nil {
		debugLog("git.oauth.complete provider missing providerId=%s err=%v", oauthState.ProviderID, err)
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_provider_disabled")
		return
	}
	debugLog("git.oauth.complete provider loaded providerId=%s type=%s baseUrl=%s scope=%s ownerRef=%s authType=%s", provider.ID, provider.Type, provider.BaseURL, provider.Scope, provider.OwnerRef, provider.AuthType)
	if !service.CanUseGitProvider(stateUser, provider, h.projects.UserHasProject) {
		debugLog("git.oauth.complete provider forbidden providerId=%s userId=%s", provider.ID, stateUser.ID)
		h.audit(oauthState.UserID, "git.oauth.complete", oauthState.ProviderID, false, "provider access denied")
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_provider_forbidden")
		return
	}
	callbackBaseURL := strings.TrimSpace(oauthState.CallbackOrigin)
	if callbackBaseURL == "" {
		callbackBaseURL = baseURL
	}
	clientSecret := h.secrets.Resolve(provider.ClientSecretRef)
	debugLog("git.oauth.complete secret resolved providerId=%s clientSecretSet=%t clientSecretRefSet=%t", provider.ID, strings.TrimSpace(clientSecret) != "", strings.TrimSpace(provider.ClientSecretRef) != "")
	oauthConfig, err := gitprovider.OAuthConfig(provider, gitOAuthCallbackURL(callbackBaseURL), clientSecret)
	if err != nil {
		debugLog("git.oauth.complete oauth config failed providerId=%s err=%v", provider.ID, err)
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_provider_invalid")
		return
	}
	debugLog("git.oauth.complete oauth config providerId=%s authURL=%s tokenURL=%s redirectURL=%s scopes=%s", provider.ID, oauthConfig.Endpoint.AuthURL, oauthConfig.Endpoint.TokenURL, oauthConfig.RedirectURL, strings.Join(oauthConfig.Scopes, ","))
	egressCtx := h.egressContextForUser(ctx.Request.Context(), stateUser, 15*time.Second)
	debugLog("git.oauth.complete exchanging code providerId=%s tokenURL=%s redirectURL=%s codeLength=%d", provider.ID, oauthConfig.Endpoint.TokenURL, oauthConfig.RedirectURL, len(code))
	token, err := oauthConfig.Exchange(egressCtx, code)
	if err != nil {
		debugLog("git.oauth.complete code exchange failed providerId=%s tokenURL=%s redirectURL=%s err=%v", provider.ID, oauthConfig.Endpoint.TokenURL, oauthConfig.RedirectURL, err)
		h.audit(oauthState.UserID, "git.oauth.complete", provider.ID, false, "code exchange failed")
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_code_invalid")
		return
	}
	debugLog("git.oauth.complete code exchange succeeded providerId=%s accessTokenSet=%t refreshTokenSet=%t expiry=%s tokenType=%s", provider.ID, token.AccessToken != "", token.RefreshToken != "", token.Expiry.Format(time.RFC3339), token.TokenType)

	client := gitprovider.NewClientWithPolicy(provider, token.AccessToken, h.egressPolicyForUser(stateUser))
	debugLog("git.oauth.complete loading git user providerId=%s type=%s baseUrl=%s", provider.ID, provider.Type, provider.BaseURL)
	gitUser, err := client.CurrentUser(egressCtx)
	if err != nil {
		debugLog("git.oauth.complete git user failed providerId=%s err=%v", provider.ID, err)
		h.audit(oauthState.UserID, "git.oauth.complete", provider.ID, false, "git user failed")
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_user_failed")
		return
	}
	debugLog("git.oauth.complete git user loaded providerId=%s externalUserId=%s username=%s", provider.ID, gitUser.ExternalID(), gitUser.Username())
	account, err := h.upsertGitAccountFromOAuth(oauthState.UserID, provider, gitUser, token)
	if err != nil {
		debugLog("git.oauth.complete account save failed providerId=%s userId=%s externalUserId=%s err=%v", provider.ID, oauthState.UserID, gitUser.ID, err)
		h.audit(oauthState.UserID, "git.oauth.complete", provider.ID, false, "save failed")
		ctx.Redirect(http.StatusFound, "/login?error=git_oauth_save_failed")
		return
	}
	debugLog("git.oauth.complete account saved accountId=%s providerId=%s userId=%s username=%s status=%s", account.ID, provider.ID, account.UserID, account.Username, account.Status)
	h.audit(stateUser.ID, "git.oauth.complete", provider.ID, true, account.ID)

	redirectTarget := buildFrontendRedirect(baseURL, oauthState.FrontendOrigin, oauthState.RedirectPath, account.ID)
	debugLog("git.oauth.complete redirecting accountId=%s target=%s", account.ID, redirectTarget)
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
	ownerRef := strings.TrimSpace(input.OwnerRef)
	providerType := normalizeGitProviderType(input.Type)
	if providerType == "github" {
		scope = "global"
		ownerRef = ""
		if err := h.requireSingleGitHubProvider(ctx, ""); err != nil {
			return
		}
	} else if !h.normalizeAndSetGitScopeOwner(ctx, user, scope, &ownerRef, nil) {
		return
	}

	provider := model.GitProvider{
		ID:       id.New("gitp"),
		Type:     providerType,
		Name:     strings.TrimSpace(input.Name),
		BaseURL:  normalizeGitBaseURL(providerType, input.BaseURL),
		Scope:    scope,
		OwnerRef: ownerRef,
		AuthType: normalizeGitAuthType(input.AuthType),
		ClientID: strings.TrimSpace(input.ClientID),
		Enabled:  input.Enabled,
	}
	if strings.TrimSpace(input.ClientSecret) != "" {
		provider.ClientSecretRef = h.secrets.Store(input.ClientSecret, user.ID, "git_provider:"+provider.ID)
	}
	if provider.Name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入 Git Provider 名称")
		return
	}
	if provider.BaseURL == "" {
		provider.BaseURL = defaultGitBaseURL(provider.Type)
	}

	if err := h.db.Create(&provider).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "git_provider.create", provider.ID, true, provider.Type)
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
	if err := h.db.First(&provider, "id = ?", ctx.Param("providerId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "git provider not found")
		return
	}

	var input gitProviderInput
	if !bindJSON(ctx, &input) {
		return
	}

	providerType := normalizeGitProviderType(input.Type)
	scope := normalizeGitScope(input.Scope)
	ownerRef := strings.TrimSpace(input.OwnerRef)
	if providerType == "github" {
		scope = "global"
		ownerRef = ""
		if !h.providerIsSingleFor(ctx, provider.ID, providerType) {
			writeError(ctx, http.StatusBadRequest, "GitHub Provider 仅支持单个实例配置")
			return
		}
	} else if !h.normalizeAndSetGitScopeOwner(ctx, user, scope, &ownerRef, &provider) {
		return
	}

	provider.Type = providerType
	provider.Name = strings.TrimSpace(input.Name)
	provider.BaseURL = normalizeGitBaseURL(providerType, input.BaseURL)
	provider.Scope = scope
	provider.OwnerRef = ownerRef
	provider.AuthType = normalizeGitAuthType(input.AuthType)
	provider.ClientID = strings.TrimSpace(input.ClientID)
	if strings.TrimSpace(input.ClientSecret) != "" {
		provider.ClientSecretRef = h.secrets.Store(input.ClientSecret, user.ID, "git_provider:"+provider.ID)
	}
	provider.Enabled = input.Enabled
	if provider.Name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入 Git Provider 名称")
		return
	}
	if err := h.db.Save(&provider).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "git_provider.update", provider.ID, true, provider.Type)
	ctx.JSON(http.StatusOK, gitProviderResponse(provider))
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
	if err := h.db.First(&provider, "id = ?", ctx.Param("providerId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "git provider not found")
		return
	}
	if err := h.db.Delete(&provider).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "git_provider.delete", provider.ID, true, provider.Type)
	ctx.Status(http.StatusNoContent)
}
