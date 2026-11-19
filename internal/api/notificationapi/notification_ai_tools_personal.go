package notificationapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type opsGetMyNotificationsInput struct {
	IncludePresets     *bool `json:"includePresets,omitempty" default:"false" desc:"是否返回 6 类个人机器人 Webhook 预设及所需密钥键名说明"`
	IncludeChannels    *bool `json:"includeChannels,omitempty" default:"true" desc:"是否返回已配置个人通知渠道（仅回显 secretSet，绝不回显密钥）"`
	IncludePreferences *bool `json:"includePreferences,omitempty" default:"true" desc:"是否返回 emailEnabled 与已订阅的失败事件类型"`
	IncludeDeliveries  *bool `json:"includeDeliveries,omitempty" default:"false" desc:"是否返回最近 20 条个人通知投递记录"`
}

type opsSaveMyNotificationChannelInput struct {
	Channel         *string           `json:"channel,omitempty" desc:"省略为新建；传入个人渠道 ID 或唯一名称为增量修改"`
	Name            *string           `json:"name,omitempty" desc:"渠道名称（新建必填，1..160 字符）"`
	PresetID        *string           `json:"presetId,omitempty" enum:"feishu-bot|lark-bot|wecom-bot|dingtalk-bot|slack-webhook|discord-webhook" desc:"个人机器人预设（新建必填，创建后不可修改）"`
	UpsertSecrets   map[string]string `json:"upsertSecrets,omitempty" sensitive:"true" desc:"按预设白名单写入机器人令牌或签名密钥（未提及键原样保留）"`
	ClearSecretKeys []string          `json:"clearSecretKeys,omitempty" desc:"显式清除可选密钥键（如取消 SignSecret）"`
	Enabled         *bool             `json:"enabled,omitempty" desc:"是否启用（新建默认 true）"`
}

type opsDeleteMyNotificationChannelInput struct {
	Channel string `json:"channel" desc:"个人通知渠道 ID 或唯一名称"`
}

type opsConfigureMyNotificationPreferencesInput struct {
	EmailEnabled     *bool    `json:"emailEnabled,omitempty" desc:"个人失败事件邮件通知开关"`
	AddEventTypes    []string `json:"addEventTypes,omitempty" enum:"build.failed|build.webhook_rejected|release.failed|runtime.pod_oom_killed|runtime.pod_crash_loop|runtime.pod_start_failed|runtime.workload_degraded|runtime.recovered|hook.failed|gateway.apply_failed|certificate.failed|certificate.expired" desc:"增量开启订阅的个人失败事件类型"`
	RemoveEventTypes []string `json:"removeEventTypes,omitempty" desc:"增量取消订阅的个人失败事件类型"`
}

type opsTestOrRetryMyNotificationInput struct {
	Channel         *string `json:"channel,omitempty" desc:"测试指定个人通知渠道 ID 或名称（与 retryDeliveryId 互斥二选一）"`
	RetryDeliveryID *string `json:"retryDeliveryId,omitempty" desc:"重试归属本人的失败通知投递记录 ID（与 channel 互斥二选一）"`
}

type lockedPersonalChannelMutation struct {
	ResolvedChannelID string `json:"resolvedChannelId,omitempty"`
	DeliveryID        string `json:"deliveryId,omitempty"`
}

func (h *Handler) registerOpsPersonalNotificationAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[opsGetMyNotificationsInput, map[string]any]{
		Name:           "ops_get_my_notifications",
		Domain:         aitool.DomainOps,
		Title:          "查看个人通知设置与渠道",
		Summary:        "查看当前用户个人的通知偏好开关、6 种可用个人机器人预设、已配置个人通知渠道及最近投递记录",
		Tags:           []string{"notifications", "personal"},
		AliasesZH:      []string{"查看我的通知设置", "个人机器人渠道", "个人通知记录"},
		AliasesEN:      []string{"get my notifications", "personal notification channels", "my notification preferences"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionUserRead)},
		Execute:        h.aiOpsGetMyNotifications,
	})

	aitool.Register(reg, aitool.Spec[opsSaveMyNotificationChannelInput, notificationChannelResponse]{
		Name:           "ops_save_my_notification_channel",
		Domain:         aitool.DomainOps,
		Title:          "保存个人机器人通知渠道",
		Summary:        "创建或增量修改个人机器人通知渠道（单用户上限 10 个，仅支持 6 种个人预设，Secret 只进不出）",
		Tags:           []string{"notifications", "personal"},
		AliasesZH:      []string{"创建个人通知机器人", "修改我的 Webhook 渠道", "配置个人飞书钉钉通知"},
		AliasesEN:      []string{"save my notification channel", "create personal webhook"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionUserWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_get_my_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				Collection:      true,
				CollectionPath:  "channels",
				StaticArguments: map[string]any{"includeChannels": true},
				Expect:          "present",
			},
		},
		Preview: h.aiPreviewOpsSaveMyNotificationChannel,
		Execute: h.aiExecuteOpsSaveMyNotificationChannel,
	})

	aitool.Register(reg, aitool.Spec[opsDeleteMyNotificationChannelInput, map[string]any]{
		Name:           "ops_delete_my_notification_channel",
		Domain:         aitool.DomainOps,
		Title:          "删除个人机器人通知渠道",
		Summary:        "删除当前用户本人拥有的个人机器人通知渠道",
		Tags:           []string{"notifications", "personal"},
		AliasesZH:      []string{"删除个人通知渠道", "删除我的机器人通知"},
		AliasesEN:      []string{"delete my notification channel", "remove personal webhook"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionUserWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_get_my_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				Collection:      true,
				CollectionPath:  "channels",
				StaticArguments: map[string]any{"includeChannels": true},
				Expect:          "absent",
			},
		},
		Preview: h.aiPreviewOpsDeleteMyNotificationChannel,
		Execute: h.aiExecuteOpsDeleteMyNotificationChannel,
	})

	aitool.Register(reg, aitool.Spec[opsConfigureMyNotificationPreferencesInput, userNotificationPreferenceResponse]{
		Name:           "ops_configure_my_notification_preferences",
		Domain:         aitool.DomainOps,
		Title:          "配置个人邮件通知偏好",
		Summary:        "增量修改个人邮件通知开关与订阅的失败事件类型",
		Tags:           []string{"notifications", "personal"},
		AliasesZH:      []string{"修改个人通知偏好", "开关邮件通知", "订阅失败通知"},
		AliasesEN:      []string{"configure my notification preferences", "update personal email notifications"},
		ApprovalTier:   aitool.ApprovalTierL1,
		Idempotent:     true,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionUserWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_get_my_notifications",
				Family:          "resource",
				Singleton:       true,
				StaticArguments: map[string]any{"includePreferences": true},
				Expect:          "present",
			},
		},
		Execute: h.aiOpsConfigureMyNotificationPreferences,
	})

	aitool.Register(reg, aitool.Spec[opsTestOrRetryMyNotificationInput, map[string]any]{
		Name:           "ops_test_or_retry_my_notification",
		Domain:         aitool.DomainOps,
		Title:          "测试个人通知渠道或重试个人投递",
		Summary:        "发送个人机器人渠道测试消息或重试失败的个人通知投递记录",
		Tags:           []string{"notifications", "personal"},
		AliasesZH:      []string{"测试个人通知渠道", "重试个人通知投递"},
		AliasesEN:      []string{"test my notification channel", "retry personal notification delivery"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectExecute,
		RequiredScopes: []string{string(authz.ActionUserWrite)},
		Preview:        h.aiPreviewOpsTestOrRetryMyNotification,
		Execute:        h.aiExecuteOpsTestOrRetryMyNotification,
	})
}

