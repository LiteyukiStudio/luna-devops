package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/aiapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"gorm.io/gorm"
)

type adminGetSystemConfigsInput struct {
	Keys []string `json:"keys,omitempty" desc:"可选指定要查询的系统配置键名列表；省略时返回平台全部 50 项全局配置的元数据定义与当前生效值（5 项密钥仅回显 secretSet 布尔状态）"`
}

type systemConfigSecretAction struct {
	Key       string  `json:"key" required:"true" enum:"ai.provider.api_key|ai.web.proxy_pool|ai.observability.prometheus_token|ai.observability.loki_token|ai.observability.tempo_token" desc:"平台加密密钥配置键名"`
	Operation *string `json:"operation,omitempty" enum:"set|clear" default:"set" desc:"密钥操作类型：set（默认，写入新密钥）或 clear（清除已存密钥）"`
	Value     *string `json:"value,omitempty" sensitive:"true" desc:"operation=set 时必填的新密钥明文（writeOnly，绝不回显）"`
}

type adminUpdateSystemConfigsInput struct {
	SetConfigs      map[string]string          `json:"setConfigs,omitempty" desc:"增量设置非敏感配置项（仅允许 45 项非 secret 配置键的非空字符串值；清空可空字符串请用 clearConfigKeys；密钥请用 secretActions）"`
	ClearConfigKeys []string                   `json:"clearConfigKeys,omitempty" desc:"显式清空允许置空的非敏感字符串或多行文本配置键（如 site.logoUrl、security.egress.* 等）"`
	SecretActions   []systemConfigSecretAction `json:"secretActions,omitempty" desc:"对 5 项平台加密密钥配置执行增量设置（set）或清除（clear）"`
}

type systemConfigItemView struct {
	Key       string   `json:"key"`
	Type      string   `json:"type"`
	Public    bool     `json:"public"`
	Default   string   `json:"default,omitempty"`
	Options   []string `json:"options,omitempty"`
	Value     *string  `json:"value,omitempty"`
	SecretSet *bool    `json:"secretSet,omitempty"`
}

var nonClearableStringConfigKeys = map[string]bool{
	"site.title":                         true,
	"billing.creditsDisplayName":         true,
	"billing.fiatCurrencyUnit":           true,
	"billing.creditsPerFiatUnit":         true,
	"billing.lowBalanceThresholdCredits": true,
}

func (h *Handlers) registerConfigAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[adminGetSystemConfigsInput, map[string]any]{
		Name:           "admin_get_system_configs",
		Domain:         aitool.DomainAdmin,
		Title:          "查看平台全局系统配置",
		Summary:        "查看平台全部 50 项全局系统配置（45 项非敏感配置与 5 项加密密钥配置）的元数据定义与当前生效值，绝不回显明文密钥",
		AliasesZH:      []string{"系统配置", "全局设置", "站点设置", "出站白名单配置", "计费阈值配置"},
		AliasesEN:      []string{"get system configs", "platform settings", "global configs"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionConfigRead)},
		Execute:        h.executeAdminGetSystemConfigs,
	})

	aitool.Register(reg, aitool.Spec[adminUpdateSystemConfigsInput, map[string]any]{
		Name:           "admin_update_system_configs",
		Domain:         aitool.DomainAdmin,
		Title:          "增量修改平台全局系统配置",
		Summary:        "纯增量更新平台全局配置项，严格物理隔离 45 项非敏感配置与 5 项加密密钥配置，具备启用态联动防断连保护",
		AliasesZH:      []string{"修改系统配置", "更新全局设置", "配置 AI Provider", "更新站点设置"},
		AliasesEN:      []string{"update system configs", "save platform settings"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionConfigWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID: "admin_get_system_configs",
			Family:      "resource",
			Singleton:   true,
		}},
		Preview: h.previewAdminUpdateSystemConfigs,
		Execute: h.executeAdminUpdateSystemConfigs,
	})
}

func (h *Handlers) executeAdminGetSystemConfigs(tc *aitool.ToolContext, in adminGetSystemConfigsInput) (map[string]any, error) {
	tc.SetNoStore()
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	var filterSet map[string]bool
	if len(in.Keys) > 0 {
		filterSet = make(map[string]bool, len(in.Keys))
		for _, rawKey := range in.Keys {
			key := strings.TrimSpace(rawKey)
			if key == "" {
				continue
			}
			if configDefinitionByKey(key) == nil {
				return nil, aitool.BadRequest("config.unknown_key", fmt.Sprintf("unknown config key: %s", key))
			}
			filterSet[key] = true
		}
	}

	rawValues, err := h.loadRawSystemConfigMap(tc.DB)
	if err != nil {
		return nil, err
	}
	return buildMaskedSystemConfigsResponse(rawValues, filterSet), nil
}

