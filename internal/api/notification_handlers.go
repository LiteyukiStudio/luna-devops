package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type notificationChannelInput struct {
	Name        string            `json:"name"`
	AdapterKind string            `json:"adapterKind"`
	Config      json.RawMessage   `json:"config"`
	Secrets     map[string]string `json:"secrets"`
	Enabled     *bool             `json:"enabled"`
}

type notificationTemplateInput struct {
	Name             string `json:"name"`
	EventType        string `json:"eventType"`
	AdapterKind      string `json:"adapterKind"`
	Locale           string `json:"locale"`
	SubjectTemplate  string `json:"subjectTemplate"`
	BodyTemplate     string `json:"bodyTemplate"`
	JSONBodyTemplate string `json:"jsonBodyTemplate"`
	Enabled          *bool  `json:"enabled"`
}

type notificationRuleInput struct {
	Name       string                  `json:"name"`
	EventTypes []string                `json:"eventTypes"`
	Filter     notification.RuleFilter `json:"filter"`
	ChannelIDs []string                `json:"channelIds"`
	TemplateID string                  `json:"templateId"`
	Locale     string                  `json:"locale"`
	Enabled    *bool                   `json:"enabled"`
}

type notificationPresetChannelInput struct {
	Name    string            `json:"name"`
	Secrets map[string]string `json:"secrets"`
	Enabled *bool             `json:"enabled"`
}

type notificationChannelResponse struct {
	model.NotificationChannel
	Config    any             `json:"config"`
	SecretSet map[string]bool `json:"secretSet"`
}

func (h *Handlers) ListNotificationPresets(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	ctx.JSON(http.StatusOK, notification.WebhookPresets())
}

func (h *Handlers) ListNotificationChannels(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	pagination := paginationFromQuery(ctx)
	var total int64
	query := h.db.Model(&model.NotificationChannel{})
	if search := strings.TrimSpace(ctx.Query("search")); search != "" {
		query = query.Where("name ILIKE ? or adapter_kind ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var channels []model.NotificationChannel
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"adapter":   "adapter_kind",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&channels).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.populateLatestNotificationDeliveries(channels); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(notificationChannelResponses(channels), total, pagination))
}

func (h *Handlers) CreateNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var input notificationChannelInput
	if !bindJSON(ctx, &input) {
		return
	}
	channel, ok := h.notificationChannelFromInput(ctx, user, input, model.NotificationChannel{ID: id.New("nch")})
	if !ok {
		return
	}
	if err := h.db.Create(&channel).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.channel.create", channel.ID, true, "")
	ctx.JSON(http.StatusCreated, notificationChannelResponseFor(channel))
}

