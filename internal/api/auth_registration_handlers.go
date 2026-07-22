package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	authRegistrationSettingsID = "default"
	emailRegistrationCodeTTL   = 10 * time.Minute
	emailRegistrationMaxTries  = 5
)

type authRegistrationSettingsInput struct {
	AllowEmailRegistration        bool   `json:"allowEmailRegistration"`
	AllowOIDCRegistration         bool   `json:"allowOidcRegistration"`
	AllowExternalIdentityPassword bool   `json:"allowExternalIdentityPassword"`
	SMTPHost                      string `json:"smtpHost"`
	SMTPPort                      int    `json:"smtpPort"`
	SMTPSecurity                  string `json:"smtpSecurity"`
	SMTPUsername                  string `json:"smtpUsername"`
	SMTPPassword                  string `json:"smtpPassword"`
	SMTPFromAddress               string `json:"smtpFromAddress"`
	SMTPFromName                  string `json:"smtpFromName"`
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
	settings := h.ensureAuthRegistrationSettings()
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
	ctx.JSON(http.StatusOK, authRegistrationSettingsResponse(h.ensureAuthRegistrationSettings()))
}

func (h *Handlers) UpdateAuthRegistrationSettings(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeSecuritySettingsUpdate) {
		return
	}

	var input authRegistrationSettingsInput
	if !bindJSON(ctx, &input) {
		return
	}
	settings := h.ensureAuthRegistrationSettings()
	settings.AllowEmailRegistration = input.AllowEmailRegistration
	settings.AllowOIDCRegistration = input.AllowOIDCRegistration
	settings.AllowExternalIdentityPassword = input.AllowExternalIdentityPassword
	settings.SMTPHost = strings.TrimSpace(input.SMTPHost)
	settings.SMTPPort = input.SMTPPort
	settings.SMTPSecurity = strings.ToLower(strings.TrimSpace(input.SMTPSecurity))
	settings.SMTPUsername = strings.TrimSpace(input.SMTPUsername)
	settings.SMTPFromAddress = strings.TrimSpace(input.SMTPFromAddress)
	settings.SMTPFromName = strings.TrimSpace(input.SMTPFromName)
	if settings.SMTPPort == 0 {
		settings.SMTPPort = 587
	}
	if settings.SMTPSecurity == "" {
		settings.SMTPSecurity = "starttls"
	}
	if settings.SMTPFromName == "" {
		settings.SMTPFromName = "Luna DevOps"
	}
	if err := validateAuthRegistrationSettings(settings, strings.TrimSpace(input.SMTPPassword) != ""); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "registration.settings_invalid", err.Error())
		return
	}
	if password := strings.TrimSpace(input.SMTPPassword); password != "" {
		ref := h.secrets.Store(password, user.ID, "auth_registration_settings:smtp_password")
		if ref == "" {
			writeErrorCode(ctx, http.StatusInternalServerError, "registration.smtp_secret_failed", "failed to store SMTP password")
			return
		}
		settings.SMTPPasswordRef = ref
	}
	if settings.SMTPUsername != "" && settings.SMTPPasswordRef == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "registration.smtp_password_required", "SMTP password is required when username is set")
		return
	}
	if err := h.db.Save(&settings).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "registration.settings_update_failed", err.Error())
		return
	}
	h.audit(user.ID, "auth.registration_settings.update", settings.ID, true, "registration settings updated")
	ctx.JSON(http.StatusOK, authRegistrationSettingsResponse(settings))
}

func (h *Handlers) RequestEmailRegistrationCode(ctx *gin.Context) {
	if !h.allowSensitiveAuthAttempt(ctx, "email_registration_ip", 8, 10*time.Minute) {
		return
	}
	settings := h.ensureAuthRegistrationSettings()
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
	if err := h.db.Model(&model.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
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
	if err := h.db.Create(&challenge).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "registration.challenge_failed", err.Error())
		return
	}
	if err := h.sendRegistrationEmail(ctx.Request.Context(), settings, challenge, code); err != nil {
		_ = h.db.Delete(&challenge).Error
		writeErrorCode(ctx, http.StatusBadGateway, "registration.email_send_failed", err.Error())
		return
	}
	ctx.JSON(http.StatusAccepted, gin.H{"challengeId": challenge.ID, "expiresAt": challenge.ExpiresAt})
}

