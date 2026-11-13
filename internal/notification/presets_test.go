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

func TestPersonalWebhookPresetsExcludeUserControlledDestinations(t *testing.T) {
	personal := PersonalWebhookPresets()
	for _, preset := range personal {
		if preset.ID == "gotify" {
			t.Fatal("personal presets must not expose the user-controlled Gotify host")
		}
	}
	if len(personal) != len(WebhookPresets())-1 {
		t.Fatalf("personal preset count = %d, all preset count = %d", len(personal), len(WebhookPresets()))
	}
}

func TestFeishuInteractiveCardRendersSemanticColorsAndDiagnostics(t *testing.T) {
	exit137 := int32(137)
	cases := []struct {
		name      string
		event     Event
		locale    string
		wantColor string
		wantText  string
	}{
		{
			name: "runtime oom error red",
			event: Event{
				Type:             "runtime.pod_oom_killed",
				Severity:         SeverityError,
				Project:          EntityRef{Name: "轻雪空间"},
				Application:      EntityRef{Name: "OpenList"},
				DeploymentTarget: EntityRef{Name: "生产环境"},
				Runtime: RuntimeIncidentContext{
					PodName:          "openlist-7d8f9-abc",
					ContainerName:    "app",
					Reason:           "OOMKilled",
					ExitCode:         &exit137,
					RestartCount:     5,
					ReadyReplicas:    0,
					DesiredReplicas:  2,
					DiagnosticDetail: "Container exceeded memory limit (512Mi)",
				},
				Links:      map[string]string{"primary": "https://devops.example.com/projects/prj_1/applications/app_1?tab=deployments#targetId=dplt_1"},
				OccurredAt: time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC),
				Message:    "Pod openlist-7d8f9-abc 因 OOMKilled 被终止",
			},
			locale:    "zh",
			wantColor: "red",
			wantText:  "ExitCode: 137",
		},
		{
			name: "runtime recovered green",
			event: Event{
				Type:             "runtime.recovered",
				Severity:         SeverityInfo,
				Project:          EntityRef{Name: "轻雪空间"},
				Application:      EntityRef{Name: "OpenList"},
				DeploymentTarget: EntityRef{Name: "生产环境"},
				Runtime: RuntimeIncidentContext{
					ReadyReplicas:   2,
					DesiredReplicas: 2,
					DurationSeconds: 185,
				},
				OccurredAt: time.Date(2026, 8, 28, 8, 3, 5, 0, time.UTC),
				Message:    "工作负载已恢复正常",
			},
			locale:    "zh",
			wantColor: "green",
			wantText:  "185s",
		},
		{
			name:      "test notification blue",
			event:     TestEvent(time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)),
			locale:    "en",
			wantColor: "blue",
			wantText:  "View Details",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithTimeZone(context.Background(), "Asia/Shanghai")
			msg, err := (WebhookAdapter{}).Render(ctx, tc.event, Template{
				JSON: feishuInteractiveTemplate(tc.locale),
			}, json.RawMessage(`{"method":"POST","url":"https://open.feishu.cn/open-apis/bot/v2/hook/test"}`), nil, nil, tc.locale)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			var decoded feishuInteractiveEnvelope
			if err := json.Unmarshal(msg.JSON, &decoded); err != nil {
				t.Fatalf("Unmarshal card JSON: %v\n%s", err, msg.JSON)
			}
			if decoded.MsgType != "interactive" {
				t.Fatalf("msg_type = %q, want interactive", decoded.MsgType)
			}
			if decoded.Card.Header.Template != tc.wantColor {
				t.Fatalf("header.template = %q, want %q", decoded.Card.Header.Template, tc.wantColor)
			}
			if !json.Valid(msg.JSON) || !containsSubstring(string(msg.JSON), tc.wantText) {
				t.Fatalf("card JSON missing %q:\n%s", tc.wantText, msg.JSON)
			}
		})
	}
}

func containsSubstring(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && (haystack == needle || indexOfSubstring(haystack, needle) >= 0))
}

func indexOfSubstring(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
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
