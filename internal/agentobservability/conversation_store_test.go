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
