package agentobservability

import (
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestToolObservabilityQueriesAgainstPostgres(t *testing.T) {
	databaseURL := os.Getenv("AGENT_OBSERVABILITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AGENT_OBSERVABILITY_TEST_DATABASE_URL is not configured")
	}
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	store := NewConversationStore(db)
	start := time.Now().Add(-365 * 24 * time.Hour)
	period, err := store.SummarizeTools(t.Context(), start)
	if err != nil {
		t.Fatalf("summarize tools: %v", err)
	}
	runs, err := store.SummarizeRuns(t.Context(), start)
	if err != nil {
		t.Fatalf("summarize runs: %v", err)
	}
	if runs.InputTokens < 0 || runs.OutputTokens < 0 || runs.DurationP95Seconds < 0 {
		t.Fatalf("invalid run summary: %#v", runs)
	}
	result, err := store.ListToolSummaries(t.Context(), ToolSummaryListOptions{Start: start, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list tool summaries: %v", err)
	}
	if period.Total == 0 || len(result.Items) == 0 {
		return
	}
	calls, err := store.ListToolCalls(t.Context(), ToolCallListOptions{
		Start: start, OperationID: result.Items[0].OperationID, Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if calls.Total == 0 || len(calls.Items) == 0 {
		t.Fatalf("summary %q has no matching calls", result.Items[0].OperationID)
	}
	for _, call := range calls.Items {
		assertNoSensitiveToolKeys(t, call.Arguments)
		assertNoSensitiveToolKeys(t, call.Result)
	}
}

func TestAuthoritativeUsageAggregationAgainstPostgres(t *testing.T) {
	databaseURL := os.Getenv("AGENT_OBSERVABILITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AGENT_OBSERVABILITY_TEST_DATABASE_URL is not configured")
	}
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
	})

	prefix := fmt.Sprintf("agent_obs_usage_%d", time.Now().UnixNano())
	ownerID := prefix + "_owner"
	conversationID := prefix + "_conversation"
	runA := prefix + "_run_a"
	runB := prefix + "_run_b"
	turnA := prefix + "_turn_a"
	turnB := prefix + "_turn_b"
	periodStart := time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)
	seedAuthoritativeUsageFixture(t, tx, authoritativeUsageFixture{
		Prefix: prefix, OwnerID: ownerID, ConversationID: conversationID,
		RunA: runA, RunB: runB, TurnA: turnA, TurnB: turnB, PeriodStart: periodStart,
	})

	store := NewConversationStore(tx)
	summary, err := store.SummarizeRuns(t.Context(), periodStart)
	if err != nil {
		t.Fatalf("summarize authoritative usage: %v", err)
	}
	if summary.InputTokens != 300 || summary.OutputTokens != 60 || summary.DurationP95Seconds != 10 {
		t.Fatalf("period summary = %#v", summary)
	}
	if summary.CacheReadInputTokens != nil {
		t.Fatalf("partially unreported cache read total must be null: %#v", summary.CacheReadInputTokens)
	}
	assertInt64Pointer(t, "period cache write", summary.CacheWriteInputTokens, 5)
	assertInt64Pointer(t, "period reasoning", summary.ReasoningOutputTokens, 7)

	stats, err := store.loadRunStats(t.Context(), []string{runA, runB})
	if err != nil {
		t.Fatalf("load authoritative run usage: %v", err)
	}
	if stats[runA].InputTokens != 300 || stats[runA].OutputTokens != 60 {
		t.Fatalf("run A stats = %#v", stats[runA])
	}
	if stats[runA].CacheReadInputTokens != nil {
		t.Fatalf("run A partially unreported cache read total must be null: %#v", stats[runA])
	}
	assertInt64Pointer(t, "run A cache write", stats[runA].CacheWriteInputTokens, 5)
	assertInt64Pointer(t, "run A reasoning", stats[runA].ReasoningOutputTokens, 7)
	if stats[runB].InputTokens != 11 || stats[runB].OutputTokens != 3 {
		t.Fatalf("run B stats = %#v", stats[runB])
	}
	assertInt64Pointer(t, "run B explicit zero cache read", stats[runB].CacheReadInputTokens, 0)
	if stats[runB].CacheWriteInputTokens != nil || stats[runB].ReasoningOutputTokens != nil {
		t.Fatalf("run B unreported breakdowns must stay null: %#v", stats[runB])
	}
}

type authoritativeUsageFixture struct {
	Prefix         string
	OwnerID        string
	ConversationID string
	RunA           string
	RunB           string
	TurnA          string
	TurnB          string
	PeriodStart    time.Time
}