func resolvePersonalChannelForAI(db *gorm.DB, userID, channelRef string) (model.NotificationChannel, error) {
	channelRef = strings.TrimSpace(channelRef)
	if channelRef == "" {
		return model.NotificationChannel{}, aitool.BadRequest("notification.channel_required", "channel is required")
	}
	var channels []model.NotificationChannel
	if err := personalNotificationChannels(db, userID).Order("created_at asc, id asc").Find(&channels).Error; err != nil {
		return model.NotificationChannel{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		channels,
		"notification_channel",
		"notification.channel_not_found",
		channelRef,
		func(c model.NotificationChannel) string { return c.ID },
		func(c model.NotificationChannel) string { return c.Name },
	)
}

func (h *Handler) aiOpsGetMyNotifications(tc *aitool.ToolContext, in opsGetMyNotificationsInput) (map[string]any, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	tc.SetNoStore()

	includePresets := notifBoolOrDefault(in.IncludePresets, false)
	includeChannels := notifBoolOrDefault(in.IncludeChannels, true)
	includePreferences := notifBoolOrDefault(in.IncludePreferences, true)
	includeDeliveries := notifBoolOrDefault(in.IncludeDeliveries, false)

	out := map[string]any{
		"userId": user.ID,
	}

	if includePreferences {
		pref := notification.DefaultUserNotificationPreference(user.ID)
		if err := db.First(&pref, "user_id = ?", user.ID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		out["preferences"] = myNotificationPreferenceResponse(pref)
	}

	if includePresets {
		out["presets"] = notification.PersonalWebhookPresets()
	}

	if includeChannels {
		var channels []model.NotificationChannel
		if err := personalNotificationChannels(db.Model(&model.NotificationChannel{}), user.ID).
			Order("created_at desc, id desc").
			Find(&channels).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if err := h.populateLatestNotificationDeliveries(channels, tc.Context()); err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		out["channels"] = notificationChannelResponses(channels)
	}

	if includeDeliveries {
		var deliveries []model.NotificationDelivery
		if err := personalNotificationDeliveries(db.Model(&model.NotificationDelivery{}), user.ID).
			Order("created_at desc, id desc").
			Limit(20).
			Find(&deliveries).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		h.enrichPersonalNotificationDeliveries(deliveries, user, tc.Context())
		if deliveries == nil {
			deliveries = []model.NotificationDelivery{}
		}
		out["deliveries"] = deliveries
	}

	return out, nil
}

func validatePersonalChannelDraft(
	db *gorm.DB,
	user model.User,
	existing *model.NotificationChannel,
	in opsSaveMyNotificationChannelInput,
) (model.NotificationChannel, notification.WebhookPreset, map[string]string, []string, []string, []aitool.PreviewChange, error) {
	isCreate := existing == nil
	var preset notification.WebhookPreset
	if isCreate {
		if in.PresetID == nil || strings.TrimSpace(*in.PresetID) == "" {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.preset_required", "presetId is required when creating a personal notification channel")
		}
		found, ok := personalNotificationPreset(*in.PresetID)
		if !ok {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.preset_not_found", "presetId must be one of the 6 supported personal webhook presets")
		}
		preset = found

		var count int64
		if err := personalNotificationChannels(db.Model(&model.NotificationChannel{}), user.ID).Count(&count).Error; err != nil {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if count >= personalNotificationChannelLimit {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.Conflict("notification.channel_limit_reached", "personal notification channel limit (10) reached")
		}
	} else {
		if in.PresetID != nil && strings.TrimSpace(*in.PresetID) != "" && strings.TrimSpace(*in.PresetID) != existing.PresetID {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.preset_immutable", "presetId cannot be changed after creation")
		}
		found, ok := personalNotificationPreset(existing.PresetID)
		if !ok {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.preset_not_found", "channel preset is invalid")
		}
		preset = found
	}

	var draft model.NotificationChannel
	if isCreate {
		draft = model.NotificationChannel{
			ID:          id.New("nch"),
			Scope:       "user",
			OwnerRef:    user.ID,
			OwnerUserID: user.ID,
			PresetID:    preset.ID,
			AdapterKind: preset.AdapterKind,
			ConfigJSON:  preset.ConfigTemplate,
			Enabled:     true,
			CreatedBy:   user.ID,
		}
	} else {
		draft = *existing
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || utf8.RuneCountInString(name) > personalNotificationNameMaxLength {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.channel_name_invalid", "personal notification channel name must contain 1 to 160 characters")
		}
		draft.Name = name
	} else if isCreate {
		return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.channel_name_invalid", "name is required when creating a personal notification channel")
	}

	if in.Enabled != nil {
		draft.Enabled = *in.Enabled
	}

	toSet, toClear, sensitiveKeys, err := normalizeSecretMutations(notification.AdapterKindWebhook, in.UpsertSecrets, in.ClearSecretKeys)
	if err != nil {
		return model.NotificationChannel{}, preset, nil, nil, nil, nil, err
	}

	requiredSet := make(map[string]bool, len(preset.SecretFields))
	allowedSet := make(map[string]bool, len(preset.SecretFields)+len(preset.OptionalSecretFields))
	for _, f := range preset.SecretFields {
		requiredSet[f] = true
		allowedSet[f] = true
	}
	for _, f := range preset.OptionalSecretFields {
		allowedSet[f] = true
	}

	for key, val := range toSet {
		if !allowedSet[key] {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.secret_field_invalid", fmt.Sprintf("secret key %q is not supported by preset %s", key, preset.ID))
		}
		if utf8.RuneCountInString(val) > personalNotificationSecretMaxLength {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.secret_too_long", "secret value exceeds maximum length")
		}
	}
	for _, key := range toClear {
		if !allowedSet[key] {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.secret_field_invalid", fmt.Sprintf("secret key %q is not supported by preset %s", key, preset.ID))
		}
		if requiredSet[key] {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.secret_required", fmt.Sprintf("required secret %q cannot be cleared", key))
		}
	}

	existingRefs := decodeStringMap(draft.SecretRefsJSON)
	for _, reqField := range preset.SecretFields {
		if strings.TrimSpace(toSet[reqField]) == "" && strings.TrimSpace(existingRefs[reqField]) == "" {
			return model.NotificationChannel{}, preset, nil, nil, nil, nil, aitool.BadRequest("notification.secret_required", fmt.Sprintf("required secret %q must be provided in upsertSecrets", reqField))
		}
	}

	var changes []aitool.PreviewChange
	if isCreate {
		changes = append(changes,
			aitool.PreviewChange{Field: "name", After: draft.Name, Action: "create"},
			aitool.PreviewChange{Field: "presetId", After: draft.PresetID, Action: "create"},
			aitool.PreviewChange{Field: "enabled", After: draft.Enabled, Action: "create"},
		)
	} else {
		if draft.Name != existing.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: draft.Name, Action: "update"})
		}
		if draft.Enabled != existing.Enabled {
			changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: draft.Enabled, Action: "update"})
		}
	}

	return draft, preset, toSet, toClear, sensitiveKeys, changes, nil
}

