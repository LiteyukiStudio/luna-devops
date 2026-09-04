package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
)

const (
	notificationReconcileBatchSize  = 100
	notificationReconcilePendingAge = 5 * time.Minute
)

func (r *Runner) handleNotificationReconcile(ctx context.Context, task *asynq.Task) error {
	var payload tasks.NotificationReconcilePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("%w: invalid notification reconcile payload: %v", asynq.SkipRetry, err)
	}
	return workerStage(ctx, "notification.reconcile", func(stageCtx context.Context) error {
		return r.reconcileNotificationDeliveries(stageCtx, time.Now())
	})
}

func (r *Runner) reconcileNotificationDeliveries(ctx context.Context, now time.Time) error {
	if r.notificationDeliveryEnqueuer == nil {
		return errors.New("notification delivery queue is unavailable")
	}
	db := r.db.WithContext(ctx)
	service := notification.Service{DB: r.db, Enqueuer: r.notificationDeliveryEnqueuer}
	fanoutErr := r.reconcileNotificationFanoutEvents(ctx, service)
	pendingCutoff := now.Add(-notificationReconcilePendingAge)
	sendingCutoff := now.Add(-tasks.PolicyForType(tasks.TypeNotificationDeliver).Timeout)
	var deliveries []model.NotificationDelivery
	if err := db.Where(
		`status = ?
			or (status = ? and (updated_at is null or updated_at <= ?))
			or (status = ? and (updated_at is null or updated_at <= ?))
			or (status = ? and (
				(started_at is not null and started_at <= ?)
				or (started_at is null and (updated_at is null or updated_at <= ?))
			))`,
		"enqueue_failed",
		"pending",
		pendingCutoff,
		notificationDeliveryRetryPendingStatus,
		pendingCutoff,
		"sending",
		sendingCutoff,
		sendingCutoff,
	).Order("queued_at asc nulls first, created_at asc nulls first, id asc").
		Limit(notificationReconcileBatchSize).
		Find(&deliveries).Error; err != nil {
		return errors.Join(fanoutErr, err)
	}

	errList := make([]error, 0)
	for _, delivery := range deliveries {
		claimQuery := db.Model(&model.NotificationDelivery{}).
			Where("id = ? and status = ? and attempt_count = ?", delivery.ID, delivery.Status, delivery.AttemptCount)
		switch delivery.Status {
		case "enqueue_failed":
		case "pending", notificationDeliveryRetryPendingStatus:
			claimQuery = claimQuery.Where("updated_at is null or updated_at <= ?", pendingCutoff)
		case "sending":
			claimQuery = claimQuery.Where(
				`(started_at is not null and started_at <= ?)
					or (started_at is null and (updated_at is null or updated_at <= ?))`,
				sendingCutoff,
				sendingCutoff,
			)
		default:
			continue
		}
		claim := claimQuery.Updates(map[string]any{
			"status":        "pending",
			"error_message": "",
			"started_at":    nil,
			"finished_at":   nil,
		})
		if claim.Error != nil {
			errList = append(errList, fmt.Errorf("claim notification delivery %s: %w", delivery.ID, claim.Error))
			continue
		}
		if claim.RowsAffected == 0 {
			continue
		}

		delivery.Status = "pending"
		delivery.ErrorMessage = ""
		delivery.StartedAt = nil
		delivery.FinishedAt = nil
		if enqueueErr := service.EnqueueDeliveries(ctx, []model.NotificationDelivery{delivery}); enqueueErr != nil {
			errList = append(errList, fmt.Errorf("enqueue notification delivery %s: %w", delivery.ID, enqueueErr))
		}
	}
	return errors.Join(fanoutErr, errors.Join(errList...))
}

func (r *Runner) reconcileNotificationFanoutEvents(ctx context.Context, service notification.Service) error {
	var events []model.PlatformEvent
	if err := r.db.WithContext(ctx).
		Where("notification_fanout_status = ?", notification.NotificationFanoutStatusPending).
		Order("occurred_at asc, id asc").
		Limit(notificationReconcileBatchSize).
		Find(&events).Error; err != nil {
		return err
	}

	errList := make([]error, 0)
	for _, event := range events {
		if _, err := service.ReconcileEvent(ctx, event.ID); err != nil {
			errList = append(errList, fmt.Errorf("reconcile notification event %s fanout: %w", event.ID, err))
		}
	}
	return errors.Join(errList...)
}
