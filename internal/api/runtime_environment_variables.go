package api

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
		if strings.TrimSpace(item.ValueMode) != runtimeEnvironmentValueModePublic {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_value_mode_invalid", "普通运行时环境变量必须使用 public 类型")
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

func runtimeEnvironmentVariables(publicRaw, secretRaw string) []runtimeEnvironmentVariableResponse {
	publicValues, err := runtimeconfig.ParseLegacyKeyValue(publicRaw)
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
