package model

import (
	"gorm.io/gorm"
	"time"
)

type User struct {
	ID        string         `gorm:"primaryKey" json:"id"`
	Email     string         `gorm:"uniqueIndex;not null" json:"email"`
	Name      string         `gorm:"not null" json:"name"`
	AvatarURL string         `json:"avatarUrl"`
	AuthType  string         `gorm:"not null;default:local" json:"authType"`
	Role      string         `gorm:"not null;default:user" json:"role"`
	Language  string         `gorm:"not null;default:zh-CN" json:"language"`
	Password  string         `json:"-"`
	Disabled  bool           `gorm:"not null;default:false" json:"disabled"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type UserSession struct {
	ID               string    `gorm:"primaryKey" json:"id"`
	UserID           string    `gorm:"index;not null" json:"userId"`
	ImpersonatorID   string    `gorm:"index" json:"impersonatorId"`
	RememberFamilyID string    `gorm:"index;not null;default:''" json:"-"`
	TokenHash        string    `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt        time.Time `gorm:"index;not null" json:"expiresAt"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
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
	ID        string     `gorm:"primaryKey" json:"id"`
	UserID    string     `gorm:"index" json:"userId"`
	Name      string     `gorm:"not null" json:"name"`
	Scope     string     `gorm:"not null" json:"scope"`
	TokenHash string     `gorm:"not null" json:"-"`
	ExpiresAt *time.Time `json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}
