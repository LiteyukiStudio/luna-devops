package aitool

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"strings"
)

// 允许的字符集与默认参数。生成值属于用户敏感数据：只通过工具结果回传给模型和
// 当前用户，不得写入日志、Span 属性或任何可观测字段（遥测侧由 Agent 掩码）。
const (
	defaultSecretLength = 32
	minSecretLength     = 8
	maxSecretLength     = 256
	defaultSecretCount  = 1
	maxSecretCount      = 10
)

const (
	alphanumericCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	numericCharset      = "0123456789"
)

func (s *Service) generateSecret(arguments map[string]any) (Result, error) {
	length := intArgument(arguments, "length")
	if length == 0 {
		length = defaultSecretLength
	}
	if length < minSecretLength || length > maxSecretLength {
		return Result{}, ErrInvalidInput
	}
	count := intArgument(arguments, "count")
	if count == 0 {
		count = defaultSecretCount
	}
	if count < 1 || count > maxSecretCount {
		return Result{}, ErrInvalidInput
	}
	encoding := strings.ToLower(strings.TrimSpace(stringArgument(arguments, "encoding")))
	if encoding == "" {
		encoding = "alphanumeric"
	}
	secrets := make([]string, 0, count)
	for range count {
		value, err := randomSecret(length, encoding)
		if err != nil {
			return Result{}, err
		}
		secrets = append(secrets, value)
	}
	return Result{Value: map[string]any{
		"secrets":  secrets,
		"encoding": encoding,
		"length":   length,
	}}, nil
}

func randomSecret(length int, encoding string) (string, error) {
	switch encoding {
	case "base64":
		// base64 编码 4 字符表示 3 字节，向上取整到 4 的倍数，避免末尾 `=` 填充。
		rawLength := (length + 3) / 4 * 3
		raw := make([]byte, rawLength)
		if _, err := rand.Read(raw); err != nil {
			return "", ErrStorage
		}
		encoded := base64.StdEncoding.EncodeToString(raw)
		return encoded[:length], nil
	case "hex":
		// hex 每字符 4 位，向上取整到偶数长度，避免半字节。
		rawLength := (length + 1) / 2
		raw := make([]byte, rawLength)
		if _, err := rand.Read(raw); err != nil {
			return "", ErrStorage
		}
		encoded := hex.EncodeToString(raw)
		return encoded[:length], nil
	case "numeric":
		return randomFromCharset(numericCharset, length)
	case "alphanumeric", "":
		return randomFromCharset(alphanumericCharset, length)
	default:
		return "", ErrInvalidInput
	}
}

func randomFromCharset(charset string, length int) (string, error) {
	charsetLength := big.NewInt(int64(len(charset)))
	output := make([]byte, length)
	for i := range output {
		index, err := rand.Int(rand.Reader, charsetLength)
		if err != nil {
			return "", ErrStorage
		}
		output[i] = charset[index.Int64()]
	}
	return string(output), nil
}
