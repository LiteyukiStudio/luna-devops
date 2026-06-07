package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) SearchRegistryRepositories(ctx *gin.Context) {
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
	cacheKey := registrySearchCacheKey("repositories", user.ID, registry.ID, search, ctx.Query("page"), ctx.Query("pageSize"))
	if cached, ok := h.registrySearchCache.get(cacheKey); ok {
		ctx.JSON(http.StatusOK, cached)
		return
	}
	credential := h.registryCredentialInput(user, registry)
	result, err := registryprovider.SearchRepositories(ctx.Request.Context(), registry.Provider, registry.Endpoint, "", search, page, pageSize, h.egressPolicyForUser(user), credential)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "镜像站上游接口调用失败，请检查凭据权限或稍后重试")
		return
	}
	response := gin.H{"items": result.Items, "page": page, "pageSize": pageSize, "total": result.Total, "limited": result.Limited}
	h.registrySearchCache.set(cacheKey, response)
	ctx.JSON(http.StatusOK, response)
}

func (h *Handlers) ListRegistryRepositoryTags(ctx *gin.Context) {
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
	cacheKey := registrySearchCacheKey("tags", user.ID, registry.ID, repository, ctx.Query("limit"))
	if cached, ok := h.registrySearchCache.get(cacheKey); ok {
		ctx.JSON(http.StatusOK, cached)
		return
	}
	credential := h.registryCredentialInput(user, registry)
	result, err := registryprovider.ListTags(ctx.Request.Context(), registry.Provider, registry.Endpoint, repository, limit, h.egressPolicyForUser(user), credential)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "镜像站上游接口调用失败，请检查凭据权限或稍后重试")
		return
	}
	response := gin.H{"items": result.Items, "total": result.Total, "limited": result.Limited}
	h.registrySearchCache.set(cacheKey, response)
	ctx.JSON(http.StatusOK, response)
}

func (h *Handlers) registryCredentialInput(user model.User, registry model.ArtifactRegistry) registryprovider.Credential {
	credentialInput := registryprovider.Credential{}
	if credential, ok := h.registryCredentialFor(user, registry); ok {
		credentialInput.Secret = h.secrets.Resolve(credential.TokenRef)
		if credentialInput.Secret == "" {
			credentialInput.Secret = h.secrets.Resolve(credential.PasswordRef)
		}
		credentialInput.Username = credential.Username
	}
	return credentialInput
}

func (h *Handlers) allowRegistrySearch(ctx *gin.Context, userID string) bool {
	if h.rateLimiter.allow("registry_search:"+userID, 60, time.Minute) {
		return true
	}
	writeError(ctx, http.StatusTooManyRequests, "镜像搜索请求过于频繁，请稍后再试")
	return false
}