func (h *Handlers) previewAdminUpdateSystemConfigs(tc *aitool.ToolContext, in adminUpdateSystemConfigsInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	rawCurrent, err := h.loadRawSystemConfigMap(tc.DB)
	if err != nil {
		return nil, err
	}

	plan, err := h.prepareSystemConfigsMutation(rawCurrent, in)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Update %d system configuration key(s)", len(plan.changedKeys)),
			TargetResource: "system_configs",
			Changes:        plan.changes,
			SensitiveKeys:  plan.sensitiveKeys,
		},
	}, nil
}

func (h *Handlers) executeAdminUpdateSystemConfigs(tc *aitool.ToolContext, in adminUpdateSystemConfigsInput) (map[string]any, error) {
	tc.SetNoStore()
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}
	if len(in.SecretActions) > 0 && !h.secrets.Available() {
		for _, act := range in.SecretActions {
			op := strings.ToLower(strings.TrimSpace(derefConfigStr(act.Operation)))
			if op == "" || op == "set" {
				return nil, aitool.NewError(http.StatusServiceUnavailable, "ai.secret_store_unavailable", "secret store is unavailable")
			}
		}
	}

	rawCurrent, err := h.loadRawSystemConfigMap(tc.DB)
	if err != nil {
		return nil, err
	}
	plan, err := h.prepareSystemConfigsMutation(rawCurrent, in)
	if err != nil {
		return nil, err
	}

	err = tc.DB.Transaction(func(tx *gorm.DB) error {
		if _, lockErr := lockActiveUserRole(tx, tc.User.ID, authz.PlatformRoleAdmin); lockErr != nil {
			return aitool.Forbidden("config.admin.required", "platform administrator role is required")
		}

		valuesToUpsert := make(map[string]string, len(plan.nonSecretUpdates)+len(plan.secretOps))
		for k, v := range plan.nonSecretUpdates {
			valuesToUpsert[k] = v
		}

		for _, secOp := range plan.secretOps {
			var existingRow model.AppConfig
			oldRef := ""
			if fErr := tx.First(&existingRow, "key = ?", secOp.key).Error; fErr == nil {
				oldRef = strings.TrimSpace(existingRow.Value)
			}
			resName := systemConfigSecretResourceName(secOp.key)

			if secOp.operation == "set" {
				newRef, sErr := h.secrets.StoreContextWithDB(tc.Context(), tx, secOp.value, tc.User.ID, resName)
				if sErr != nil || newRef == "" {
					return aitool.NewError(http.StatusInternalServerError, "ai.secret_store_failed", fmt.Sprintf("failed to store secret for %s", secOp.key))
				}
				if oldRef != "" && oldRef != newRef {
					_ = h.secrets.DeleteRefContextWithDB(tc.Context(), tx, oldRef, resName)
				}
				valuesToUpsert[secOp.key] = newRef
			} else {
				if oldRef != "" {
					if dErr := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, oldRef, resName); dErr != nil {
						return aitool.NewError(http.StatusInternalServerError, "ai.secret_delete_failed", fmt.Sprintf("failed to delete secret for %s", secOp.key))
					}
				}
				valuesToUpsert[secOp.key] = ""
			}
		}

		if uErr := upsertConfigValuesInTransaction(tx, valuesToUpsert); uErr != nil {
			return uErr
		}

		now := time.Now()
		if aErr := tx.Create(&model.AuditLog{
			ID:        id.New("aud"),
			UserID:    tc.User.ID,
			Action:    "config.update",
			Resource:  "system.configs",
			Success:   true,
			Message:   "updated keys: " + strings.Join(plan.changedKeys, ","),
			CreatedAt: now,
		}).Error; aErr != nil {
			return aErr
		}
		if aiapi.ContainsAIConfig(valuesToUpsert) {
			if aiErr := tx.Create(&model.AuditLog{
				ID:        id.New("aud"),
				UserID:    tc.User.ID,
				Action:    "ai.settings_update",
				Resource:  "ai.settings",
				Success:   true,
				Message:   "AI security settings updated",
				CreatedAt: now,
			}).Error; aiErr != nil {
				return aiErr
			}
		}
		return nil
	})
	if err != nil {
		if _, ok := aitool.AsToolError(err); ok {
			return nil, err
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "config.update_failed", "configuration update failed")
	}

	if h.configs != nil {
		h.configs.reload(tc.DB)
	}

	rawAfter, err := h.loadRawSystemConfigMap(tc.DB)
	if err != nil {
		return nil, err
	}
	out := buildMaskedSystemConfigsResponse(rawAfter, nil)
	out["updatedKeys"] = plan.changedKeys
	return out, nil
}

