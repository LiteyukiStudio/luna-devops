package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/buildruntime"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	buildJobAppName            = "liteyuki-build-job"
	buildJobServiceAccountName = "liteyuki-build-job"
	buildJobScope              = "build"
	defaultBuildCPURequest     = "2"
	defaultBuildMemoryRequest  = "4Gi"
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

	result, err := r.followBuildJob(ctx, client, namespace, jobName, job, taskPayload.Build.Hooks, resolved.SensitiveValues)
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
	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, runtimeClusterKubeconfigError(err)
	}
	return kubernetes.NewForConfig(restConfig)
}

func (r *Runner) startBuildJob(ctx context.Context, client kubernetes.Interface, namespace string, secretName string, jobName string, environment model.Environment, run model.BuildRun, task builder.Task) error {
	if err := ensureBuildJobServiceAccount(ctx, client, namespace); err != nil {
		return err
	}
	secret := buildJobSecret(secretName, task, r.buildNPMRegistry, r.buildCacheEnabled, r.buildCacheTag)
	secrets := client.CoreV1().Secrets(namespace)
	if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		if err := secrets.Delete(ctx, secretName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return err
		}
	}
	cleanupSecret := func() {
		_ = secrets.Delete(context.Background(), secretName, metav1.DeleteOptions{})
	}
	job := buildJobSpec(jobName, secretName, environment, run, task, r.buildExecutorImage, r.buildNPMRegistry, r.buildCacheEnabled, r.buildCacheTag, r.buildJobTTLSeconds)
	jobs := client.BatchV1().Jobs(namespace)
	if _, err := jobs.Create(ctx, job, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			cleanupSecret()
			return err
		}
		propagation := metav1.DeletePropagationBackground
		if err := jobs.Delete(ctx, jobName, metav1.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !apierrors.IsNotFound(err) {
			cleanupSecret()
			return err
		}
		if _, err := jobs.Create(ctx, job, metav1.CreateOptions{}); err != nil {
			cleanupSecret()
			return err
		}
	}
	return nil
}

