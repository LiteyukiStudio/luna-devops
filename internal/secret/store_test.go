package secret

import (
	"errors"
	"testing"
)

func TestValidateEncryptionConfigRequiresKeyInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SECRET_ENCRYPTION_KEY", "")

	if err := ValidateEncryptionConfig(); !errors.Is(err, ErrMissingEncryptionKey) {
		t.Fatalf("ValidateEncryptionConfig() error = %v, want ErrMissingEncryptionKey", err)
	}
}

func TestEncryptDoesNotPanicWhenProductionKeyMissing(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SECRET_ENCRYPTION_KEY", "")

	if got := Encrypt("secret"); got != "" {
		t.Fatalf("Encrypt() = %q, want empty ref when key is missing", got)
	}
}

func TestDevelopmentUsesLocalEncryptionFallback(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "")

	ref := Encrypt("secret")
	if ref == "" {
		t.Fatal("Encrypt() returned empty ref in development")
	}
	if got := ResolveInline(ref); got != "secret" {
		t.Fatalf("ResolveInline() = %q, want secret", got)
	}
}
