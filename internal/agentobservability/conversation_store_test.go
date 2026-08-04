package agentobservability

import "testing"

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