func ensureBuildJobServiceAccount(ctx context.Context, client kubernetes.Interface, namespace string) error {
	automount := false
	labels := buildJobBaseLabels()
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   buildJobServiceAccountName,
			Labels: labels,
		},
		AutomountServiceAccountToken: &automount,
	}
	serviceAccounts := client.CoreV1().ServiceAccounts(namespace)
	existing, err := serviceAccounts.Get(ctx, buildJobServiceAccountName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = serviceAccounts.Create(ctx, serviceAccount, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	changed := false
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for key, value := range labels {
		if existing.Labels[key] == value {
			continue
		}
		existing.Labels[key] = value
		changed = true
	}
	if existing.AutomountServiceAccountToken == nil || *existing.AutomountServiceAccountToken {
		existing.AutomountServiceAccountToken = &automount
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = serviceAccounts.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func buildJobBaseLabels() map[string]string {
	return map[string]string{
		kubeprovider.ManagedByLabel:     kubeprovider.ManagedByValue,
		kubeprovider.ApplicationNameKey: buildJobAppName,
		kubeprovider.ScopeLabel:         buildJobScope,
	}
}

func buildJobSecret(name string, task builder.Task, npmRegistry string, cacheEnabled bool, cacheTag string) *corev1.Secret {
	env := map[string]string{
		"GIT_CLONE_URL":                 task.Repository.CloneURL,
		"GIT_ACCESS_TOKEN":              task.Repository.AccessToken,
		"SOURCE_BRANCH":                 task.Repository.SourceBranch,
		"SOURCE_TAG":                    task.Repository.SourceTag,
		"SOURCE_COMMIT":                 task.Repository.SourceCommit,
		"LITEYUKI_PROJECT_ID":           task.ProjectID,
		"LITEYUKI_APPLICATION_ID":       task.ApplicationID,
		"LITEYUKI_DEPLOYMENT_TARGET_ID": task.DeploymentTargetID,
		"LITEYUKI_BUILD_RUN_ID":         task.BuildRunID,
		"LITEYUKI_BUILD_JOB_ID":         task.JobID,
		"DOCKERFILE_PATH":               builder.StringDefault(task.Build.DockerfilePath, "Dockerfile"),
		"BUILD_CONTEXT":                 builder.StringDefault(task.Build.BuildContext, "."),
		"BUILD_DIRECTORY":               task.Build.BuildDirectory,
		"CACHE_ENABLED":                 builder.BoolEnvValue(cacheEnabled),
		"CACHE_TAG":                     builder.StringDefault(strings.TrimSpace(cacheTag), "buildcache"),
		"NPM_REGISTRY":                  strings.TrimSpace(npmRegistry),
		"REGISTRY_ENDPOINT":             task.Registry.Endpoint,
		"REGISTRY_USERNAME":             task.Registry.Username,
		"REGISTRY_PASSWORD":             task.Registry.Password,
		"IMAGE_REF":                     task.Registry.ImageRef,
		"IMAGE_NAME_PREFIX":             task.Registry.ImageNamePrefix,
		"IMAGE_TAG_TEMPLATE":            task.Registry.ImageTagTemplate,
	}
	hookIDsByPhase := builder.HookIDsByPhase(task.Build.Hooks)
	env["PRE_PULL_HOOK_IDS"] = strings.Join(hookIDsByPhase["prePull"], ",")
	env["POST_PULL_HOOK_IDS"] = strings.Join(hookIDsByPhase["postPull"], ",")
	env["PRE_BUILD_HOOK_IDS"] = strings.Join(hookIDsByPhase["preBuild"], ",")
	env["POST_BUILD_HOOK_IDS"] = strings.Join(hookIDsByPhase["postBuild"], ",")
	env["PRE_PUSH_HOOK_IDS"] = strings.Join(hookIDsByPhase["prePush"], ",")
	env["POST_PUSH_HOOK_IDS"] = strings.Join(hookIDsByPhase["postPush"], ",")
	buildEnv := builder.NormalizedBuildEnv(task.Build.Env)
	if strings.TrimSpace(npmRegistry) != "" {
		if _, ok := buildEnv["NPM_REGISTRY"]; !ok {
			buildEnv["NPM_REGISTRY"] = strings.TrimSpace(npmRegistry)
		}
		if _, ok := buildEnv["npm_config_registry"]; !ok {
			buildEnv["npm_config_registry"] = strings.TrimSpace(npmRegistry)
		}
	}
	buildEnvKeys := make([]string, 0, len(buildEnv))
	for key, value := range buildEnv {
		env[key] = value
		buildEnvKeys = append(buildEnvKeys, key)
	}
	env["BUILD_ENV_KEYS"] = strings.Join(buildEnvKeys, ",")

	data := map[string]string{"run.sh": builder.ExecutorScript()}
	for _, hook := range task.Build.Hooks {
		if strings.TrimSpace(hook.ID) == "" || strings.TrimSpace(hook.Script) == "" {
			continue
		}
		data[hook.ID+".sh"] = hook.Script
		data[hook.ID+".meta"] = builder.HookMetadataEnv(hook)
	}
	for key, value := range env {
		data["env-"+key] = value
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Type:       corev1.SecretTypeOpaque,
		StringData: data,
	}
}

func buildJobSpec(jobName string, secretName string, environment model.Environment, run model.BuildRun, task builder.Task, image string, npmRegistry string, cacheEnabled bool, cacheTag string, ttlSeconds int64) *batchv1.Job {
	backoffLimit := int32(0)
	ttl := int32(ttlSeconds)
	runAsUser := int64(1000)
	runAsGroup := int64(1000)
	runAsNonRoot := true
	allowPrivilegeEscalation := true
	mode := int32(0o555)
	labels := buildJobBaseLabels()
	labels[kubeprovider.ProjectIDLabel] = task.ProjectID
	labels[kubeprovider.ApplicationIDLabel] = task.ApplicationID
	labels[kubeprovider.EnvironmentIDLabel] = environment.ID
	labels[kubeprovider.DeploymentTargetIDLabel] = task.DeploymentTargetID
	labels["liteyuki.devops/build-run-id"] = task.BuildRunID
	labels["liteyuki.devops/build-job-id"] = task.JobID
	items := []corev1.KeyToPath{{Key: "run.sh", Path: "run.sh", Mode: &mode}}
	for _, hook := range task.Build.Hooks {
		if strings.TrimSpace(hook.ID) == "" || strings.TrimSpace(hook.Script) == "" {
			continue
		}
		items = append(items,
			corev1.KeyToPath{Key: hook.ID + ".sh", Path: "hooks/" + hook.ID + ".sh", Mode: &mode},
			corev1.KeyToPath{Key: hook.ID + ".meta", Path: "hooks/" + hook.ID + ".meta"},
		)
	}
	env := []corev1.EnvVar{
		{Name: "HOME", Value: "/workspace/home"},
		{Name: "BUILDKITD_FLAGS", Value: "--oci-worker-no-process-sandbox"},
	}
	resources := buildJobResourceRequirements(run)
	for key := range buildJobSecret("", task, npmRegistry, cacheEnabled, cacheTag).StringData {
		if strings.HasPrefix(key, "env-") {
			env = append(env, corev1.EnvVar{
				Name: strings.TrimPrefix(key, "env-"),
				ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  key,
				}},
			})
		}
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName:           buildJobServiceAccountName,
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: boolPtr(false),
					SecurityContext: &corev1.PodSecurityContext{
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{{
						Name:            "executor",
						Image:           image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"/bin/sh", "-ec", "mkdir -p /workspace/hooks /workspace/home; cp /executor/run.sh /workspace/run.sh; if [ -d /executor/hooks ]; then cp -R /executor/hooks/. /workspace/hooks/; fi; chmod +x /workspace/run.sh /workspace/hooks/*.sh 2>/dev/null || true; /workspace/run.sh"},
						Env:             env,
						Resources:       resources,
						SecurityContext: &corev1.SecurityContext{
							RunAsUser:                &runAsUser,
							RunAsGroup:               &runAsGroup,
							RunAsNonRoot:             &runAsNonRoot,
							AllowPrivilegeEscalation: &allowPrivilegeEscalation,
							SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined},
							AppArmorProfile:          &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeUnconfined},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "workspace", MountPath: "/workspace"},
							{Name: "buildkit-state", MountPath: "/home/user/.local/share/buildkit"},
							{Name: "executor-files", MountPath: "/executor", ReadOnly: true},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "buildkit-state", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "executor-files", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
							SecretName: secretName,
							Items:      items,
						}}},
					},
				},
			},
		},
	}
}