func (h *Handlers) UpdateNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var existing model.NotificationChannel
	if err := h.db.First(&existing, "id = ?", ctx.Param("channelId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	var input notificationChannelInput
	if !bindJSON(ctx, &input) {
		return
	}
	channel, ok := h.notificationChannelFromInput(ctx, user, input, existing)
	if !ok {
		return
	}
	if err := h.db.Save(&channel).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.channel.update", channel.ID, true, "")
	ctx.JSON(http.StatusOK, notificationChannelResponseFor(channel))
}

func (h *Handlers) DeleteNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	if err := h.db.Delete(&model.NotificationChannel{}, "id = ?", ctx.Param("channelId")).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.channel.delete", ctx.Param("channelId"), true, "")
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) TestNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var channel model.NotificationChannel
	if err := h.db.First(&channel, "id = ?", ctx.Param("channelId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	adapter, err := notification.DefaultRegistry().Adapter(channel.AdapterKind)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := adapter.Test(ctx.Request.Context(), json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), h.secrets); err != nil {
		h.audit(user.ID, "notification.channel.test", channel.ID, false, err.Error())
		writeError(ctx, http.StatusBadGateway, err.Error())
		return
	}
	h.audit(user.ID, "notification.channel.test", channel.ID, true, "")
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type latestNotificationDelivery struct {
	ChannelID    string
	Status       string
	ErrorMessage string
	FinishedAt   *time.Time
}

func (h *Handlers) populateLatestNotificationDeliveries(channels []model.NotificationChannel) error {
	if len(channels) == 0 {
		return nil
	}
	channelIDs := make([]string, 0, len(channels))
	for _, channel := range channels {
		channelIDs = append(channelIDs, channel.ID)
	}
	var latest []latestNotificationDelivery
	if err := h.db.Raw(`
		SELECT DISTINCT ON (channel_id)
			channel_id,
			status,
			error_message,
			finished_at
		FROM notification_deliveries
		WHERE channel_id IN ?
		ORDER BY channel_id, created_at DESC, id DESC
	`, channelIDs).Scan(&latest).Error; err != nil {
		return err
	}
	byChannelID := make(map[string]latestNotificationDelivery, len(latest))
	for _, delivery := range latest {
		byChannelID[delivery.ChannelID] = delivery
	}
	for index := range channels {
		delivery, ok := byChannelID[channels[index].ID]
		if !ok {
			continue
		}
		channels[index].LastDeliveryStatus = delivery.Status
		channels[index].LastDeliveryError = delivery.ErrorMessage
		channels[index].LastDeliveredAt = delivery.FinishedAt
	}
	return nil
}

func (h *Handlers) CreateNotificationChannelFromPreset(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var preset notification.WebhookPreset
	found := false
	for _, item := range notification.WebhookPresets() {
		if item.ID == ctx.Param("presetId") {
			preset = item
			found = true
			break
		}
	}
	if !found {
		writeError(ctx, http.StatusNotFound, "notification preset not found")
		return
	}
	var input notificationPresetChannelInput
	if !bindJSON(ctx, &input) {
		return
	}
	configRaw := json.RawMessage(preset.ConfigTemplate)
	secretRefs := map[string]string{}
	for _, field := range preset.SecretFields {
		value := strings.TrimSpace(input.Secrets[field])
		if value == "" {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.secret_required", "notification preset secret is required")
			return
		}
		secretRefs[field] = h.secrets.Store(value, user.ID, "notification_channel:preset:"+preset.ID+":"+field)
	}
	secretRefsRaw := mustJSON(secretRefs)
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	channel := model.NotificationChannel{
		ID:             id.New("nch"),
		Name:           firstNonEmpty(input.Name, preset.Name),
		AdapterKind:    preset.AdapterKind,
		ConfigJSON:     string(configRaw),
		SecretRefsJSON: secretRefsRaw,
		Enabled:        enabled,
		CreatedBy:      user.ID,
	}
	if err := validateNotificationChannel(channel, h.secrets); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	template := model.NotificationTemplate{
		ID:               id.New("ntp"),
		Name:             preset.Name + " default",
		EventType:        "build.failed",
		AdapterKind:      preset.AdapterKind,
		JSONBodyTemplate: preset.JSONBodyTemplate,
		Enabled:          true,
		CreatedBy:        user.ID,
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&channel).Error; err != nil {
			return err
		}
		return tx.Create(&template).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.preset.create_channel", channel.ID, true, preset.ID)
	ctx.JSON(http.StatusCreated, gin.H{"channel": notificationChannelResponseFor(channel), "template": template})
}

func (h *Handlers) ListNotificationTemplates(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	pagination := paginationFromQuery(ctx)
	var total int64
	query := h.db.Model(&model.NotificationTemplate{})
	if eventType := strings.TrimSpace(ctx.Query("eventType")); eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	if adapterKind := strings.TrimSpace(ctx.Query("adapterKind")); adapterKind != "" {
		query = query.Where("adapter_kind = ?", adapterKind)
	}
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var templates []model.NotificationTemplate
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"eventType": "event_type",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&templates).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(templates, total, pagination))
}

func (h *Handlers) CreateNotificationTemplate(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var input notificationTemplateInput
	if !bindJSON(ctx, &input) {
		return
	}
	template, ok := notificationTemplateFromInput(ctx, input, model.NotificationTemplate{ID: id.New("ntp"), CreatedBy: user.ID})
	if !ok {
		return
	}
	if err := h.db.Create(&template).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.template.create", template.ID, true, "")
	ctx.JSON(http.StatusCreated, template)
}

func (h *Handlers) UpdateNotificationTemplate(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var existing model.NotificationTemplate
	if err := h.db.First(&existing, "id = ?", ctx.Param("templateId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification template not found")
		return
	}
	var input notificationTemplateInput
	if !bindJSON(ctx, &input) {
		return
	}
	template, ok := notificationTemplateFromInput(ctx, input, existing)
	if !ok {
		return
	}
	if err := h.db.Save(&template).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.template.update", template.ID, true, "")
	ctx.JSON(http.StatusOK, template)
}

