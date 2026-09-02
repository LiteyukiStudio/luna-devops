package identityapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/crypto/bcrypt"
)

func TestEnsureInitialAdminFailurePreservesTraceAndDoesNotRecordPassword(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	parentContext, parentSpan := provider.Tracer("initial-admin-test").Start(t.Context(), "test.startup")
	parentSpanID := parentSpan.SpanContext().SpanID()
	secretMarker := "initial-admin-password-must-not-enter-telemetry"
	err := EnsureInitialAdmin(parentContext, nil, "production", InitialAdminConfig{Password: secretMarker})
	if !errors.Is(err, ErrInitialAdminDatabaseUnavailable) {
		t.Fatalf("EnsureInitialAdmin() error = %v, want database unavailable", err)
	}
	parentSpan.End()

	var operationSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "auth.initial_admin.ensure" {
			operationSpan = span
			break
		}
	}
	if operationSpan == nil {
		t.Fatal("auth.initial_admin.ensure span was not recorded")
	}
	if operationSpan.Parent().SpanID() != parentSpanID {
		t.Fatalf("operation parent = %s, want %s", operationSpan.Parent().SpanID(), parentSpanID)
	}
	if operationSpan.Status().Code != codes.Error {
		t.Fatalf("operation status = %v, want error", operationSpan.Status().Code)
	}
	for _, attribute := range operationSpan.Attributes() {
		if strings.Contains(attribute.Value.Emit(), secretMarker) {
			t.Fatal("operation attribute leaked the initial administrator password")
		}
	}
	for _, event := range operationSpan.Events() {
		for _, attribute := range event.Attributes {
			if strings.Contains(attribute.Value.Emit(), secretMarker) {
				t.Fatal("operation event leaked the initial administrator password")
			}
		}
	}
}

func TestInitialAdminUserNormalizesConfiguration(t *testing.T) {
	user, err := initialAdminUser(InitialAdminConfig{
		Email: "  Admin@Example.com ", Name: " ", Password: " secure password ",
	})
	if err != nil {
		t.Fatalf("initialAdminUser() error = %v", err)
	}
	if user.Email != "admin@example.com" || user.Name != user.Email {
		t.Fatalf("identity = %q / %q", user.Email, user.Name)
	}
	if user.Role != authz.PlatformRoleAdmin || user.Language != "zh-CN" {
		t.Fatalf("role/language = %q / %q", user.Role, user.Language)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(" secure password ")); err != nil {
		t.Fatalf("password was not preserved exactly: %v", err)
	}
}

func TestInitialAdminUserRejectsInvalidConfigurationWithoutLeakingPassword(t *testing.T) {
	secret := "unique-password-that-must-not-leak"
	testCases := []struct {
		name  string
		input InitialAdminConfig
	}{
		{name: "missing email", input: InitialAdminConfig{Password: secret}},
		{name: "display address", input: InitialAdminConfig{Email: "Admin <admin@example.com>", Password: secret}},
		{name: "short password", input: InitialAdminConfig{Email: "admin@example.com", Password: "1234567"}},
		{name: "long password", input: InitialAdminConfig{Email: "admin@example.com", Password: strings.Repeat("x", 73)}},
		{name: "unsupported language", input: InitialAdminConfig{Email: "admin@example.com", Password: secret, Language: "ja-JP"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := initialAdminUser(testCase.input)
			if !errors.Is(err, ErrInitialAdminConfigInvalid) {
				t.Fatalf("initialAdminUser() error = %v, want config invalid", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("configuration error leaked the password")
			}
		})
	}
}
