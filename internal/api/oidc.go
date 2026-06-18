package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const oidcStateTTL = 10 * time.Minute

func (h *Handlers) StartOIDC(ctx *gin.Context) {
	provider, ok := h.enabledAuthProvider(ctx.Param("providerId"))
	if !ok {
		writeError(ctx, http.StatusNotFound, "OIDC provider not found")
		return
	}

	if strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")) == "" {
		writeError(ctx, http.StatusInternalServerError, "PUBLIC_BASE_URL is required")
		return
	}

	mode := ctx.DefaultQuery("mode", "login")
	state := "oidc_" + randomHex(32)
	nonce := randomHex(24)
	redirectPath := sanitizeRedirectPath(ctx.DefaultQuery("redirect", "/projects"))
	userID := ""

	if mode == "bind" {
		user, ok := h.currentUser(ctx)
		if !ok {
			return
		}
		userID = user.ID
	} else {
		mode = "login"
	}

	egressCtx := h.adminConfiguredEgressContext(ctx.Request.Context(), 15*time.Second)
	oidcProvider, err := oidc.NewProvider(egressCtx, provider.IssuerURL)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "OIDC provider discovery failed")
		return
	}

	authState := oidcAuthStateValue{
		Nonce:        nonce,
		ProviderID:   provider.ID,
		UserID:       userID,
		Mode:         mode,
		RedirectPath: redirectPath,
	}
	if err := h.oauthStates.SaveOIDC(ctx.Request.Context(), state, authState, oidcStateTTL); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	oauthConfig := h.oauth2Config(provider, oidcProvider)
	ctx.Redirect(http.StatusFound, oauthConfig.AuthCodeURL(state, oidc.Nonce(nonce)))
}

func (h *Handlers) CompleteOIDC(ctx *gin.Context) {
	plainState := ctx.Query("state")
	code := ctx.Query("code")
	if plainState == "" || code == "" {
		h.redirectAuthError(ctx, "oidc_callback_invalid")
		return
	}

	if strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")) == "" {
		h.redirectAuthError(ctx, "oidc_callback_invalid")
		return
	}

	authState, ok, err := h.oauthStates.ConsumeOIDC(ctx.Request.Context(), plainState)
	if err != nil {
		h.audit("", "oidc.callback", "oidc_state", false, "state missing or expired")
		h.redirectAuthError(ctx, "oidc_state_invalid")
		return
	}
	if !ok {
		h.audit("", "oidc.callback", "oidc_state", false, "state missing or expired")
		h.redirectAuthError(ctx, "oidc_state_invalid")
		return
	}

	provider, ok := h.enabledAuthProvider(authState.ProviderID)
	if !ok {
		h.audit(authState.UserID, "oidc.callback", authState.ProviderID, false, "provider disabled")
		h.redirectAuthError(ctx, "oidc_provider_disabled")
		return
	}

	egressCtx := h.adminConfiguredEgressContext(ctx.Request.Context(), 15*time.Second)
	oidcProvider, err := oidc.NewProvider(egressCtx, provider.IssuerURL)
	if err != nil {
		h.audit(authState.UserID, "oidc.callback", provider.ID, false, err.Error())
		h.redirectAuthError(ctx, "oidc_discovery_failed")
		return
	}

	oauthConfig := h.oauth2Config(provider, oidcProvider)
	token, err := oauthConfig.Exchange(egressCtx, code)
	if err != nil {
		h.audit(authState.UserID, "oidc.callback", provider.ID, false, "code exchange failed")
		h.redirectAuthError(ctx, "oidc_code_invalid")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		h.audit(authState.UserID, "oidc.callback", provider.ID, false, "id_token missing")
		h.redirectAuthError(ctx, "oidc_token_invalid")
		return
	}

	idToken, err := oidcProvider.Verifier(&oidc.Config{ClientID: provider.ClientID}).Verify(ctx.Request.Context(), rawIDToken)
	if err != nil {
		h.audit(authState.UserID, "oidc.callback", provider.ID, false, "id_token verify failed")
		h.redirectAuthError(ctx, "oidc_token_invalid")
		return
	}
	if idToken.Nonce != authState.Nonce {
		h.audit(authState.UserID, "oidc.callback", provider.ID, false, "nonce mismatch")
		h.redirectAuthError(ctx, "oidc_state_invalid")
		return
	}

	claims, err := oidcClaimsFromToken(idToken, provider)
	if err != nil {
		h.audit(authState.UserID, "oidc.callback", provider.ID, false, err.Error())
		h.redirectAuthError(ctx, "oidc_token_invalid")
		return
	}

	if authState.Mode == "bind" {
		h.completeOIDCBind(ctx, authState, provider, claims)
		return
	}

	user, err := h.findOrCreateOIDCUser(provider, claims)
	if err != nil {
		h.audit("", "oidc.login", provider.ID, false, err.Error())
		h.redirectAuthError(ctx, authErrorCode(err))
		return
	}
	if !h.createSession(ctx, user.ID) {
		return
	}
	if !h.createRememberToken(ctx, user.ID) {
		return
	}
	h.audit(user.ID, "oidc.login", provider.ID, true, "login succeeded")
	ctx.Redirect(http.StatusFound, authState.RedirectPath)
}

