package worker

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm/clause"
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
	var runtimeErr error
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		observationPeriodStart := now.UTC().Truncate(time.Minute)
		observationPeriodEnd := observationPeriodStart.Add(time.Minute)
		project, err := workerStageValue(ctx, "billing.load_project", func(stageCtx context.Context) (model.Project, error) {
			var project model.Project
			err := r.db.WithContext(stageCtx).First(&project, "id = ?", target.ProjectID).Error
			return project, err
		}, attribute.String("deployment_target.id", target.ID))
		if err != nil {
			continue
		}
		settlementErr := workerStage(ctx, "billing.settle_runtime", func(stageCtx context.Context) error {
			return r.settleRuntimeUsageForTarget(stageCtx, service, target, windows)
		}, attribute.String("deployment_target.id", target.ID))
		runtimeErr = errors.Join(runtimeErr, settlementErr)
		manager, err := workerStageValue(ctx, "billing.connect_runtime", func(stageCtx context.Context) (kubeprovider.NamespaceManager, error) {
			return r.kubernetesManager(stageCtx, deploymentTargetEnvironment(target))
		}, attribute.String("deployment_target.id", target.ID))
		if err != nil {
			telemetry.LogError(ctx, "Runtime resource observation failed to connect to the cluster", "runtime.resource_observation.failed", "runtime.resource_observe", "runtime.cluster_unavailable", err,
				slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID), slog.String("runtime_cluster_id", target.ClusterID), slog.String("deployment_target_id", target.ID),
				slog.String("period_start", observationPeriodStart.Format(time.RFC3339)), slog.String("period_end", observationPeriodEnd.Format(time.RFC3339)),
				slog.Int("sample_count", 0), slog.Int("metrics_sample_count", 0))
			continue
		}
		cluster, err := r.runtimeClusterForEnvironment(ctx, deploymentTargetEnvironment(target))
		if err != nil {
			telemetry.LogError(ctx, "Runtime resource observation failed to resolve the cluster", "runtime.resource_observation.failed", "runtime.resource_observe", "runtime.cluster_unavailable", err,
				slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID), slog.String("runtime_cluster_id", target.ClusterID), slog.String("deployment_target_id", target.ID),
				slog.String("period_start", observationPeriodStart.Format(time.RFC3339)), slog.String("period_end", observationPeriodEnd.Format(time.RFC3339)),
				slog.Int("sample_count", 0), slog.Int("metrics_sample_count", 0))
			continue
		}
		namespace := target.Namespace
		if namespace == "" {
			namespace = projectNamespace(project)
		}
		snapshot, err := workerStageValue(ctx, "billing.observe_runtime", func(stageCtx context.Context) (kubeprovider.DeploymentSnapshot, error) {
			return manager.GetWorkloadSnapshot(stageCtx, namespace, applicationResourceName(target), target.WorkloadType)
		}, attribute.String("deployment_target.id", target.ID))
		if err != nil {
			telemetry.LogError(ctx, "Runtime resource observation failed to query the workload", "runtime.resource_observation.failed", "runtime.resource_observe", "runtime.workload_unavailable", err,
				slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID),
				slog.String("runtime_cluster_id", cluster.ID), slog.String("deployment_target_id", target.ID),
				slog.String("period_start", observationPeriodStart.Format(time.RFC3339)), slog.String("period_end", observationPeriodEnd.Format(time.RFC3339)),
				slog.Int("sample_count", 0), slog.Int("metrics_sample_count", 0))
			continue
		}
		{
			metrics := kubeprovider.RuntimeMetricsSnapshot{Available: false, Reason: "metrics_unavailable", UpdatedAt: snapshot.ObservedAt}
			if provider, ok := manager.(interface {
				RuntimeMetrics(context.Context, kubeprovider.RuntimeMetricsOptions) (kubeprovider.RuntimeMetricsSnapshot, error)
			}); ok {
				metrics, err = workerStageValue(ctx, "runtime.metrics.query", func(stageCtx context.Context) (kubeprovider.RuntimeMetricsSnapshot, error) {
					return provider.RuntimeMetrics(stageCtx, kubeprovider.RuntimeMetricsOptions{
						Namespace: namespace, DeploymentTargetID: target.ID, WorkloadName: applicationResourceName(target), WorkloadType: target.WorkloadType,
					})
				}, attribute.String("deployment_target.id", target.ID), attribute.String("runtime_cluster.id", cluster.ID))
				if err != nil {
					metrics = kubeprovider.RuntimeMetricsSnapshot{Available: false, Reason: "metrics_unavailable", UpdatedAt: snapshot.ObservedAt}
				}
			}
			if observationErr := r.recordRuntimeObservation(ctx, target, cluster, snapshot, metrics); observationErr != nil {
				telemetry.LogError(ctx, "Runtime resource observation failed", "runtime.resource_observation.failed", "runtime.resource_observe", "runtime.resource_observation_failed", observationErr,
					slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID),
					slog.String("runtime_cluster_id", cluster.ID), slog.String("deployment_target_id", target.ID),
					slog.String("period_start", observationPeriodStart.Format(time.RFC3339)), slog.String("period_end", observationPeriodEnd.Format(time.RFC3339)),
					slog.Int("sample_count", 0), slog.Int("metrics_sample_count", 0))
				return observationErr
			}
		}
	}
	volumeErr := workerStage(ctx, "billing.settle_project_volumes", func(stageCtx context.Context) error {
		return r.settleProjectVolumeStorageWindows(stageCtx, service, windows)
	})
	transferErr := workerStage(ctx, "billing.settle_volume_transfers", func(stageCtx context.Context) error {
		return r.settleVolumeTransferUsage(stageCtx, service, now)
	})
	return errors.Join(runtimeErr, volumeErr, transferErr)
}

