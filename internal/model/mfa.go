package model

import "time"

type UserMFAConfig struct {
	ID                       string     `gorm:"primaryKey" json:"id"`
	UserID                   string     `gorm:"uniqueIndex;not null" json:"userId"`
	TOTPSecretRef            string     `gorm:"not null" json:"-"`
	Enabled                  bool       `gorm:"index;not null;default:false" json:"enabled"`
	ConfirmedAt              *time.Time `json:"confirmedAt"`
	RecoveryCodesGeneratedAt *time.Time `json:"recoveryCodesGeneratedAt"`
	LastTOTPCounter          *int64     `json:"-"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

type MFARecoveryCode struct {
	ID        string     `gorm:"primaryKey" json:"id"`
	UserID    string     `gorm:"index;not null" json:"userId"`
	CodeHash  string     `gorm:"not null" json:"-"`
	UsedAt    *time.Time `gorm:"index" json:"usedAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

type StepUpAssertion struct {
	ID                string    `gorm:"primaryKey" json:"id"`
	UserID            string    `gorm:"index;not null" json:"userId"`
	SessionID         string    `gorm:"index;uniqueIndex:idx_step_up_assertions_session_purpose;not null" json:"sessionId"`
	Purpose           string    `gorm:"index;uniqueIndex:idx_step_up_assertions_session_purpose;not null" json:"purpose"`
	VerifiedAt        time.Time `gorm:"not null" json:"verifiedAt"`
	LastActivityAt    time.Time `gorm:"index;not null" json:"lastActivityAt"`
	IdleExpiresAt     time.Time `gorm:"index;not null" json:"idleExpiresAt"`
	AbsoluteExpiresAt time.Time `gorm:"index;not null" json:"absoluteExpiresAt"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