type preparedSecretOp struct {
	key       string
	operation string
	value     string
}

type preparedSystemConfigPlan struct {
	nonSecretUpdates map[string]string
	secretOps        []preparedSecretOp
	changes          []aitool.PreviewChange
	sensitiveKeys    []string
	changedKeys      []string
}

func (h *Handlers) prepareSystemConfigsMutation(rawCurrent map[string]string, in adminUpdateSystemConfigsInput) (preparedSystemConfigPlan, error) {
	if len(in.SetConfigs) == 0 && len(in.ClearConfigKeys) == 0 && len(in.SecretActions) == 0 {
		return preparedSystemConfigPlan{}, aitool.BadRequest("config.update_noop", "at least one of setConfigs, clearConfigKeys, or secretActions is required")
	}

	nonSecretAny := make(map[string]any, len(in.SetConfigs))
	setKeys := make([]string, 0, len(in.SetConfigs))
	for k := range in.SetConfigs {
		setKeys = append(setKeys, k)
	}
	sort.Strings(setKeys)

	for _, key := range setKeys {
		val := in.SetConfigs[key]
		def := configDefinitionByKey(key)
		if def == nil {
			return preparedSystemConfigPlan{}, aitool.BadRequest("config.unknown_key", fmt.Sprintf("unknown config key: %s", key))
		}
		if def.Type == "secret" {
			return preparedSystemConfigPlan{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"config.secret_key_isolated",
				fmt.Sprintf("%s is a secret configuration key and cannot be passed in setConfigs", key),
				"Use secretActions with operation=set or operation=clear for secret configuration keys.",
			)
		}
		if strings.TrimSpace(val) == "" {
			return preparedSystemConfigPlan{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"config.empty_value_forbidden",
				fmt.Sprintf("empty value is not allowed in setConfigs for %s", key),
				"Pass nullable string/textarea keys in clearConfigKeys instead of passing an empty string.",
			)
		}
		nonSecretAny[key] = val
	}

	if err := aiapi.ValidateAIConfigInputTypes(nonSecretAny, aiConfigDefinition); err != nil {
		return preparedSystemConfigPlan{}, aitool.BadRequest("ai.config_invalid", err.Error())
	}
	validatedSet, err := validateConfigValues(nonSecretAny)
	if err != nil {
		return preparedSystemConfigPlan{}, aitool.BadRequest("config.value_invalid", err.Error())
	}

	nonSecretUpdates := make(map[string]string, len(validatedSet)+len(in.ClearConfigKeys))
	for k, v := range validatedSet {
		nonSecretUpdates[k] = v
	}

	seenClear := make(map[string]bool, len(in.ClearConfigKeys))
	for _, rawKey := range in.ClearConfigKeys {
		key := strings.TrimSpace(rawKey)
		if key == "" || seenClear[key] {
			continue
		}
		seenClear[key] = true
		def := configDefinitionByKey(key)
		if def == nil {
			return preparedSystemConfigPlan{}, aitool.BadRequest("config.unknown_key", fmt.Sprintf("unknown config key: %s", key))
		}
		if def.Type == "secret" {
			return preparedSystemConfigPlan{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"config.secret_key_isolated",
				fmt.Sprintf("%s is a secret configuration key and cannot be cleared via clearConfigKeys", key),
				"Use secretActions with operation=clear to clear a secret configuration key.",
			)
		}
		if _, conflict := nonSecretUpdates[key]; conflict {
			return preparedSystemConfigPlan{}, aitool.BadRequest("config.key_conflict", fmt.Sprintf("key %s cannot appear in both setConfigs and clearConfigKeys", key))
		}
		if !isClearableNonSecretConfigKey(def) {
			return preparedSystemConfigPlan{}, aitool.BadRequest("config.clear_not_supported", fmt.Sprintf("config key %s does not support clearing to empty", key))
		}
		nonSecretUpdates[key] = ""
	}

	seenSecrets := make(map[string]bool, len(in.SecretActions))
	secretOps := make([]preparedSecretOp, 0, len(in.SecretActions))
	var sensitiveKeys []string

	for _, act := range in.SecretActions {
		key := strings.TrimSpace(act.Key)
		def := configDefinitionByKey(key)
		if def == nil || def.Type != "secret" {
			return preparedSystemConfigPlan{}, aitool.BadRequest("config.invalid_secret_key", fmt.Sprintf("%s is not a valid secret configuration key", key))
		}
		if seenSecrets[key] {
			return preparedSystemConfigPlan{}, aitool.BadRequest("config.duplicate_secret_key", fmt.Sprintf("duplicate secret key %s in secretActions", key))
		}
		seenSecrets[key] = true

		op := strings.ToLower(strings.TrimSpace(derefConfigStr(act.Operation)))
		if op == "" {
			op = "set"
		}
		switch op {
		case "set":
			val := strings.TrimSpace(derefConfigStr(act.Value))
			if val == "" {
				return preparedSystemConfigPlan{}, aitool.BadRequest("config.secret_value_required", fmt.Sprintf("non-empty value is required when setting secret %s", key))
			}
			if key == "ai.web.proxy_pool" {
				if _, pErr := aitool.ParseWebProxyPool(splitProxyPool(val)); pErr != nil {
					return preparedSystemConfigPlan{}, aitool.BadRequest("ai.config_invalid", "ai.web.proxy_pool is invalid")
				}
			}
			secretOps = append(secretOps, preparedSecretOp{key: key, operation: "set", value: val})
			sensitiveKeys = append(sensitiveKeys, key)
		case "clear":
			if act.Value != nil && strings.TrimSpace(*act.Value) != "" {
				return preparedSystemConfigPlan{}, aitool.BadRequest("config.argument_invalid", fmt.Sprintf("value must be omitted when clearing secret %s", key))
			}
			secretOps = append(secretOps, preparedSecretOp{key: key, operation: "clear"})
			sensitiveKeys = append(sensitiveKeys, key)
		default:
			return preparedSystemConfigPlan{}, aitool.BadRequest("config.argument_invalid", "secretActions.operation must be set or clear")
		}
	}

	effectiveValues := make(map[string]string, len(configDefinitions))
	effectiveSecretSet := make(map[string]bool, 5)
	for _, def := range configDefinitions {
		raw := rawCurrent[def.Key]
		if def.Type == "secret" {
			effectiveSecretSet[def.Key] = secret.HasValue(raw)
		} else {
			effectiveValues[def.Key] = raw
		}
	}
	for k, v := range nonSecretUpdates {
		effectiveValues[k] = v
	}
	for _, secOp := range secretOps {
		effectiveSecretSet[secOp.key] = secOp.operation == "set"
	}

	// Self-protection linkage guards when features are enabled
	if isTruthySystemConfigBool(effectiveValues[aiapi.AssistantEnabledConfigKey]) {
		if strings.TrimSpace(effectiveValues["ai.provider.base_url"]) == "" || !effectiveSecretSet["ai.provider.api_key"] {
			return preparedSystemConfigPlan{}, aitool.Conflict(
				"ai.config_in_use",
				"ai.provider.base_url and ai.provider.api_key cannot be empty or cleared while ai.assistant.enabled is true",
			)
		}
	}
	if isTruthySystemConfigBool(effectiveValues["ai.web.proxy_enabled"]) {
		if !effectiveSecretSet["ai.web.proxy_pool"] {
			return preparedSystemConfigPlan{}, aitool.Conflict(
				"ai.config_in_use",
				"ai.web.proxy_pool cannot be empty or cleared while ai.web.proxy_enabled is true",
			)
		}
	}
	if isTruthySystemConfigBool(effectiveValues["ai.observability.enabled"]) {
		for _, urlKey := range []string{
			"ai.observability.prometheus_url",
			"ai.observability.loki_url",
			"ai.observability.tempo_url",
		} {
			if strings.TrimSpace(effectiveValues[urlKey]) == "" {
				return preparedSystemConfigPlan{}, aitool.Conflict(
					"ai.config_in_use",
					fmt.Sprintf("%s cannot be empty or cleared while ai.observability.enabled is true", urlKey),
				)
			}
		}
	}

	if h != nil && h.domains != nil && h.domains.ai != nil {
		validationSubmission := make(map[string]string, len(nonSecretUpdates)+len(secretOps))
		for k, v := range nonSecretUpdates {
			validationSubmission[k] = v
		}
		for _, secOp := range secretOps {
			if secOp.operation == "set" {
				validationSubmission[secOp.key] = "true"
			}
		}
		if vErr := h.domains.ai.ValidateAIConfigValues(validationSubmission); vErr != nil {
			return preparedSystemConfigPlan{}, aitool.BadRequest("ai.config_invalid", vErr.Error())
		}
	}

	changedSet := make(map[string]bool)
	var changes []aitool.PreviewChange

	nonSecretKeysSorted := make([]string, 0, len(nonSecretUpdates))
	for k := range nonSecretUpdates {
		nonSecretKeysSorted = append(nonSecretKeysSorted, k)
	}
	sort.Strings(nonSecretKeysSorted)
	for _, k := range nonSecretKeysSorted {
		after := nonSecretUpdates[k]
		before := rawCurrent[k]
		action := "update"
		if after == "" {
			action = "clear"
		}
		changes = append(changes, aitool.PreviewChange{
			Field:  k,
			Before: before,
			After:  after,
			Action: action,
		})
		changedSet[k] = true
	}

	for _, secOp := range secretOps {
		wasSet := secret.HasValue(rawCurrent[secOp.key])
		willBeSet := secOp.operation == "set"
		changes = append(changes, aitool.PreviewChange{
			Field:  secOp.key + ".secretSet",
			Before: wasSet,
			After:  willBeSet,
			Action: secOp.operation,
		})
		changedSet[secOp.key] = true
	}

	changedKeys := make([]string, 0, len(changedSet))
	for k := range changedSet {
		changedKeys = append(changedKeys, k)
	}
	sort.Strings(changedKeys)
	sort.Strings(sensitiveKeys)

	return preparedSystemConfigPlan{
		nonSecretUpdates: nonSecretUpdates,
		secretOps:        secretOps,
		changes:          changes,
		sensitiveKeys:    sensitiveKeys,
		changedKeys:      changedKeys,
	}, nil
}

