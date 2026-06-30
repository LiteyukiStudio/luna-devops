package worker

import (
	"context"
	"fmt"
	"log"
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
	r.refreshGatewayRouteMetrics()
	r.retryPendingResourceCleanups(ctx)
	return nil
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
			message := fmt.Sprintf("deployment_missing: Kubernetes Deployment %s/%s not found", namespace, resourceName)
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
