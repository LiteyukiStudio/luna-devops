package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestPersonalNotificationInputsRejectInvalidPresetSecretsAndPreferences(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		body   string
		handle func(*Handlers, *gin.Context)
		code   string
		status int
	}{
		{
			name:   "unknown preset",
			method: http.MethodPost,
			target: "/api/v1/me/notification-channels",
			body:   `{"name":"unknown","presetId":"unknown","secrets":{}}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.CreateMyNotificationChannel(ctx) },
			code:   "notification.preset_not_found",
		},
		{
			name:   "custom adapter and config fields",
			method: http.MethodPost,
			target: "/api/v1/me/notification-channels",
			body:   `{"name":"private smtp","presetId":"feishu-bot","secrets":{"WebhookToken":"token"},"adapterKind":"smtp","config":{}}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.CreateMyNotificationChannel(ctx) },
			code:   "request.invalid_json",
		},
		{
			name:   "missing preset secret",
			method: http.MethodPost,
			target: "/api/v1/me/notification-channels",
			body:   `{"name":"Feishu","presetId":"feishu-bot","secrets":{}}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.CreateMyNotificationChannel(ctx) },
			code:   "notification.secret_required",
		},
		{
			name:   "unknown preference event",
			method: http.MethodPut,
			target: "/api/v1/me/notification-preferences",
			body:   `{"emailEnabled":true,"eventTypes":["build.failed","custom.failed"]}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.UpdateMyNotificationPreferences(ctx) },
			code:   "notification.preference_event_types_invalid",
		},
		{
			name:   "missing both preference fields",
			method: http.MethodPut,
			target: "/api/v1/me/notification-preferences",
			body:   `{}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.UpdateMyNotificationPreferences(ctx) },
			code:   "notification.preference_required",
		},
		{
			name:   "missing email preference field",
			method: http.MethodPut,
			target: "/api/v1/me/notification-preferences",
			body:   `{"eventTypes":[]}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.UpdateMyNotificationPreferences(ctx) },
			code:   "notification.preference_required",
		},
		{
			name:   "missing event preference field",
			method: http.MethodPut,
			target: "/api/v1/me/notification-preferences",
			body:   `{"emailEnabled":false}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.UpdateMyNotificationPreferences(ctx) },
			code:   "notification.preference_required",
		},
		{
			name:   "unknown preference field",
			method: http.MethodPut,
			target: "/api/v1/me/notification-preferences",
			body:   `{"emailEnabled":true,"eventTypes":[],"unexpected":true}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.UpdateMyNotificationPreferences(ctx) },
			code:   "request.invalid_json",
		},
		{
			name:   "multiple JSON values",
			method: http.MethodPut,
			target: "/api/v1/me/notification-preferences",
			body:   `{"emailEnabled":true,"eventTypes":[]} {}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.UpdateMyNotificationPreferences(ctx) },
			code:   "request.invalid_json",
		},
		{
			name:   "request body over 64 KiB",
			method: http.MethodPost,
			target: "/api/v1/me/notification-channels",
			body:   `{"name":"` + strings.Repeat("a", personalNotificationRequestMaxBytes) + `","presetId":"feishu-bot","secrets":{"WebhookToken":"token"}}`,
			handle: func(h *Handlers, ctx *gin.Context) { h.CreateMyNotificationChannel(ctx) },
			code:   "notification.request_too_large",
			status: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder, ctx := newUserNotificationHandlerContext(tt.method, tt.target, "usr_current", tt.body)
			tt.handle(&Handlers{}, ctx)

			wantStatus := tt.status
			if wantStatus == 0 {
				wantStatus = http.StatusBadRequest
			}
			if recorder.Code != wantStatus {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.code) {
				t.Fatalf("body = %s, want error code %q", recorder.Body.String(), tt.code)
			}
		})
	}
}

func TestPersonalNotificationChannelPresetControlsStoredAdapterAndConfig(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "personal-notification-preset-test-key")
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_preset_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.NotificationChannel{}, &model.SecretValue{}, &model.AuditLog{})
		},
	})
	handlers := &Handlers{db: db, secrets: secret.NewStore(db, nil)}

	recorder, ctx := newUserNotificationHandlerContext(
		http.MethodPost,
		"/api/v1/me/notification-channels",
		"usr_current",
		`{"name":"My Feishu","presetId":"feishu-bot","secrets":{"WebhookToken":"token-value"},"enabled":true}`,
	)
	handlers.CreateMyNotificationChannel(ctx)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var channel model.NotificationChannel
	if err := db.First(&channel, "owner_user_id = ?", "usr_current").Error; err != nil {
		t.Fatalf("load created channel: %v", err)
	}
	if channel.AdapterKind != notification.AdapterKindWebhook || !strings.Contains(channel.ConfigJSON, "open.feishu.cn") || strings.Contains(channel.ConfigJSON, "attacker.example") {
		t.Fatalf("stored channel escaped preset: adapter=%q config=%s", channel.AdapterKind, channel.ConfigJSON)
	}
	secretRefs := decodeStringMap(channel.SecretRefsJSON)
	if len(secretRefs) != 1 || secretRefs["WebhookToken"] == "" {
		t.Fatalf("stored secret refs = %#v, want only preset fields", secretRefs)
	}
	originalConfig := channel.ConfigJSON
	originalSecretRef := secretRefs["WebhookToken"]

	updateRecorder, updateCtx := newUserNotificationHandlerContext(
		http.MethodPut,
		"/api/v1/me/notification-channels/"+channel.ID,
		"usr_current",
		`{"name":"Renamed Feishu","secrets":{},"enabled":false}`,
	)
	updateCtx.Params = gin.Params{{Key: "channelId", Value: channel.ID}}
	handlers.UpdateMyNotificationChannel(updateCtx)
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateRecorder.Code, updateRecorder.Body.String())
	}
	if err := db.First(&channel, "id = ?", channel.ID).Error; err != nil {
		t.Fatalf("reload updated channel: %v", err)
	}
	if channel.Name != "Renamed Feishu" || channel.Enabled || channel.AdapterKind != notification.AdapterKindWebhook || channel.ConfigJSON != originalConfig {
		t.Fatalf("updated channel = %#v", channel)
	}
	if got := decodeStringMap(channel.SecretRefsJSON)["WebhookToken"]; got != originalSecretRef {
		t.Fatalf("empty secret update replaced existing reference: got %q want %q", got, originalSecretRef)
	}

	rotateRecorder, rotateCtx := newUserNotificationHandlerContext(
		http.MethodPut,
		"/api/v1/me/notification-channels/"+channel.ID,
		"usr_current",
		`{"name":"Renamed Feishu","secrets":{"WebhookToken":"rotated-token"},"enabled":false}`,
	)
	rotateCtx.Params = gin.Params{{Key: "channelId", Value: channel.ID}}
	handlers.UpdateMyNotificationChannel(rotateCtx)
	if rotateRecorder.Code != http.StatusOK {
		t.Fatalf("rotate status = %d, body = %s", rotateRecorder.Code, rotateRecorder.Body.String())
	}
	if err := db.First(&channel, "id = ?", channel.ID).Error; err != nil {
		t.Fatal(err)
	}
	rotatedRef := decodeStringMap(channel.SecretRefsJSON)["WebhookToken"]
	if rotatedRef == "" || rotatedRef == originalSecretRef || handlers.secrets.ResolveContext(ctx.Request.Context(), originalSecretRef) != "" || handlers.secrets.ResolveContext(ctx.Request.Context(), rotatedRef) != "rotated-token" {
		t.Fatalf("secret rotation did not replace and remove the old ref: old=%q new=%q", originalSecretRef, rotatedRef)
	}
	var secretCount int64
	if err := db.Model(&model.SecretValue{}).Count(&secretCount).Error; err != nil || secretCount != 1 {
		t.Fatalf("secret count after rotation = %d, err = %v", secretCount, err)
	}

	deleteRecorder, deleteCtx := newUserNotificationHandlerContext(http.MethodDelete, "/api/v1/me/notification-channels/"+channel.ID, "usr_current", "")
	deleteCtx.Params = gin.Params{{Key: "channelId", Value: channel.ID}}
	handlers.DeleteMyNotificationChannel(deleteCtx)
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
	if err := db.Model(&model.SecretValue{}).Count(&secretCount).Error; err != nil || secretCount != 0 {
		t.Fatalf("secret count after delete = %d, err = %v", secretCount, err)
	}
}

func TestPersonalNotificationSecretAndNameLimits(t *testing.T) {
	if _, code := personalNotificationPresetSecrets(
		map[string]string{"WebhookToken": "token", "Unexpected": "value"},
		[]string{"WebhookToken"},
	); code != "notification.secret_field_invalid" {
		t.Fatalf("extra secret code = %q", code)
	}
	if _, code := personalNotificationPresetSecrets(
		map[string]string{"WebhookToken": strings.Repeat("x", personalNotificationSecretMaxLength+1)},
		[]string{"WebhookToken"},
	); code != "notification.secret_too_long" {
		t.Fatalf("long secret code = %q", code)
	}
	if _, code := personalNotificationExistingSecrets(
		map[string]string{"Unexpected": "value"},
		`{"WebhookToken":"secret-ref"}`,
	); code != "notification.secret_field_invalid" {
		t.Fatalf("extra update secret code = %q", code)
	}

	recorder, ctx := newUserNotificationHandlerContext(http.MethodPost, "/api/v1/me/notification-channels", "usr_current", "")
	if validatePersonalNotificationChannelName(ctx, strings.Repeat("名", personalNotificationNameMaxLength+1)) {
		t.Fatal("overlong personal channel name was accepted")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "notification.channel_name_invalid") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestPersonalNotificationPresetsExcludeGotify(t *testing.T) {
	recorder, ctx := newUserNotificationHandlerContext(http.MethodGet, "/api/v1/me/notification-presets", "usr_current", "")
	(&Handlers{}).ListMyNotificationPresets(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"gotify"`) || !strings.Contains(recorder.Body.String(), `"feishu-bot"`) {
		t.Fatalf("personal preset response = %s", recorder.Body.String())
	}
}

