package model

import "time"

const (
	AccessTokenSourcePersonal   = "personal"
	AccessTokenSourceOAuth      = "oauth"
	AccessTokenSourceKubeconfig = "kubeconfig"
)

// KubeAccessBinding fixes one kubeconfig context to a single project,
// runtime cluster and optional application boundary. It intentionally stores
// no role or permission snapshot; those are re-evaluated for every request.
type KubeAccessBinding struct {
	ID               string    `gorm:"primaryKey" json:"id"`
	AccessTokenID    string    `gorm:"index;not null" json:"credentialId"`
	ProjectID        string    `gorm:"index;not null" json:"projectId"`
	RuntimeClusterID string    `gorm:"index;not null" json:"runtimeClusterId"`
	ApplicationID    *string   `gorm:"index" json:"applicationId,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}