func (h *Handlers) completeOIDCBind(ctx *gin.Context, authState oidcAuthStateValue, provider model.AuthProvider, claims oidcIdentityClaims) {
	var user model.User
	if err := h.db.First(&user, "id = ? and disabled = ?", authState.UserID, false).Error; err != nil {
		h.audit(authState.UserID, "oidc.bind", provider.ID, false, "user not found")
		h.redirectAuthError(ctx, "auth_forbidden")
		return
	}

	_, err := h.bindExternalIdentityToUser(user, provider, claims)
	if err != nil {
		h.audit(user.ID, "oidc.bind", provider.ID, false, err.Error())
		h.redirectAuthError(ctx, "oidc_bind_failed")
		return
	}

	h.audit(user.ID, "oidc.bind", provider.ID, true, "identity bound")
	ctx.Redirect(http.StatusFound, "/settings/security")
}

func (h *Handlers) oauth2Config(provider model.AuthProvider, oidcProvider *oidc.Provider) oauth2.Config {
	return oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: h.resolveSecret(provider.ClientSecretRef),
		Endpoint:     oidcProvider.Endpoint(),
		RedirectURL:  oidcCallbackURL(externalBaseURL()),
		Scopes:       normalizeScopes(provider.Scopes),
	}
}

func (h *Handlers) enabledAuthProvider(providerID string) (model.AuthProvider, bool) {
	var provider model.AuthProvider
	err := h.db.First(&provider, "id = ? and enabled = ? and type = ?", providerID, true, "oidc").Error
	return provider, err == nil
}

func (h *Handlers) findOrCreateOIDCUser(provider model.AuthProvider, claims oidcIdentityClaims) (model.User, error) {
	subject := strings.TrimSpace(claims.Subject)
	if provider.ID == "" || subject == "" {
		return model.User{}, errOIDCInvalidIdentity
	}

	var identity model.ExternalIdentity
	if err := h.db.First(&identity, "provider_id = ? and subject = ?", provider.ID, subject).Error; err == nil {
		now := time.Now()
		identity.Email = normalizeEmail(claims.Email)
		identity.EmailVerified = claims.EmailVerified
		identity.Username = strings.TrimSpace(claims.Username)
		identity.LastLoginAt = &now
		_ = h.db.Save(&identity).Error

		var user model.User
		if err := h.db.First(&user, "id = ? and disabled = ?", identity.UserID, false).Error; err != nil {
			return model.User{}, err
		}
		return user, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, err
	}

	if err := h.evaluateAdmission(claims); err != nil {
		return model.User{}, err
	}

	email := normalizeEmail(claims.Email)
	var existing model.User
	if err := h.db.First(&existing, "email = ? and disabled = ?", email, false).Error; err == nil {
		if _, err := h.bindExternalIdentityToUser(existing, provider, claims); err != nil {
			return model.User{}, err
		}
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, err
	}

	now := time.Now()
	policy := h.ensureAdmissionPolicy()
	user := model.User{
		ID:       id.New("usr"),
		Email:    email,
		Name:     fallback(strings.TrimSpace(claims.Name), email),
		AuthType: "oidc",
		Role:     normalizeUserRole(policy.DefaultRole),
		Language: "zh-CN",
	}
	identity = model.ExternalIdentity{
		ID:            id.New("ext"),
		UserID:        user.ID,
		ProviderID:    provider.ID,
		Subject:       subject,
		Email:         email,
		EmailVerified: claims.EmailVerified,
		Username:      strings.TrimSpace(claims.Username),
		LastLoginAt:   &now,
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return tx.Create(&identity).Error
	}); err != nil {
		return model.User{}, err
	}

	return user, nil
}

