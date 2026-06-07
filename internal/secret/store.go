package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
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

type AuditFunc func(userID, action, resource string, success bool, message string)

type Store struct {
	db    *gorm.DB
	audit AuditFunc
}

func NewStore(db *gorm.DB, audit AuditFunc) Store {
	return Store{db: db, audit: audit}
}

func (s Store) Store(secret, createdBy, resource string) string {
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
	if err := s.db.Create(&value).Error; err != nil {
		return ""
	}
	if s.audit != nil {
		s.audit(createdBy, "secret.write", value.ID, true, value.Resource)
	}
	return storedSecretIDPrefix + value.ID
}

func (s Store) Resolve(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, storedSecretIDPrefix) {
		var value model.SecretValue
		if err := s.db.First(&value, "id = ?", strings.TrimPrefix(ref, storedSecretIDPrefix)).Error; err != nil {
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
		keyMaterial = "liteyuki-devops-local-secret"
	}
	sum := sha256.Sum256([]byte(keyMaterial))
	return sum[:], nil
}
