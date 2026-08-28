package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

type SMTPAdapter struct{}

type SMTPConfig struct {
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	Security  string   `json:"security"`
	Username  string   `json:"username"`
	From      string   `json:"from"`
	To        []string `json:"to"`
	Cc        []string `json:"cc"`
	Bcc       []string `json:"bcc"`
	Timeout   int      `json:"timeoutSeconds"`
	Password  string   `json:"password,omitempty"`
	SecretRef string   `json:"passwordRef,omitempty"`
}

func (SMTPAdapter) Kind() string {
	return AdapterKindSMTP
}

func (SMTPAdapter) Validate(ctx context.Context, config json.RawMessage, secretResolver SecretResolver) error {
	cfg, err := parseSMTPConfig(config)
	if err != nil {
		return err
	}
	if cfg.password(ctx, secretResolver) == "" && cfg.Username != "" {
		return fmt.Errorf("smtp password is required when username is set")
	}
	return nil
}

func (SMTPAdapter) Render(ctx context.Context, event Event, tpl Template, _ json.RawMessage, secrets json.RawMessage, secretResolver SecretResolver, _ string) (RenderedMessage, error) {
	return renderMessage(event, tpl, resolveSecretMap(ctx, secrets, secretResolver))
}

func (adapter SMTPAdapter) Send(ctx context.Context, config json.RawMessage, _ json.RawMessage, message RenderedMessage, secretResolver SecretResolver) (SendResult, error) {
	cfg, err := parseSMTPConfig(config)
	if err != nil {
		return SendResult{}, err
	}
	if strings.TrimSpace(message.Subject) == "" {
		return SendResult{}, fmt.Errorf("smtp subject is required")
	}
	if strings.TrimSpace(message.Body) == "" {
		return SendResult{}, fmt.Errorf("smtp body is required")
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if err := sendSMTP(ctx, cfg, message, secretResolver, timeout); err != nil {
		return SendResult{}, err
	}
	return SendResult{StatusCode: 250, ResponseSnippet: "sent"}, nil
}

func (adapter SMTPAdapter) Test(ctx context.Context, config json.RawMessage, secrets json.RawMessage, resolver SecretResolver) error {
	event := TestEvent(time.Now())
	template := DefaultTemplateFor(AdapterKindSMTP, event.Type, event.Locale)
	message, err := adapter.Render(ctx, event, TemplateFromModel(template), config, secrets, resolver, "")
	if err != nil {
		return err
	}
	_, err = adapter.Send(ctx, config, secrets, message, resolver)
	return err
}

func parseSMTPConfig(raw json.RawMessage) (SMTPConfig, error) {
	var cfg SMTPConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return SMTPConfig{}, err
	}
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Security = strings.ToLower(strings.TrimSpace(cfg.Security))
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.From = strings.TrimSpace(cfg.From)
	if cfg.Port <= 0 {
		cfg.Port = 587
	}
	if cfg.Security == "" {
		cfg.Security = "starttls"
	}
	if cfg.Host == "" {
		return SMTPConfig{}, fmt.Errorf("smtp host is required")
	}
	if cfg.From == "" {
		return SMTPConfig{}, fmt.Errorf("smtp from is required")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return SMTPConfig{}, fmt.Errorf("smtp from is invalid: %w", err)
	}
	recipients := append(append([]string{}, cfg.To...), append(cfg.Cc, cfg.Bcc...)...)
	if len(recipients) == 0 {
		return SMTPConfig{}, fmt.Errorf("smtp recipient is required")
	}
	for _, recipient := range recipients {
		if _, err := mail.ParseAddress(strings.TrimSpace(recipient)); err != nil {
			return SMTPConfig{}, fmt.Errorf("smtp recipient is invalid: %w", err)
		}
	}
	switch cfg.Security {
	case "none", "starttls", "tls":
	default:
		return SMTPConfig{}, fmt.Errorf("smtp security must be none, starttls, or tls")
	}
	return cfg, nil
}

