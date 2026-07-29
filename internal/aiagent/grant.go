package aiagent

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidGrant = errors.New("invalid AI grant")

type RunActorGrant struct {
	Audience     string `json:"aud"`
	Purpose      string `json:"purpose"`
	RunID        string `json:"runId"`
	UserID       string `json:"userId"`
	SessionID    string `json:"sessionId"`
	OAuthGrantID string `json:"oauthGrantId,omitempty"`
	ProjectID    string `json:"projectId,omitempty"`
	IssuedAt     int64  `json:"iat"`
	ExpiresAt    int64  `json:"exp"`
}

type DelegationClaims struct {
	Audience      string   `json:"aud"`
	Purpose       string   `json:"purpose"`
	RunID         string   `json:"runId"`
	ToolCallID    string   `json:"toolCallId"`
	OperationID   string   `json:"operationId"`
	UserID        string   `json:"userId"`
	SessionID     string   `json:"sessionId"`
	ProjectID     string   `json:"projectId,omitempty"`
	Scopes        []string `json:"scopes"`
	ArgumentsHash string   `json:"argumentsHash"`
	IssuedAt      int64    `json:"iat"`
	ExpiresAt     int64    `json:"exp"`
}

func SignRunActorGrant(claims RunActorGrant, key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", ErrInvalidGrant
	}
	return signCompact(claims, key)
}

func VerifyRunActorGrant(token, key string, now time.Time) (RunActorGrant, error) {
	var claims RunActorGrant
	if err := verifyCompact(token, key, &claims); err != nil {
		return claims, err
	}
	if claims.Audience != "luna-ai-run-grant" || claims.Purpose != "agent_delegation_exchange" ||
		claims.RunID == "" || claims.UserID == "" || claims.SessionID == "" ||
		claims.IssuedAt > now.Unix()+5 || claims.ExpiresAt <= now.Unix() {
		return claims, ErrInvalidGrant
	}
	return claims, nil
}

func SignDelegationToken(claims DelegationClaims, key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", ErrInvalidGrant
	}
	return signCompact(claims, key)
}

func VerifyDelegationToken(token, key string, now time.Time) (DelegationClaims, error) {
	var claims DelegationClaims
	if err := verifyCompact(token, key, &claims); err != nil {
		return claims, err
	}
	if claims.Audience != "luna-api-ai-tools" || claims.Purpose != "execute_registered_tool" ||
		claims.RunID == "" || claims.ToolCallID == "" || claims.OperationID == "" ||
		claims.UserID == "" || claims.SessionID == "" || claims.IssuedAt > now.Unix()+5 ||
		claims.ExpiresAt <= now.Unix() || claims.ExpiresAt-claims.IssuedAt > 60 {
		return claims, ErrInvalidGrant
	}
	return claims, nil
}

func signCompact(value any, key string) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func verifyCompact(token, key string, target any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || strings.TrimSpace(key) == "" {
		return ErrInvalidGrant
	}
	unsigned := parts[0] + "." + parts[1]
	actual, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return ErrInvalidGrant
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(unsigned))
	if !hmac.Equal(actual, mac.Sum(nil)) {
		return ErrInvalidGrant
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, target) != nil {
		return ErrInvalidGrant
	}
	return nil
}
