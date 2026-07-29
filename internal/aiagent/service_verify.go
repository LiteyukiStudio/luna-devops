package aiagent

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"time"
)

func VerifyAgentServiceJWT(token, publicKeyPEM string, now time.Time) error {
	block, _ := pem.Decode([]byte(strings.TrimSpace(publicKeyPEM)))
	if block == nil {
		return ErrInvalidGrant
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return ErrInvalidGrant
	}
	publicKey, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return ErrInvalidGrant
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ErrInvalidGrant
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(publicKey, []byte(parts[0]+"."+parts[1]), signature) {
		return ErrInvalidGrant
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ErrInvalidGrant
	}
	var claims struct {
		Audience  any   `json:"aud"`
		IssuedAt  int64 `json:"iat"`
		ExpiresAt int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || !audienceContains(claims.Audience, "luna-api-internal") ||
		claims.IssuedAt > now.Unix()+5 || claims.ExpiresAt <= now.Unix() || claims.ExpiresAt-claims.IssuedAt > 60 {
		return ErrInvalidGrant
	}
	return nil
}

func audienceContains(raw any, expected string) bool {
	switch value := raw.(type) {
	case string:
		return value == expected
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok && text == expected {
				return true
			}
		}
	}
	return false
}
