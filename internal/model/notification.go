package model

import (
	"time"

	"gorm.io/gorm"
)

type NotificationChannel struct {
	ID                 string         `gorm:"primaryKey" json:"id"`
	ProjectID          string         `gorm:"index;not null;default:''" json:"projectId"`
	OwnerUserID        string         `gorm:"index;not null;default:''" json:"ownerUserId"`
	Name               string         `gorm:"not null" json:"name"`
	AdapterKind        string         `gorm:"index;not null" json:"adapterKind"`
	ConfigJSON         string         `gorm:"type:jsonb;not null;default:'{}'" json:"configJson"`
	SecretRefsJSON     string         `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	Enabled            bool           `gorm:"not null;default:true" json:"enabled"`
	LastDeliveryStatus string         `gorm:"-" json:"lastDeliveryStatus"`
	LastDeliveryError  string         `gorm:"-" json:"lastDeliveryError"`
	LastDeliveredAt    *time.Time     `gorm:"-" json:"lastDeliveredAt"`
	CreatedBy          string         `json:"createdBy"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

type NotificationTemplate struct {
	ID               string         `gorm:"primaryKey" json:"id"`
	ProjectID        string         `gorm:"index;not null;default:''" json:"projectId"`
	Name             string         `gorm:"not null" json:"name"`
	EventType        string         `gorm:"index;not null" json:"eventType"`
	AdapterKind      string         `gorm:"index;not null" json:"adapterKind"`
	Locale           string         `gorm:"index;not null;default:''" json:"locale"`
	SubjectTemplate  string         `gorm:"type:text;not null;default:''" json:"subjectTemplate"`
	BodyTemplate     string         `gorm:"type:text;not null;default:''" json:"bodyTemplate"`
	JSONBodyTemplate string         `gorm:"type:text;not null;default:''" json:"jsonBodyTemplate"`
	Enabled          bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedBy        string         `json:"createdBy"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

type NotificationRule struct {
	ID             string         `gorm:"primaryKey" json:"id"`
	ProjectID      string         `gorm:"index;not null;default:''" json:"projectId"`
	Name           string         `gorm:"not null" json:"name"`
	EventTypesJSON string         `gorm:"type:jsonb;not null;default:'[]'" json:"eventTypesJson"`
	FilterJSON     string         `gorm:"type:jsonb;not null;default:'{}'" json:"filterJson"`
	ChannelIDsJSON string         `gorm:"type:jsonb;not null;default:'[]'" json:"channelIdsJson"`
	TemplateID     string         `gorm:"index;not null;default:''" json:"templateId"`
	Locale         string         `gorm:"not null;default:''" json:"locale"`
	Enabled        bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedBy      string         `json:"createdBy"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type NotificationDelivery struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	ProjectID       string     `gorm:"index;not null;default:''" json:"projectId"`
	RecipientUserID string     `gorm:"index;not null;default:'';uniqueIndex:idx_notification_deliveries_event_channel_recipient" json:"recipientUserId"`
	EventID         string     `gorm:"index;not null;uniqueIndex:idx_notification_deliveries_event_channel_recipient" json:"eventId"`
	EventType       string     `gorm:"index;not null" json:"eventType"`
	Severity        string     `gorm:"index;not null;default:''" json:"severity"`
	ChannelID       string     `gorm:"index;not null;uniqueIndex:idx_notification_deliveries_event_channel_recipient" json:"channelId"`
	AdapterKind     string     `gorm:"index;not null" json:"adapterKind"`
	RuleID          string     `gorm:"index;not null;default:''" json:"ruleId"`
	TemplateID      string     `gorm:"index;not null;default:''" json:"templateId"`
	EventJSON       string     `gorm:"type:jsonb;not null;default:'{}'" json:"eventJson"`
	Status          string     `gorm:"index;not null;default:pending" json:"status"`
	AttemptCount    int        `gorm:"not null;default:0" json:"attemptCount"`
	DurationMillis  int64      `gorm:"not null;default:0" json:"durationMillis"`
	ErrorMessage    string     `gorm:"type:text;not null;default:''" json:"errorMessage"`
	RequestSnapshot string     `gorm:"type:jsonb;not null;default:'{}'" json:"requestSnapshot"`
	ResponseSnippet string     `gorm:"type:text;not null;default:''" json:"responseSnippet"`
	QueuedAt        time.Time  `json:"queuedAt"`
	StartedAt       *time.Time `json:"startedAt"`
	FinishedAt      *time.Time `json:"finishedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type UserNotificationPreference struct {
	UserID         string    `gorm:"primaryKey" json:"userId"`
	EmailEnabled   bool      `gorm:"not null;default:true" json:"emailEnabled"`
	EventTypesJSON string    `gorm:"type:jsonb;not null;default:'[\"build.failed\",\"release.failed\",\"hook.failed\",\"gateway.apply_failed\",\"certificate.failed\",\"certificate.expired\"]'" json:"eventTypesJson"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
