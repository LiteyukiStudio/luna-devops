package aiagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
)

func TestLocalAgentCommunication(t *testing.T) {
	baseURL := os.Getenv("AI_AGENT_INTEGRATION_URL")
	if baseURL == "" {
		t.Skip("AI_AGENT_INTEGRATION_URL is not set")
	}
	keys, err := LoadInternalKeys()
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewHTTPClient(baseURL, keys.ServiceToken, keys.ActorSigningKey)
	if err != nil {
		t.Fatal(err)
	}
	actor := ActorContext{
		UserID: "usr_local_smoke", SessionID: "sess_local_smoke",
		Locale: "zh-CN", RequestID: "req_local_smoke",
	}
	response, err := client.Do(context.Background(), actor, Request{
		Method: http.MethodGet, Path: "/internal/v1/capabilities",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var capabilities struct {
		Available bool `json:"available"`
	}
	if response.StatusCode != http.StatusOK || json.Unmarshal(body, &capabilities) != nil || !capabilities.Available {
		t.Fatalf("capabilities response = %d %s", response.StatusCode, body)
	}

	response, err = client.Do(context.Background(), actor, Request{
		Method: http.MethodPost, Path: "/internal/v1/conversations",
		ContentType: "application/json", Body: []byte(`{"title":"本地通信验证"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var conversation struct {
		ID string `json:"id"`
	}
	if response.StatusCode != http.StatusCreated || json.Unmarshal(body, &conversation) != nil || conversation.ID == "" {
		t.Fatalf("conversation response = %d %s", response.StatusCode, body)
	}
}
