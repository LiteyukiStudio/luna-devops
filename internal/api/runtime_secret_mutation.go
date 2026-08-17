package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
)

type runtimeSecretGeneration struct {
	Length   int    `json:"length"`
	Encoding string `json:"encoding"`
}

type runtimeSecretMutationInput struct {
	Values   map[string]string                  `json:"values"`
	Generate map[string]runtimeSecretGeneration `json:"generate"`
	Clear    []string                           `json:"clear"`
}

type runtimeSecretMutationResponse struct {
	ConfiguredKeys []string `json:"configuredKeys"`
	GeneratedKeys  []string `json:"generatedKeys"`
	ClearedKeys    []string `json:"clearedKeys"`
	SecretKeys     []string `json:"secretKeys"`
	SecretRefsSet  bool     `json:"secretRefsSet"`
}

func (h *Handlers) applyRuntimeSecretMutation(ctx *gin.Context, user model.User, input runtimeSecretMutationInput, refs map[string]string, resource string) (map[string]string, runtimeSecretMutationResponse, bool) {
	if !validateRuntimeSecretMutation(ctx, &input) {
		return nil, runtimeSecretMutationResponse{}, false
	}
	configuredKeys := make([]string, 0, len(input.Values))
	generatedKeys := make([]string, 0, len(input.Generate))
	clearedKeys := make([]string, 0, len(input.Clear))
	seenClear := map[string]struct{}{}

	for key, value := range input.Values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		ref := h.secrets.StoreContext(ctx.Request.Context(), value, user.ID, resource+":"+key)
		if strings.TrimSpace(ref) == "" {
			writeErrorCode(ctx, http.StatusInternalServerError, "deployment.secret_store_unavailable", "密钥保存失败")
			return nil, runtimeSecretMutationResponse{}, false
		}
		refs[key] = ref
		configuredKeys = append(configuredKeys, key)
	}

	for key, generation := range input.Generate {
		if generation.Length == 0 {
			generation.Length = 32
		}
		if strings.TrimSpace(generation.Encoding) == "" {
			generation.Encoding = "base64"
		}
		value, err := secret.Generate(generation.Length, generation.Encoding)
		if err != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_generation_invalid", "密钥生成参数无效")
			return nil, runtimeSecretMutationResponse{}, false
		}
		ref := h.secrets.StoreContext(ctx.Request.Context(), value, user.ID, resource+":"+key)
		if strings.TrimSpace(ref) == "" {
			writeErrorCode(ctx, http.StatusInternalServerError, "deployment.secret_store_unavailable", "密钥保存失败")
			return nil, runtimeSecretMutationResponse{}, false
		}
		refs[key] = ref
		generatedKeys = append(generatedKeys, key)
	}

	for _, rawKey := range input.Clear {
		key := strings.TrimSpace(rawKey)
		if _, duplicate := seenClear[key]; duplicate {
			continue
		}
		seenClear[key] = struct{}{}
		if _, exists := refs[key]; exists {
			delete(refs, key)
			clearedKeys = append(clearedKeys, key)
		}
	}

	sort.Strings(configuredKeys)
	sort.Strings(generatedKeys)
	sort.Strings(clearedKeys)
	keys := make([]string, 0, len(refs))
	for key, ref := range refs {
		if isBuildEnvKey(key) && strings.TrimSpace(ref) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return refs, runtimeSecretMutationResponse{
		ConfiguredKeys: configuredKeys,
		GeneratedKeys:  generatedKeys,
		ClearedKeys:    clearedKeys,
		SecretKeys:     keys,
		SecretRefsSet:  len(keys) > 0,
	}, true
}

func validateRuntimeSecretMutation(ctx *gin.Context, input *runtimeSecretMutationInput) bool {
	for key := range input.Values {
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_key_invalid", "密钥字段名无效")
			return false
		}
		if _, exists := input.Generate[key]; exists {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "同一密钥不能同时填写和生成")
			return false
		}
	}
	for key, generation := range input.Generate {
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_key_invalid", "密钥字段名无效")
			return false
		}
		if generation.Length == 0 {
			generation.Length = 32
			input.Generate[key] = generation
		}
		if strings.TrimSpace(generation.Encoding) == "" {
			generation.Encoding = "base64"
			input.Generate[key] = generation
		}
		if err := secret.ValidateGeneration(generation.Length, generation.Encoding); err != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_generation_invalid", "密钥生成参数无效")
			return false
		}
	}
	for _, rawKey := range input.Clear {
		key := strings.TrimSpace(rawKey)
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_key_invalid", "密钥字段名无效")
			return false
		}
		if _, exists := input.Values[key]; exists {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "同一密钥不能同时设置和清除")
			return false
		}
		if _, exists := input.Generate[key]; exists {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "同一密钥不能同时生成和清除")
			return false
		}
	}
	return true
}
