package agentobservability

import (
	"strings"
	"testing"
)

func TestSummarizeRunsUsesTheOverviewSecondsContract(t *testing.T) {
	if !strings.Contains(summarizeRunsSQL, "EXTRACT(EPOCH FROM (completed_at - started_at)) AS duration_seconds") {
		t.Fatal("run duration SQL must return seconds for the overview contract")
	}
	if strings.Contains(summarizeRunsSQL, "* 1000") || strings.Contains(summarizeRunsSQL, "duration_ms") {
		t.Fatal("run duration SQL must not expose milliseconds to the seconds-based overview field")
	}
}

func TestTraceIDFromContext(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "valid", raw: `{"traceparent":"00-0123456789ABCDEF0123456789ABCDEF-0123456789abcdef-01"}`, want: "0123456789abcdef0123456789abcdef"},
		{name: "invalid length", raw: `{"traceparent":"00-1234-0123456789abcdef-01"}`},
		{name: "invalid hex", raw: `{"traceparent":"00-z123456789abcdef0123456789abcdef-0123456789abcdef-01"}`},
		{name: "malformed json", raw: `{`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := traceIDFromContext([]byte(test.raw)); got != test.want {
				t.Fatalf("traceIDFromContext() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMessageTextOnlyReturnsTextParts(t *testing.T) {
	raw := []byte(`{"parts":[{"type":"text","text":"first"},{"type":"structured_data","text":"secret"},{"type":"text","text":"second"}]}`)
	if got := messageText(raw); got != "first\nsecond" {
		t.Fatalf("messageText() = %q", got)
	}
}

func TestConversationSortClauseUsesWhitelist(t *testing.T) {
	if got := conversationSortClause("title; DROP TABLE users", "asc"); got != "c.updated_at asc" {
		t.Fatalf("unsafe sort clause = %q", got)
	}
	if got := conversationSortClause("turnCount", "desc"); got != "turn_count desc" {
		t.Fatalf("turn sort clause = %q", got)
	}
}

func TestTurnSortClauseUsesWhitelistAndStableTieBreaker(t *testing.T) {
	if got := turnSortClause("duration; DROP TABLE users", "asc"); got != "t.created_at asc, t.id asc" {
		t.Fatalf("unsafe turn sort clause = %q", got)
	}
	if got := turnSortClause("duration", "desc"); got != "(EXTRACT(EPOCH FROM (r.completed_at - r.started_at)) * 1000) desc, t.id desc" {
		t.Fatalf("duration sort clause = %q", got)
	}
}

func TestToolSortClausesUseWhitelists(t *testing.T) {
	if got := toolSummarySortClause("successRate; DROP TABLE ai.tool_calls", "asc"); got != "last_called_at asc, operation_id asc" {
		t.Fatalf("unsafe tool summary sort clause = %q", got)
	}
	if got := toolSummarySortClause("successRate", "desc"); got != "success_rate desc, operation_id asc" {
		t.Fatalf("tool summary sort clause = %q", got)
	}
	if got := toolCallSortClause("user", "asc"); got != "u.name asc, item.id asc" {
		t.Fatalf("tool call sort clause = %q", got)
	}
}

func TestPageWithinTotalClampsEmptyAndOverflowPages(t *testing.T) {
	if got := pageWithinTotal(4, 20, 0); got != 1 {
		t.Fatalf("empty result page = %d", got)
	}
	if got := pageWithinTotal(9, 20, 41); got != 3 {
		t.Fatalf("overflow result page = %d", got)
	}
	if got := pageWithinTotal(2, 20, 41); got != 2 {
		t.Fatalf("valid result page = %d", got)
	}
}

func TestBuildConversationLoops(t *testing.T) {
	items := []ConversationRunItem{
		{ID: "thinking-1", Type: "reasoning_summary"},
		{ID: "tool-1", Type: "tool_call"},
		{ID: "thinking-2", Type: "reasoning_summary"},
		{ID: "message-2", Type: "assistant_message", Text: "done"},
	}
	loops := buildConversationLoops(items)
	if len(loops) != 2 || len(loops[0].Items) != 2 || len(loops[1].Items) != 2 {
		t.Fatalf("unexpected loops: %#v", loops)
	}
}

func TestSummarizeTurnPeriodUsesOnlyTerminalTurnsForSuccessRate(t *testing.T) {
	summary := summarizeTurnPeriod(8, 3, 4)
	if summary.Total != 8 || summary.SuccessRate != 75 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	empty := summarizeTurnPeriod(2, 0, 0)
	if empty.SuccessRate != 0 {
		t.Fatalf("non-terminal turns must not produce a success rate: %#v", empty)
	}
}

func TestSummarizeToolPeriodUsesOnlyExecutedTerminalCalls(t *testing.T) {
	summary := summarizeToolPeriod(10, 7, 1)
	if summary.Total != 10 || summary.Successful != 7 || summary.Failed != 1 || summary.SuccessRate != 87.5 {
		t.Fatalf("unexpected tool summary: %#v", summary)
	}
	empty := summarizeToolPeriod(3, 0, 0)
	if empty.SuccessRate != 0 {
		t.Fatalf("non-terminal calls must not produce a success rate: %#v", empty)
	}
}

func TestToolCallFromContentSanitizesSensitiveValues(t *testing.T) {
	raw := []byte(`{"toolCallId":"tool-1","operationId":"get_build","status":"succeeded","arguments":{"projectId":"project-1","token":"hidden","nested":{"password":"hidden","ok":true}},"result":{"requestId":"req-1","secret":"hidden"},"traceId":"ABCDEFABCDEFABCDEFABCDEFABCDEFAB"}`)
	call := toolCallFromContent("item-1", "completed", raw)
	if call.TraceID != "abcdefabcdefabcdefabcdefabcdefab" || call.Arguments["projectId"] != "project-1" {
		t.Fatalf("unexpected tool call: %#v", call)
	}
	if _, ok := call.Arguments["token"]; ok {
		t.Fatal("token must be removed")
	}
	nested := call.Arguments["nested"].(map[string]any)
	if _, ok := nested["password"]; ok {
		t.Fatal("nested password must be removed")
	}
	result := call.Result.(map[string]any)
	if _, ok := result["secret"]; ok {
		t.Fatal("result secret must be removed")
	}
}

func TestDecodeSanitizedToolValueDropsSensitiveFields(t *testing.T) {
	value := decodeSanitizedToolValue([]byte(`{"requestId":"req-1","token":"hidden","nested":{"password":"hidden","ok":true}}`))
	result := value.(map[string]any)
	if result["requestId"] != "req-1" {
		t.Fatalf("unexpected decoded result: %#v", result)
	}
	if _, ok := result["token"]; ok {
		t.Fatal("token must be removed")
	}
	nested := result["nested"].(map[string]any)
	if _, ok := nested["password"]; ok {
		t.Fatal("nested password must be removed")
	}
}
