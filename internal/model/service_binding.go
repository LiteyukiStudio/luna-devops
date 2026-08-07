package model

import (
	"encoding/json"
	"strings"
	"time"
)

// SecretMapping 定义跨服务 Secret 引用：将目标部署的一个 K8s Secret key 注入到源部署的环境变量。
// 仅存储 key 名，AI 和前端不接触值；Worker 渲染时解析。
type SecretMapping struct {
	SourceEnvVar    string `json:"sourceEnvVar"`
	TargetSecretKey string `json:"targetSecretKey"`
}

func DecodeSecretMap(raw string) []SecretMapping {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var mappings []SecretMapping
	if err := json.Unmarshal([]byte(raw), &mappings); err != nil {
		return nil
	}
	return mappings
}

func EncodeSecretMap(mappings []SecretMapping) string {
	if len(mappings) == 0 {
		return ""
	}
	data, err := json.Marshal(mappings)
	if err != nil {
		return ""
	}
	return string(data)
}

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
	SecretMap                string    `gorm:"type:text;not null;default:''" json:"credentialMap"`
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
