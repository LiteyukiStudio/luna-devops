package aiagent

import "testing"

func TestDeriveInternalKeysStableVector(t *testing.T) {
	keys, err := DeriveInternalKeys("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if keys.ServiceToken != "rc9uB_qX7ORPNH5SB-_AhAPh3hgMj0qMkjfVkHqRDco" {
		t.Fatalf("unexpected service token: %s", keys.ServiceToken)
	}
	if keys.CallbackServiceToken == "" || keys.ActorSigningKey == "" {
		t.Fatalf("derived service identity keys are incomplete: %#v", keys)
	}
}

func TestDeriveInternalKeysRejectsShortSecret(t *testing.T) {
	if _, err := DeriveInternalKeys("too-short"); err == nil {
		t.Fatal("expected short secret to be rejected")
	}
}
