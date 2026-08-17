package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type runtimeSecretGeneration struct {
	Length   int    `json:"length"`
	Encoding string `json:"encoding"`
}

type runtimeSecretMutationRequestItem struct {
	Key        string                   `json:"key"`
	ValueMode  string                   `json:"valueMode"`
	Operation  string                   `json:"operation"`
	Value      string                   `json:"value"`
	Generation *runtimeSecretGeneration `json:"generation"`
}

type runtimeSecretMutationRequest struct {
	Items []runtimeSecretMutationRequestItem `json:"items"`
}

type runtimeSecretMutationInput struct {
	Values   map[string]string                  `json:"values"`
	Generate map[string]runtimeSecretGeneration `json:"generate"`
	Clear    []string                           `json:"clear"`
}

type runtimeSecretMutationResponse struct {
	ConfiguredKeys       []string                             `json:"configuredKeys"`
	GeneratedKeys        []string                             `json:"generatedKeys"`
	ClearedKeys          []string                             `json:"clearedKeys"`
	EnvironmentVariables []runtimeEnvironmentVariableResponse `json:"environmentVariables"`
}

type runtimeSecretMutationOwner struct {
	ResourceID     string
	ResourcePrefix string
	AuditAction    string
	LoadRefs       func(tx *gorm.DB) (string, error)
	LoadPublic     func(tx *gorm.DB) (string, error)
	SaveRefs       func(tx *gorm.DB, encoded string) error
	EncodeRefs     func(refs map[string]string) string
}

type preparedRuntimeSecretMutation struct {
	values        map[string]string
	configuredKey []string
	generatedKey  []string
	clearKey      []string
}

var (
	errRuntimeSecretMutationUnavailable = errors.New("runtime secret mutation unavailable")
	errRuntimeEnvironmentModeConflict   = errors.New("runtime environment value mode conflict")
)

func prepareRuntimeSecretMutation(input runtimeSecretMutationInput) (preparedRuntimeSecretMutation, error) {
	prepared := preparedRuntimeSecretMutation{values: map[string]string{}}
	for key, value := range input.Values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		prepared.values[key] = value
		prepared.configuredKey = append(prepared.configuredKey, key)
	}
	for key, generation := range input.Generate {
		value, err := secret.Generate(generation.Length, generation.Encoding)
		if err != nil {
			return preparedRuntimeSecretMutation{}, err
		}
		prepared.values[key] = value
		prepared.generatedKey = append(prepared.generatedKey, key)
	}
	seenClear := map[string]struct{}{}
	for _, rawKey := range input.Clear {
		key := strings.TrimSpace(rawKey)
		if _, duplicate := seenClear[key]; duplicate {
			continue
		}
		seenClear[key] = struct{}{}
		prepared.clearKey = append(prepared.clearKey, key)
	}
	sort.Strings(prepared.configuredKey)
	sort.Strings(prepared.generatedKey)
	sort.Strings(prepared.clearKey)
	return prepared, nil
}

func (h *Handlers) mutateRuntimeSecrets(ctx context.Context, user model.User, prepared preparedRuntimeSecretMutation, owner runtimeSecretMutationOwner) (runtimeSecretMutationResponse, error) {
	var response runtimeSecretMutationResponse
	err := h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rawRefs, err := owner.LoadRefs(tx.WithContext(ctx))
		if err != nil {
			return err
		}
		previousRefs, err := decodeRuntimeSecretRefs(rawRefs)
		if err != nil {
			return errRuntimeSecretMutationUnavailable
		}
		nextRefs := copyStringMap(previousRefs)
		if owner.LoadPublic != nil {
			publicRaw, err := owner.LoadPublic(tx.WithContext(ctx))
			if err != nil {
				return err
			}
			publicValues := runtimeConfigMap(publicRaw)
			for key := range prepared.values {
				if _, conflict := publicValues[key]; conflict {
					return errRuntimeEnvironmentModeConflict
				}
			}
		}

		for key, value := range prepared.values {
			ref, err := h.secrets.StoreContextWithDB(ctx, tx, value, user.ID, owner.ResourcePrefix+":"+key)
			if err != nil {
				return errRuntimeSecretMutationUnavailable
			}
			nextRefs[key] = ref
		}

		clearedKeys := make([]string, 0, len(prepared.clearKey))
		for _, key := range prepared.clearKey {
			if _, exists := nextRefs[key]; !exists {
				continue
			}
			delete(nextRefs, key)
			clearedKeys = append(clearedKeys, key)
		}

		if err := owner.SaveRefs(tx.WithContext(ctx), owner.EncodeRefs(nextRefs)); err != nil {
			return err
		}
		if err := h.deleteSupersededRuntimeSecrets(ctx, tx, owner.ResourcePrefix, previousRefs, nextRefs); err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Create(&model.AuditLog{
			ID:        id.New("aud"),
			UserID:    strings.TrimSpace(user.ID),
			Action:    owner.AuditAction,
			Resource:  owner.ResourceID,
			Success:   true,
			Message:   "runtime secret state updated",
			CreatedAt: time.Now(),
		}).Error; err != nil {
			return err
		}

		keys := make([]string, 0, len(nextRefs))
		for key, ref := range nextRefs {
			if isBuildEnvKey(key) && strings.TrimSpace(ref) != "" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		response = runtimeSecretMutationResponse{
			ConfiguredKeys:       prepared.configuredKey,
			GeneratedKeys:        prepared.generatedKey,
			ClearedKeys:          clearedKeys,
			EnvironmentVariables: secretEnvironmentVariables(keys),
		}
		return nil
	})
	if err != nil {
		return runtimeSecretMutationResponse{}, err
	}
	return response, nil
}

