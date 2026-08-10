package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/security"
)

type WebhookAdapter struct{}

type WebhookConfig struct {
	Method               string            `json:"method"`
	URL                  string            `json:"url"`
	Headers              map[string]string `json:"headers"`
	Timeout              int               `json:"timeoutSeconds"`
	TestJSONBodyTemplate string            `json:"testJsonBodyTemplate"`
}

func (WebhookAdapter) Kind() string {
	return AdapterKindWebhook
}

func (adapter WebhookAdapter) Validate(_ context.Context, config json.RawMessage, _ SecretResolver) error {
	cfg, err := parseWebhookConfig(config)
	if err != nil {
		return err
	}
	if !allowedWebhookMethod(cfg.Method) {
		return fmt.Errorf("webhook method %q is not allowed", cfg.Method)
	}
	if strings.Contains(cfg.URL, "{{") {
		if !strings.HasPrefix(strings.TrimSpace(cfg.URL), "http://") && !strings.HasPrefix(strings.TrimSpace(cfg.URL), "https://") {
			return fmt.Errorf("webhook url template must start with http or https")
		}
		return nil
	}
	_, err = security.PublicEgressPolicy().ValidateURL(cfg.URL)
	return err
}

func (WebhookAdapter) Render(ctx context.Context, event Event, tpl Template, config json.RawMessage, secrets json.RawMessage, secretResolver SecretResolver, _ string) (RenderedMessage, error) {
	cfg, err := parseWebhookConfig(config)
	if err != nil {
		return RenderedMessage{}, err
	}
	secretValues := resolveSecretMap(ctx, secrets, secretResolver)
	message, err := renderMessage(event, tpl, secretValues)
	if err != nil {
		return RenderedMessage{}, err
	}
	data := templateData{Event: event, Secrets: secretValues}
	method, err := renderTemplate("webhook-method", cfg.Method, data)
	if err != nil {
		return RenderedMessage{}, fmt.Errorf("render webhook method: %w", err)
	}
	urlValue, err := renderTemplate("webhook-url", cfg.URL, data)
	if err != nil {
		return RenderedMessage{}, fmt.Errorf("render webhook url: %w", err)
	}
	headers := map[string]string{}
	for key, value := range cfg.Headers {
		renderedKey, err := renderTemplate("webhook-header-key", key, data)
		if err != nil {
			return RenderedMessage{}, fmt.Errorf("render webhook header key: %w", err)
		}
		renderedValue, err := renderTemplate("webhook-header-value", value, data)
		if err != nil {
			return RenderedMessage{}, fmt.Errorf("render webhook header value: %w", err)
		}
		headers[renderedKey] = renderedValue
	}
	message.Method = normalizeWebhookMethod(method)
	message.URL = strings.TrimSpace(urlValue)
	message.Headers = headers
	if !allowedWebhookMethod(message.Method) {
		return RenderedMessage{}, fmt.Errorf("webhook method %q is not allowed", message.Method)
	}
	if _, err := security.PublicEgressPolicy().ValidateURL(message.URL); err != nil {
		return RenderedMessage{}, err
	}
	return message, nil
}

func (adapter WebhookAdapter) Send(ctx context.Context, config json.RawMessage, _ json.RawMessage, message RenderedMessage, _ SecretResolver) (SendResult, error) {
	cfg, err := parseWebhookConfig(config)
	if err != nil {
		return SendResult{}, err
	}
	method := normalizeWebhookMethod(firstNonEmpty(message.Method, cfg.Method))
	if !allowedWebhookMethod(method) {
		return SendResult{}, fmt.Errorf("webhook method %q is not allowed", method)
	}
	if len(message.JSON) == 0 {
		return SendResult{}, fmt.Errorf("webhook json body is required")
	}
	rawURL := firstNonEmpty(message.URL, cfg.URL)
	parsedURL, err := security.PublicEgressPolicy().ValidateURL(rawURL)
	if err != nil {
		return SendResult{}, err
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	req, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), bytes.NewReader(message.JSON))
	if err != nil {
		return SendResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		key = strings.TrimSpace(key)
		if key == "" || blockedWebhookHeader(key) {
			continue
		}
		req.Header.Set(key, value)
	}
	for key, value := range message.Headers {
		key = strings.TrimSpace(key)
		if key == "" || blockedWebhookHeader(key) {
			continue
		}
		req.Header.Set(key, value)
	}
	client := security.NewHTTPClient(security.PublicEgressPolicy(), timeout)
	resp, err := client.Do(req)
	if err != nil {
		return SendResult{}, err
	}
	defer resp.Body.Close()
	snippet := responseSnippet(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SendResult{StatusCode: resp.StatusCode, ResponseSnippet: snippet}, fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return SendResult{StatusCode: resp.StatusCode, ResponseSnippet: snippet}, nil
}

func (adapter WebhookAdapter) Test(ctx context.Context, config json.RawMessage, secrets json.RawMessage, resolver SecretResolver) error {
	cfg, err := parseWebhookConfig(config)
	if err != nil {
		return err
	}
	event := TestEvent(time.Now())
	testBody := strings.TrimSpace(cfg.TestJSONBodyTemplate)
	if testBody == "" {
		testBody = TemplateFromModel(DefaultTemplateFor(AdapterKindWebhook, event.Type, event.Locale)).JSON
	}
	message, err := adapter.Render(ctx, event, Template{JSON: testBody}, config, secrets, resolver, "")
	if err != nil {
		return err
	}
	_, err = adapter.Send(ctx, config, secrets, message, resolver)
	return err
}

func parseWebhookConfig(raw json.RawMessage) (WebhookConfig, error) {
	var cfg WebhookConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return WebhookConfig{}, err
	}
	cfg.Method = normalizeWebhookMethod(cfg.Method)
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}
	if cfg.URL == "" {
		return WebhookConfig{}, fmt.Errorf("webhook url is required")
	}
	return cfg, nil
}

func normalizeWebhookMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return http.MethodPost
	}
	return method
}

func allowedWebhookMethod(method string) bool {
	switch normalizeWebhookMethod(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func blockedWebhookHeader(header string) bool {
	switch strings.ToLower(strings.TrimSpace(header)) {
	case "host", "content-length", "transfer-encoding", "connection":
		return true
	default:
		return false
	}
}

func responseSnippet(reader io.Reader) string {
	if reader == nil {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(reader, 4096))
	return strings.TrimSpace(string(data))
}
