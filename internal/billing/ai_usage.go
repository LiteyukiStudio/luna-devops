package billing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAIUsageSettlementFailed = errors.New("billing.ai_usage_settlement_failed")

const (
	ReasonAIUsage              = "ai.usage"
	ResourceTypeAIModelRequest = "ai_model_request"
	defaultAIUsageBatchSize    = 200
	aiUsageSchemaVersion       = 1
)

type AIModelPricingSnapshot struct {
	InputCreditsPerMillion        decimal.Decimal
	OutputCreditsPerMillion       decimal.Decimal
	CachedInputCreditsPerMillion  decimal.Decimal
	CachedOutputCreditsPerMillion decimal.Decimal
}

type AIModelUsageInput struct {
	EventID            string
	RunID              string
	UserID             string
	ModelID            string
	ModelName          string
	InputTokens        int64
	OutputTokens       int64
	CachedInputTokens  int64
	CachedOutputTokens int64
	Pricing            AIModelPricingSnapshot
	OccurredAt         time.Time
}

type pendingAIModelUsage struct {
	EventID    string          `gorm:"column:event_id"`
	RunID      string          `gorm:"column:run_id"`
	UserID     string          `gorm:"column:user_id"`
	Data       json.RawMessage `gorm:"column:data"`
	OccurredAt time.Time       `gorm:"column:occurred_at"`
}

type pendingAIModelReservation struct {
	ID                            string          `gorm:"column:id"`
	RunID                         string          `gorm:"column:run_id"`
	UserID                        string          `gorm:"column:owner_user_id"`
	ModelID                       string          `gorm:"column:model_id"`
	ModelName                     string          `gorm:"column:model_name"`
	InputTokens                   int64           `gorm:"column:input_tokens"`
	OutputTokens                  int64           `gorm:"column:output_tokens"`
	CachedInputTokens             int64           `gorm:"column:cached_input_tokens"`
	CachedOutputTokens            int64           `gorm:"column:cached_output_tokens"`
	InputCreditsPerMillion        decimal.Decimal `gorm:"column:input_credits_per_million"`
	OutputCreditsPerMillion       decimal.Decimal `gorm:"column:output_credits_per_million"`
	CachedInputCreditsPerMillion  decimal.Decimal `gorm:"column:cached_input_credits_per_million"`
	CachedOutputCreditsPerMillion decimal.Decimal `gorm:"column:cached_output_credits_per_million"`
	OccurredAt                    time.Time       `gorm:"column:created_at"`
}

type aiModelCompletedData struct {
	ModelID   string `json:"modelId"`
	ModelName string `json:"modelName"`
	Usage     struct {
		InputTokens        int64 `json:"inputTokens"`
		OutputTokens       int64 `json:"outputTokens"`
		CachedInputTokens  int64 `json:"cachedInputTokens"`
		CachedOutputTokens int64 `json:"cachedOutputTokens"`
	} `json:"usage"`
	Pricing struct {
		InputCreditsPerMillion        decimal.Decimal `json:"inputCreditsPerMillion"`
		OutputCreditsPerMillion       decimal.Decimal `json:"outputCreditsPerMillion"`
		CachedInputCreditsPerMillion  decimal.Decimal `json:"cachedInputCreditsPerMillion"`
		CachedOutputCreditsPerMillion decimal.Decimal `json:"cachedOutputCreditsPerMillion"`
	} `json:"pricing"`
}

