package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Runner) syncBuildJobStatus(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		reconcileCtx, end := telemetry.StartOperation(ctx, "worker", "build.reconcile_expired")
		err := r.scoped(reconcileCtx).markExpiredBuildJobsLost()
		end(err)
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
	if err := workerStage(ctx, "runtime.sync_releases", r.syncReleaseRuntimeStatus); err != nil {
		return err
	}
	r.retryPendingResourceCleanups(ctx)
	return nil
}

func (r *Runner) syncReleaseRuntimeStatus(ctx context.Context) error {
	if r.db == nil {
		return nil
	}
	var releases []model.Release
	if err := r.db.
		Where("status in ?", []string{"pending", "running"}).
		Order("created_at desc").
		Limit(200).
		Find(&releases).Error; err != nil {
		return err
	}
	for _, release := range releases {
		_ = workerStage(ctx, "runtime.observe_release", func(stageCtx context.Context) error {
			return r.syncReleaseRuntimeSnapshot(stageCtx, release)
		}, attribute.String("release.id", release.ID))
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
			return r.markReleaseRolloutFailed(ctx, release, message)
		}
		return err
	}
	r.recordDeploymentRuntimeMetric(snapshot)
	if snapshot.Phase == kubeprovider.DeploymentFailed {
		return r.markReleaseRolloutFailed(ctx, release, firstNonEmpty(snapshot.Message, "Deployment runtime check failed"))
	}
	if snapshot.Phase == kubeprovider.DeploymentSucceeded {
		r.appendReleaseLog(release, firstNonEmpty(snapshot.Message, "Deployment rollout completed"))
		return r.finishDeployRelease(ctx, release, "succeeded", firstNonEmpty(snapshot.Message, "Deployment rollout completed"))
	}
	return r.db.Model(&model.Release{}).Where("id = ?", release.ID).Updates(map[string]any{
		"status":  "running",
		"message": firstNonEmpty(snapshot.Message, release.Message),
	}).Error
}

func deploymentTargetWorkloadKind(target model.DeploymentTarget) string {
	switch strings.ToLower(strings.TrimSpace(target.WorkloadType)) {
	case "statefulset", "stateful-set":
		return "StatefulSet"
	default:
		return "Deployment"
	}
}

func (r *Runner) markReleaseRolloutFailed(ctx context.Context, release model.Release, message string) error {
	if err := r.finishDeployRelease(ctx, release, "failed", message); err != nil {
		return err
	}
	r.appendReleaseLog(release, "发布收敛失败: "+message)
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