func (h *Handlers) DeleteNotificationTemplate(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	if err := h.db.Delete(&model.NotificationTemplate{}, "id = ?", ctx.Param("templateId")).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.template.delete", ctx.Param("templateId"), true, "")
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListNotificationRules(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	pagination := paginationFromQuery(ctx)
	var total int64
	query := h.db.Model(&model.NotificationRule{})
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var rules []model.NotificationRule
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"createdAt": "created_at",
		"updatedAt": "updated_at",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&rules).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(rules, total, pagination))
}

func (h *Handlers) CreateNotificationRule(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var input notificationRuleInput
	if !bindJSON(ctx, &input) {
		return
	}
	rule, ok := h.notificationRuleFromInput(ctx, input, model.NotificationRule{ID: id.New("nrl"), CreatedBy: user.ID})
	if !ok {
		return
	}
	if err := h.db.Create(&rule).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.rule.create", rule.ID, true, "")
	ctx.JSON(http.StatusCreated, rule)
}

func (h *Handlers) UpdateNotificationRule(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var existing model.NotificationRule
	if err := h.db.First(&existing, "id = ?", ctx.Param("ruleId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification rule not found")
		return
	}
	var input notificationRuleInput
	if !bindJSON(ctx, &input) {
		return
	}
	rule, ok := h.notificationRuleFromInput(ctx, input, existing)
	if !ok {
		return
	}
	if err := h.db.Save(&rule).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.rule.update", rule.ID, true, "")
	ctx.JSON(http.StatusOK, rule)
}

func (h *Handlers) DeleteNotificationRule(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	if err := h.db.Delete(&model.NotificationRule{}, "id = ?", ctx.Param("ruleId")).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "notification.rule.delete", ctx.Param("ruleId"), true, "")
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListNotificationDeliveries(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	pagination := paginationFromQuery(ctx)
	var total int64
	query := h.db.Model(&model.NotificationDelivery{})
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if eventType := strings.TrimSpace(ctx.Query("eventType")); eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var deliveries []model.NotificationDelivery
	if err := query.Order(orderByClause(pagination, map[string]string{
		"createdAt": "created_at",
		"status":    "status",
		"eventType": "event_type",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&deliveries).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(deliveries, total, pagination))
}

func (h *Handlers) notificationChannelFromInput(ctx *gin.Context, user model.User, input notificationChannelInput, channel model.NotificationChannel) (model.NotificationChannel, bool) {
	channel.Name = strings.TrimSpace(input.Name)
	channel.AdapterKind = strings.TrimSpace(input.AdapterKind)
	if channel.Name == "" || channel.AdapterKind == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.channel_required", "notification channel name and adapter kind are required")
		return model.NotificationChannel{}, false
	}
	configRaw := normalizedJSON(input.Config, channel.ConfigJSON)
	secretRefs := decodeStringMap(channel.SecretRefsJSON)
	if input.Secrets == nil {
		input.Secrets = map[string]string{}
	}
	configRaw, secretRefs = h.captureNotificationSecrets(user, channel.ID, channel.AdapterKind, configRaw, input.Secrets, secretRefs)
	channel.ConfigJSON = string(configRaw)
	channel.SecretRefsJSON = mustJSON(secretRefs)
	if input.Enabled != nil {
		channel.Enabled = *input.Enabled
	} else if channel.ID != "" && channel.CreatedAt.IsZero() {
		channel.Enabled = true
	}
	channel.CreatedBy = firstNonEmpty(channel.CreatedBy, user.ID)
	if err := validateNotificationChannel(channel, h.secrets); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return model.NotificationChannel{}, false
	}
	return channel, true
}

func notificationTemplateFromInput(ctx *gin.Context, input notificationTemplateInput, template model.NotificationTemplate) (model.NotificationTemplate, bool) {
	template.Name = strings.TrimSpace(input.Name)
	template.EventType = strings.TrimSpace(input.EventType)
	template.AdapterKind = strings.TrimSpace(input.AdapterKind)
	template.Locale = strings.TrimSpace(input.Locale)
	template.SubjectTemplate = input.SubjectTemplate
	template.BodyTemplate = input.BodyTemplate
	template.JSONBodyTemplate = input.JSONBodyTemplate
	if input.Enabled != nil {
		template.Enabled = *input.Enabled
	} else if template.ID != "" && template.CreatedAt.IsZero() {
		template.Enabled = true
	}
	if template.Name == "" || template.EventType == "" || template.AdapterKind == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.template_required", "notification template name, event type and adapter kind are required")
		return model.NotificationTemplate{}, false
	}
	return template, true
}

