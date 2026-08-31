package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"gorm.io/gorm/clause"
)

func (r *Runner) settleKubectlRuntimeUsageWindows(ctx context.Context, now time.Time) error {
	if r == nil || r.db == nil {
		return nil
	}
	clusters, err := r.listActiveKubectlRuntimeClusters(ctx)
	if err != nil {
		return err
	}
	var result error
	for _, cluster := range clusters {
		if err := ctx.Err(); err != nil {
			return err
		}
		clusterProjects, err := r.listKubectlGatewayProjects(ctx, cluster)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		manager, err := r.kubectlGatewayManagerForCluster(ctx, cluster)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		proxyClient, err := manager.NewGatewayProxyClient(ctx, kubeprovider.GatewayTokenRequestOptions{})
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		for _, project := range clusterProjects {
			if err := r.recordKubectlRuntimeObservations(ctx, now, cluster, project, proxyClient); err != nil {
				result = errors.Join(result, err)
			}
		}
		if err := r.settleRecordedKubectlRuntimeWindows(ctx, cluster, completedHourlyWindows(now, runtimeBillingLookbackHours)); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (r *Runner) listActiveKubectlRuntimeClusters(ctx context.Context) ([]model.RuntimeCluster, error) {
	var clusters []model.RuntimeCluster
	if err := runtimecluster.ActiveScope(r.db.WithContext(ctx)).Where("type in ?", []string{"kubernetes", "k3s"}).Order("created_at asc").Find(&clusters).Error; err != nil {
		return nil, err
	}
	filtered := make([]model.RuntimeCluster, 0, len(clusters))
	for _, cluster := range clusters {
		if !clusterKubectlGatewayEnabled(cluster) {
			continue
		}
		filtered = append(filtered, cluster)
	}
	return filtered, nil
}

func (r *Runner) recordKubectlRuntimeObservations(ctx context.Context, now time.Time, cluster model.RuntimeCluster, project model.Project, client *kubeprovider.Client) error {
	workloads, err := client.ListKubectlRuntimeWorkloads(ctx, kubeprovider.KubectlRuntimeObservationOptions{
		RuntimeClusterID: cluster.ID,
		ProjectID:        project.ID,
		Namespace:        projectNamespace(project),
	})
	if err != nil {
		return err
	}
	for _, workload := range workloads {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateKubectlRuntimeWorkload(cluster, project, workload); err != nil {
			return err
		}
		metrics := kubeprovider.RuntimeMetricsSnapshot{Available: false, Reason: "metrics_unavailable", UpdatedAt: now.UTC()}
		metrics, err = client.RuntimeMetrics(ctx, workload.RuntimeMetricsOptions())
		if err != nil {
			metrics = kubeprovider.RuntimeMetricsSnapshot{Available: false, Reason: "metrics_unavailable", UpdatedAt: now.UTC()}
		}
		if err := r.recordKubectlRuntimeObservation(ctx, cluster, workload, metrics, now); err != nil {
			return err
		}
	}
	return nil
}

func validateKubectlRuntimeWorkload(cluster model.RuntimeCluster, project model.Project, workload kubeprovider.KubectlRuntimeWorkload) error {
	if strings.TrimSpace(workload.RuntimeClusterID) != strings.TrimSpace(cluster.ID) || strings.TrimSpace(workload.ProjectID) != strings.TrimSpace(project.ID) ||
		strings.TrimSpace(workload.Namespace) != strings.TrimSpace(projectNamespace(project)) || strings.TrimSpace(workload.ResourceUID) == "" ||
		strings.TrimSpace(workload.ManagementSource) != kubeprovider.KubectlGatewayManagementSourceValue {
		return fmt.Errorf("kubectl runtime workload is outside the requested cluster and project ownership boundary")
	}
	return nil
}

func (r *Runner) recordKubectlRuntimeObservation(ctx context.Context, cluster model.RuntimeCluster, workload kubeprovider.KubectlRuntimeWorkload, metrics kubeprovider.RuntimeMetricsSnapshot, now time.Time) error {
	observedAt := workload.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = now.UTC()
	}
	periodStart, periodEnd, ok := runtimeObservationWindow(observedAt, now.UTC())
	if !ok {
		return nil
	}
	observationCode := "runtime.metrics_observed"
	if !metrics.Available {
		observationCode = "runtime.metrics_unavailable"
	}
	applicationID := optionalStringPtr(workload.ApplicationID)
	observation := model.RuntimeObservation{
		ManagementSource:       kubeprovider.KubectlGatewayManagementSourceValue,
		ResourceKind:           workload.Kind,
		ResourceUID:            workload.ResourceUID,
		ApplicationID:          applicationID,
		ID:                     id.New("robs"),
		DeploymentTargetID:     nil,
		RuntimeClusterID:       cluster.ID,
		ProjectID:              workload.ProjectID,
		PeriodStart:            periodStart,
		PeriodEnd:              periodEnd,
		DesiredReplicas:        workload.DesiredReplicas,
		UpdatedReplicas:        workload.UpdatedReplicas,
		ReadyReplicas:          workload.ReadyReplicas,
		AvailableReplicas:      workload.AvailableReplicas,
		EffectiveCPURequest:    workload.EffectiveCPURequest,
		EffectiveMemoryRequest: workload.EffectiveMemoryRequest,
		CPUUsageMilli:          metrics.CPUUsageMilli,
		MemoryUsageBytes:       metrics.MemoryUsageBytes,
		MetricsAvailable:       metrics.Available,
		PodCount:               metrics.PodCount,
		ContainerCount:         metrics.ContainerCount,
		CPURequestPercent:      cluster.CPURequestPercent,
		MemoryRequestPercent:   cluster.MemoryRequestPercent,
		CPULimitPercent:        cluster.CPULimitPercent,
		MemoryLimitPercent:     cluster.MemoryLimitPercent,
		WorkloadCreatedAt:      workload.CreatedAt,
		Status:                 workload.Status,
		ObservationCode:        observationCode,
		ObservedAt:             observedAt,
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "runtime_cluster_id"}, {Name: "project_id"}, {Name: "resource_uid"}, {Name: "period_start"},
		},
		DoNothing: true,
	}).Create(&observation).Error
	if err == nil {
		metricsSampleCount := 0
		if metrics.Available {
			metricsSampleCount = 1
		}
		telemetry.Logger().InfoContext(ctx, "Kubectl runtime resource observation completed",
			slog.String("event.name", "runtime.resource_observation.completed"),
			slog.String("operation", "runtime.resource_observe"),
			slog.String("outcome", "succeeded"),
			slog.String("resource.type", "kubectl_workload"),
			slog.String("management_source", kubeprovider.KubectlGatewayManagementSourceValue),
			slog.Int("sample_count", 1),
			slog.Int("metrics_sample_count", metricsSampleCount))
	}
	return err
}

