package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

type ApplicationResourcesSpec struct {
	Name                  string
	Namespace             string
	ProjectID             string
	ApplicationID         string
	EnvironmentID         string
	DeploymentTargetID    string
	ReleaseID             string
	Image                 string
	Replicas              int32
	ServicePort           int32
	CPURequest            string
	MemoryRequest         string
	RolloutTimeoutSeconds int32
	ConfigData            map[string]string
	SecretData            map[string]string
	ConfigFiles           []ApplicationConfigFile
	SecretFiles           []ApplicationConfigFile
	DataRetentionEnabled  bool
	DataCapacity          string
	DataMountPath         string
	DataVolumes           []ApplicationDataVolume
	ForceImagePull        bool
}

type ApplicationConfigFile struct {
	Path    string
	Key     string
	Content string
}

type ApplicationDataVolume struct {
	Name      string
	MountPath string
	Capacity  string
}

type HookJobSpec struct {
	Name               string
	Namespace          string
	ProjectID          string
	ApplicationID      string
	BuildRunID         string
	EnvironmentID      string
	DeploymentTargetID string
	ReleaseID          string
	HookRunID          string
	Phase              string
	Image              string
	GitBranch          string
	GitTag             string
	GitRefName         string
	GitRefType         string
	GitRef             string
	GitSHA             string
	GitShortSHA        string
	Shell              string
	Script             string
	TimeoutSeconds     int32
	ConfigMapName      string
	SecretName         string
}

type HookJobResult struct {
	Succeeded bool
	ExitCode  int32
	Message   string
	Logs      string
}

type DataExportSpec struct {
	Name      string
	Namespace string
	PVCName   string
	MountPath string
	Volumes   []DataExportVolume
}

type DataExportVolume struct {
	Name    string
	PVCName string
}

func (c *Client) ApplyApplicationResources(ctx context.Context, spec ApplicationResourcesSpec) error {
	if err := validateApplicationResourcesSpec(spec); err != nil {
		return err
	}
	objectLabels := appObjectLabels(spec)
	selectorLabels := appSelectorLabels(spec)
	if err := c.applyApplicationRuntimeConfig(ctx, spec, objectLabels); err != nil {
		return err
	}
	if spec.DataRetentionEnabled {
		for _, volume := range persistentDataVolumes(spec) {
			if err := c.applyPersistentDataVolume(ctx, spec, volume, objectLabels); err != nil {
				return err
			}
		}
	}
	if err := c.applyDeployment(ctx, spec, objectLabels, selectorLabels); err != nil {
		return err
	}
	return c.applyService(ctx, spec, objectLabels, selectorLabels)
}

