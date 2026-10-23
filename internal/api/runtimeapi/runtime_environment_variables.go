package runtimeapi

import (
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"github.com/gin-gonic/gin"
)

const (
	runtimeEnvironmentValueModePublic = "public"
	runtimeEnvironmentValueModeSecret = "secret"
	maxRuntimeEnvironmentVariables    = 128
	maxRuntimeEnvironmentValueLength  = 8192
)

type runtimeEnvironmentVariableInput struct {
	Key       string `json:"key"`
	ValueMode string `json:"valueMode"`
	Value     string `json:"value"`
	// SecretOperation 仅用于 secret 项：set、generate 或 clear。未出现在列表中的已有密钥保持不变。
	SecretOperation string                   `json:"secretOperation,omitempty"`
	Generation      *runtimeSecretGeneration `json:"generation,omitempty"`
}

type runtimeEnvironmentVariableResponse struct {
	Key        string `json:"key"`
	ValueMode  string `json:"valueMode"`
	Value      string `json:"value,omitempty"`
	Configured bool   `json:"configured"`
}

func normalizePublicEnvironmentVariables(ctx *gin.Context, items []runtimeEnvironmentVariableInput) (map[string]string, bool) {
	if len(items) > maxRuntimeEnvironmentVariables {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_items_invalid", "运行时环境变量数量超过限制")
		return nil, false
	}
	values := make(map[string]string, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.Key)
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_key_invalid", "运行时环境变量名称无效")
			return nil, false
		}
		if _, duplicate := values[key]; duplicate {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_key_duplicate", "运行时环境变量名称不能重复")
			return nil, false
		}
		if strings.TrimSpace(item.ValueMode) != runtimeEnvironmentValueModePublic || strings.TrimSpace(item.SecretOperation) != "" || item.Generation != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_value_mode_invalid", "普通运行时环境变量必须使用 public 类型且不能携带密钥操作")
			return nil, false
		}
		if utf8.RuneCountInString(item.Value) > maxRuntimeEnvironmentValueLength {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_value_too_long", "运行时环境变量值过长")
			return nil, false
		}
		values[key] = item.Value
	}
	return values, true
}

// normalizeEnvironmentVariables 拆分统一键值表：public 项是完整期望集合；secret 项只携带显式操作。
// 同一请求里普通项只能与“清除”同名，用于加密转普通。
func normalizeEnvironmentVariables(ctx *gin.Context, items []runtimeEnvironmentVariableInput) (map[string]string, runtimeSecretMutationInput, bool) {
	empty := runtimeSecretMutationInput{Values: map[string]string{}, Generate: map[string]runtimeSecretGeneration{}}
	if len(items) > maxRuntimeEnvironmentVariables {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_items_invalid", "运行时环境变量数量超过限制")
		return nil, empty, false
	}
	publicItems := make([]runtimeEnvironmentVariableInput, 0, len(items))
	secretItems := make([]runtimeSecretMutationRequestItem, 0)
	for _, item := range items {
		switch strings.TrimSpace(item.ValueMode) {
		case runtimeEnvironmentValueModePublic:
			publicItems = append(publicItems, item)
		case runtimeEnvironmentValueModeSecret:
			secretItems = append(secretItems, runtimeSecretMutationRequestItem{
				Key:        item.Key,
				ValueMode:  item.ValueMode,
				Operation:  item.SecretOperation,
				Value:      item.Value,
				Generation: item.Generation,
			})
		default:
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_value_mode_invalid", "运行时环境变量类型无效")
			return nil, empty, false
		}
	}
	publicValues, ok := normalizePublicEnvironmentVariables(ctx, publicItems)
	if !ok {
		return nil, empty, false
	}
	if len(secretItems) == 0 {
		return publicValues, empty, true
	}
	secretInput, ok := runtimeSecretMutationInputFromRequest(ctx, runtimeSecretMutationRequest{Items: secretItems})
	if !ok || !validateRuntimeSecretMutation(ctx, &secretInput) {
		return nil, empty, false
	}
	for _, value := range secretInput.Values {
		if value == "" {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_value_required", "设置加密配置时必须填写值")
			return nil, empty, false
		}
	}
	for key := range publicValues {
		_, set := secretInput.Values[key]
		_, generated := secretInput.Generate[key]
		if set || generated {
			writeRuntimeEnvironmentKeyConflict(ctx)
			return nil, empty, false
		}
	}
	return publicValues, secretInput, true
}

func writeRuntimeEnvironmentKeyConflict(ctx *gin.Context) {
	writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_key_conflict", "普通配置与加密配置的名称不能重复")
}

func runtimeEnvironmentVariables(publicRaw, secretRaw string) []runtimeEnvironmentVariableResponse {
	publicValues, err := runtimeconfig.DecodeKeyValue(publicRaw)
	if err != nil {
		publicValues = map[string]string{}
	}
	secretKeys := runtimeSecretKeys(secretRaw)
	items := make([]runtimeEnvironmentVariableResponse, 0, len(publicValues)+len(secretKeys))
	for key, value := range publicValues {
		items = append(items, runtimeEnvironmentVariableResponse{
			Key:        key,
			ValueMode:  runtimeEnvironmentValueModePublic,
			Value:      value,
			Configured: true,
		})
	}
	for _, key := range secretKeys {
		items = append(items, runtimeEnvironmentVariableResponse{
			Key:        key,
			ValueMode:  runtimeEnvironmentValueModeSecret,
			Configured: true,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Key == items[j].Key {
			return items[i].ValueMode < items[j].ValueMode
		}
		return items[i].Key < items[j].Key
	})
	return items
}

func publicEnvironmentVariableInputs(raw string) []runtimeEnvironmentVariableInput {
	values := runtimeConfigMap(raw)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]runtimeEnvironmentVariableInput, 0, len(keys))
	for _, key := range keys {
		items = append(items, runtimeEnvironmentVariableInput{Key: key, ValueMode: runtimeEnvironmentValueModePublic, Value: values[key]})
	}
	return items
}
