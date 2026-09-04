package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) RunHookJob(ctx context.Context, spec HookJobSpec) (HookJobResult, error) {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" || strings.TrimSpace(spec.Image) == "" {
		return HookJobResult{}, fmt.Errorf("hook job name, namespace and image are required")
	}
	labels := baseManagedLabels(spec.Name)
	setLabel(labels, ProjectIDLabel, spec.ProjectID)
	setLabel(labels, ApplicationIDLabel, spec.ApplicationID)
	setLabel(labels, DeploymentTargetIDLabel, spec.DeploymentTargetID)
	setLabel(labels, ReleaseIDLabel, spec.ReleaseID)
	setLabel(labels, HookRunIDLabel, spec.HookRunID)
	setLabel(labels, HookPhaseLabel, spec.Phase)
	scriptMapName := spec.Name + "-script"
	scriptMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: scriptMapName, Namespace: spec.Namespace, Labels: labels},
		Data:       map[string]string{"run.sh": spec.Script},
	}
	if err := c.applyHookScriptConfigMap(ctx, scriptMap); err != nil {
		return HookJobResult{}, err
	}
	shell := strings.TrimSpace(spec.Shell)
	if shell != "bash" {
		shell = "sh"
	}
	timeout := spec.TimeoutSeconds
	if timeout <= 0 {
		timeout = 300
	}
	backoffLimit := int32(0)
	mode := int32(0o755)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   int64Ptr(int64(timeout)),
			TTLSecondsAfterFinished: int32Ptr(hookJobFailureTTLSeconds),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: boolPtr(false),
					RestartPolicy:                corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "hook",
						Image:   spec.Image,
						Command: []string{shell, "/luna-hooks/run.sh"},
						EnvFrom: []corev1.EnvFromSource{
							{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.ConfigMapName}}},
							{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.SecretName}}},
						},
						Env: []corev1.EnvVar{
							{Name: "LITEYUKI_PROJECT_ID", Value: spec.ProjectID},
							{Name: "LITEYUKI_APPLICATION_ID", Value: spec.ApplicationID},
							{Name: "LITEYUKI_BUILD_RUN_ID", Value: spec.BuildRunID},
							{Name: "LITEYUKI_DEPLOYMENT_TARGET_ID", Value: spec.DeploymentTargetID},
							{Name: "LUNA_DEPLOYMENT_STAGE", Value: spec.Stage},
							{Name: "LITEYUKI_RELEASE_ID", Value: spec.ReleaseID},
							{Name: "LITEYUKI_HOOK_RUN_ID", Value: spec.HookRunID},
							{Name: "LITEYUKI_HOOK_PHASE", Value: spec.Phase},
							{Name: "LITEYUKI_IMAGE_REF", Value: spec.Image},
							{Name: "LITEYUKI_GIT_BRANCH", Value: spec.GitBranch},
							{Name: "LITEYUKI_GIT_TAG", Value: spec.GitTag},
							{Name: "LITEYUKI_GIT_REF_NAME", Value: spec.GitRefName},
							{Name: "LITEYUKI_GIT_REF_TYPE", Value: spec.GitRefType},
							{Name: "LITEYUKI_GIT_REF", Value: spec.GitRef},
							{Name: "LITEYUKI_GIT_SHA", Value: spec.GitSHA},
							{Name: "LITEYUKI_GIT_SHORT_SHA", Value: spec.GitShortSHA},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "hook-script", MountPath: "/luna-hooks", ReadOnly: true}},
					}},
					Volumes: []corev1.Volume{{Name: "hook-script", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: scriptMapName},
						DefaultMode:          &mode,
					}}}},
				},
			},
		},
	}
	_ = c.client.BatchV1().Jobs(spec.Namespace).Delete(ctx, spec.Name, metav1.DeleteOptions{})
	createdJob, err := c.client.BatchV1().Jobs(spec.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return HookJobResult{}, err
	}
	_ = c.attachHookScriptOwner(ctx, spec.Namespace, scriptMapName, createdJob)
	result, err := c.waitForHookJob(ctx, spec.Namespace, spec.Name, time.Duration(timeout)*time.Second)
	if err != nil {
		return result, err
	}
	ttlSeconds := hookJobFailureTTLSeconds
	if result.Succeeded {
		ttlSeconds = hookJobSuccessTTLSeconds
	}
	_ = c.setHookJobTTL(ctx, spec.Namespace, spec.Name, ttlSeconds)
	return result, nil
}

func (c *Client) applyHookScriptConfigMap(ctx context.Context, item *corev1.ConfigMap) error {
	existing, err := c.client.CoreV1().ConfigMaps(item.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().ConfigMaps(item.Namespace).Create(ctx, item, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = item.Labels
	existing.Data = item.Data
	_, err = c.client.CoreV1().ConfigMaps(item.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) attachHookScriptOwner(ctx context.Context, namespace string, name string, owner *batchv1.Job) error {
	item, err := c.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	item.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(owner, batchv1.SchemeGroupVersion.WithKind("Job"))}
	_, err = c.client.CoreV1().ConfigMaps(namespace).Update(ctx, item, metav1.UpdateOptions{})
	return err
}

func (c *Client) setHookJobTTL(ctx context.Context, namespace string, name string, ttlSeconds int32) error {
	job, err := c.client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	job.Spec.TTLSecondsAfterFinished = int32Ptr(ttlSeconds)
	_, err = c.client.BatchV1().Jobs(namespace).Update(ctx, job, metav1.UpdateOptions{})
	return err
}

func (c *Client) waitForHookJob(ctx context.Context, namespace string, name string, timeout time.Duration) (HookJobResult, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		job, err := c.client.BatchV1().Jobs(namespace).Get(waitCtx, name, metav1.GetOptions{})
		if err != nil {
			return HookJobResult{}, err
		}
		if job.Status.Succeeded > 0 {
			logs := c.hookJobLogs(waitCtx, namespace, name)
			return HookJobResult{Succeeded: true, Message: "hook job succeeded", Logs: logs}, nil
		}
		if job.Status.Failed > 0 {
			logs := c.hookJobLogs(waitCtx, namespace, name)
			return HookJobResult{Succeeded: false, ExitCode: 1, Message: "hook job failed", Logs: logs}, nil
		}
		select {
		case <-waitCtx.Done():
			logs := c.hookJobLogs(ctx, namespace, name)
			return HookJobResult{Succeeded: false, ExitCode: 124, Message: fmt.Sprintf("hook job timed out after %s", timeout), Logs: logs}, nil
		case <-ticker.C:
		}
	}
}

func (c *Client) hookJobLogs(ctx context.Context, namespace string, jobName string) string {
	pods, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
	if err != nil || len(pods.Items) == 0 {
		return ""
	}
	req := c.client.CoreV1().Pods(namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{})
	data, err := req.Do(ctx).Raw()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
