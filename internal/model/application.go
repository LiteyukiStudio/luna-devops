package model

import (
	"time"

	"gorm.io/gorm"
)

type Application struct {
	ID               string         `gorm:"primaryKey" json:"id"`
	ProjectID        string         `gorm:"uniqueIndex:idx_applications_project_identifier_active,where:deleted_at IS NULL;index;not null" json:"projectId"`
	Identifier       string         `gorm:"column:identifier;uniqueIndex:idx_applications_project_identifier_active,where:deleted_at IS NULL;index;not null" json:"identifier"`
	Name             string         `gorm:"not null" json:"name"`
	Icon             string         `gorm:"not null;default:'box'" json:"icon"`
	DeleteStatus     string         `gorm:"index;not null;default:active" json:"deleteStatus"`
	DeleteMessage    string         `gorm:"type:text;not null;default:''" json:"deleteMessage"`
	DeleteStartedAt  *time.Time     `json:"deleteStartedAt"`
	DeleteFinishedAt *time.Time     `json:"deleteFinishedAt"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

type AppConfig struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `gorm:"not null" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}
