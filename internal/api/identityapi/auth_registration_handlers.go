package identityapi

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/platformmail"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	authRegistrationSettingsID = "default"
	emailRegistrationCodeTTL   = 10 * time.Minute
	emailRegistrationMaxTries  = 5
	passwordFreshLoginMaxAge   = 5 * time.Minute
)

type authRegistrationSettingsInput struct {
	AllowEmailRegistration        bool `json:"allowEmailRegistration"`
	AllowOIDCRegistration         bool `json:"allowOidcRegistration"`
	AllowExternalIdentityPassword bool `json:"allowExternalIdentityPassword"`
}

type requestEmailRegistrationCodeInput struct {
	Email    string `json:"email" binding:"required"`
	Language string `json:"language"`
}

type completeEmailRegistrationInput struct {
	ChallengeID string `json:"challengeId" binding:"required"`
	Code        string `json:"code" binding:"required"`
	Email       string `json:"email" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Password    string `json:"password" binding:"required"`
	Language    string `json:"language"`
	RememberMe  bool   `json:"rememberMe"`
}

type updateMyPasswordInput struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword" binding:"required"`
}

func (h *Handlers) GetAuthRegistrationStatus(ctx *gin.Context) {
	settings := h.ensureAuthRegistrationSettings(ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{
		"emailRegistrationEnabled":        settings.AllowEmailRegistration,
		"oidcRegistrationEnabled":         settings.AllowOIDCRegistration,
		"externalIdentityPasswordEnabled": settings.AllowExternalIdentityPassword,
	})
}

func (h *Handlers) GetAuthRegistrationSettings(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	ctx.JSON(http.StatusOK, authRegistrationSettingsResponse(h.ensureAuthRegistrationSettings(ctx.Request.Context())))
}

func (h *Handlers) UpdateAuthRegistrationSettings(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input authRegistrationSettingsInput
	if !bindJSON(ctx, &input) {
		return
	}
	settings := h.ensureAuthRegistrationSettings(ctx.Request.Context())
	settings.AllowEmailRegistration = input.AllowEmailRegistration
	settings.AllowOIDCRegistration = input.AllowOIDCRegistration
	settings.AllowExternalIdentityPassword = input.AllowExternalIdentityPassword
	if settings.AllowEmailRegistration {
		mailSettings, err := platformmail.Get(ctx.Request.Context(), h.dbFor(ctx))
		if err != nil {
			writeErrorCode(ctx, http.StatusInternalServerError, "registration.mail_settings_failed", err.Error())
			return
		}
		if err := platformmail.Validate(mailSettings, false); err != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "registration.mail_not_configured", err.Error())
			return
		}
	}
	if err := h.dbFor(ctx).Save(&settings).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "registration.settings_update_failed", err.Error())
		return
	}
	h.auditWithContext(user.ID, "auth.registration_settings.update", settings.ID, true, "registration settings updated", ctx.Request.Context())
	ctx.JSON(http.StatusOK, authRegistrationSettingsResponse(settings))
}

func (h *Handlers) RequestEmailRegistrationCode(ctx *gin.Context) {
	if !h.allowSensitiveAuthAttempt(ctx, "email_registration_ip", 8, 10*time.Minute) {
		return
	}
	settings := h.ensureAuthRegistrationSettings(ctx.Request.Context())
	if !settings.AllowEmailRegistration {
		writeErrorCode(ctx, http.StatusForbidden, "registration.email_disabled", "email registration is disabled")
		return
	}
	var input requestEmailRegistrationCodeInput
	if !bindJSON(ctx, &input) {
		return
	}
	email, err := normalizedRegistrationEmail(input.Email)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "registration.email_invalid", err.Error())
		return
	}
	if !h.allowSensitiveAuthKey(ctx, "email_registration_account", hashToken(email), 3, 10*time.Minute) {
		return
	}
	var count int64
	if err := h.dbFor(ctx).Model(&model.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "registration.lookup_failed", err.Error())
		return
	}
	if count > 0 {
		writeErrorCode(ctx, http.StatusConflict, "registration.email_exists", "email is already registered")
		return
	}
	code, err := registrationVerificationCode()
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "registration.code_failed", err.Error())
		return
	}
	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "registration.code_failed", err.Error())
		return
	}
	challenge := model.EmailRegistrationChallenge{
		ID:        id.New("regc"),
		Email:     email,
		CodeHash:  string(codeHash),
		Language:  normalizeLanguage(input.Language),
		ExpiresAt: time.Now().Add(emailRegistrationCodeTTL),
	}
	if err := h.dbFor(ctx).Create(&challenge).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "registration.challenge_failed", err.Error())
		return
	}
	if err := h.sendRegistrationEmail(ctx.Request.Context(), challenge, code); err != nil {
		_ = h.dbFor(ctx).Delete(&challenge).Error
		writeErrorCode(ctx, http.StatusBadGateway, "registration.email_send_failed", err.Error())
		return
	}
	ctx.JSON(http.StatusAccepted, gin.H{"challengeId": challenge.ID, "expiresAt": challenge.ExpiresAt})
}