func (c *Client) ApplyApplicationRuntimeConfig(ctx context.Context, spec ApplicationResourcesSpec) error {
	if err := validateApplicationResourcesSpec(spec); err != nil {
		return err
	}
	objectLabels := appObjectLabels(spec)
	if err := c.applyApplicationRuntimeConfig(ctx, spec, objectLabels); err != nil {
		return err
	}
	if spec.DataRetentionEnabled {
		for _, volume := range persistentDataVolumes(spec) {
			if err := c.applyPersistentDataVolume(ctx, spec, volume, objectLabels); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) ApplyPersistentDataVolume(ctx context.Context, spec ApplicationResourcesSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" {
		return fmt.Errorf("application resource name and namespace are required")
	}
	for _, volume := range persistentDataVolumes(spec) {
		if _, err := persistentDataCapacity(volume); err != nil {
			return err
		}
		if err := c.applyPersistentDataVolume(ctx, spec, volume, appObjectLabels(spec)); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyApplicationRuntimeConfig(ctx context.Context, spec ApplicationResourcesSpec, objectLabels map[string]string) error {
	if err := c.applyConfigMap(ctx, spec, objectLabels); err != nil {
		return err
	}
	if err := c.applySecret(ctx, spec, objectLabels); err != nil {
		return err
	}
	if err := c.applyConfigFilesConfigMap(ctx, spec, objectLabels); err != nil {
		return err
	}
	if err := c.applySecretFilesSecret(ctx, spec, objectLabels); err != nil {
		return err
	}
	return nil
}

func validateApplicationResourcesSpec(spec ApplicationResourcesSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" {
		return fmt.Errorf("application resource name and namespace are required")
	}
	if strings.TrimSpace(spec.Image) == "" {
		return fmt.Errorf("release image is required")
	}
	if spec.ServicePort <= 0 || spec.ServicePort > 65535 {
		return fmt.Errorf("service port must be between 1 and 65535")
	}
	if _, err := resourceRequirements(spec); err != nil {
		return err
	}
	if spec.DataRetentionEnabled {
		for _, volume := range persistentDataVolumes(spec) {
			if _, err := persistentDataCapacity(volume); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) applyConfigMap(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string) error {
	item := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: spec.Name + "-config", Namespace: spec.Namespace, Labels: labels}, Data: spec.ConfigData}
	existing, err := c.client.CoreV1().ConfigMaps(spec.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Create(ctx, item, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = item.Labels
	existing.Data = item.Data
	_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applySecret(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string) error {
	data := make(map[string][]byte, len(spec.SecretData))
	for key, value := range spec.SecretData {
		data[key] = []byte(value)
	}
	item := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: spec.Name + "-secret", Namespace: spec.Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, Data: data}
	existing, err := c.client.CoreV1().Secrets(spec.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().Secrets(spec.Namespace).Create(ctx, item, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = item.Labels
	existing.Type = item.Type
	existing.Data = item.Data
	_, err = c.client.CoreV1().Secrets(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyConfigFilesConfigMap(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string) error {
	if len(spec.ConfigFiles) == 0 {
		return c.deleteConfigMapIfExists(ctx, spec.Namespace, spec.Name+"-config-files")
	}
	data := make(map[string]string, len(spec.ConfigFiles))
	for _, file := range spec.ConfigFiles {
		data[file.Key] = file.Content
	}
	item := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: spec.Name + "-config-files", Namespace: spec.Namespace, Labels: labels}, Data: data}
	existing, err := c.client.CoreV1().ConfigMaps(spec.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Create(ctx, item, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = item.Labels
	existing.Data = item.Data
	_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applySecretFilesSecret(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string) error {
	if len(spec.SecretFiles) == 0 {
		return c.deleteSecretIfExists(ctx, spec.Namespace, spec.Name+"-secret-files")
	}
	data := make(map[string][]byte, len(spec.SecretFiles))
	for _, file := range spec.SecretFiles {
		data[file.Key] = []byte(file.Content)
	}
	item := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: spec.Name + "-secret-files", Namespace: spec.Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, Data: data}
	existing, err := c.client.CoreV1().Secrets(spec.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().Secrets(spec.Namespace).Create(ctx, item, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = item.Labels
	existing.Type = item.Type
	existing.Data = item.Data
	_, err = c.client.CoreV1().Secrets(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) deleteConfigMapIfExists(ctx context.Context, namespace string, name string) error {
	err := c.client.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) deleteSecretIfExists(ctx context.Context, namespace string, name string) error {
	err := c.client.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) applyPersistentDataVolume(ctx context.Context, spec ApplicationResourcesSpec, volume ApplicationDataVolume, labels map[string]string) error {
	capacity, err := persistentDataCapacity(volume)
	if err != nil {
		return err
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: persistentDataPVCName(spec, volume), Namespace: spec.Namespace, Labels: labels},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: capacity}},
		},
	}
	existing, err := c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = pvc.Labels
	existing.Spec.Resources.Requests[corev1.ResourceStorage] = capacity
	_, err = c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyDeployment(ctx context.Context, spec ApplicationResourcesSpec, objectLabels map[string]string, selectorLabels map[string]string) error {
	replicas := spec.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	progressDeadlineSeconds := spec.RolloutTimeoutSeconds
	if progressDeadlineSeconds <= 0 {
		progressDeadlineSeconds = 600
	}
	container := corev1.Container{
		Name:            "app",
		Image:           spec.Image,
		ImagePullPolicy: applicationImagePullPolicy(spec),
		Ports:           []corev1.ContainerPort{{ContainerPort: spec.ServicePort}},
		EnvFrom: []corev1.EnvFromSource{
			{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.Name + "-config"}}},
			{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.Name + "-secret"}}},
		},
		Resources: mustResourceRequirements(spec),
	}
	volumes := []corev1.Volume{}
	if len(spec.ConfigFiles) > 0 {
		volumes = append(volumes, corev1.Volume{
			Name: "config-files",
			VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: spec.Name + "-config-files"},
				Items:                configFileKeyPaths(spec.ConfigFiles),
			}},
		})
		for _, file := range spec.ConfigFiles {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "config-files", MountPath: file.Path, SubPath: file.Key, ReadOnly: true})
		}
	}
	if len(spec.SecretFiles) > 0 {
		volumes = append(volumes, corev1.Volume{
			Name: "secret-files",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
				SecretName: spec.Name + "-secret-files",
				Items:      configFileKeyPaths(spec.SecretFiles),
			}},
		})
		for _, file := range spec.SecretFiles {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "secret-files", MountPath: file.Path, SubPath: file.Key, ReadOnly: true})
		}
	}
	if spec.DataRetentionEnabled {
		for _, dataVolume := range persistentDataVolumes(spec) {
			volumeName := persistentDataVolumeName(dataVolume)
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: volumeName, MountPath: dataVolume.MountPath})
			volumes = append(volumes, corev1.Volume{
				Name:         volumeName,
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: persistentDataPVCName(spec, dataVolume)}},
			})
		}
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: objectLabels},
		Spec: appsv1.DeploymentSpec{
			Replicas:                &replicas,
			Selector:                &metav1.LabelSelector{MatchLabels: selectorLabels},
			ProgressDeadlineSeconds: &progressDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selectorLabels, Annotations: appPodTemplateAnnotations(spec)},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{container}, Volumes: volumes},
			},
		},
	}
	existing, err := c.client.AppsV1().Deployments(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.AppsV1().Deployments(spec.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = objectLabels
	existing.Spec = deployment.Spec
	_, err = c.client.AppsV1().Deployments(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func configFileKeyPaths(files []ApplicationConfigFile) []corev1.KeyToPath {
	items := make([]corev1.KeyToPath, 0, len(files))
	for _, file := range files {
		items = append(items, corev1.KeyToPath{Key: file.Key, Path: file.Key})
	}
	return items
}

func (c *Client) StreamDataArchive(ctx context.Context, spec DataExportSpec, output io.Writer) error {
	if c.restConfig == nil {
		return fmt.Errorf("kubernetes rest config is required")
	}
	exportVolumes := spec.Volumes
	if len(exportVolumes) == 0 && strings.TrimSpace(spec.PVCName) != "" {
		exportVolumes = []DataExportVolume{{Name: "data", PVCName: spec.PVCName}}
	}
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" || len(exportVolumes) == 0 {
		return fmt.Errorf("data export name, namespace and pvc name are required")
	}
	for _, volume := range exportVolumes {
		if strings.TrimSpace(volume.Name) == "" || strings.TrimSpace(volume.PVCName) == "" {
			return fmt.Errorf("data export volume name and pvc name are required")
		}
	}
	podName := dnsLabel(firstNonEmpty(spec.Name, "data-export"))
	singleVolume := len(exportVolumes) == 1
	mountPath := firstNonEmpty(spec.MountPath, "/data")
	volumeMounts := make([]corev1.VolumeMount, 0, len(exportVolumes))
	volumes := make([]corev1.Volume, 0, len(exportVolumes))
	for _, volume := range exportVolumes {
		name := persistentDataVolumeName(ApplicationDataVolume{Name: volume.Name})
		targetMountPath := mountPath
		if !singleVolume {
			targetMountPath = "/mnt/" + name
		}
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: name, MountPath: targetMountPath, ReadOnly: true})
		volumes = append(volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: volume.PVCName,
				ReadOnly:  true,
			}},
		})
	}
	tarRoot := mountPath
	if !singleVolume {
		tarRoot = "/mnt"
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: spec.Namespace, Labels: baseManagedLabels(podName)},
		Spec: corev1.PodSpec{
			AutomountServiceAccountToken: boolPtr(false),
			RestartPolicy:                corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:         "export",
				Image:        "busybox:1.36",
				Command:      []string{"sh", "-c", "sleep 300"},
				VolumeMounts: volumeMounts,
			}},
			Volumes: volumes,
		},
	}
	_ = c.client.CoreV1().Pods(spec.Namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if _, err := c.client.CoreV1().Pods(spec.Namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return err
	}
	defer c.client.CoreV1().Pods(spec.Namespace).Delete(context.Background(), podName, metav1.DeleteOptions{})
	if err := c.waitForPodRunning(ctx, spec.Namespace, podName); err != nil {
		return err
	}
	req := c.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(spec.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "export",
			Command:   []string{"tar", "czf", "-", "-C", tarRoot, "."},
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: output, Stderr: &stderr}); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}
	return nil
}

