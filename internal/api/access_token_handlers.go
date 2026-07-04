package api

import (
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func (h *Handlers) ListAccessTokens(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	pagination := paginationFromQuery(ctx)
	var tokens []model.AccessToken
	query := h.db.Model(&model.AccessToken{}).Where("user_id = ? and revoked_at is null", user.ID)
	query = applySearch(ctx, query, "name", "scope")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"createdAt": "created_at",
		"expiresAt": "expires_at",
		"name":      "name",
		"scope":     "scope",
		"status":    "revoked_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&tokens).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(tokens, total, pagination))
}

func (h *Handlers) ListAccessTokenScopes(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"items": service.AccessTokenScopeCatalog(user.Role),
	})
}

func (h *Handlers) CreateAccessToken(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input accessTokenInput
	if !bindJSON(ctx, &input) {
		return
	}

	scope := normalizeAccessTokenScope(input.Scope)
	if scope == "" {
		writeError(ctx, http.StatusBadRequest, "Access Token scope 不受支持")
		return
	}
	if !userCanCreateAccessTokenScope(user, scope) {
		writeError(ctx, http.StatusForbidden, "无权创建该 Access Token scope")
		return
	}

	plainToken := "lyd_" + randomHex(24)
	token := model.AccessToken{
		ID:        id.New("tok"),
		UserID:    user.ID,
		Name:      input.Name,
		Scope:     scope,
		TokenHash: hashToken(plainToken),
	}

	if input.ExpiresInDays > 0 {
		expiresAt := time.Now().Add(time.Duration(input.ExpiresInDays) * 24 * time.Hour)
		token.ExpiresAt = &expiresAt
	}

	if err := h.db.Create(&token).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "access_token.create", token.ID, true, scope)

	ctx.JSON(http.StatusCreated, gin.H{
		"token":       token,
		"accessToken": plainToken,
	})
}

func (h *Handlers) RevokeAccessToken(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var token model.AccessToken
	if err := h.db.First(&token, "id = ? and user_id = ?", ctx.Param("tokenId"), user.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "token not found")
		return
	}
	now := time.Now()
	token.RevokedAt = &now
	if err := h.db.Save(&token).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "access_token.revoke", token.ID, true, "")
	ctx.JSON(http.StatusOK, token)
}

type accessTokenInput struct {
	Name          string `json:"name" binding:"required"`
	Scope         string `json:"scope"`
	ExpiresInDays int    `json:"expiresInDays"`
}

func normalizeAccessTokenScope(scopeText string) string {
	return service.NormalizeAccessTokenScope(scopeText)
}

func userCanCreateAccessTokenScope(user model.User, scopeText string) bool {
	return service.UserCanCreateAccessTokenScope(user.Role, scopeText)
}
