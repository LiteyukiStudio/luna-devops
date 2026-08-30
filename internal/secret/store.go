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
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const (
	encryptedSecretRefPrefix = "secret:v1:"
	storedSecretIDPrefix     = "secret-id:"
)

var ErrMissingEncryptionKey = errors.New("SECRET_ENCRYPTION_KEY is required in production")

var ErrStoreUnavailable = errors.New("secret store is unavailable")

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

type Codec struct {
	key []byte
}

func NewCodec(keyMaterial string) (Codec, error) {
	keyMaterial = strings.TrimSpace(keyMaterial)
	if keyMaterial == "" {
		return Codec{}, ErrMissingEncryptionKey
	}
	sum := sha256.Sum256([]byte(keyMaterial))
	return Codec{key: append([]byte(nil), sum[:]...)}, nil
}

func (c Codec) Available() bool {
	return len(c.key) == 32
}

func (c Codec) Encrypt(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if !c.Available() {
		return ""
	}
	block, err := aes.NewCipher(c.key)
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

func (c Codec) ResolveInline(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, encryptedSecretRefPrefix) {
		payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ref, encryptedSecretRefPrefix))
		if err != nil {
			return ""
		}
		if !c.Available() {
			return ""
		}
		block, err := aes.NewCipher(c.key)
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
	codec        Codec
}

func NewStore(db *gorm.DB, audit ContextAuditFunc, codec Codec) Store {
	return Store{db: db, auditContext: audit, codec: codec}
}

func (s Store) WithDB(db *gorm.DB) Store {
	s.db = db
	return s
}

func (s Store) Available() bool {
	return s.codec.Available()
}

func (s Store) Encrypt(plaintext string) string {
	return s.codec.Encrypt(plaintext)
}

func (s Store) StoreContext(ctx context.Context, secret, createdBy, resource string) string {
	ref, err := s.StoreContextWithDB(ctx, s.db, secret, createdBy, resource)
	if err != nil {
		return ""
	}
	if s.auditContext != nil {
		s.auditContext(ctx, createdBy, "secret.write", strings.TrimPrefix(ref, storedSecretIDPrefix), true, strings.TrimSpace(resource))
	}
	return ref
}

// StoreContextWithDB persists a secret through the supplied GORM handle and
// returns an error to the transaction owner. This method deliberately does not
// emit the StoreContext secret.write callback: transaction owners must write a
// single aggregate audit record through the same transaction, so neither an
// audit nor a secret row can outlive a rolled-back owner update.
func (s Store) StoreContextWithDB(ctx context.Context, db *gorm.DB, plaintext, createdBy, resource string) (string, error) {
	if db == nil {
		return "", ErrStoreUnavailable
	}
	cipherRef := s.codec.Encrypt(plaintext)
	if cipherRef == "" {
		return "", ErrStoreUnavailable
	}
	value := model.SecretValue{
		ID:        id.New("sec"),
		CipherRef: cipherRef,
		CreatedBy: strings.TrimSpace(createdBy),
		Resource:  strings.TrimSpace(resource),
	}
	if err := db.WithContext(ctx).Create(&value).Error; err != nil {
		return "", err
	}
	return storedSecretIDPrefix + value.ID, nil
}

// DeleteRefContextWithDB removes a stored secret only when both its reference
// and owning resource match.
func (s Store) DeleteRefContextWithDB(ctx context.Context, db *gorm.DB, ref, resource string) error {
	if db == nil {
		return ErrStoreUnavailable
	}
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, storedSecretIDPrefix) {
		return nil
	}
	return db.WithContext(ctx).
		Where("id = ? and resource = ?", strings.TrimPrefix(ref, storedSecretIDPrefix), strings.TrimSpace(resource)).
		Delete(&model.SecretValue{}).Error
}

func (s Store) ResolveContext(ctx context.Context, ref string) string {
	ref = strings.TrimSpace(ref)
	if s.db == nil || !strings.HasPrefix(ref, storedSecretIDPrefix) {
		return ""
	}
	var value model.SecretValue
	if err := s.db.WithContext(ctx).First(&value, "id = ?", strings.TrimPrefix(ref, storedSecretIDPrefix)).Error; err != nil {
		return ""
	}
	return s.codec.ResolveInline(value.CipherRef)
}

func HasValue(ref string) bool {
	return strings.HasPrefix(strings.TrimSpace(ref), storedSecretIDPrefix)
}

func SafeClientSecretRef(ref string) string {
	return ""
}