func TestPersonalNotificationTestRateLimitIsPerUserAndStable(t *testing.T) {
	server := miniredis.RunT(t)
	handlers := &Handlers{mode: "production", rateLimiter: newRateLimiter(server.Addr())}
	t.Cleanup(func() { _ = handlers.rateLimiter.redis.Close() })

	for attempt := 1; attempt <= personalNotificationTestRateLimit+1; attempt++ {
		recorder, ctx := newUserNotificationHandlerContext(http.MethodPost, "/api/v1/me/notification-channels/nch_test/test", "usr_sensitive", "")
		allowed := handlers.allowPersonalNotificationTest(ctx, "usr_sensitive")
		if attempt <= personalNotificationTestRateLimit {
			if !allowed {
				t.Fatalf("attempt %d was unexpectedly denied: %s", attempt, recorder.Body.String())
			}
			continue
		}
		if allowed || recorder.Code != http.StatusTooManyRequests || !strings.Contains(recorder.Body.String(), "notification.test_rate_limited") {
			t.Fatalf("attempt %d: allowed=%t status=%d body=%s", attempt, allowed, recorder.Code, recorder.Body.String())
		}
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, "usr_sensitive") {
			t.Fatalf("rate limit key exposes user ID: %q", key)
		}
	}
}

func TestPersonalNotificationChannelLimit(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_channel_limit_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.NotificationChannel{}, &model.SecretValue{}, &model.AuditLog{})
		},
	})
	channels := make([]model.NotificationChannel, 0, personalNotificationChannelLimit)
	for index := int64(0); index < personalNotificationChannelLimit; index++ {
		channels = append(channels, model.NotificationChannel{
			ID: "nch_limit_" + string(rune('a'+index)), OwnerUserID: "usr_current", Name: "channel",
			AdapterKind: notification.AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true,
		})
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, secrets: secret.NewStore(db, nil)}
	recorder, ctx := newUserNotificationHandlerContext(
		http.MethodPost,
		"/api/v1/me/notification-channels",
		"usr_current",
		`{"name":"extra","presetId":"feishu-bot","secrets":{"WebhookToken":"token"}}`,
	)
	handlers.CreateMyNotificationChannel(ctx)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "notification.channel_limit_reached") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestPersonalNotificationChannelLimitIsAtomic(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "personal-notification-atomic-limit-test-key")
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_atomic_limit_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.NotificationChannel{}, &model.SecretValue{}, &model.AuditLog{})
		},
	})
	channels := make([]model.NotificationChannel, 0, personalNotificationChannelLimit-1)
	for index := int64(0); index < personalNotificationChannelLimit-1; index++ {
		channels = append(channels, model.NotificationChannel{
			ID: "nch_atomic_" + string(rune('a'+index)), OwnerUserID: "usr_current", Name: "channel",
			AdapterKind: notification.AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true,
		})
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, secrets: secret.NewStore(db, nil)}
	recorders := make([]*httptest.ResponseRecorder, 2)
	contexts := make([]*gin.Context, 2)
	for index := range contexts {
		recorders[index], contexts[index] = newUserNotificationHandlerContext(
			http.MethodPost,
			"/api/v1/me/notification-channels",
			"usr_current",
			`{"name":"concurrent","presetId":"feishu-bot","secrets":{"WebhookToken":"token"}}`,
		)
	}
	var workers sync.WaitGroup
	for index := range contexts {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			handlers.CreateMyNotificationChannel(contexts[index])
		}(index)
	}
	workers.Wait()

	created, limited := 0, 0
	for _, recorder := range recorders {
		switch recorder.Code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			if !strings.Contains(recorder.Body.String(), "notification.channel_limit_reached") {
				t.Fatalf("unexpected conflict body: %s", recorder.Body.String())
			}
			limited++
		default:
			t.Fatalf("unexpected status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	}
	if created != 1 || limited != 1 {
		t.Fatalf("created = %d, limited = %d", created, limited)
	}
	var channelCount int64
	if err := personalNotificationChannels(db.Model(&model.NotificationChannel{}), "usr_current").Count(&channelCount).Error; err != nil {
		t.Fatal(err)
	}
	if channelCount != personalNotificationChannelLimit {
		t.Fatalf("channel count = %d", channelCount)
	}
	var secretCount int64
	if err := db.Model(&model.SecretValue{}).Count(&secretCount).Error; err != nil {
		t.Fatal(err)
	}
	if secretCount != 1 {
		t.Fatalf("secret count = %d, want only the committed channel secret", secretCount)
	}
}

func TestPersonalNotificationTestFailureUsesStableAuditMessage(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_test_audit_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.AuditLog{})
		},
	})
	handlers := &Handlers{db: db}
	recorder, ctx := newUserNotificationHandlerContext(http.MethodPost, "/api/v1/me/notification-channels/nch_test/test", "usr_current", "")
	handlers.writePersonalNotificationTestFailure(
		ctx,
		"usr_current",
		"nch_test",
		errors.New(`Post "https://hooks.slack.com/services/path-secret-marker": remote failure`),
	)
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "notification.channel_test_failed") || strings.Contains(recorder.Body.String(), "path-secret-marker") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var audit model.AuditLog
	if err := db.First(&audit, "action = ?", "notification.personal_channel.test").Error; err != nil {
		t.Fatal(err)
	}
	if audit.Message != "notification.channel_test_failed" || strings.Contains(audit.Message, "path-secret-marker") {
		t.Fatalf("audit message = %q", audit.Message)
	}
}

