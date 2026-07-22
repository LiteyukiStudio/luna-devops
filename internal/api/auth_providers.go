package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListAuthProviders(ctx *gin.Context) {
	var providers []model.AuthProvider
	query := h.db.Order("is_default desc, created_at desc")
	if ctx.Query("includeDisabled") == "true" {
		if !h.requirePlatformAdmin(ctx) {
			return
		}
	} else {
		query = query.Where("enabled = ?", true)
	}
	if err := query.Find(&providers).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, authProviderResponses(providers))
}

func (h *Handlers) GetOIDCCallbackURL(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	publicBaseURL := externalBaseURL()
	callbackURL := ""
	if publicBaseURL != "" {
		callbackURL = oidcCallbackURL(publicBaseURL)
	}
	ctx.JSON(http.StatusOK, gin.H{
		"publicBaseUrl": publicBaseURL,
		"callbackUrl":   callbackURL,
		"configured":    callbackURL != "",
	})
}

func (h *Handlers) CreateAuthProvider(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input authProviderInput
	if !bindJSON(ctx, &input) {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeAuthProviderUpdate) {
		return
	}
	providerID := id.New("ap")
	if secret := strings.TrimSpace(input.ClientSecret); secret != "" {
		input.ClientSecretRef = h.secrets.Store(secret, "", "auth_provider:"+providerID+":client_secret")
		input.ClientSecret = ""
	}

	provider, ok := authProviderFromInput(input, providerID, "")
	if !ok {
		writeError(ctx, http.StatusBadRequest, "请输入有效的 OIDC Provider 配置")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if provider.IsDefault {
			if err := tx.Model(&model.AuthProvider{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&provider).Error
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	ctx.JSON(http.StatusCreated, authProviderResponse(provider))
}

func (h *Handlers) UpdateAuthProvider(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var provider model.AuthProvider
	if err := h.db.First(&provider, "id = ?", ctx.Param("providerId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "auth provider not found")
		return
	}

	var input authProviderInput
	if !bindJSON(ctx, &input) {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeAuthProviderUpdate) {
		return
	}
	if secret := strings.TrimSpace(input.ClientSecret); secret != "" {
		input.ClientSecretRef = h.secrets.Store(secret, "", "auth_provider:"+provider.ID+":client_secret")
		input.ClientSecret = ""
	}

	next, ok := authProviderFromInput(input, provider.ID, provider.ClientSecretRef)
	if !ok {
		writeError(ctx, http.StatusBadRequest, "请输入有效的 OIDC Provider 配置")
		return
	}

	provider.Type = next.Type
	provider.Name = next.Name
	provider.Enabled = next.Enabled
	provider.IssuerURL = next.IssuerURL
	provider.ClientID = next.ClientID
	provider.ClientSecretRef = next.ClientSecretRef
	provider.Scopes = next.Scopes
	provider.GroupClaim = next.GroupClaim
	provider.EmailClaim = next.EmailClaim
	provider.UsernameClaim = next.UsernameClaim
	provider.IsDefault = next.IsDefault

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if provider.IsDefault {
			if err := tx.Model(&model.AuthProvider{}).
				Where("id <> ? and is_default = ?", provider.ID, true).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Save(&provider).Error
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, authProviderResponse(provider))
}

func (h *Handlers) ListMyExternalIdentities(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var identities []externalIdentityResponse
	if err := h.db.Table("external_identities").
		Select("external_identities.id, external_identities.user_id, external_identities.provider_id, auth_providers.name as provider_name, external_identities.subject, external_identities.email, external_identities.email_verified, external_identities.username, external_identities.last_login_at, external_identities.created_at").
		Joins("join auth_providers on auth_providers.id = external_identities.provider_id").
		Where("external_identities.user_id = ?", user.ID).
		Order("external_identities.created_at desc").
		Scan(&identities).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, identities)
}

func (h *Handlers) UnbindMyExternalIdentity(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var identity model.ExternalIdentity
	if err := h.db.First(&identity, "id = ? and user_id = ?", ctx.Param("identityId"), user.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "external identity not found")
		return
	}

	var identityCount int64
	_ = h.db.Model(&model.ExternalIdentity{}).Where("user_id = ?", user.ID).Count(&identityCount).Error
	if identityCount <= 1 && strings.TrimSpace(user.Password) == "" {
		writeError(ctx, http.StatusBadRequest, "请先设置本地密码或绑定另一个第三方登录")
		return
	}

	if err := h.db.Delete(&identity).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) bindExternalIdentityToUser(user model.User, provider model.AuthProvider, claims oidcIdentityClaims) (model.ExternalIdentity, error) {
	subject := strings.TrimSpace(claims.Subject)
	if user.ID == "" || provider.ID == "" || subject == "" {
		return model.ExternalIdentity{}, errors.New("external identity requires user, provider and subject")
	}

	var existing model.ExternalIdentity
	if err := h.db.First(&existing, "provider_id = ? and subject = ?", provider.ID, subject).Error; err == nil {
		if existing.UserID == user.ID {
			return existing, nil
		}
		return model.ExternalIdentity{}, errors.New("external identity already belongs to another user")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ExternalIdentity{}, err
	}

	identity := model.ExternalIdentity{
		ID:            id.New("ext"),
		UserID:        user.ID,
		ProviderID:    provider.ID,
		Subject:       subject,
		Email:         normalizeEmail(claims.Email),
		EmailVerified: claims.EmailVerified,
		Username:      strings.TrimSpace(claims.Username),
	}
	if err := h.db.Create(&identity).Error; err != nil {
		return model.ExternalIdentity{}, err
	}
	return identity, nil
}

func authProviderFromInput(input authProviderInput, providerID string, existingSecretRef string) (model.AuthProvider, bool) {
	providerType := strings.ToLower(strings.TrimSpace(input.Type))
	if providerType == "" {
		providerType = "oidc"
	}
	clientSecretRef := strings.TrimSpace(input.ClientSecretRef)
	if strings.TrimSpace(input.ClientSecret) != "" {
		return model.AuthProvider{}, false
	} else if clientSecretRef == "" {
		clientSecretRef = existingSecretRef
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	provider := model.AuthProvider{
		ID:              providerID,
		Type:            providerType,
		Name:            strings.TrimSpace(input.Name),
		Enabled:         enabled,
		IssuerURL:       strings.TrimRight(strings.TrimSpace(input.IssuerURL), "/"),
		ClientID:        strings.TrimSpace(input.ClientID),
		ClientSecretRef: clientSecretRef,
		Scopes:          fallback(strings.TrimSpace(input.Scopes), "openid profile email"),
		GroupClaim:      fallback(strings.TrimSpace(input.GroupClaim), "groups"),
		EmailClaim:      fallback(strings.TrimSpace(input.EmailClaim), "email"),
		UsernameClaim:   fallback(strings.TrimSpace(input.UsernameClaim), "preferred_username"),
		IsDefault:       input.IsDefault,
	}
	return provider, provider.Type == "oidc" && provider.Name != "" && provider.IssuerURL != "" && provider.ClientID != ""
}

func authProviderResponses(providers []model.AuthProvider) []authProviderOutput {
	result := make([]authProviderOutput, 0, len(providers))
	for _, provider := range providers {
		result = append(result, authProviderResponse(provider))
	}
	return result
}

func authProviderResponse(provider model.AuthProvider) authProviderOutput {
	return authProviderOutput{
		ID:              provider.ID,
		Type:            provider.Type,
		Name:            provider.Name,
		Enabled:         provider.Enabled,
		IssuerURL:       provider.IssuerURL,
		ClientID:        provider.ClientID,
		ClientSecretRef: secret.SafeClientSecretRef(provider.ClientSecretRef),
		ClientSecretSet: secret.HasValue(provider.ClientSecretRef),
		Scopes:          provider.Scopes,
		GroupClaim:      provider.GroupClaim,
		EmailClaim:      provider.EmailClaim,
		UsernameClaim:   provider.UsernameClaim,
		IsDefault:       provider.IsDefault,
		CreatedAt:       provider.CreatedAt,
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

type authProviderInput struct {
	Type            string `json:"type"`
	Name            string `json:"name" binding:"required"`
	Enabled         *bool  `json:"enabled"`
	IssuerURL       string `json:"issuerUrl" binding:"required"`
	ClientID        string `json:"clientId" binding:"required"`
	ClientSecret    string `json:"clientSecret"`
	ClientSecretRef string `json:"clientSecretRef"`
	Scopes          string `json:"scopes"`
	GroupClaim      string `json:"groupClaim"`
	EmailClaim      string `json:"emailClaim"`
	UsernameClaim   string `json:"usernameClaim"`
	IsDefault       bool   `json:"isDefault"`
}

type authProviderOutput struct {
	ID              string    `json:"id"`
	Type            string    `json:"type"`
	Name            string    `json:"name"`
	Enabled         bool      `json:"enabled"`
	IssuerURL       string    `json:"issuerUrl"`
	ClientID        string    `json:"clientId"`
	ClientSecretRef string    `json:"clientSecretRef"`
	ClientSecretSet bool      `json:"clientSecretSet"`
	Scopes          string    `json:"scopes"`
	GroupClaim      string    `json:"groupClaim"`
	EmailClaim      string    `json:"emailClaim"`
	UsernameClaim   string    `json:"usernameClaim"`
	IsDefault       bool      `json:"isDefault"`
	CreatedAt       time.Time `json:"createdAt"`
}

type oidcIdentityClaims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Username      string
	Name          string
	Groups        []string
}

type externalIdentityResponse struct {
	ID            string     `json:"id"`
	UserID        string     `json:"userId"`
	ProviderID    string     `json:"providerId"`
	ProviderName  string     `json:"providerName"`
	Subject       string     `json:"subject"`
	Email         string     `json:"email"`
	EmailVerified bool       `json:"emailVerified"`
	Username      string     `json:"username"`
	LastLoginAt   *time.Time `json:"lastLoginAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}