func (h *Handlers) CompleteEmailRegistration(ctx *gin.Context) {
	if !h.allowSensitiveAuthAttempt(ctx, "email_registration_complete_ip", 12, 10*time.Minute) {
		return
	}
	if !h.ensureAuthRegistrationSettings().AllowEmailRegistration {
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
		Role:     "user",
		Language: normalizeLanguage(input.Language),
		Password: string(passwordHash),
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
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
	h.audit(user.ID, "auth.email_registration", user.ID, true, "email registration completed")
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
		if !h.ensureAuthRegistrationSettings().AllowExternalIdentityPassword {
			writeErrorCode(ctx, http.StatusForbidden, "password.enrollment_disabled", "password enrollment is disabled")
			return
		}
		session, sessionOK := h.currentSessionFromCookie(ctx)
		if !sessionOK || session.UserID != user.ID || session.ImpersonatorID != "" || session.PrimaryAuthenticatedAt == nil || time.Since(*session.PrimaryAuthenticatedAt) > mfaEnrollmentOIDCSessionMaxAge {
			writeErrorCode(ctx, http.StatusUnauthorized, "password.fresh_login_required", "sign in again before setting a password")
			return
		}
	}
	if !h.requireStepUp(ctx, user, stepUpPurposePasswordUpdate) {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "password.update_failed", err.Error())
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
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
	h.audit(user.ID, "auth.password_update", user.ID, true, "password updated and sessions revoked")
	ctx.Status(http.StatusNoContent)
}

var (
	errRegistrationChallengeInvalid = errors.New("registration challenge is invalid")
	errRegistrationCodeInvalid      = errors.New("registration code is invalid")
)

func (h *Handlers) ensureAuthRegistrationSettings() model.AuthRegistrationSettings {
	var settings model.AuthRegistrationSettings
	if err := h.db.First(&settings, "id = ?", authRegistrationSettingsID).Error; err == nil {
		return settings
	}
	settings = model.AuthRegistrationSettings{
		ID:                    authRegistrationSettingsID,
		AllowOIDCRegistration: true,
		SMTPPort:              587,
		SMTPSecurity:          "starttls",
		SMTPFromName:          "Luna DevOps",
	}
	_ = h.db.Create(&settings).Error
	return settings
}

func authRegistrationSettingsResponse(settings model.AuthRegistrationSettings) gin.H {
	return gin.H{
		"allowEmailRegistration":        settings.AllowEmailRegistration,
		"allowOidcRegistration":         settings.AllowOIDCRegistration,
		"allowExternalIdentityPassword": settings.AllowExternalIdentityPassword,
		"smtpHost":                      settings.SMTPHost,
		"smtpPort":                      settings.SMTPPort,
		"smtpSecurity":                  settings.SMTPSecurity,
		"smtpUsername":                  settings.SMTPUsername,
		"smtpPasswordSet":               strings.TrimSpace(settings.SMTPPasswordRef) != "",
		"smtpFromAddress":               settings.SMTPFromAddress,
		"smtpFromName":                  settings.SMTPFromName,
	}
}

func validateAuthRegistrationSettings(settings model.AuthRegistrationSettings, passwordProvided bool) error {
	if settings.SMTPPort < 1 || settings.SMTPPort > 65535 {
		return errors.New("SMTP port must be between 1 and 65535")
	}
	switch settings.SMTPSecurity {
	case "none", "starttls", "tls":
	default:
		return errors.New("SMTP security must be none, starttls, or tls")
	}
	if settings.AllowEmailRegistration {
		if settings.SMTPHost == "" || settings.SMTPFromAddress == "" {
			return errors.New("SMTP host and sender address are required when email registration is enabled")
		}
		address, err := mail.ParseAddress(settings.SMTPFromAddress)
		if err != nil || !strings.EqualFold(address.Address, settings.SMTPFromAddress) {
			return errors.New("SMTP sender address is invalid")
		}
		if settings.SMTPUsername != "" && settings.SMTPPasswordRef == "" && !passwordProvided {
			return errors.New("SMTP password is required when username is set")
		}
	}
	return nil
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

func (h *Handlers) sendRegistrationEmail(ctx context.Context, settings model.AuthRegistrationSettings, challenge model.EmailRegistrationChallenge, code string) error {
	from := settings.SMTPFromAddress
	if settings.SMTPFromName != "" {
		from = (&mail.Address{Name: settings.SMTPFromName, Address: settings.SMTPFromAddress}).String()
	}
	cfg := notification.SMTPConfig{
		Host:      settings.SMTPHost,
		Port:      settings.SMTPPort,
		Security:  settings.SMTPSecurity,
		Username:  settings.SMTPUsername,
		From:      from,
		To:        []string{challenge.Email},
		Timeout:   15,
		SecretRef: settings.SMTPPasswordRef,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	message := notification.RenderedMessage{
		Subject: "Luna DevOps verification code",
		Body:    fmt.Sprintf("Your Luna DevOps verification code is %s. It expires in 10 minutes.", code),
	}
	if challenge.Language == "zh-CN" {
		message.Subject = "Luna DevOps 邮箱验证码"
		message.Body = fmt.Sprintf("你的 Luna DevOps 邮箱验证码是 %s，10 分钟内有效。", code)
	}
	_, err = (notification.SMTPAdapter{}).Send(ctx, raw, nil, message, h.secrets)
	return err
}