func (h *Handler) aiPreviewOpsSaveMyNotificationChannel(tc *aitool.ToolContext, in opsSaveMyNotificationChannelInput) (*aitool.PreviewResult, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var existing *model.NotificationChannel
	resolvedID := ""
	if in.Channel != nil && strings.TrimSpace(*in.Channel) != "" {
		found, resolveErr := resolvePersonalChannelForAI(db, user.ID, *in.Channel)
		if resolveErr != nil {
			return nil, resolveErr
		}
		existing = &found
		resolvedID = found.ID
	}

	draft, _, _, _, sensitiveKeys, changes, err := validatePersonalChannelDraft(db, user, existing, in)
	if err != nil {
		return nil, err
	}
	if err := validateNotificationChannel(tc.Context(), draft, h.secrets()); err != nil {
		return nil, aitool.BadRequest("notification.channel_invalid", err.Error())
	}

	summary := fmt.Sprintf("Create personal notification channel %q (%s)", draft.Name, draft.PresetID)
	if existing != nil {
		summary = fmt.Sprintf("Update personal notification channel %q (%s)", draft.Name, existing.ID)
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "personal_notification_channel",
			TargetID:       resolvedID,
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
		},
		LockedPayload: lockedPersonalChannelMutation{ResolvedChannelID: resolvedID},
	}, nil
}

