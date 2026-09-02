package notificationapi

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	PersonalNotificationRequestMaxBytes = personalNotificationRequestMaxBytes
	PersonalNotificationChannelLimit    = personalNotificationChannelLimit
	PersonalNotificationNameMaxLength   = personalNotificationNameMaxLength
	PersonalNotificationSecretMaxLength = personalNotificationSecretMaxLength
	PersonalNotificationTestRateLimit   = personalNotificationTestRateLimit
)

func DecodeStringMap(raw string) map[string]string {
	return decodeStringMap(raw)
}

func PersonalNotificationPresetSecrets(values map[string]string, required []string) (map[string]string, string) {
	return personalNotificationPresetSecrets(values, required)
}

func PersonalNotificationExistingSecrets(values map[string]string, existingRefsJSON string) (map[string]string, string) {
	return personalNotificationExistingSecrets(values, existingRefsJSON)
}

func ValidatePersonalNotificationChannelName(ctx *gin.Context, name string) bool {
	return validatePersonalNotificationChannelName(ctx, name)
}

func PersonalNotificationChannels(db *gorm.DB, userID string) *gorm.DB {
	return personalNotificationChannels(db, userID)
}
