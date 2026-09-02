package gitapi

import (
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/gin-gonic/gin"
)

type GitWebhookPushPayload = gitWebhookPushPayload

type GitBranchFilterResult struct {
	Items        []gitprovider.Branch
	MatchedTotal int
}

func (h *Handler) ExternalBaseURL(ctx *gin.Context) string { return h.externalBaseURL(ctx) }

func (h *Handler) CanUseGitAccount(ctx *gin.Context, user model.User, account model.GitAccount) bool {
	return h.canUseGitAccount(ctx, user, account)
}

func GitAccountNeedsRefresh(account model.GitAccount) bool { return gitAccountNeedsRefresh(account) }

func FilterGitBranches(branches []gitprovider.Branch, search string, limit int) GitBranchFilterResult {
	result := filterGitBranches(branches, search, limit)
	return GitBranchFilterResult{Items: result.items, MatchedTotal: result.matchedTotal}
}

func VerifyGitWebhookSignature(header http.Header, body []byte, secret string) bool {
	return verifyGitWebhookSignature(header, body, secret)
}

func HMACSHA256Hex(body []byte, secret string) string { return hmacSHA256Hex(body, secret) }

func GitWebhookCommitSHA(body []byte) string { return gitWebhookCommitSHA(body) }

func ParseGitWebhookPushPayload(header http.Header, body []byte) (GitWebhookPushPayload, bool) {
	return parseGitWebhookPushPayload(header, body)
}

func GitUpstreamErrorStatusAndCode(err error) (int, string) {
	return gitUpstreamErrorStatusAndCode(err)
}

func BuildFrontendRedirect(defaultOrigin, frontendOrigin, path, accountID string) string {
	return buildFrontendRedirect(defaultOrigin, frontendOrigin, path, accountID)
}

func GitOAuthCallbackURL(origin string) string { return gitOAuthCallbackURL(origin) }

func SanitizeFrontendOrigin(raw, defaultOrigin string) string {
	return sanitizeFrontendOrigin(raw, defaultOrigin)
}

func GitRepositoryPagination(ctx *gin.Context) paginationParams { return gitRepositoryPagination(ctx) }

func RemotePageTotal(pagination paginationParams, itemCount int) int64 {
	return remotePageTotal(pagination, itemCount)
}

func NormalizeRepositoryBindingOwner(value string) string {
	return normalizeRepositoryBindingOwner(value)
}

func NormalizeRepositoryBindingRepo(value string) string {
	return normalizeRepositoryBindingRepo(value)
}

func FirstNonEmpty(values ...string) string { return firstNonEmpty(values...) }

func PositiveInt(value string, fallbackValue int) int { return positiveInt(value, fallbackValue) }
