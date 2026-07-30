package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/hibiken/asynq"
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
		var project model.Project
		if err := r.db.First(&project, "id = ?", target.ProjectID).Error; err != nil {
			log.Printf("live billing observation skipped target=%s: load project: %v", target.ID, err)
			continue
		}
		environment := deploymentTargetEnvironment(target)
		manager, err := r.kubernetesManager(environment)
		if err != nil {
			log.Printf("live billing observation unavailable target=%s: %v", target.ID, err)
			continue
		}
		namespace := target.Namespace
		if namespace == "" {
			namespace = projectNamespace(project)
		}
		snapshot, err := manager.GetWorkloadSnapshot(ctx, namespace, applicationResourceName(target), target.WorkloadType)
		if err != nil {
			log.Printf("live runtime billing observation unavailable target=%s: %v", target.ID, err)
		} else if snapshot.DesiredReplicas > 0 {
			liveEnvironment := environment
			liveEnvironment.Replicas = int(snapshot.DesiredReplicas)
			r.settleRuntimeUsageForTarget(service, target, liveEnvironment, snapshot.CreatedAt, windows)
		}
		claims, err := manager.ListManagedPersistentVolumeClaims(ctx, namespace, target.ID)
		if err != nil {
			log.Printf("live storage billing observation unavailable target=%s: %v", target.ID, err)
			continue
		}
		r.settleStorageUsageForTarget(ctx, service, target, claims, windows)
	}
	return nil
}

func (r *Runner) settleRuntimeUsageForTarget(service billing.Service, target model.DeploymentTarget, environment model.Environment, workloadCreatedAt time.Time, windows []hourlyWindow) {
	for _, window := range windows {
		periodStart, periodEnd, ok := runtimeBillingEffectivePeriod(window.Start, window.End, workloadCreatedAt)
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

func (r *Runner) settleStorageUsageForTarget(ctx context.Context, service billing.Service, target model.DeploymentTarget, claims []kubeprovider.PersistentVolumeClaimSnapshot, windows []hourlyWindow) {
	if len(claims) == 0 {
		return
	}
	liveVolumes := make([]map[string]string, 0, len(claims))
	storageCreatedAt := claims[0].CreatedAt
	for _, claim := range claims {
		liveVolumes = append(liveVolumes, map[string]string{"name": claim.Name, "capacity": claim.Capacity})
		if claim.CreatedAt.After(storageCreatedAt) {
			storageCreatedAt = claim.CreatedAt
		}
	}
	encodedVolumes, err := json.Marshal(liveVolumes)
	if err != nil {
		log.Printf("live storage billing observation skipped target=%s: encode PVC capacities: %v", target.ID, err)
		return
	}
	liveTarget := target
	liveTarget.DataRetentionEnabled = true
	liveTarget.DataVolumes = string(encodedVolumes)
	liveTarget.DataCapacity = ""
	for _, window := range windows {
		if err := ctx.Err(); err != nil {
			return
		}
		periodStart, periodEnd, ok := storageBillingEffectivePeriod(window.Start, window.End, storageCreatedAt)
		if !ok {
			continue
		}
		err := service.SettleStorageTargetWindow(billing.StorageUsageInput{
			Target:      liveTarget,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
			ActorID:     "system",
		})
		if err != nil && !errors.Is(err, billing.ErrAlreadySettled) {
			log.Printf("storage billing settlement skipped target=%s window=%s: %v", target.ID, window.Start.Format(time.RFC3339), err)
		}
	}
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

func runtimeBillingEffectivePeriod(windowStart time.Time, windowEnd time.Time, workloadCreatedAt time.Time) (time.Time, time.Time, bool) {
	start := windowStart
	if workloadCreatedAt.After(start) {
		start = workloadCreatedAt
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