func (s Service) SettleAIModelUsage(input AIModelUsageInput) error {
	if strings.TrimSpace(input.EventID) == "" || strings.TrimSpace(input.RunID) == "" || strings.TrimSpace(input.UserID) == "" {
		return errors.New("AI model usage identity is incomplete")
	}
	if strings.TrimSpace(input.ModelID) == "" || strings.TrimSpace(input.ModelName) == "" {
		return errors.New("AI model usage model snapshot is incomplete")
	}
	if input.InputTokens < 0 || input.OutputTokens < 0 || input.CachedInputTokens < 0 || input.CachedOutputTokens < 0 {
		return errors.New("AI model token usage cannot be negative")
	}
	if input.Pricing.InputCreditsPerMillion.IsNegative() || input.Pricing.OutputCreditsPerMillion.IsNegative() ||
		input.Pricing.CachedInputCreditsPerMillion.IsNegative() || input.Pricing.CachedOutputCreditsPerMillion.IsNegative() {
		return errors.New("AI model pricing cannot be negative")
	}
	periodStart := input.OccurredAt
	if periodStart.IsZero() {
		periodStart = time.Now()
	}
	periodEnd := periodStart.Add(time.Nanosecond)
	inputQuantity := tokenBillingQuantity(maxNormalTokens(input.InputTokens, input.CachedInputTokens))
	outputQuantity := tokenBillingQuantity(maxNormalTokens(input.OutputTokens, input.CachedOutputTokens))
	cachedInputQuantity := tokenBillingQuantity(input.CachedInputTokens)
	cachedOutputQuantity := tokenBillingQuantity(input.CachedOutputTokens)
	metadata, _ := json.Marshal(map[string]any{
		"usageSchemaVersion": aiUsageSchemaVersion,
		"runId":              input.RunID,
		"usageEventId":       input.EventID,
		"modelId":            input.ModelID,
		"modelName":          input.ModelName,
		"inputTokens":        input.InputTokens,
		"outputTokens":       input.OutputTokens,
		"cachedInputTokens":  input.CachedInputTokens,
		"cachedOutputTokens": input.CachedOutputTokens,
		"pricing": map[string]any{
			"inputCreditsPerMillion":        input.Pricing.InputCreditsPerMillion,
			"outputCreditsPerMillion":       input.Pricing.OutputCreditsPerMillion,
			"cachedInputCreditsPerMillion":  input.Pricing.CachedInputCreditsPerMillion,
			"cachedOutputCreditsPerMillion": input.Pricing.CachedOutputCreditsPerMillion,
		},
	})
	now := time.Now()
	records := []model.BillingUsageRecord{
		newAIUsageRecord(MeterAIInputTokens, inputQuantity, input.Pricing.InputCreditsPerMillion, input, periodStart, periodEnd, metadata, now),
		newAIUsageRecord(MeterAIOutputTokens, outputQuantity, input.Pricing.OutputCreditsPerMillion, input, periodStart, periodEnd, metadata, now),
		newAIUsageRecord(MeterAICachedInputTokens, cachedInputQuantity, input.Pricing.CachedInputCreditsPerMillion, input, periodStart, periodEnd, metadata, now),
		newAIUsageRecord(MeterAICachedOutputTokens, cachedOutputQuantity, input.Pricing.CachedOutputCreditsPerMillion, input, periodStart, periodEnd, metadata, now),
	}
	return s.debitUserUsages(records, ReasonAIUsage, "AI model token usage", "system", input.UserID)
}

func newAIUsageRecord(meter string, quantity, rate decimal.Decimal, input AIModelUsageInput, periodStart, periodEnd time.Time, metadata []byte, now time.Time) model.BillingUsageRecord {
	return model.BillingUsageRecord{
		// AI 费用只归属发起用户，不关联项目空间：ProjectID 恒为空。
		ID: id.New("busg"), ProjectID: "", Meter: meter,
		Quantity: quantity, Unit: "million_tokens", AmountCredits: quantity.Mul(rate),
		ResourceType: ResourceTypeAIModelRequest, ResourceID: input.EventID,
		PeriodStart: periodStart, PeriodEnd: periodEnd, Status: "settled", Metadata: string(metadata), SettledAt: &now,
	}
}

func (s Service) SettlePendingAIModelUsage(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > defaultAIUsageBatchSize {
		limit = defaultAIUsageBatchSize
	}
	db := s.DB.WithContext(ctx)
	var pending []pendingAIModelReservation
	err := db.Raw(`
		SELECT id, run_id, owner_user_id, model_id, model_name,
		       input_tokens, output_tokens, cached_input_tokens, cached_output_tokens,
		       input_credits_per_million, output_credits_per_million,
		       cached_input_credits_per_million, cached_output_credits_per_million,
		       created_at
		FROM ai.model_budget_reservations
		WHERE state = 'confirmed'
		ORDER BY updated_at ASC, created_at ASC
		LIMIT ?`, limit).Scan(&pending).Error
	if err != nil {
		return 0, err
	}
	settled := 0
	failed := false
	for _, item := range pending {
		err := (Service{DB: db}).settleAIModelReservation(item)
		if errors.Is(err, ErrAlreadySettled) {
			continue
		}
		if err != nil {
			failed = true
			// Move a transiently failing row behind newer confirmed work so one
			// bounded batch cannot starve the queue. Never persist raw error text.
			_ = db.Exec("UPDATE ai.model_budget_reservations SET updated_at = now() WHERE id = ? AND state = 'confirmed'", item.ID).Error
			continue
		}
		settled++
	}
	if failed {
		return settled, ErrAIUsageSettlementFailed
	}
	return settled, nil
}

