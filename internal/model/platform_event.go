package model

import "time"

type PlatformEvent struct {
	ID                       string    `gorm:"primaryKey" json:"id"`
	Type                     string    `gorm:"index;not null" json:"type"`
	Category                 string    `gorm:"index;not null" json:"category"`
	Severity                 string    `gorm:"index;not null" json:"severity"`
	Status                   string    `gorm:"index;not null" json:"status"`
	ProjectID                string    `gorm:"index;not null;default:''" json:"projectId"`
	ApplicationID            string    `gorm:"index;not null;default:''" json:"applicationId"`
	DeploymentTargetID       string    `gorm:"index;not null;default:''" json:"deploymentTargetId"`
	ResourceType             string    `gorm:"index;not null;default:''" json:"resourceType"`
	ResourceID               string    `gorm:"index;not null;default:''" json:"resourceId"`
	ActorID                  string    `gorm:"index;not null;default:''" json:"actorId"`
	ResourceOwnerUserID      string    `gorm:"column:resource_owner_user_id;index;not null;default:''" json:"resourceOwnerUserId"`
	SummaryKey               string    `gorm:"not null;default:''" json:"summaryKey"`
	Message                  string    `gorm:"type:text;not null;default:''" json:"message"`
	DetailJSON               string    `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	LinksJSON                string    `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	CorrelationID            string    `gorm:"index;not null;default:''" json:"correlationId"`
	TraceID                  string    `gorm:"index;not null;default:''" json:"traceId"`
	NotificationFanoutStatus string    `gorm:"column:notification_fanout_status;index;not null;default:'';check:chk_platform_events_notification_fanout_status,notification_fanout_status IN ('','pending','completed')" json:"-"`
	FanoutTraceparent        string    `gorm:"column:fanout_traceparent;type:text;not null;default:''" json:"-"`
	FanoutTracestate         string    `gorm:"column:fanout_tracestate;type:text;not null;default:''" json:"-"`
	DedupKey                 *string   `gorm:"uniqueIndex" json:"-"`
	OccurredAt               time.Time `gorm:"index;not null" json:"occurredAt"`
	CreatedAt                time.Time `gorm:"index;not null" json:"createdAt"`
}