func buildJobResourceRequirements(run model.BuildRun) corev1.ResourceRequirements {
	cpu := strings.TrimSpace(run.BuildCPURequest)
	if cpu == "" {
		cpu = defaultBuildCPURequest
	}
	memory := strings.TrimSpace(run.BuildMemoryRequest)
	if memory == "" {
		memory = defaultBuildMemoryRequest
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
		},
	}
}

func (r *Runner) followBuildJob(ctx context.Context, client kubernetes.Interface, namespace string, jobName string, job model.BuildJob, hooks []builder.HookPayload, sensitiveValues []string) (builder.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.buildJobTimeoutSeconds)*time.Second)
	defer cancel()
	hookLabels := builder.HookLabelsByRunID(hooks)
	resultCh := make(chan builder.Result, 1)
	logErrCh := make(chan error, 1)
	go func() {
		result, err := r.streamBuildPodLogs(ctx, client, namespace, jobName, job, hookLabels, sensitiveValues)
		if err != nil {
			logErrCh <- err
			return
		}
		resultCh <- result
	}()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var result builder.Result
	for {
		select {
		case parsed := <-resultCh:
			result = parsed
		case err := <-logErrCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				return result, err
			}
		case <-ctx.Done():
			_ = r.deleteKubernetesBuildJob(context.Background(), client, namespace, jobName)
			return result, fmt.Errorf("build job timed out after %ds", r.buildJobTimeoutSeconds)
		case <-ticker.C:
			canceled, err := r.buildRunCanceled(job)
			if err != nil {
				return result, err
			}
			if canceled {
				_ = r.deleteKubernetesBuildJob(context.Background(), client, namespace, jobName)
				return result, errBuildRunCanceled
			}
			_ = r.db.Model(&model.BuildJob{}).Where("id = ? and status = ?", job.ID, "running").Update("last_heartbeat_at", time.Now()).Error
			kubeJob, err := client.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
			if err != nil {
				return result, err
			}
			if kubeJob.Status.Succeeded > 0 {
				if strings.TrimSpace(result.ImageRef) == "" {
					select {
					case parsed := <-resultCh:
						result = parsed
					case <-time.After(2 * time.Second):
					}
				}
				return result, nil
			}
			if kubeJob.Status.Failed > 0 {
				message := r.buildKubernetesJobFailureMessage(ctx, client, namespace, jobName, firstNonEmpty(result.Message, "kubernetes build job failed"))
				return result, errors.New(message)
			}
		}
	}
}

