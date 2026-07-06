package model

import (
	"gorm.io/gorm"
	"time"
)

type Project struct {
	ID                  string         `gorm:"primaryKey" json:"id"`
	Slug                string         `gorm:"uniqueIndex:idx_projects_slug_active,where:deleted_at IS NULL;not null" json:"slug"`
	Name                string         `gorm:"not null" json:"name"`
	Description         string         `json:"description"`
	NamespaceStrategy   string         `gorm:"not null" json:"namespaceStrategy"`
	MaxConcurrentBuilds int            `gorm:"not null;default:2" json:"maxConcurrentBuilds"`
	BillingOwnerUserID  string         `gorm:"index;not null;default:''" json:"billingOwnerUserId"`
	SystemKey           string         `gorm:"index;not null;default:''" json:"systemKey"`
	DeleteStatus        string         `gorm:"index;not null;default:active" json:"deleteStatus"`
	DeleteMessage       string         `gorm:"type:text;not null;default:''" json:"deleteMessage"`
	DeleteStartedAt     *time.Time     `json:"deleteStartedAt"`
	DeleteFinishedAt    *time.Time     `json:"deleteFinishedAt"`
	DashboardOrder      int            `gorm:"->;column:dashboard_order;-:migration" json:"dashboardOrder"`
	LastUsedAt          *time.Time     `gorm:"->;column:last_used_at;-:migration" json:"lastUsedAt"`
	UseCount            int            `gorm:"->;column:use_count;-:migration" json:"useCount"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

type ProjectMember struct {
	ID             string     `gorm:"primaryKey" json:"id"`
	ProjectID      string     `gorm:"index;not null" json:"projectId"`
	UserID         string     `gorm:"index;not null" json:"userId"`
	Role           string     `gorm:"not null" json:"role"`
	DashboardOrder int        `gorm:"not null;default:0" json:"dashboardOrder"`
	LastUsedAt     *time.Time `gorm:"index" json:"lastUsedAt"`
	UseCount       int        `gorm:"not null;default:0" json:"useCount"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type ProjectPin struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"uniqueIndex:idx_project_pins_user_project;index;not null" json:"userId"`
	ProjectID string    `gorm:"uniqueIndex:idx_project_pins_user_project;index;not null" json:"projectId"`
	PinnedAt  time.Time `gorm:"index;not null" json:"pinnedAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ProjectHookConfig struct {
	ID             string         `gorm:"primaryKey" json:"id"`
	ProjectID      string         `gorm:"index;not null" json:"projectId"`
	Name           string         `gorm:"not null" json:"name"`
	Script         string         `gorm:"type:text;not null" json:"script"`
	Shell          string         `gorm:"not null;default:sh" json:"shell"`
	TimeoutSeconds int            `gorm:"not null;default:300" json:"timeoutSeconds"`
	FailurePolicy  string         `gorm:"not null;default:fail" json:"failurePolicy"`
	CreatedBy      string         `gorm:"index" json:"createdBy"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type HookRun struct {
	ID                 string     `gorm:"primaryKey" json:"id"`
	ProjectID          string     `gorm:"index;not null" json:"projectId"`
	HookConfigID       string     `gorm:"index" json:"hookConfigId"`
	BuildRunID         string     `gorm:"index" json:"buildRunId"`
	BuildJobID         string     `gorm:"index" json:"buildJobId"`
	ReleaseID          string     `gorm:"index" json:"releaseId"`
	ApplicationID      string     `gorm:"index" json:"applicationId"`
	EnvironmentID      string     `gorm:"index" json:"environmentId"`
	DeploymentTargetID string     `gorm:"index" json:"deploymentTargetId"`
	Name               string     `gorm:"not null" json:"name"`
	Phase              string     `gorm:"index;not null" json:"phase"`
	Status             string     `gorm:"index;not null;default:queued" json:"status"`
	ScriptSnapshot     string     `gorm:"type:text;not null" json:"scriptSnapshot"`
	Shell              string     `gorm:"not null;default:sh" json:"shell"`
	ImageRef           string     `json:"imageRef"`
	TimeoutSeconds     int        `gorm:"not null;default:300" json:"timeoutSeconds"`
	FailurePolicy      string     `gorm:"not null;default:fail" json:"failurePolicy"`
	ExitCode           int        `json:"exitCode"`
	Message            string     `json:"message"`
	StartedAt          *time.Time `json:"startedAt"`
	FinishedAt         *time.Time `json:"finishedAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type HookRunLog struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	HookRunID string    `gorm:"uniqueIndex;not null" json:"hookRunId"`
	ProjectID string    `gorm:"index;not null" json:"projectId"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
