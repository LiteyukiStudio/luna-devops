package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
)

// DeliveryIdempotencyKey scopes a caller-supplied delivery command key without
// exposing the server-side scope to clients.
func DeliveryIdempotencyKey(ctx *gin.Context, scope string) (string, bool) {
	raw := strings.TrimSpace(ctx.GetHeader("Idempotency-Key"))
	if raw == "" {
		return "", true
	}
	if len(raw) < 8 || len(raw) > 128 {
		return "", false
	}
	return scope + ":" + raw, true
}

func DeliveryRequestHash(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}