func (r *Runner) streamBuildPodLogs(ctx context.Context, client kubernetes.Interface, namespace string, jobName string, job model.BuildJob, hookLabels map[string]string, sensitiveValues []string) (builder.Result, error) {
	podName, err := waitForBuildPod(ctx, client, namespace, jobName)
	if err != nil {
		return builder.Result{}, err
	}
	stream, err := client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{Follow: true, Container: "executor"}).Stream(ctx)
	if err != nil {
		return builder.Result{}, err
	}
	defer stream.Close()
	reader := bufio.NewReader(stream)
	var result builder.Result
	var lastProgressKey string
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimRight(line, "\n")
		if parsed, ok := builder.ParseResultMarkerLine(line); ok {
			if strings.TrimSpace(parsed.ImageRef) != "" {
				result = parsed
			}
			if err == nil {
				continue
			}
		}
		if rendered, control := builder.HandleHookControlLine(line, hookLabels, func(hookRunID string, content string) error {
			return r.appendBuildHookRunLog(hookRunID, job.ProjectID, content, sensitiveValues)
		}, func(hookRunID string, hookResult builder.HookResult) error {
			return r.completeBuildHookRun(hookRunID, job.ProjectID, hookResult)
		}); control {
			if strings.TrimSpace(rendered) != "" {
				r.appendBuildLog(job, rendered, sensitiveValues)
			}
		} else if strings.TrimSpace(line) != "" {
			r.appendBuildLog(job, line, sensitiveValues)
			progress := builder.ProgressFromLogLine(line)
			if progress.Key != "" && progress.Key != lastProgressKey {
				lastProgressKey = progress.Key
				_ = r.db.Model(&model.BuildJob{}).Where("id = ? and status = ?", job.ID, "running").Update("message", progress.Key).Error
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return result, nil
			}
			return result, err
		}
	}
}

func waitForBuildPod(ctx context.Context, client kubernetes.Interface, namespace string, jobName string) (string, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
		if err != nil {
			return "", err
		}
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil {
				continue
			}
			if buildPodLogsAvailable(pod) {
				return pod.Name, nil
			}
			if err := buildPodStartupError(pod); err != nil {
				return "", err
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func buildPodLogsAvailable(pod corev1.Pod) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != "executor" {
			continue
		}
		return status.State.Running != nil || status.State.Terminated != nil
	}
	return false
}

func buildPodStartupError(pod corev1.Pod) error {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != "executor" || status.State.Waiting == nil {
			continue
		}
		waiting := status.State.Waiting
		switch waiting.Reason {
		case "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "CreateContainerConfigError", "CreateContainerError":
			return fmt.Errorf("build pod %s executor failed to start: %s: %s", pod.Name, waiting.Reason, waiting.Message)
		}
	}
	return nil
}

func (r *Runner) buildKubernetesJobFailureMessage(ctx context.Context, client kubernetes.Interface, namespace string, jobName string, fallback string) string {
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
	if err != nil || len(pods.Items) == 0 {
		return fallback
	}
	messages := make([]string, 0, 4)
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		if message := buildPodFailureMessage(pod); message != "" {
			messages = append(messages, message)
		}
		if eventMessage := buildPodEventFailureMessage(ctx, client, namespace, pod.Name); eventMessage != "" {
			messages = append(messages, eventMessage)
		}
	}
	if len(messages) == 0 {
		return fallback
	}
	return fallback + ": " + strings.Join(messages, "; ")
}