func (h *Handler) aiExecuteOpsSaveMyNotificationChannel(tc *aitool.ToolContext, in opsSaveMyNotificationChannelInput) (notificationChannelResponse, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return notificationChannelResponse{}, err
	}
	db := h.aiDB(tc)

	var locked lockedPersonalChannelMutation
	_ = tc.UnmarshalLockedPayload(&locked)

	var existing *model.NotificationChannel
	if locked.ResolvedChannelID != "" {
		var found model.NotificationChannel
		if err := personalNotificationChannels(db, user.ID).First(&found, "id = ?", locked.ResolvedChannelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notificationChannelResponse{}, aitool.NotFound("notification.channel_not_found", "notification channel was not found")
			}
			return notificationChannelResponse{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		existing = &found
	} else if in.Channel != nil && strings.TrimSpace(*in.Channel) != "" {
		found, resolveErr := resolvePersonalChannelForAI(db, user.ID, *in.Channel)
		if resolveErr != nil {
			return notificationChannelResponse{}, resolveErr
		}
		existing = &found
	}

	draft, _, toSet, toClear, _, _, err := validatePersonalChannelDraft(db, user, existing, in)
	if err != nil {
		return notificationChannelResponse{}, err
	}
	if err := validateNotificationChannel(tc.Context(), draft, h.secrets()); err != nil {
		return notificationChannelResponse{}, aitool.BadRequest("notification.channel_invalid", err.Error())
	}

	if existing == nil {
		wantEnabled := draft.Enabled
		txErr := db.Transaction(func(tx *gorm.DB) error {
			lockKey := "personal_notification_channels:" + strings.TrimSpace(user.ID)
			if err := tx.Exec("select pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey).Error; err != nil {
				return err
			}
			var count int64
			if err := personalNotificationChannels(tx.Model(&model.NotificationChannel{}), user.ID).Count(&count).Error; err != nil {
				return err
			}
			if count >= personalNotificationChannelLimit {
				return errPersonalNotificationChannelLimit
			}
			secretRefs := make(map[string]string, len(toSet))
			for key, val := range toSet {
				ref, storeErr := h.secrets().StoreContextWithDB(tc.Context(), tx, val, user.ID, "notification_channel:"+draft.ID+":"+key)
				if storeErr != nil {
					return storeErr
				}
				secretRefs[key] = ref
			}
			draft.SecretRefsJSON = mustJSON(secretRefs)
			if err := tx.Create(&draft).Error; err != nil {
				return err
			}
			if draft.Enabled != wantEnabled {
				if err := tx.Model(&model.NotificationChannel{}).Where("id = ?", draft.ID).Update("enabled", wantEnabled).Error; err != nil {
					return err
				}
				draft.Enabled = wantEnabled
			}
			return nil
		})
		if errors.Is(txErr, errPersonalNotificationChannelLimit) {
			return notificationChannelResponse{}, aitool.Conflict("notification.channel_limit_reached", "personal notification channel limit reached")
		}
		if txErr != nil {
			return notificationChannelResponse{}, aitool.NewError(http.StatusInternalServerError, "notification.channel_save_failed", txErr.Error())
		}
		h.auditWithContext(user.ID, "notification.personal_channel.create", draft.ID, true, "", tc.Context())
		return notificationChannelResponseFor(draft), nil
	}

	txErr := db.Transaction(func(tx *gorm.DB) error {
		var lockedRow model.NotificationChannel
		if err := personalNotificationChannels(tx, user.ID).Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRow, "id = ?", existing.ID).Error; err != nil {
			return err
		}
		secretRefs := decodeStringMap(lockedRow.SecretRefsJSON)
		for _, clearKey := range toClear {
			if oldRef := strings.TrimSpace(secretRefs[clearKey]); oldRef != "" {
				resource := "notification_channel:" + lockedRow.ID + ":" + clearKey
				if err := h.secrets().DeleteRefContextWithDB(tc.Context(), tx, oldRef, resource); err != nil {
					return err
				}
			}
			delete(secretRefs, clearKey)
		}
		for key, val := range toSet {
			resource := "notification_channel:" + lockedRow.ID + ":" + key
			newRef, storeErr := h.secrets().StoreContextWithDB(tc.Context(), tx, val, user.ID, resource)
			if storeErr != nil {
				return storeErr
			}
			oldRef := strings.TrimSpace(secretRefs[key])
			secretRefs[key] = newRef
			if oldRef != "" && oldRef != newRef {
				if err := h.secrets().DeleteRefContextWithDB(tc.Context(), tx, oldRef, resource); err != nil {
					return err
				}
			}
		}
		lockedRow.Name = draft.Name
		lockedRow.Enabled = draft.Enabled
		lockedRow.SecretRefsJSON = mustJSON(secretRefs)
		if err := tx.Save(&lockedRow).Error; err != nil {
			return err
		}
		draft = lockedRow
		return nil
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return notificationChannelResponse{}, aitool.NotFound("notification.channel_not_found", "notification channel was not found")
	}
	if txErr != nil {
		return notificationChannelResponse{}, aitool.NewError(http.StatusInternalServerError, "notification.channel_save_failed", txErr.Error())
	}
	h.auditWithContext(user.ID, "notification.personal_channel.update", draft.ID, true, "", tc.Context())
	return notificationChannelResponseFor(draft), nil
}

