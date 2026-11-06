package deploymentapi

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

// deploymentBundleV1RemovedConfigurationFields 是 schemaVersion 1 包在 configuration 中曾经导出、现已删除的字段。
// 旧版本导出的包仍可能携带它们；导入前按版本显式丢弃，其余未知字段继续被严格解码拒绝。
var deploymentBundleV1RemovedConfigurationFields = []string{"concurrencyPolicy"}

func bindDeploymentBundleJSON(ctx *gin.Context, destination *deploymentTargetBundleImportRequest) bool {
	return bindDeploymentBundlePayload(ctx, destination, migrateDeploymentBundleRequest)
}

func bindDeploymentBundleCandidateJSON(ctx *gin.Context, destination *deploymentBundleReferenceCandidatesRequest) bool {
	return bindDeploymentBundlePayload(ctx, destination, nil)
}

// migrateDeploymentBundleRequest 把旧版本包迁移为当前结构：只针对已知版本删除已知字段，不放开其他未知字段。
// 结构不符合预期时原样返回，由严格解码给出 invalid_json。
func migrateDeploymentBundleRequest(payload []byte) ([]byte, error) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(payload, &request); err != nil || request == nil {
		return payload, nil
	}
	var bundle map[string]json.RawMessage
	if err := json.Unmarshal(request["bundle"], &bundle); err != nil || bundle == nil {
		return payload, nil
	}
	var schemaVersion int
	if err := json.Unmarshal(bundle["schemaVersion"], &schemaVersion); err != nil || schemaVersion != 1 {
		return payload, nil
	}
	var configuration map[string]json.RawMessage
	if err := json.Unmarshal(bundle["configuration"], &configuration); err != nil || configuration == nil {
		return payload, nil
	}
	changed := false
	for _, field := range deploymentBundleV1RemovedConfigurationFields {
		if _, exists := configuration[field]; exists {
			delete(configuration, field)
			changed = true
		}
	}
	if !changed {
		return payload, nil
	}
	var err error
	if bundle["configuration"], err = json.Marshal(configuration); err != nil {
		return nil, err
	}
	if request["bundle"], err = json.Marshal(bundle); err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

func bindDeploymentBundlePayload(ctx *gin.Context, destination any, migrate func([]byte) ([]byte, error)) bool {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, deploymentBundleMaxBytes)
	payload, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			writeDeploymentBundleCode(ctx, "deployment_bundle.too_large")
		} else {
			writeDeploymentBundleCode(ctx, "deployment_bundle.invalid_json")
		}
		return false
	}
	if !utf8.Valid(payload) {
		writeDeploymentBundleCode(ctx, "deployment_bundle.invalid_json")
		return false
	}
	if err := validateDeploymentBundleJSON(payload); err != nil {
		writeDeploymentBundleCode(ctx, "deployment_bundle.invalid_json")
		return false
	}
	if migrate != nil {
		if payload, err = migrate(payload); err != nil {
			writeDeploymentBundleCode(ctx, "deployment_bundle.invalid_json")
			return false
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeDeploymentBundleCode(ctx, "deployment_bundle.invalid_json")
		return false
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeDeploymentBundleCode(ctx, "deployment_bundle.invalid_json")
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
