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
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
)

const (
	personalEmailDigestMaxEvents           = 20
	notificationDeliveryRetryPendingStatus = "retry_pending"
)

var personalEmailDeliveryActiveStatuses = []string{"pending", "sending", "enqueue_failed"}

type personalEmailBatchOutcome struct {
	taskErr        error
	nextDigestTime time.Time
}

func loadPersonalDeliveryPolicy(ctx context.Context, db *gorm.DB, delivery model.NotificationDelivery) (notification.PersonalRecipientPolicy, string, error) {
	return notification.LoadPersonalRecipientPolicy(ctx, db, delivery.RecipientUserID)
}

func validatePersonalDeliveryEvent(
	ctx context.Context,
	db *gorm.DB,
	delivery model.NotificationDelivery,
	policy notification.PersonalRecipientPolicy,
) (notification.Event, string, error) {
	var storedEvent model.PlatformEvent
	if err := db.WithContext(ctx).First(&storedEvent, "id = ?", strings.TrimSpace(delivery.EventID)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notification.Event{}, notification.PersonalEventIntegrityInvalidCode, nil
		}
		return notification.Event{}, "", err
	}
	if strings.TrimSpace(delivery.EventID) == "" ||
		strings.TrimSpace(delivery.EventType) != strings.TrimSpace(storedEvent.Type) ||
		strings.TrimSpace(delivery.ProjectID) != strings.TrimSpace(storedEvent.ProjectID) {
		return notification.Event{}, notification.PersonalEventIntegrityInvalidCode, nil
	}

	var event notification.Event
	if err := json.Unmarshal([]byte(storedEvent.DetailJSON), &event); err != nil {
		return notification.Event{}, notification.PersonalEventIntegrityInvalidCode, nil
	}
	if strings.TrimSpace(event.ID) != strings.TrimSpace(storedEvent.ID) ||
		strings.TrimSpace(event.Type) != strings.TrimSpace(storedEvent.Type) ||
		strings.TrimSpace(event.Project.ID) != strings.TrimSpace(storedEvent.ProjectID) ||
		strings.TrimSpace(event.Actor.ID) != strings.TrimSpace(storedEvent.ActorID) ||
		strings.TrimSpace(event.ResourceOwnerUserID) != strings.TrimSpace(storedEvent.ResourceOwnerUserID) {
		return notification.Event{}, notification.PersonalEventIntegrityInvalidCode, nil
	}
	if code, err := policy.CheckEvent(ctx, db, storedEvent); err != nil {
		return notification.Event{}, "", err
	} else if code != "" {
		return notification.Event{}, code, nil
	}
	if code := policy.CheckAdapter(delivery.AdapterKind); code != "" {
		return notification.Event{}, code, nil
	}
	return event, "", nil
}

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
	if isPersonalEmailDelivery(delivery) {
		return r.handlePersonalEmailDelivery(ctx, delivery)
	}
	startedAt := time.Now()
	lease, claimed, err := claimNotificationDelivery(db, delivery, startedAt)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	personalDelivery := strings.TrimSpace(delivery.RecipientUserID) != ""
	var event notification.Event
	if personalDelivery {
		policy, code, err := loadPersonalDeliveryPolicy(ctx, db, delivery)
		if err != nil {
			return err
		}
		if code == "" {
			event, code, err = validatePersonalDeliveryEvent(ctx, db, delivery, policy)
			if err != nil {
				return err
			}
		}
		if code != "" {
			if _, err := markNotificationDeliveryLeaseFailed(db, lease, errors.New(code), 0, "", ""); err != nil {
				return err
			}
			return fmt.Errorf("%w: %s", asynq.SkipRetry, code)
		}
	}
	var channel model.NotificationChannel
	channelQuery := db.Where("id = ? and enabled = ?", delivery.ChannelID, true)
	if !personalDelivery {
		channelQuery = channelQuery.Where("owner_user_id = ?", "")
	} else {
		channelQuery = channelQuery.Where(
			"owner_user_id = ? and adapter_kind = ?",
			delivery.RecipientUserID,
			notification.AdapterKindWebhook,
		)
	}
	if err := channelQuery.First(&channel).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if _, updateErr := markNotificationDeliveryLeaseFailed(db, lease, notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_channel_unavailable", err), 0, "", ""); updateErr != nil {
			return updateErr
		}
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	if !personalDelivery {
		if err := json.Unmarshal([]byte(delivery.EventJSON), &event); err != nil {
			if _, updateErr := markNotificationDeliveryLeaseFailed(db, lease, notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_payload_invalid", err), 0, "", ""); updateErr != nil {
				return updateErr
			}
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
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
		if _, updateErr := markNotificationDeliveryLeaseFailed(db, lease, notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_adapter_invalid", err), 0, "", ""); updateErr != nil {
			return updateErr
		}
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	message, err := workerStageValue(ctx, "notification.render", func(stageCtx context.Context) (notification.RenderedMessage, error) {
		return adapter.Render(stageCtx, event, template, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), r.secrets, event.Locale)
	})
	if err != nil {
		if _, updateErr := markNotificationDeliveryLeaseFailed(db, lease, notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_render_failed", err), time.Since(startedAt), "", ""); updateErr != nil {
			return updateErr
		}
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	requestSnapshot := r.notificationRequestSnapshot(ctx, message, channel.SecretRefsJSON)
	result, err := workerStageValue(ctx, "notification.send", func(stageCtx context.Context) (notification.SendResult, error) {
		return adapter.Send(stageCtx, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), message, r.secrets)
	})
	if err != nil {
		storedErr := notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_send_failed", err)
		duration := time.Since(startedAt)
		responseSnippet := notificationDeliveryResponseSnippetForStorage(delivery, result.ResponseSnippet)
		skipRetry, updateErr := markNotificationDeliveryLeaseSendFailure(
			db,
			lease,
			result,
			err,
			storedErr,
			duration,
			requestSnapshot,
			responseSnippet,
		)
		if updateErr != nil {
			return updateErr
		}
		if skipRetry {
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
	if _, err := updateNotificationDeliveryLease(db, lease, updates); err != nil {
		return err
	}
	return nil
}

type notificationDeliveryLease struct {
	DeliveryID string
	Generation int
}

func claimNotificationDelivery(db *gorm.DB, delivery model.NotificationDelivery, startedAt time.Time) (notificationDeliveryLease, bool, error) {
	switch delivery.Status {
	case "pending", "enqueue_failed", notificationDeliveryRetryPendingStatus:
	case "sending":
		if !notificationDeliverySendingIsStale(delivery, startedAt) {
			return notificationDeliveryLease{}, false, nil
		}
	default:
		return notificationDeliveryLease{}, false, nil
	}

	query := db.Model(&model.NotificationDelivery{}).
		Where("id = ? and status = ? and attempt_count = ?", delivery.ID, delivery.Status, delivery.AttemptCount)
	if delivery.Status == "sending" {
		staleBefore := startedAt.Add(-tasks.PolicyForType(tasks.TypeNotificationDeliver).Timeout)
		query = query.Where(
			"(started_at is not null and started_at <= ?) or (started_at is null and (updated_at is null or updated_at <= ?))",
			staleBefore,
			staleBefore,
		)
	}
	result := query.Updates(map[string]any{
		"status":        "sending",
		"attempt_count": gorm.Expr("attempt_count + 1"),
		"started_at":    &startedAt,
		"finished_at":   nil,
		"error_message": "",
	})
	if result.Error != nil {
		return notificationDeliveryLease{}, false, result.Error
	}
	if result.RowsAffected != 1 {
		return notificationDeliveryLease{}, false, nil
	}
	return notificationDeliveryLease{DeliveryID: delivery.ID, Generation: delivery.AttemptCount + 1}, true, nil
}

func notificationDeliverySendingIsStale(delivery model.NotificationDelivery, now time.Time) bool {
	if delivery.Status != "sending" {
		return false
	}
	staleBefore := now.Add(-tasks.PolicyForType(tasks.TypeNotificationDeliver).Timeout)
	if delivery.StartedAt == nil {
		return !delivery.UpdatedAt.After(staleBefore)
	}
	return !delivery.StartedAt.After(staleBefore)
}

func updateNotificationDeliveryLease(db *gorm.DB, lease notificationDeliveryLease, updates map[string]any) (bool, error) {
	result := db.Model(&model.NotificationDelivery{}).
		Where("id = ? and status = ? and attempt_count = ?", lease.DeliveryID, "sending", lease.Generation).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *Runner) handlePersonalEmailDelivery(ctx context.Context, delivery model.NotificationDelivery) error {
	return r.handlePersonalEmailDeliveries(ctx, delivery.RecipientUserID, delivery.ID)
}

func (r *Runner) handleNotificationEmailDigest(ctx context.Context, task *asynq.Task) error {
	var payload tasks.NotificationEmailDigestPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	if strings.TrimSpace(payload.RecipientUserID) == "" || payload.DueAtUnix <= 0 {
		return fmt.Errorf("%w: invalid notification email digest payload", asynq.SkipRetry)
	}
	return r.handlePersonalEmailDeliveries(ctx, payload.RecipientUserID, "")
}

func (r *Runner) handlePersonalEmailDeliveries(ctx context.Context, recipientUserID string, triggerDeliveryID string) error {
	recipientUserID = strings.TrimSpace(recipientUserID)
	triggerDeliveryID = strings.TrimSpace(triggerDeliveryID)
	if recipientUserID == "" {
		return fmt.Errorf("%w: notification personal email recipient is required", asynq.SkipRetry)
	}
	var outcome personalEmailBatchOutcome
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"select pg_advisory_xact_lock(hashtextextended(?, 0))",
			"notification:personal-email:"+recipientUserID,
		).Error; err != nil {
			return err
		}
		var err error
		outcome, err = r.handlePersonalEmailDeliveriesLocked(ctx, tx, recipientUserID, triggerDeliveryID)
		return err
	})
	if err != nil {
		return err
	}
	if !outcome.nextDigestTime.IsZero() {
		if err := r.schedulePersonalEmailDigest(ctx, recipientUserID, outcome.nextDigestTime); err != nil {
			return err
		}
	}
	return outcome.taskErr
}

