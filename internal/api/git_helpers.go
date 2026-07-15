package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func (h *Handlers) gitOAuthRedirectURL(ctx *gin.Context) string {
	return h.externalBaseURL(ctx) + "/api/v1/git/oauth/callback"
}

func (h *Handlers) gitWebhookURL(ctx *gin.Context, bindingID string) string {
	return h.externalBaseURL(ctx) + "/api/v1/git/webhooks/" + url.PathEscape(bindingID)
}

func (h *Handlers) externalBaseURL(_ *gin.Context) string {
	if value := strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"); value != "" {
		return value
	}
	return ""
}

func verifyGitWebhookSignature(header http.Header, body []byte, secret string) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return false
	}
	expected := hmacSHA256Hex(body, secret)
	for _, value := range []string{
		header.Get("X-Hub-Signature-256"),
		header.Get("X-Gitea-Signature"),
		header.Get("X-Gogs-Signature"),
	} {
		value = strings.TrimSpace(strings.TrimPrefix(value, "sha256="))
		if value != "" && hmac.Equal([]byte(value), []byte(expected)) {
			return true
		}
	}
	return false
}

func hmacSHA256Hex(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func gitWebhookEvent(header http.Header) string {
	for _, key := range []string{"X-GitHub-Event", "X-Gitea-Event", "X-Gogs-Event"} {
		if value := strings.TrimSpace(header.Get(key)); value != "" {
			return value
		}
	}
	return "unknown"
}

func gitWebhookCommitSHA(body []byte) string {
	var payload struct {
		After string `json:"after"`
		SHA   string `json:"sha"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if strings.TrimSpace(payload.After) != "" {
		return strings.TrimSpace(payload.After)
	}
	return strings.TrimSpace(payload.SHA)
}

type gitWebhookPushPayload struct {
	Event             string
	CommitSHA         string
	SourceBranch      string
	SourceTag         string
	Deleted           bool
	SourceAuthorName  string
	SourceAuthorEmail string
	TriggeredByName   string
	TriggeredByEmail  string
}

func parseGitWebhookPushPayload(header http.Header, body []byte) (gitWebhookPushPayload, bool) {
	event := gitWebhookEvent(header)
	payload := gitWebhookPushPayload{Event: event}
	if event != "push" {
		return payload, false
	}
	var raw struct {
		Ref     string `json:"ref"`
		After   string `json:"after"`
		SHA     string `json:"sha"`
		Deleted bool   `json:"deleted"`
		Pusher  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"pusher"`
		Sender struct {
			Login string `json:"login"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"sender"`
		HeadCommit struct {
			ID     string `json:"id"`
			Author struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"author"`
			Committer struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"committer"`
		} `json:"head_commit"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return payload, false
	}
	payload.CommitSHA = firstNonEmpty(raw.After, raw.SHA, raw.HeadCommit.ID)
	payload.Deleted = raw.Deleted || isZeroGitSHA(payload.CommitSHA)
	if branch, ok := strings.CutPrefix(strings.TrimSpace(raw.Ref), "refs/heads/"); ok {
		payload.SourceBranch = strings.TrimSpace(branch)
	}
	if tag, ok := strings.CutPrefix(strings.TrimSpace(raw.Ref), "refs/tags/"); ok {
		payload.SourceTag = strings.TrimSpace(tag)
	}
	payload.SourceAuthorName = firstNonEmpty(raw.HeadCommit.Author.Name, raw.HeadCommit.Committer.Name)
	payload.SourceAuthorEmail = firstNonEmpty(raw.HeadCommit.Author.Email, raw.HeadCommit.Committer.Email)
	payload.TriggeredByName = firstNonEmpty(raw.Pusher.Name, raw.Sender.Name, raw.Sender.Login)
	payload.TriggeredByEmail = firstNonEmpty(raw.Pusher.Email, raw.Sender.Email)
	return payload, payload.SourceBranch != "" || payload.SourceTag != ""
}

func isZeroGitSHA(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, char := range value {
		if char != '0' {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func positiveInt(value string, fallbackValue int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallbackValue
	}
	return parsed
}

func (h *Handlers) findEnabledGitProvider(ctx *gin.Context, providerID string) (model.GitProvider, bool) {
	var provider model.GitProvider
	if err := h.db.First(&provider, "id = ? and enabled = ?", strings.TrimSpace(providerID), true).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "git provider not found")
		return provider, false
	}
	if !h.canUseGitProvider(ctx, provider) {
		return provider, false
	}
	return provider, true
}

func (h *Handlers) findGitAccountForUser(ctx *gin.Context, userID, accountID string) (model.GitAccount, bool) {
	var account model.GitAccount
	user, ok := h.currentUser(ctx)
	if !ok {
		return account, false
	}
	if user.ID != userID {
		writeError(ctx, http.StatusForbidden, "无权访问该 Git 账号")
		return account, false
	}
	if err := h.db.First(&account, "id = ?", strings.TrimSpace(accountID)).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "git account not found")
		return account, false
	}
	if !h.canUseGitAccount(ctx, user, account) {
		return account, false
	}
	provider, ok := h.findEnabledGitProvider(ctx, account.ProviderID)
	if !ok {
		return account, false
	}
	if !h.canUseGitProvider(ctx, provider) {
		return account, false
	}

	if account.Status != "connected" {
		writeError(ctx, http.StatusBadRequest, "Git 账号未连接")
		return account, false
	}
	return account, true
}

func (h *Handlers) canUseGitProvider(ctx *gin.Context, provider model.GitProvider) bool {
	user, ok := h.currentUser(ctx)
	if !ok {
		return false
	}
	if h.canUseScopedResourceByID(user, provider.Scope, provider.OwnerRef, scopedResourceGitProvider, provider.ID) {
		return true
	}
	writeError(ctx, http.StatusForbidden, "无权访问该 Git Provider")
	return false
}

func (h *Handlers) canManageGitProvider(ctx *gin.Context, user model.User, provider model.GitProvider) bool {
	return h.canManageScopedResourceByID(ctx, user, provider.Scope, provider.OwnerRef, scopedResourceGitProvider, provider.ID, "无权维护该 Git Provider")
}

func (h *Handlers) canUseGitAccount(ctx *gin.Context, user model.User, account model.GitAccount) bool {
	if h.canUseScopedResourceByID(user, account.Scope, account.OwnerRef, scopedResourceGitAccount, account.ID) {
		return true
	}
	writeError(ctx, http.StatusForbidden, "无权访问该 Git 凭据")
	return false
}

func (h *Handlers) canManageGitAccount(ctx *gin.Context, user model.User, account model.GitAccount) bool {
	return h.canManageScopedResourceByID(ctx, user, account.Scope, account.OwnerRef, scopedResourceGitAccount, account.ID, "无权维护该 Git 凭据")
}

func (h *Handlers) findApplicationByID(ctx *gin.Context, applicationID string) (model.Application, bool) {
	var app model.Application
	err := h.db.First(&app, "id = ? and project_id = ?", strings.TrimSpace(applicationID), ctx.Param("projectId")).Error
	if err != nil {
		writeError(ctx, http.StatusNotFound, "application not found")
		return app, false
	}
	return app, true
}

func (h *Handlers) syncApplicationRepositoryURL(binding model.RepositoryBinding) {
	_ = binding
}

func writeGitUpstreamError(ctx *gin.Context, err error) {
	if err != nil {
		fmt.Printf("git upstream error: %s\n", gitUpstreamLogMessage(err))
	}
	status, code := gitUpstreamErrorStatusAndCode(err)
	writeErrorKey(ctx, status, requestLanguage(ctx), code)
}

func gitUpstreamErrorStatusAndCode(err error) (int, string) {
	upstreamErr, ok := gitprovider.AsUpstreamError(err)
	if !ok {
		if isGitNetworkError(err) {
			return http.StatusBadGateway, "git.network_failed"
		}
		return http.StatusBadGateway, "git.upstream_failed"
	}
	if isWebhookCallbackUnreachable(upstreamErr) {
		return http.StatusBadRequest, "git.webhook_callback_unreachable"
	}
	if isWebhookCallbackInvalid(upstreamErr) {
		return http.StatusBadRequest, "git.webhook_callback_invalid"
	}
	if upstreamContains(upstreamErr, "hook already exists", "hook exists") {
		return http.StatusConflict, "git.webhook_already_exists"
	}
	if upstreamContains(upstreamErr, "endpoint has been spammed", "abuse", "rate limit") {
		return http.StatusTooManyRequests, "git.webhook_rate_limited"
	}
	switch upstreamErr.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return http.StatusForbidden, "git.permission_denied"
	case http.StatusNotFound:
		return http.StatusNotFound, "git.repository_not_found"
	case http.StatusUnprocessableEntity:
		return http.StatusBadRequest, "git.validation_failed"
	default:
		return http.StatusBadGateway, "git.upstream_failed"
	}
}

func isGitNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such host") ||
		strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "network is unreachable") ||
		strings.Contains(message, "context deadline exceeded")
}

func isWebhookCallbackUnreachable(err *gitprovider.UpstreamError) bool {
	if !upstreamHasHookURLDetail(err) {
		return false
	}
	return upstreamContains(err, "localhost", "127.0.0.1", "::1", "public internet", "not reachable", "host is not supported")
}

func isWebhookCallbackInvalid(err *gitprovider.UpstreamError) bool {
	if !upstreamHasHookURLDetail(err) {
		return false
	}
	return upstreamContains(err, "invalid", "protocol", "http", "https", "url")
}

func upstreamHasHookURLDetail(err *gitprovider.UpstreamError) bool {
	for _, detail := range err.Details {
		if strings.EqualFold(strings.TrimSpace(detail.Resource), "Hook") && strings.EqualFold(strings.TrimSpace(detail.Field), "url") {
			return true
		}
	}
	return false
}

func upstreamContains(err *gitprovider.UpstreamError, needles ...string) bool {
	haystackParts := []string{err.Message}
	for _, detail := range err.Details {
		haystackParts = append(haystackParts, detail.Resource, detail.Code, detail.Field, detail.Message)
	}
	haystack := strings.ToLower(strings.Join(haystackParts, " "))
	for _, needle := range needles {
		if strings.Contains(haystack, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func gitUpstreamLogMessage(err error) string {
	upstreamErr, ok := gitprovider.AsUpstreamError(err)
	if !ok {
		if err == nil {
			return "unknown"
		}
		return fmt.Sprintf("%T", err)
	}
	parts := []string{fmt.Sprintf("status=%d", upstreamErr.StatusCode)}
	if strings.TrimSpace(upstreamErr.Message) != "" {
		parts = append(parts, "message="+strings.TrimSpace(upstreamErr.Message))
	}
	for _, detail := range upstreamErr.Details {
		fields := []string{}
		if strings.TrimSpace(detail.Resource) != "" {
			fields = append(fields, "resource="+strings.TrimSpace(detail.Resource))
		}
		if strings.TrimSpace(detail.Field) != "" {
			fields = append(fields, "field="+strings.TrimSpace(detail.Field))
		}
		if strings.TrimSpace(detail.Code) != "" {
			fields = append(fields, "code="+strings.TrimSpace(detail.Code))
		}
		if strings.TrimSpace(detail.Message) != "" {
			fields = append(fields, "message="+strings.TrimSpace(detail.Message))
		}
		if len(fields) > 0 {
			parts = append(parts, strings.Join(fields, " "))
		}
	}
	return strings.Join(parts, "; ")
}

func normalizeGitProviderType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "gitea", "gitlab":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "github"
	}
}

func normalizeGitAuthType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "github-app", "pat":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "oauth"
	}
}

func normalizeGitScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "project":
		return "project"
	case "user":
		return "user"
	case "global":
		return "global"
	default:
		return "user"
	}
}

func (h *Handlers) providerIsSingleFor(ctx *gin.Context, providerID, providerType string) bool {
	if providerType != "github" {
		return true
	}
	query := h.db.Model(&model.GitProvider{}).Where("type = ?", providerType)
	if providerID != "" {
		query = query.Where("id <> ?", providerID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if count == 0 {
		return true
	}
	return false
}

func (h *Handlers) requireSingleGitHubProvider(ctx *gin.Context, providerID string) error {
	if h.providerIsSingleFor(ctx, providerID, "github") {
		return nil
	}
	writeError(ctx, http.StatusBadRequest, "GitHub Provider 仅支持一个实例")
	return fmt.Errorf("github provider already exists")
}

func normalizeGitAccountStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "expired", "revoked":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "connected"
	}
}

func gitProviderResponses(providers []model.GitProvider) []gin.H {
	responses := make([]gin.H, 0, len(providers))
	for _, provider := range providers {
		responses = append(responses, gitProviderResponse(provider))
	}
	return responses
}

func (h *Handlers) gitProviderResponsesForUser(user model.User, providers []model.GitProvider) []gin.H {
	responses := make([]gin.H, 0, len(providers))
	for _, provider := range providers {
		provider.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceGitProvider, provider.ID)
		response := gitProviderResponse(provider)
		if !h.canInspectScopedResourceConfigByID(user, provider.Scope, provider.OwnerRef, scopedResourceGitProvider, provider.ID) {
			response["baseUrl"] = ""
			response["clientId"] = ""
		}
		responses = append(responses, response)
	}
	return responses
}

func gitProviderResponse(provider model.GitProvider) gin.H {
	return gin.H{
		"id":              provider.ID,
		"type":            provider.Type,
		"name":            provider.Name,
		"baseUrl":         provider.BaseURL,
		"scope":           provider.Scope,
		"ownerRef":        provider.OwnerRef,
		"projectIds":      jsonList(provider.ProjectIDs),
		"authType":        provider.AuthType,
		"clientId":        provider.ClientID,
		"clientSecretSet": secret.HasValue(provider.ClientSecretRef),
		"enabled":         provider.Enabled,
		"createdAt":       provider.CreatedAt,
	}
}

func gitAccountResponses(accounts []model.GitAccount) []gin.H {
	responses := make([]gin.H, 0, len(accounts))
	for _, account := range accounts {
		responses = append(responses, gitAccountResponse(account))
	}
	return responses
}

func gitAccountResponse(account model.GitAccount) gin.H {
	return gin.H{
		"id":              account.ID,
		"userId":          account.UserID,
		"scope":           account.Scope,
		"ownerRef":        account.OwnerRef,
		"projectIds":      jsonList(account.ProjectIDs),
		"providerId":      account.ProviderID,
		"externalUserId":  account.ExternalUserID,
		"username":        account.Username,
		"avatarUrl":       account.AvatarURL,
		"scopes":          account.Scopes,
		"accessTokenSet":  secret.HasValue(account.AccessTokenRef),
		"refreshTokenSet": secret.HasValue(account.RefreshTokenRef),
		"expiresAt":       account.ExpiresAt,
		"status":          account.Status,
		"createdAt":       account.CreatedAt,
	}
}

func normalizeWebhookStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "created", "disabled", "failed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "pending"
	}
}

func normalizeGitBaseURL(providerType string, baseURL string) string {
	providerType = normalizeGitProviderType(providerType)
	if providerType == "github" {
		return defaultGitBaseURL("github")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return defaultGitBaseURL(providerType)
	}
	return baseURL
}

func defaultGitBaseURL(providerType string) string {
	switch providerType {
	case "github":
		return "https://github.com"
	default:
		return ""
	}
}

func defaultCloneURL(provider model.GitProvider, owner, repo string) string {
	baseURL := strings.TrimRight(provider.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGitBaseURL(provider.Type)
	}
	if baseURL == "" {
		return ""
	}
	return baseURL + "/" + owner + "/" + repo + ".git"
}

type gitProviderInput struct {
	Type         string   `json:"type"`
	Name         string   `json:"name" binding:"required"`
	BaseURL      string   `json:"baseUrl"`
	Scope        string   `json:"scope"`
	OwnerRef     string   `json:"ownerRef"`
	ProjectIDs   []string `json:"projectIds"`
	AuthType     string   `json:"authType"`
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret"`
	Enabled      bool     `json:"enabled"`
}

type gitAccountInput struct {
	ProviderID     string   `json:"providerId" binding:"required"`
	Scope          string   `json:"scope"`
	OwnerRef       string   `json:"ownerRef"`
	ProjectIDs     []string `json:"projectIds"`
	ExternalUserID string   `json:"externalUserId"`
	Username       string   `json:"username" binding:"required"`
	AvatarURL      string   `json:"avatarUrl"`
	AccessToken    string   `json:"accessToken"`
	RefreshToken   string   `json:"refreshToken"`
	Scopes         []string `json:"scopes"`
	Status         string   `json:"status"`
}

type repositoryBindingInput struct {
	ApplicationID        string `json:"applicationId" binding:"required"`
	GitAccountID         string `json:"gitAccountId" binding:"required"`
	Owner                string `json:"owner" binding:"required"`
	Repo                 string `json:"repo" binding:"required"`
	CloneURL             string `json:"cloneUrl"`
	DefaultBranch        string `json:"defaultBranch"`
	WebhookStatus        string `json:"webhookStatus"`
	AutoConfigureWebhook *bool  `json:"autoConfigureWebhook"`
}

type repositoryBindingResponse struct {
	model.RepositoryBinding
	WebhookCallbackURL string `json:"webhookCallbackUrl"`
	ProviderName       string `json:"providerName"`
	ProviderType       string `json:"providerType"`
	AccountUsername    string `json:"accountUsername"`
	AccountOwnerEmail  string `json:"accountOwnerEmail"`
	AccountOwnerName   string `json:"accountOwnerName"`
	ApplicationName    string `json:"applicationName"`
}
