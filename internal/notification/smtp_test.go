package notification

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestSMTPAdapterValidatesConfig(t *testing.T) {
	adapter := SMTPAdapter{}
	config := json.RawMessage(`{
		"host": "smtp.example.com",
		"port": 587,
		"security": "starttls",
		"from": "DevOps <devops@example.com>",
		"to": ["ops@example.com"]
	}`)
	if err := adapter.Validate(context.Background(), config, nil); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestSMTPAdapterAllowsReusableSenderAccountAndRequiresRecipientOnSend(t *testing.T) {
	adapter := SMTPAdapter{}
	reusableAccount := json.RawMessage(`{"host":"smtp.example.com","from":"devops@example.com"}`)
	if err := adapter.Validate(context.Background(), reusableAccount, nil); err != nil {
		t.Fatalf("expected reusable SMTP sender account to validate without default recipient, got: %v", err)
	}
	if _, err := adapter.Send(context.Background(), reusableAccount, nil, RenderedMessage{Subject: "test", Body: "body"}, nil); err == nil {
		t.Fatal("expected Send without effective recipient to fail")
	}
	if err := adapter.Validate(context.Background(), json.RawMessage(`{"host":"127.0.0.1","from":"devops@example.com"}`), nil); err == nil {
		t.Fatal("expected PublicEgressPolicy to block loopback SMTP host by default")
	}
}

func TestSMTPAdapterRendersSubjectAndBody(t *testing.T) {
	adapter := SMTPAdapter{}
	event := Event{
		Type:       "release.failed",
		Severity:   SeverityError,
		Message:    "rollout failed",
		OccurredAt: time.Now(),
	}
	message, err := adapter.Render(context.Background(), event, Template{
		Subject: "[{{.Event.Severity}}] {{.Event.Type}}",
		Body:    "{{.Event.Message}}",
	}, nil, nil, nil, "")
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if message.Subject != "[error] release.failed" || message.Body != "rollout failed" {
		t.Fatalf("message = %#v", message)
	}
}

func TestSMTPAdapterRendersRichHTMLMultipartForDefaultTemplateWithTimeZone(t *testing.T) {
	adapter := SMTPAdapter{}
	exit137 := int32(137)
	event := Event{
		Type:             "runtime.pod_oom_killed",
		Severity:         SeverityError,
		Project:          EntityRef{Name: "轻雪空间"},
		Application:      EntityRef{Name: "OpenList"},
		DeploymentTarget: EntityRef{Name: "生产环境"},
		Runtime: RuntimeIncidentContext{
			PodName:          "openlist-abc",
			ContainerName:    "app",
			Reason:           "OOMKilled",
			ExitCode:         &exit137,
			RestartCount:     4,
			DiagnosticDetail: "Killed due to cgroup memory limit",
		},
		Links:      map[string]string{"primary": "https://devops.example.com/projects/p1/applications/a1?tab=deployments#targetId=t1"},
		OccurredAt: time.Date(2026, 8, 28, 2, 30, 0, 0, time.UTC),
		Message:    "容器超出内存限制被终止",
	}
	defaultTpl := TemplateFromModel(DefaultTemplateFor(AdapterKindSMTP, event.Type, "zh-CN"))
	ctx := WithTimeZone(context.Background(), "Asia/Shanghai")
	message, err := adapter.Render(ctx, event, defaultTpl, nil, nil, nil, "zh-CN")
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(message.Subject, "OOMKilled") || !strings.Contains(message.Subject, "OpenList") {
		t.Fatalf("unexpected subject: %q", message.Subject)
	}
	if !strings.Contains(message.HTMLBody, "2026-08-28 10:30:00 CST") {
		t.Fatalf("expected Asia/Shanghai formatted timestamp in HTMLBody:\n%s", message.HTMLBody)
	}
	if !strings.Contains(message.HTMLBody, "ExitCode: 137") || !strings.Contains(message.Body, "ExitCode: 137") {
		t.Fatalf("expected diagnostic exit code in HTML and text body")
	}
}

func TestBuildSMTPMessageUsesMultipartAlternativeForHTML(t *testing.T) {
	raw := buildSMTPMessage(SMTPConfig{
		From: "Luna DevOps <devops@example.com>",
		To:   []string{"operator@example.com"},
		Bcc:  []string{"audit@example.com"},
	}, RenderedMessage{
		Subject:  "发布失败",
		Body:     "plain fallback",
		HTMLBody: "<!doctype html><html><body><strong>rich body</strong></body></html>",
	})

	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v\n%s", err, raw)
	}
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("ParseMediaType() error = %v", err)
	}
	if mediaType != "multipart/alternative" || params["boundary"] == "" {
		t.Fatalf("Content-Type = %q, params = %#v", mediaType, params)
	}
	if message.Header.Get("Bcc") != "" {
		t.Fatalf("Bcc header must not be emitted: %q", message.Header.Get("Bcc"))
	}

	parts := map[string]string{}
	reader := multipart.NewReader(message.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}
		partType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("part Content-Type error = %v", err)
		}
		decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
		if err != nil {
			t.Fatalf("decode %s part: %v", partType, err)
		}
		parts[partType] = strings.TrimSpace(string(decoded))
	}
	if parts["text/plain"] != "plain fallback" {
		t.Fatalf("text/plain = %q", parts["text/plain"])
	}
	if parts["text/html"] != "<!doctype html><html><body><strong>rich body</strong></body></html>" {
		t.Fatalf("text/html = %q", parts["text/html"])
	}
}

func TestBuildSMTPMessageKeepsPlainTextForMessagesWithoutHTML(t *testing.T) {
	raw := buildSMTPMessage(SMTPConfig{
		From: "devops@example.com",
		To:   []string{"operator@example.com"},
	}, RenderedMessage{Subject: "test", Body: "plain only"})

	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	mediaType, _, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("ParseMediaType() error = %v", err)
	}
	if mediaType != "text/plain" {
		t.Fatalf("Content-Type = %q", mediaType)
	}
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, message.Body))
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if strings.TrimSpace(string(decoded)) != "plain only" {
		t.Fatalf("body = %q", decoded)
	}
}
