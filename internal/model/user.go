package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID               string         `gorm:"primaryKey" json:"id"`
	Email            string         `gorm:"uniqueIndex;not null" json:"email"`
	Name             string         `gorm:"not null" json:"name"`
	AvatarURL        string         `json:"avatarUrl"`
	Role             string         `gorm:"not null;default:user" json:"role"`
	Language         string         `gorm:"not null;default:zh-CN" json:"language"`
	BrandColorPreset string         `gorm:"not null;default:''" json:"brandColorPreset"`
	Password         string         `json:"-"`
	Disabled         bool           `gorm:"not null;default:false" json:"disabled"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

type AuthRegistrationSettings struct {
	ID                            string    `gorm:"primaryKey" json:"id"`
	AllowEmailRegistration        bool      `gorm:"not null;default:false" json:"allowEmailRegistration"`
	AllowOIDCRegistration         bool      `gorm:"column:allow_oidc_registration;not null;default:true" json:"allowOidcRegistration"`
	AllowExternalIdentityPassword bool      `gorm:"not null;default:false" json:"allowExternalIdentityPassword"`
	SMTPHost                      string    `gorm:"not null;default:''" json:"smtpHost"`
	SMTPPort                      int       `gorm:"not null;default:587" json:"smtpPort"`
	SMTPSecurity                  string    `gorm:"not null;default:starttls" json:"smtpSecurity"`
	SMTPUsername                  string    `gorm:"not null;default:''" json:"smtpUsername"`
	SMTPPasswordRef               string    `gorm:"not null;default:''" json:"-"`
	SMTPFromAddress               string    `gorm:"not null;default:''" json:"smtpFromAddress"`
	SMTPFromName                  string    `gorm:"not null;default:'Luna DevOps'" json:"smtpFromName"`
	CreatedAt                     time.Time `json:"createdAt"`
	UpdatedAt                     time.Time `json:"updatedAt"`
}

type EmailRegistrationChallenge struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	Email      string     `gorm:"index;not null" json:"email"`
	CodeHash   string     `gorm:"not null" json:"-"`
	Language   string     `gorm:"not null;default:zh-CN" json:"language"`
	Attempts   int        `gorm:"not null;default:0" json:"attempts"`
	ExpiresAt  time.Time  `gorm:"index;not null" json:"expiresAt"`
	ConsumedAt *time.Time `gorm:"index" json:"-"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type UserSession struct {
	ID                     string     `gorm:"primaryKey" json:"id"`
	UserID                 string     `gorm:"index;not null" json:"userId"`
	ImpersonatorID         string     `gorm:"index" json:"impersonatorId"`
	RememberFamilyID       string     `gorm:"index;not null;default:''" json:"-"`
	PrimaryAuthenticatedAt *time.Time `gorm:"index" json:"-"`
	TokenHash              string     `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt              time.Time  `gorm:"index;not null" json:"expiresAt"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

type UserRememberToken struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	UserID     string     `gorm:"index;not null" json:"userId"`
	FamilyID   string     `gorm:"index;not null" json:"-"`
	TokenHash  string     `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt  time.Time  `gorm:"index;not null" json:"expiresAt"`
	ConsumedAt *time.Time `gorm:"index" json:"-"`
	RevokedAt  *time.Time `gorm:"index" json:"-"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type AccessToken struct {
	ID                 string     `gorm:"primaryKey" json:"id"`
	UserID             string     `gorm:"index" json:"userId"`
	Name               string     `gorm:"not null" json:"name"`
	Scope              string     `gorm:"not null" json:"scope"`
	TokenHash          string     `gorm:"uniqueIndex;not null" json:"-"`
	Source             string     `gorm:"index;not null;default:personal" json:"source"`
	OAuthApplicationID string     `gorm:"column:oauth_application_id;index" json:"oauthApplicationId,omitempty"`
	OAuthGrantID       string     `gorm:"column:oauth_grant_id;index" json:"oauthGrantId,omitempty"`
	ExpiresAt          *time.Time `json:"expiresAt"`
	RevokedAt          *time.Time `json:"revokedAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}
