package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/platformmail"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
)

func (r *Runner) handleNotificationDeliver(ctx context.Context, task *asynq.Task) error {
	var payload tasks.NotificationDeliverPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	db := r.db.WithContext(ctx)
	var delivery model.NotificationDelivery
	if err := db.First(&delivery, "id = ?", payload.DeliveryID).Error; err != nil {
		return err
	}
	if delivery.Status == "succeeded" {
		return nil
	}
	if isPersonalEmailDelivery(delivery) {
		return r.handlePersonalEmailDelivery(ctx, delivery)
	}
	var channel model.NotificationChannel
	channelQuery := db.Where("id = ? and enabled = ?", delivery.ChannelID, true)
	if strings.TrimSpace(delivery.RecipientUserID) == "" {
		channelQuery = channelQuery.Where("owner_user_id = ?", "")
	} else {
		channelQuery = channelQuery.Where(
			"owner_user_id = ? and adapter_kind = ?",
			delivery.RecipientUserID,
			notification.AdapterKindWebhook,
		)
	}
	if err := channelQuery.First(&channel).Error; err != nil {
		_ = r.markNotificationDeliveryFailed(ctx, delivery, notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_channel_unavailable", err), 0, "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	var event notification.Event
	if err := json.Unmarshal([]byte(delivery.EventJSON), &event); err != nil {
		_ = r.markNotificationDeliveryFailed(ctx, delivery, notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_payload_invalid", err), 0, "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	template := notification.Template{}
	if delivery.RecipientUserID == "" && delivery.TemplateID != "" {
		var modelTemplate model.NotificationTemplate
		if err := db.First(&modelTemplate, "id = ? and enabled = ?", delivery.TemplateID, true).Error; err == nil {
			template = notification.TemplateFromModel(modelTemplate)
		}
	}
	if template == (notification.Template{}) {
		template = notification.TemplateFromModel(notification.DefaultTemplateForChannel(channel, event, event.Locale))
	}
	registry := notification.DefaultRegistry()
	adapter, err := registry.Adapter(channel.AdapterKind)
	if err != nil {
		_ = r.markNotificationDeliveryFailed(ctx, delivery, notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_adapter_invalid", err), 0, "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	startedAt := time.Now()
	_ = db.Model(&delivery).Updates(map[string]any{
		"status":        "sending",
		"attempt_count": delivery.AttemptCount + 1,
		"started_at":    &startedAt,
	}).Error
	message, err := workerStageValue(ctx, "notification.render", func(stageCtx context.Context) (notification.RenderedMessage, error) {
		return adapter.Render(stageCtx, event, template, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), r.secrets, event.Locale)
	})
	if err != nil {
		_ = r.markNotificationDeliveryFailed(ctx, delivery, notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_render_failed", err), time.Since(startedAt), "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	requestSnapshot := r.notificationRequestSnapshot(ctx, message, channel.SecretRefsJSON)
	result, err := workerStageValue(ctx, "notification.send", func(stageCtx context.Context) (notification.SendResult, error) {
		return adapter.Send(stageCtx, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), message, r.secrets)
	})
	if err != nil {
		_ = r.markNotificationDeliveryFailed(
			ctx,
			delivery,
			notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_send_failed", err),
			time.Since(startedAt),
			requestSnapshot,
			notificationDeliveryResponseSnippetForStorage(delivery, result.ResponseSnippet),
		)
		if notificationSendErrorShouldSkipRetry(result, err) {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		return err
	}
	finishedAt := time.Now()
	updates := notificationDeliverySucceededUpdates(
		time.Since(startedAt),
		requestSnapshot,
		notificationDeliveryResponseSnippetForStorage(delivery, result.ResponseSnippet),
		finishedAt,
	)
	if err := db.Model(&delivery).Updates(updates).Error; err != nil {
		return err
	}
	return nil
}

func (r *Runner) handlePersonalEmailDelivery(ctx context.Context, delivery model.NotificationDelivery) error {
	db := r.db.WithContext(ctx)
	var user model.User
	if err := db.First(&user, "id = ? and disabled = ?", delivery.RecipientUserID, false).Error; err != nil {
		_ = r.markNotificationDeliveryFailed(ctx, delivery, errors.New("notification.personal_email_recipient_unavailable"), 0, "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	var event notification.Event
	if err := json.Unmarshal([]byte(delivery.EventJSON), &event); err != nil {
		_ = r.markNotificationDeliveryFailed(ctx, delivery, errors.New("notification.personal_email_payload_invalid"), 0, "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}

	startedAt := time.Now()
	_ = db.Model(&delivery).Updates(map[string]any{
		"status":        "sending",
		"attempt_count": delivery.AttemptCount + 1,
		"started_at":    &startedAt,
	}).Error
	template := notification.TemplateFromModel(notification.DefaultTemplateFor(notification.AdapterKindSMTP, event.Type, event.Locale))
	message, err := workerStageValue(ctx, "notification.render", func(stageCtx context.Context) (notification.RenderedMessage, error) {
		return (notification.SMTPAdapter{}).Render(stageCtx, event, template, nil, nil, r.secrets, event.Locale)
	})
	if err != nil {
		_ = r.markNotificationDeliveryFailed(ctx, delivery, errors.New("notification.personal_email_render_failed"), time.Since(startedAt), "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	requestSnapshot := r.notificationRequestSnapshot(ctx, message, "{}")
	result, err := workerStageValue(ctx, "notification.send", func(stageCtx context.Context) (notification.SendResult, error) {
		if r.personalEmailSender != nil {
			return r.personalEmailSender(stageCtx, user.Email, message)
		}
		return platformmail.Send(stageCtx, r.db, r.secrets, user.Email, message)
	})
	if err != nil {
		_ = r.markNotificationDeliveryFailed(ctx, delivery, personalEmailDeliveryError(err), time.Since(startedAt), requestSnapshot, result.ResponseSnippet)
		if notificationSendErrorShouldSkipRetry(result, err) {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		return err
	}
	finishedAt := time.Now()
	return db.Model(&delivery).Updates(notificationDeliverySucceededUpdates(
		time.Since(startedAt),
		requestSnapshot,
		result.ResponseSnippet,
		finishedAt,
	)).Error
}

func personalEmailDeliveryError(err error) error {
	switch {
	case errors.Is(err, platformmail.ErrInvalidSettings):
		return errors.New("notification.personal_email_not_configured")
	case errors.Is(err, platformmail.ErrInvalidRecipient):
		return errors.New("notification.personal_email_recipient_invalid")
	default:
		return errors.New("notification.personal_email_send_failed")
	}
}

func notificationDeliveryErrorForStorage(delivery model.NotificationDelivery, personalCode string, err error) error {
	if strings.TrimSpace(delivery.RecipientUserID) == "" {
		return err
	}
	return errors.New(personalCode)
}

func notificationDeliveryResponseSnippetForStorage(delivery model.NotificationDelivery, responseSnippet string) string {
	if strings.TrimSpace(delivery.RecipientUserID) != "" {
		return ""
	}
	return responseSnippet
}

func isPersonalEmailDelivery(delivery model.NotificationDelivery) bool {
	return delivery.ChannelID == notification.UserEmailChannelID &&
		delivery.AdapterKind == notification.AdapterKindSMTP &&
		strings.TrimSpace(delivery.RecipientUserID) != ""
}

func notificationDeliverySucceededUpdates(duration time.Duration, requestSnapshot string, responseSnippet string, finishedAt time.Time) map[string]any {
	return map[string]any{
		"status":           "succeeded",
		"duration_millis":  duration.Milliseconds(),
		"error_message":    "",
		"request_snapshot": requestSnapshot,
		"response_snippet": responseSnippet,
		"finished_at":      &finishedAt,
	}
}

func (r *Runner) markNotificationDeliveryFailed(ctx context.Context, delivery model.NotificationDelivery, err error, duration time.Duration, requestSnapshot string, responseSnippet string) error {
	finishedAt := time.Now()
	updates := map[string]any{
		"status":          "failed",
		"duration_millis": duration.Milliseconds(),
		"error_message":   err.Error(),
		"finished_at":     &finishedAt,
	}
	if requestSnapshot != "" {
		updates["request_snapshot"] = requestSnapshot
	}
	if responseSnippet != "" {
		updates["response_snippet"] = responseSnippet
	}
	if updateErr := r.db.WithContext(ctx).Model(&delivery).Updates(updates).Error; updateErr != nil {
		return updateErr
	}
	return nil
}

func notificationSendErrorShouldSkipRetry(result notification.SendResult, err error) bool {
	return (result.StatusCode >= 400 && result.StatusCode < 500 && result.StatusCode != 429) ||
		errors.Is(err, platformmail.ErrInvalidSettings) ||
		errors.Is(err, platformmail.ErrInvalidRecipient)
}

func (r *Runner) notificationRequestSnapshot(ctx context.Context, message notification.RenderedMessage, secretRefsJSON string) string {
	redactor := notificationRedactor(ctx, r.secrets, secretRefsJSON)
	snapshot := map[string]any{
		"method":  message.Method,
		"url":     redactor(message.URL),
		"headers": redactStringMap(message.Headers, redactor),
		"subject": message.Subject,
	}
	data, _ := json.Marshal(snapshot)
	return string(data)
}

func notificationRedactor(ctx context.Context, resolver interface {
	ResolveContext(context.Context, string) string
}, secretRefsJSON string) func(string) string {
	refs := map[string]string{}
	_ = json.Unmarshal([]byte(secretRefsJSON), &refs)
	secrets := make([]string, 0, len(refs))
	for _, ref := range refs {
		if value := resolver.ResolveContext(ctx, ref); value != "" {
			secrets = append(secrets, value)
		}
	}
	return func(value string) string {
		for _, secretValue := range secrets {
			value = strings.ReplaceAll(value, secretValue, "***")
		}
		return value
	}
}

func redactStringMap(values map[string]string, redactor func(string) string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = redactor(value)
	}
	return out
}