func TestUpdatePersonalNotificationPreferencesAllowsEmptyEventsAndPersistsFalse(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_preference_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.UserNotificationPreference{}, &model.AuditLog{})
		},
	})
	handlers := &Handlers{db: db}
	recorder, ctx := newUserNotificationHandlerContext(
		http.MethodPut,
		"/api/v1/me/notification-preferences",
		"usr_current",
		`{"emailEnabled":false,"eventTypes":[]}`,
	)
	handlers.UpdateMyNotificationPreferences(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var preference model.UserNotificationPreference
	if err := db.First(&preference, "user_id = ?", "usr_current").Error; err != nil {
		t.Fatalf("load preference: %v", err)
	}
	if preference.EmailEnabled || preference.EventTypesJSON != "[]" {
		t.Fatalf("stored preference = %#v", preference)
	}
}

func TestPersonalNotificationChannelsAreOwnerScoped(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_api_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.NotificationChannel{}, &model.NotificationDelivery{})
		},
	})
	channels := []model.NotificationChannel{
		{ID: "nch_current", OwnerUserID: "usr_current", Name: "current webhook", AdapterKind: notification.AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true},
		{ID: "nch_other", OwnerUserID: "usr_other", Name: "other webhook", AdapterKind: notification.AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true},
		{ID: "nch_shared", OwnerUserID: "", Name: "shared webhook", AdapterKind: notification.AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true},
		{ID: "nch_current_smtp", OwnerUserID: "usr_current", Name: "forged smtp", AdapterKind: notification.AdapterKindSMTP, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create notification channels: %v", err)
	}
	handlers := &Handlers{db: db}

	recorder, ctx := newUserNotificationHandlerContext(http.MethodGet, "/api/v1/me/notification-channels", "usr_current", "")
	handlers.ListMyNotificationChannels(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []model.NotificationChannel `json:"items"`
		Total int64                       `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if response.Total != 1 || len(response.Items) != 1 || response.Items[0].ID != "nch_current" {
		t.Fatalf("owner-scoped list = %#v", response)
	}

	mutations := []struct {
		name   string
		method string
		body   string
		handle func(*gin.Context)
	}{
		{
			name:   "update",
			method: http.MethodPut,
			body:   `{"name":"changed","secrets":{},"enabled":true}`,
			handle: handlers.UpdateMyNotificationChannel,
		},
		{name: "delete", method: http.MethodDelete, handle: handlers.DeleteMyNotificationChannel},
		{name: "test", method: http.MethodPost, handle: handlers.TestMyNotificationChannel},
	}
	for _, mutation := range mutations {
		t.Run("cannot "+mutation.name+" another user's channel", func(t *testing.T) {
			recorder, ctx := newUserNotificationHandlerContext(mutation.method, "/api/v1/me/notification-channels/nch_other", "usr_current", mutation.body)
			ctx.Params = gin.Params{{Key: "channelId", Value: "nch_other"}}
			mutation.handle(ctx)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}

	var other model.NotificationChannel
	if err := db.First(&other, "id = ?", "nch_other").Error; err != nil {
		t.Fatalf("other user's channel changed or deleted: %v", err)
	}
	if other.Name != "other webhook" {
		t.Fatalf("other user's channel name = %q", other.Name)
	}
}

func newUserNotificationHandlerContext(method, target, userID, body string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set(currentUserContextKey, model.User{ID: userID, Language: "zh-CN"})
	return recorder, ctx
}