func buildPodFailureMessage(pod corev1.Pod) string {
	parts := make([]string, 0, 4)
	if pod.Status.Phase != "" {
		parts = append(parts, "pod="+string(pod.Status.Phase))
	}
	if pod.Status.Reason != "" {
		parts = append(parts, "reason="+pod.Status.Reason)
	}
	if pod.Status.Message != "" {
		parts = append(parts, "message="+strings.TrimSpace(pod.Status.Message))
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != "executor" {
			continue
		}
		if status.State.Terminated != nil {
			terminated := status.State.Terminated
			containerParts := []string{fmt.Sprintf("executor terminated: reason=%s", firstNonEmpty(terminated.Reason, "Error")), fmt.Sprintf("exitCode=%d", terminated.ExitCode)}
			if terminated.Message != "" {
				containerParts = append(containerParts, "message="+strings.TrimSpace(terminated.Message))
			}
			parts = append(parts, strings.Join(containerParts, " "))
		}
		if status.State.Waiting != nil {
			waiting := status.State.Waiting
			containerParts := []string{"executor waiting: reason=" + firstNonEmpty(waiting.Reason, "Waiting")}
			if waiting.Message != "" {
				containerParts = append(containerParts, "message="+strings.TrimSpace(waiting.Message))
			}
			parts = append(parts, strings.Join(containerParts, " "))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "pod " + pod.Name + " " + strings.Join(parts, ", ")
}

func buildPodEventFailureMessage(ctx context.Context, client kubernetes.Interface, namespace string, podName string) string {
	events, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.name=" + podName})
	if err != nil || len(events.Items) == 0 {
		return ""
	}
	items := make([]corev1.Event, 0, len(events.Items))
	for _, event := range events.Items {
		if event.Type != corev1.EventTypeWarning && event.Reason != "Failed" && event.Reason != "BackOff" && event.Reason != "Evicted" {
			continue
		}
		items = append(items, event)
	}
	if len(items) == 0 {
		return ""
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := eventTime(items[i])
		right := eventTime(items[j])
		return left.Before(right)
	})
	latest := items[len(items)-1]
	message := strings.TrimSpace(latest.Message)
	if message == "" {
		message = latest.Reason
	}
	return "event " + firstNonEmpty(latest.Reason, "Warning") + ": " + message
}

func eventTime(event corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if !event.FirstTimestamp.IsZero() {
		return event.FirstTimestamp.Time
	}
	return event.CreationTimestamp.Time
}

var errBuildRunCanceled = errors.New("build run canceled")

func (r *Runner) buildRunCanceled(job model.BuildJob) (bool, error) {
	var run model.BuildRun
	if err := r.db.First(&run, "id = ? and project_id = ?", job.BuildRunID, job.ProjectID).Error; err != nil {
		return false, err
	}
	return run.Status == "canceled", nil
}

func (r *Runner) completeBuildJob(job model.BuildJob, run model.BuildRun, result builder.Result) (model.BuildRun, error) {
	finishedAt := time.Now()
	var completedRun model.BuildRun
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var lockedJob model.BuildJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedJob, "id = ? and project_id = ?", job.ID, job.ProjectID).Error; err != nil {
			return err
		}
		var lockedRun model.BuildRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRun, "id = ? and project_id = ?", run.ID, run.ProjectID).Error; err != nil {
			return err
		}
		if lockedJob.Status != "running" || lockedRun.Status == "canceled" {
			return nil
		}
		imageRef := firstNonEmpty(result.ImageRef, lockedRun.ImageRef)
		sourceCommit := firstNonEmpty(result.SourceCommit, lockedRun.SourceCommit)
		sourceAuthorName := firstNonEmpty(result.SourceAuthorName, lockedRun.SourceAuthorName)
		sourceAuthorEmail := firstNonEmpty(result.SourceAuthorEmail, lockedRun.SourceAuthorEmail)
		if err := tx.Model(&model.BuildJob{}).Where("id = ?", lockedJob.ID).Updates(map[string]any{
			"status":            "succeeded",
			"message":           firstNonEmpty(result.Message, "builder task succeeded"),
			"lease_token":       "",
			"lease_until":       nil,
			"last_heartbeat_at": &finishedAt,
			"finished_at":       &finishedAt,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.BuildRun{}).Where("id = ?", lockedRun.ID).Updates(map[string]any{
			"status":              "succeeded",
			"image_ref":           imageRef,
			"image_digest":        result.ImageDigest,
			"source_commit":       sourceCommit,
			"source_author_name":  sourceAuthorName,
			"source_author_email": sourceAuthorEmail,
			"finished_at":         &finishedAt,
		}).Error; err != nil {
			return err
		}
		lockedRun.Status = "succeeded"
		lockedRun.ImageRef = imageRef
		lockedRun.ImageDigest = result.ImageDigest
		lockedRun.SourceCommit = sourceCommit
		lockedRun.SourceAuthorName = sourceAuthorName
		lockedRun.SourceAuthorEmail = sourceAuthorEmail
		lockedRun.FinishedAt = &finishedAt
		if imageRef != "" {
			image := containerImageFromBuildRun(lockedRun, imageRef, result.ImageDigest, sourceCommit)
			if image.ID != "" {
				if err := tx.Create(&image).Error; err != nil {
					return err
				}
			}
		}
		completedRun = lockedRun
		return nil
	})
	return completedRun, err
}

