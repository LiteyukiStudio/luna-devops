package api

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBootstrapStatusHidesDevLoginHintInProduction(t *testing.T) {
	t.Setenv("LOCAL_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("LOCAL_ADMIN_PASSWORD", "secret-password")

	status := bootstrapStatusResponse("production", false)

	if status["devLoginEnabled"] != false {
		t.Fatalf("expected dev login disabled in production, got %v", status["devLoginEnabled"])
	}
	if _, ok := status["devLoginHint"]; ok {
		t.Fatal("expected production bootstrap status to omit devLoginHint")
	}
}

func TestBootstrapStatusIncludesDevLoginHintInDevelopment(t *testing.T) {
	t.Setenv("LOCAL_ADMIN_EMAIL", "Admin@Example.com")
	t.Setenv("LOCAL_ADMIN_PASSWORD", "secret-password")

	status := bootstrapStatusResponse("development", true)

	if status["devLoginEnabled"] != true {
		t.Fatalf("expected dev login enabled in development, got %v", status["devLoginEnabled"])
	}
	hint, ok := status["devLoginHint"].(gin.H)
	if !ok {
		t.Fatalf("expected devLoginHint map, got %T", status["devLoginHint"])
	}
	if hint["email"] != "admin@example.com" {
		t.Fatalf("expected normalized dev email, got %q", hint["email"])
	}
	if hint["password"] != "secret-password" {
		t.Fatalf("expected configured dev password, got %q", hint["password"])
	}
}
