package aiagent

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

const (
	InternalSecretEnvironment = "AI_INTERNAL_SECRET"
	internalSecretSalt        = "luna-devops/ai-internal/v1"
)

type InternalKeys struct {
	ServiceToken                        string
	ActorSigningKey                     string
	CallbackServiceToken                string
	RunActorGrantSigningKey             string
	DelegationTokenSigningKey           string
	ConversationAuthorizationSigningKey string
	RunGrantEncryptionKeyBytes          []byte
}

func LoadInternalKeys() (InternalKeys, error) {
	return DeriveInternalKeys(os.Getenv(InternalSecretEnvironment))
}

func DeriveInternalKeys(secret string) (InternalKeys, error) {
	if len([]byte(strings.TrimSpace(secret))) < 32 {
		return InternalKeys{}, errors.New("AI_INTERNAL_SECRET must contain at least 32 bytes")
	}
	deriveText := func(purpose string) (string, error) {
		key, err := deriveInternalKey(secret, purpose)
		if err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(key), nil
	}
	serviceToken, err := deriveText("api-service-token")
	if err != nil {
		return InternalKeys{}, err
	}
	actorSigningKey, err := deriveText("actor-context-signing-key")
	if err != nil {
		return InternalKeys{}, err
	}
	callbackServiceToken, err := deriveText("agent-callback-service-token")
	if err != nil {
		return InternalKeys{}, err
	}
	runActorGrantSigningKey, err := deriveText("run-actor-grant-signing-key")
	if err != nil {
		return InternalKeys{}, err
	}
	delegationTokenSigningKey, err := deriveText("delegation-token-signing-key")
	if err != nil {
		return InternalKeys{}, err
	}
	conversationAuthorizationSigningKey, err := deriveText("conversation-authorization-signing-key")
	if err != nil {
		return InternalKeys{}, err
	}
	runGrantEncryptionKey, err := deriveInternalKey(secret, "run-grant-encryption-key")
	if err != nil {
		return InternalKeys{}, err
	}
	return InternalKeys{
		ServiceToken:                        serviceToken,
		ActorSigningKey:                     actorSigningKey,
		CallbackServiceToken:                callbackServiceToken,
		RunActorGrantSigningKey:             runActorGrantSigningKey,
		DelegationTokenSigningKey:           delegationTokenSigningKey,
		ConversationAuthorizationSigningKey: conversationAuthorizationSigningKey,
		RunGrantEncryptionKeyBytes:          runGrantEncryptionKey,
	}, nil
}

func deriveInternalKey(secret, purpose string) ([]byte, error) {
	return hkdf.Key(sha256.New, []byte(strings.TrimSpace(secret)), []byte(internalSecretSalt), "luna-devops/ai/"+purpose+"/v1", 32)
}