func (h *Handler) aiPreviewOpsDeleteMyNotificationChannel(tc *aitool.ToolContext, in opsDeleteMyNotificationChannelInput) (*aitool.PreviewResult, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	channel, err := resolvePersonalChannelForAI(h.aiDB(tc), user.ID, in.Channel)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete personal notification channel %q (%s)", channel.Name, channel.ID),
			TargetResource: "personal_notification_channel",
			TargetID:       channel.ID,
			Changes: []aitool.PreviewChange{
				{Field: "channel", Before: channel.Name, Action: "delete"},
			},
		},
		LockedPayload: lockedPersonalChannelMutation{ResolvedChannelID: channel.ID},
	}, nil
}

func (h *Handler) aiExecuteOpsDeleteMyNotificationChannel(tc *aitool.ToolContext, in opsDeleteMyNotificationChannelInput) (map[string]any, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var locked lockedPersonalChannelMutation
	_ = tc.UnmarshalLockedPayload(&locked)
	channelID := strings.TrimSpace(locked.ResolvedChannelID)
	if channelID == "" {
		resolved, resolveErr := resolvePersonalChannelForAI(db, user.ID, in.Channel)
		if resolveErr != nil {
			return nil, resolveErr
		}
		channelID = resolved.ID
	}

	txErr := db.Transaction(func(tx *gorm.DB) error {
		var channel model.NotificationChannel
		if err := personalNotificationChannels(tx, user.ID).Clauses(clause.Locking{Strength: "UPDATE"}).First(&channel, "id = ?", channelID).Error; err != nil {
			return err
		}
		for key, ref := range decodeStringMap(channel.SecretRefsJSON) {
			if strings.TrimSpace(ref) == "" {
				continue
			}
			resource := "notification_channel:" + channel.ID + ":" + key
			if err := h.secrets().DeleteRefContextWithDB(tc.Context(), tx, ref, resource); err != nil {
				return err
			}
		}
		return tx.Delete(&channel).Error
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("notification.channel_not_found", "notification channel was not found")
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "notification.channel_delete_failed", txErr.Error())
	}

	h.auditWithContext(user.ID, "notification.personal_channel.delete", channelID, true, "", tc.Context())
	return map[string]any{
		"id":      channelID,
		"deleted": true,
	}, nil
}

