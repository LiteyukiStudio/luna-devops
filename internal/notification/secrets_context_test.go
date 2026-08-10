package notification

import (
	"context"
	"encoding/json"
	"testing"
)

type contextCapturingSecretResolver struct {
	context context.Context
}

func (r *contextCapturingSecretResolver) ResolveContext(ctx context.Context, ref string) string {
	r.context = ctx
	return ref + "-value"
}

func TestResolveSecretMapPropagatesCallerContext(t *testing.T) {
	type contextKey string
	const key contextKey = "trace"
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key, "trace-value"))
	cancel()
	resolver := &contextCapturingSecretResolver{}

	resolved := resolveSecretMap(ctx, json.RawMessage(`{"token":"token-ref"}`), resolver)

	if resolved["token"] != "token-ref-value" {
		t.Fatalf("resolved token = %q", resolved["token"])
	}
	if resolver.context == nil || resolver.context.Value(key) != "trace-value" {
		t.Fatal("secret resolver did not receive caller context values")
	}
	if resolver.context.Err() != context.Canceled {
		t.Fatalf("secret resolver context error = %v, want context.Canceled", resolver.context.Err())
	}
}
