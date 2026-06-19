package model

import (
	"time"

	"github.com/shopspring/decimal"
)

type ProjectWallet struct {
	ID             string          `gorm:"primaryKey" json:"id"`
	ProjectID      string          `gorm:"uniqueIndex;not null" json:"projectId"`
	BalanceCredits decimal.Decimal `gorm:"type:numeric(24,8);not null;default:0" json:"balanceCredits"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type BillingRateRule struct {
	ID             string          `gorm:"primaryKey" json:"id"`
	Meter          string          `gorm:"uniqueIndex;not null" json:"meter"`
	Unit           string          `gorm:"not null" json:"unit"`
	CreditsPerUnit decimal.Decimal `gorm:"type:numeric(24,8);not null;default:0" json:"creditsPerUnit"`
	Enabled        bool            `gorm:"not null;default:true" json:"enabled"`
	Description    string          `json:"description"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type BillingUsageRecord struct {
	ID            string          `gorm:"primaryKey" json:"id"`
	ProjectID     string          `gorm:"index;not null" json:"projectId"`
	ApplicationID string          `gorm:"index" json:"applicationId"`
	Meter         string          `gorm:"uniqueIndex:idx_billing_usage_resource_meter;index;not null" json:"meter"`
	Quantity      decimal.Decimal `gorm:"type:numeric(24,8);not null;default:0" json:"quantity"`
	Unit          string          `gorm:"not null" json:"unit"`
	AmountCredits decimal.Decimal `gorm:"type:numeric(24,8);not null;default:0" json:"amountCredits"`
	ResourceType  string          `gorm:"uniqueIndex:idx_billing_usage_resource_meter;index;not null" json:"resourceType"`
	ResourceID    string          `gorm:"uniqueIndex:idx_billing_usage_resource_meter;index;not null" json:"resourceId"`
	PeriodStart   time.Time       `gorm:"index;not null" json:"periodStart"`
	PeriodEnd     time.Time       `gorm:"index;not null" json:"periodEnd"`
	Status        string          `gorm:"index;not null;default:settled" json:"status"`
	Metadata      string          `gorm:"type:text;not null;default:''" json:"metadata"`
	SettledAt     *time.Time      `json:"settledAt"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type BillingLedgerEntry struct {
	ID                  string          `gorm:"primaryKey" json:"id"`
	ProjectID           string          `gorm:"index;not null" json:"projectId"`
	Type                string          `gorm:"index;not null" json:"type"`
	AmountCredits       decimal.Decimal `gorm:"type:numeric(24,8);not null;default:0" json:"amountCredits"`
	BalanceAfterCredits decimal.Decimal `gorm:"type:numeric(24,8);not null;default:0" json:"balanceAfterCredits"`
	Reason              string          `gorm:"index;not null" json:"reason"`
	Meter               string          `gorm:"index" json:"meter"`
	UsageRecordID       string          `gorm:"index" json:"usageRecordId"`
	ResourceType        string          `gorm:"index" json:"resourceType"`
	ResourceID          string          `gorm:"index" json:"resourceId"`
	IdempotencyKey      string          `gorm:"index;not null;default:''" json:"idempotencyKey"`
	Description         string          `json:"description"`
	CreatedBy           string          `gorm:"index" json:"createdBy"`
	CreatedAt           time.Time       `json:"createdAt"`
}