func (h *Handler) aiOpsConfigureMyNotificationPreferences(tc *aitool.ToolContext, in opsConfigureMyNotificationPreferencesInput) (userNotificationPreferenceResponse, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return userNotificationPreferenceResponse{}, err
	}
	db := h.aiDB(tc)

	allowedEvents := notification.DefaultFailureEventTypes()
	allowedMap := make(map[string]bool, len(allowedEvents))
	for _, ev := range allowedEvents {
		allowedMap[ev] = true
	}

	addSet := map[string]bool{}
	for _, ev := range in.AddEventTypes {
		ev = strings.TrimSpace(ev)
		if ev == "" {
			continue
		}
		if !allowedMap[ev] {
			return userNotificationPreferenceResponse{}, aitool.BadRequest("notification.preference_event_types_invalid", fmt.Sprintf("unsupported personal failure event type %q", ev))
		}
		addSet[ev] = true
	}
	removeSet := map[string]bool{}
	for _, ev := range in.RemoveEventTypes {
		ev = strings.TrimSpace(ev)
		if ev == "" {
			continue
		}
		if !allowedMap[ev] {
			return userNotificationPreferenceResponse{}, aitool.BadRequest("notification.preference_event_types_invalid", fmt.Sprintf("unsupported personal failure event type %q", ev))
		}
		if addSet[ev] {
			return userNotificationPreferenceResponse{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("event type %q cannot appear in both addEventTypes and removeEventTypes", ev))
		}
		removeSet[ev] = true
	}

	pref := notification.DefaultUserNotificationPreference(user.ID)
	if err := db.First(&pref, "user_id = ?", user.ID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return userNotificationPreferenceResponse{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	emailEnabled := pref.EmailEnabled
	if in.EmailEnabled != nil {
		emailEnabled = *in.EmailEnabled
	}

	currentSet := map[string]bool{}
	for _, ev := range decodeStringList(pref.EventTypesJSON) {
		if allowedMap[ev] {
			currentSet[ev] = true
		}
	}
	for ev := range addSet {
		currentSet[ev] = true
	}
	for ev := range removeSet {
		delete(currentSet, ev)
	}

	mergedEvents := make([]string, 0, len(currentSet))
	for _, ev := range allowedEvents {
		if currentSet[ev] {
			mergedEvents = append(mergedEvents, ev)
		}
	}

	now := time.Now()
	saved := model.UserNotificationPreference{
		UserID:         user.ID,
		EmailEnabled:   emailEnabled,
		EventTypesJSON: notification.EncodeStringList(mergedEvents),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Model(&model.UserNotificationPreference{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"email_enabled", "event_types_json", "updated_at"}),
	}).Create(map[string]any{
		"user_id":          saved.UserID,
		"email_enabled":    saved.EmailEnabled,
		"event_types_json": saved.EventTypesJSON,
		"created_at":       saved.CreatedAt,
		"updated_at":       saved.UpdatedAt,
	}).Error; err != nil {
		return userNotificationPreferenceResponse{}, aitool.NewError(http.StatusInternalServerError, "notification.preference_save_failed", err.Error())
	}

	h.auditWithContext(user.ID, "notification.preference.update", user.ID, true, "", tc.Context())
	return myNotificationPreferenceResponse(saved), nil
}

func validateTestOrRetryChoice(channel, retryDeliveryID *string) (string, string, error) {
	ch := ""
	if channel != nil {
		ch = strings.TrimSpace(*channel)
	}
	dlv := ""
	if retryDeliveryID != nil {
		dlv = strings.TrimSpace(*retryDeliveryID)
	}
	if (ch == "" && dlv == "") || (ch != "" && dlv != "") {
		return "", "", aitool.BadRequest("argument_invalid", "exactly one of channel or retryDeliveryId must be provided")
	}
	return ch, dlv, nil
}

func (h *Handler) aiPreviewOpsTestOrRetryMyNotification(tc *aitool.ToolContext, in opsTestOrRetryMyNotificationInput) (*aitool.PreviewResult, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	chRef, deliveryID, err := validateTestOrRetryChoice(in.Channel, in.RetryDeliveryID)
	if err != nil {
		return nil, err
	}

	if chRef != "" {
		channel, resolveErr := resolvePersonalChannelForAI(db, user.ID, chRef)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Send test notification to personal channel %q (%s)", channel.Name, channel.PresetID),
				TargetResource: "personal_notification_channel",
				TargetID:       channel.ID,
				Impact:         []string{"Dispatches a real external test webhook message to your personal bot endpoint."},
			},
			LockedPayload: lockedPersonalChannelMutation{ResolvedChannelID: channel.ID},
		}, nil
	}

	var delivery model.NotificationDelivery
	if err := personalNotificationDeliveries(db, user.ID).First(&delivery, "id = ?", deliveryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("notification.delivery_not_found", "personal notification delivery was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if err := ensureDeliveryRetryable(delivery, time.Now()); err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Retry failed personal notification delivery %s (event=%s)", delivery.ID, delivery.EventType),
			TargetResource: "notification_delivery",
			TargetID:       delivery.ID,
			Changes: []aitool.PreviewChange{
				{Field: "status", Before: delivery.Status, After: "pending", Action: "retry"},
			},
		},
		LockedPayload: lockedPersonalChannelMutation{DeliveryID: delivery.ID},
	}, nil
}

