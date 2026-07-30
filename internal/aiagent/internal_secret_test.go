package aiagent

import (
	"encoding/hex"
	"testing"
)

func TestDeriveInternalKeysStableVector(t *testing.T) {
	keys, err := DeriveInternalKeys("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if keys.ServiceToken != "rc9uB_qX7ORPNH5SB-_AhAPh3hgMj0qMkjfVkHqRDco" {
		t.Fatalf("unexpected service token: %s", keys.ServiceToken)
	}
	if got := hex.EncodeToString(keys.RunGrantEncryptionKeyBytes); got != "68fb9e789fd931374447396477b8964d3ba519f0ca94564026d90262e2f1e7d0" {
		t.Fatalf("unexpected run grant encryption key: %s", got)
	}
}

func TestDeriveInternalKeysRejectsShortSecret(t *testing.T) {
	if _, err := DeriveInternalKeys("too-short"); err == nil {
		t.Fatal("expected short secret to be rejected")
	}
}
