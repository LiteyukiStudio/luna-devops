package notificationapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	personalNotificationRequestMaxBytes = 64 << 10
	personalNotificationChannelLimit    = int64(10)
	personalNotificationNameMaxLength   = 160
	personalNotificationSecretMaxLength = 4096
	personalNotificationTestRateLimit   = 10
)

var (
	errPersonalNotificationChannelLimit   = errors.New("personal notification channel limit reached")
	errPersonalNotificationSecretsInvalid = errors.New("personal notification secrets are invalid")
)

type userNotificationPreferenceInput struct {
	EmailEnabled *bool     `json:"emailEnabled"`
	EventTypes   *[]string `json:"eventTypes"`
}

type userNotificationPreferenceResponse struct {
	EmailEnabled bool     `json:"emailEnabled"`
	EventTypes   []string `json:"eventTypes"`
}

type personalNotificationChannelCreateInput struct {
	Name     string            `json:"name"`
	PresetID string            `json:"presetId"`
	Secrets  map[string]string `json:"secrets"`
	Enabled  *bool             `json:"enabled"`
}

type personalNotificationChannelUpdateInput struct {
	Name    string            `json:"name"`
	Secrets map[string]string `json:"secrets"`
	Enabled *bool             `json:"enabled"`
}

func (h *Handler) GetMyNotificationPreferences(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	preference := notification.DefaultUserNotificationPreference(user.ID)
	if err := h.dbFor(ctx).First(&preference, "user_id = ?", user.ID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, myNotificationPreferenceResponse(preference))
}

func (h *Handler) UpdateMyNotificationPreferences(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input userNotificationPreferenceInput
	if !bindPersonalNotificationJSON(ctx, &input) {
		return
	}
	if input.EmailEnabled == nil || input.EventTypes == nil {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.preference_required", "emailEnabled and eventTypes are required")
		return
	}
	eventTypes, valid := normalizeNotificationEventTypes(*input.EventTypes)
	if !valid {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.preference_event_types_invalid", "notification event types must be unique supported failure events")
		return
	}
	now := time.Now()
	preference := model.UserNotificationPreference{
		UserID:         user.ID,
		EmailEnabled:   *input.EmailEnabled,
		EventTypesJSON: notification.EncodeStringList(eventTypes),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := h.dbFor(ctx).Model(&model.UserNotificationPreference{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"email_enabled", "event_types_json", "updated_at"}),
	}).Create(map[string]any{
		"user_id":          preference.UserID,
		"email_enabled":    preference.EmailEnabled,
		"event_types_json": preference.EventTypesJSON,
		"created_at":       preference.CreatedAt,
		"updated_at":       preference.UpdatedAt,
	}).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.preference.update", user.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, myNotificationPreferenceResponse(preference))
}

func (h *Handler) ListMyNotificationPresets(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}
	ctx.JSON(http.StatusOK, notification.PersonalWebhookPresets())
}

func (h *Handler) ListMyNotificationChannels(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := personalNotificationChannels(h.dbFor(ctx).Model(&model.NotificationChannel{}), user.ID)
	if search := strings.TrimSpace(ctx.Query("search")); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var channels []model.NotificationChannel
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"createdAt": "created_at",
		"updatedAt": "updated_at",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&channels).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.populateLatestNotificationDeliveries(channels, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(notificationChannelResponses(channels), total, pagination))
}

