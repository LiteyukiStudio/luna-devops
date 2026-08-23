package billing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAIUsageSettlementFailed = errors.New("billing.ai_usage_settlement_failed")

const (
	ReasonAIUsage              = "ai.usage"
	ResourceTypeAIModelRequest = "ai_model_request"
	defaultAIUsageBatchSize    = 200
	aiUsageSchemaVersion       = 2
)

type AIModelPricingSnapshot struct {
	InputCreditsPerMillion       decimal.Decimal
	OutputCreditsPerMillion      decimal.Decimal
	CachedInputCreditsPerMillion decimal.Decimal
}

type AIModelUsageInput struct {
	EventID                   string
	RunID                     string
	UserID                    string
	ModelID                   string
	ModelName                 string
	PromptTokens              int64
	CompletionTokens          int64
	TotalTokens               int64
	CachedPromptTokens        int64
	CacheWritePromptTokens    int64
	ReasoningCompletionTokens int64
	Pricing                   AIModelPricingSnapshot
	OccurredAt                time.Time
}

type pendingAIModelUsage struct {
	ID                           string          `gorm:"column:id"`
	CreditHoldID                 string          `gorm:"column:credit_hold_id"`
	RunID                        string          `gorm:"column:run_id"`
	UserID                       string          `gorm:"column:owner_user_id"`
	ModelID                      string          `gorm:"column:model_id"`
	ModelName                    string          `gorm:"column:model_name"`
	PromptTokens                 int64           `gorm:"column:prompt_tokens"`
	CompletionTokens             int64           `gorm:"column:completion_tokens"`
	TotalTokens                  int64           `gorm:"column:total_tokens"`
	CachedPromptTokens           int64           `gorm:"column:cached_prompt_tokens"`
	CacheWritePromptTokens       int64           `gorm:"column:cache_write_prompt_tokens"`
	ReasoningCompletionTokens    int64           `gorm:"column:reasoning_completion_tokens"`
	InputCreditsPerMillion       decimal.Decimal `gorm:"column:input_credits_per_million"`
	OutputCreditsPerMillion      decimal.Decimal `gorm:"column:output_credits_per_million"`
	CachedInputCreditsPerMillion decimal.Decimal `gorm:"column:cached_input_credits_per_million"`
	OccurredAt                   time.Time       `gorm:"column:occurred_at"`
}

func (s Service) SettleAIModelUsage(input AIModelUsageInput) error {
	if err := validateReportedUsage(input); err != nil {
		return err
	}
	periodStart := input.OccurredAt
	if periodStart.IsZero() {
		periodStart = time.Now()
	}
	records := aiUsageRecords(input, periodStart, input.EventID)
	return s.debitUserUsages(records, ReasonAIUsage, "AI model token usage", "system", input.UserID)
}

func validateReportedUsage(input AIModelUsageInput) error {
	if strings.TrimSpace(input.EventID) == "" || strings.TrimSpace(input.RunID) == "" || strings.TrimSpace(input.UserID) == "" {
		return errors.New("AI model usage identity is incomplete")
	}
	if strings.TrimSpace(input.ModelID) == "" || strings.TrimSpace(input.ModelName) == "" {
		return errors.New("AI model usage model snapshot is incomplete")
	}
	if input.PromptTokens < 0 || input.CompletionTokens < 0 || input.TotalTokens < 0 ||
		input.CachedPromptTokens < 0 || input.CacheWritePromptTokens < 0 || input.ReasoningCompletionTokens < 0 {
		return errors.New("AI model token usage cannot be negative")
	}
	if input.TotalTokens != input.PromptTokens+input.CompletionTokens ||
		input.CachedPromptTokens+input.CacheWritePromptTokens > input.PromptTokens ||
		input.ReasoningCompletionTokens > input.CompletionTokens {
		return errors.New("AI model provider usage relationships are invalid")
	}
	if input.Pricing.InputCreditsPerMillion.IsNegative() || input.Pricing.OutputCreditsPerMillion.IsNegative() ||
		input.Pricing.CachedInputCreditsPerMillion.IsNegative() {
		return errors.New("AI model pricing cannot be negative")
	}
	return nil
}

func aiUsageRecords(input AIModelUsageInput, periodStart time.Time, resourceID string) []model.BillingUsageRecord {
	periodEnd := periodStart.Add(time.Nanosecond)
	normalPrompt := input.PromptTokens - input.CachedPromptTokens - input.CacheWritePromptTokens
	metadata, _ := json.Marshal(map[string]any{
		"usageSchemaVersion": aiUsageSchemaVersion,
		"runId":              input.RunID, "usageId": input.EventID, "modelId": input.ModelID, "modelName": input.ModelName,
		"promptTokens": input.PromptTokens, "completionTokens": input.CompletionTokens, "totalTokens": input.TotalTokens,
		"cachedPromptTokens": input.CachedPromptTokens, "cacheWritePromptTokens": input.CacheWritePromptTokens,
		"reasoningCompletionTokens": input.ReasoningCompletionTokens,
	})
	now := time.Now()
	return []model.BillingUsageRecord{
		newAIUsageRecord(MeterAIPromptTokens, tokenBillingQuantity(normalPrompt), input.Pricing.InputCreditsPerMillion, input, resourceID, periodStart, periodEnd, metadata, now),
		newAIUsageRecord(MeterAICompletionTokens, tokenBillingQuantity(input.CompletionTokens), input.Pricing.OutputCreditsPerMillion, input, resourceID, periodStart, periodEnd, metadata, now),
		newAIUsageRecord(MeterAICachedPromptTokens, tokenBillingQuantity(input.CachedPromptTokens), input.Pricing.CachedInputCreditsPerMillion, input, resourceID, periodStart, periodEnd, metadata, now),
		newAIUsageRecord(MeterAICacheWritePromptTokens, tokenBillingQuantity(input.CacheWritePromptTokens), input.Pricing.CachedInputCreditsPerMillion, input, resourceID, periodStart, periodEnd, metadata, now),
	}
}