func (h *Handler) aiExecuteOpsTestOrRetryMyNotification(tc *aitool.ToolContext, in opsTestOrRetryMyNotificationInput) (map[string]any, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	chRef, deliveryID, err := validateTestOrRetryChoice(in.Channel, in.RetryDeliveryID)
	if err != nil {
		return nil, err
	}

	var locked lockedPersonalChannelMutation
	_ = tc.UnmarshalLockedPayload(&locked)

	if chRef != "" {
		var channel model.NotificationChannel
		if locked.ResolvedChannelID != "" {
			if err := personalNotificationChannels(db, user.ID).First(&channel, "id = ?", locked.ResolvedChannelID).Error; err != nil {
				return nil, aitool.NotFound("notification.channel_not_found", "notification channel was not found")
			}
		} else {
			resolved, resolveErr := resolvePersonalChannelForAI(db, user.ID, chRef)
			if resolveErr != nil {
				return nil, resolveErr
			}
			channel = resolved
		}

		if !h.allowPersonalNotificationTest(tc.Gin, user.ID) {
			return nil, tc.AbortedError("notification.test_rate_limited", "personal notification test rate limit exceeded")
		}
		adapter, err := notification.DefaultRegistry().Adapter(channel.AdapterKind)
		if err != nil {
			return nil, aitool.BadRequest("notification.adapter_invalid", err.Error())
		}
		testCtx := notification.WithEgressPolicy(tc.Context(), notification.EgressPolicyForScope("user"))
		if err := adapter.Test(testCtx, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), h.secrets()); err != nil {
			h.auditWithContext(user.ID, "notification.personal_channel.test", channel.ID, false, "notification.channel_test_failed", tc.Context())
			return nil, aitool.NewError(http.StatusBadGateway, "notification.channel_test_failed", err.Error())
		}
		h.auditWithContext(user.ID, "notification.personal_channel.test", channel.ID, true, "", tc.Context())
		return map[string]any{
			"mode":      "test",
			"channelId": channel.ID,
			"status":    "ok",
		}, nil
	}

	targetDeliveryID := firstNonEmpty(locked.DeliveryID, deliveryID)
	var existing model.NotificationDelivery
	if err := personalNotificationDeliveries(db, user.ID).First(&existing, "id = ?", targetDeliveryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("notification.delivery_not_found", "personal notification delivery was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if err := ensureDeliveryRetryable(existing, time.Now()); err != nil {
		return nil, err
	}

	svc := notification.Service{DB: db, Enqueuer: h.host.DeliveryEnqueuer()}
	retried, retryErr := svc.RetryDelivery(tc.Context(), existing.ID)
	if errors.Is(retryErr, notification.ErrDeliveryActive) {
		return nil, aitool.Conflict("notification.delivery_not_retryable", retryErr.Error())
	}
	if retryErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "notification.delivery_retry_failed", retryErr.Error())
	}
	items := []model.NotificationDelivery{retried}
	h.enrichPersonalNotificationDeliveries(items, user, tc.Context())
	h.auditWithContext(user.ID, "notification.personal_delivery.retry", retried.ID, true, "", tc.Context())

	return map[string]any{
		"mode":       "retry",
		"deliveryId": items[0].ID,
		"status":     items[0].Status,
		"delivery":   items[0],
	}, nil
}
