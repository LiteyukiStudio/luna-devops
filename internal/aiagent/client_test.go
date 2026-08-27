package aiagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientSignsActorContextAndDoesNotTrustBodyIdentity(t *testing.T) {
	var received ActorContext
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer service-token" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		encoded := request.Header.Get("X-Luna-Actor-Context")
		payload, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(payload, &received); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"ok":true}`)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "service-token", "actor-key")
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return time.Unix(100, 0) }
	response, err := client.Do(context.Background(), ActorContext{
		UserID: "usr_session", SessionID: "sess_1", Locale: "zh-CN", RequestID: "req_1",
	}, Request{Method: http.MethodPost, Path: "/internal/v1/conversations", Body: []byte(`{"title":"test"}`)})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if received.UserID != "usr_session" || received.SessionID != "sess_1" {
		t.Fatalf("actor = %#v", received)
	}
	if received.IssuedAt != 100 || received.ExpiresAt != 160 {
		t.Fatalf("actor timestamps = %d..%d", received.IssuedAt, received.ExpiresAt)
	}
}

func TestConfigDefaultsFailClosed(t *testing.T) {
	t.Setenv("AI_ASSISTANT_AVAILABLE", "")
	t.Setenv("AI_AGENT_BASE_URL", "")
	t.Setenv("AI_INTERNAL_SECRET", "")
	config := LoadConfig()
	client, err := config.Client()
	if config.Available || client != nil || err != nil {
		t.Fatal("AI agent must be disabled by default")
	}
}

func TestEnabledConfigReportsClientInitializationError(t *testing.T) {
	config := Config{Available: true, BaseURL: "://invalid", ServiceToken: "service-token", ActorSigningKey: "actor-key"}
	client, err := config.Client()
	if client != nil || err == nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("client = %#v, error = %v", client, err)
	}
}

func TestHTTPClientRejectsMissingTrustMaterial(t *testing.T) {
	_, err := NewHTTPClient("http://agent.internal", "", "key")
	if err == nil || !strings.Contains(err.Error(), "trust material") {
		t.Fatalf("error = %v", err)
	}
}

func TestConfigClientUsesConfiguredTimeout(t *testing.T) {
	config := Config{Available: true, BaseURL: "http://agent.internal", ServiceToken: "service-token", ActorSigningKey: "actor-key", Timeout: "23s"}
	client, err := config.Client()
	if err != nil {
		t.Fatal(err)
	}
	httpClient := client.(*HTTPClient)
	if httpClient.httpClient.Timeout != 23*time.Second || httpClient.streamClient.Timeout != 0 {
		t.Fatalf("timeouts = %s, %s", httpClient.httpClient.Timeout, httpClient.streamClient.Timeout)
	}
}

func TestConfigClientRejectsInvalidTimeout(t *testing.T) {
	config := Config{Available: true, BaseURL: "http://agent.internal", ServiceToken: "service-token", ActorSigningKey: "actor-key", Timeout: "later"}
	_, err := config.Client()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
}
