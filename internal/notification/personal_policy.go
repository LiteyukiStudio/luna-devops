package notification

import (
	"context"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const (
	PersonalRecipientUnavailableCode  = "notification.personal_recipient_unavailable"
	PersonalRecipientNotRelatedCode   = "notification.personal_recipient_not_related"
	PersonalProjectAccessRevokedCode  = "notification.personal_project_access_revoked"
	PersonalEventUnsubscribedCode     = "notification.personal_event_unsubscribed"
	PersonalEmailDisabledCode         = "notification.personal_email_disabled"
	PersonalEventIntegrityInvalidCode = "notification.personal_event_integrity_invalid"
)

type PersonalRecipientPolicy struct {
	User       model.User
	Preference model.UserNotificationPreference
}

func PersonalRecipientUserIDs(actorID string, resourceOwnerUserID string) []string {
	userIDs := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, candidate := range []string{actorID, resourceOwnerUserID} {
		userID := strings.TrimSpace(candidate)
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		userIDs = append(userIDs, userID)
	}
	return userIDs
}

func LoadPersonalRecipientPolicy(ctx context.Context, db *gorm.DB, recipientUserID string) (PersonalRecipientPolicy, string, error) {
	recipientUserID = strings.TrimSpace(recipientUserID)
	if db == nil || recipientUserID == "" {
		return PersonalRecipientPolicy{}, PersonalRecipientUnavailableCode, nil
	}

	var user model.User
	if err := db.WithContext(ctx).Where("id = ? and disabled = ?", recipientUserID, false).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return PersonalRecipientPolicy{}, PersonalRecipientUnavailableCode, nil
		}
		return PersonalRecipientPolicy{}, "", err
	}

	preference := DefaultUserNotificationPreference(user.ID)
	if err := db.WithContext(ctx).First(&preference, "user_id = ?", user.ID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return PersonalRecipientPolicy{}, "", err
	}
	return PersonalRecipientPolicy{User: user, Preference: preference}, "", nil
}

func (policy PersonalRecipientPolicy) CheckEvent(ctx context.Context, db *gorm.DB, event model.PlatformEvent) (string, error) {
	recipientUserID := strings.TrimSpace(policy.User.ID)
	if recipientUserID == "" || !personalRecipientContains(event, recipientUserID) {
		return PersonalRecipientNotRelatedCode, nil
	}
	if projectID := strings.TrimSpace(event.ProjectID); projectID != "" && !authz.IsPlatformAdmin(policy.User.Role) {
		var count int64
		if err := db.WithContext(ctx).Model(&model.ProjectMember{}).
			Where("project_id = ? and user_id = ?", projectID, recipientUserID).
			Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return PersonalProjectAccessRevokedCode, nil
		}
	}
	if !containsString(decodeStringList(policy.Preference.EventTypesJSON), event.Type) {
		return PersonalEventUnsubscribedCode, nil
	}
	return "", nil
}

func (policy PersonalRecipientPolicy) CheckAdapter(adapterKind string) string {
	if adapterKind != AdapterKindSMTP {
		return ""
	}
	if !policy.Preference.EmailEnabled || strings.TrimSpace(policy.User.Email) == "" {
		return PersonalEmailDisabledCode
	}
	return ""
}

func personalRecipientContains(event model.PlatformEvent, recipientUserID string) bool {
	for _, userID := range PersonalRecipientUserIDs(event.ActorID, event.ResourceOwnerUserID) {
		if userID == recipientUserID {
			return true
		}
	}
	return false
}
