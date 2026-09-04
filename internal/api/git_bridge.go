package api

import (
	"context"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"net/http"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/buildapi"
	"github.com/LiteyukiStudio/devops/internal/api/gitapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/gin-gonic/gin"
)

type gitHost struct {
	domainHost
}

func (host gitHost) OAuthStateStore() gitapi.OAuthStateStore {
	return gitOAuthStateStoreAdapter{handlers: host.handlers}
}

func (host gitHost) DebugLog(format string, args ...any) {
	host.handlers.debugLog(format, args...)
}

func (host gitHost) EgressContextForUser(ctx context.Context, user model.User, timeout time.Duration) context.Context {
	return host.handlers.egressContextForUser(ctx, user, timeout)
}

func (host gitHost) PrepareBuildRunRequest(user model.User, run *model.BuildRun, ctx context.Context) error {
	return host.handlers.domains.build.PrepareBuildRunRequest(user, run, ctx)
}

func (host gitHost) QueueBuildRun(ctx context.Context, user model.User, run model.BuildRun) (model.BuildRun, error) {
	return host.handlers.domains.build.QueueBuildRun(ctx, user, run)
}

func (host gitHost) DeploymentTargetMatchesBuildRun(target model.DeploymentTarget, run model.BuildRun) bool {
	return deploymentTargetMatchesBuildRun(target, run)
}

func (host gitHost) BuildRunActorName(user model.User) string {
	return buildapi.BuildRunActorName(user)
}

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

func (h *Handlers) externalBaseURL(ctx *gin.Context) string {
	return h.domains.git.ExternalBaseURL(ctx)
}
func (h *Handlers) canUseGitAccount(ctx *gin.Context, user model.User, account model.GitAccount) bool {
	return h.domains.git.CanUseGitAccount(ctx, user, account)
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
func gitRepositoryPagination(ctx *gin.Context) transportapi.PaginationParams {
	return gitapi.GitRepositoryPagination(ctx)
}
func remotePageTotal(pagination transportapi.PaginationParams, itemCount int) int64 {
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
