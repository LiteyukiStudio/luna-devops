package api

import (
	"context"
	"net/http"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/gitapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type gitHost struct {
	handlers *Handlers
}

func (host gitHost) DBFor(ctx *gin.Context) *gorm.DB { return host.handlers.dbFor(ctx) }

func (host gitHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}

func (host gitHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}

func (host gitHost) RequirePlatformAdmin(ctx *gin.Context) bool {
	return host.handlers.requirePlatformAdmin(ctx)
}

func (host gitHost) ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return resolveListVisibility(ctx, user)
}

func (host gitHost) ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return host.handlers.applyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}

func (host gitHost) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool) {
	return host.handlers.normalizeScopedOwnerWithProjects(ctx, user, scope, ownerRef, projectIDs, globalError)
}

func (host gitHost) NormalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, scope string, projectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool) {
	return host.handlers.normalizeCredentialScopeWithinParent(ctx, user, scope, projectIDs, parentScope, parentProjectIDs, globalError)
}

func (host gitHost) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return host.handlers.canManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}

func (host gitHost) CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return host.handlers.canInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (host gitHost) CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return host.handlers.canUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (host gitHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return host.handlers.replaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}

func (host gitHost) ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return host.handlers.scopedResourceProjectIDs(resourceType, resourceID, ctx)
}

func (host gitHost) ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return host.handlers.scopedResourceProjectIDMap(resourceType, resourceIDs, ctx)
}

func (host gitHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProject(ctx, action)
}

func (host gitHost) WriteProjectAuthorizationError(ctx *gin.Context, err error) {
	writeProjectAuthorizationError(ctx, err)
}

func (host gitHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}

func (host gitHost) SecretStore() secret.Store { return host.handlers.secrets }

func (host gitHost) OAuthStateStore() gitapi.OAuthStateStore {
	return gitOAuthStateStoreAdapter{handlers: host.handlers}
}

func (host gitHost) PublicBaseURL() string { return host.handlers.config.PublicBaseURL }

func (host gitHost) DebugLog(format string, args ...any) {
	host.handlers.debugLog(format, args...)
}

func (host gitHost) EgressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy {
	return host.handlers.egressPolicyForUser(user, ctx)
}

func (host gitHost) EgressContextForUser(ctx context.Context, user model.User, timeout time.Duration) context.Context {
	return host.handlers.egressContextForUser(ctx, user, timeout)
}

func (host gitHost) PrepareBuildRunRequest(user model.User, run *model.BuildRun, ctx context.Context) error {
	return host.handlers.prepareBuildRunRequest(user, run, ctx)
}

func (host gitHost) QueueBuildRun(ctx context.Context, user model.User, run model.BuildRun) (model.BuildRun, error) {
	return host.handlers.queueBuildRun(ctx, user, run)
}

func (host gitHost) DeploymentTargetMatchesBuildRun(target model.DeploymentTarget, run model.BuildRun) bool {
	return deploymentTargetMatchesBuildRun(target, run)
}

func (host gitHost) BuildRunActorName(user model.User) string { return buildRunActorName(user) }

type gitOAuthStateStoreAdapter struct {
	handlers *Handlers
}

func (store gitOAuthStateStoreAdapter) SaveGit(ctx context.Context, state string, value gitapi.GitOAuthStateValue, ttl time.Duration) error {
	return store.handlers.oauthStates.SaveGit(ctx, state, gitOAuthStateValue{
		ProviderID:     value.ProviderID,
		UserID:         value.UserID,
		RedirectPath:   value.RedirectPath,
		FrontendOrigin: value.FrontendOrigin,
		CallbackOrigin: value.CallbackOrigin,
	}, ttl)
}

func (store gitOAuthStateStoreAdapter) ConsumeGit(ctx context.Context, state string) (gitapi.GitOAuthStateValue, bool, error) {
	value, ok, err := store.handlers.oauthStates.ConsumeGit(ctx, state)
	return gitapi.GitOAuthStateValue{
		ProviderID:     value.ProviderID,
		UserID:         value.UserID,
		RedirectPath:   value.RedirectPath,
		FrontendOrigin: value.FrontendOrigin,
		CallbackOrigin: value.CallbackOrigin,
	}, ok, err
}

func (h *Handlers) gitAPI() *gitapi.Handler { return gitapi.New(gitHost{handlers: h}) }

