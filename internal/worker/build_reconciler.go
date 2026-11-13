package worker

import (
	"context"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
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
		err := r.scoped(reconcileCtx).reclaimExpiredBuildJobs(reconcileCtx)
		end(err)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// buildQueueTimeoutFactor bounds how long a build may wait in the queue
// relative to its own execution timeout before it is reclaimed.
const buildQueueTimeoutFactor = 2

type expiredBuildJob struct {
	job  model.BuildJob
	run  model.BuildRun
	code string
}

// reclaimExpiredBuildJobs moves builds that exceeded their execution or queue
// deadline into the timeout state. Running builds additionally have their
// Kubernetes Job removed and their usage settled.
func (r *Runner) reclaimExpiredBuildJobs(ctx context.Context) error {
	now := time.Now()
	defaultTimeout := effectiveBuildTimeoutSeconds(0, r.buildJobTimeoutSeconds)
	var reclaimed []expiredBuildJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var running []model.BuildJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Joins("join build_runs on build_runs.id = build_jobs.build_run_id").
			Where("build_jobs.status = ?", "running").
			Where(
				"build_jobs.started_at is not null and build_jobs.started_at < (?::timestamptz - (coalesce(nullif(build_runs.build_timeout_seconds, 0), ?) * interval '1 second'))",
				now,
				defaultTimeout,
			).
			Order("build_jobs.started_at asc").
			Limit(50).
			Find(&running).Error; err != nil {
			return err
		}
		var queued []model.BuildJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Joins("join build_runs on build_runs.id = build_jobs.build_run_id").
			Where("build_jobs.status = ?", "queued").
			Where(
				"build_jobs.created_at < (?::timestamptz - (coalesce(nullif(build_runs.build_timeout_seconds, 0), ?) * ? * interval '1 second'))",
				now,
				defaultTimeout,
				buildQueueTimeoutFactor,
			).
			Order("build_jobs.created_at asc").
			Limit(50).
			Find(&queued).Error; err != nil {
			return err
		}
		candidates := make([]expiredBuildJob, 0, len(running)+len(queued))
		for _, job := range running {
			candidates = append(candidates, expiredBuildJob{job: job, code: model.BuildFailureTimeout})
		}
		for _, job := range queued {
			candidates = append(candidates, expiredBuildJob{job: job, code: model.BuildFailureQueueTimeout})
		}
		for _, candidate := range candidates {
			job := candidate.job
			var run model.BuildRun
			if err := tx.First(&run, "id = ? and project_id = ?", job.BuildRunID, job.ProjectID).Error; err != nil {
				return err
			}
			finishedAt := now
			updates := expiredBuildJobUpdates(candidate.code, finishedAt)
			if err := tx.Model(&model.BuildJob{}).
				Where("id = ? and status = ?", job.ID, job.Status).
				Updates(updates).Error; err != nil {
				return err
			}
			result := tx.Model(&model.BuildRun{}).
				Where("id = ? and project_id = ? and status in ?", job.BuildRunID, job.ProjectID, model.BuildActiveStatuses).
				Updates(map[string]any{
					"status":       "timeout",
					"failure_code": candidate.code,
					"finished_at":  &finishedAt,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			run.Status = "timeout"
			run.FailureCode = candidate.code
			run.FinishedAt = &finishedAt
			candidate.run = run
			reclaimed = append(reclaimed, candidate)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, item := range reclaimed {
		r.recordBuildRunMetrics(ctx, item.run)
		r.emitBuildFailed(ctx, item.run, item.code)
		if item.job.Status == "running" {
			_ = workerStage(ctx, "build.reclaim_job", func(stageCtx context.Context) error {
				return r.releaseExpiredBuildJob(stageCtx, item.job, item.run)
			})
		}
	}
	return nil
}

// releaseExpiredBuildJob removes the Kubernetes Job of a timed-out build and
// settles the usage accumulated until the timeout.
func (r *Runner) releaseExpiredBuildJob(ctx context.Context, job model.BuildJob, run model.BuildRun) error {
	var target model.DeploymentTarget
	targetErr := r.db.WithContext(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", run.DeploymentTargetID, run.ProjectID, run.ApplicationID).Error
	var cleanupErr error
	if targetErr == nil {
		cleanupErr = r.deleteExpiredBuildJobResources(ctx, job, run, target)
	}
	settleErr := r.settleBuildUsage(ctx, job.ID, run.ID, run.ProjectID, target)
	return errors.Join(ignoreRecordNotFound(targetErr), cleanupErr, settleErr)
}

func (r *Runner) deleteExpiredBuildJobResources(ctx context.Context, job model.BuildJob, run model.BuildRun, target model.DeploymentTarget) error {
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, "id = ?", run.ProjectID).Error; err != nil {
		return err
	}
	client, err := r.kubernetesClient(ctx, target)
	if err != nil {
		return err
	}
	namespace := deploymentNamespace(project)
	jobName := buildKubernetesJobName(job.ID)
	return errors.Join(
		r.deleteKubernetesBuildJob(ctx, client, namespace, jobName),
		r.cleanupBuildJobSecrets(ctx, client, namespace, jobName+"-secret"),
	)
}

func (r *Runner) handleSyncStatus(ctx context.Context, task *asynq.Task) error {
	if err := workerStage(ctx, "runtime.sync_releases", r.syncReleaseRuntimeStatus); err != nil {
		return err
	}
	if err := workerStage(ctx, "cleanup.recover_stale", r.recoverStaleResourceCleanups); err != nil {
		return err
	}
	_ = workerStage(ctx, "runtime.patrol_workloads", r.patrolRuntimeWorkloads)
	_ = workerStage(ctx, "gateway.patrol_certificates", r.patrolGatewayCertificates)
	return nil
}

// releaseWorkflowTimeout is the total deadline of one release measured from
// its start (or creation while still pending). It covers queue retries of the
// deploy task, after which the release is failed with a stable timeout code.
func (r *Runner) releaseWorkflowTimeout() time.Duration {
	budget := tasks.PolicyForType(tasks.TypeDeployRun).Timeout
	if rollout := time.Duration(r.deployRolloutTimeoutSeconds) * time.Second; rollout > budget {
		budget = rollout
	}
	return 2 * budget
}

// syncReleaseRuntimeStatus is the safety net for releases whose deploy task is
// no longer driving them. It never touches pending releases or releases still
// inside the deploy task window, and it only judges a release by a workload
// whose pod template carries that release ID.
func (r *Runner) syncReleaseRuntimeStatus(ctx context.Context) error {
	now := time.Now()
	if err := r.expireStaleReleases(ctx, now); err != nil {
		return err
	}
	observeBefore := now.Add(-tasks.PolicyForType(tasks.TypeDeployRun).Timeout)
	var releases []model.Release
	if err := r.db.WithContext(ctx).
		Where("status = ? and started_at is not null and started_at < ?", "running", observeBefore).
		Order("started_at asc").
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

func (r *Runner) expireStaleReleases(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-r.releaseWorkflowTimeout())
	var releases []model.Release
	if err := r.db.WithContext(ctx).
		Where("(status = ? and started_at is not null and started_at < ?) or (status = ? and created_at < ?)", "running", cutoff, "pending", cutoff).
		Order("created_at asc").
		Limit(100).
		Find(&releases).Error; err != nil {
		return err
	}
	for _, release := range releases {
		transitioned, err := r.finishDeployRelease(ctx, release, "failed", model.ReleaseFailureTimeout, "release workflow timed out")
		if err != nil {
			return err
		}
		if transitioned {
			r.appendReleaseLog(ctx, release, "发布超过总时限，已标记为失败")
		}
	}
	return nil
}

func (r *Runner) syncReleaseRuntimeSnapshot(ctx context.Context, release model.Release) error {
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, "id = ?", release.ProjectID).Error; err != nil {
		return err
	}
	deploymentTarget, err := r.releaseDeploymentTarget(ctx, release)
	if err != nil {
		return err
	}
	manager, err := r.kubernetesManager(ctx, deploymentTarget)
	if err != nil {
		return err
	}
	namespace := deploymentNamespace(project)
	resourceName := applicationResourceName(deploymentTarget)
	snapshot, err := manager.GetDeploymentSnapshot(ctx, namespace, resourceName)
	if err != nil {
		// A missing or unreachable workload is not evidence about this release;
		// the release workflow timeout settles it.
		return err
	}
	if snapshot.ReleaseID != release.ID {
		superseded, err := r.releaseSupersededBy(ctx, release, snapshot.ReleaseID)
		if err != nil || !superseded {
			return err
		}
		return r.markReleaseRolloutFailed(ctx, release, model.ReleaseFailureSuperseded, "workload now belongs to newer release "+snapshot.ReleaseID)
	}
	r.recordDeploymentRuntimeMetric(ctx, snapshot)
	if snapshot.Phase == kubeprovider.DeploymentFailed {
		return r.markReleaseRolloutFailed(ctx, release, model.ReleaseFailureRolloutFailed, firstNonEmpty(snapshot.Message, "Deployment runtime check failed"))
	}
	if snapshot.Phase == kubeprovider.DeploymentSucceeded {
		message := firstNonEmpty(snapshot.Message, "Deployment rollout completed")
		transitioned, err := r.finishDeployRelease(ctx, release, "succeeded", "", message)
		if err == nil && transitioned {
			r.appendReleaseLog(ctx, release, message)
		}
		return err
	}
	return r.db.WithContext(ctx).Model(&model.Release{}).Where("id = ? and status = ?", release.ID, "running").Updates(map[string]any{
		"message": firstNonEmpty(snapshot.Message, release.Message),
	}).Error
}

// releaseSupersededBy reports whether the workload was taken over by a newer
// release of the same deployment target.
func (r *Runner) releaseSupersededBy(ctx context.Context, release model.Release, workloadReleaseID string) (bool, error) {
	if workloadReleaseID == "" {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Release{}).
		Where("id = ? and project_id = ? and deployment_target_id = ? and created_at > ?", workloadReleaseID, release.ProjectID, release.DeploymentTargetID, release.CreatedAt).
		Count(&count).Error
	return count > 0, err
}

func (r *Runner) markReleaseRolloutFailed(ctx context.Context, release model.Release, code string, message string) error {
	transitioned, err := r.finishDeployRelease(ctx, release, "failed", code, message)
	if err == nil && transitioned {
		r.appendReleaseLog(ctx, release, "发布收敛失败: "+message)
	}
	return err
}

func expiredBuildJobUpdates(code string, finishedAt time.Time) map[string]any {
	return map[string]any{
		"status":       "timeout",
		"failure_code": code,
		"message":      code,
		"finished_at":  &finishedAt,
	}
}
