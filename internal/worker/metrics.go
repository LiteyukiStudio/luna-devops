package worker

import (
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observability"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func (r *Runner) recordBuildRunMetrics(run model.BuildRun) {
	if r.workerMetrics == nil {
		return
	}
	r.workerMetrics.RecordBuildRun(observability.BusinessRunMetric{
		Status:     run.Status,
		Type:       run.TriggerType,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
		CreatedAt:  run.CreatedAt,
	})
}

func (r *Runner) recordReleaseMetrics(release model.Release) {
	if r.workerMetrics == nil {
		return
	}
	r.workerMetrics.RecordRelease(observability.BusinessRunMetric{
		Status:     release.Status,
		Type:       release.Type,
		StartedAt:  release.StartedAt,
		FinishedAt: release.FinishedAt,
		CreatedAt:  release.CreatedAt,
	})
}

func (r *Runner) recordGatewaySyncMetric(operation string, result string, startedAt time.Time) {
	if r.workerMetrics == nil {
		return
	}
	r.workerMetrics.RecordGatewaySync(operation, result, time.Since(startedAt))
}

func (r *Runner) refreshGatewayRouteMetrics() {
	if r.workerMetrics == nil || r.db == nil {
		return
	}
	var routes []model.GatewayRoute
	if err := r.db.Find(&routes).Error; err != nil {
		return
	}
	metrics := make([]observability.GatewayRouteMetric, 0, len(routes))
	for _, route := range routes {
		metrics = append(metrics, observability.GatewayRouteMetric{
			Status:            route.Status,
			TLSMode:           route.TLSMode,
			DNSStatus:         route.DNSStatus,
			CertificateStatus: route.CertificateStatus,
		})
	}
	r.workerMetrics.SetGatewayRoutes(metrics)
}

func (r *Runner) recordDeploymentRuntimeMetric(target model.DeploymentTarget, environment model.Environment, snapshot kubeprovider.DeploymentSnapshot) {
	if r.workerMetrics == nil {
		return
	}
	r.workerMetrics.SetDeploymentRuntime(observability.DeploymentRuntimeMetric{
		DeploymentTargetID: target.ID,
		EnvironmentID:      environment.ID,
		DesiredReplicas:    snapshot.DesiredReplicas,
		ReadyReplicas:      snapshot.ReadyReplicas,
		AvailableReplicas:  snapshot.AvailableReplicas,
		UpdatedReplicas:    snapshot.UpdatedReplicas,
	})
}
