package api

import (
	"encoding/json"
	"fmt"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/aiapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/retention"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type configDefinition struct {
	Key     string   `json:"key"`
	Type    string   `json:"type"`
	Public  bool     `json:"public"`
	Default string   `json:"default"`
	Options []string `json:"options,omitempty"`
}

type configDefinitionResponse struct {
	Key            string   `json:"key"`
	LabelKey       string   `json:"labelKey"`
	DescriptionKey string   `json:"descriptionKey"`
	Type           string   `json:"type"`
	Public         bool     `json:"public"`
	Default        string   `json:"default"`
	Options        []string `json:"options,omitempty"`
}

var configDefinitions = []configDefinition{
	{
		Key:     aiapi.AssistantEnabledConfigKey,
		Type:    "boolean",
		Public:  false,
		Default: "false",
	},
	{Key: "ai.provider.base_url", Type: "string", Default: ""},
	{Key: "ai.provider.api_key", Type: "secret", Default: ""},
	{
		Key:     "ai.provider.compatibility",
		Type:    "select",
		Default: "auto",
		Options: []string{"auto", "openai", "deepseek"},
	},
	{
		Key:     "ai.provider.prompt_cache_key_mode",
		Type:    "select",
		Default: "auto",
		Options: []string{"auto", "enabled", "disabled"},
	},
	{
		Key:     "ai.provider.channel_affinity_enabled",
		Type:    "boolean",
		Default: "true",
	},
	{Key: "ai.web.proxy_enabled", Type: "boolean", Default: "false"},
	{Key: "ai.web.proxy_pool", Type: "secret", Default: ""},
	{Key: "ai.runtime.provider_timeout_seconds", Type: "number", Default: "300"},
	{Key: "ai.runtime.max_request_retries", Type: "number", Default: "5"},
	{Key: "ai.runtime.run_timeout_seconds", Type: "number", Default: "3600"},
	{Key: "ai.runtime.agent_concurrent_runs", Type: "number", Default: "10"},
	{Key: "ai.observability.enabled", Type: "boolean", Default: "false"},
	{Key: "ai.observability.prometheus_url", Type: "string", Default: ""},
	{Key: "ai.observability.prometheus_token", Type: "secret", Default: ""},
	{Key: "ai.observability.loki_url", Type: "string", Default: ""},
	{Key: "ai.observability.loki_tenant_id", Type: "string", Default: ""},
	{Key: "ai.observability.loki_token", Type: "secret", Default: ""},
	{Key: "ai.observability.tempo_url", Type: "string", Default: ""},
	{Key: "ai.observability.tempo_tenant_id", Type: "string", Default: ""},
	{Key: "ai.observability.tempo_token", Type: "secret", Default: ""},
	{Key: "ai.access.mode", Type: "select", Default: "all_authenticated", Options: []string{"all_authenticated", "admins"}},
	{Key: "ai.quota.user_concurrent_runs", Type: "number", Default: "10"},
	{
		Key:     "site.title",
		Type:    "string",
		Public:  true,
		Default: "Luna DevOps",
	},
	{
		Key:     "site.logoUrl",
		Type:    "string",
		Public:  true,
		Default: "",
	},
	{
		Key:     "site.faviconUrl",
		Type:    "string",
		Public:  true,
		Default: "",
	},
	{
		Key:     "site.loginSubtitle",
		Type:    "string",
		Public:  true,
		Default: "使用本地账号登录控制台",
	},
	{
		Key:     siteBrandColorPresetKey,
		Type:    "select",
		Public:  true,
		Default: defaultBrandColorPreset,
		Options: brandColorPresetOptions,
	},
	{
		Key:     siteMinimalModeDefaultKey,
		Type:    "boolean",
		Public:  true,
		Default: "false",
	},
	{
		Key:     "site.operationsDashboardUrl",
		Type:    "string",
		Public:  false,
		Default: "",
	},
	{
		Key:     "security.egress.domainAllowList",
		Type:    "textarea",
		Public:  false,
		Default: "",
	},
	{
		Key:     "security.egress.domainBlockList",
		Type:    "textarea",
		Public:  false,
		Default: "",
	},
	{
		Key:     "security.egress.ipAllowList",
		Type:    "textarea",
		Public:  false,
		Default: "",
	},
	{
		Key:     "security.egress.ipBlockList",
		Type:    "textarea",
		Public:  false,
		Default: security.ReservedIPBlockListText(),
	},
	{
		Key:     "security.egress.allowedPorts",
		Type:    "textarea",
		Public:  false,
		Default: "",
	},
	{
		Key:     "retention.platformEventsDays",
		Type:    "number",
		Public:  false,
		Default: "90",
	},
	{
		Key:     "retention.notificationDeliveriesDays",
		Type:    "number",
		Public:  false,
		Default: "90",
	},
	{
		Key:     "retention.buildLogsDays",
		Type:    "number",
		Public:  false,
		Default: "30",
	},
	{
		Key:     "retention.releaseLogsDays",
		Type:    "number",
		Public:  false,
		Default: "90",
	},
	{
		Key:     "retention.hookRunLogsDays",
		Type:    "number",
		Public:  false,
		Default: "90",
	},
	{
		Key:     "retention.expiredAuthDataDays",
		Type:    "number",
		Public:  false,
		Default: "30",
	},
	{
		Key:     "billing.creditsDisplayName",
		Type:    "string",
		Public:  true,
		Default: "Credits",
	},
	{
		Key:     "billing.fiatCurrencyUnit",
		Type:    "string",
		Public:  true,
		Default: "CNY",
	},
	{
		Key:     "billing.creditsPerFiatUnit",
		Type:    "string",
		Public:  true,
		Default: "1000",
	},
	{
		Key:     "billing.lowBalanceThresholdCredits",
		Type:    "string",
		Public:  false,
		Default: "100",
	},
	{
		Key:     "billing.blockNewBuildsWhenInsufficient",
		Type:    "select",
		Public:  false,
		Default: "false",
		Options: []string{"true", "false"},
	},
	{
		Key:     "billing.blockDeployChangesWhenInsufficient",
		Type:    "select",
		Public:  false,
		Default: "false",
		Options: []string{"true", "false"},
	},
	{
		Key:     volume.ProjectManagedCapacityLimitConfigKey,
		Type:    "number",
		Public:  false,
		Default: "0",
	},
}

