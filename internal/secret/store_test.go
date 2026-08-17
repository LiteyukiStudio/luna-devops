package secret

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGenerateUsesBoundedRequestedEncoding(t *testing.T) {
	for _, test := range []struct {
		encoding string
		alphabet string
	}{
		{encoding: "hex", alphabet: "0123456789abcdef"},
		{encoding: "base64", alphabet: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"},
		{encoding: "alphanumeric", alphabet: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"},
		{encoding: "numeric", alphabet: "0123456789"},
	} {
		value, err := Generate(32, test.encoding)
		if err != nil {
			t.Fatalf("Generate(%q): %v", test.encoding, err)
		}
		if len(value) != 32 {
			t.Fatalf("Generate(%q) length = %d, want 32", test.encoding, len(value))
		}
		for _, char := range value {
			if !strings.ContainsRune(test.alphabet, char) {
				t.Fatalf("Generate(%q) returned unsupported character %q", test.encoding, char)
			}
		}
	}
}

func TestGenerateRejectsInvalidPolicy(t *testing.T) {
	for _, test := range []struct {
		length   int
		encoding string
	}{
		{length: 7, encoding: "base64"},
		{length: 257, encoding: "base64"},
		{length: 32, encoding: "yaml"},
	} {
		if _, err := Generate(test.length, test.encoding); err == nil {
			t.Fatalf("Generate(%d, %q) returned nil error", test.length, test.encoding)
		}
	}
}

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

func TestStoreContextWithDBUsesExplicitDatabaseHandle(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "runtime-secret-store-test-key")
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}

	auditCalls := 0
	store := NewStore(nil, func(context.Context, string, string, string, bool, string) { auditCalls++ })
	ref, err := store.StoreContextWithDB(t.Context(), db, "transaction-secret", "usr_test", "runtime_config:set_test:runtime:TOKEN")
	if err != nil {
		t.Fatalf("StoreContextWithDB() error = %v", err)
	}
	if !strings.HasPrefix(ref, storedSecretIDPrefix) {
		t.Fatalf("StoreContextWithDB() ref = %q, want stored ref", ref)
	}
	if err := store.DeleteRefContextWithDB(t.Context(), db, ref, "runtime_config:set_test:runtime:TOKEN"); err != nil {
		t.Fatalf("DeleteRefContextWithDB() error = %v", err)
	}
	if auditCalls != 0 {
		t.Fatalf("transaction-bound store emitted %d standalone audits, want aggregate owner audit only", auditCalls)
	}
}