func (h *Handlers) CompleteEmailRegistration(ctx *gin.Context) {
	if !h.allowSensitiveAuthAttempt(ctx, "email_registration_complete_ip", 12, 10*time.Minute) {
		return
	}
	if !h.ensureAuthRegistrationSettings(ctx.Request.Context()).AllowEmailRegistration {
		writeErrorCode(ctx, http.StatusForbidden, "registration.email_disabled", "email registration is disabled")
		return
	}
	var input completeEmailRegistrationInput
	if !bindJSON(ctx, &input) {
		return
	}
	email, err := normalizedRegistrationEmail(input.Email)
	if err != nil || len(input.Password) < 8 || strings.TrimSpace(input.Name) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "registration.input_invalid", "name, email, and a password of at least 8 characters are required")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "registration.password_failed", err.Error())
		return
	}
	user := model.User{
		ID:       id.New("usr"),
		Email:    email,
		Name:     strings.TrimSpace(input.Name),
		Role:     authz.PlatformRoleUser,
		Language: normalizeLanguage(input.Language),
		Password: string(passwordHash),
	}
	err = h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var challenge model.EmailRegistrationChallenge
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&challenge, "id = ?", strings.TrimSpace(input.ChallengeID)).Error; err != nil {
			return err
		}
		if challenge.Email != email || challenge.ConsumedAt != nil || time.Now().After(challenge.ExpiresAt) || challenge.Attempts >= emailRegistrationMaxTries {
			return errRegistrationChallengeInvalid
		}
		if bcrypt.CompareHashAndPassword([]byte(challenge.CodeHash), []byte(strings.TrimSpace(input.Code))) != nil {
			challenge.Attempts++
			if err := tx.Save(&challenge).Error; err != nil {
				return err
			}
			return errRegistrationCodeInvalid
		}
		now := time.Now()
		challenge.ConsumedAt = &now
		if err := tx.Save(&challenge).Error; err != nil {
			return err
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return createDefaultUserProject(tx, user)
	})
	if err != nil {
		switch {
		case errors.Is(err, errRegistrationCodeInvalid):
			writeErrorCode(ctx, http.StatusUnauthorized, "registration.code_invalid", "verification code is invalid")
		case errors.Is(err, errRegistrationChallengeInvalid), errors.Is(err, gorm.ErrRecordNotFound):
			writeErrorCode(ctx, http.StatusGone, "registration.challenge_invalid", "registration challenge is invalid or expired")
		default:
			writeErrorCode(ctx, http.StatusConflict, "registration.create_failed", err.Error())
		}
		return
	}
	if !h.createLoginCredentials(ctx, user.ID, input.RememberMe) {
		return
	}
	h.auditWithContext(user.ID, "auth.email_registration", user.ID, true, "email registration completed", ctx.Request.Context())
	ctx.JSON(http.StatusCreated, gin.H{"user": currentUserResponse(user)})
}

