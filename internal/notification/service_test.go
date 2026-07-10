package notification

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestDefaultTemplateForChannelUsesWebhookPresetBody(t *testing.T) {
	preset := WebhookPresets()[0]
	channel := model.NotificationChannel{
		AdapterKind: AdapterKindWebhook,
		ConfigJSON:  preset.ConfigTemplate,
	}

	tpl := DefaultTemplateForChannel(channel, Event{Type: "build.failed"}, "")

	if !strings.Contains(tpl.JSONBodyTemplate, `"msg_type": "post"`) {
		t.Fatalf("template did not use preset body: %s", tpl.JSONBodyTemplate)
	}
	if strings.Contains(tpl.JSONBodyTemplate, `"text": "[{{.Event.Severity}}]`) {
		t.Fatalf("template unexpectedly used generic webhook body: %s", tpl.JSONBodyTemplate)
	}
}

func TestDefaultTemplateForChannelFallsBackToGenericWebhookBody(t *testing.T) {
	channel := model.NotificationChannel{
		AdapterKind: AdapterKindWebhook,
		ConfigJSON:  `{"method":"POST","url":"https://example.com/webhook"}`,
	}

	tpl := DefaultTemplateForChannel(channel, Event{Type: "build.failed"}, "")

	if !strings.Contains(tpl.JSONBodyTemplate, `"text":`) {
		t.Fatalf("template did not use generic webhook body: %s", tpl.JSONBodyTemplate)
	}
}