func (c *Client) waitForPodRunning(ctx context.Context, namespace string, name string) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		pod, err := c.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			return false, fmt.Errorf("export pod finished before streaming: %s", pod.Status.Phase)
		}
		return pod.Status.Phase == corev1.PodRunning, nil
	})
}

func (c *Client) applyService(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string, selectorLabels map[string]string) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: labels},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels,
			Ports: []corev1.ServicePort{{
				Port:       spec.ServicePort,
				TargetPort: intstrFromInt32(spec.ServicePort),
			}},
		},
	}
	existing, err := c.client.CoreV1().Services(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().Services(spec.Namespace).Create(ctx, service, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = labels
	existing.Spec.Selector = selectorLabels
	existing.Spec.Ports = service.Spec.Ports
	_, err = c.client.CoreV1().Services(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) RunHookJob(ctx context.Context, spec HookJobSpec) (HookJobResult, error) {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" || strings.TrimSpace(spec.Image) == "" {
		return HookJobResult{}, fmt.Errorf("hook job name, namespace and image are required")
	}
	labels := baseManagedLabels(spec.Name)
	setLabel(labels, ProjectIDLabel, spec.ProjectID)
	setLabel(labels, ApplicationIDLabel, spec.ApplicationID)
	setLabel(labels, EnvironmentIDLabel, spec.EnvironmentID)
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
			BackoffLimit:          &backoffLimit,
			ActiveDeadlineSeconds: int64Ptr(int64(timeout)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: boolPtr(false),
					RestartPolicy:                corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "hook",
						Image:   spec.Image,
						Command: []string{shell, "/liteyuki-hooks/run.sh"},
						EnvFrom: []corev1.EnvFromSource{
							{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.ConfigMapName}}},
							{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.SecretName}}},
						},
						Env: []corev1.EnvVar{
							{Name: "LITEYUKI_PROJECT_ID", Value: spec.ProjectID},
							{Name: "LITEYUKI_APPLICATION_ID", Value: spec.ApplicationID},
							{Name: "LITEYUKI_BUILD_RUN_ID", Value: spec.BuildRunID},
							{Name: "LITEYUKI_ENVIRONMENT_ID", Value: spec.EnvironmentID},
							{Name: "LITEYUKI_DEPLOYMENT_TARGET_ID", Value: spec.DeploymentTargetID},
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
						VolumeMounts: []corev1.VolumeMount{{Name: "hook-script", MountPath: "/liteyuki-hooks", ReadOnly: true}},
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
	if _, err := c.client.BatchV1().Jobs(spec.Namespace).Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return HookJobResult{}, err
	}
	return c.waitForHookJob(ctx, spec.Namespace, spec.Name, time.Duration(timeout)*time.Second)
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

