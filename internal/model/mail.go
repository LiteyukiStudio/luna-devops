package model

import "time"

type PlatformMailSettings struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	Host        string    `gorm:"not null;default:''" json:"host"`
	Port        int       `gorm:"not null;default:587" json:"port"`
	Security    string    `gorm:"not null;default:starttls" json:"security"`
	Username    string    `gorm:"not null;default:''" json:"username"`
	PasswordRef string    `gorm:"not null;default:''" json:"-"`
	FromAddress string    `gorm:"not null;default:''" json:"fromAddress"`
	FromName    string    `gorm:"not null;default:'Luna DevOps'" json:"fromName"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
