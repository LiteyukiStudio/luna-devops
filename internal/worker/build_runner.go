package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/buildruntime"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"k8s.io/client-go/kubernetes"
)

const (
	buildJobAppName            = "luna-build-job"
	buildJobServiceAccountName = "luna-build-job"
	buildJobScope              = "build"
	defaultBuildCPURequest     = "2"
	defaultBuildMemoryRequest  = "4Gi"
	defaultBuildTimeoutSeconds = int64(1800)
)

func (r *Runner) handleBuildRun(ctx context.Context, task *asynq.Task) error {
	var payload tasks.BuildRunPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	// Build tasks are not retried by the queue, so every early exit below must
	// leave the run in a terminal state instead of an indefinitely queued one.
	payloadJob := model.BuildJob{ID: payload.BuildJobID, BuildRunID: payload.BuildRunID, ProjectID: payload.ProjectID}
	payloadRun := model.BuildRun{ID: payload.BuildRunID, ProjectID: payload.ProjectID}
	var run model.BuildRun
	if err := r.db.WithContext(ctx).First(&run, "id = ? and project_id = ?", payload.BuildRunID, payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		_ = r.failBuildJob(ctx, payloadJob, payloadRun, model.BuildFailureInternalError, "load build run: "+err.Error())
		return err
	}
	var job model.BuildJob
	if err := r.db.WithContext(ctx).First(&job, "id = ? and build_run_id = ? and project_id = ?", payload.BuildJobID, run.ID, run.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = r.failBuildJob(ctx, payloadJob, run, model.BuildFailureInternalError, "build job not found")
			return nil
		}
		_ = r.failBuildJob(ctx, payloadJob, run, model.BuildFailureInternalError, "load build job: "+err.Error())
		return err
	}
	if !buildJobCanStart(job.Status) || run.Status == "canceled" {
		return nil
	}
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, "id = ?", run.ProjectID).Error; err != nil {
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureProjectUnavailable, "load project: "+err.Error())
		return ignoreRecordNotFound(err)
	}
	if !model.DeleteStatusActive(project.DeleteStatus) {
		return r.failBuildJob(ctx, job, run, model.BuildFailureProjectUnavailable, "project is being deleted")
	}
	var target model.DeploymentTarget
	if err := r.db.WithContext(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", run.DeploymentTargetID, run.ProjectID, run.ApplicationID).Error; err != nil {
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureTargetUnavailable, "load deployment target: "+err.Error())
		return ignoreRecordNotFound(err)
	}
	if !model.DeleteStatusActive(target.DeleteStatus) {
		return r.failBuildJob(ctx, job, run, model.BuildFailureTargetUnavailable, "deployment target is being deleted")
	}
	cluster, err := r.runtimeClusterForDeploymentTarget(ctx, target)
	if err != nil {
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureClusterUnavailable, err.Error())
		return err
	}
	if err := workerStage(ctx, "build.ensure_capacity", func(stageCtx context.Context) error {
		if err := r.applyBuildConcurrencyPolicy(stageCtx, run, target); err != nil {
			return err
		}
		return r.ensureBuildCapacity(project, cluster, target)
	}); err != nil {
		if errors.Is(err, errBuildCapacityUnavailable) && r.taskClient != nil {
			_ = r.db.WithContext(ctx).Model(&model.BuildJob{}).Where("id = ? and status = ?", job.ID, "queued").Update("message", buildCapacityMessage(err)).Error
			if _, enqueueErr := r.taskClient.EnqueueBuildRunAfter(ctx, payload, buildCapacityRetryDelay); enqueueErr != nil {
				_ = r.failBuildJob(ctx, job, run, model.BuildFailureDispatchFailed, "requeue after capacity wait: "+enqueueErr.Error())
				return enqueueErr
			}
			return nil
		}
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureCapacityCheckFailed, err.Error())
		return err
	}
	namespace := deploymentNamespace(project)
	if err := workerStage(ctx, "build.ensure_namespace", func(stageCtx context.Context) error {
		return r.ensureProjectNamespace(stageCtx, namespace, project, target)
	}); err != nil {
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureNamespaceFailed, "namespace prepare failed: "+err.Error())
		return err
	}
	client, err := r.kubernetesClient(ctx, target)
	if err != nil {
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureClusterUnavailable, err.Error())
		return err
	}

	resolved, err := workerStageValue(ctx, "build.resolve_task", func(stageCtx context.Context) (buildruntime.ResolvedTask, error) {
		return (buildruntime.Resolver{DB: r.db, Secrets: r.secrets}).ResolveBuildTask(stageCtx, r.db.WithContext(stageCtx), run, job)
	})
	if err != nil {
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureTaskResolveFailed, err.Error())
		return err
	}
	taskPayload := resolved.Task
	jobName := buildKubernetesJobName(job.ID)
	secretName := jobName + "-secret"
	if err := workerStage(ctx, "build.start_job", func(stageCtx context.Context) error {
		return r.startBuildJob(stageCtx, client, namespace, secretName, jobName, run, taskPayload)
	}); err != nil {
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureJobStartFailed, err.Error())
		return err
	}
	defer r.cleanupBuildJobSecrets(context.WithoutCancel(ctx), client, namespace, secretName)
	now := time.Now()
	if err := r.markBuildJobRunning(ctx, job, run, jobName, namespace, taskPayload.Registry.ImageRef, now); err != nil {
		_ = r.deleteKubernetesBuildJob(context.WithoutCancel(ctx), client, namespace, jobName)
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureInternalError, "mark build running: "+err.Error())
		return err
	}
	run.Status = "running"
	run.ImageRef = taskPayload.Registry.ImageRef
	run.StartedAt = &now
	r.emitBuildEvent(ctx, run, "started", "Build started")

	result, err := workerStageValue(ctx, "build.follow_job", func(stageCtx context.Context) (builder.Result, error) {
		return r.followBuildJob(stageCtx, client, namespace, jobName, job, run, taskPayload.Build.Hooks, resolved.SensitiveValues)
	})
	if err != nil {
		if errors.Is(err, errBuildRunCanceled) {
			_ = r.settleBuildUsage(ctx, job.ID, run.ID, run.ProjectID, target)
			return nil
		}
		if errors.Is(err, errBuildJobTimedOut) {
			_ = r.finishBuildJobUnsuccessfully(ctx, job, run, "timeout", model.BuildFailureTimeout, err.Error())
		} else {
			_ = r.failBuildJob(ctx, job, run, buildFollowFailureCode(err), err.Error())
		}
		_ = r.settleBuildUsage(ctx, job.ID, run.ID, run.ProjectID, target)
		return err
	}
	if strings.TrimSpace(result.ImageRef) == "" {
		result.ImageRef = taskPayload.Registry.ImageRef
	}
	completedRun, err := workerStageValue(ctx, "build.complete_job", func(context.Context) (model.BuildRun, error) {
		return r.completeBuildJob(ctx, job, run, result)
	})
	if err != nil {
		_ = r.failBuildJob(ctx, job, run, model.BuildFailureInternalError, "complete build: "+err.Error())
		_ = r.settleBuildUsage(ctx, job.ID, run.ID, run.ProjectID, target)
		return err
	}
	_ = r.settleBuildUsage(ctx, job.ID, run.ID, run.ProjectID, target)
	if completedRun.ID != "" {
		r.emitBuildEvent(ctx, completedRun, "succeeded", "Build succeeded")
		r.enqueueAutoDeploymentsForBuildRun(ctx, completedRun)
	}
	return nil
}