func appSelectorLabels(spec ApplicationResourcesSpec) map[string]string {
	labels := baseManagedLabels(spec.Name)
	setLabel(labels, ProjectIDLabel, spec.ProjectID)
	setLabel(labels, ApplicationIDLabel, spec.ApplicationID)
	setLabel(labels, EnvironmentIDLabel, spec.EnvironmentID)
	setLabel(labels, DeploymentTargetIDLabel, spec.DeploymentTargetID)
	return labels
}

func appObjectLabels(spec ApplicationResourcesSpec) map[string]string {
	labels := appSelectorLabels(spec)
	setLabel(labels, ReleaseIDLabel, spec.ReleaseID)
	return labels
}

func appPodTemplateAnnotations(spec ApplicationResourcesSpec) map[string]string {
	annotations := map[string]string{}
	setLabel(annotations, ReleaseIDLabel, spec.ReleaseID)
	return annotations
}

func applicationImagePullPolicy(spec ApplicationResourcesSpec) corev1.PullPolicy {
	if spec.ForceImagePull {
		return corev1.PullAlways
	}
	return corev1.PullIfNotPresent
}

func persistentDataVolumes(spec ApplicationResourcesSpec) []ApplicationDataVolume {
	if len(spec.DataVolumes) > 0 {
		volumes := make([]ApplicationDataVolume, 0, len(spec.DataVolumes))
		for _, volume := range spec.DataVolumes {
			name := firstNonEmpty(volume.Name, "data")
			volumes = append(volumes, ApplicationDataVolume{
				Name:      name,
				MountPath: firstNonEmpty(volume.MountPath, "/data"),
				Capacity:  firstNonEmpty(volume.Capacity, "1Gi"),
			})
		}
		return volumes
	}
	return []ApplicationDataVolume{{
		Name:      "data",
		MountPath: persistentDataMountPath(spec),
		Capacity:  firstNonEmpty(spec.DataCapacity, "1Gi"),
	}}
}

