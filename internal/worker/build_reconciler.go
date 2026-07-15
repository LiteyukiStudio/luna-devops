package worker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Runner) syncBuildJobStatus(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := r.markExpiredBuildJobsLost(); err != nil {
			log.Printf("build job status sync failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) markExpiredBuildJobsLost() error {
	if r.db == nil {
		return nil
	}
	now := time.Now()
	var lostRuns []model.BuildRun
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var jobs []model.BuildJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Joins("join build_runs on build_runs.id = build_jobs.build_run_id").
			Where("build_jobs.status = ?", "running").
			Where(
				"(build_jobs.lease_until is not null and build_jobs.lease_until < ?) or (build_jobs.lease_until is null and build_jobs.started_at is not null and build_jobs.started_at < (?::timestamptz - (coalesce(nullif(build_runs.build_timeout_seconds, 0), ?) * interval '1 second')))",
				now,
				now,
				effectiveBuildTimeoutSeconds(0, r.buildJobTimeoutSeconds),
			).
			Order("build_jobs.started_at asc, build_jobs.lease_until asc").
			Limit(50).
			Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			var run model.BuildRun
			if err := tx.First(&run, "id = ? and project_id = ?", job.BuildRunID, job.ProjectID).Error; err != nil {
				return err
			}
			finishedAt := now
			if err := tx.Model(&model.BuildJob{}).
				Where("id = ? and status = ?", job.ID, "running").
				Updates(expiredBuildJobUpdates(finishedAt)).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.BuildRun{}).
				Where("id = ? and project_id = ? and status = ?", job.BuildRunID, job.ProjectID, "running").
				Updates(map[string]any{
					"status":      "lost",
					"finished_at": &finishedAt,
				}).Error; err != nil {
				return err
			}
			run.Status = "lost"
			run.FinishedAt = &finishedAt
			lostRuns = append(lostRuns, run)
		}
		return nil
	})
	if err == nil {
		for _, run := range lostRuns {
			r.recordBuildRunMetrics(run)
		}
	}
	return err
}

func (r *Runner) handleSyncStatus(ctx context.Context, task *asynq.Task) error {
	log.Printf("received task type=%s payload=%s", task.Type(), string(task.Payload()))
	if err := r.syncReleaseRuntimeStatus(ctx); err != nil {
		return err
	}
	if err := r.syncGatewayCertificateStatus(ctx); err != nil {
		return err
	}
	r.refreshGatewayRouteMetrics()
	r.retryPendingResourceCleanups(ctx)
	return nil
}

func (r *Runner) syncGatewayCertificateStatus(ctx context.Context) error {
	if r.db == nil {
		return nil
	}
	var routes []model.GatewayRoute
	if err := r.db.
		Where("tls_mode = ? and enabled = ? and delete_status = ?", "http-challenge", true, "active").
		Order("updated_at asc").
		Limit(200).
		Find(&routes).Error; err != nil {
		return err
	}
	for _, route := range routes {
		if err := r.syncGatewayCertificateSnapshot(ctx, route); err != nil {
			log.Printf("gateway certificate status sync skipped route=%s: %v", route.ID, err)
		}
	}
	return nil
}

func (r *Runner) syncGatewayCertificateSnapshot(ctx context.Context, route model.GatewayRoute) error {
	var project model.Project
	if err := r.db.First(&project, "id = ?", route.ProjectID).Error; err != nil {
		return err
	}
	var target model.DeploymentTarget
	if err := r.db.First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
		return err
	}
	environment := deploymentTargetEnvironment(target)
	snapshot, configured, err := r.gatewayCertificateSnapshot(ctx, route, project, environment, deploymentNamespace(project, environment))
	if err != nil {
		return err
	}
	if !configured {
		return nil
	}
	cluster, err := r.runtimeClusterForEnvironment(environment)
	if err != nil {
		return err
	}
	if err := r.db.Model(&model.GatewayRoute{}).Where("id = ?", route.ID).Updates(gatewayCertificateRuntimeUpdates(snapshot, cluster, r.certManagerClusterIssuer)).Error; err != nil {
		return err
	}
	r.emitCertificateSnapshotEvent(ctx, route, snapshot, cluster)
	return nil
}

