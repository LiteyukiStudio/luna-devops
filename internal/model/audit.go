package model

import "time"

type AuditLog struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"index" json:"userId"`
	Action    string    `gorm:"index;not null" json:"action"`
	Resource  string    `gorm:"index;not null" json:"resource"`
	Success   bool      `gorm:"not null" json:"success"`
	Message   string    `json:"message"`
	Metadata  *string   `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
