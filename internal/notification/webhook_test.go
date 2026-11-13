package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestWebhookAdapterRendersURLHeadersAndBody(t *testing.T) {
	adapter := WebhookAdapter{}
	config := json.RawMessage(`{
		"method": "POST",
		"url": "https://8.8.8.8/hooks/{{.Secrets.Token}}",
		"headers": {
			"X-Project": "{{.Event.Project.Identifier}}"
		}
	}`)
	secrets := json.RawMessage(`{"Token":"token-ref"}`)
	event := Event{
		ID:         "evt_1",
		Type:       "hook.failed",
		Severity:   SeverityError,
		Message:    "migration failed",
		OccurredAt: time.Now(),
		Project:    EntityRef{Name: "Demo", Identifier: "demo"},
	}
	message, err := adapter.Render(context.Background(), event, Template{JSON: `{"text": {{json .Event.Message}}}`}, config, secrets, StaticSecretResolver{"token-ref": "abc"}, "")
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if message.URL != "https://8.8.8.8/hooks/abc" {
		t.Fatalf("url = %q", message.URL)
	}
	if message.Headers["X-Project"] != "demo" {
		t.Fatalf("headers = %#v", message.Headers)
	}
	if string(message.JSON) != `{"text":"migration failed"}` {
		t.Fatalf("json = %s", string(message.JSON))
	}
}

func TestWebhookAdapterRejectsUnsafeMethod(t *testing.T) {
	adapter := WebhookAdapter{}
	err := adapter.Validate(context.Background(), json.RawMessage(`{"method":"GET","url":"https://example.com"}`), nil)
	if err == nil {
		t.Fatal("expected unsafe method to fail")
	}
}

func TestWebhookAdapterRegistry(t *testing.T) {
	registry := DefaultRegistry()
	if _, err := registry.Adapter(AdapterKindWebhook); err != nil {
		t.Fatalf("webhook adapter missing: %v", err)
	}
	if _, err := registry.Adapter(AdapterKindSMTP); err != nil {
		t.Fatalf("smtp adapter missing: %v", err)
	}
}

func TestWebhookHTTPClientDoesNotFollowRedirectWithCredentialHeader(t *testing.T) {
	requests := 0
	client := newWebhookHTTPClient(time.Second)
	client.Transport = webhookRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if requests > 1 && request.Header.Get("X-Gotify-Key") != "" {
			t.Fatal("credential header was forwarded to a redirect target")
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://redirect.example/target"}},
			Body:       io.NopCloser(strings.NewReader("redirect")),
			Request:    request,
		}, nil
	})

	request, err := http.NewRequest(http.MethodPost, "https://origin.example/message", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Gotify-Key", "credential-marker")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if requests != 1 || response.StatusCode != http.StatusFound {
		t.Fatalf("requests = %d, status = %d", requests, response.StatusCode)
	}
}

type webhookRoundTripFunc func(*http.Request) (*http.Response, error)

func (run webhookRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return run(request)
}

func TestWebhookSignatureAndBusinessErrorDetection(t *testing.T) {
	now := time.Unix(1700000000, 0)
	feishuURL, err := EgressPolicyFromContext(context.Background()).ValidateURL("https://open.feishu.cn/open-apis/bot/v2/hook/token")
	if err != nil {
		t.Fatalf("ValidateURL error: %v", err)
	}
	_, signedPayload, err := applyWebhookSignature(feishuURL, []byte(`{"msg_type":"interactive"}`), "feishu", "sec_123", now)
	if err != nil {
		t.Fatalf("applyWebhookSignature feishu error: %v", err)
	}
	var feishuBody map[string]any
	if err := json.Unmarshal(signedPayload, &feishuBody); err != nil {
		t.Fatalf("unmarshal signed feishu payload: %v", err)
	}
	if feishuBody["timestamp"] != "1700000000" || feishuBody["sign"] == "" {
		t.Fatalf("unexpected feishu signed body: %#v", feishuBody)
	}

	dingtalkURL, err := EgressPolicyFromContext(context.Background()).ValidateURL("https://oapi.dingtalk.com/robot/send?access_token=abc")
	if err != nil {
		t.Fatalf("ValidateURL dingtalk error: %v", err)
	}
	signedDingtalkURL, _, err := applyWebhookSignature(dingtalkURL, []byte(`{"msgtype":"markdown"}`), "dingtalk", "SEC_abc", now)
	if err != nil {
		t.Fatalf("applyWebhookSignature dingtalk error: %v", err)
	}
	if signedDingtalkURL.Query().Get("timestamp") == "" || signedDingtalkURL.Query().Get("sign") == "" {
		t.Fatalf("expected dingtalk query signature, got %s", signedDingtalkURL.String())
	}

	if err := webhookResponseBusinessError(`{"code":0,"msg":"success"}`); err != nil {
		t.Fatalf("expected code 0 to succeed, got %v", err)
	}
	if err := webhookResponseBusinessError(`{"code":19021,"msg":"sign match fail"}`); err == nil {
		t.Fatal("expected feishu code 19021 to fail")
	}
	if err := webhookResponseBusinessError(`{"errcode":310000,"errmsg":"keywords not in content"}`); err == nil {
		t.Fatal("expected dingtalk errcode 310000 to fail")
	}
}
