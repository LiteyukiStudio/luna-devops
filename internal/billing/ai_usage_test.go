package billing

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestValidateReportedUsageAcceptsNormalizedGatewayTotals(t *testing.T) {
	valid := AIModelUsageInput{
		EventID:                   "aiuse_gemini_proxy",
		RunID:                     "airun_1",
		UserID:                    "usr_1",
		ModelID:                   "aimod_gemini",
		ModelName:                 "gemini-2.5-pro",
		PromptTokens:              150,
		CompletionTokens:          40,
		TotalTokens:               260,
		CachedPromptTokens:        100,
		CacheWritePromptTokens:    20,
		ReasoningCompletionTokens: 70,
		Pricing: AIModelPricingSnapshot{
			InputCreditsPerMillion:       decimal.NewFromInt(2),
			OutputCreditsPerMillion:      decimal.NewFromInt(8),
			CachedInputCreditsPerMillion: decimal.NewFromFloat(0.5),
		},
		OccurredAt: time.Now().UTC(),
	}
	if err := validateReportedUsage(valid); err != nil {
		t.Fatalf("validateReportedUsage rejected normalized gateway usage: %v", err)
	}

	records := aiUsageRecords(valid, valid.OccurredAt, valid.EventID)
	if len(records) != 4 {
		t.Fatalf("records count = %d, want 4", len(records))
	}
	for _, record := range records {
		if record.ProjectID != "" {
			t.Fatalf("AI usage record ProjectID = %q, want empty", record.ProjectID)
		}
	}

	invalidTotal := valid
	invalidTotal.TotalTokens = 189
	if err := validateReportedUsage(invalidTotal); err == nil {
		t.Fatal("validateReportedUsage accepted total_tokens < prompt_tokens + completion_tokens")
	}

	invalidCache := valid
	invalidCache.CachedPromptTokens = 140
	if err := validateReportedUsage(invalidCache); err == nil {
		t.Fatal("validateReportedUsage accepted cached + cache_write > prompt_tokens")
	}

	withoutCache := valid
	withoutCache.CachedPromptTokens = 0
	withoutCache.CacheWritePromptTokens = 0
	withoutCacheRecords := aiUsageRecords(withoutCache, withoutCache.OccurredAt, withoutCache.EventID)
	if len(withoutCacheRecords) != 2 {
		t.Fatalf("records without cache tokens = %d, want 2", len(withoutCacheRecords))
	}

	zeroUsage := valid
	zeroUsage.PromptTokens = 0
	zeroUsage.CompletionTokens = 0
	zeroUsage.TotalTokens = 0
	zeroUsage.CachedPromptTokens = 0
	zeroUsage.CacheWritePromptTokens = 0
	zeroUsage.ReasoningCompletionTokens = 0
	if got := aiUsageRecords(zeroUsage, zeroUsage.OccurredAt, zeroUsage.EventID); len(got) != 0 {
		t.Fatalf("zero-token records = %d, want 0", len(got))
	}
}