func (h *Handlers) notificationRuleFromInput(ctx *gin.Context, input notificationRuleInput, rule model.NotificationRule) (model.NotificationRule, bool) {
	rule.Name = strings.TrimSpace(input.Name)
	rule.EventTypesJSON = notification.EncodeStringList(input.EventTypes)
	rule.FilterJSON = notification.EncodeRuleFilter(input.Filter)
	rule.ChannelIDsJSON = notification.EncodeStringList(input.ChannelIDs)
	rule.TemplateID = strings.TrimSpace(input.TemplateID)
	rule.Locale = strings.TrimSpace(input.Locale)
	if input.Enabled != nil {
		rule.Enabled = *input.Enabled
	} else if rule.ID != "" && rule.CreatedAt.IsZero() {
		rule.Enabled = true
	}
	if rule.Name == "" || len(input.ChannelIDs) == 0 {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_required", "notification rule name and channels are required")
		return model.NotificationRule{}, false
	}
	var count int64
	if err := h.db.Model(&model.NotificationChannel{}).Where("id in ?", input.ChannelIDs).Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.NotificationRule{}, false
	}
	if count != int64(len(input.ChannelIDs)) {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.channel_not_found", "notification channel not found")
		return model.NotificationRule{}, false
	}
	return rule, true
}

func validateNotificationChannel(channel model.NotificationChannel, resolver notification.SecretResolver) error {
	adapter, err := notification.DefaultRegistry().Adapter(channel.AdapterKind)
	if err != nil {
		return err
	}
	return adapter.Validate(context.Background(), json.RawMessage(channel.ConfigJSON), resolver)
}

func (h *Handlers) captureNotificationSecrets(user model.User, channelID string, adapterKind string, configRaw json.RawMessage, secretValues map[string]string, existing map[string]string) (json.RawMessage, map[string]string) {
	config := map[string]any{}
	_ = json.Unmarshal(configRaw, &config)
	if password, ok := config["password"].(string); ok && strings.TrimSpace(password) != "" {
		secretValues["password"] = password
		delete(config, "password")
	}
	for key, value := range secretValues {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		existing[key] = h.secrets.Store(value, user.ID, "notification_channel:"+channelID+":"+key)
	}
	if adapterKind == notification.AdapterKindSMTP {
		if ref := strings.TrimSpace(existing["password"]); ref != "" {
			config["passwordRef"] = ref
		}
	}
	data, err := json.Marshal(config)
	if err != nil || len(config) == 0 {
		return configRaw, existing
	}
	return data, existing
}

func notificationChannelResponses(channels []model.NotificationChannel) []notificationChannelResponse {
	responses := make([]notificationChannelResponse, 0, len(channels))
	for _, channel := range channels {
		responses = append(responses, notificationChannelResponseFor(channel))
	}
	return responses
}

func notificationChannelResponseFor(channel model.NotificationChannel) notificationChannelResponse {
	config := sanitizeNotificationConfig(channel.ConfigJSON)
	sanitizedConfigJSON := mustJSON(config)
	channel.ConfigJSON = sanitizedConfigJSON
	return notificationChannelResponse{
		NotificationChannel: channel,
		Config:              config,
		SecretSet:           secretSetMap(channel.SecretRefsJSON),
	}
}

func sanitizeNotificationConfig(raw string) any {
	value, ok := decodeJSONValue(raw).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	for key := range value {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "password") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "token") {
			delete(value, key)
		}
	}
	return value
}

func normalizedJSON(input json.RawMessage, fallback string) json.RawMessage {
	if len(input) == 0 || strings.TrimSpace(string(input)) == "" {
		if strings.TrimSpace(fallback) != "" {
			return json.RawMessage(fallback)
		}
		return json.RawMessage(`{}`)
	}
	var normalized any
	if err := json.Unmarshal(input, &normalized); err != nil {
		return json.RawMessage(`{}`)
	}
	data, _ := json.Marshal(normalized)
	return data
}

func decodeJSONValue(raw string) any {
	var value any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &value); err != nil {
		return map[string]any{}
	}
	return value
}

func decodeStringMap(raw string) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &out)
	return out
}

func secretSetMap(raw string) map[string]bool {
	refs := decodeStringMap(raw)
	out := map[string]bool{}
	for key, value := range refs {
		out[key] = strings.TrimSpace(value) != ""
	}
	return out
}

func mustJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
