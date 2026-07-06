package worker

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

const runtimeBillingLookbackHours = 6

func (r *Runner) handleBillingRuntime(ctx context.Context, task *asynq.Task) error {
	log.Printf("received task type=%s payload=%s", task.Type(), string(task.Payload()))
	return r.settleRuntimeUsageWindows(ctx, time.Now())
}

func (r *Runner) settleRuntimeUsageWindows(ctx context.Context, now time.Time) error {
	if r.db == nil {
		return nil
	}
	windows := completedHourlyWindows(now, runtimeBillingLookbackHours)
	if len(windows) == 0 {
		return nil
	}
	var targets []model.DeploymentTarget
	if err := r.db.
		Joins("join projects on projects.id = deployment_targets.project_id").
		Where("deployment_targets.enabled = ? and deployment_targets.delete_status in ? and projects.system_key = ?", true, []string{"active", ""}, "").
		Order("deployment_targets.created_at asc").
		Find(&targets).Error; err != nil {
		return err
	}
	service := billing.Service{DB: r.db}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.settleStorageUsageForTarget(ctx, service, target, windows)
		environment, release, ok := r.runtimeBillingTargetContext(target)
		if !ok {
			continue
		}
		releaseStart := runtimeBillingReleaseStart(release)
		for _, window := range windows {
			periodStart, periodEnd, ok := runtimeBillingEffectivePeriod(window.Start, window.End, target.CreatedAt, releaseStart)
			if !ok {
				continue
			}
			err := service.SettleRuntimeTargetWindow(billing.RuntimeUsageInput{
				ProjectID:          target.ProjectID,
				ApplicationID:      target.ApplicationID,
				DeploymentTargetID: target.ID,
				Environment:        environment,
				PeriodStart:        periodStart,
				PeriodEnd:          periodEnd,
				ActorID:            "system",
			})
			if err != nil && !errors.Is(err, billing.ErrAlreadySettled) {
				log.Printf("runtime billing settlement skipped target=%s window=%s: %v", target.ID, window.Start.Format(time.RFC3339), err)
			}
		}
	}
	return nil
}

func (r *Runner) settleStorageUsageForTarget(ctx context.Context, service billing.Service, target model.DeploymentTarget, windows []hourlyWindow) {
	if !target.DataRetentionEnabled {
		return
	}
	for _, window := range windows {
		if err := ctx.Err(); err != nil {
			return
		}
		periodStart, periodEnd, ok := storageBillingEffectivePeriod(window.Start, window.End, target.CreatedAt)
		if !ok {
			continue
		}
		err := service.SettleStorageTargetWindow(billing.StorageUsageInput{
			Target:      target,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
			ActorID:     "system",
		})
		if err != nil && !errors.Is(err, billing.ErrAlreadySettled) {
			log.Printf("storage billing settlement skipped target=%s window=%s: %v", target.ID, window.Start.Format(time.RFC3339), err)
		}
	}
}

func (r *Runner) runtimeBillingTargetContext(target model.DeploymentTarget) (model.Environment, model.Release, bool) {
	environment := deploymentTargetEnvironment(target)
	var release model.Release
	if err := r.db.
		Where("project_id = ? and application_id = ? and deployment_target_id = ? and status in ?", target.ProjectID, target.ApplicationID, target.ID, []string{"running", "succeeded"}).
		Order("created_at desc").
		First(&release).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return environment, release, false
		}
		if !r.canUseLegacyReleaseForRuntimeBilling(target) {
			return environment, release, false
		}
		query := r.db.
			Where("project_id = ? and application_id = ? and deployment_target_id = ? and status in ?", target.ProjectID, target.ApplicationID, "", []string{"running", "succeeded"})
		if strings.TrimSpace(target.EnvironmentID) != "" {
			query = query.Where("environment_id = ?", strings.TrimSpace(target.EnvironmentID))
		}
		if err := query.Order("created_at desc").First(&release).Error; err != nil {
			return environment, release, false
		}
	}
	return environment, release, true
}

func (r *Runner) canUseLegacyReleaseForRuntimeBilling(target model.DeploymentTarget) bool {
	query := r.db.Model(&model.DeploymentTarget{}).
		Where("project_id = ? and application_id = ? and enabled = ? and delete_status in ?", target.ProjectID, target.ApplicationID, true, []string{"active", ""})
	if strings.TrimSpace(target.EnvironmentID) != "" {
		query = query.Where("environment_id = ?", strings.TrimSpace(target.EnvironmentID))
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false
	}
	return count == 1
}

func runtimeBillingReleaseStart(release model.Release) time.Time {
	if release.FinishedAt != nil && !release.FinishedAt.IsZero() {
		return *release.FinishedAt
	}
	if release.StartedAt != nil && !release.StartedAt.IsZero() {
		return *release.StartedAt
	}
	return release.CreatedAt
}

type hourlyWindow struct {
	Start time.Time
	End   time.Time
}

func completedHourlyWindows(now time.Time, lookbackHours int) []hourlyWindow {
	if lookbackHours <= 0 {
		return nil
	}
	end := now.UTC().Truncate(time.Hour)
	windows := make([]hourlyWindow, 0, lookbackHours)
	for index := lookbackHours; index >= 1; index-- {
		start := end.Add(-time.Duration(index) * time.Hour)
		windows = append(windows, hourlyWindow{Start: start, End: start.Add(time.Hour)})
	}
	return windows
}

func runtimeBillingEffectivePeriod(windowStart time.Time, windowEnd time.Time, targetCreatedAt time.Time, releaseStart time.Time) (time.Time, time.Time, bool) {
	start := windowStart
	if targetCreatedAt.After(start) {
		start = targetCreatedAt
	}
	if releaseStart.After(start) {
		start = releaseStart
	}
	if !windowEnd.After(start) {
		return time.Time{}, time.Time{}, false
	}
	return start, windowEnd, true
}

func storageBillingEffectivePeriod(windowStart time.Time, windowEnd time.Time, targetCreatedAt time.Time) (time.Time, time.Time, bool) {
	start := windowStart
	if targetCreatedAt.After(start) {
		start = targetCreatedAt
	}
	if !windowEnd.After(start) {
		return time.Time{}, time.Time{}, false
	}
	return start, windowEnd, true
}