// markBuildJobRunning records that the Kubernetes Job was created. Both rows
// only move forward from queued so a concurrent cancel or timeout wins.
func (r *Runner) markBuildJobRunning(ctx context.Context, job model.BuildJob, run model.BuildRun, jobName string, namespace string, imageRef string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.BuildJob{}).Where("id = ? and status = ?", job.ID, "queued").Updates(map[string]any{
			"status":        "running",
			"message":       "kubernetes_job_started",
			"executor_id":   jobName,
			"executor_name": "kubernetes job",
			"log_ref":       "kubernetes-job:" + namespace + "/" + jobName,
			"attempts":      gorm.Expr("attempts + 1"),
			"started_at":    &now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.BuildRun{}).Where("id = ? and status = ?", run.ID, "queued").Updates(map[string]any{
			"status":     "running",
			"image_ref":  imageRef,
			"started_at": &now,
		}).Error
	})
}

func ignoreRecordNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (r *Runner) settleBuildUsage(ctx context.Context, jobID string, runID string, projectID string, target model.DeploymentTarget) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "worker", "build.settle_usage")
	defer func() { end(err) }()
	var run model.BuildRun
	if err := r.db.First(&run, "id = ? and project_id = ?", runID, projectID).Error; err != nil {
		return err
	}
	var job model.BuildJob
	if err := r.db.First(&job, "id = ? and build_run_id = ? and project_id = ?", jobID, runID, projectID).Error; err != nil {
		return err
	}
	finishedAt := time.Now()
	if run.FinishedAt != nil {
		finishedAt = *run.FinishedAt
	}
	err = (billing.Service{DB: r.db}).SettleBuildRun(billing.BuildUsageInput{
		Run:              run,
		Job:              job,
		DeploymentTarget: target,
		FinishedAt:       finishedAt,
	})
	if err != nil && !errors.Is(err, billing.ErrAlreadySettled) {
		message := strings.TrimSpace(job.Message)
		if message != "" {
			message += "; "
		}
		message += "billing settlement failed: " + err.Error()
		_ = r.db.Model(&model.BuildJob{}).Where("id = ? and project_id = ?", job.ID, projectID).Update("message", message).Error
		return err
	}
	return nil
}

func buildJobCanStart(status string) bool {
	return status == "queued"
}

func (r *Runner) kubernetesClient(ctx context.Context, target model.DeploymentTarget) (kubernetes.Interface, error) {
	kubeconfig, err := r.kubeconfigForDeploymentTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	restConfig, err := kubeprovider.SafeRESTConfigFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, runtimeClusterKubeconfigError(err)
	}
	restConfig.Wrap(telemetry.InstrumentHTTPTransport)
	return kubernetes.NewForConfig(restConfig)
}