func oidcClaimsFromToken(idToken *oidc.IDToken, provider model.AuthProvider) (oidcIdentityClaims, error) {
	var raw map[string]any
	if err := idToken.Claims(&raw); err != nil {
		return oidcIdentityClaims{}, err
	}

	subject := stringClaim(raw, "sub")
	if subject == "" {
		return oidcIdentityClaims{}, errOIDCInvalidIdentity
	}

	return oidcIdentityClaims{
		Subject:       subject,
		Email:         stringClaim(raw, fallback(provider.EmailClaim, "email")),
		EmailVerified: boolClaim(raw, "email_verified"),
		Username:      stringClaim(raw, fallback(provider.UsernameClaim, "preferred_username")),
		Name:          fallback(stringClaim(raw, "name"), stringClaim(raw, fallback(provider.UsernameClaim, "preferred_username"))),
		Groups:        stringListClaim(raw, fallback(provider.GroupClaim, "groups")),
	}, nil
}

func (h *Handlers) redirectAuthError(ctx *gin.Context, code string) {
	ctx.Redirect(http.StatusFound, "/login?auth_error="+url.QueryEscape(code))
}

func externalBaseURL() string {
	return strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/")
}

func oidcCallbackURL(publicBaseURL string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if publicBaseURL == "" {
		return ""
	}
	return publicBaseURL + "/api/v1/auth/oidc/callback"
}

func (h *Handlers) resolveSecret(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "env:") {
		return os.Getenv(strings.TrimPrefix(ref, "env:"))
	}
	return h.secrets.Resolve(ref)
}

func normalizeScopes(scopes string) []string {
	fields := strings.Fields(scopes)
	if len(fields) == 0 {
		return []string{oidc.ScopeOpenID, "profile", "email"}
	}
	if fields[0] != oidc.ScopeOpenID {
		fields = append([]string{oidc.ScopeOpenID}, fields...)
	}
	return fields
}

func sanitizeRedirectPath(path string) string {
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") {
		return path
	}
	return "/projects"
}

func stringClaim(claims map[string]any, key string) string {
	value, _ := claims[key].(string)
	return strings.TrimSpace(value)
}

func boolClaim(claims map[string]any, key string) bool {
	value, _ := claims[key].(bool)
	return value
}

func stringListClaim(claims map[string]any, key string) []string {
	value, ok := claims[key]
	if !ok {
		return nil
	}

	switch typed := value.(type) {
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
		return result
	case []string:
		return typed
	case string:
		return splitCSV(typed)
	default:
		bytes, _ := json.Marshal(typed)
		return []string{string(bytes)}
	}
}

func authErrorCode(err error) string {
	switch {
	case errors.Is(err, errOIDCDisabled):
		return "oidc_disabled"
	case errors.Is(err, errOIDCEmailRequired):
		return "oidc_email_required"
	case errors.Is(err, errOIDCGroupDenied):
		return "oidc_group_denied"
	case errors.Is(err, errOIDCAdmissionDenied):
		return "oidc_admission_denied"
	default:
		return "oidc_login_failed"
	}
}

func (h *Handlers) validateOIDCProvider(ctx context.Context, provider model.AuthProvider) error {
	egressCtx := h.adminConfiguredEgressContext(ctx, 15*time.Second)
	if _, err := oidc.NewProvider(egressCtx, provider.IssuerURL); err != nil {
		return fmt.Errorf("OIDC discovery failed: %w", err)
	}
	return nil
}