func (h *Handlers) loadRawSystemConfigMap(db *gorm.DB) (map[string]string, error) {
	values := make(map[string]string, len(configDefinitions))
	for _, def := range configDefinitions {
		values[def.Key] = def.Default
	}
	if db != nil {
		var rows []model.AppConfig
		if err := db.Find(&rows).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		for _, row := range rows {
			values[row.Key] = row.Value
		}
		return values, nil
	}
	if h != nil && h.configs != nil {
		h.configs.mu.RLock()
		for k, v := range h.configs.values {
			values[k] = v
		}
		h.configs.mu.RUnlock()
	}
	return values, nil
}

func buildMaskedSystemConfigsResponse(rawValues map[string]string, filterSet map[string]bool) map[string]any {
	items := make([]systemConfigItemView, 0, len(configDefinitions))
	values := make(map[string]string, len(configDefinitions))
	secretStatus := make(map[string]bool, 5)
	secretKeys := make([]string, 0, 5)

	for _, def := range configDefinitions {
		if filterSet != nil && !filterSet[def.Key] {
			continue
		}
		rawVal := rawValues[def.Key]
		if def.Type == "secret" {
			configured := secret.HasValue(rawVal)
			secretStatus[def.Key] = configured
			if configured {
				secretKeys = append(secretKeys, def.Key)
			}
			items = append(items, systemConfigItemView{
				Key:       def.Key,
				Type:      def.Type,
				Public:    def.Public,
				SecretSet: &configured,
			})
			continue
		}
		valCopy := rawVal
		values[def.Key] = valCopy
		items = append(items, systemConfigItemView{
			Key:     def.Key,
			Type:    def.Type,
			Public:  def.Public,
			Default: def.Default,
			Options: def.Options,
			Value:   &valCopy,
		})
	}
	sort.Strings(secretKeys)

	return map[string]any{
		"items":        items,
		"values":       values,
		"secretStatus": secretStatus,
		"secretKeys":   secretKeys,
		"total":        len(items),
	}
}

func isClearableNonSecretConfigKey(def *configDefinition) bool {
	if def == nil {
		return false
	}
	if def.Type != "string" && def.Type != "textarea" {
		return false
	}
	return !nonClearableStringConfigKeys[def.Key]
}

func isTruthySystemConfigBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func systemConfigSecretResourceName(key string) string {
	switch key {
	case "ai.provider.api_key":
		return "ai_provider:api_key"
	case "ai.web.proxy_pool":
		return "ai_web:proxy_pool"
	default:
		return strings.ReplaceAll(key, ".", ":")
	}
}

func derefConfigStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