func seedAuthoritativeUsageFixture(t *testing.T, tx *gorm.DB, fixture authoritativeUsageFixture) {
	t.Helper()
	periodUsageTime := fixture.PeriodStart.Add(24 * time.Hour)
	previousUsageTime := fixture.PeriodStart.Add(-24 * time.Hour)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO ai.conversations (id, owner_user_id, title, created_at, updated_at)
			VALUES (?, ?, 'authoritative usage fixture', ?, ?)`, []any{fixture.ConversationID, fixture.OwnerID, fixture.PeriodStart, fixture.PeriodStart}},
		{`INSERT INTO ai.turns (id, conversation_id, turn_index, status, input, selected_run_id, created_at)
			VALUES (?, ?, 0, 'completed', 'run A', ?, ?), (?, ?, 1, 'completed', 'run B', ?, ?)`, []any{
			fixture.TurnA, fixture.ConversationID, fixture.RunA, fixture.PeriodStart,
			fixture.TurnB, fixture.ConversationID, fixture.RunB, previousUsageTime,
		}},
		{`INSERT INTO ai.runs (
			id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version,
			tool_catalog_digest, actor_session_id, created_at, started_at, completed_at
		) VALUES (?, ?, ?, ?, 0, 'completed', 'test', 'test', 'test', ?, ?, ?),
			(?, ?, ?, ?, 0, 'completed', 'test', 'test', 'test', ?, ?, ?)`, []any{
			fixture.RunA, fixture.OwnerID, fixture.ConversationID, fixture.TurnA,
			fixture.PeriodStart, fixture.PeriodStart, fixture.PeriodStart.Add(10 * time.Second),
			fixture.RunB, fixture.OwnerID, fixture.ConversationID, fixture.TurnB,
			previousUsageTime, previousUsageTime, previousUsageTime.Add(20 * time.Second),
		}},
	}
	for _, statement := range statements {
		if err := tx.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("seed conversation/run fixture: %v", err)
		}
	}

	type usageRow struct {
		suffix     string
		runID      string
		operation  string
		attempt    int
		prompt     int64
		completion int64
		cacheRead  *int64
		cacheWrite *int64
		reasoning  *int64
		occurredAt time.Time
	}
	zero := int64(0)
	five := int64(5)
	seven := int64(7)
	thirty := int64(30)
	rows := []usageRow{
		{suffix: "assistant_1", runID: fixture.RunA, operation: "assistant", attempt: 1, prompt: 100, completion: 20, cacheRead: &thirty, cacheWrite: &five, reasoning: &seven, occurredAt: periodUsageTime},
		{suffix: "assistant_2", runID: fixture.RunA, operation: "assistant", attempt: 2, prompt: 200, completion: 40, cacheWrite: &zero, reasoning: &zero, occurredAt: periodUsageTime.Add(time.Second)},
		{suffix: "summary", runID: fixture.RunA, operation: "summary", attempt: 1, prompt: 9000, completion: 900, occurredAt: periodUsageTime.Add(2 * time.Second)},
		{suffix: "previous", runID: fixture.RunB, operation: "assistant", attempt: 1, prompt: 11, completion: 3, cacheRead: &zero, occurredAt: previousUsageTime},
	}
	for _, row := range rows {
		holdID := fixture.Prefix + "_hold_" + row.suffix
		usageID := fixture.Prefix + "_usage_" + row.suffix
		if err := tx.Exec(`INSERT INTO ai.model_credit_holds (
			id, run_id, owner_user_id, operation, attempt, state, model_id, model_name,
			max_context_tokens_snapshot, max_output_tokens_snapshot,
			input_credits_per_million, output_credits_per_million, cached_input_credits_per_million,
			max_risk_credits, actual_credits, expires_at
		) VALUES (?, ?, ?, ?, ?, 'usage_recorded', 'model-fixture', 'fixture', 4096, 512, 0, 0, 0, 0, 0, ?)`,
			holdID, row.runID, fixture.OwnerID, row.operation, row.attempt, fixture.PeriodStart.AddDate(2, 0, 0)).Error; err != nil {
			t.Fatalf("seed usage hold %s: %v", row.suffix, err)
		}
		if err := tx.Exec(`INSERT INTO ai.model_usages (
			id, credit_hold_id, run_id, owner_user_id, operation, attempt, status, settlement_status,
			model_id, model_name, max_context_tokens_snapshot, prompt_tokens, completion_tokens, total_tokens,
			cached_prompt_tokens, cache_write_prompt_tokens, reasoning_completion_tokens, call_type, occurred_at
		) VALUES (?, ?, ?, ?, ?, ?, 'reported', 'pending', 'model-fixture', 'fixture', 4096, ?, ?, ?, ?, ?, ?, 'stream', ?)`,
			usageID, holdID, row.runID, fixture.OwnerID, row.operation, row.attempt,
			row.prompt, row.completion, row.prompt+row.completion,
			row.cacheRead, row.cacheWrite, row.reasoning, row.occurredAt).Error; err != nil {
			t.Fatalf("seed authoritative usage %s: %v", row.suffix, err)
		}
	}

	if err := tx.Exec(`INSERT INTO ai.run_events (id, run_id, event_sequence, type, data, created_at)
		VALUES (?, ?, 1, 'model.completed', '{"usage":{"inputTokens":999999,"outputTokens":888888}}'::jsonb, ?)`,
		fixture.Prefix+"_poison_event", fixture.RunA, periodUsageTime).Error; err != nil {
		t.Fatalf("seed compatibility event poison value: %v", err)
	}
}

func assertInt64Pointer(t *testing.T, name string, value *int64, want int64) {
	t.Helper()
	if value == nil || *value != want {
		t.Fatalf("%s = %#v, want %d", name, value, want)
	}
}

func assertNoSensitiveToolKeys(t *testing.T, value any) {
	t.Helper()
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			assertNoSensitiveToolKeys(t, item)
		}
	case map[string]any:
		for key, item := range typed {
			if sensitiveToolKey(key) {
				t.Fatalf("sensitive tool key %q reached observability response", key)
			}
			assertNoSensitiveToolKeys(t, item)
		}
	}
}