func (r *Runner) failBuildJob(job model.BuildJob, run model.BuildRun, message string) error {
	finishedAt := time.Now()
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.BuildJob{}).Where("id = ? and project_id = ? and status in ?", job.ID, job.ProjectID, []string{"queued", "running"}).Updates(map[string]any{
			"status":      "failed",
			"message":     firstNonEmpty(message, "builder task failed"),
			"lease_token": "",
			"lease_until": nil,
			"finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.BuildRun{}).Where("id = ? and project_id = ? and status in ?", run.ID, run.ProjectID, []string{"queued", "running"}).Updates(map[string]any{
			"status":      "failed",
			"finished_at": &finishedAt,
		}).Error
	})
}

func (r *Runner) appendBuildLog(job model.BuildJob, content string, sensitiveValues []string) {
	content = trimBuildLogContent(redactSensitiveLogContent(content, sensitiveValues))
	if strings.TrimSpace(content) == "" {
		return
	}
	var existing model.BuildLog
	err := r.db.First(&existing, "build_job_id = ? and project_id = ?", job.ID, job.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = r.db.Create(&model.BuildLog{
			ID:         id.New("blog"),
			ProjectID:  job.ProjectID,
			BuildRunID: job.BuildRunID,
			BuildJobID: job.ID,
			Content:    content,
		}).Error
		return
	}
	if err != nil {
		return
	}
	existing.Content = trimBuildLogContent(existing.Content + "\n" + content)
	_ = r.db.Save(&existing).Error
}

func (r *Runner) appendBuildHookRunLog(hookRunID string, projectID string, content string, sensitiveValues []string) error {
	content = trimBuildLogContent(redactSensitiveLogContent(content, sensitiveValues))
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var hookRun model.HookRun
	if err := r.db.First(&hookRun, "id = ? and project_id = ?", hookRunID, projectID).Error; err != nil {
		return err
	}
	if hookRun.Status == "queued" {
		now := time.Now()
		_ = r.db.Model(&hookRun).Updates(map[string]any{"status": "running", "started_at": &now}).Error
	}
	var existing model.HookRunLog
	err := r.db.First(&existing, "hook_run_id = ? and project_id = ?", hookRun.ID, hookRun.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(&model.HookRunLog{
			ID:        id.New("hlog"),
			ProjectID: hookRun.ProjectID,
			HookRunID: hookRun.ID,
			Content:   content,
		}).Error
	}
	if err != nil {
		return err
	}
	existing.Content = trimBuildLogContent(existing.Content + "\n" + content)
	return r.db.Save(&existing).Error
}

