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
)

type AIModelUsageInput struct {
	EventID      string
	RunID        string
	UserID       string
	InputTokens  int64
	OutputTokens int64
	OccurredAt   time.Time
}

type pendingAIModelUsage struct {
	EventID    string          `gorm:"column:event_id"`
	RunID      string          `gorm:"column:run_id"`
	UserID     string          `gorm:"column:user_id"`
	Data       json.RawMessage `gorm:"column:data"`
	OccurredAt time.Time       `gorm:"column:occurred_at"`
}

type aiModelCompletedData struct {
	Usage struct {
		InputTokens  int64 `json:"inputTokens"`
		OutputTokens int64 `json:"outputTokens"`
	} `json:"usage"`
}

func (s Service) SettleAIModelUsage(input AIModelUsageInput) error {
	if strings.TrimSpace(input.EventID) == "" || strings.TrimSpace(input.RunID) == "" || strings.TrimSpace(input.UserID) == "" {
		return errors.New("AI model usage identity is incomplete")
	}
	if input.InputTokens < 0 || input.OutputTokens < 0 {
		return errors.New("AI model token usage cannot be negative")
	}
	inputRate, err := s.rate(MeterAIInputTokens)
	if err != nil {
		return err
	}
	outputRate, err := s.rate(MeterAIOutputTokens)
	if err != nil {
		return err
	}
	periodStart := input.OccurredAt
	if periodStart.IsZero() {
		periodStart = time.Now()
	}
	periodEnd := periodStart.Add(time.Nanosecond)
	inputQuantity := tokenBillingQuantity(input.InputTokens)
	outputQuantity := tokenBillingQuantity(input.OutputTokens)
	metadata, _ := json.Marshal(map[string]any{
		"runId":        input.RunID,
		"usageEventId": input.EventID,
		"inputTokens":  input.InputTokens,
		"outputTokens": input.OutputTokens,
	})
	now := time.Now()
	records := []model.BillingUsageRecord{
		{
			// AI 费用只归属发起用户，不关联项目空间：ProjectID 恒为空。
			ID: id.New("busg"), ProjectID: "", Meter: MeterAIInputTokens,
			Quantity: inputQuantity, Unit: "1000_tokens", AmountCredits: inputQuantity.Mul(inputRate),
			ResourceType: ResourceTypeAIModelRequest, ResourceID: input.EventID,
			PeriodStart: periodStart, PeriodEnd: periodEnd, Status: "settled", Metadata: string(metadata), SettledAt: &now,
		},
		{
			ID: id.New("busg"), ProjectID: "", Meter: MeterAIOutputTokens,
			Quantity: outputQuantity, Unit: "1000_tokens", AmountCredits: outputQuantity.Mul(outputRate),
			ResourceType: ResourceTypeAIModelRequest, ResourceID: input.EventID,
			PeriodStart: periodStart, PeriodEnd: periodEnd, Status: "settled", Metadata: string(metadata), SettledAt: &now,
		},
	}
	return s.debitUserUsages(records, ReasonAIUsage, "AI model token usage", "system", input.UserID)
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
		  AND (
			NOT EXISTS (
				SELECT 1 FROM billing_usage_records AS usage
				WHERE usage.resource_type = ? AND usage.resource_id = event.id AND usage.meter = ?
			)
			OR NOT EXISTS (
				SELECT 1 FROM billing_usage_records AS usage
				WHERE usage.resource_type = ? AND usage.resource_id = event.id AND usage.meter = ?
			)
		  )
		ORDER BY event.created_at ASC
		LIMIT ?`, ResourceTypeAIModelRequest, MeterAIInputTokens, ResourceTypeAIModelRequest, MeterAIOutputTokens, limit).Scan(&pending).Error
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
			InputTokens: data.Usage.InputTokens, OutputTokens: data.Usage.OutputTokens, OccurredAt: item.OccurredAt,
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

func tokenBillingQuantity(tokens int64) decimal.Decimal {
	return decimal.NewFromInt(tokens).Div(decimal.NewFromInt(1000))
}
