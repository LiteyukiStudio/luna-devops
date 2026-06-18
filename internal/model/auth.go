package model

import (
	"gorm.io/gorm"
	"time"
)

type AuthProvider struct {
	ID              string         `gorm:"primaryKey" json:"id"`
	Type            string         `gorm:"not null" json:"type"`
	Name            string         `gorm:"not null" json:"name"`
	Enabled         bool           `gorm:"not null;default:true" json:"enabled"`
	IssuerURL       string         `gorm:"not null" json:"issuerUrl"`
	ClientID        string         `gorm:"not null" json:"clientId"`
	ClientSecretRef string         `json:"clientSecretRef"`
	Scopes          string         `gorm:"not null;default:openid profile email" json:"scopes"`
	GroupClaim      string         `gorm:"not null;default:groups" json:"groupClaim"`
	EmailClaim      string         `gorm:"not null;default:email" json:"emailClaim"`
	UsernameClaim   string         `gorm:"not null;default:preferred_username" json:"usernameClaim"`
	IsDefault       bool           `gorm:"not null;default:false" json:"isDefault"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type ExternalIdentity struct {
	ID            string     `gorm:"primaryKey" json:"id"`
	UserID        string     `gorm:"index:idx_external_identities_user_provider,unique;not null" json:"userId"`
	ProviderID    string     `gorm:"index:idx_external_identities_provider_subject,unique;index:idx_external_identities_user_provider,unique;not null" json:"providerId"`
	Subject       string     `gorm:"index:idx_external_identities_provider_subject,unique;not null" json:"subject"`
	Email         string     `json:"email"`
	EmailVerified bool       `gorm:"not null;default:false" json:"emailVerified"`
	Username      string     `json:"username"`
	LastLoginAt   *time.Time `json:"lastLoginAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type AuthAdmissionPolicy struct {
	ID                       string    `gorm:"primaryKey" json:"id"`
	AllowLocalLogin          bool      `gorm:"not null;default:true" json:"allowLocalLogin"`
	AllowOIDCLogin           bool      `gorm:"not null;default:true" json:"allowOidcLogin"`
	RequireVerifiedOIDCEmail bool      `gorm:"not null;default:true" json:"requireVerifiedOidcEmail"`
	AllowedEmailDomains      string    `json:"allowedEmailDomains"`
	AllowedOIDCGroups        string    `json:"allowedOidcGroups"`
	InvitedEmails            string    `json:"invitedEmails"`
	DefaultRole              string    `gorm:"not null;default:user" json:"defaultRole"`
	CreatedAt                time.Time `json:"createdAt"`
	UpdatedAt                time.Time `json:"updatedAt"`
}
