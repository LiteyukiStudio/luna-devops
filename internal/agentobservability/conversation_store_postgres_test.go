package agentobservability

import (
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