func (r *Runner) handlePersonalEmailDeliveriesLocked(ctx context.Context, tx *gorm.DB, recipientUserID string, triggerDeliveryID string) (personalEmailBatchOutcome, error) {
	var deliveries []model.NotificationDelivery
	if err := tx.Where(
		"recipient_user_id = ? and channel_id = ? and adapter_kind = ? and status in ?",
		recipientUserID,
		notification.UserEmailChannelID,
		notification.AdapterKindSMTP,
		personalEmailDeliveryActiveStatuses,
	).Order("queued_at asc, id asc").Limit(personalEmailDigestMaxEvents).Find(&deliveries).Error; err != nil {
		return personalEmailBatchOutcome{}, err
	}
	if len(deliveries) == 0 {
		return personalEmailBatchOutcome{}, nil
	}
	if triggerDeliveryID != "" {
		leads, err := personalEmailDeliveryTriggerLeads(tx, recipientUserID, triggerDeliveryID)
		if err != nil {
			return personalEmailBatchOutcome{}, err
		}
		if !leads {
			return personalEmailBatchOutcome{}, nil
		}
	}

	policy, code, err := notification.LoadPersonalRecipientPolicy(ctx, tx, recipientUserID)
	if err != nil {
		return personalEmailBatchOutcome{}, err
	}
	if code == "" {
		code = policy.CheckAdapter(notification.AdapterKindSMTP)
	}
	if code != "" {
		if updateErr := markPersonalEmailDeliveriesFailed(tx, deliveryIDs(deliveries), code, 0, "", ""); updateErr != nil {
			return personalEmailBatchOutcome{}, updateErr
		}
		return personalEmailOutcomeWithRemaining(tx, recipientUserID, time.Now(), nil)
	}

	validDeliveries := make([]model.NotificationDelivery, 0, len(deliveries))
	events := make([]notification.Event, 0, len(deliveries))
	for _, delivery := range deliveries {
		event, validationCode, validationErr := validatePersonalDeliveryEvent(ctx, tx, delivery, policy)
		if validationErr != nil {
			return personalEmailBatchOutcome{}, validationErr
		}
		if validationCode != "" {
			if updateErr := markPersonalEmailDeliveriesFailed(tx, []string{delivery.ID}, validationCode, 0, "", ""); updateErr != nil {
				return personalEmailBatchOutcome{}, updateErr
			}
			continue
		}
		validDeliveries = append(validDeliveries, delivery)
		events = append(events, event)
	}
	if len(validDeliveries) == 0 {
		return personalEmailOutcomeWithRemaining(tx, recipientUserID, time.Now(), nil)
	}

	cooldownResolver := r.personalEmailCooldown
	if cooldownResolver == nil {
		cooldownResolver = platformmail.PersonalEmailAggregationCooldown
	}
	cooldown, err := cooldownResolver(ctx, tx)
	if err != nil {
		return personalEmailBatchOutcome{}, err
	}
	if cooldown > 0 {
		var latest struct {
			StartedAt *time.Time
		}
		if err := tx.Model(&model.NotificationDelivery{}).
			Select("max(started_at) as started_at").
			Where(
				"recipient_user_id = ? and channel_id = ? and adapter_kind = ? and started_at is not null",
				recipientUserID,
				notification.UserEmailChannelID,
				notification.AdapterKindSMTP,
			).Scan(&latest).Error; err != nil {
			return personalEmailBatchOutcome{}, err
		}
		if latest.StartedAt != nil {
			nextAllowedAt := latest.StartedAt.Add(cooldown)
			if nextAllowedAt.After(time.Now()) {
				return personalEmailBatchOutcome{nextDigestTime: nextAllowedAt}, nil
			}
		}
	}

	startedAt := time.Now()
	ids := deliveryIDs(validDeliveries)
	if err := tx.Model(&model.NotificationDelivery{}).Where("id in ?", ids).Updates(map[string]any{
		"status":        "sending",
		"attempt_count": gorm.Expr("attempt_count + 1"),
		"started_at":    &startedAt,
		"finished_at":   nil,
		"error_message": "",
	}).Error; err != nil {
		return personalEmailBatchOutcome{}, err
	}

	message, err := workerStageValue(ctx, "notification.render", func(context.Context) (notification.RenderedMessage, error) {
		return notification.RenderPersonalEmailDigest(events, policy.User.Language)
	}, attribute.Int("notification.event_count", len(events)))
	if err != nil {
		if updateErr := markPersonalEmailDeliveriesFailed(tx, ids, "notification.personal_email_render_failed", time.Since(startedAt), "", ""); updateErr != nil {
			return personalEmailBatchOutcome{}, updateErr
		}
		return personalEmailOutcomeWithRemaining(tx, recipientUserID, time.Now(), fmt.Errorf("%w: %v", asynq.SkipRetry, err))
	}
	requestSnapshot := r.notificationRequestSnapshot(ctx, message, "{}")
	result, err := workerStageValue(ctx, "notification.send", func(stageCtx context.Context) (notification.SendResult, error) {
		if r.personalEmailSender != nil {
			return r.personalEmailSender(stageCtx, policy.User.Email, message)
		}
		return platformmail.Send(stageCtx, tx, r.secrets.WithDB(tx), policy.User.Email, message)
	}, attribute.Int("notification.event_count", len(events)))
	if err != nil {
		storedError := personalEmailDeliveryError(err).Error()
		duration := time.Since(startedAt)
		if notificationSendErrorShouldSkipRetry(result, err) {
			if updateErr := markPersonalEmailDeliveriesFailed(tx, ids, storedError, duration, requestSnapshot, result.ResponseSnippet); updateErr != nil {
				return personalEmailBatchOutcome{}, updateErr
			}
			return personalEmailOutcomeWithRemaining(tx, recipientUserID, startedAt.Add(cooldown), fmt.Errorf("%w: %v", asynq.SkipRetry, err))
		}
		failedIDs, pendingIDs, enqueueFailedIDs := personalEmailFailureDeliveryIDs(validDeliveries)
		if updateErr := markPersonalEmailDeliveriesFailed(tx, failedIDs, storedError, duration, requestSnapshot, result.ResponseSnippet); updateErr != nil {
			return personalEmailBatchOutcome{}, updateErr
		}
		if updateErr := markPersonalEmailDeliveriesRetryable(tx, pendingIDs, "pending", storedError, duration, requestSnapshot, result.ResponseSnippet); updateErr != nil {
			return personalEmailBatchOutcome{}, updateErr
		}
		if updateErr := markPersonalEmailDeliveriesRetryable(tx, enqueueFailedIDs, "enqueue_failed", storedError, duration, requestSnapshot, result.ResponseSnippet); updateErr != nil {
			return personalEmailBatchOutcome{}, updateErr
		}
		if len(pendingIDs)+len(enqueueFailedIDs) > 0 {
			return personalEmailBatchOutcome{taskErr: err}, nil
		}
		return personalEmailOutcomeWithRemaining(tx, recipientUserID, startedAt.Add(cooldown), fmt.Errorf("%w: %v", asynq.SkipRetry, err))
	}
	finishedAt := time.Now()
	if err := tx.Model(&model.NotificationDelivery{}).Where("id in ?", ids).Updates(notificationDeliverySucceededUpdates(
		time.Since(startedAt),
		requestSnapshot,
		result.ResponseSnippet,
		finishedAt,
	)).Error; err != nil {
		return personalEmailBatchOutcome{}, err
	}
	return personalEmailOutcomeWithRemaining(tx, recipientUserID, startedAt.Add(cooldown), nil)
}

