package model

import (
	"time"

	"gorm.io/gorm"
)

type AppTemplateInstallation struct {
	ID                 string         `gorm:"primaryKey" json:"id"`
	TemplateID         string         `gorm:"index;not null" json:"templateId"`
	TemplateVersion    string         `gorm:"not null;default:''" json:"templateVersion"`
	ProjectID          string         `gorm:"index;not null" json:"projectId"`
	ApplicationID      string         `gorm:"index;not null" json:"applicationId"`
	DeploymentTargetID string         `gorm:"index;not null;default:''" json:"deploymentTargetId"`
	ReleaseID          string         `gorm:"index;not null;default:''" json:"releaseId"`
	ValuesSnapshot     string         `gorm:"type:text;not null;default:'{}'" json:"valuesSnapshot"`
	CreatedBy          string         `gorm:"index;not null;default:''" json:"createdBy"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

// AppTemplateInstallationComponent 记录多组件模板一次安装中每个组件创建的应用、部署配置和首个 Release。
type AppTemplateInstallationComponent struct {
	ID                 string    `gorm:"primaryKey" json:"id"`
	InstallationID     string    `gorm:"not null;uniqueIndex:idx_app_template_installation_components_component,priority:1" json:"installationId"`
	ComponentKey       string    `gorm:"not null;uniqueIndex:idx_app_template_installation_components_component,priority:2" json:"componentKey"`
	ProjectID          string    `gorm:"index;not null" json:"projectId"`
	ApplicationID      string    `gorm:"index;not null" json:"applicationId"`
	DeploymentTargetID string    `gorm:"not null" json:"deploymentTargetId"`
	ReleaseID          string    `gorm:"not null;default:''" json:"releaseId"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// PlatformAppTemplate 保存平台管理员维护的平台级应用商城模板，spec_json 与内置模板运行规格同构。
type PlatformAppTemplate struct {
	ID               string    `gorm:"primaryKey" json:"id"`
	Slug             string    `gorm:"not null;uniqueIndex:idx_platform_app_templates_slug" json:"slug"`
	Name             string    `gorm:"not null" json:"name"`
	Description      string    `gorm:"type:text;not null;default:''" json:"description"`
	Category         string    `gorm:"not null;index:idx_platform_app_templates_enabled_category,priority:2" json:"category"`
	Icon             string    `gorm:"not null;default:''" json:"icon"`
	Version          string    `gorm:"not null;default:''" json:"version"`
	IsMultiComponent bool      `gorm:"not null;default:false" json:"isMultiComponent"`
	Enabled          bool      `gorm:"not null;default:true;index:idx_platform_app_templates_enabled_category,priority:1" json:"enabled"`
	Featured         bool      `gorm:"not null;default:false" json:"featured"`
	PopularityWeight int       `gorm:"not null;default:0" json:"popularityWeight"`
	SpecJSON         string    `gorm:"column:spec_json;type:jsonb;not null;default:'{}'" json:"specJson"`
	CreatedBy        string    `gorm:"index;not null;default:''" json:"createdBy"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