type configCache struct {
	mu     sync.RWMutex
	values map[string]string
	db     *gorm.DB
}

func newConfigCache(db *gorm.DB) *configCache {
	cache := &configCache{values: map[string]string{}, db: db}
	cache.reload(db)
	return cache
}

func (c *configCache) reload(db *gorm.DB) {
	values := map[string]string{}
	for _, definition := range configDefinitions {
		values[definition.Key] = definition.Default
	}

	var rows []model.AppConfig
	if err := db.Find(&rows).Error; err == nil {
		for _, row := range rows {
			values[row.Key] = row.Value
		}
	}

	c.mu.Lock()
	c.values = values
	c.mu.Unlock()
}

func (c *configCache) get(keys []string) map[string]string {
	c.mu.RLock()
	result := map[string]string{}
	for _, key := range keys {
		if value, ok := c.values[key]; ok {
			if definition := configDefinitionByKey(key); definition != nil && definition.Type == "secret" {
				result[key] = strconv.FormatBool(secret.HasValue(value))
				continue
			}
			result[key] = value
		}
	}
	c.mu.RUnlock()

	return result
}

func (c *configCache) set(key, value string) {
	c.mu.Lock()
	c.values[key] = value
	c.mu.Unlock()
}

func (h *Handlers) GetPublicConfigs(ctx *gin.Context) {
	var input configKeysInput
	if !transportapi.BindJSON(ctx, &input) {
		return
	}
	ctx.JSON(http.StatusOK, h.configs.get(publicConfigKeys(input.Keys)))
}

func (h *Handlers) GetConfigs(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		transportapi.WriteErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}

	ctx.JSON(http.StatusOK, h.configs.get(knownConfigKeys()))
}