func personalEmailDeliveryTriggerLeads(tx *gorm.DB, recipientUserID string, triggerDeliveryID string) (bool, error) {
	scope := func() *gorm.DB {
		return tx.Model(&model.NotificationDelivery{}).Where(
			"recipient_user_id = ? and channel_id = ? and adapter_kind = ?",
			recipientUserID,
			notification.UserEmailChannelID,
			notification.AdapterKindSMTP,
		)
	}
	var trigger model.NotificationDelivery
	if err := scope().Where("id = ?", triggerDeliveryID).First(&trigger).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	var activeLeader model.NotificationDelivery
	err := scope().Where("status in ?", []string{"pending", "sending"}).
		Order("queued_at asc, id asc").First(&activeLeader).Error
	if err == nil {
		return activeLeader.ID == triggerDeliveryID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	if trigger.Status != "succeeded" {
		return false, nil
	}

	var lastSucceededLeader model.NotificationDelivery
	err = scope().Where("status = ? and finished_at is not null", "succeeded").
		Order("finished_at desc, queued_at asc, id asc").First(&lastSucceededLeader).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return lastSucceededLeader.ID == triggerDeliveryID, nil
}

func personalEmailOutcomeWithRemaining(tx *gorm.DB, recipientUserID string, nextDigestTime time.Time, taskErr error) (personalEmailBatchOutcome, error) {
	var remaining int64
	if err := tx.Model(&model.NotificationDelivery{}).Where(
		"recipient_user_id = ? and channel_id = ? and adapter_kind = ? and status in ?",
		recipientUserID,
		notification.UserEmailChannelID,
		notification.AdapterKindSMTP,
		personalEmailDeliveryActiveStatuses,
	).Count(&remaining).Error; err != nil {
		return personalEmailBatchOutcome{}, err
	}
	outcome := personalEmailBatchOutcome{taskErr: taskErr}
	if remaining > 0 {
		outcome.nextDigestTime = nextDigestTime
	}
	return outcome, nil
}

func personalEmailFailureDeliveryIDs(deliveries []model.NotificationDelivery) (failedIDs []string, pendingIDs []string, enqueueFailedIDs []string) {
	maxAttempts := tasks.PolicyForType(tasks.TypeNotificationEmailDigest).MaxRetry + 1
	for _, delivery := range deliveries {
		if delivery.AttemptCount+1 >= maxAttempts {
			failedIDs = append(failedIDs, delivery.ID)
		} else if delivery.Status == "enqueue_failed" {
			enqueueFailedIDs = append(enqueueFailedIDs, delivery.ID)
		} else {
			pendingIDs = append(pendingIDs, delivery.ID)
		}
	}
	return failedIDs, pendingIDs, enqueueFailedIDs
}

func (r *Runner) schedulePersonalEmailDigest(ctx context.Context, recipientUserID string, dueAt time.Time) error {
	if dueAt.Nanosecond() != 0 {
		dueAt = dueAt.Truncate(time.Second).Add(time.Second)
	}
	now := time.Now()
	if !dueAt.After(now) {
		dueAt = now.Truncate(time.Second).Add(time.Second)
	}
	payload := tasks.NotificationEmailDigestPayload{RecipientUserID: recipientUserID, DueAtUnix: dueAt.Unix()}
	var err error
	if r.enqueueEmailDigest != nil {
		_, err = r.enqueueEmailDigest(ctx, payload)
	} else if r.taskClient != nil {
		_, err = r.taskClient.EnqueueNotificationEmailDigest(ctx, payload)
	} else {
		return errors.New("notification email digest queue is unavailable")
	}
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func deliveryIDs(deliveries []model.NotificationDelivery) []string {
	ids := make([]string, 0, len(deliveries))
	for _, delivery := range deliveries {
		ids = append(ids, delivery.ID)
	}
	return ids
}

func markPersonalEmailDeliveriesFailed(tx *gorm.DB, ids []string, errorCode string, duration time.Duration, requestSnapshot string, responseSnippet string) error {
	if len(ids) == 0 {
		return nil
	}
	finishedAt := time.Now()
	updates := map[string]any{
		"status":          "failed",
		"duration_millis": duration.Milliseconds(),
		"error_message":   errorCode,
		"finished_at":     &finishedAt,
	}
	if requestSnapshot != "" {
		updates["request_snapshot"] = requestSnapshot
	}
	if responseSnippet != "" {
		updates["response_snippet"] = responseSnippet
	}
	return tx.Model(&model.NotificationDelivery{}).Where("id in ?", ids).Updates(updates).Error
}

func markPersonalEmailDeliveriesRetryable(tx *gorm.DB, ids []string, status string, errorCode string, duration time.Duration, requestSnapshot string, responseSnippet string) error {
	if len(ids) == 0 {
		return nil
	}
	updates := map[string]any{
		"status":          status,
		"duration_millis": duration.Milliseconds(),
		"error_message":   errorCode,
		"finished_at":     nil,
	}
	if requestSnapshot != "" {
		updates["request_snapshot"] = requestSnapshot
	}
	if responseSnippet != "" {
		updates["response_snippet"] = responseSnippet
	}
	return tx.Model(&model.NotificationDelivery{}).Where("id in ?", ids).Updates(updates).Error
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

func markNotificationDeliveryLeaseFailed(db *gorm.DB, lease notificationDeliveryLease, err error, duration time.Duration, requestSnapshot string, responseSnippet string) (bool, error) {
	finishedAt := time.Now()
	updates := notificationDeliveryLeaseFailureUpdates(err, duration, requestSnapshot, responseSnippet)
	updates["status"] = "failed"
	updates["finished_at"] = &finishedAt
	return updateNotificationDeliveryLease(db, lease, updates)
}

func markNotificationDeliveryLeaseRetryPending(db *gorm.DB, lease notificationDeliveryLease, err error, duration time.Duration, requestSnapshot string, responseSnippet string) (bool, error) {
	updates := notificationDeliveryLeaseFailureUpdates(err, duration, requestSnapshot, responseSnippet)
	updates["status"] = notificationDeliveryRetryPendingStatus
	updates["finished_at"] = nil
	return updateNotificationDeliveryLease(db, lease, updates)
}

func markNotificationDeliveryLeaseSendFailure(
	db *gorm.DB,
	lease notificationDeliveryLease,
	result notification.SendResult,
	sendErr error,
	storedErr error,
	duration time.Duration,
	requestSnapshot string,
	responseSnippet string,
) (bool, error) {
	if notificationSendErrorShouldSkipRetry(result, sendErr) {
		_, err := markNotificationDeliveryLeaseFailed(db, lease, storedErr, duration, requestSnapshot, responseSnippet)
		return true, err
	}
	_, err := markNotificationDeliveryLeaseRetryPending(db, lease, storedErr, duration, requestSnapshot, responseSnippet)
	return false, err
}

func notificationDeliveryLeaseFailureUpdates(err error, duration time.Duration, requestSnapshot string, responseSnippet string) map[string]any {
	updates := map[string]any{
		"duration_millis": duration.Milliseconds(),
		"error_message":   err.Error(),
	}
	if requestSnapshot != "" {
		updates["request_snapshot"] = requestSnapshot
	}
	if responseSnippet != "" {
		updates["response_snippet"] = responseSnippet
	}
	return updates
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