func (cfg SMTPConfig) password(ctx context.Context, resolver SecretResolver) string {
	if strings.TrimSpace(cfg.Password) != "" {
		return cfg.Password
	}
	if resolver == nil {
		return ""
	}
	return resolver.ResolveContext(ctx, cfg.SecretRef)
}

func sendSMTP(ctx context.Context, cfg SMTPConfig, message RenderedMessage, resolver SecretResolver, timeout time.Duration) error {
	operationCtx, end := telemetry.StartOperation(ctx, "notification", "smtp.send",
		attribute.String("server.address", cfg.Host),
		attribute.Int("server.port", cfg.Port),
		attribute.String("network.transport", "tcp"),
	)
	err := sendSMTPMessage(operationCtx, cfg, message, resolver, timeout)
	end(err)
	return err
}

func sendSMTPMessage(ctx context.Context, cfg SMTPConfig, message RenderedMessage, resolver SecretResolver, timeout time.Duration) error {
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := net.Dialer{Timeout: timeout}
	var conn net.Conn
	var err error
	if cfg.Security == "tls" {
		conn, err = tls.DialWithDialer(&dialer, "tcp", address, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if cfg.Security == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	password := cfg.password(ctx, resolver)
	if cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.Username, password, cfg.Host)); err != nil {
			return err
		}
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return err
	}
	if err := client.Mail(from.Address); err != nil {
		return err
	}
	recipients := append(append([]string{}, cfg.To...), append(cfg.Cc, cfg.Bcc...)...)
	for _, recipient := range recipients {
		if err := client.Rcpt(strings.TrimSpace(recipient)); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(buildSMTPMessage(cfg, message)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildSMTPMessage(cfg SMTPConfig, message RenderedMessage) []byte {
	var out bytes.Buffer
	writeHeader := func(key string, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(&out, "%s: %s\r\n", key, value)
		}
	}
	writeHeader("From", cfg.From)
	writeHeader("To", strings.Join(cfg.To, ", "))
	writeHeader("Cc", strings.Join(cfg.Cc, ", "))
	writeHeader("Subject", mimeEncoded(message.Subject))
	writeHeader("MIME-Version", "1.0")
	if strings.TrimSpace(message.HTMLBody) != "" {
		writeSMTPMultipartAlternative(&out, writeHeader, message)
		return out.Bytes()
	}
	writeHeader("Content-Type", `text/plain; charset="UTF-8"`)
	writeHeader("Content-Transfer-Encoding", "base64")
	out.WriteString("\r\n")
	writeSMTPBase64(&out, message.Body)
	return out.Bytes()
}

func writeSMTPMultipartAlternative(out *bytes.Buffer, writeHeader func(string, string), message RenderedMessage) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writeHeader("Content-Type", `multipart/alternative; boundary="`+writer.Boundary()+`"`)
	out.WriteString("\r\n")

	writeSMTPAlternativePart(writer, `text/plain; charset="UTF-8"`, message.Body)
	writeSMTPAlternativePart(writer, `text/html; charset="UTF-8"`, message.HTMLBody)
	_ = writer.Close()
	out.Write(body.Bytes())
}

func writeSMTPAlternativePart(writer *multipart.Writer, contentType string, body string) {
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", contentType)
	header.Set("Content-Transfer-Encoding", "base64")
	part, err := writer.CreatePart(header)
	if err != nil {
		return
	}
	writeSMTPBase64(part, body)
}

func writeSMTPBase64(writer io.Writer, body string) {
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	for len(encoded) > 76 {
		_, _ = io.WriteString(writer, encoded[:76])
		_, _ = io.WriteString(writer, "\r\n")
		encoded = encoded[76:]
	}
	_, _ = io.WriteString(writer, encoded)
	_, _ = io.WriteString(writer, "\r\n")
}

func mimeEncoded(value string) string {
	return mimeWordEncoder.Encode("utf-8", value)
}

var mimeWordEncoder = mimeBEncoding{}

type mimeBEncoding struct{}

func (mimeBEncoding) Encode(charset string, value string) string {
	if value == "" {
		return ""
	}
	return "=?utf-8?B?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}