func (s Service) settleAIModelReservation(item pendingAIModelReservation) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		// Keep the global lock order wallet -> reservation. Agent reservations and
		// ordinary debits use the same order, preventing a settlement/debit cycle.
		if err := ensureWallet(tx, item.UserID); err != nil {
			return err
		}
		var wallet model.UserWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "user_id = ?", item.UserID).Error; err != nil {
			return err
		}
		var state struct {
			State       string `gorm:"column:state"`
			OwnerUserID string `gorm:"column:owner_user_id"`
		}
		if err := tx.Raw("SELECT state, owner_user_id FROM ai.model_budget_reservations WHERE id = ? FOR UPDATE", item.ID).Scan(&state).Error; err != nil {
			return err
		}
		if state.OwnerUserID != item.UserID {
			return errors.New("AI model reservation owner changed")
		}
		if state.State == "settled" {
			return ErrAlreadySettled
		}
		if state.State != "confirmed" {
			return errors.New("AI model reservation is not ready for settlement")
		}
		input := AIModelUsageInput{
			EventID: item.ID, RunID: item.RunID, UserID: item.UserID,
			ModelID: item.ModelID, ModelName: item.ModelName,
			InputTokens: item.InputTokens, OutputTokens: item.OutputTokens,
			CachedInputTokens: item.CachedInputTokens, CachedOutputTokens: item.CachedOutputTokens,
			Pricing: AIModelPricingSnapshot{
				InputCreditsPerMillion: item.InputCreditsPerMillion, OutputCreditsPerMillion: item.OutputCreditsPerMillion,
				CachedInputCreditsPerMillion: item.CachedInputCreditsPerMillion, CachedOutputCreditsPerMillion: item.CachedOutputCreditsPerMillion,
			},
			OccurredAt: item.OccurredAt,
		}
		periodStart := input.OccurredAt
		periodEnd := periodStart.Add(time.Nanosecond)
		metadata, _ := json.Marshal(map[string]any{"runId": input.RunID, "reservationId": item.ID, "modelId": input.ModelID, "modelName": input.ModelName})
		now := time.Now()
		records := []model.BillingUsageRecord{
			newAIUsageRecord(MeterAIInputTokens, tokenBillingQuantity(maxNormalTokens(input.InputTokens, input.CachedInputTokens)), input.Pricing.InputCreditsPerMillion, input, periodStart, periodEnd, metadata, now),
			newAIUsageRecord(MeterAIOutputTokens, tokenBillingQuantity(maxNormalTokens(input.OutputTokens, input.CachedOutputTokens)), input.Pricing.OutputCreditsPerMillion, input, periodStart, periodEnd, metadata, now),
			newAIUsageRecord(MeterAICachedInputTokens, tokenBillingQuantity(input.CachedInputTokens), input.Pricing.CachedInputCreditsPerMillion, input, periodStart, periodEnd, metadata, now),
			newAIUsageRecord(MeterAICachedOutputTokens, tokenBillingQuantity(input.CachedOutputTokens), input.Pricing.CachedOutputCreditsPerMillion, input, periodStart, periodEnd, metadata, now),
		}
		if err := debitUsagesForUser(tx, records, ReasonAIUsage, "AI model token usage", "system", item.UserID, item.ID); err != nil && !errors.Is(err, ErrAlreadySettled) {
			return err
		}
		return tx.Exec("UPDATE ai.model_budget_reservations SET state = 'settled', updated_at = now() WHERE id = ? AND state = 'confirmed'", item.ID).Error
	})
}

func maxNormalTokens(total, cached int64) int64 {
	normal := total - cached
	if normal < 0 {
		return 0
	}
	return normal
}

func tokenBillingQuantity(tokens int64) decimal.Decimal {
	return decimal.NewFromInt(tokens).Div(decimal.NewFromInt(1_000_000))
}
