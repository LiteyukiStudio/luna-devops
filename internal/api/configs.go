package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type configDefinition struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Public      bool   `json:"public"`
	Default     string `json:"default"`
}

var configDefinitions = []configDefinition{
	{
		Key:         "site.title",
		Label:       "网站标题",
		Description: "浏览器标题和控制台品牌名称。",
		Type:        "string",
		Public:      true,
		Default:     "Liteyuki DevOps",
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
}

type configCache struct {
	mu     sync.RWMutex
	values map[string]string
}

func newConfigCache(db *gorm.DB) *configCache {
	cache := &configCache{values: map[string]string{}}
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
	defer c.mu.RUnlock()

	result := map[string]string{}
	for _, key := range keys {
		if value, ok := c.values[key]; ok {
			result[key] = value
		}
	}
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
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}

	ctx.JSON(http.StatusOK, h.configs.get(knownConfigKeys()))
}

func (h *Handlers) ListConfigDefinitions(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}

	ctx.JSON(http.StatusOK, configDefinitions)
}

func (h *Handlers) UpdateConfigs(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}

	var input updateConfigsInput
	if !bindJSON(ctx, &input) {
		return
	}

	for key, rawValue := range input.Values {
		if !isKnownConfigKey(key) {
			writeError(ctx, http.StatusBadRequest, fmt.Sprintf("unknown config key: %s", key))
			return
		}
		value, err := configValueToString(rawValue)
		if err != nil {
			writeError(ctx, http.StatusBadRequest, err.Error())
			return
		}
		row := model.AppConfig{Key: key, Value: value, UpdatedAt: time.Now()}
		if err := h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).Create(&row).Error; err != nil {
			writeError(ctx, http.StatusBadRequest, err.Error())
			return
		}
	}
	h.configs.reload(h.db)

	ctx.JSON(http.StatusOK, h.configs.get(knownConfigKeys()))
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
	for _, definition := range configDefinitions {
		if definition.Key == key {
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
		return fmt.Sprintf("%v", typed), nil
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
