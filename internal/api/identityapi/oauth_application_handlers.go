package identityapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListOAuthApplications(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.dbFor(ctx).Model(&model.OAuthApplication{}).Where("owner_user_id = ? and revoked_at is null", user.ID)
	query = applySearch(ctx, query, "name", "description", "client_id")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var applications []model.OAuthApplication
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name": "name", "createdAt": "created_at", "updatedAt": "updated_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&applications).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]oauthApplicationResponse, 0, len(applications))
	for _, application := range applications {
		items = append(items, oauthApplicationToResponse(application))
	}
	ctx.JSON(http.StatusOK, paginatedResponse(items, total, pagination))
}

func (h *Handlers) CreateOAuthApplication(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	var input oauthApplicationInput
	if !bindJSON(ctx, &input) {
		return
	}
	application, valid := oauthApplicationFromInput(ctx, user, input)
	if !valid {
		return
	}
	plainSecret := "lyo_secret_" + randomHex(32)
	application.ID = id.New("oapp")
	application.ClientID = "lyo_app_" + randomHex(16)
	application.ClientSecretHash = hashToken(plainSecret)
	if err := h.dbFor(ctx).Create(&application).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "oauth_application.create", application.ID, true, application.AllowedScopes, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, oauthApplicationSecretResponse{
		Application:  oauthApplicationToResponse(application),
		ClientSecret: plainSecret,
	})
}

func (h *Handlers) UpdateOAuthApplication(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	var input oauthApplicationInput
	if !bindJSON(ctx, &input) {
		return
	}
	next, valid := oauthApplicationFromInput(ctx, user, input)
	if !valid {
		return
	}
	existing, err := updateOwnedOAuthApplication(h.dbFor(ctx), ctx.Param("applicationId"), user.ID, next)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "OAuth application not found")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "oauth_application.update", existing.ID, true, existing.AllowedScopes, ctx.Request.Context())
	ctx.JSON(http.StatusOK, oauthApplicationToResponse(existing))
}

func (h *Handlers) RotateOAuthApplicationSecret(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	plainSecret := "lyo_secret_" + randomHex(32)
	application, err := rotateOwnedOAuthApplicationSecret(h.dbFor(ctx), ctx.Param("applicationId"), user.ID, hashToken(plainSecret))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "OAuth application not found")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "oauth_application.rotate_secret", application.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, oauthApplicationSecretResponse{
		Application:  oauthApplicationToResponse(application),
		ClientSecret: plainSecret,
	})
}

func (h *Handlers) DeleteOAuthApplication(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	application, err := deleteOwnedOAuthApplication(h.dbFor(ctx), ctx.Param("applicationId"), user.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "OAuth application not found")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "oauth_application.revoke", application.ID, true, "", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListMyOAuthGrants(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.dbFor(ctx).Model(&model.OAuthGrant{}).Where("user_id = ? and revoked_at is null", user.ID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var grants []model.OAuthGrant
	if err := query.Order(orderByClause(pagination, map[string]string{
		"createdAt": "created_at", "updatedAt": "updated_at",
	}, "updated_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&grants).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]oauthGrantResponse, 0, len(grants))
	for _, grant := range grants {
		var application model.OAuthApplication
		if err := h.dbFor(ctx).First(&application, "id = ?", grant.ApplicationID).Error; err != nil {
			continue
		}
		scope := grant.Scope
		if isLunaCLIApplication(application) {
			scope = ""
		}
		items = append(items, oauthGrantResponse{
			ID: grant.ID, Application: oauthApplicationToResponse(application), Scope: scope,
			CreatedAt: grant.CreatedAt, UpdatedAt: grant.UpdatedAt,
		})
	}
	ctx.JSON(http.StatusOK, paginatedResponse(items, total, pagination))
}

func (h *Handlers) RevokeMyOAuthGrant(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	var grant model.OAuthGrant
	if err := h.dbFor(ctx).First(&grant, "id = ? and user_id = ? and revoked_at is null", ctx.Param("grantId"), user.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "OAuth authorization not found")
		return
	}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error { return revokeOAuthGrant(tx, grant.ID, time.Now()) }); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "oauth_grant.revoke", grant.ID, true, "", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func oauthApplicationFromInput(ctx *gin.Context, user model.User, input oauthApplicationInput) (model.OAuthApplication, bool) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 100 || len(input.Description) > 1000 {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.application.invalid", "OAuth application name or description is invalid")
		return model.OAuthApplication{}, false
	}
	redirectURIs, ok := normalizeOAuthRedirectURIs(input.RedirectURIs)
	if !ok {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.redirect_uri.invalid", "At least one valid HTTPS or localhost redirect URI is required")
		return model.OAuthApplication{}, false
	}
	if !validOptionalHTTPURL(input.HomepageURL) || !validOptionalHTTPURL(input.LogoURL) {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.application.invalid_url", "Homepage and logo URLs must use HTTP or HTTPS")
		return model.OAuthApplication{}, false
	}
	scope := normalizeOAuthScope(input.AllowedScopes)
	if scope == "" || !userCanAuthorizeOAuthScope(user, scope) {
		writeErrorCode(ctx, http.StatusForbidden, "oauth.scope.forbidden", "OAuth application scope is not allowed")
		return model.OAuthApplication{}, false
	}
	if !allowedOAuthAccessTokenLifetimeDays[input.AccessTokenLifetimeDays] {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.token_lifetime.invalid", "Unsupported access token lifetime")
		return model.OAuthApplication{}, false
	}
	return model.OAuthApplication{
		OwnerUserID: &user.ID,
		Name:        name, Description: strings.TrimSpace(input.Description), HomepageURL: strings.TrimSpace(input.HomepageURL),
		LogoURL: strings.TrimSpace(input.LogoURL), RedirectURIs: encodeStringList(redirectURIs), AllowedScopes: scope,
		AccessTokenLifetimeDays: input.AccessTokenLifetimeDays,
	}, true
}
