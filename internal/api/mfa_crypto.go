package api

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

const (
	mfaTOTPIssuer        = "Luna DevOps"
	mfaRecoveryCodeCount = 10
	mfaRecoveryCodeBytes = 10
)

type mfaTOTPEnrollment struct {
	Secret        string
	OTPAuthURL    string
	QRCodeDataURL string
}

func generateTOTPEnrollment(accountName string) (mfaTOTPEnrollment, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      mfaTOTPIssuer,
		AccountName: strings.TrimSpace(accountName),
		Period:      30,
		SecretSize:  20,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return mfaTOTPEnrollment{}, err
	}
	image, err := key.Image(256, 256)
	if err != nil {
		return mfaTOTPEnrollment{}, err
	}
	var imageBuffer bytes.Buffer
	if err := png.Encode(&imageBuffer, image); err != nil {
		return mfaTOTPEnrollment{}, err
	}
	return mfaTOTPEnrollment{
		Secret:        key.Secret(),
		OTPAuthURL:    key.URL(),
		QRCodeDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBuffer.Bytes()),
	}, nil
}

func validateTOTPCode(secret, code string, at time.Time) bool {
	_, valid := matchTOTPCounter(secret, code, at)
	return valid
}

func matchTOTPCounter(secret, code string, at time.Time) (int64, bool) {
	const period int64 = 30
	code = strings.TrimSpace(code)
	secret = strings.TrimSpace(secret)
	if len(code) != 6 || secret == "" {
		return 0, false
	}
	opts := totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	}
	currentCounter := at.Unix() / period
	matchedCounter := int64(-1)
	for offset := int64(-1); offset <= 1; offset++ {
		counter := currentCounter + offset
		if counter < 0 {
			continue
		}
		candidate, err := totp.GenerateCodeCustom(secret, time.Unix(counter*period, 0), opts)
		if err == nil && subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 && counter > matchedCounter {
			matchedCounter = counter
		}
	}
	return matchedCounter, matchedCounter >= 0
}

func generateRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, mfaRecoveryCodeCount)
	hashes := make([]string, 0, mfaRecoveryCodeCount)
	seen := make(map[string]struct{}, mfaRecoveryCodeCount)
	for len(codes) < mfaRecoveryCodeCount {
		raw := make([]byte, mfaRecoveryCodeBytes)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		normalized := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
		if _, exists := seen[normalized]; exists {
			continue
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(normalized), bcrypt.DefaultCost)
		if err != nil {
			return nil, nil, err
		}
		seen[normalized] = struct{}{}
		codes = append(codes, formatRecoveryCode(normalized))
		hashes = append(hashes, string(hash))
	}
	return codes, hashes, nil
}

func normalizeRecoveryCode(code string) string {
	var builder strings.Builder
	for _, char := range strings.ToUpper(strings.TrimSpace(code)) {
		if (char >= 'A' && char <= 'Z') || (char >= '2' && char <= '7') {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func formatRecoveryCode(normalized string) string {
	if len(normalized) != 16 {
		return normalized
	}
	return fmt.Sprintf("%s-%s-%s-%s", normalized[0:4], normalized[4:8], normalized[8:12], normalized[12:16])
}