func newAIUsageRecord(meter string, quantity, rate decimal.Decimal, input AIModelUsageInput, resourceID string, periodStart, periodEnd time.Time, metadata []byte, now time.Time) model.BillingUsageRecord {
	return model.BillingUsageRecord{
		ID: id.New("busg"), ProjectID: "", Meter: meter,
		Quantity: quantity, Unit: "million_tokens", AmountCredits: quantity.Mul(rate),
		ResourceType: ResourceTypeAIModelRequest, ResourceID: resourceID,
		PeriodStart: periodStart, PeriodEnd: periodEnd, Status: "settled", Metadata: string(metadata), SettledAt: &now,
	}
}

func (s Service) SettlePendingAIModelUsage(ctx context.Context, limit int) (int, error) {
	ctx, end := telemetry.StartOperation(ctx, "billing", "ai_usage.settle_batch")
	var operationErr error
	defer func() { end(operationErr) }()
	if limit <= 0 || limit > defaultAIUsageBatchSize {
		limit = defaultAIUsageBatchSize
	}
	db := s.DB.WithContext(ctx)
	var pending []pendingAIModelUsage
	err := db.Raw(`
		SELECT usage.id, usage.credit_hold_id, usage.run_id, usage.owner_user_id,
		       usage.model_id, usage.model_name, usage.prompt_tokens, usage.completion_tokens,
		       usage.total_tokens, COALESCE(usage.cached_prompt_tokens, 0) AS cached_prompt_tokens,
		       COALESCE(usage.cache_write_prompt_tokens, 0) AS cache_write_prompt_tokens,
		       COALESCE(usage.reasoning_completion_tokens, 0) AS reasoning_completion_tokens,
		       hold.input_credits_per_million, hold.output_credits_per_million,
		       hold.cached_input_credits_per_million, usage.occurred_at
		FROM ai.model_usages usage
		JOIN ai.model_credit_holds hold ON hold.id = usage.credit_hold_id
		WHERE usage.status = 'reported' AND usage.settlement_status = 'pending'
		  AND hold.state = 'usage_recorded'
		ORDER BY usage.occurred_at ASC, usage.id ASC
		LIMIT ?`, limit).Scan(&pending).Error
	if err != nil {
		operationErr = err
		return 0, err
	}
	settled := 0
	failed := false
	for _, item := range pending {
		if err := (Service{DB: db}).settleReportedAIUsage(ctx, item); err != nil {
			if errors.Is(err, ErrAlreadySettled) {
				continue
			}
			failed = true
			continue
		}
		settled++
	}
	if failed {
		operationErr = ErrAIUsageSettlementFailed
		return settled, ErrAIUsageSettlementFailed
	}
	return settled, nil
}

func (s Service) settleReportedAIUsage(ctx context.Context, item pendingAIModelUsage) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "billing", "ai_usage.settle")
	defer func() { end(err) }()
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureWallet(tx, item.UserID); err != nil {
			return err
		}
		var wallet model.UserWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "user_id = ?", item.UserID).Error; err != nil {
			return err
		}
		var holdState string
		if err := tx.Raw("SELECT state FROM ai.model_credit_holds WHERE id = ? FOR UPDATE", item.CreditHoldID).Scan(&holdState).Error; err != nil {
			return err
		}
		var settlementStatus string
		if err := tx.Raw("SELECT settlement_status FROM ai.model_usages WHERE id = ? FOR UPDATE", item.ID).Scan(&settlementStatus).Error; err != nil {
			return err
		}
		if settlementStatus == "settled" || holdState == "settled" {
			return ErrAlreadySettled
		}
		if settlementStatus != "pending" || holdState != "usage_recorded" {
			return errors.New("AI model usage is not eligible for settlement")
		}
		input := AIModelUsageInput{
			EventID: item.ID, RunID: item.RunID, UserID: item.UserID, ModelID: item.ModelID, ModelName: item.ModelName,
			PromptTokens: item.PromptTokens, CompletionTokens: item.CompletionTokens, TotalTokens: item.TotalTokens,
			CachedPromptTokens: item.CachedPromptTokens, CacheWritePromptTokens: item.CacheWritePromptTokens,
			ReasoningCompletionTokens: item.ReasoningCompletionTokens,
			Pricing:                   AIModelPricingSnapshot{InputCreditsPerMillion: item.InputCreditsPerMillion, OutputCreditsPerMillion: item.OutputCreditsPerMillion, CachedInputCreditsPerMillion: item.CachedInputCreditsPerMillion},
			OccurredAt:                item.OccurredAt,
		}
		if err := validateReportedUsage(input); err != nil {
			return err
		}
		records := aiUsageRecords(input, item.OccurredAt, item.ID)
		if err := debitUsagesForUser(tx, records, ReasonAIUsage, "AI model token usage", "system", item.UserID, item.CreditHoldID); err != nil && !errors.Is(err, ErrAlreadySettled) {
			return err
		}
		if err := tx.Exec("UPDATE ai.model_usages SET settlement_status = 'settled', settled_at = now() WHERE id = ? AND settlement_status = 'pending'", item.ID).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE ai.model_credit_holds SET state = 'settled', updated_at = now() WHERE id = ? AND state = 'usage_recorded'", item.CreditHoldID).Error
	})
}

func tokenBillingQuantity(tokens int64) decimal.Decimal {
	return decimal.NewFromInt(tokens).Div(decimal.NewFromInt(1_000_000))
}