func (r *Runner) settleRecordedKubectlRuntimeWindows(ctx context.Context, cluster model.RuntimeCluster, windows []hourlyWindow) error {
	if len(windows) == 0 {
		return nil
	}
	type resourceRow struct {
		ProjectID     string
		ApplicationID *string
		ResourceUID   string
	}
	var resources []resourceRow
	if err := r.db.WithContext(ctx).Model(&model.RuntimeObservation{}).
		Select("project_id, application_id, resource_uid").
		Where("runtime_cluster_id = ? and management_source = ?", cluster.ID, kubeprovider.KubectlGatewayManagementSourceValue).
		Group("project_id, application_id, resource_uid").
		Find(&resources).Error; err != nil {
		return err
	}
	service := billing.Service{DB: r.db}
	var result error
	for _, row := range resources {
		projectID := strings.TrimSpace(row.ProjectID)
		resourceUID := strings.TrimSpace(row.ResourceUID)
		if projectID == "" || resourceUID == "" {
			continue
		}
		applicationID := ""
		if row.ApplicationID != nil {
			applicationID = strings.TrimSpace(*row.ApplicationID)
		}
		syntheticTargetID := kubeprovider.KubectlRuntimeSyntheticTargetID(cluster.ID, projectID, applicationID, resourceUID)
		for _, window := range windows {
			var observations []model.RuntimeObservation
			query := r.db.WithContext(ctx).
				Where("runtime_cluster_id = ? AND project_id = ? AND resource_uid = ? AND management_source = ? AND period_start < ? AND period_end > ?",
					cluster.ID, projectID, resourceUID, kubeprovider.KubectlGatewayManagementSourceValue, window.End, window.Start)
			if applicationID == "" {
				query = query.Where("application_id IS NULL")
			} else {
				query = query.Where("application_id = ?", applicationID)
			}
			if err := query.Order("period_start asc").Find(&observations).Error; err != nil {
				result = errors.Join(result, err)
				continue
			}
			aggregation, ok := aggregateRuntimeObservations(window, observations)
			if !ok || (!aggregation.cpuBilled.IsPositive() && !aggregation.memoryBilled.IsPositive()) {
				continue
			}
			err := service.SettleRuntimeTargetAggregation(billing.RuntimeAggregatedUsageInput{
				Context:                       ctx,
				ProjectID:                     projectID,
				ApplicationID:                 applicationID,
				DeploymentTargetID:            syntheticTargetID,
				EnvironmentID:                 "",
				PeriodStart:                   window.Start,
				PeriodEnd:                     window.End,
				CPUCoreHours:                  aggregation.cpuBilled,
				MemoryGiBHours:                aggregation.memoryBilled,
				CPURequestFloorCoreHours:      aggregation.cpuRequest,
				MemoryRequestFloorGiBHours:    aggregation.memoryRequest,
				CPUActualObservedCoreHours:    aggregation.cpuActual,
				MemoryActualObservedGiBHours:  aggregation.memoryActual,
				SampleCount:                   aggregation.sampleCount,
				MetricsSampleCount:            aggregation.metricsSampleCount,
				ObservedDurationSeconds:       aggregation.observedSeconds,
				ExpectedDurationSeconds:       int64(window.End.Sub(window.Start) / time.Second),
				ClusterResourcePolicySnapshot: aggregation.policySnapshots,
				ActorID:                       "system",
			})
			if err != nil && !errors.Is(err, billing.ErrAlreadySettled) {
				result = errors.Join(result, err)
			}
		}
	}
	return result
}

func optionalStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
