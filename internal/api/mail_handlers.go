package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/platformmail"
	"github.com/gin-gonic/gin"
)

type platformMailSettingsInput struct {
	Host                         string `json:"host"`
	Port                         int    `json:"port"`
	Security                     string `json:"security"`
	Username                     string `json:"username"`
	Password                     string `json:"password"`
	FromAddress                  string `json:"fromAddress"`
	FromName                     string `json:"fromName"`
	PersonalEmailCooldownSeconds *int   `json:"personalEmailCooldownSeconds" binding:"required"`
}

type platformMailTestInput struct {
	Recipient string `json:"recipient" binding:"required"`
}

type platformMailSettingsResponse struct {
	Host                         string `json:"host"`
	Port                         int    `json:"port"`
	Security                     string `json:"security"`
	Username                     string `json:"username"`
	PasswordSet                  bool   `json:"passwordSet"`
	FromAddress                  string `json:"fromAddress"`
	FromName                     string `json:"fromName"`
	PersonalEmailCooldownSeconds int    `json:"personalEmailCooldownSeconds"`
}

func (h *Handlers) GetPlatformMailSettings(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	settings, err := platformmail.Get(ctx.Request.Context(), h.dbFor(ctx))
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "mail.settings_load_failed", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, platformMailSettingsResponseFor(settings))
}

func (h *Handlers) UpdatePlatformMailSettings(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input platformMailSettingsInput
	if !bindJSON(ctx, &input) {
		return
	}
	existing, err := platformmail.Get(ctx.Request.Context(), h.dbFor(ctx))
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "mail.settings_load_failed", err.Error())
		return
	}
	settings, password := platformMailSettingsFromInput(existing, input)
	if err := platformmail.Validate(settings, password != ""); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "mail.settings_invalid", err.Error())
		return
	}
	if password != "" {
		settings.PasswordRef = h.secrets.StoreContext(
			ctx.Request.Context(),
			password,
			user.ID,
			"platform_mail_settings:"+platformmail.SettingsID+":password",
		)
		if settings.PasswordRef == "" {
			writeErrorCode(ctx, http.StatusInternalServerError, "mail.secret_store_failed", "failed to store SMTP password")
			return
		}
	}
	settings, err = platformmail.Save(ctx.Request.Context(), h.dbFor(ctx), settings)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "mail.settings_update_failed", err.Error())
		return
	}
	h.auditWithContext(user.ID, "mail.settings.update", settings.ID, true, "mail settings updated", ctx.Request.Context())
	ctx.JSON(http.StatusOK, platformMailSettingsResponseFor(settings))
}

func (h *Handlers) TestPlatformMailSettings(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input platformMailTestInput
	if !bindJSON(ctx, &input) {
		return
	}
	recipient, err := normalizedRegistrationEmail(input.Recipient)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "mail.recipient_invalid", err.Error())
		return
	}
	message := notification.RenderedMessage{
		Subject: "Luna DevOps test email",
		Body:    "This test email confirms that the Luna DevOps platform mail service is configured correctly.",
	}
	_, err = platformmail.Send(ctx.Request.Context(), h.dbFor(ctx), h.secrets, recipient, message)
	if err != nil {
		h.auditWithContext(user.ID, "mail.settings.test", platformmail.SettingsID, false, err.Error(), ctx.Request.Context())
		writeErrorCode(ctx, http.StatusBadGateway, "mail.test_failed", err.Error())
		return
	}
	h.auditWithContext(user.ID, "mail.settings.test", platformmail.SettingsID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func platformMailSettingsFromInput(existing model.PlatformMailSettings, input platformMailSettingsInput) (model.PlatformMailSettings, string) {
	settings := existing
	settings.Host = input.Host
	settings.Port = input.Port
	settings.Security = input.Security
	settings.Username = input.Username
	settings.FromAddress = input.FromAddress
	settings.FromName = input.FromName
	if input.PersonalEmailCooldownSeconds != nil {
		settings.PersonalEmailCooldownSeconds = *input.PersonalEmailCooldownSeconds
	}
	return platformmail.Normalize(settings), strings.TrimSpace(input.Password)
}

func platformMailSettingsResponseFor(settings model.PlatformMailSettings) platformMailSettingsResponse {
	settings = platformmail.Normalize(settings)
	return platformMailSettingsResponse{
		Host:                         settings.Host,
		Port:                         settings.Port,
		Security:                     settings.Security,
		Username:                     settings.Username,
		PasswordSet:                  settings.PasswordRef != "",
		FromAddress:                  settings.FromAddress,
		FromName:                     settings.FromName,
		PersonalEmailCooldownSeconds: settings.PersonalEmailCooldownSeconds,
	}
}
