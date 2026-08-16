package secret

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"os"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const (
	encryptedSecretRefPrefix = "secret:v1:"
	storedSecretIDPrefix     = "secret-id:"
)

var ErrMissingEncryptionKey = errors.New("SECRET_ENCRYPTION_KEY is required in production")

// Generate returns a cryptographically random value for a server-side secret binding.
// The plaintext must be passed directly to StoreContext and must not be returned to an Agent.
func Generate(length int, encoding string) (string, error) {
	if err := ValidateGeneration(length, encoding); err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "hex":
		raw := make([]byte, (length+1)/2)
		if _, err := io.ReadFull(rand.Reader, raw); err != nil {
			return "", err
		}
		return hex.EncodeToString(raw)[:length], nil
	case "base64":
		raw := make([]byte, (length*3+3)/4)
		if _, err := io.ReadFull(rand.Reader, raw); err != nil {
			return "", err
		}
		return base64.RawStdEncoding.EncodeToString(raw)[:length], nil
	case "alphanumeric":
		return randomCharset("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", length)
	case "numeric":
		return randomCharset("0123456789", length)
	default:
		return "", errors.New("secret encoding is invalid")
	}
}

// ValidateGeneration checks the bounded server-side secret generation policy
// without producing or exposing a value.
func ValidateGeneration(length int, encoding string) error {
	if length < 8 || length > 256 {
		return errors.New("secret length must be between 8 and 256")
	}
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "hex", "base64", "alphanumeric", "numeric":
		return nil
	default:
		return errors.New("secret encoding is invalid")
	}
}

func randomCharset(charset string, length int) (string, error) {
	result := make([]byte, length)
	upper := big.NewInt(int64(len(charset)))
	for index := range result {
		value, err := rand.Int(rand.Reader, upper)
		if err != nil {
			return "", err
		}
		result[index] = charset[value.Int64()]
	}
	return string(result), nil
}

func Encrypt(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	key, err := secretRefKey()
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return ""
	}
	payload := append(nonce, gcm.Seal(nil, nonce, []byte(secret), nil)...)
	return encryptedSecretRefPrefix + base64.RawURLEncoding.EncodeToString(payload)
}

func ResolveInline(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, encryptedSecretRefPrefix) {
		payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ref, encryptedSecretRefPrefix))
		if err != nil {
			return ""
		}
		key, err := secretRefKey()
		if err != nil {
			return ""
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return ""
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return ""
		}
		if len(payload) < gcm.NonceSize() {
			return ""
		}
		nonce := payload[:gcm.NonceSize()]
		ciphertext := payload[gcm.NonceSize():]
		secret, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return ""
		}
		return string(secret)
	}
	return ""
}

type ContextAuditFunc func(ctx context.Context, userID, action, resource string, success bool, message string)

type Store struct {
	db           *gorm.DB
	auditContext ContextAuditFunc
}

func NewStore(db *gorm.DB, audit ContextAuditFunc) Store {
	return Store{db: db, auditContext: audit}
}

func (s Store) StoreContext(ctx context.Context, secret, createdBy, resource string) string {
	cipherRef := Encrypt(secret)
	if cipherRef == "" {
		return ""
	}
	value := model.SecretValue{
		ID:        id.New("sec"),
		CipherRef: cipherRef,
		CreatedBy: strings.TrimSpace(createdBy),
		Resource:  strings.TrimSpace(resource),
	}
	if err := s.db.WithContext(ctx).Create(&value).Error; err != nil {
		return ""
	}
	if s.auditContext != nil {
		s.auditContext(ctx, createdBy, "secret.write", value.ID, true, value.Resource)
	}
	return storedSecretIDPrefix + value.ID
}

func (s Store) ResolveContext(ctx context.Context, ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, storedSecretIDPrefix) {
		var value model.SecretValue
		if err := s.db.WithContext(ctx).First(&value, "id = ?", strings.TrimPrefix(ref, storedSecretIDPrefix)).Error; err != nil {
			return ""
		}
		return ResolveInline(value.CipherRef)
	}
	return ResolveInline(ref)
}

func HasValue(ref string) bool {
	return strings.TrimSpace(ref) != ""
}

func SafeClientSecretRef(ref string) string {
	return ""
}

func ValidateEncryptionConfig() error {
	_, err := secretRefKey()
	return err
}

func secretRefKey() ([]byte, error) {
	keyMaterial := strings.TrimSpace(os.Getenv("SECRET_ENCRYPTION_KEY"))
	if keyMaterial == "" {
		if config.RuntimeMode() == "production" {
			return nil, ErrMissingEncryptionKey
		}
		keyMaterial = "luna-devops-local-secret"
	}
	sum := sha256.Sum256([]byte(keyMaterial))
	return sum[:], nil
}
