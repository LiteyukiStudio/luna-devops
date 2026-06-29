package worker

import (
	"context"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

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
	timeoutSeconds := effectiveBuildTimeoutSeconds(run.BuildTimeoutSeconds, r.buildJobTimeoutSeconds)
	job := buildJobSpec(jobName, secretName, environment, run, task, r.buildExecutorImage, r.buildNPMRegistry, r.buildCacheEnabled, r.buildCacheTag, timeoutSeconds, r.buildJobTTLSeconds)
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
func buildJobSpec(jobName string, secretName string, environment model.Environment, run model.BuildRun, task builder.Task, image string, npmRegistry string, cacheEnabled bool, cacheTag string, timeoutSeconds int64, ttlSeconds int64) *batchv1.Job {
	backoffLimit := int32(0)
	ttl := int32(ttlSeconds)
	activeDeadlineSeconds := timeoutSeconds
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
			ActiveDeadlineSeconds:   &activeDeadlineSeconds,
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
func boolPtr(value bool) *bool {
	return &value
}
