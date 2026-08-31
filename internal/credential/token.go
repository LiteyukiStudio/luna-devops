package credential

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// Generate creates an opaque credential and its irreversible database hash.
// The plaintext must only be retained long enough to build the one-time
// response returned to the caller.
func Generate(prefix string, randomBytes int) (plaintext string, hash string) {
	plaintext = prefix + RandomHex(randomBytes)
	return plaintext, Hash(plaintext)
}

// RandomHex returns randomBytes bytes encoded as lowercase hexadecimal.
func RandomHex(randomBytes int) string {
	if randomBytes <= 0 {
		panic("credential random byte count must be positive")
	}
	value := make([]byte, randomBytes)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value)
}

// Hash produces the stable representation stored in credential tables.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// Matches performs a constant-time comparison against a stored hexadecimal
// hash. It is useful when an indexed hash lookup is not available.
func Matches(storedHash, plaintext string) bool {
	expected, err := hex.DecodeString(storedHash)
	if err != nil || len(expected) != sha256.Size {
		return false
	}
	actual := sha256.Sum256([]byte(plaintext))
	return subtle.ConstantTimeCompare(expected, actual[:]) == 1
}