func (h *Handlers) UpdateMyPassword(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input updateMyPasswordInput
	if !bindJSON(ctx, &input) {
		return
	}
	if len(input.NewPassword) < 8 {
		writeErrorCode(ctx, http.StatusBadRequest, "password.too_short", "password must contain at least 8 characters")
		return
	}
	hasPassword := strings.TrimSpace(user.Password) != ""
	if hasPassword {
		if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.CurrentPassword)) != nil {
			writeErrorCode(ctx, http.StatusUnauthorized, "password.current_invalid", "current password is invalid")
			return
		}
	} else {
		if !h.ensureAuthRegistrationSettings(ctx.Request.Context()).AllowExternalIdentityPassword {
			writeErrorCode(ctx, http.StatusForbidden, "password.enrollment_disabled", "password enrollment is disabled")
			return
		}
		session, sessionOK := h.currentSessionFromCookie(ctx)
		if !sessionOK || session.UserID != user.ID || session.ImpersonatorID != "" || session.PrimaryAuthenticatedAt == nil || time.Since(*session.PrimaryAuthenticatedAt) > passwordFreshLoginMaxAge {
			writeErrorCode(ctx, http.StatusUnauthorized, "password.fresh_login_required", "sign in again before setting a password")
			return
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "password.update_failed", err.Error())
		return
	}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", user.ID).Update("password", string(hash)).Error; err != nil {
			return err
		}
		return revokeUserAuthentication(tx, user.ID)
	}); err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "password.update_failed", err.Error())
		return
	}
	clearSessionCookie(ctx)
	clearRememberCookie(ctx, user.ID)
	h.auditWithContext(user.ID, "auth.password_update", user.ID, true, "password updated and sessions revoked", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

var (
	errRegistrationChallengeInvalid = errors.New("registration challenge is invalid")
	errRegistrationCodeInvalid      = errors.New("registration code is invalid")
)

func (h *Handlers) ensureAuthRegistrationSettings(ctx context.Context) model.AuthRegistrationSettings {
	var settings model.AuthRegistrationSettings
	if err := h.dbWithContext(ctx).First(&settings, "id = ?", authRegistrationSettingsID).Error; err == nil {
		return settings
	}
	settings = model.AuthRegistrationSettings{
		ID:                    authRegistrationSettingsID,
		AllowOIDCRegistration: true,
	}
	_ = h.dbWithContext(ctx).Create(&settings).Error
	return settings
}

func authRegistrationSettingsResponse(settings model.AuthRegistrationSettings) gin.H {
	return gin.H{
		"allowEmailRegistration":        settings.AllowEmailRegistration,
		"allowOidcRegistration":         settings.AllowOIDCRegistration,
		"allowExternalIdentityPassword": settings.AllowExternalIdentityPassword,
	}
}

func normalizedRegistrationEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	if err != nil || !strings.EqualFold(address.Address, value) {
		return "", errors.New("email address is invalid")
	}
	return value, nil
}

func registrationVerificationCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

type platformMailSendFunc func(
	context.Context,
	*gorm.DB,
	notification.SecretResolver,
	string,
	notification.RenderedMessage,
) (notification.SendResult, error)

func (h *Handlers) sendRegistrationEmail(ctx context.Context, challenge model.EmailRegistrationChallenge, code string) error {
	return sendRegistrationEmailWith(ctx, h.dbWithContext(ctx), h.secrets, challenge, code, platformmail.Send)
}

func sendRegistrationEmailWith(
	ctx context.Context,
	db *gorm.DB,
	resolver notification.SecretResolver,
	challenge model.EmailRegistrationChallenge,
	code string,
	send platformMailSendFunc,
) error {
	message := notification.RenderedMessage{
		Subject: "Luna DevOps verification code",
		Body:    fmt.Sprintf("Your Luna DevOps verification code is %s. It expires in 10 minutes.", code),
	}
	if challenge.Language == "zh-CN" {
		message.Subject = "Luna DevOps 邮箱验证码"
		message.Body = fmt.Sprintf("你的 Luna DevOps 邮箱验证码是 %s，10 分钟内有效。", code)
	}
	_, err := send(ctx, db, resolver, challenge.Email, message)
	return err
}
