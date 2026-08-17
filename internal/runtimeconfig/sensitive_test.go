package runtimeconfig

import (
	"reflect"
	"testing"
)

func TestPotentialSecretCoversCommonPlaintextRisks(t *testing.T) {
	for _, item := range []struct{ key, value string }{
		{key: "REDIS_PASS", value: "value"},
		{key: "APIKEY", value: "value"},
		{key: "AUTH", value: "value"},
		{key: "DATABASE_DSN", value: "postgres://db/app"},
		{key: "PUBLIC_URL", value: "https://user:password@example.com/app"},
		{key: "PUBLIC_URL", value: "https://example.com/app?token=value"},
	} {
		if !PotentialSecret(item.key, item.value) {
			t.Fatalf("PotentialSecret(%q, value) = false", item.key)
		}
	}
}

func TestSuspectedSecretKeysNeverReturnsValues(t *testing.T) {
	keys, err := SuspectedSecretKeys(`{"LOG_LEVEL":"debug","TOKEN":"must-not-leak","PUBLIC_URL":"https://example.com?password=must-not-leak"}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"PUBLIC_URL", "TOKEN"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %#v, want %#v", keys, want)
	}
	for _, key := range keys {
		if key == "must-not-leak" {
			t.Fatal("diagnostic returned a secret value")
		}
	}
}
