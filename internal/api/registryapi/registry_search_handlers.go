package registryapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/gin-gonic/gin"
)

func (h *Handler) SearchRegistryRepositories(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}
	if !h.allowRegistrySearch(ctx, user.ID) {
		return
	}
	page := positiveInt(ctx.DefaultQuery("page", "1"), 1)
	pageSize := positiveInt(ctx.DefaultQuery("pageSize", "10"), 10)
	if pageSize > 20 {
		pageSize = 20
	}
	search := strings.TrimSpace(ctx.Query("search"))
	credential := h.registryCredentialInput(ctx.Request.Context(), user, registry)
	result, err := registryprovider.SearchRepositories(ctx.Request.Context(), registry.Provider, registry.Endpoint, "", search, page, pageSize, h.egressPolicyForUser(user, ctx.Request.Context()), credential)
	if err != nil {
		if up, ok := registryprovider.AsUpstreamError(err); ok && (up.StatusCode == http.StatusUnauthorized || up.StatusCode == http.StatusForbidden) {
			writeErrorKey(ctx, http.StatusForbidden, requestLanguage(ctx), "registry.authentication_required")
			return
		}
		writeError(ctx, http.StatusBadGateway, "镜像站上游接口调用失败，请检查凭据权限或稍后重试")
		return
	}
	response := gin.H{"items": result.Items, "page": page, "pageSize": pageSize, "total": result.Total, "limited": result.Limited}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) ListRegistryRepositoryTags(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}
	if !h.allowRegistrySearch(ctx, user.ID) {
		return
	}
	repository := strings.Trim(ctx.Query("repository"), "/")
	if repository == "" {
		writeError(ctx, http.StatusBadRequest, "repository is required")
		return
	}
	limit := positiveInt(ctx.DefaultQuery("limit", "20"), 20)
	if limit > 50 {
		limit = 50
	}
	credential := h.registryCredentialInput(ctx.Request.Context(), user, registry)
	result, err := registryprovider.ListTags(ctx.Request.Context(), registry.Provider, registry.Endpoint, repository, limit, h.egressPolicyForUser(user, ctx.Request.Context()), credential)
	if err != nil {
		if up, ok := registryprovider.AsUpstreamError(err); ok && (up.StatusCode == http.StatusUnauthorized || up.StatusCode == http.StatusForbidden) {
			writeErrorKey(ctx, http.StatusForbidden, requestLanguage(ctx), "registry.authentication_required")
			return
		}
		writeError(ctx, http.StatusBadGateway, "镜像站上游接口调用失败，请检查凭据权限或稍后重试")
		return
	}
	response := gin.H{"items": result.Items, "total": result.Total, "limited": result.Limited}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) registryCredentialInput(ctx context.Context, user model.User, registry model.ArtifactRegistry) registryprovider.Credential {
	credentialInput := registryprovider.Credential{}
	if credential, ok := h.registryCredentialFor(user, registry, ctx); ok {
		credentialInput.Secret = h.resolveSecret(ctx, credential.TokenRef)
		if credentialInput.Secret == "" {
			credentialInput.Secret = h.resolveSecret(ctx, credential.PasswordRef)
		}
		credentialInput.Username = credential.Username
	}
	return credentialInput
}

func (h *Handler) allowRegistrySearch(ctx *gin.Context, userID string) bool {
	return h.host.AllowRegistrySearch(ctx, userID)
}
