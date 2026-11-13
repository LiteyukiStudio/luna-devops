package notificationapi

import (
	"context"
	"encoding/json"
	"errors"
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
	Scope       string            `json:"scope"`
	ProjectIDs  []string          `json:"projectIds"`
	PresetID    string            `json:"presetId"`
	AdapterKind string            `json:"adapterKind"`
	Config      json.RawMessage   `json:"config"`
	Secrets     map[string]string `json:"secrets"`
	Enabled     *bool             `json:"enabled"`
}

type notificationChannelTestInput struct {
	TestRecipient string `json:"testRecipient"`
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

type NotificationRuleInput struct {
	Name                  string          `json:"name"`
	EventTypes            []string        `json:"eventTypes"`
	Filter                json.RawMessage `json:"filter"`
	ChannelIDs            []string        `json:"channelIds"`
	ChannelTargets        json.RawMessage `json:"channelTargets"`
	TemplateID            string          `json:"templateId"`
	Locale                string          `json:"locale"`
	TimeZone              string          `json:"timeZone"`
	RemindIntervalMinutes *int            `json:"remindIntervalMinutes"`
	MaxReminders          *int            `json:"maxReminders"`
	NotifyOnRecovery      *bool           `json:"notifyOnRecovery"`
	Enabled               *bool           `json:"enabled"`
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
	CanManage bool            `json:"canManage"`
}

func (h *Handler) ListNotificationPresets(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	ctx.JSON(http.StatusOK, notification.WebhookPresets())
}

func (h *Handler) ListNotificationChannels(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	pagination := paginationFromQuery(ctx)
	var total int64
	query := h.dbFor(ctx).Model(&model.NotificationChannel{}).Where("owner_user_id = ?", "")
	if scopeFilter := strings.TrimSpace(ctx.Query("scope")); scopeFilter != "" {
		query = query.Where("scope = ?", notification.NormalizeChannelScope(scopeFilter))
	}
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
	if err := h.populateLatestNotificationDeliveries(channels, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.populateChannelProjectIDs(channels, ctx.Request.Context())
	ctx.JSON(http.StatusOK, paginatedResponse(notificationChannelResponsesWithManage(channels, true, true), total, pagination))
}

func (h *Handler) CreateNotificationChannel(ctx *gin.Context) {
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
	channel, ok := h.notificationChannelFromInput(ctx, user, input, model.NotificationChannel{ID: id.New("nch"), Scope: "global", OwnerUserID: ""})
	if !ok {
		return
	}
	channel.Scope = "global"
	if err := h.dbFor(ctx).Create(&channel).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.channel.create", channel.ID, true, "", ctx.Request.Context())
	resp := notificationChannelResponseFor(channel)
	resp.CanManage = true
	ctx.JSON(http.StatusCreated, resp)
}

func (h *Handler) UpdateNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var existing model.NotificationChannel
	if err := h.dbFor(ctx).First(&existing, "id = ? and owner_user_id = ?", ctx.Param("channelId"), "").Error; err != nil {
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
	if err := h.dbFor(ctx).Save(&channel).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.channel.update", channel.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, notificationChannelResponseFor(channel))
}

func (h *Handler) DeleteNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	if err := h.dbFor(ctx).Delete(&model.NotificationChannel{}, "id = ? and owner_user_id = ?", ctx.Param("channelId"), "").Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.channel.delete", ctx.Param("channelId"), true, "", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) TestNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var channel model.NotificationChannel
	if err := h.dbFor(ctx).First(&channel, "id = ? and owner_user_id = ?", ctx.Param("channelId"), "").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	adapter, err := notification.DefaultRegistry().Adapter(channel.AdapterKind)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	testCtx := notification.WithEgressPolicy(ctx.Request.Context(), notification.EgressPolicyForScope(channel.Scope))
	if recipient := h.resolveTestRecipient(ctx, user); recipient != "" {
		testCtx = notification.WithTestRecipient(testCtx, recipient)
	}
	if err := adapter.Test(testCtx, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), h.secrets()); err != nil {
		h.auditWithContext(user.ID, "notification.channel.test", channel.ID, false, err.Error(), ctx.Request.Context())
		writeError(ctx, http.StatusBadGateway, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.channel.test", channel.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type latestNotificationDelivery struct {
	ChannelID    string
	Status       string
	ErrorMessage string
	FinishedAt   *time.Time
}

func (h *Handler) populateLatestNotificationDeliveries(channels []model.NotificationChannel, ctx context.Context) error {
	if len(channels) == 0 {
		return nil
	}
	channelIDs := make([]string, 0, len(channels))
	for _, channel := range channels {
		channelIDs = append(channelIDs, channel.ID)
	}
	var latest []latestNotificationDelivery
	if err := h.dbWithContext(ctx).Raw(`
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

func (h *Handler) CreateNotificationChannelFromPreset(ctx *gin.Context) {
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
		secretRefs[field] = h.secrets().StoreContext(ctx.Request.Context(), value, user.ID, "notification_channel:preset:"+preset.ID+":"+field)
	}
	for _, field := range preset.OptionalSecretFields {
		if value := strings.TrimSpace(input.Secrets[field]); value != "" {
			secretRefs[field] = h.secrets().StoreContext(ctx.Request.Context(), value, user.ID, "notification_channel:preset:"+preset.ID+":"+field)
		}
	}
	secretRefsRaw := mustJSON(secretRefs)
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	channel := model.NotificationChannel{
		ID:             id.New("nch"),
		Scope:          "global",
		PresetID:       preset.ID,
		OwnerUserID:    "",
		Name:           firstNonEmpty(input.Name, preset.Name),
		AdapterKind:    preset.AdapterKind,
		ConfigJSON:     string(configRaw),
		SecretRefsJSON: secretRefsRaw,
		Enabled:        enabled,
		CreatedBy:      user.ID,
	}
	if err := validateNotificationChannel(ctx.Request.Context(), channel, h.secrets()); err != nil {
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
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&channel).Error; err != nil {
			return err
		}
		return tx.Create(&template).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.preset.create_channel", channel.ID, true, preset.ID, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, gin.H{"channel": notificationChannelResponseFor(channel), "template": template})
}

func (h *Handler) ListNotificationTemplates(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	pagination := paginationFromQuery(ctx)
	var total int64
	query := h.dbFor(ctx).Model(&model.NotificationTemplate{})
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

func (h *Handler) CreateNotificationTemplate(ctx *gin.Context) {
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
	if err := h.dbFor(ctx).Create(&template).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.template.create", template.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusCreated, template)
}

func (h *Handler) UpdateNotificationTemplate(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var existing model.NotificationTemplate
	if err := h.dbFor(ctx).First(&existing, "id = ?", ctx.Param("templateId")).Error; err != nil {
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
	if err := h.dbFor(ctx).Save(&template).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.template.update", template.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, template)
}

func (h *Handler) DeleteNotificationTemplate(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	if err := h.dbFor(ctx).Delete(&model.NotificationTemplate{}, "id = ?", ctx.Param("templateId")).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.template.delete", ctx.Param("templateId"), true, "", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) ListNotificationRules(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	pagination := paginationFromQuery(ctx)
	var total int64
	query := h.dbFor(ctx).Model(&model.NotificationRule{}).Where("project_id = ?", "")
	if search := strings.TrimSpace(ctx.Query("search")); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}
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

func (h *Handler) CreateNotificationRule(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var input NotificationRuleInput
	if !bindJSON(ctx, &input) {
		return
	}
	rule, ok := h.NotificationRuleFromInput(ctx, input, model.NotificationRule{ID: id.New("nrl"), CreatedBy: user.ID})
	if !ok {
		return
	}
	if err := createNotificationRule(h.dbFor(ctx), &rule); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.rule.create", rule.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusCreated, rule)
}

func (h *Handler) UpdateNotificationRule(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var existing model.NotificationRule
	if err := h.dbFor(ctx).First(&existing, "id = ?", ctx.Param("ruleId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification rule not found")
		return
	}
	var input NotificationRuleInput
	if !bindJSON(ctx, &input) {
		return
	}
	rule, ok := h.NotificationRuleFromInput(ctx, input, existing)
	if !ok {
		return
	}
	if err := h.dbFor(ctx).Save(&rule).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.rule.update", rule.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, rule)
}

func (h *Handler) DeleteNotificationRule(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	if err := h.dbFor(ctx).Delete(&model.NotificationRule{}, "id = ?", ctx.Param("ruleId")).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.rule.delete", ctx.Param("ruleId"), true, "", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) ListNotificationDeliveries(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	pagination := paginationFromQuery(ctx)
	var total int64
	query := h.dbFor(ctx).Model(&model.NotificationDelivery{}).Where("recipient_user_id = ?", "")
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if eventType := strings.TrimSpace(ctx.Query("eventType")); eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	if channelID := strings.TrimSpace(ctx.Query("channelId")); channelID != "" {
		query = query.Where("channel_id = ?", channelID)
	}
	if ruleID := strings.TrimSpace(ctx.Query("ruleId")); ruleID != "" {
		query = query.Where("rule_id = ?", ruleID)
	}
	if eventID := strings.TrimSpace(ctx.Query("eventId")); eventID != "" {
		query = query.Where("event_id = ?", eventID)
	}
	if projectID := strings.TrimSpace(ctx.Query("projectId")); projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var deliveries []model.NotificationDelivery
	if err := query.Order(orderByClause(pagination, map[string]string{
		"createdAt":      "created_at",
		"queuedAt":       "queued_at",
		"finishedAt":     "finished_at",
		"durationMillis": "duration_millis",
		"status":         "status",
		"eventType":      "event_type",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&deliveries).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.enrichNotificationDeliveries(deliveries, true, ctx.Request.Context())
	ctx.JSON(http.StatusOK, paginatedResponse(deliveries, total, pagination))
}

func (h *Handler) RetryNotificationDelivery(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	deliveryID := strings.TrimSpace(ctx.Param("deliveryId"))
	var existing model.NotificationDelivery
	if err := h.dbFor(ctx).First(&existing, "id = ? and recipient_user_id = ?", deliveryID, "").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification delivery not found")
		return
	}
	svc := notification.Service{DB: h.dbFor(ctx), Enqueuer: h.host.DeliveryEnqueuer()}
	retried, err := svc.RetryDelivery(ctx.Request.Context(), existing.ID)
	if errors.Is(err, notification.ErrDeliveryActive) {
		writeErrorCode(ctx, http.StatusConflict, "notification.delivery_active", err.Error())
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	items := []model.NotificationDelivery{retried}
	h.enrichNotificationDeliveries(items, true, ctx.Request.Context())
	h.auditWithContext(user.ID, "notification.delivery.retry", retried.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, items[0])
}

func (h *Handler) notificationChannelFromInput(ctx *gin.Context, user model.User, input notificationChannelInput, channel model.NotificationChannel) (model.NotificationChannel, bool) {
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
	configRaw, secretRefs = h.captureNotificationSecrets(ctx.Request.Context(), user, channel.ID, channel.AdapterKind, configRaw, input.Secrets, secretRefs)
	channel.ConfigJSON = string(configRaw)
	channel.SecretRefsJSON = mustJSON(secretRefs)
	if input.Enabled != nil {
		channel.Enabled = *input.Enabled
	} else if channel.ID != "" && channel.CreatedAt.IsZero() {
		channel.Enabled = true
	}
	channel.CreatedBy = firstNonEmpty(channel.CreatedBy, user.ID)
	if err := validateNotificationChannel(ctx.Request.Context(), channel, h.secrets()); err != nil {
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

func (h *Handler) NotificationRuleFromInput(ctx *gin.Context, input NotificationRuleInput, rule model.NotificationRule) (model.NotificationRule, bool) {
	filterInput := input.Filter
	if strings.TrimSpace(rule.ProjectID) != "" {
		filterInput = normalizeProjectRuleFilterJSON(filterInput, rule.ProjectID)
	}
	filter, err := notification.DecodeRuleFilter(filterInput)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_filter_invalid", err.Error())
		return model.NotificationRule{}, false
	}
	if strings.TrimSpace(rule.ProjectID) != "" {
		filter.Scope = notification.RuleScopeProjects
		filter.ProjectIDs = []string{strings.TrimSpace(rule.ProjectID)}
	}
	channelTargets, err := notification.DecodeChannelTargets(input.ChannelTargets)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_targets_invalid", err.Error())
		return model.NotificationRule{}, false
	}
	eventTypes := normalizedNotificationRuleValues(input.EventTypes)
	channelIDs := normalizedNotificationRuleValues(input.ChannelIDs)
	rule.Name = strings.TrimSpace(input.Name)
	rule.EventTypesJSON = notification.EncodeStringList(eventTypes)
	rule.FilterJSON, err = notification.EncodeRuleFilter(filter)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_filter_invalid", err.Error())
		return model.NotificationRule{}, false
	}
	rule.ChannelIDsJSON = notification.EncodeStringList(channelIDs)
	rule.ChannelTargetsJSON, err = notification.EncodeChannelTargets(channelTargets)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_targets_invalid", err.Error())
		return model.NotificationRule{}, false
	}
	rule.TemplateID = strings.TrimSpace(input.TemplateID)
	rule.Locale = strings.TrimSpace(input.Locale)
	timeZoneInput := strings.TrimSpace(input.TimeZone)
	if timeZoneInput == "" {
		timeZoneInput = firstNonEmpty(rule.TimeZone, notification.DefaultTimeZone)
	}
	validatedTimeZone, err := notification.ValidateTimeZone(timeZoneInput)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_timezone_invalid", err.Error())
		return model.NotificationRule{}, false
	}
	rule.TimeZone = validatedTimeZone
	if input.RemindIntervalMinutes != nil {
		if *input.RemindIntervalMinutes < 0 || *input.RemindIntervalMinutes > 10080 {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_throttle_invalid", "remindIntervalMinutes must be between 0 and 10080")
			return model.NotificationRule{}, false
		}
		rule.RemindIntervalMinutes = *input.RemindIntervalMinutes
	} else if rule.CreatedAt.IsZero() && rule.RemindIntervalMinutes == 0 {
		rule.RemindIntervalMinutes = 60
	}
	if input.MaxReminders != nil {
		if *input.MaxReminders < 0 || *input.MaxReminders > 100 {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_throttle_invalid", "maxReminders must be between 0 and 100")
			return model.NotificationRule{}, false
		}
		rule.MaxReminders = *input.MaxReminders
	} else if rule.CreatedAt.IsZero() && rule.MaxReminders == 0 {
		rule.MaxReminders = 3
	}
	if input.NotifyOnRecovery != nil {
		rule.NotifyOnRecovery = *input.NotifyOnRecovery
	} else if rule.CreatedAt.IsZero() {
		rule.NotifyOnRecovery = true
	}
	if input.Enabled != nil {
		rule.Enabled = *input.Enabled
	} else if rule.ID != "" && rule.CreatedAt.IsZero() {
		rule.Enabled = true
	}
	if rule.Name == "" || len(eventTypes) == 0 || len(channelIDs) == 0 {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_required", "notification rule name, event types and channels are required")
		return model.NotificationRule{}, false
	}
	if filter.Scope == notification.RuleScopeProjects {
		var projectCount int64
		if err := h.dbFor(ctx).Model(&model.Project{}).Where("id in ?", filter.ProjectIDs).Count(&projectCount).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return model.NotificationRule{}, false
		}
		if projectCount != int64(len(filter.ProjectIDs)) {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_project_not_found", "notification rule project not found")
			return model.NotificationRule{}, false
		}
	}
	var channels []model.NotificationChannel
	if err := h.dbFor(ctx).Where("id in ? and owner_user_id = ?", channelIDs, "").Find(&channels).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.NotificationRule{}, false
	}
	if len(channels) != len(channelIDs) {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.channel_not_found", "notification channel not found")
		return model.NotificationRule{}, false
	}
	boundProjectChannels := map[string]bool{}
	if strings.TrimSpace(rule.ProjectID) != "" {
		var bindings []model.ScopedResourceProjectBinding
		if err := h.dbFor(ctx).Where(
			"resource_type = ? and project_id = ? and resource_id in ?",
			notification.ScopedResourceChannelType,
			strings.TrimSpace(rule.ProjectID),
			channelIDs,
		).Find(&bindings).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return model.NotificationRule{}, false
		}
		for _, binding := range bindings {
			boundProjectChannels[binding.ResourceID] = true
		}
	}
	for _, channel := range channels {
		scope := notification.NormalizeChannelScope(channel.Scope)
		if strings.TrimSpace(rule.ProjectID) == "" {
			if scope != "global" {
				writeErrorCode(ctx, http.StatusBadRequest, "notification.channel_not_found", "global notification rules can only use global channels")
				return model.NotificationRule{}, false
			}
		} else if scope != "global" && !(scope == "project" && (boundProjectChannels[channel.ID] || strings.TrimSpace(channel.ProjectID) == strings.TrimSpace(rule.ProjectID))) {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.channel_not_found", "notification channel is not bound to this project space")
			return model.NotificationRule{}, false
		}
		if channel.AdapterKind == notification.AdapterKindSMTP {
			override := channelTargets[channel.ID]
			if _, recipients, applyErr := notification.ApplySMTPTargetOverride(json.RawMessage(channel.ConfigJSON), override); applyErr != nil || len(recipients) == 0 {
				writeErrorCode(ctx, http.StatusBadRequest, "notification.smtp_recipient_required", "smtp channel requires at least one recipient on channel or rule")
				return model.NotificationRule{}, false
			}
		}
	}
	return rule, true
}

func createNotificationRule(db *gorm.DB, rule *model.NotificationRule) error {
	wantMaxReminders := rule.MaxReminders
	wantNotifyOnRecovery := rule.NotifyOnRecovery
	wantEnabled := rule.Enabled
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rule).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		if rule.MaxReminders != wantMaxReminders {
			updates["max_reminders"] = wantMaxReminders
			rule.MaxReminders = wantMaxReminders
		}
		if rule.NotifyOnRecovery != wantNotifyOnRecovery {
			updates["notify_on_recovery"] = wantNotifyOnRecovery
			rule.NotifyOnRecovery = wantNotifyOnRecovery
		}
		if rule.Enabled != wantEnabled {
			updates["enabled"] = wantEnabled
			rule.Enabled = wantEnabled
		}
		if len(updates) == 0 {
			return nil
		}
		return tx.Model(&model.NotificationRule{}).Where("id = ?", rule.ID).Updates(updates).Error
	})
}

func normalizedNotificationRuleValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func validateNotificationChannel(ctx context.Context, channel model.NotificationChannel, resolver notification.SecretResolver) error {
	adapter, err := notification.DefaultRegistry().Adapter(channel.AdapterKind)
	if err != nil {
		return err
	}
	scope := channel.Scope
	if strings.TrimSpace(channel.OwnerUserID) != "" {
		scope = "user"
	}
	validateCtx := notification.WithEgressPolicy(ctx, notification.EgressPolicyForScope(scope))
	return adapter.Validate(validateCtx, json.RawMessage(channel.ConfigJSON), resolver)
}

func (h *Handler) captureNotificationSecrets(ctx context.Context, user model.User, channelID string, adapterKind string, configRaw json.RawMessage, secretValues map[string]string, existing map[string]string) (json.RawMessage, map[string]string) {
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
		existing[key] = h.secrets().StoreContext(ctx, value, user.ID, "notification_channel:"+channelID+":"+key)
	}
	if adapterKind == notification.AdapterKindSMTP {
		ref := strings.TrimSpace(existing["password"])
		if ref == "" {
			for key, candidate := range existing {
				if strings.EqualFold(key, "password") && strings.TrimSpace(candidate) != "" {
					ref = strings.TrimSpace(candidate)
					break
				}
			}
		}
		if ref != "" {
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
	return notificationChannelResponsesWithManage(channels, false, true)
}

func notificationChannelResponsesWithManage(channels []model.NotificationChannel, canManage bool, includeRecipients bool) []notificationChannelResponse {
	responses := make([]notificationChannelResponse, 0, len(channels))
	for _, channel := range channels {
		resp := notificationChannelResponseForWithPrivacy(channel, includeRecipients)
		resp.CanManage = canManage
		responses = append(responses, resp)
	}
	return responses
}

func notificationChannelResponseFor(channel model.NotificationChannel) notificationChannelResponse {
	return notificationChannelResponseForWithPrivacy(channel, true)
}

func notificationChannelResponseForWithPrivacy(channel model.NotificationChannel, includeSensitiveConfig bool) notificationChannelResponse {
	config := sanitizeNotificationConfigWithPrivacy(channel.ConfigJSON, includeSensitiveConfig)
	sanitizedConfigJSON := mustJSON(config)
	channel.ConfigJSON = sanitizedConfigJSON
	return notificationChannelResponse{
		NotificationChannel: channel,
		Config:              config,
		SecretSet:           secretSetMap(channel.SecretRefsJSON),
	}
}

func sanitizeNotificationConfig(raw string) any {
	return sanitizeNotificationConfigWithPrivacy(raw, true)
}

func sanitizeNotificationConfigWithPrivacy(raw string, includeSensitiveConfig bool) any {
	value, ok := decodeJSONValue(raw).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	for key := range value {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "password") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "token") {
			delete(value, key)
			continue
		}
		if !includeSensitiveConfig {
			switch normalized {
			case "to", "cc", "bcc", "username", "url", "headers", "testjsonbodytemplate":
				delete(value, key)
			}
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