func (r *Runner) emitCertificateSnapshotEvent(ctx context.Context, route model.GatewayRoute, snapshot kubeprovider.CertificateSnapshot, cluster model.RuntimeCluster) {
	status := snapshot.Phase
	renewed := status == kubeprovider.CertificateIssued && route.CertificateStatus == kubeprovider.CertificateIssued && !sameEventTime(route.CertificateNotAfter, snapshot.NotAfter)
	if route.CertificateStatus == status && !renewed {
		return
	}
	if renewed {
		status = "renewed"
	}
	route.CertificateStatus = snapshot.Phase
	route.CertificateMessage = snapshot.Message
	route.CertificateNotAfter = snapshot.NotAfter
	route.CertificateIssuerKind = gatewayCertificateIssuerKind(cluster)
	route.CertificateIssuerName = gatewayCertificateIssuerName(cluster, r.certManagerClusterIssuer)
	r.emitCertificateEvent(ctx, route, status, snapshot.Message)
}

func sameEventTime(left *time.Time, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func (r *Runner) syncReleaseRuntimeStatus(ctx context.Context) error {
	if r.db == nil {
		return nil
	}
	var releases []model.Release
	if err := r.db.
		Where("status in ?", []string{"pending", "running", "succeeded"}).
		Order("created_at desc").
		Limit(200).
		Find(&releases).Error; err != nil {
		return err
	}
	for _, release := range releases {
		if err := r.syncReleaseRuntimeSnapshot(ctx, release); err != nil {
			log.Printf("release runtime status sync skipped release=%s: %v", release.ID, err)
		}
	}
	return nil
}

func (r *Runner) syncReleaseRuntimeSnapshot(ctx context.Context, release model.Release) error {
	var project model.Project
	if err := r.db.First(&project, "id = ?", release.ProjectID).Error; err != nil {
		return err
	}
	var application model.Application
	if err := r.db.First(&application, "id = ? and project_id = ?", release.ApplicationID, release.ProjectID).Error; err != nil {
		return err
	}
	deploymentTarget, err := r.releaseDeploymentTarget(release)
	if err != nil {
		return err
	}
	environment := deploymentTargetEnvironment(deploymentTarget)
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return err
	}
	namespace := deploymentNamespace(project, environment)
	resourceName := applicationResourceName(deploymentTarget)
	snapshot, err := manager.GetDeploymentSnapshot(ctx, namespace, resourceName)
	if err != nil {
		if isKubernetesNotFound(err) {
			message := fmt.Sprintf("deployment_missing: Kubernetes %s %s/%s not found", deploymentTargetWorkloadKind(deploymentTarget), namespace, resourceName)
			return r.markReleaseRuntimeDrift(release, message)
		}
		return err
	}
	r.recordDeploymentRuntimeMetric(deploymentTarget, environment, snapshot)
	if snapshot.Phase == kubeprovider.DeploymentFailed {
		return r.markReleaseRuntimeDrift(release, firstNonEmpty(snapshot.Message, "Deployment runtime check failed"))
	}
	if release.Status == "pending" || release.Status == "running" {
		if snapshot.Phase == kubeprovider.DeploymentSucceeded {
			r.appendReleaseLog(release, firstNonEmpty(snapshot.Message, "Deployment rollout completed"))
			return r.finishDeployRelease(release, "succeeded", firstNonEmpty(snapshot.Message, "Deployment rollout completed"))
		}
		return r.db.Model(&model.Release{}).Where("id = ?", release.ID).Updates(map[string]any{
			"status":  "running",
			"message": firstNonEmpty(snapshot.Message, release.Message),
		}).Error
	}
	return nil
}

func deploymentTargetWorkloadKind(target model.DeploymentTarget) string {
	switch strings.ToLower(strings.TrimSpace(target.WorkloadType)) {
	case "statefulset", "stateful-set":
		return "StatefulSet"
	default:
		return "Deployment"
	}
}

func (r *Runner) markReleaseRuntimeDrift(release model.Release, message string) error {
	if err := r.finishDeployRelease(release, "failed", message); err != nil {
		return err
	}
	r.appendReleaseLog(release, "运行态漂移: "+message)
	return nil
}

func expiredBuildJobUpdates(finishedAt time.Time) map[string]any {
	return map[string]any{
		"status":      "lost",
		"message":     "lease_expired",
		"lease_token": "",
		"lease_until": nil,
		"finished_at": &finishedAt,
	}
}
