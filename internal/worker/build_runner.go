package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/buildruntime"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
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
	var run model.BuildRun
	if err := r.db.First(&run, "id = ? and project_id = ?", payload.BuildRunID, payload.ProjectID).Error; err != nil {
		return err
	}
	var job model.BuildJob
	if err := r.db.First(&job, "id = ? and build_run_id = ? and project_id = ?", payload.BuildJobID, run.ID, run.ProjectID).Error; err != nil {
		return err
	}
	if !buildJobCanStart(job.Status) || run.Status == "canceled" {
		return nil
	}
	var project model.Project
	if err := r.db.First(&project, "id = ?", run.ProjectID).Error; err != nil {
		return err
	}
	var target model.DeploymentTarget
	if err := r.db.First(&target, "id = ? and project_id = ? and application_id = ?", run.DeploymentTargetID, run.ProjectID, run.ApplicationID).Error; err != nil {
		return err
	}
	environment := deploymentTargetEnvironment(target)
	cluster, err := r.runtimeClusterForEnvironment(environment)
	if err != nil {
		_ = r.failBuildJob(job, run, err.Error())
		return err
	}
	if err := r.ensureBuildCapacity(project, cluster, environment); err != nil {
		if errors.Is(err, errBuildCapacityUnavailable) {
			_ = r.db.Model(&model.BuildJob{}).Where("id = ? and status = ?", job.ID, "queued").Update("message", buildCapacityMessage(err)).Error
			if r.taskClient != nil {
				if _, enqueueErr := r.taskClient.EnqueueBuildRunAfter(ctx, payload, buildCapacityRetryDelay); enqueueErr != nil {
					return enqueueErr
				}
				return nil
			}
		}
		return err
	}
	namespace := deploymentNamespace(project, environment)
	if err := r.ensureProjectNamespace(ctx, namespace, project, environment); err != nil {
		_ = r.failBuildJob(job, run, "namespace prepare failed: "+err.Error())
		return err
	}
	client, err := r.kubernetesClient(environment)
	if err != nil {
		_ = r.failBuildJob(job, run, err.Error())
		return err
	}

	resolved, err := (buildruntime.Resolver{DB: r.db, Secrets: r.secrets}).ResolveBuildTask(r.db, run, job)
	if err != nil {
		_ = r.failBuildJob(job, run, err.Error())
		return err
	}
	taskPayload := resolved.Task
	jobName := buildKubernetesJobName(job.ID)
	secretName := jobName + "-secret"
	if err := r.startBuildJob(ctx, client, namespace, secretName, jobName, environment, run, taskPayload); err != nil {
		_ = r.failBuildJob(job, run, err.Error())
		return err
	}
	defer r.cleanupBuildJobSecrets(context.Background(), client, namespace, secretName)
	now := time.Now()
	if err := r.db.Model(&model.BuildJob{}).Where("id = ? and status = ?", job.ID, "queued").Updates(map[string]any{
		"status":            "running",
		"message":           "kubernetes_job_started",
		"executor_id":       jobName,
		"executor_name":     "kubernetes job",
		"log_ref":           "kubernetes-job:" + namespace + "/" + jobName,
		"attempts":          gorm.Expr("attempts + 1"),
		"started_at":        &now,
		"last_heartbeat_at": &now,
	}).Error; err != nil {
		return err
	}
	if err := r.db.Model(&model.BuildRun{}).Where("id = ? and status = ?", run.ID, "queued").Updates(map[string]any{
		"status":     "running",
		"image_ref":  taskPayload.Registry.ImageRef,
		"started_at": &now,
	}).Error; err != nil {
		return err
	}
	run.Status = "running"
	run.ImageRef = taskPayload.Registry.ImageRef
	run.StartedAt = &now
	r.emitBuildEvent(ctx, run, "started", "Build started")

	result, err := r.followBuildJob(ctx, client, namespace, jobName, job, run, taskPayload.Build.Hooks, resolved.SensitiveValues)
	if err != nil {
		if errors.Is(err, errBuildRunCanceled) {
			r.settleBuildUsage(job.ID, run.ID, run.ProjectID, environment)
			return nil
		}
		_ = r.failBuildJob(job, run, err.Error())
		r.settleBuildUsage(job.ID, run.ID, run.ProjectID, environment)
		return err
	}
	if strings.TrimSpace(result.ImageRef) == "" {
		result.ImageRef = taskPayload.Registry.ImageRef
	}
	completedRun, err := r.completeBuildJob(job, run, result)
	if err != nil {
		return err
	}
	r.settleBuildUsage(job.ID, run.ID, run.ProjectID, environment)
	if completedRun.ID != "" {
		r.emitBuildEvent(ctx, completedRun, "succeeded", "Build succeeded")
		r.enqueueAutoDeploymentsForBuildRun(ctx, completedRun)
	}
	return nil
}

func (r *Runner) settleBuildUsage(jobID string, runID string, projectID string, environment model.Environment) {
	var run model.BuildRun
	if err := r.db.First(&run, "id = ? and project_id = ?", runID, projectID).Error; err != nil {
		return
	}
	var job model.BuildJob
	if err := r.db.First(&job, "id = ? and build_run_id = ? and project_id = ?", jobID, runID, projectID).Error; err != nil {
		return
	}
	finishedAt := time.Now()
	if run.FinishedAt != nil {
		finishedAt = *run.FinishedAt
	}
	buildEnvironment := environment
	err := (billing.Service{DB: r.db}).SettleBuildRun(billing.BuildUsageInput{
		Run:         run,
		Job:         job,
		Environment: buildEnvironment,
		FinishedAt:  finishedAt,
	})
	if err != nil && !errors.Is(err, billing.ErrAlreadySettled) {
		message := strings.TrimSpace(job.Message)
		if message != "" {
			message += "; "
		}
		message += "billing settlement failed: " + err.Error()
		_ = r.db.Model(&model.BuildJob{}).Where("id = ? and project_id = ?", job.ID, projectID).Update("message", message).Error
	}
}

func buildJobCanStart(status string) bool {
	return status == "queued"
}

func (r *Runner) kubernetesClient(environment model.Environment) (kubernetes.Interface, error) {
	kubeconfig, err := r.kubeconfigForEnvironment(environment)
	if err != nil {
		return nil, err
	}
	restConfig, err := kubeprovider.SafeRESTConfigFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, runtimeClusterKubeconfigError(err)
	}
	return kubernetes.NewForConfig(restConfig)
}
