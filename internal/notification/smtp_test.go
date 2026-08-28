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

func TestSMTPAdapterRequiresRecipient(t *testing.T) {
	adapter := SMTPAdapter{}
	err := adapter.Validate(context.Background(), json.RawMessage(`{"host":"smtp.example.com","from":"devops@example.com"}`), nil)
	if err == nil {
		t.Fatal("expected missing recipient to fail")
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
