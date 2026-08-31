package credential

import (
	"strings"
	"testing"
)

func TestGenerateAndMatches(t *testing.T) {
	plaintext, hash := Generate("lyk_", 24)
	if !strings.HasPrefix(plaintext, "lyk_") || len(plaintext) != len("lyk_")+48 {
		t.Fatalf("plaintext has unexpected shape: %q", plaintext)
	}
	if !Matches(hash, plaintext) {
		t.Fatal("generated plaintext does not match its hash")
	}
	if Matches(hash, plaintext+"x") || Matches("invalid", plaintext) {
		t.Fatal("mismatched or malformed credentials must be rejected")
	}
}
