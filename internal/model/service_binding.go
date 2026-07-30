package model

import "time"

// ServiceBinding declares a runtime-affecting dependency between two
// deployment targets. It stores only stable platform identifiers and
// non-secret addressing metadata.
type ServiceBinding struct {
	ID                       string    `gorm:"primaryKey" json:"id"`
	ProjectID                string    `gorm:"index;not null" json:"projectId"`
	SourceApplicationID      string    `gorm:"index;not null" json:"sourceApplicationId"`
	SourceDeploymentTargetID string    `gorm:"index;not null" json:"sourceDeploymentTargetId"`
	TargetApplicationID      string    `gorm:"index;not null" json:"targetApplicationId"`
	TargetDeploymentTargetID string    `gorm:"index;not null" json:"targetDeploymentTargetId"`
	TargetPortName           string    `gorm:"not null" json:"targetPortName"`
	TargetPort               int       `gorm:"not null" json:"targetPort"`
	Protocol                 string    `gorm:"not null" json:"protocol"`
	Path                     string    `gorm:"not null;default:''" json:"path"`
	InjectionMode            string    `gorm:"not null" json:"injectionMode"`
	URLEnvVar                string    `gorm:"not null;default:''" json:"urlEnvVar"`
	HostEnvVar               string    `gorm:"not null;default:''" json:"hostEnvVar"`
	PortEnvVar               string    `gorm:"not null;default:''" json:"portEnvVar"`
	Enabled                  bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedBy                string    `gorm:"index;not null" json:"createdBy"`
	CreatedAt                time.Time `json:"createdAt"`
	UpdatedAt                time.Time `json:"updatedAt"`
}

// ProjectTopologyEdge is a display-only dependency declaration. It never
// changes deployment configuration or Kubernetes resources.
type ProjectTopologyEdge struct {
	ID                       string    `gorm:"primaryKey" json:"id"`
	ProjectID                string    `gorm:"index;not null" json:"projectId"`
	SourceApplicationID      string    `gorm:"index;not null" json:"sourceApplicationId"`
	SourceDeploymentTargetID string    `gorm:"index;not null;default:''" json:"sourceDeploymentTargetId"`
	TargetApplicationID      string    `gorm:"index;not null" json:"targetApplicationId"`
	TargetDeploymentTargetID string    `gorm:"index;not null;default:''" json:"targetDeploymentTargetId"`
	RelationType             string    `gorm:"index;not null" json:"relationType"`
	Protocol                 string    `gorm:"not null;default:''" json:"protocol"`
	Port                     int       `gorm:"not null;default:0" json:"port"`
	Description              string    `gorm:"type:text;not null;default:''" json:"description"`
	CreatedBy                string    `gorm:"index;not null" json:"createdBy"`
	CreatedAt                time.Time `json:"createdAt"`
	UpdatedAt                time.Time `json:"updatedAt"`
}
