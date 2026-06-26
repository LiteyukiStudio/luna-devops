package api

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
)

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

func TestAuthProviderResponseHidesStoredClientSecret(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	provider := model.AuthProvider{ClientSecretRef: secret.Encrypt("super-secret")}

	output := authProviderResponse(provider)

	if output.ClientSecretRef != "" {
		t.Fatalf("expected stored client secret ref to be hidden, got %q", output.ClientSecretRef)
	}
	if !output.ClientSecretSet {
		t.Fatal("expected clientSecretSet to be true")
	}
}

func TestBuildVariableSetResponseHidesVariablesWithoutInspectPermission(t *testing.T) {
	h := &Handlers{}
	set := model.BuildVariableSet{
		ID:        "bvs_test",
		Scope:     "global",
		Variables: `{"PUBLIC_FLAG":"true","API_URL":"https://api.example.com"}`,
	}

	output := h.buildVariableSetResponseForUser(model.User{ID: "usr_member", Role: "user"}, set)

	if output.CanInspectVariables {
		t.Fatal("expected regular user to be unable to inspect global build variables")
	}
	if output.Variables != "{}" {
		t.Fatalf("expected variables to be hidden, got %q", output.Variables)
	}
	if output.VariableCount != 2 {
		t.Fatalf("expected variable count to remain visible, got %d", output.VariableCount)
	}
}

func TestBuildVariableSetResponseShowsVariablesWithInspectPermission(t *testing.T) {
	h := &Handlers{}
	set := model.BuildVariableSet{
		ID:        "bvs_test",
		Scope:     "user",
		OwnerRef:  "usr_owner",
		Variables: `{"PUBLIC_FLAG":"true"}`,
	}

	output := h.buildVariableSetResponseForUser(model.User{ID: "usr_owner", Role: "user"}, set)

	if !output.CanInspectVariables {
		t.Fatal("expected owner to inspect personal build variables")
	}
	if output.Variables != set.Variables {
		t.Fatalf("expected variables to be visible, got %q", output.Variables)
	}
	if output.VariableCount != 1 {
		t.Fatalf("expected variable count to be 1, got %d", output.VariableCount)
	}
}

func TestAuthProviderFromInputPreservesExistingSecret(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	existingSecretRef := secret.Encrypt("old-secret")
	provider, ok := authProviderFromInput(authProviderInput{
		Type:      "oidc",
		Name:      "Casdoor",
		IssuerURL: "https://sso.example.com",
		ClientID:  "devops",
	}, "ap_existing", existingSecretRef)

	if !ok {
		t.Fatal("expected auth provider input to be valid")
	}
	if provider.ClientSecretRef != existingSecretRef {
		t.Fatalf("expected existing secret ref to be preserved, got %q", provider.ClientSecretRef)
	}
}

func TestResolveSecretSupportsStoredAndEnvRefsOnly(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	t.Setenv("OIDC_TEST_SECRET", "env-secret")
	h := &Handlers{}

	if secret := h.resolveSecret(secret.Encrypt("stored-secret")); secret != "stored-secret" {
		t.Fatalf("expected stored secret, got %q", secret)
	}
	if secret := h.resolveSecret("literal:literal-secret"); secret != "" {
		t.Fatalf("expected literal secret ref to be rejected, got %q", secret)
	}
	if secret := h.resolveSecret("plain-secret"); secret != "" {
		t.Fatalf("expected bare secret ref to be rejected, got %q", secret)
	}
	if secret := h.resolveSecret("env:OIDC_TEST_SECRET"); secret != "env-secret" {
		t.Fatalf("expected env secret, got %q", secret)
	}
}
