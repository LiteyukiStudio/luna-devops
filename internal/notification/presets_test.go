package notification

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestWebhookPresetsRenderValidMessages(t *testing.T) {
	adapter := WebhookAdapter{}
	event := Event{
		ID:          "evt_1",
		Type:        "release.failed",
		Severity:    SeverityError,
		Message:     "rollout failed",
		OccurredAt:  time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		Project:     EntityRef{Name: "Luna"},
		Application: EntityRef{Name: "API"},
		DeploymentTarget: EntityRef{
			Name: "prod",
		},
	}

	for _, preset := range WebhookPresets() {
		t.Run(preset.ID, func(t *testing.T) {
			_, resolver := presetSecretFixtures(preset.SecretFields)
			secretValues := presetSecretValues(preset.SecretFields)
			config := json.RawMessage(preset.ConfigTemplate)
			if err := adapter.Validate(context.Background(), config, resolver); err != nil {
				t.Fatalf("Validate returned error: %v", err)
			}
			var cfg WebhookConfig
			if err := json.Unmarshal(config, &cfg); err != nil {
				t.Fatalf("config is not json: %v", err)
			}
			renderedURL, err := renderTemplate("preset-url", cfg.URL, templateData{Event: event, Secrets: secretValues})
			if err != nil {
				t.Fatalf("render url returned error: %v", err)
			}
			if renderedURL == "" {
				t.Fatal("rendered url is empty")
			}
			message, err := renderMessage(event, Template{JSON: preset.JSONBodyTemplate}, secretValues)
			if err != nil {
				t.Fatalf("renderMessage returned error: %v", err)
			}
			if len(message.JSON) == 0 {
				t.Fatal("rendered json is empty")
			}
			var body any
			if err := json.Unmarshal(message.JSON, &body); err != nil {
				t.Fatalf("rendered body is not json: %v", err)
			}
		})
	}
}

func TestWebhookPresetsIncludePlatformTestTemplates(t *testing.T) {
	for _, preset := range WebhookPresets() {
		t.Run(preset.ID, func(t *testing.T) {
			var cfg WebhookConfig
			if err := json.Unmarshal([]byte(preset.ConfigTemplate), &cfg); err != nil {
				t.Fatalf("config is not json: %v", err)
			}
			if cfg.TestJSONBodyTemplate == "" {
				t.Fatal("test body template is empty")
			}
			if _, err := renderMessage(TestEvent(time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)), Template{JSON: cfg.TestJSONBodyTemplate}, nil); err != nil {
				t.Fatalf("test template does not render: %v", err)
			}
		})
	}
}

func presetSecretFixtures(fields []string) (json.RawMessage, StaticSecretResolver) {
	secretRefs := map[string]string{}
	resolver := StaticSecretResolver{}
	values := map[string]string{
		"WebhookToken": "token",
		"WebhookKey":   "key",
		"GotifyHost":   "gotify.example.com",
		"AppToken":     "app-token",
		"AccessToken":  "access-token",
		"WebhookPath":  "T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
		"WebhookID":    "1234567890",
	}
	for _, field := range fields {
		ref := field + "-ref"
		secretRefs[field] = ref
		if value := values[field]; value != "" {
			resolver[ref] = value
			continue
		}
		resolver[ref] = "secret"
	}
	data, _ := json.Marshal(secretRefs)
	return data, resolver
}

func presetSecretValues(fields []string) map[string]string {
	values := map[string]string{}
	_, resolver := presetSecretFixtures(fields)
	for _, field := range fields {
		values[field] = resolver.ResolveContext(context.Background(), field+"-ref")
	}
	return values
}