func (h *Handlers) ListConfigDefinitions(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}

	definitions := make([]configDefinitionResponse, 0, len(configDefinitions))
	for _, definition := range configDefinitions {
		definitions = append(definitions, configDefinitionResponse{
			Key:            definition.Key,
			LabelKey:       "settings.configDefinitions." + definition.Key + ".label",
			DescriptionKey: "settings.configDefinitions." + definition.Key + ".description",
			Type:           definition.Type,
			Public:         definition.Public,
			Default:        definition.Default,
			Options:        definition.Options,
		})
	}

	ctx.JSON(http.StatusOK, definitions)
}

func (h *Handlers) UpdateConfigs(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		transportapi.WriteErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}

	var input updateConfigsInput
	if !transportapi.BindJSON(ctx, &input) {
		return
	}
	if err := aiapi.ValidateAIConfigInputTypes(input.Values, aiConfigDefinition); err != nil {
		transportapi.WriteErrorCode(ctx, http.StatusBadRequest, "ai.config_invalid", err.Error())
		return
	}
	apiKeyInput := ""
	if raw, exists := input.Values["ai.provider.api_key"]; exists {
		if value, ok := raw.(string); ok {
			apiKeyInput = strings.TrimSpace(value)
		}
		delete(input.Values, "ai.provider.api_key")
	}
	proxyPoolInput := ""
	if raw, exists := input.Values["ai.web.proxy_pool"]; exists {
		if value, ok := raw.(string); ok {
			proxyPoolInput = strings.TrimSpace(value)
		}
		delete(input.Values, "ai.web.proxy_pool")
		if proxyPoolInput != "" {
			if _, err := aitool.ParseWebProxyPool(splitProxyPool(proxyPoolInput)); err != nil {
				transportapi.WriteErrorCode(ctx, http.StatusBadRequest, "ai.config_invalid", "ai.web.proxy_pool is invalid")
				return
			}
		}
	}
	observabilitySecretInputs := map[string]string{}
	for _, key := range []string{
		"ai.observability.prometheus_token",
		"ai.observability.loki_token",
		"ai.observability.tempo_token",
	} {
		if raw, exists := input.Values[key]; exists {
			if value, ok := raw.(string); ok {
				observabilitySecretInputs[key] = strings.TrimSpace(value)
			}
			delete(input.Values, key)
		}
	}
	values, err := validateConfigValues(input.Values)
	if err != nil {
		transportapi.WriteError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if proxyPoolInput != "" {
		values["ai.web.proxy_pool"] = "true"
	}
	for key, value := range observabilitySecretInputs {
		if value != "" {
			values[key] = "true"
		}
	}
	if err := h.domains.ai.ValidateAIConfigValues(values); err != nil {
		transportapi.WriteErrorCode(ctx, http.StatusBadRequest, "ai.config_invalid", err.Error())
		return
	}
	if apiKeyInput != "" {
		ref := h.secrets.StoreContext(ctx.Request.Context(), apiKeyInput, user.ID, "ai_provider:api_key")
		if ref == "" {
			transportapi.WriteErrorCode(ctx, http.StatusInternalServerError, "ai.secret_store_failed", "AI Provider API key could not be stored")
			return
		}
		values["ai.provider.api_key"] = ref
	}
	if proxyPoolInput != "" {
		ref := h.secrets.StoreContext(ctx.Request.Context(), proxyPoolInput, user.ID, "ai_web:proxy_pool")
		if ref == "" {
			transportapi.WriteErrorCode(ctx, http.StatusInternalServerError, "ai.secret_store_failed", "AI web proxy pool could not be stored")
			return
		}
		values["ai.web.proxy_pool"] = ref
	}
	for key, value := range observabilitySecretInputs {
		if value == "" {
			continue
		}
		ref := h.secrets.StoreContext(ctx.Request.Context(), value, user.ID, strings.ReplaceAll(key, ".", ":"))
		if ref == "" {
			transportapi.WriteErrorCode(ctx, http.StatusInternalServerError, "ai.secret_store_failed", "Agent observability credential could not be stored")
			return
		}
		values[key] = ref
	}
	aiSecurityChanged := aiapi.ContainsAIConfig(values)

	err = h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockActiveUserRole(tx, user.ID, authz.PlatformRoleAdmin); err != nil {
			return err
		}
		if err := upsertConfigValuesInTransaction(tx, values); err != nil {
			return err
		}
		if aiSecurityChanged {
			return tx.Create(&model.AuditLog{
				ID: id.New("aud"), UserID: user.ID, Action: "ai.settings_update", Resource: "ai.settings",
				Success: true, Message: "AI security settings updated", CreatedAt: time.Now(),
			}).Error
		}
		return nil
	})
	if err != nil {
		transportapi.WriteErrorCode(ctx, http.StatusInternalServerError, "config.update_failed", "configuration update failed")
		return
	}
	h.configs.reload(h.dbFor(ctx))

	ctx.JSON(http.StatusOK, h.configs.get(knownConfigKeys()))
}

