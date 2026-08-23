package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// AIModel is a platform-managed OpenAI-compatible model and its price book.
// Prices are stored in credits per one million tokens.
type AIModel struct {
	ID                           string          `gorm:"primaryKey" json:"id"`
	Name                         string          `gorm:"uniqueIndex;not null" json:"name"`
	MaxContextTokens             int64           `gorm:"column:max_context_tokens;not null" json:"maxContextTokens"`
	MaxOutputTokens              int64           `gorm:"column:max_output_tokens;not null" json:"maxOutputTokens"`
	InputCreditsPerMillion       decimal.Decimal `gorm:"column:input_credits_per_million;type:numeric(24,8);not null;default:0" json:"inputCreditsPerMillion"`
	OutputCreditsPerMillion      decimal.Decimal `gorm:"column:output_credits_per_million;type:numeric(24,8);not null;default:0" json:"outputCreditsPerMillion"`
	CachedInputCreditsPerMillion decimal.Decimal `gorm:"column:cached_input_credits_per_million;type:numeric(24,8);not null;default:0" json:"cachedInputCreditsPerMillion"`
	Enabled                      bool            `gorm:"not null" json:"enabled"`
	CreatedAt                    time.Time       `json:"createdAt"`
	UpdatedAt                    time.Time       `json:"updatedAt"`
}
