package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func bindDeploymentBundleJSON(ctx *gin.Context, destination *deploymentTargetBundleImportRequest) bool {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, deploymentBundleMaxBytes)
	payload, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			writeErrorCode(ctx, http.StatusRequestEntityTooLarge, "deployment_bundle.too_large", "deployment bundle exceeds the 1 MiB limit")
		} else {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment_bundle.invalid_json", "deployment bundle JSON could not be read")
		}
		return false
	}
	if !utf8.Valid(payload) {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_bundle.invalid_json", "deployment bundle must be UTF-8 JSON")
		return false
	}
	if err := validateDeploymentBundleJSON(payload); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_bundle.invalid_json", err.Error())
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_bundle.invalid_json", "deployment bundle contains an unknown or invalid field")
		return false
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_bundle.invalid_json", "deployment bundle must contain exactly one JSON value")
		return false
	}
	return true
}

func validateDeploymentBundleJSON(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := consumeUniqueJSONValue(decoder, 0); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

func consumeUniqueJSONValue(decoder *json.Decoder, depth int) error {
	if depth > deploymentBundleMaxDepth {
		return fmt.Errorf("deployment bundle exceeds the maximum JSON nesting depth")
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid deployment bundle JSON")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return fmt.Errorf("invalid deployment bundle JSON object")
			}
			key, keyOK := keyToken.(string)
			if !keyOK {
				return fmt.Errorf("invalid deployment bundle JSON object key")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("deployment bundle contains duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := consumeUniqueJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim('}') {
			return fmt.Errorf("invalid deployment bundle JSON object")
		}
	case '[':
		for decoder.More() {
			if err := consumeUniqueJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim(']') {
			return fmt.Errorf("invalid deployment bundle JSON array")
		}
	default:
		return fmt.Errorf("invalid deployment bundle JSON delimiter")
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	}
	return errors.New("additional JSON value")
}

func deploymentBundleDigest(bundle deploymentTargetBundle) (string, error) {
	payload, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
