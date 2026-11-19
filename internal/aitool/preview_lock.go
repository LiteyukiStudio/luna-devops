package aitool

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const previewLockPrefix = "ailock_v1"

var (
	ErrPreviewLockMissing  = errors.New("ai.preview_lock_missing")
	ErrPreviewLockInvalid  = errors.New("ai.preview_lock_invalid")
	ErrPreviewLockExpired  = errors.New("ai.preview_lock_expired")
	ErrPreviewLockMismatch = errors.New("ai.preview_lock_mismatch")
)

type PreviewLockClaims struct {
	RunID                 string          `json:"runId"`
	ToolCallID            string          `json:"toolCallId"`
	OperationID           string          `json:"operationId"`
	ArgumentsHash         string          `json:"argumentsHash"`
	OwnerUserID           string          `json:"ownerUserId"`
	ConversationProjectID string          `json:"conversationProjectId,omitempty"`
	IssuedAt              int64           `json:"issuedAt"`
	ExpiresAt             int64           `json:"expiresAt"`
	LockedPayload         json.RawMessage `json:"lockedPayload,omitempty"`
}

func SignPreviewLock(secret []byte, claims PreviewLockClaims, lockedPayload any) (string, error) {
	if len(secret) == 0 {
		return "", fmt.Errorf("%w: empty signing secret", ErrPreviewLockInvalid)
	}
	if lockedPayload != nil {
		raw, err := json.Marshal(lockedPayload)
		if err != nil {
			return "", fmt.Errorf("marshal preview lock payload: %w", err)
		}
		claims.LockedPayload = raw
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal preview lock claims: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	sig := computePreviewLockHMAC(secret, encodedPayload)
	encodedSig := base64.RawURLEncoding.EncodeToString(sig)
	return previewLockPrefix + "." + encodedPayload + "." + encodedSig, nil
}

func VerifyPreviewLock(secrets [][]byte, token string, expected PreviewLockClaims, now time.Time) (json.RawMessage, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrPreviewLockMissing
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != previewLockPrefix {
		return nil, ErrPreviewLockInvalid
	}
	providedSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrPreviewLockInvalid
	}
	verified := false
	for _, secret := range secrets {
		if len(secret) == 0 {
			continue
		}
		expectedSig := computePreviewLockHMAC(secret, parts[1])
		if hmac.Equal(providedSig, expectedSig) {
			verified = true
			break
		}
	}
	if !verified {
		return nil, ErrPreviewLockInvalid
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrPreviewLockInvalid
	}
	var claims PreviewLockClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrPreviewLockInvalid
	}
	if claims.ExpiresAt <= now.Unix() {
		return nil, ErrPreviewLockExpired
	}
	if claims.RunID != expected.RunID ||
		claims.ToolCallID != expected.ToolCallID ||
		claims.OperationID != expected.OperationID ||
		claims.ArgumentsHash != expected.ArgumentsHash ||
		claims.OwnerUserID != expected.OwnerUserID ||
		claims.ConversationProjectID != expected.ConversationProjectID {
		return nil, ErrPreviewLockMismatch
	}
	return claims.LockedPayload, nil
}

func computePreviewLockHMAC(secret []byte, encodedPayload string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(previewLockPrefix))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}

// CanonicalJSONHash computes `sha256:<hex>` matching luna-agent's hashCanonicalJSON.
func CanonicalJSONHash(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		trimmed = []byte("{}")
	}
	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return "", err
	}
	canonical, err := canonicalJSONValue(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func canonicalJSONValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "null", nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case string:
		return marshalNoEscapeHTML(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", errors.New("ai.invalid_tool_arguments")
		}
		return marshalNoEscapeHTML(v)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			s, err := canonicalJSONValue(item)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, ",") + "]", nil
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			keyJSON, err := marshalNoEscapeHTML(k)
			if err != nil {
				return "", err
			}
			valJSON, err := canonicalJSONValue(v[k])
			if err != nil {
				return "", err
			}
			parts = append(parts, keyJSON+":"+valJSON)
		}
		return "{" + strings.Join(parts, ",") + "}", nil
	default:
		return "", errors.New("ai.invalid_tool_arguments")
	}
}

func marshalNoEscapeHTML(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}
