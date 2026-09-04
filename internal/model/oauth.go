package model

import "time"

type OAuthApplication struct {
	ID                      string     `gorm:"primaryKey" json:"id"`
	OwnerUserID             *string    `gorm:"index" json:"ownerUserId,omitempty"`
	Name                    string     `gorm:"not null" json:"name"`
	Description             string     `json:"description"`
	HomepageURL             string     `json:"homepageUrl"`
	LogoURL                 string     `json:"logoUrl"`
	ClientID                string     `gorm:"uniqueIndex;not null" json:"clientId"`
	ClientSecretHash        string     `gorm:"not null" json:"-"`
	RedirectURIs            string     `gorm:"type:text;not null" json:"-"`
	AllowedScopes           string     `gorm:"type:text;not null" json:"allowedScopes"`
	AccessTokenLifetimeDays int        `gorm:"not null;default:30" json:"accessTokenLifetimeDays"`
	RevokedAt               *time.Time `gorm:"index" json:"revokedAt"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}

func (OAuthApplication) TableName() string { return "oauth_applications" }

type OAuthGrant struct {
	ID            string     `gorm:"primaryKey" json:"id"`
	ApplicationID string     `gorm:"index;not null" json:"applicationId"`
	UserID        string     `gorm:"index;not null" json:"userId"`
	Scope         string     `gorm:"type:text;not null" json:"scope"`
	RevokedAt     *time.Time `gorm:"index" json:"revokedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (OAuthGrant) TableName() string { return "oauth_grants" }

type OAuthAuthorizationCode struct {
	ID                  string     `gorm:"primaryKey" json:"id"`
	ApplicationID       string     `gorm:"index;not null" json:"applicationId"`
	GrantID             *string    `gorm:"index" json:"grantId,omitempty"`
	UserID              string     `gorm:"index;not null" json:"userId"`
	CodeHash            string     `gorm:"uniqueIndex;not null" json:"-"`
	RedirectURI         string     `gorm:"type:text;not null" json:"-"`
	Scope               string     `gorm:"type:text;not null" json:"scope"`
	CodeChallenge       string     `gorm:"not null" json:"-"`
	CodeChallengeMethod string     `gorm:"not null" json:"-"`
	ExpiresAt           time.Time  `gorm:"index;not null" json:"expiresAt"`
	ConsumedAt          *time.Time `gorm:"index" json:"-"`
	CreatedAt           time.Time  `json:"createdAt"`
}

func (OAuthAuthorizationCode) TableName() string { return "oauth_authorization_codes" }

type OAuthRefreshToken struct {
	ID            string     `gorm:"primaryKey" json:"id"`
	ApplicationID string     `gorm:"index;not null" json:"applicationId"`
	GrantID       string     `gorm:"index;not null" json:"grantId"`
	FamilyID      string     `gorm:"index;not null;default:''" json:"-"`
	UserID        string     `gorm:"index;not null" json:"userId"`
	TokenHash     string     `gorm:"uniqueIndex;not null" json:"-"`
	Scope         string     `gorm:"type:text;not null" json:"scope"`
	ExpiresAt     time.Time  `gorm:"index;not null" json:"expiresAt"`
	ConsumedAt    *time.Time `gorm:"index" json:"-"`
	RevokedAt     *time.Time `gorm:"index" json:"-"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (OAuthRefreshToken) TableName() string { return "oauth_refresh_tokens" }

type OAuthDeviceAuthorization struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	ApplicationID   string     `gorm:"index;not null" json:"applicationId"`
	GrantID         *string    `gorm:"index" json:"grantId,omitempty"`
	UserID          *string    `gorm:"index" json:"userId,omitempty"`
	DeviceCodeHash  string     `gorm:"uniqueIndex;not null" json:"-"`
	UserCodeHash    string     `gorm:"uniqueIndex;not null" json:"-"`
	Status          string     `gorm:"index;not null" json:"status"`
	IntervalSeconds int        `gorm:"not null" json:"intervalSeconds"`
	LastPolledAt    *time.Time `json:"lastPolledAt,omitempty"`
	ExpiresAt       time.Time  `gorm:"index;not null" json:"expiresAt"`
	ApprovedAt      *time.Time `json:"approvedAt,omitempty"`
	DeniedAt        *time.Time `json:"deniedAt,omitempty"`
	ConsumedAt      *time.Time `gorm:"index" json:"consumedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func (OAuthDeviceAuthorization) TableName() string { return "oauth_device_authorizations" }
