package worker

import (
	"context"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"k8s.io/apimachinery/pkg/api/resource"
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
	if err := r.db.WithContext(ctx).
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
		project, err := workerStageValue(ctx, "billing.load_project", func(stageCtx context.Context) (model.Project, error) {
			var project model.Project
			err := r.db.WithContext(stageCtx).First(&project, "id = ?", target.ProjectID).Error
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
	}
	volumeErr := workerStage(ctx, "billing.settle_project_volumes", func(stageCtx context.Context) error {
		return r.settleProjectVolumeStorageWindows(stageCtx, service, windows)
	})
	transferErr := workerStage(ctx, "billing.settle_volume_transfers", func(stageCtx context.Context) error {
		return r.settleVolumeTransferUsage(stageCtx, service, now)
	})
	return errors.Join(volumeErr, transferErr)
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

func (r *Runner) settleProjectVolumeStorageWindows(ctx context.Context, service billing.Service, windows []hourlyWindow) error {
	if r.db == nil || len(windows) == 0 {
		return nil
	}
	const pageSize = 100
	type observationGroup struct {
		clusterID string
		namespace string
		projectID string
	}
	cursor := ""
	var result error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		items := make([]model.ProjectVolume, 0, pageSize)
		query := r.db.WithContext(ctx).Where("ownership_mode = ?", model.ProjectVolumeOwnershipManaged)
		if cursor != "" {
			query = query.Where("id > ?", cursor)
		}
		if err := query.Order("id ASC").Limit(pageSize).Find(&items).Error; err != nil {
			return err
		}
		groups := make(map[observationGroup][]model.ProjectVolume)
		for _, item := range items {
			key := observationGroup{clusterID: item.ClusterID, namespace: item.Namespace, projectID: item.ProjectID}
			groups[key] = append(groups[key], item)
		}
		for key, volumes := range groups {
			provider, err := r.projectVolumeProvider(ctx, key.clusterID)
			if err != nil {
				result = errors.Join(result, err)
				continue
			}
			claimNames := make([]string, 0, len(volumes))
			for _, item := range volumes {
				claimNames = append(claimNames, item.ClaimName)
			}
			observations, err := provider.ObserveProjectVolumeClaims(ctx, key.namespace, key.projectID, claimNames)
			if err != nil {
				result = errors.Join(result, err)
				continue
			}
			for _, item := range volumes {
				observation, exists := observations[item.ClaimName]
				if !exists || !observation.Exists {
					continue
				}
				storageCreatedAt := item.CreatedAt
				if observation.CreatedAt.After(storageCreatedAt) {
					storageCreatedAt = observation.CreatedAt
				}
				capacity := observation.Capacity
				if capacity == "" {
					capacity = observation.RequestedCapacity
				}
				quantity, parseErr := resource.ParseQuantity(capacity)
				if parseErr != nil || quantity.Value() <= 0 {
					continue
				}
				for _, window := range windows {
					periodStart, periodEnd, ok := storageBillingEffectivePeriod(window.Start, window.End, storageCreatedAt)
					if !ok {
						continue
					}
					err := service.SettleProjectVolumeStorageWindow(ctx, billing.ProjectVolumeStorageUsageInput{
						Volume: item, ObservedCapacityBytes: quantity.Value(),
						PeriodStart: periodStart, PeriodEnd: periodEnd, ActorID: "system",
					})
					if err != nil && !errors.Is(err, billing.ErrAlreadySettled) {
						result = errors.Join(result, err)
					}
				}
			}
		}
		if len(items) < pageSize {
			break
		}
		cursor = items[len(items)-1].ID
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
