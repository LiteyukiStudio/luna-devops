package aitool

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateSecretDefaults(t *testing.T) {
	result, err := NewService(nil).Execute(context.Background(), Request{
		OperationID: "generateSecret",
		Arguments:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	value, ok := result.Value.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", result.Value)
	}
	secrets, ok := value["secrets"].([]string)
	if !ok || len(secrets) != 1 {
		t.Fatalf("secrets = %#v", value["secrets"])
	}
	if len(secrets[0]) != defaultSecretLength {
		t.Fatalf("default length = %d, want %d", len(secrets[0]), defaultSecretLength)
	}
	if encoding := value["encoding"]; encoding != "alphanumeric" {
		t.Fatalf("default encoding = %v, want alphanumeric", encoding)
	}
}

func TestGenerateSecretEncodings(t *testing.T) {
	cases := []struct {
		encoding string
		alphabet string
	}{
		{"base64", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"},
		{"hex", "0123456789abcdef"},
		{"alphanumeric", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"},
		{"numeric", "0123456789"},
	}
	for _, tc := range cases {
		result, err := NewService(nil).Execute(context.Background(), Request{
			OperationID: "generateSecret",
			Arguments:   map[string]any{"length": 24, "encoding": tc.encoding, "count": 3},
		})
		if err != nil {
			t.Fatalf("generate secret (%s): %v", tc.encoding, err)
		}
		value := result.Value.(map[string]any)
		secrets := value["secrets"].([]string)
		if len(secrets) != 3 {
			t.Fatalf("count = %d, want 3", len(secrets))
		}
		for _, secret := range secrets {
			if len(secret) != 24 {
				t.Fatalf("length = %d, want 24", len(secret))
			}
			for _, char := range secret {
				if !strings.ContainsRune(tc.alphabet, char) {
					t.Fatalf("character %q outside %s alphabet", char, tc.encoding)
				}
			}
		}
	}
}

func TestGenerateSecretBatchDistinct(t *testing.T) {
	result, err := NewService(nil).Execute(context.Background(), Request{
		OperationID: "generateSecret",
		Arguments:   map[string]any{"length": 32, "count": 5},
	})
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	secrets := result.Value.(map[string]any)["secrets"].([]string)
	seen := map[string]struct{}{}
	for _, secret := range secrets {
		if _, dup := seen[secret]; dup {
			t.Fatalf("duplicate generated secret: %q", secret)
		}
		seen[secret] = struct{}{}
	}
}

func TestGenerateSecretRejectsInvalidInput(t *testing.T) {
	cases := []map[string]any{
		{"length": 4},        // below minimum
		{"length": 300},      // above maximum
		{"count": 11},        // above maximum
		{"encoding": "yaml"}, // unknown encoding
	}
	for _, arguments := range cases {
		_, err := NewService(nil).Execute(context.Background(), Request{
			OperationID: "generateSecret",
			Arguments:   arguments,
		})
		if err != ErrInvalidInput {
			t.Fatalf("arguments %#v: err = %v, want ErrInvalidInput", arguments, err)
		}
	}
}
