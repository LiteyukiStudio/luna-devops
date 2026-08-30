package api

import (
	"os"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/secret"
)

func mustTestConfig(t *testing.T) Config {
	t.Helper()
	if os.Getenv("PUBLIC_BASE_URL") == "" {
		t.Setenv("PUBLIC_BASE_URL", "https://test.invalid")
	}
	if os.Getenv("SECRET_ENCRYPTION_KEY") == "" {
		t.Setenv("SECRET_ENCRYPTION_KEY", "test-secret-encryption-key")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load test config: %v", err)
	}
	return cfg
}

func mustTestSecretCodec(t *testing.T) secret.Codec {
	t.Helper()
	codec, err := secret.NewCodec("test-secret-encryption-key")
	if err != nil {
		t.Fatalf("create test secret codec: %v", err)
	}
	return codec
}

func mustTestAIKeys(t *testing.T) aiagent.InternalKeys {
	t.Helper()
	keys, err := aiagent.DeriveInternalKeys(os.Getenv(aiagent.InternalSecretEnvironment))
	if err != nil {
		t.Fatalf("derive test AI keys: %v", err)
	}
	return keys
}