func (r *Runner) settleRuntimeUsageForTarget(ctx context.Context, service billing.Service, target model.DeploymentTarget, windows []hourlyWindow) error {
	var result error
	for _, window := range windows {
		if err := ctx.Err(); err != nil {
			return err
		}
		var observations []model.RuntimeObservation
		if err := r.db.WithContext(ctx).
			Where("deployment_target_id = ? AND period_start < ? AND period_end > ?", target.ID, window.End, window.Start).
			Order("period_start asc").Find(&observations).Error; err != nil {
			telemetry.LogError(ctx, "Runtime billing settlement failed while loading observations",
				"billing.runtime_settlement.failed", "billing.runtime_settle", "dependency.postgres.unavailable", err,
				slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID),
				slog.String("deployment_target_id", target.ID), slog.String("runtime_cluster_id", target.ClusterID), slog.String("period_start", window.Start.Format(time.RFC3339)),
				slog.String("period_end", window.End.Format(time.RFC3339)), slog.Int("sample_count", 0), slog.Int("metrics_sample_count", 0))
			result = errors.Join(result, err)
			continue
		}
		if len(observations) == 0 {
			telemetry.LogWarn(ctx, "Runtime billing settlement skipped because no minute observations exist",
				"billing.runtime_settlement.skipped", "billing.runtime_settle", "billing.runtime_observation_missing", nil,
				slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID),
				slog.String("deployment_target_id", target.ID), slog.String("runtime_cluster_id", target.ClusterID),
				slog.String("period_start", window.Start.Format(time.RFC3339)), slog.String("period_end", window.End.Format(time.RFC3339)),
				slog.Int("sample_count", 0), slog.Int("metrics_sample_count", 0))
			continue
		}
		aggregation, ok := aggregateRuntimeObservations(window, observations)
		if !ok {
			continue
		}
		if !aggregation.cpuBilled.IsPositive() && !aggregation.memoryBilled.IsPositive() {
			telemetry.LogWarn(ctx, "Runtime billing settlement skipped because the observed workload produced no billable usage",
				"billing.runtime_settlement.skipped", "billing.runtime_settle", "billing.runtime_usage_zero", nil,
				slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID),
				slog.String("deployment_target_id", target.ID), slog.String("runtime_cluster_id", observations[0].RuntimeClusterID),
				slog.String("period_start", window.Start.Format(time.RFC3339)), slog.String("period_end", window.End.Format(time.RFC3339)),
				slog.Int("sample_count", aggregation.sampleCount), slog.Int("metrics_sample_count", aggregation.metricsSampleCount))
			continue
		}
		err := service.SettleRuntimeTargetAggregation(billing.RuntimeAggregatedUsageInput{
			Context:            ctx,
			ProjectID:          target.ProjectID,
			ApplicationID:      target.ApplicationID,
			DeploymentTargetID: target.ID,
			EnvironmentID:      target.EnvironmentID,
			PeriodStart:        window.Start, PeriodEnd: window.End,
			CPUCoreHours: aggregation.cpuBilled, MemoryGiBHours: aggregation.memoryBilled,
			CPURequestFloorCoreHours: aggregation.cpuRequest, MemoryRequestFloorGiBHours: aggregation.memoryRequest,
			CPUActualObservedCoreHours: aggregation.cpuActual, MemoryActualObservedGiBHours: aggregation.memoryActual,
			SampleCount: aggregation.sampleCount, MetricsSampleCount: aggregation.metricsSampleCount,
			ObservedDurationSeconds: aggregation.observedSeconds, ExpectedDurationSeconds: int64(window.End.Sub(window.Start) / time.Second),
			ClusterResourcePolicySnapshot: aggregation.policySnapshots,
			ActorID:                       "system",
		})
		if errors.Is(err, billing.ErrAlreadySettled) {
			telemetry.LogWarn(ctx, "Runtime billing settlement skipped because the hourly usage is already settled",
				"billing.runtime_settlement.skipped", "billing.runtime_settle", "billing.runtime_already_settled", nil,
				slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID),
				slog.String("deployment_target_id", target.ID), slog.String("runtime_cluster_id", observations[0].RuntimeClusterID),
				slog.String("period_start", window.Start.Format(time.RFC3339)), slog.String("period_end", window.End.Format(time.RFC3339)),
				slog.Int("sample_count", aggregation.sampleCount), slog.Int("metrics_sample_count", aggregation.metricsSampleCount))
		} else if err != nil {
			telemetry.LogError(ctx, "Runtime billing settlement failed", "billing.runtime_settlement.failed", "billing.runtime_settle", "billing.runtime_settlement_failed", err,
				slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID),
				slog.String("deployment_target_id", target.ID), slog.String("runtime_cluster_id", observations[0].RuntimeClusterID),
				slog.String("period_start", window.Start.Format(time.RFC3339)), slog.String("period_end", window.End.Format(time.RFC3339)),
				slog.Int("sample_count", aggregation.sampleCount), slog.Int("metrics_sample_count", aggregation.metricsSampleCount))
			result = errors.Join(result, err)
		} else if err == nil {
			telemetry.Logger().InfoContext(ctx, "Runtime billing settlement completed",
				slog.String("event.name", "billing.runtime_settlement.completed"), slog.String("operation", "billing.runtime_settle"), slog.String("outcome", "succeeded"),
				slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID),
				slog.String("deployment_target_id", target.ID), slog.String("runtime_cluster_id", observations[0].RuntimeClusterID),
				slog.String("period_start", window.Start.Format(time.RFC3339)), slog.String("period_end", window.End.Format(time.RFC3339)),
				slog.Int("sample_count", aggregation.sampleCount), slog.Int("metrics_sample_count", aggregation.metricsSampleCount))
		}
	}
	return result
}

