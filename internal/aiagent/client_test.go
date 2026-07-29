package aiagent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJWTHTTPClientUsesIndependentShortLivedEdDSATokens(t *testing.T) {
	servicePublic, servicePrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	actorPublic, actorPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var serviceClaims, actorClaims map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		serviceClaims = verifyTestJWT(t, strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "), servicePublic)
		actorClaims = verifyTestJWT(t, request.Header.Get("X-Luna-Actor-Context"), actorPublic)
		if request.Header.Get("X-Luna-Actor-Signature") != "" {
			t.Fatal("detached HMAC signature must not accompany JWT Actor Context")
		}
		_, _ = io.WriteString(writer, `{}`)
	}))
	defer server.Close()

	client, err := NewJWTHTTPClient(server.URL, privateKeyPEM(t, servicePrivate), privateKeyPEM(t, actorPrivate))
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return time.Unix(500, 0) }
	response, err := client.Do(context.Background(), ActorContext{UserID: "usr_1", SessionID: "sess_1", RequestID: "req_1"}, Request{Method: http.MethodGet, Path: "/internal/v1/capabilities"})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if serviceClaims["aud"] != "luna-agent" || serviceClaims["exp"].(float64)-serviceClaims["iat"].(float64) != 60 {
		t.Fatalf("service claims = %#v", serviceClaims)
	}
	actor := actorClaims["actor"].(map[string]any)
	if actor["userId"] != "usr_1" || actor["sessionId"] != "sess_1" {
		t.Fatalf("actor claims = %#v", actorClaims)
	}
}

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
	t.Setenv("AI_AGENT_SERVICE_TOKEN", "")
	t.Setenv("AI_ACTOR_CONTEXT_SIGNING_KEY", "")
	config := LoadConfig()
	if config.Available || config.Client() != nil {
		t.Fatal("AI agent must be disabled by default")
	}
}

func TestHTTPClientRejectsMissingTrustMaterial(t *testing.T) {
	_, err := NewHTTPClient("http://agent.internal", "", "key")
	if err == nil || !strings.Contains(err.Error(), "trust material") {
		t.Fatalf("error = %v", err)
	}
}

func privateKeyPEM(t *testing.T, key ed25519.PrivateKey) string {
	t.Helper()
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
}

func verifyTestJWT(t *testing.T, token string, key ed25519.PublicKey) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT parts = %d", len(parts))
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), signature) {
		t.Fatal("invalid JWT signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}
