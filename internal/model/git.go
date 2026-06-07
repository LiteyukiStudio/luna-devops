package model

import (
	"gorm.io/gorm"
	"time"
)

type GitProvider struct {
	ID              string         `gorm:"primaryKey" json:"id"`
	Type            string         `gorm:"not null" json:"type"`
	Name            string         `gorm:"not null" json:"name"`
	BaseURL         string         `gorm:"not null" json:"baseUrl"`
	Scope           string         `gorm:"not null;default:user" json:"scope"`
	OwnerRef        string         `gorm:"index" json:"ownerRef"`
	AuthType        string         `gorm:"not null;default:oauth" json:"authType"`
	ClientID        string         `json:"clientId"`
	ClientSecretRef string         `json:"clientSecretRef"`
	Enabled         bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type GitAccount struct {
	ID              string         `gorm:"primaryKey" json:"id"`
	UserID          string         `gorm:"index;not null" json:"userId"`
	ProviderID      string         `gorm:"index;not null" json:"providerId"`
	Scope           string         `gorm:"not null;default:user" json:"scope"`
	OwnerRef        string         `gorm:"index" json:"ownerRef"`
	ExternalUserID  string         `json:"externalUserId"`
	Username        string         `gorm:"not null" json:"username"`
	AvatarURL       string         `json:"avatarUrl"`
	AccessTokenRef  string         `json:"accessTokenRef"`
	RefreshTokenRef string         `json:"refreshTokenRef"`
	Scopes          string         `json:"scopes"`
	AccessScope     string         `gorm:"not null;default:personal" json:"accessScope"`
	ExpiresAt       *time.Time     `json:"expiresAt"`
	Status          string         `gorm:"not null;default:connected" json:"status"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type RepositoryBinding struct {
	ID            string         `gorm:"primaryKey" json:"id"`
	ProjectID     string         `gorm:"index;not null" json:"projectId"`
	ApplicationID string         `gorm:"index;not null" json:"applicationId"`
	GitProviderID string         `gorm:"index;not null" json:"gitProviderId"`
	GitAccountID  string         `gorm:"index;not null" json:"gitAccountId"`
	Owner         string         `gorm:"not null" json:"owner"`
	Repo          string         `gorm:"not null" json:"repo"`
	CloneURL      string         `gorm:"not null" json:"cloneUrl"`
	DefaultBranch string         `gorm:"not null;default:main" json:"defaultBranch"`
	WebhookStatus string         `gorm:"not null;default:pending" json:"webhookStatus"`
	WebhookID     string         `json:"webhookId"`
	WebhookSecret string         `json:"-"`
	CredentialRef string         `json:"credentialRef"`
	LastEvent     string         `json:"lastEvent"`
	LastCommitSHA string         `json:"lastCommitSha"`
	LastWebhookAt *time.Time     `json:"lastWebhookAt"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}