func (h *Handler) CreateMyNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input personalNotificationChannelCreateInput
	if !bindPersonalNotificationJSON(ctx, &input) {
		return
	}
	if !validatePersonalNotificationChannelName(ctx, input.Name) {
		return
	}
	preset, found := personalNotificationPreset(input.PresetID)
	if !found {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.preset_not_found", "notification preset not found")
		return
	}
	secrets, secretErrorCode := personalNotificationPresetSecrets(input.Secrets, preset.SecretFields)
	if secretErrorCode != "" {
		writeErrorCode(ctx, http.StatusBadRequest, secretErrorCode, "notification preset secrets are invalid")
		return
	}
	channel := model.NotificationChannel{
		ID:          id.New("nch"),
		OwnerUserID: user.ID,
		Name:        strings.TrimSpace(input.Name),
		AdapterKind: preset.AdapterKind,
		ConfigJSON:  preset.ConfigTemplate,
		Enabled:     true,
		CreatedBy:   user.ID,
	}
	if input.Enabled != nil {
		channel.Enabled = *input.Enabled
	}
	if err := validateNotificationChannel(ctx.Request.Context(), channel, h.secrets()); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		lockKey := "personal_notification_channels:" + strings.TrimSpace(user.ID)
		if err := tx.Exec("select pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey).Error; err != nil {
			return err
		}
		var channelCount int64
		if err := personalNotificationChannels(tx.Model(&model.NotificationChannel{}), user.ID).Count(&channelCount).Error; err != nil {
			return err
		}
		if channelCount >= personalNotificationChannelLimit {
			return errPersonalNotificationChannelLimit
		}
		secretRefs := make(map[string]string, len(secrets))
		for key, value := range secrets {
			ref, err := h.secrets().StoreContextWithDB(
				ctx.Request.Context(),
				tx,
				value,
				user.ID,
				"notification_channel:"+channel.ID+":"+key,
			)
			if err != nil {
				return err
			}
			secretRefs[key] = ref
		}
		channel.SecretRefsJSON = mustJSON(secretRefs)
		return tx.Create(&channel).Error
	})
	if errors.Is(err, errPersonalNotificationChannelLimit) {
		writeErrorCode(ctx, http.StatusConflict, "notification.channel_limit_reached", "personal notification channel limit reached")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.personal_channel.create", channel.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusCreated, notificationChannelResponseFor(channel))
}

func (h *Handler) UpdateMyNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input personalNotificationChannelUpdateInput
	if !bindPersonalNotificationJSON(ctx, &input) {
		return
	}
	if !validatePersonalNotificationChannelName(ctx, input.Name) {
		return
	}
	var channel model.NotificationChannel
	secretErrorCode := ""
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		query := personalNotificationChannels(tx, user.ID).Clauses(clause.Locking{Strength: "UPDATE"})
		if err := query.First(&channel, "id = ?", strings.TrimSpace(ctx.Param("channelId"))).Error; err != nil {
			return err
		}
		secrets, code := personalNotificationExistingSecrets(input.Secrets, channel.SecretRefsJSON)
		if code != "" {
			secretErrorCode = code
			return errPersonalNotificationSecretsInvalid
		}
		channel.Name = strings.TrimSpace(input.Name)
		if input.Enabled != nil {
			channel.Enabled = *input.Enabled
		}
		if err := validateNotificationChannel(ctx.Request.Context(), channel, h.secrets()); err != nil {
			return err
		}
		secretRefs := decodeStringMap(channel.SecretRefsJSON)
		for key, value := range secrets {
			resource := "notification_channel:" + channel.ID + ":" + key
			newRef, err := h.secrets().StoreContextWithDB(ctx.Request.Context(), tx, value, user.ID, resource)
			if err != nil {
				return err
			}
			oldRef := secretRefs[key]
			secretRefs[key] = newRef
			if err := h.secrets().DeleteRefContextWithDB(ctx.Request.Context(), tx, oldRef, resource); err != nil {
				return err
			}
		}
		channel.SecretRefsJSON = mustJSON(secretRefs)
		return tx.Save(&channel).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	if errors.Is(err, errPersonalNotificationSecretsInvalid) {
		writeErrorCode(ctx, http.StatusBadRequest, secretErrorCode, "notification preset secrets are invalid")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.personal_channel.update", channel.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, notificationChannelResponseFor(channel))
}

func (h *Handler) DeleteMyNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var channel model.NotificationChannel
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		query := personalNotificationChannels(tx, user.ID).Clauses(clause.Locking{Strength: "UPDATE"})
		if err := query.First(&channel, "id = ?", strings.TrimSpace(ctx.Param("channelId"))).Error; err != nil {
			return err
		}
		for key, ref := range decodeStringMap(channel.SecretRefsJSON) {
			resource := "notification_channel:" + channel.ID + ":" + key
			if err := h.secrets().DeleteRefContextWithDB(ctx.Request.Context(), tx, ref, resource); err != nil {
				return err
			}
		}
		return tx.Delete(&channel).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.personal_channel.delete", channel.ID, true, "", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) TestMyNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var channel model.NotificationChannel
	if err := personalNotificationChannels(h.dbFor(ctx), user.ID).
		First(&channel, "id = ?", strings.TrimSpace(ctx.Param("channelId"))).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	if !h.allowPersonalNotificationTest(ctx, user.ID) {
		return
	}
	adapter, err := notification.DefaultRegistry().Adapter(channel.AdapterKind)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := adapter.Test(ctx.Request.Context(), []byte(channel.ConfigJSON), []byte(channel.SecretRefsJSON), h.secrets()); err != nil {
		h.WritePersonalNotificationTestFailure(ctx, user.ID, channel.ID, err)
		return
	}
	h.auditWithContext(user.ID, "notification.personal_channel.test", channel.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) WritePersonalNotificationTestFailure(ctx *gin.Context, userID, channelID string, err error) {
	const code = "notification.channel_test_failed"
	h.auditWithContext(userID, "notification.personal_channel.test", channelID, false, code, ctx.Request.Context())
	writeErrorCode(ctx, http.StatusBadGateway, code, err.Error())
}

