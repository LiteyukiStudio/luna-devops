package model

import (
	"gorm.io/gorm"
	"time"
)

type Project struct {
	ID                string         `gorm:"primaryKey" json:"id"`
	Slug              string         `gorm:"uniqueIndex:idx_projects_slug_active,where:deleted_at IS NULL;not null" json:"slug"`
	Name              string         `gorm:"not null" json:"name"`
	Description       string         `json:"description"`
	NamespaceStrategy string         `gorm:"not null" json:"namespaceStrategy"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

type ProjectMember struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	ProjectID string    `gorm:"index;not null" json:"projectId"`
	UserID    string    `gorm:"index;not null" json:"userId"`
	Role      string    `gorm:"not null" json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ProjectPin struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"uniqueIndex:idx_project_pins_user_project;index;not null" json:"userId"`
	ProjectID string    `gorm:"uniqueIndex:idx_project_pins_user_project;index;not null" json:"projectId"`
	PinnedAt  time.Time `gorm:"index;not null" json:"pinnedAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
