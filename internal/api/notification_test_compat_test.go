package api

import (
	"io"

	"github.com/LiteyukiStudio/devops/internal/api/notificationapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type platformMailSettingsInput = notificationapi.PlatformMailSettingsInput
type platformMailSettingsResponse = notificationapi.PlatformMailSettingsResponse

func platformMailSettingsFromInput(existing model.PlatformMailSettings, input platformMailSettingsInput) (model.PlatformMailSettings, string) {
	return notificationapi.PlatformMailSettingsFromInput(existing, input)
}

func platformMailSettingsResponseFor(settings model.PlatformMailSettings) platformMailSettingsResponse {
	return notificationapi.PlatformMailSettingsResponseFor(settings)
}

type notificationRuleInput = notificationapi.NotificationRuleInput

func (h *Handlers) notificationRuleFromInput(ctx *gin.Context, input notificationRuleInput, rule model.NotificationRule) (model.NotificationRule, bool) {
	return h.notificationAPI().NotificationRuleFromInput(ctx, input, rule)
}

const (
	personalNotificationRequestMaxBytes = notificationapi.PersonalNotificationRequestMaxBytes
	personalNotificationChannelLimit    = notificationapi.PersonalNotificationChannelLimit
	personalNotificationNameMaxLength   = notificationapi.PersonalNotificationNameMaxLength
	personalNotificationSecretMaxLength = notificationapi.PersonalNotificationSecretMaxLength
	personalNotificationTestRateLimit   = notificationapi.PersonalNotificationTestRateLimit
)

func decodeStringMap(raw string) map[string]string {
	return notificationapi.DecodeStringMap(raw)
}

func personalNotificationPresetSecrets(values map[string]string, required []string) (map[string]string, string) {
	return notificationapi.PersonalNotificationPresetSecrets(values, required)
}

func personalNotificationExistingSecrets(values map[string]string, existingRefsJSON string) (map[string]string, string) {
	return notificationapi.PersonalNotificationExistingSecrets(values, existingRefsJSON)
}

func validatePersonalNotificationChannelName(ctx *gin.Context, name string) bool {
	return notificationapi.ValidatePersonalNotificationChannelName(ctx, name)
}

func personalNotificationChannels(db *gorm.DB, userID string) *gorm.DB {
	return notificationapi.PersonalNotificationChannels(db, userID)
}

func (h *Handlers) writePersonalNotificationTestFailure(ctx *gin.Context, userID, channelID string, err error) {
	h.notificationAPI().WritePersonalNotificationTestFailure(ctx, userID, channelID, err)
}

type inboxMessageResponse = notificationapi.InboxMessageResponse
type inboxChangedEvent = notificationapi.InboxChangedEvent

func inboxMessageResponseFor(message model.InboxMessage) inboxMessageResponse {
	return notificationapi.InboxMessageResponseFor(message)
}

func newInboxChangeBroker() *notificationapi.InboxChangeBroker {
	return notificationapi.NewInboxChangeBroker()
}

func writeInboxChangedEvent(writer io.Writer, event inboxChangedEvent) error {
	return notificationapi.WriteInboxChangedEvent(writer, event)
}