func (h *Handler) ListMyNotificationDeliveries(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := personalNotificationDeliveries(h.dbFor(ctx).Model(&model.NotificationDelivery{}), user.ID)
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if eventType := strings.TrimSpace(ctx.Query("eventType")); eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	var total int64
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

func myNotificationPreferenceResponse(preference model.UserNotificationPreference) userNotificationPreferenceResponse {
	return userNotificationPreferenceResponse{
		EmailEnabled: preference.EmailEnabled,
		EventTypes:   decodeStringList(preference.EventTypesJSON),
	}
}

func normalizeNotificationEventTypes(values []string) ([]string, bool) {
	allowed := make(map[string]bool, len(notification.DefaultFailureEventTypes()))
	for _, eventType := range notification.DefaultFailureEventTypes() {
		allowed[eventType] = true
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !allowed[value] || seen[value] {
			return nil, false
		}
		seen[value] = true
		out = append(out, value)
	}
	return out, true
}

func personalNotificationPreset(presetID string) (notification.WebhookPreset, bool) {
	presetID = strings.TrimSpace(presetID)
	for _, preset := range notification.PersonalWebhookPresets() {
		if preset.ID == presetID {
			return preset, true
		}
	}
	return notification.WebhookPreset{}, false
}

func personalNotificationPresetSecrets(values map[string]string, required []string) (map[string]string, string) {
	allowed := make(map[string]bool, len(required))
	for _, field := range required {
		allowed[field] = true
	}
	for field := range values {
		if !allowed[field] {
			return nil, "notification.secret_field_invalid"
		}
	}
	secrets := make(map[string]string, len(required))
	for _, field := range required {
		value := strings.TrimSpace(values[field])
		if value == "" {
			return nil, "notification.secret_required"
		}
		if utf8.RuneCountInString(value) > personalNotificationSecretMaxLength {
			return nil, "notification.secret_too_long"
		}
		secrets[field] = value
	}
	return secrets, ""
}

func personalNotificationExistingSecrets(values map[string]string, existingRefsJSON string) (map[string]string, string) {
	existing := decodeStringMap(existingRefsJSON)
	for field := range values {
		if _, allowed := existing[field]; !allowed {
			return nil, "notification.secret_field_invalid"
		}
	}
	secrets := make(map[string]string, len(existing))
	for field := range existing {
		if value := strings.TrimSpace(values[field]); value != "" {
			if utf8.RuneCountInString(value) > personalNotificationSecretMaxLength {
				return nil, "notification.secret_too_long"
			}
			secrets[field] = value
		}
	}
	return secrets, ""
}

func bindPersonalNotificationJSON(ctx *gin.Context, value any) bool {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, personalNotificationRequestMaxBytes)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return writePersonalNotificationJSONError(ctx, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values are not allowed")
		}
		return writePersonalNotificationJSONError(ctx, err)
	}
	return true
}

func writePersonalNotificationJSONError(ctx *gin.Context, err error) bool {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		writeErrorCode(ctx, http.StatusRequestEntityTooLarge, "notification.request_too_large", "personal notification request exceeds 64 KiB")
		return false
	}
	writeErrorCode(ctx, http.StatusBadRequest, "request.invalid_json", "personal notification request JSON is invalid")
	return false
}

func validatePersonalNotificationChannelName(ctx *gin.Context, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > personalNotificationNameMaxLength {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.channel_name_invalid", "personal notification channel name must contain 1 to 160 characters")
		return false
	}
	return true
}

func personalNotificationChannels(db *gorm.DB, userID string) *gorm.DB {
	return db.Where("owner_user_id = ? and adapter_kind = ?", strings.TrimSpace(userID), notification.AdapterKindWebhook)
}

func personalNotificationDeliveries(db *gorm.DB, userID string) *gorm.DB {
	return db.Where("recipient_user_id = ?", strings.TrimSpace(userID))
}