func (r *Runner) recordRuntimeObservation(ctx context.Context, target model.DeploymentTarget, cluster model.RuntimeCluster, snapshot kubeprovider.DeploymentSnapshot, metrics kubeprovider.RuntimeMetricsSnapshot) error {
	observedAt := snapshot.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	periodStart, periodEnd, ok := runtimeObservationWindow(observedAt, time.Now().UTC())
	if !ok {
		return nil
	}
	policy := resourcepolicy.Policy{CPURequestPercent: cluster.CPURequestPercent, MemoryRequestPercent: cluster.MemoryRequestPercent, CPULimitPercent: cluster.CPULimitPercent, MemoryLimitPercent: cluster.MemoryLimitPercent}
	effective, err := resourcepolicy.Calculate(target.CPURequest, target.MemoryRequest, policy)
	if err != nil {
		return err
	}
	observationCode := "runtime.metrics_observed"
	if !metrics.Available {
		observationCode = "runtime.metrics_unavailable"
	}
	observation := model.RuntimeObservation{
		ID:                     id.New("robs"),
		DeploymentTargetID:     target.ID,
		RuntimeClusterID:       cluster.ID,
		ProjectID:              target.ProjectID,
		PeriodStart:            periodStart,
		PeriodEnd:              periodEnd,
		DesiredReplicas:        snapshot.DesiredReplicas,
		UpdatedReplicas:        snapshot.UpdatedReplicas,
		ReadyReplicas:          snapshot.ReadyReplicas,
		AvailableReplicas:      snapshot.AvailableReplicas,
		EffectiveCPURequest:    effective.CPURequest,
		EffectiveMemoryRequest: effective.MemoryRequest,
		CPUUsageMilli:          metrics.CPUUsageMilli,
		MemoryUsageBytes:       metrics.MemoryUsageBytes,
		MetricsAvailable:       metrics.Available,
		PodCount:               metrics.PodCount,
		ContainerCount:         metrics.ContainerCount,
		CPURequestPercent:      policy.CPURequestPercent, MemoryRequestPercent: policy.MemoryRequestPercent,
		CPULimitPercent: policy.CPULimitPercent, MemoryLimitPercent: policy.MemoryLimitPercent,
		WorkloadCreatedAt: snapshot.CreatedAt,
		Status:            snapshot.Phase,
		ObservationCode:   observationCode,
		ObservedAt:        observedAt,
	}
	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "deployment_target_id"}, {Name: "period_start"}},
		DoNothing: true,
	}).Create(&observation).Error
	if err == nil {
		metricsSampleCount := 0
		if metrics.Available {
			metricsSampleCount = 1
		}
		telemetry.Logger().InfoContext(ctx, "Runtime resource observation completed",
			slog.String("event.name", "runtime.resource_observation.completed"), slog.String("operation", "runtime.resource_observe"), slog.String("outcome", "succeeded"),
			slog.String("resource.type", "deployment_target"), slog.String("resource.id", target.ID),
			slog.String("runtime_cluster_id", cluster.ID), slog.String("deployment_target_id", target.ID),
			slog.String("period_start", periodStart.Format(time.RFC3339)), slog.String("period_end", periodEnd.Format(time.RFC3339)),
			slog.Int("sample_count", 1), slog.Int("metrics_sample_count", metricsSampleCount))
	}
	return err
}

