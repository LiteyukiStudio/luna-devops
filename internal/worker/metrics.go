package worker

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observability"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func (r *Runner) recordBuildRunMetrics(ctx context.Context, run model.BuildRun) {
	if r.workerMetrics == nil {
		return
	}
	r.workerMetrics.RecordBuildRun(ctx, observability.BusinessRunMetric{
		Status:     run.Status,
		Type:       run.TriggerType,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
		CreatedAt:  run.CreatedAt,
	})
}

func (r *Runner) recordReleaseMetrics(ctx context.Context, release model.Release) {
	if r.workerMetrics == nil {
		return
	}
	r.workerMetrics.RecordRelease(ctx, observability.BusinessRunMetric{
		Status:     release.Status,
		Type:       release.Type,
		StartedAt:  release.StartedAt,
		FinishedAt: release.FinishedAt,
		CreatedAt:  release.CreatedAt,
	})
}

func (r *Runner) recordGatewaySyncMetric(ctx context.Context, operation string, result string, startedAt time.Time) {
	if r.workerMetrics == nil {
		return
	}
	r.workerMetrics.RecordGatewaySync(ctx, operation, result, time.Since(startedAt))
}

func (r *Runner) recordDeploymentRuntimeMetric(ctx context.Context, snapshot kubeprovider.DeploymentSnapshot) {
	if r.workerMetrics == nil {
		return
	}
	r.workerMetrics.SetDeploymentRuntime(ctx, observability.DeploymentRuntimeMetric{
		DesiredReplicas:   snapshot.DesiredReplicas,
		ReadyReplicas:     snapshot.ReadyReplicas,
		AvailableReplicas: snapshot.AvailableReplicas,
		UpdatedReplicas:   snapshot.UpdatedReplicas,
	})
}
