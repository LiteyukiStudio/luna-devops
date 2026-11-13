package model

import "time"

type RuntimeAlertIncident struct {
	ID                    string     `gorm:"primaryKey" json:"id"`
	ProjectID             string     `gorm:"index:idx_runtime_alert_incidents_project_status;not null" json:"projectId"`
	ApplicationID         string     `gorm:"not null;default:''" json:"applicationId"`
	DeploymentTargetID    string     `gorm:"index;not null;default:''" json:"deploymentTargetId"`
	ResourceID            string     `gorm:"index;not null;default:''" json:"resourceId"`
	IncidentKind          string     `gorm:"not null" json:"incidentKind"`
	Status                string     `gorm:"index:idx_runtime_alert_incidents_project_status;not null;default:'active'" json:"status"`
	Fingerprint           string     `gorm:"not null;default:''" json:"fingerprint"`
	ConsecutiveFailures   int        `gorm:"not null;default:1" json:"consecutiveFailures"`
	FirstDetectedAt       time.Time  `gorm:"not null" json:"firstDetectedAt"`
	LastObservedAt        time.Time  `gorm:"not null" json:"lastObservedAt"`
	LastNotifiedAt        *time.Time `json:"lastNotifiedAt"`
	NotificationCount     int        `gorm:"not null;default:0" json:"notificationCount"`
	RuleNotificationsJSON string     `gorm:"type:jsonb;not null;default:'{}'" json:"ruleNotificationsJson"`
	LastEventID           string     `gorm:"not null;default:''" json:"lastEventId"`
	InboxCoalesceKey      string     `gorm:"not null;default:''" json:"inboxCoalesceKey"`
	ResolvedAt            *time.Time `json:"resolvedAt"`
	SummaryJSON           string     `gorm:"type:jsonb;not null;default:'{}'" json:"summaryJson"`
	CreatedAt             time.Time  `gorm:"not null" json:"createdAt"`
	UpdatedAt             time.Time  `gorm:"not null" json:"updatedAt"`
}