func pendingConfigValue(values map[string]string, key, current string) (string, error) {
	value, exists := values[key]
	if !exists {
		return current, nil
	}
	return value, nil
}

func validateConfigValues(input map[string]any) (map[string]string, error) {
	values := make(map[string]string, len(input))
	for key, rawValue := range input {
		definition := configDefinitionByKey(key)
		if definition == nil {
			return nil, fmt.Errorf("unknown config key: %s", key)
		}
		value, err := configValueToString(rawValue)
		if err != nil {
			return nil, fmt.Errorf("invalid config value for %s: %w", key, err)
		}
		if len(definition.Options) > 0 && !configOptionAllowed(value, definition.Options) {
			return nil, fmt.Errorf("invalid config value for %s", key)
		}
		if strings.HasPrefix(key, "retention.") {
			days, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || days < 0 || days > retention.MaxRetentionDays {
				return nil, fmt.Errorf("invalid config value for %s: must be an integer between 0 and %d", key, retention.MaxRetentionDays)
			}
		}
		if key == volume.ProjectManagedCapacityLimitConfigKey {
			limitGiB, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil || limitGiB < 0 || limitGiB > 1048576 {
				return nil, fmt.Errorf("invalid config value for %s: must be an integer between 0 and 1048576", key)
			}
		}
		values[key] = value
	}
	return values, nil
}

func upsertConfigValues(db *gorm.DB, values map[string]string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return upsertConfigValuesInTransaction(tx, values)
	})
}

func upsertConfigValuesInTransaction(tx *gorm.DB, values map[string]string) error {
	now := time.Now()
	for key, value := range values {
		row := model.AppConfig{Key: key, Value: value, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func isBooleanConfigValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false", "1", "0", "yes", "no", "on", "off", "enabled", "disabled":
		return true
	default:
		return false
	}
}

func publicConfigKeys(keys []string) []string {
	allowed := map[string]bool{}
	for _, definition := range configDefinitions {
		if definition.Public {
			allowed[definition.Key] = true
		}
	}

	result := make([]string, 0, len(keys))
	for _, key := range keys {
		if allowed[key] {
			result = append(result, key)
		}
	}
	return result
}

func knownConfigKeys() []string {
	keys := make([]string, 0, len(configDefinitions))
	for _, definition := range configDefinitions {
		keys = append(keys, definition.Key)
	}
	return keys
}

func isKnownConfigKey(key string) bool {
	return configDefinitionByKey(key) != nil
}

func configDefinitionByKey(key string) *configDefinition {
	for _, definition := range configDefinitions {
		if definition.Key == key {
			return &definition
		}
	}
	return nil
}

func configOptionAllowed(value string, options []string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func configValueToString(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case bool:
		return fmt.Sprintf("%t", typed), nil
	case float64:
		// JSON numbers are decoded as float64. Keep their decimal representation
		// stable so large integer configuration values do not become scientific
		// notation (for example 2000000 -> 2e+06) before integer validation.
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

type configKeysInput struct {
	Keys []string `json:"keys"`
}

type updateConfigsInput struct {
	Values map[string]any `json:"values"`
}