func (r *Runner) completeBuildHookRun(hookRunID string, projectID string, result builder.HookResult) error {
	finishedAt := time.Now()
	status := "failed"
	if result.Succeeded {
		status = "succeeded"
	}
	return r.db.Model(&model.HookRun{}).Where("id = ? and project_id = ?", hookRunID, projectID).Updates(map[string]any{
		"status":      status,
		"exit_code":   result.ExitCode,
		"message":     result.Message,
		"finished_at": &finishedAt,
	}).Error
}

func trimBuildLogContent(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 262144 {
		return content
	}
	return content[len(content)-262144:]
}

const redactedLogValue = "[REDACTED]"

type sensitiveLogPattern struct {
	regex       *regexp.Regexp
	replacement string
}

var sensitiveLogPatterns = []sensitiveLogPattern{
	{regex: regexp.MustCompile(`(?i)(authorization:\s*(?:bearer|basic)\s+)[^\s]+`), replacement: "${1}" + redactedLogValue},
	{regex: regexp.MustCompile(`(?i)(x-access-token:)[^@\s]+(@)`), replacement: "${1}" + redactedLogValue + "${2}"},
	{regex: regexp.MustCompile(`(?i)\b((?:password|token|secret|access_token|refresh_token)=)[^\s&]+`), replacement: "${1}" + redactedLogValue},
}

func redactSensitiveLogContent(content string, sensitiveValues []string) string {
	output := content
	for _, pattern := range sensitiveLogPatterns {
		output = pattern.regex.ReplaceAllString(output, pattern.replacement)
	}
	for _, value := range normalizedSensitiveLogValues(sensitiveValues) {
		output = strings.ReplaceAll(output, value, redactedLogValue)
	}
	return output
}

func normalizedSensitiveLogValues(values []string) []string {
	seen := map[string]bool{}
	output := make([]string, 0, len(values)*3)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len(value) < 4 {
			continue
		}
		for _, candidate := range []string{value, url.QueryEscape(value), url.PathEscape(value)} {
			if candidate == "" || seen[candidate] {
				continue
			}
			seen[candidate] = true
			output = append(output, candidate)
		}
	}
	return output
}

func (r *Runner) cleanupBuildJobSecrets(ctx context.Context, client kubernetes.Interface, namespace string, secretName string) error {
	err := client.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *Runner) deleteKubernetesBuildJob(ctx context.Context, client kubernetes.Interface, namespace string, jobName string) error {
	propagation := metav1.DeletePropagationBackground
	return client.BatchV1().Jobs(namespace).Delete(ctx, jobName, metav1.DeleteOptions{PropagationPolicy: &propagation})
}

func buildKubernetesJobName(jobID string) string {
	return "build-" + strings.Trim(sanitizeKubernetesName(jobID), "-")
}

func sanitizeKubernetesName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	previousDash := false
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			out.WriteRune(char)
			previousDash = false
			continue
		}
		if !previousDash {
			out.WriteByte('-')
			previousDash = true
		}
	}
	result := strings.Trim(out.String(), "-")
	if result == "" {
		result = "job"
	}
	if len(result) > 57 {
		result = strings.Trim(result[:57], "-")
	}
	return result
}

func boolPtr(value bool) *bool {
	return &value
}

func containerImageFromBuildRun(run model.BuildRun, imageRef string, digest string, sourceCommit string) model.ContainerImage {
	if strings.TrimSpace(run.TargetRegistryID) == "" || strings.TrimSpace(run.TargetRepository) == "" {
		return model.ContainerImage{}
	}
	return model.ContainerImage{
		ID:            id.New("img"),
		ProjectID:     run.ProjectID,
		ApplicationID: run.ApplicationID,
		RegistryID:    run.TargetRegistryID,
		Repository:    strings.Trim(strings.TrimSpace(run.TargetRepository), "/"),
		Tag:           firstNonEmpty(strings.TrimSpace(run.TargetTag), "latest"),
		Digest:        strings.TrimSpace(digest),
		ImageRef:      strings.TrimSpace(imageRef),
		SourceType:    "build",
		BuildRunID:    run.ID,
		SourceCommit:  strings.TrimSpace(sourceCommit),
		ScanStatus:    "unknown",
		CreatedBy:     run.CreatedBy,
	}
}