func runtimeObservationWindow(observedAt, now time.Time) (time.Time, time.Time, bool) {
	observedAt = observedAt.UTC()
	now = now.UTC()
	periodStart := observedAt.Truncate(time.Minute)
	if !periodStart.Equal(now.Truncate(time.Minute)) {
		return time.Time{}, time.Time{}, false
	}
	return periodStart, periodStart.Add(time.Minute), true
}

type runtimePolicySnapshot struct {
	CPURequestPercent    int `json:"cpuRequestPercent"`
	MemoryRequestPercent int `json:"memoryRequestPercent"`
	CPULimitPercent      int `json:"cpuLimitPercent"`
	MemoryLimitPercent   int `json:"memoryLimitPercent"`
}

type runtimeAggregation struct {
	cpuBilled, memoryBilled   decimal.Decimal
	cpuRequest, memoryRequest decimal.Decimal
	cpuActual, memoryActual   decimal.Decimal
	sampleCount               int
	metricsSampleCount        int
	observedSeconds           int64
	policySnapshots           []runtimePolicySnapshot
}

func aggregateRuntimeObservations(window hourlyWindow, observations []model.RuntimeObservation) (runtimeAggregation, bool) {
	result := runtimeAggregation{}
	sort.Slice(observations, func(i, j int) bool { return observations[i].PeriodStart.Before(observations[j].PeriodStart) })
	cursor := window.Start
	seenPolicies := map[runtimePolicySnapshot]bool{}
	for _, observation := range observations {
		start := observation.PeriodStart
		if start.Before(window.Start) {
			start = window.Start
		}
		if start.Before(cursor) {
			start = cursor
		}
		if observation.WorkloadCreatedAt.After(start) {
			start = observation.WorkloadCreatedAt
		}
		end := observation.PeriodEnd
		if end.After(window.End) {
			end = window.End
		}
		if !end.After(start) {
			continue
		}
		cursor = end
		result.sampleCount++
		if observation.MetricsAvailable {
			result.metricsSampleCount++
		}
		policy := runtimePolicySnapshot{observation.CPURequestPercent, observation.MemoryRequestPercent, observation.CPULimitPercent, observation.MemoryLimitPercent}
		if !seenPolicies[policy] {
			seenPolicies[policy] = true
			result.policySnapshots = append(result.policySnapshots, policy)
		}
		if observation.DesiredReplicas <= 0 {
			continue
		}
		durationNanos := decimal.NewFromInt(end.Sub(start).Nanoseconds())
		durationHours := durationNanos.Div(decimal.NewFromInt(int64(time.Hour)))
		result.observedSeconds += int64(end.Sub(start) / time.Second)

		requestCPUMilli := quantityMilliValue(observation.EffectiveCPURequest) * int64(observation.DesiredReplicas)
		actualCPUMilli := int64(0)
		if observation.MetricsAvailable {
			actualCPUMilli = observation.CPUUsageMilli
		}
		billedCPUMilli := requestCPUMilli
		if actualCPUMilli > billedCPUMilli {
			billedCPUMilli = actualCPUMilli
		}
		cpuRequest := decimal.NewFromInt(requestCPUMilli).Div(decimal.NewFromInt(1000)).Mul(durationHours)
		cpuActual := decimal.NewFromInt(actualCPUMilli).Div(decimal.NewFromInt(1000)).Mul(durationHours)
		result.cpuRequest = result.cpuRequest.Add(cpuRequest)
		result.cpuActual = result.cpuActual.Add(cpuActual)
		result.cpuBilled = result.cpuBilled.Add(decimal.NewFromInt(billedCPUMilli).Div(decimal.NewFromInt(1000)).Mul(durationHours))

		requestMemoryBytes := quantityValue(observation.EffectiveMemoryRequest) * int64(observation.DesiredReplicas)
		actualMemoryBytes := int64(0)
		if observation.MetricsAvailable {
			actualMemoryBytes = observation.MemoryUsageBytes
		}
		billedMemoryBytes := requestMemoryBytes
		if actualMemoryBytes > billedMemoryBytes {
			billedMemoryBytes = actualMemoryBytes
		}
		gib := decimal.NewFromInt(1024 * 1024 * 1024)
		result.memoryRequest = result.memoryRequest.Add(decimal.NewFromInt(requestMemoryBytes).Div(gib).Mul(durationHours))
		result.memoryActual = result.memoryActual.Add(decimal.NewFromInt(actualMemoryBytes).Div(gib).Mul(durationHours))
		result.memoryBilled = result.memoryBilled.Add(decimal.NewFromInt(billedMemoryBytes).Div(gib).Mul(durationHours))
	}
	return result, result.sampleCount > 0
}

func quantityMilliValue(value string) int64 {
	if value == "" {
		return 0
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil || quantity.Sign() <= 0 {
		return 0
	}
	return quantity.MilliValue()
}

func quantityValue(value string) int64 {
	if value == "" {
		return 0
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil || quantity.Sign() <= 0 {
		return 0
	}
	return quantity.Value()
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
