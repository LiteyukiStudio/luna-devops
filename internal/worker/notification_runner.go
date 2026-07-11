package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
)

func (r *Runner) handleNotificationDeliver(ctx context.Context, task *asynq.Task) error {
	var payload tasks.NotificationDeliverPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	var delivery model.NotificationDelivery
	if err := r.db.First(&delivery, "id = ?", payload.DeliveryID).Error; err != nil {
		return err
	}
	if delivery.Status == "succeeded" {
		return nil
	}
	var channel model.NotificationChannel
	if err := r.db.First(&channel, "id = ? and enabled = ?", delivery.ChannelID, true).Error; err != nil {
		_ = r.markNotificationDeliveryFailed(delivery, err, 0, "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	var event notification.Event
	if err := json.Unmarshal([]byte(delivery.EventJSON), &event); err != nil {
		_ = r.markNotificationDeliveryFailed(delivery, err, 0, "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	template := notification.Template{}
	if delivery.TemplateID != "" {
		var modelTemplate model.NotificationTemplate
		if err := r.db.First(&modelTemplate, "id = ? and enabled = ?", delivery.TemplateID, true).Error; err == nil {
			template = notification.TemplateFromModel(modelTemplate)
		}
	}
	if template == (notification.Template{}) {
		template = notification.TemplateFromModel(notification.DefaultTemplateForChannel(channel, event, event.Locale))
	}
	registry := notification.DefaultRegistry()
	adapter, err := registry.Adapter(channel.AdapterKind)
	if err != nil {
		_ = r.markNotificationDeliveryFailed(delivery, err, 0, "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	startedAt := time.Now()
	_ = r.db.Model(&delivery).Updates(map[string]any{
		"status":        "sending",
		"attempt_count": delivery.AttemptCount + 1,
		"started_at":    &startedAt,
	}).Error
	message, err := adapter.Render(ctx, event, template, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), r.secrets, event.Locale)
	if err != nil {
		_ = r.markNotificationDeliveryFailed(delivery, err, time.Since(startedAt), "", "")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	requestSnapshot := r.notificationRequestSnapshot(message, channel.SecretRefsJSON)
	result, err := adapter.Send(ctx, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), message, r.secrets)
	if err != nil {
		_ = r.markNotificationDeliveryFailed(delivery, err, time.Since(startedAt), requestSnapshot, result.ResponseSnippet)
		if notificationSendErrorShouldSkipRetry(result) {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		return err
	}
	finishedAt := time.Now()
	updates := notificationDeliverySucceededUpdates(time.Since(startedAt), requestSnapshot, result.ResponseSnippet, finishedAt)
	if err := r.db.Model(&delivery).Updates(updates).Error; err != nil {
		return err
	}
	return r.db.Model(&channel).Updates(map[string]any{
		"last_delivery_status": "succeeded",
		"last_delivery_error":  "",
		"last_delivered_at":    &finishedAt,
	}).Error
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

func (r *Runner) markNotificationDeliveryFailed(delivery model.NotificationDelivery, err error, duration time.Duration, requestSnapshot string, responseSnippet string) error {
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
	if updateErr := r.db.Model(&delivery).Updates(updates).Error; updateErr != nil {
		return updateErr
	}
	return r.db.Model(&model.NotificationChannel{}).Where("id = ?", delivery.ChannelID).Updates(map[string]any{
		"last_delivery_status": "failed",
		"last_delivery_error":  err.Error(),
	}).Error
}

func notificationSendErrorShouldSkipRetry(result notification.SendResult) bool {
	return result.StatusCode >= 400 && result.StatusCode < 500 && result.StatusCode != 429
}

func (r *Runner) notificationRequestSnapshot(message notification.RenderedMessage, secretRefsJSON string) string {
	redactor := notificationRedactor(r.secrets, secretRefsJSON)
	snapshot := map[string]any{
		"method":  message.Method,
		"url":     redactor(message.URL),
		"headers": redactStringMap(message.Headers, redactor),
		"subject": message.Subject,
	}
	data, _ := json.Marshal(snapshot)
	return string(data)
}

func notificationRedactor(resolver interface{ Resolve(string) string }, secretRefsJSON string) func(string) string {
	refs := map[string]string{}
	_ = json.Unmarshal([]byte(secretRefsJSON), &refs)
	secrets := make([]string, 0, len(refs))
	for _, ref := range refs {
		if value := resolver.Resolve(ref); value != "" {
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