func persistentDataPVCName(spec ApplicationResourcesSpec, volume ApplicationDataVolume) string {
	name := persistentDataVolumeName(volume)
	if name == "data" {
		return spec.Name + "-data"
	}
	return dnsLabel(spec.Name + "-" + name + "-data")
}

func persistentDataMountPath(spec ApplicationResourcesSpec) string {
	return firstNonEmpty(spec.DataMountPath, "/data")
}

func persistentDataVolumeName(volume ApplicationDataVolume) string {
	return dnsLabel(firstNonEmpty(volume.Name, "data"))
}

func persistentDataCapacity(volume ApplicationDataVolume) (resource.Quantity, error) {
	value := firstNonEmpty(volume.Capacity, "1Gi")
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("invalid data capacity: %w", err)
	}
	if quantity.Sign() <= 0 {
		return resource.Quantity{}, fmt.Errorf("data capacity must be greater than zero")
	}
	return quantity, nil
}

func resourceRequirements(spec ApplicationResourcesSpec) (corev1.ResourceRequirements, error) {
	requests := corev1.ResourceList{}
	if spec.CPURequest != "" {
		quantity, err := resource.ParseQuantity(spec.CPURequest)
		if err != nil {
			return corev1.ResourceRequirements{}, fmt.Errorf("invalid cpu request: %w", err)
		}
		requests[corev1.ResourceCPU] = quantity
	}
	if spec.MemoryRequest != "" {
		quantity, err := resource.ParseQuantity(spec.MemoryRequest)
		if err != nil {
			return corev1.ResourceRequirements{}, fmt.Errorf("invalid memory request: %w", err)
		}
		requests[corev1.ResourceMemory] = quantity
	}
	return corev1.ResourceRequirements{Requests: requests}, nil
}

func mustResourceRequirements(spec ApplicationResourcesSpec) corev1.ResourceRequirements {
	requirements, err := resourceRequirements(spec)
	if err != nil {
		panic(err)
	}
	return requirements
}

func intstrFromInt32(value int32) intstr.IntOrString {
	return intstr.FromInt(int(value))
}

func int64Ptr(value int64) *int64 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
