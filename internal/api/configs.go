package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
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
	Key         string   `json:"key"`
	Label       string   `json:"-"`
	Description string   `json:"-"`
	Type        string   `json:"type"`
	Public      bool     `json:"public"`
	Default     string   `json:"default"`
	Options     []string `json:"options,omitempty"`
}

const (
	aiRunMaxToolCallsMin     = 32
	aiRunMaxToolCallsMax     = 2048
	aiRunMaxToolCallsDefault = 256
)

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
		Key:         aiAssistantEnabledConfigKey,
		Label:       "启用内嵌 AI 助手",
		Description: "部署级 AI 功能可用时，允许访问范围内的已登录用户使用内嵌助手。",
		Type:        "boolean",
		Public:      false,
		Default:     "false",
	},
	{Key: "ai.provider.base_url", Label: "AI API 地址", Type: "string", Default: ""},
	{Key: "ai.provider.api_key", Label: "AI API Key", Type: "secret", Default: ""},
	{Key: "ai.web.proxy_enabled", Label: "AI 外网工具代理池", Type: "boolean", Default: "false"},
	{Key: "ai.web.proxy_pool", Label: "AI 外网工具代理地址", Type: "secret", Default: ""},
	{Key: "ai.runtime.provider_timeout_seconds", Label: "模型请求超时", Type: "number", Default: "300"},
	{Key: "ai.runtime.max_request_retries", Label: "瞬时故障重试次数", Type: "number", Default: "5"},
	{Key: "ai.runtime.run_timeout_seconds", Label: "单次 Run 超时", Type: "number", Default: "3600"},
	{Key: "ai.runtime.agent_concurrent_runs", Label: "Agent 实例并发 Run", Type: "number", Default: "10"},
	{Key: "ai.runtime.context_input_k_tokens", Label: "上下文输入预算", Description: "单次模型请求允许使用的最大输入上下文，范围 64–2048K，默认 1024K；实际用量仍受所选模型上下文容量和输出预留限制。", Type: "number", Default: "1024"},
	{Key: "ai.observability.enabled", Label: "启用 Agent 可观测", Type: "boolean", Default: "false"},
	{Key: "ai.observability.prometheus_url", Label: "Prometheus 查询地址", Type: "string", Default: ""},
	{Key: "ai.observability.prometheus_token", Label: "Prometheus 访问令牌", Type: "secret", Default: ""},
	{Key: "ai.observability.loki_url", Label: "Loki 查询地址", Type: "string", Default: ""},
	{Key: "ai.observability.loki_tenant_id", Label: "Loki Tenant ID", Type: "string", Default: ""},
	{Key: "ai.observability.loki_token", Label: "Loki 访问令牌", Type: "secret", Default: ""},
	{Key: "ai.observability.tempo_url", Label: "Tempo 查询地址", Type: "string", Default: ""},
	{Key: "ai.observability.tempo_tenant_id", Label: "Tempo Tenant ID", Type: "string", Default: ""},
	{Key: "ai.observability.tempo_token", Label: "Tempo 访问令牌", Type: "secret", Default: ""},
	{Key: "ai.access.mode", Label: "AI 访问范围", Type: "select", Default: "all_authenticated", Options: []string{"all_authenticated", "admins"}},
	{Key: "ai.quota.user_concurrent_runs", Label: "用户并发 Run", Type: "number", Default: "10"},
	{Key: "ai.quota.user_daily_tokens", Label: "用户每日 Token", Type: "number", Default: "200000"},
	{Key: "ai.quota.project_concurrent_runs", Label: "项目并发 Run", Type: "number", Default: "5"},
	{Key: "ai.quota.run_max_tool_calls", Label: "Run 最大工具调用", Type: "number", Default: strconv.Itoa(aiRunMaxToolCallsDefault)},
	{Key: "ai.quota.platform_daily_cost_soft", Label: "平台每日成本软上限", Type: "number", Default: "0"},
	{Key: "ai.quota.platform_daily_cost_hard", Label: "平台每日成本硬上限", Type: "number", Default: "0"},
	{Key: "ai.retention.conversation_days", Label: "AI 会话保留天数", Type: "number", Default: "90"},
	{Key: "ai.retention.run_event_days", Label: "AI Run 事件保留天数", Type: "number", Default: "30"},
	{Key: "ai.retention.checkpoint_days", Label: "AI Checkpoint 保留天数", Type: "number", Default: "7"},
	{
		Key:         "ai.context.max_uncompressed_turn_count",
		Label:       "未压缩轮次阈值",
		Description: "历史轮次超过该数量后强制触发压缩，范围 4–128，默认 64。",
		Type:        "number",
		Default:     "64",
	},
	{
		Key:         "ai.context.max_compression_turns_per_compile",
		Label:       "单次压缩最大轮次数",
		Description: "一次上下文编译最多送入压缩器的历史轮次数，范围 8–1024，默认 512。",
		Type:        "number",
		Default:     "512",
	},
	{
		Key:         "ai.context.summary_input_k_tokens",
		Label:       "摘要输入预算（K Token）",
		Description: "生成历史摘要时单批允许的输入 Token 预算，范围 4–512K，默认 256K。",
		Type:        "number",
		Default:     "256",
	},
	{
		Key:         "ai.context.summary_max_output_tokens",
		Label:       "摘要最大输出 Token",
		Description: "单次摘要生成允许的最大输出 Token 数，范围 200–32768，默认 16384。",
		Type:        "number",
		Default:     "16384",
	},
	{
		Key:         "ai.model.max_output_tokens",
		Label:       "模型最大输出 Token",
		Description: "助手每次回复允许的最大输出 Token 数，范围 256–131072，默认 65536。实际可用上限仍由模型服务决定。",
		Type:        "number",
		Default:     "65536",
	},
	{
		Key:         "ai.run.max_model_steps",
		Label:       "Run 最大模型轮次",
		Description: "单个 Run 内模型生成（含工具调用）的最大轮次数，用于防止失控循环，范围 1–1024，默认 256。",
		Type:        "number",
		Default:     "256",
	},
	{
		Key:         "ai.run.max_input_k_bytes",
		Label:       "用户输入大小上限（KB）",
		Description: "单轮用户输入允许的最大字节数，范围 8–8192K，默认 1024K。",
		Type:        "number",
		Default:     "1024",
	},
	{
		Key:         "ai.run.navigate_action_ttl_seconds",
		Label:       "路由跳转动作有效期（秒）",
		Description: "navigate_to_route 生成的跳转动作可被前端执行的秒数，范围 10–600，默认 120。",
		Type:        "number",
		Default:     "120",
	},
	{
		Key:         "ai.tools.max_card_repair_attempts",
		Label:       "交互卡片修复上限",
		Description: "模型生成的交互卡片参数校验失败后允许自动修正的最大次数，范围 1–10，默认 5。",
		Type:        "number",
		Default:     "5",
	},
	{
		Key:         "site.title",
		Label:       "网站标题",
		Description: "浏览器标题和控制台品牌名称。",
		Type:        "string",
		Public:      true,
		Default:     "Luna DevOps",
	},
	{
		Key:         "site.logoUrl",
		Label:       "Logo 地址",
		Description: "控制台左上角 Logo 图片地址，留空时使用默认图标。",
		Type:        "string",
		Public:      true,
		Default:     "",
	},
	{
		Key:         "site.faviconUrl",
		Label:       "Favicon 地址",
		Description: "浏览器标签页图标地址，留空时使用默认 favicon。",
		Type:        "string",
		Public:      true,
		Default:     "",
	},
	{
		Key:         "site.loginSubtitle",
		Label:       "登录页副标题",
		Description: "登录页品牌下方的短说明。",
		Type:        "string",
		Public:      true,
		Default:     "使用本地账号登录控制台",
	},
	{
		Key:         siteBrandColorPresetKey,
		Label:       "默认品牌主题色",
		Description: "控制台按钮、链接、选中态和焦点环使用的默认主题色。没有个人主题色偏好的用户会持续跟随此设置。",
		Type:        "select",
		Public:      true,
		Default:     defaultBrandColorPreset,
		Options:     brandColorPresetOptions,
	},
	{
		Key:         siteMinimalModeDefaultKey,
		Label:       "默认启用简约模式",
		Description: "启用后，未设置个人界面风格的用户默认使用中性画布；用户可以在个人资料中覆盖。",
		Type:        "boolean",
		Public:      true,
		Default:     "false",
	},
	{
		Key:         "site.operationsDashboardUrl",
		Label:       "运营面板地址",
		Description: "用于平台管理员查看运营大盘的 Grafana dashboard 或 panel iframe 地址。留空时不展示运营面板内容。",
		Type:        "string",
		Public:      false,
		Default:     "",
	},
	{
		Key:         "security.egress.domainAllowList",
		Label:       "SSRF 域名特许白名单",
		Description: "每行一个域名或通配符域名。命中后直接允许该域名，适合本地 FakeIP、内网镜像站等明确可信目标。",
		Type:        "textarea",
		Public:      false,
		Default:     "",
	},
	{
		Key:         "security.egress.domainBlockList",
		Label:       "SSRF 域名黑名单",
		Description: "每行一个域名或通配符域名。命中后直接拒绝访问。",
		Type:        "textarea",
		Public:      false,
		Default:     "",
	},
	{
		Key:         "security.egress.ipAllowList",
		Label:       "SSRF IP 白名单",
		Description: "每行一个 IP 或 CIDR。用于允许直连或解析结果命中的私网/保留地址。",
		Type:        "textarea",
		Public:      false,
		Default:     "",
	},
	{
		Key:         "security.egress.ipBlockList",
		Label:       "SSRF IP 黑名单",
		Description: "每行一个 IP 或 CIDR。用于拦截直连 IP 或非白名单域名的解析结果；域名白名单命中时不再二次检查 IP 黑名单。",
		Type:        "textarea",
		Public:      false,
		Default:     security.ReservedIPBlockListText(),
	},
	{
		Key:         "security.egress.allowedPorts",
		Label:       "SSRF 允许端口",
		Description: "可选。留空表示不限制端口；填写后每行一个端口，只允许这些端口。",
		Type:        "textarea",
		Public:      false,
		Default:     "",
	},
	{
		Key:         "retention.platformEventsDays",
		Label:       "平台事件保留天数",
		Description: "平台事件明细的保留天数，0 表示不自动清理。",
		Type:        "number",
		Public:      false,
		Default:     "90",
	},
	{
		Key:         "retention.notificationDeliveriesDays",
		Label:       "通知投递记录保留天数",
		Description: "通知投递记录的保留天数，0 表示不自动清理。",
		Type:        "number",
		Public:      false,
		Default:     "90",
	},
	{
		Key:         "retention.workerTaskEventsDays",
		Label:       "Worker 任务事件保留天数",
		Description: "Worker 任务事件的保留天数，0 表示不自动清理。",
		Type:        "number",
		Public:      false,
		Default:     "30",
	},
	{
		Key:         "retention.buildLogsDays",
		Label:       "构建日志保留天数",
		Description: "构建日志内容的保留天数，0 表示不自动清理。",
		Type:        "number",
		Public:      false,
		Default:     "30",
	},
	{
		Key:         "retention.releaseLogsDays",
		Label:       "发布日志保留天数",
		Description: "发布日志内容的保留天数，0 表示不自动清理。",
		Type:        "number",
		Public:      false,
		Default:     "90",
	},
	{
		Key:         "retention.hookRunLogsDays",
		Label:       "Hook 运行日志保留天数",
		Description: "Hook 运行日志内容的保留天数，0 表示不自动清理。",
		Type:        "number",
		Public:      false,
		Default:     "90",
	},
	{
		Key:         "retention.expiredAuthDataDays",
		Label:       "过期认证数据保留天数",
		Description: "过期认证会话与临时数据的保留天数，0 表示不自动清理。",
		Type:        "number",
		Public:      false,
		Default:     "30",
	},
	{
		Key:         "billing.creditsDisplayName",
		Label:       "Credits 展示名称",
		Description: "控制台展示平台内部 credits 时使用的名称。底层仍统一按 credits 存储和结算。",
		Type:        "string",
		Public:      true,
		Default:     "Credits",
	},
	{
		Key:         "billing.fiatCurrencyUnit",
		Label:       "现实货币单位",
		Description: "平台管理员在账单概览中查看 credits 折算金额时使用的现实货币单位，例如 CNY、USD 或 元。",
		Type:        "string",
		Public:      true,
		Default:     "CNY",
	},
	{
		Key:         "billing.creditsPerFiatUnit",
		Label:       "每 1 现实货币对应 Credits",
		Description: "用于管理员账单概览展示换算金额。例：1000 表示 1 个现实货币单位可兑换 1000 credits。",
		Type:        "string",
		Public:      true,
		Default:     "1000",
	},
	{
		Key:         "billing.freeQuotaCredits",
		Label:       "默认免费额度",
		Description: "新用户钱包可获得的默认 credits 额度。当前用于后续充值与额度策略，已创建用户不会自动补发。",
		Type:        "string",
		Public:      false,
		Default:     "0",
	},
	{
		Key:         "billing.lowBalanceThresholdCredits",
		Label:       "低余额提醒阈值",
		Description: "计费归属人余额低于该 credits 数值时，后续可用于展示提醒或触发通知。",
		Type:        "string",
		Public:      false,
		Default:     "100",
	},
	{
		Key:         "billing.overdueGracePeriodHours",
		Label:       "欠费宽限期",
		Description: "计费归属人余额不足后允许继续运行的小时数。限制策略启用后会使用该值。",
		Type:        "string",
		Public:      false,
		Default:     "72",
	},
	{
		Key:         "billing.allowNegativeBalance",
		Label:       "允许欠费余额",
		Description: "是否允许账本扣到负余额。关闭后，后续限制策略会阻止新的付费操作。",
		Type:        "select",
		Public:      false,
		Default:     "true",
		Options:     []string{"true", "false"},
	},
	{
		Key:         "billing.blockNewBuildsWhenInsufficient",
		Label:       "余额不足阻止新构建",
		Description: "开启后，计费归属人余额不足时不再接受新的构建任务。已经开始的任务仍会完成结算。",
		Type:        "select",
		Public:      false,
		Default:     "false",
		Options:     []string{"true", "false"},
	},
	{
		Key:         "billing.blockDeployChangesWhenInsufficient",
		Label:       "余额不足阻止部署变更",
		Description: "开启后，计费归属人余额不足时会阻止新发布、扩容和新增数据卷等付费变更。",
		Type:        "select",
		Public:      false,
		Default:     "false",
		Options:     []string{"true", "false"},
	},
	{
		Key:         volume.ProjectManagedCapacityLimitConfigKey,
		Label:       "项目空间托管数据卷容量上限（GiB）",
		Description: "每个项目空间可预留的托管数据卷总容量；0 表示不限制。已存在的超额卷不会被截断，但新增和扩容会被拒绝。",
		Type:        "number",
		Public:      false,
		Default:     "0",
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
	if !bindJSON(ctx, &input) {
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
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
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
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}

	var input updateConfigsInput
	if !bindJSON(ctx, &input) {
		return
	}
	if err := validateAIConfigInputTypes(input.Values); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.config_invalid", err.Error())
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
				writeErrorCode(ctx, http.StatusBadRequest, "ai.config_invalid", "ai.web.proxy_pool is invalid")
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
		writeError(ctx, http.StatusBadRequest, err.Error())
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
	if err := h.validateAIConfigValues(values); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.config_invalid", err.Error())
		return
	}
	if apiKeyInput != "" {
		ref := h.secrets.StoreContext(ctx.Request.Context(), apiKeyInput, user.ID, "ai_provider:api_key")
		if ref == "" {
			writeErrorCode(ctx, http.StatusInternalServerError, "ai.secret_store_failed", "AI Provider API key could not be stored")
			return
		}
		values["ai.provider.api_key"] = ref
	}
	if proxyPoolInput != "" {
		ref := h.secrets.StoreContext(ctx.Request.Context(), proxyPoolInput, user.ID, "ai_web:proxy_pool")
		if ref == "" {
			writeErrorCode(ctx, http.StatusInternalServerError, "ai.secret_store_failed", "AI web proxy pool could not be stored")
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
			writeErrorCode(ctx, http.StatusInternalServerError, "ai.secret_store_failed", "Agent observability credential could not be stored")
			return
		}
		values[key] = ref
	}
	aiSecurityChanged := containsAIConfig(values)

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
		writeErrorCode(ctx, http.StatusInternalServerError, "config.update_failed", "configuration update failed")
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