func (h *Handlers) ListGitProviders(ctx *gin.Context)  { h.gitAPI().ListGitProviders(ctx) }
func (h *Handlers) CreateGitProvider(ctx *gin.Context) { h.gitAPI().CreateGitProvider(ctx) }
func (h *Handlers) UpdateGitProvider(ctx *gin.Context) { h.gitAPI().UpdateGitProvider(ctx) }
func (h *Handlers) DeleteGitProvider(ctx *gin.Context) { h.gitAPI().DeleteGitProvider(ctx) }
func (h *Handlers) StartGitOAuth(ctx *gin.Context)     { h.gitAPI().StartGitOAuth(ctx) }
func (h *Handlers) CompleteGitOAuth(ctx *gin.Context)  { h.gitAPI().CompleteGitOAuth(ctx) }
func (h *Handlers) ListGitAccounts(ctx *gin.Context)   { h.gitAPI().ListGitAccounts(ctx) }
func (h *Handlers) CreateGitAccount(ctx *gin.Context)  { h.gitAPI().CreateGitAccount(ctx) }
func (h *Handlers) UpdateGitAccount(ctx *gin.Context)  { h.gitAPI().UpdateGitAccount(ctx) }
func (h *Handlers) DeleteGitAccount(ctx *gin.Context)  { h.gitAPI().DeleteGitAccount(ctx) }
func (h *Handlers) RefreshGitAccount(ctx *gin.Context) { h.gitAPI().RefreshGitAccount(ctx) }
func (h *Handlers) ListGitRepositories(ctx *gin.Context) {
	h.gitAPI().ListGitRepositories(ctx)
}
func (h *Handlers) ListGitBranches(ctx *gin.Context) { h.gitAPI().ListGitBranches(ctx) }
func (h *Handlers) ReadGitFile(ctx *gin.Context)     { h.gitAPI().ReadGitFile(ctx) }
func (h *Handlers) ListGitContents(ctx *gin.Context) { h.gitAPI().ListGitContents(ctx) }
func (h *Handlers) GetGitRepositoryBuildOptions(ctx *gin.Context) {
	h.gitAPI().GetGitRepositoryBuildOptions(ctx)
}
func (h *Handlers) ListRepositoryBindings(ctx *gin.Context) {
	h.gitAPI().ListRepositoryBindings(ctx)
}
func (h *Handlers) CreateRepositoryBinding(ctx *gin.Context) {
	h.gitAPI().CreateRepositoryBinding(ctx)
}
func (h *Handlers) UpdateRepositoryBinding(ctx *gin.Context) {
	h.gitAPI().UpdateRepositoryBinding(ctx)
}
func (h *Handlers) DeleteRepositoryBinding(ctx *gin.Context) {
	h.gitAPI().DeleteRepositoryBinding(ctx)
}
func (h *Handlers) CreateRepositoryWebhook(ctx *gin.Context) {
	h.gitAPI().CreateRepositoryWebhook(ctx)
}
func (h *Handlers) ReconfigureRepositoryWebhook(ctx *gin.Context) {
	h.gitAPI().ReconfigureRepositoryWebhook(ctx)
}
func (h *Handlers) ReceiveGitWebhook(ctx *gin.Context) { h.gitAPI().ReceiveGitWebhook(ctx) }

func (h *Handlers) externalBaseURL(ctx *gin.Context) string { return h.gitAPI().ExternalBaseURL(ctx) }
func (h *Handlers) canUseGitAccount(ctx *gin.Context, user model.User, account model.GitAccount) bool {
	return h.gitAPI().CanUseGitAccount(ctx, user, account)
}

type gitWebhookPushPayload = gitapi.GitWebhookPushPayload

func verifyGitWebhookSignature(header http.Header, body []byte, secret string) bool {
	return gitapi.VerifyGitWebhookSignature(header, body, secret)
}
func hmacSHA256Hex(body []byte, secret string) string { return gitapi.HMACSHA256Hex(body, secret) }
func gitWebhookCommitSHA(body []byte) string          { return gitapi.GitWebhookCommitSHA(body) }
func parseGitWebhookPushPayload(header http.Header, body []byte) (gitWebhookPushPayload, bool) {
	return gitapi.ParseGitWebhookPushPayload(header, body)
}
func gitUpstreamErrorStatusAndCode(err error) (int, string) {
	return gitapi.GitUpstreamErrorStatusAndCode(err)
}
func buildFrontendRedirect(defaultOrigin, frontendOrigin, path, accountID string) string {
	return gitapi.BuildFrontendRedirect(defaultOrigin, frontendOrigin, path, accountID)
}
func gitOAuthCallbackURL(origin string) string { return gitapi.GitOAuthCallbackURL(origin) }
func sanitizeFrontendOrigin(raw, defaultOrigin string) string {
	return gitapi.SanitizeFrontendOrigin(raw, defaultOrigin)
}
func gitRepositoryPagination(ctx *gin.Context) paginationParams {
	return gitapi.GitRepositoryPagination(ctx)
}
func remotePageTotal(pagination paginationParams, itemCount int) int64 {
	return gitapi.RemotePageTotal(pagination, itemCount)
}
func normalizeRepositoryBindingOwner(value string) string {
	return gitapi.NormalizeRepositoryBindingOwner(value)
}
func normalizeRepositoryBindingRepo(value string) string {
	return gitapi.NormalizeRepositoryBindingRepo(value)
}
func firstNonEmpty(values ...string) string { return gitapi.FirstNonEmpty(values...) }
func positiveInt(value string, fallbackValue int) int {
	return gitapi.PositiveInt(value, fallbackValue)
}
func gitAccountNeedsRefresh(account model.GitAccount) bool {
	return gitapi.GitAccountNeedsRefresh(account)
}

type gitBranchFilterResult struct {
	items        []gitprovider.Branch
	matchedTotal int
}

func filterGitBranches(branches []gitprovider.Branch, search string, limit int) gitBranchFilterResult {
	result := gitapi.FilterGitBranches(branches, search, limit)
	return gitBranchFilterResult{items: result.Items, matchedTotal: result.MatchedTotal}
}