func runtimeSecretMutationInputFromRequest(ctx *gin.Context, request runtimeSecretMutationRequest) (runtimeSecretMutationInput, bool) {
	if len(request.Items) == 0 || len(request.Items) > maxRuntimeEnvironmentVariables {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_items_invalid", "运行时密钥操作数量无效")
		return runtimeSecretMutationInput{}, false
	}
	input := runtimeSecretMutationInput{
		Values:   map[string]string{},
		Generate: map[string]runtimeSecretGeneration{},
	}
	seen := make(map[string]struct{}, len(request.Items))
	for _, item := range request.Items {
		key := strings.TrimSpace(item.Key)
		if strings.TrimSpace(item.ValueMode) != runtimeEnvironmentValueModeSecret {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_environment_value_mode_invalid", "运行时密钥必须使用 secret 类型")
			return runtimeSecretMutationInput{}, false
		}
		if _, duplicate := seen[key]; duplicate {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "同一密钥不能重复提交")
			return runtimeSecretMutationInput{}, false
		}
		seen[key] = struct{}{}
		switch strings.TrimSpace(item.Operation) {
		case "set":
			if utf8.RuneCountInString(item.Value) > maxRuntimeEnvironmentValueLength {
				writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_value_too_long", "运行时密钥值过长")
				return runtimeSecretMutationInput{}, false
			}
			if item.Generation != nil {
				writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "设置密钥时不能包含生成参数")
				return runtimeSecretMutationInput{}, false
			}
			input.Values[key] = item.Value
		case "generate":
			if strings.TrimSpace(item.Value) != "" {
				writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "生成密钥时不能同时提交明文值")
				return runtimeSecretMutationInput{}, false
			}
			generation := runtimeSecretGeneration{}
			if item.Generation != nil {
				generation = *item.Generation
			}
			input.Generate[key] = generation
		case "clear":
			if strings.TrimSpace(item.Value) != "" || item.Generation != nil {
				writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "清除密钥时不能包含明文值或生成参数")
				return runtimeSecretMutationInput{}, false
			}
			input.Clear = append(input.Clear, key)
		default:
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_operation_invalid", "运行时密钥操作无效")
			return runtimeSecretMutationInput{}, false
		}
	}
	return input, true
}

func secretEnvironmentVariables(keys []string) []runtimeEnvironmentVariableResponse {
	items := make([]runtimeEnvironmentVariableResponse, 0, len(keys))
	for _, key := range keys {
		items = append(items, runtimeEnvironmentVariableResponse{Key: key, ValueMode: runtimeEnvironmentValueModeSecret, Configured: true})
	}
	return items
}

func decodeRuntimeSecretRefs(raw string) (map[string]string, error) {
	refs := map[string]string{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return refs, nil
	}
	if err := json.Unmarshal([]byte(trimmed), &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func (h *Handlers) deleteSupersededRuntimeSecrets(ctx context.Context, tx *gorm.DB, resourcePrefix string, previousRefs, nextRefs map[string]string) error {
	retained := make(map[string]struct{}, len(nextRefs))
	for _, ref := range nextRefs {
		retained[strings.TrimSpace(ref)] = struct{}{}
	}
	for key, ref := range previousRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, stillRetained := retained[ref]; stillRetained {
			continue
		}
		if err := h.secrets.DeleteRefContextWithDB(ctx, tx, ref, resourcePrefix+":"+key); err != nil {
			return err
		}
	}
	return nil
}

func writeRuntimeSecretMutationError(ctx *gin.Context, ownerType string, err error) {
	if errors.Is(err, errRuntimeEnvironmentModeConflict) {
		writeErrorCode(ctx, http.StatusConflict, "deployment.runtime_environment_value_mode_conflict", "同一运行时环境变量不能同时使用普通值和密钥值")
		return
	}
	telemetry.Logger().ErrorContext(ctx.Request.Context(), "runtime secret mutation failed",
		slog.String("event.name", "runtime_secret.mutation.failed"),
		slog.String("operation", "runtime_secret.mutate"),
		slog.String("resource.type", ownerType),
		slog.String("error.type", telemetry.ErrorType(err)),
	)
	writeErrorCode(ctx, http.StatusInternalServerError, "deployment.secret_store_unavailable", "密钥保存失败")
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
