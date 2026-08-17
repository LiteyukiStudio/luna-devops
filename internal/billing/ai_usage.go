package billing

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
)

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

func (s Service) SettlePendingAIModelUsage(limit int) (int, error) {
	if limit <= 0 || limit > defaultAIUsageBatchSize {
		limit = defaultAIUsageBatchSize
	}
	var pending []pendingAIModelUsage
	err := s.DB.Raw(`
		SELECT event.id AS event_id,
		       event.run_id,
		       run.owner_user_id AS user_id,
		       event.data,
		       event.created_at AS occurred_at
		FROM ai.run_events AS event
		JOIN ai.runs AS run ON run.id = event.run_id
		WHERE event.type = 'model.completed'
		  AND NULLIF(event.data->>'modelId', '') IS NOT NULL
		  AND NULLIF(event.data->>'modelName', '') IS NOT NULL
		  AND jsonb_typeof(event.data->'pricing') = 'object'
		  AND (
			NOT EXISTS (
				SELECT 1 FROM billing_usage_records AS usage
				WHERE usage.resource_type = ? AND usage.resource_id = event.id AND usage.meter = ?
			)
			OR NOT EXISTS (
				SELECT 1 FROM billing_usage_records AS usage
				WHERE usage.resource_type = ? AND usage.resource_id = event.id AND usage.meter = ?
			)
			OR NOT EXISTS (
				SELECT 1 FROM billing_usage_records AS usage
				WHERE usage.resource_type = ? AND usage.resource_id = event.id AND usage.meter = ?
			)
			OR NOT EXISTS (
				SELECT 1 FROM billing_usage_records AS usage
				WHERE usage.resource_type = ? AND usage.resource_id = event.id AND usage.meter = ?
			)
		  )
		ORDER BY event.created_at ASC
		LIMIT ?`,
		ResourceTypeAIModelRequest, MeterAIInputTokens,
		ResourceTypeAIModelRequest, MeterAIOutputTokens,
		ResourceTypeAIModelRequest, MeterAICachedInputTokens,
		ResourceTypeAIModelRequest, MeterAICachedOutputTokens, limit).Scan(&pending).Error
	if err != nil {
		return 0, err
	}
	settled := 0
	var result error
	for _, item := range pending {
		var data aiModelCompletedData
		if err := json.Unmarshal(item.Data, &data); err != nil {
			result = errors.Join(result, fmt.Errorf("decode AI usage event %s: %w", item.EventID, err))
			continue
		}
		err := s.SettleAIModelUsage(AIModelUsageInput{
			EventID: item.EventID, RunID: item.RunID, UserID: item.UserID,
			ModelID: data.ModelID, ModelName: data.ModelName,
			InputTokens: data.Usage.InputTokens, OutputTokens: data.Usage.OutputTokens,
			CachedInputTokens: data.Usage.CachedInputTokens, CachedOutputTokens: data.Usage.CachedOutputTokens,
			Pricing: AIModelPricingSnapshot{
				InputCreditsPerMillion: data.Pricing.InputCreditsPerMillion, OutputCreditsPerMillion: data.Pricing.OutputCreditsPerMillion,
				CachedInputCreditsPerMillion: data.Pricing.CachedInputCreditsPerMillion, CachedOutputCreditsPerMillion: data.Pricing.CachedOutputCreditsPerMillion,
			},
			OccurredAt: item.OccurredAt,
		})
		if errors.Is(err, ErrAlreadySettled) {
			continue
		}
		if err != nil {
			result = errors.Join(result, fmt.Errorf("settle AI usage event %s: %w", item.EventID, err))
			continue
		}
		settled++
	}
	return settled, result
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
