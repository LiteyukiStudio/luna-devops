package model

import (
	"strings"
	"time"
)

const (
	PlatformSystemProjectKey                 = "platform"
	GatewayTrafficProbeApplicationSlug       = "gateway-traffic-probe"
	GatewayTrafficProbeServiceAccountName    = "liteyuki-gateway-traffic-probe"
	GatewayTrafficProbeAutomountServiceToken = "true"
)

type SystemComponentInstallation struct {
	ID                 string     `gorm:"primaryKey" json:"id"`
	ComponentID        string     `gorm:"uniqueIndex:idx_system_component_cluster;index;not null" json:"componentId"`
	ComponentVersion   string     `gorm:"not null;default:''" json:"componentVersion"`
	RuntimeClusterID   string     `gorm:"uniqueIndex:idx_system_component_cluster;index;not null" json:"runtimeClusterId"`
	ProjectID          string     `gorm:"index;not null;default:''" json:"projectId"`
	ApplicationID      string     `gorm:"index;not null;default:''" json:"applicationId"`
	DeploymentTargetID string     `gorm:"index;not null;default:''" json:"deploymentTargetId"`
	ReleaseID          string     `gorm:"index;not null;default:''" json:"releaseId"`
	Namespace          string     `gorm:"index;not null;default:'liteyuki-system'" json:"namespace"`
	Status             string     `gorm:"index;not null;default:pending" json:"status"`
	Message            string     `gorm:"type:text;not null;default:''" json:"message"`
	ControllerType     string     `gorm:"index;not null;default:''" json:"controllerType"`
	Mode               string     `gorm:"not null;default:''" json:"mode"`
	Config             string     `gorm:"type:text;not null;default:'{}'" json:"config"`
	ReportTokenHash    string     `gorm:"not null;default:''" json:"-"`
	LastReportedAt     *time.Time `json:"lastReportedAt"`
	LastWindowStart    *time.Time `json:"lastWindowStart"`
	LastWindowEnd      *time.Time `json:"lastWindowEnd"`
	LastError          string     `gorm:"type:text;not null;default:''" json:"lastError"`
	InstalledBy        string     `gorm:"index;not null;default:''" json:"installedBy"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func IsGatewayTrafficProbeApplication(project Project, application Application) bool {
	return strings.TrimSpace(project.SystemKey) == PlatformSystemProjectKey &&
		strings.TrimSpace(application.Slug) == GatewayTrafficProbeApplicationSlug
}

func ApplyPlatformDeploymentTargetDefaults(project Project, application Application, target DeploymentTarget) DeploymentTarget {
	if IsGatewayTrafficProbeApplication(project, application) {
		target.ServiceAccountName = GatewayTrafficProbeServiceAccountName
		target.AutomountServiceAccountToken = GatewayTrafficProbeAutomountServiceToken
	}
	return target
}
