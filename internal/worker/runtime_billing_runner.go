package worker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
)

const runtimeBillingLookbackHours = 6

func (r *Runner) handleBillingRuntime(ctx context.Context, task *asynq.Task) error {
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
		project, err := workerStageValue(ctx, "billing.load_project", func(context.Context) (model.Project, error) {
			var project model.Project
			err := r.db.First(&project, "id = ?", target.ProjectID).Error
			return project, err
		}, attribute.String("deployment_target.id", target.ID))
		if err != nil {
			continue
		}
		environment := deploymentTargetEnvironment(target)
		manager, err := workerStageValue(ctx, "billing.connect_runtime", func(stageCtx context.Context) (kubeprovider.NamespaceManager, error) {
			return r.kubernetesManager(stageCtx, environment)
		}, attribute.String("deployment_target.id", target.ID))
		if err != nil {
			continue
		}
		namespace := target.Namespace
		if namespace == "" {
			namespace = projectNamespace(project)
		}
		snapshot, err := workerStageValue(ctx, "billing.observe_runtime", func(stageCtx context.Context) (kubeprovider.DeploymentSnapshot, error) {
			return manager.GetWorkloadSnapshot(stageCtx, namespace, applicationResourceName(target), target.WorkloadType)
		}, attribute.String("deployment_target.id", target.ID))
		if err == nil && snapshot.DesiredReplicas > 0 {
			liveEnvironment := environment
			liveEnvironment.Replicas = int(snapshot.DesiredReplicas)
			_ = workerStage(ctx, "billing.settle_runtime", func(stageCtx context.Context) error {
				return r.settleRuntimeUsageForTarget(stageCtx, service, target, liveEnvironment, snapshot.CreatedAt, windows)
			}, attribute.String("deployment_target.id", target.ID))
		}
		claims, err := workerStageValue(ctx, "billing.observe_storage", func(stageCtx context.Context) ([]kubeprovider.PersistentVolumeClaimSnapshot, error) {
			return manager.ListManagedPersistentVolumeClaims(stageCtx, namespace, target.ID)
		}, attribute.String("deployment_target.id", target.ID))
		if err != nil {
			continue
		}
		_ = workerStage(ctx, "billing.settle_storage", func(stageCtx context.Context) error {
			return r.settleStorageUsageForTarget(stageCtx, service, target, claims, windows)
		}, attribute.String("deployment_target.id", target.ID))
	}
	return nil
}

func (r *Runner) settleRuntimeUsageForTarget(ctx context.Context, service billing.Service, target model.DeploymentTarget, environment model.Environment, workloadCreatedAt time.Time, windows []hourlyWindow) error {
	var result error
	for _, window := range windows {
		if err := ctx.Err(); err != nil {
			return err
		}
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
			result = errors.Join(result, err)
		}
	}
	return result
}

func (r *Runner) settleStorageUsageForTarget(ctx context.Context, service billing.Service, target model.DeploymentTarget, claims []kubeprovider.PersistentVolumeClaimSnapshot, windows []hourlyWindow) error {
	if len(claims) == 0 {
		return nil
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
		return err
	}
	liveTarget := target
	liveTarget.DataRetentionEnabled = true
	liveTarget.DataVolumes = string(encodedVolumes)
	liveTarget.DataCapacity = ""
	var result error
	for _, window := range windows {
		if err := ctx.Err(); err != nil {
			return err
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
			result = errors.Join(result, err)
		}
	}
	return result
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
