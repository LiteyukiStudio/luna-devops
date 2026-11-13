package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	SigningMode          string            `json:"signingMode,omitempty"`
	TestJSONBodyTemplate string            `json:"testJsonBodyTemplate"`
}

type WebhookBusinessError struct {
	Code    int64
	Message string
}

func (e *WebhookBusinessError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return fmt.Sprintf("webhook returned error code %d: %s", e.Code, strings.TrimSpace(e.Message))
	}
	return fmt.Sprintf("webhook returned error code %d", e.Code)
}

func (WebhookAdapter) Kind() string {
	return AdapterKindWebhook
}

func (adapter WebhookAdapter) Validate(ctx context.Context, config json.RawMessage, _ SecretResolver) error {
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
	_, err = EgressPolicyFromContext(ctx).ValidateURL(cfg.URL)
	return err
}

func (WebhookAdapter) Render(ctx context.Context, event Event, tpl Template, config json.RawMessage, secrets json.RawMessage, secretResolver SecretResolver, _ string) (RenderedMessage, error) {
	cfg, err := parseWebhookConfig(config)
	if err != nil {
		return RenderedMessage{}, err
	}
	secretValues := resolveSecretMap(ctx, secrets, secretResolver)
	timeZone := TimeZoneFromContext(ctx)
	message, err := renderMessageWithTimeZone(event, tpl, secretValues, timeZone)
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
	if _, err := EgressPolicyFromContext(ctx).ValidateURL(message.URL); err != nil {
		return RenderedMessage{}, err
	}
	return message, nil
}

func (adapter WebhookAdapter) Send(ctx context.Context, config json.RawMessage, secrets json.RawMessage, message RenderedMessage, secretResolver SecretResolver) (SendResult, error) {
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
	policy := EgressPolicyFromContext(ctx)
	rawURL := firstNonEmpty(message.URL, cfg.URL)
	parsedURL, err := policy.ValidateURL(rawURL)
	if err != nil {
		return SendResult{}, err
	}

	payloadJSON := message.JSON
	secretValues := resolveSecretMap(ctx, secrets, secretResolver)
	if signSecret := strings.TrimSpace(secretValues["SignSecret"]); signSecret != "" {
		parsedURL, payloadJSON, err = applyWebhookSignature(parsedURL, payloadJSON, cfg.SigningMode, signSecret, time.Now())
		if err != nil {
			return SendResult{}, err
		}
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	req, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), bytes.NewReader(payloadJSON))
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
	client := newWebhookHTTPClientWithPolicy(policy, timeout)
	resp, err := client.Do(req)
	if err != nil {
		return SendResult{}, err
	}
	defer resp.Body.Close()
	snippet := responseSnippet(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SendResult{StatusCode: resp.StatusCode, ResponseSnippet: snippet}, fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	if bizErr := webhookResponseBusinessError(snippet); bizErr != nil {
		return SendResult{StatusCode: resp.StatusCode, ResponseSnippet: snippet}, bizErr
	}
	return SendResult{StatusCode: resp.StatusCode, ResponseSnippet: snippet}, nil
}

func newWebhookHTTPClient(timeout time.Duration) *http.Client {
	return newWebhookHTTPClientWithPolicy(security.PublicEgressPolicy(), timeout)
}

func newWebhookHTTPClientWithPolicy(policy security.EgressPolicy, timeout time.Duration) *http.Client {
	client := security.NewHTTPClient(policy, timeout)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}

func applyWebhookSignature(parsedURL *url.URL, payloadJSON []byte, signingMode string, signSecret string, now time.Time) (*url.URL, []byte, error) {
	mode := resolveWebhookSigningMode(signingMode, parsedURL.Hostname())
	switch mode {
	case "feishu":
		signedPayload, err := signFeishuWebhookPayload(payloadJSON, signSecret, now.Unix())
		if err != nil {
			return nil, nil, err
		}
		return parsedURL, signedPayload, nil
	case "dingtalk":
		clonedURL := *parsedURL
		query := clonedURL.Query()
		timestampMillis := now.UnixMilli()
		stringToSign := fmt.Sprintf("%d\n%s", timestampMillis, signSecret)
		mac := hmac.New(sha256.New, []byte(signSecret))
		_, _ = mac.Write([]byte(stringToSign))
		signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		query.Set("timestamp", strconv.FormatInt(timestampMillis, 10))
		query.Set("sign", signature)
		clonedURL.RawQuery = query.Encode()
		return &clonedURL, payloadJSON, nil
	default:
		return parsedURL, payloadJSON, nil
	}
}

func resolveWebhookSigningMode(explicitMode string, hostname string) string {
	switch strings.ToLower(strings.TrimSpace(explicitMode)) {
	case "feishu", "lark":
		return "feishu"
	case "dingtalk":
		return "dingtalk"
	}
	host := strings.ToLower(strings.TrimSpace(hostname))
	switch {
	case host == "open.feishu.cn" || strings.HasSuffix(host, ".feishu.cn") ||
		host == "open.larksuite.com" || strings.HasSuffix(host, ".larksuite.com") ||
		host == "open.larkoffice.com" || strings.HasSuffix(host, ".larkoffice.com"):
		return "feishu"
	case host == "oapi.dingtalk.com" || strings.HasSuffix(host, ".dingtalk.com"):
		return "dingtalk"
	default:
		return ""
	}
}

func signFeishuWebhookPayload(payloadJSON []byte, signSecret string, timestampUnix int64) ([]byte, error) {
	var body map[string]any
	if err := json.Unmarshal(payloadJSON, &body); err != nil {
		return nil, fmt.Errorf("feishu signed payload must be a json object: %w", err)
	}
	stringToSign := fmt.Sprintf("%d\n%s", timestampUnix, signSecret)
	mac := hmac.New(sha256.New, []byte(stringToSign))
	body["timestamp"] = strconv.FormatInt(timestampUnix, 10)
	body["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return json.Marshal(body)
}

func webhookResponseBusinessError(snippet string) error {
	trimmed := strings.TrimSpace(snippet)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}
	for _, key := range []string{"code", "errcode", "StatusCode"} {
		rawCode, exists := payload[key]
		if !exists {
			continue
		}
		num, ok := rawCode.(json.Number)
		if !ok {
			continue
		}
		code, err := num.Int64()
		if err != nil || code == 0 {
			continue
		}
		message := ""
		for _, msgKey := range []string{"msg", "errmsg", "message", "StatusMessage"} {
			if text, ok := payload[msgKey].(string); ok && strings.TrimSpace(text) != "" {
				message = strings.TrimSpace(text)
				break
			}
		}
		return &WebhookBusinessError{Code: code, Message: message}
	}
	return nil
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
	cfg.SigningMode = strings.ToLower(strings.TrimSpace(cfg.SigningMode))
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
