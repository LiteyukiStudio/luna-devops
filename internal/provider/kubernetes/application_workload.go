package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) applyApplicationWorkload(ctx context.Context, spec ApplicationResourcesSpec, objectLabels map[string]string, selectorLabels map[string]string) (map[string]string, error) {
	switch applicationWorkloadType(spec) {
	case "StatefulSet":
		effectiveSelectorLabels, err := c.applyStatefulSet(ctx, spec, objectLabels, selectorLabels)
		if err != nil {
			return nil, err
		}
		return effectiveSelectorLabels, c.deleteStaleApplicationDeployment(ctx, spec.Namespace, spec.Name)
	default:
		effectiveSelectorLabels, err := c.applyDeployment(ctx, spec, objectLabels, selectorLabels)
		if err != nil {
			return nil, err
		}
		return effectiveSelectorLabels, c.deleteStaleApplicationStatefulSet(ctx, spec.Namespace, spec.Name)
	}
}

func (c *Client) applyDeployment(ctx context.Context, spec ApplicationResourcesSpec, objectLabels map[string]string, selectorLabels map[string]string) (map[string]string, error) {
	replicas := spec.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	progressDeadlineSeconds := spec.RolloutTimeoutSeconds
	if progressDeadlineSeconds <= 0 {
		progressDeadlineSeconds = 600
	}
	template := applicationPodTemplate(spec, objectLabels, selectorLabels)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: objectLabels},
		Spec: appsv1.DeploymentSpec{
			Replicas:                &replicas,
			Selector:                &metav1.LabelSelector{MatchLabels: selectorLabels},
			ProgressDeadlineSeconds: &progressDeadlineSeconds,
			Template:                template,
		},
	}
	existing, err := c.client.AppsV1().Deployments(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.AppsV1().Deployments(spec.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		return selectorLabels, err
	}
	if err != nil {
		return nil, err
	}
	effectiveSelectorLabels := deploymentSelectorLabels(existing, selectorLabels)
	existing.Labels = objectLabels
	existing.Spec = deployment.Spec
	existing.Spec.Selector = &metav1.LabelSelector{MatchLabels: effectiveSelectorLabels}
	existing.Spec.Template.Labels = appPodTemplateLabels(objectLabels, effectiveSelectorLabels)
	_, err = c.client.AppsV1().Deployments(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return effectiveSelectorLabels, err
}

func (c *Client) applyStatefulSet(ctx context.Context, spec ApplicationResourcesSpec, objectLabels map[string]string, selectorLabels map[string]string) (map[string]string, error) {
	replicas := spec.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: objectLabels},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: spec.Name,
			Selector:    &metav1.LabelSelector{MatchLabels: selectorLabels},
			Template:    applicationPodTemplate(spec, objectLabels, selectorLabels),
		},
	}
	existing, err := c.client.AppsV1().StatefulSets(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.AppsV1().StatefulSets(spec.Namespace).Create(ctx, statefulSet, metav1.CreateOptions{})
		return selectorLabels, err
	}
	if err != nil {
		return nil, err
	}
	effectiveSelectorLabels := statefulSetSelectorLabels(existing, selectorLabels)
	existing.Labels = objectLabels
	existing.Spec = statefulSet.Spec
	existing.Spec.Selector = &metav1.LabelSelector{MatchLabels: effectiveSelectorLabels}
	existing.Spec.Template.Labels = appPodTemplateLabels(objectLabels, effectiveSelectorLabels)
	_, err = c.client.AppsV1().StatefulSets(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return effectiveSelectorLabels, err
}

func applicationPodTemplate(spec ApplicationResourcesSpec, objectLabels map[string]string, selectorLabels map[string]string) corev1.PodTemplateSpec {
	container := corev1.Container{
		Name:            "app",
		Image:           spec.Image,
		ImagePullPolicy: applicationImagePullPolicy(spec),
		Command:         mustApplicationStringList(spec.ContainerCommand),
		Args:            mustApplicationStringList(spec.ContainerArgs),
		Ports:           containerPorts(spec),
		EnvFrom:         applicationEnvFrom(spec),
		Resources:       mustResourceRequirements(spec),
		SecurityContext: mustApplicationContainerSecurityContext(spec),
		Lifecycle:       mustApplicationLifecycle(spec),
		ReadinessProbe:  mustApplicationProbe(spec.ReadinessProbe, "readiness probe"),
		LivenessProbe:   mustApplicationProbe(spec.LivenessProbe, "liveness probe"),
		StartupProbe:    mustApplicationProbe(spec.StartupProbe, "startup probe"),
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
			volumes = append(volumes, applicationDataVolumeSource(spec, dataVolume, volumeName))
		}
	}
	availableVolumeNames := map[string]bool{}
	for _, volume := range volumes {
		availableVolumeNames[volume.Name] = true
	}
	initContainers := mustApplicationAuxContainers(spec.InitContainers, "init containers", spec, availableVolumeNames)
	sidecarContainers := mustApplicationAuxContainers(spec.SidecarContainers, "sidecar containers", spec, availableVolumeNames)
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: appPodTemplateLabels(objectLabels, selectorLabels), Annotations: appPodTemplateAnnotations(spec)},
		Spec: corev1.PodSpec{
			InitContainers:               initContainers,
			Containers:                   append([]corev1.Container{container}, sidecarContainers...),
			Volumes:                      volumes,
			SecurityContext:              mustApplicationPodSecurityContext(spec),
			NodeSelector:                 mustApplicationNodeSelector(spec),
			Tolerations:                  mustApplicationTolerations(spec),
			Affinity:                     mustApplicationAffinity(spec),
			TopologySpreadConstraints:    mustApplicationTopologySpreadConstraints(spec),
			PriorityClassName:            strings.TrimSpace(spec.PriorityClassName),
			ServiceAccountName:           strings.TrimSpace(spec.ServiceAccountName),
			AutomountServiceAccountToken: applicationAutomountServiceAccountToken(spec),
		},
	}
}

func applicationAutomountServiceAccountToken(spec ApplicationResourcesSpec) *bool {
	switch strings.ToLower(strings.TrimSpace(spec.AutomountServiceAccountToken)) {
	case "true":
		return boolPtr(true)
	case "false":
		return boolPtr(false)
	default:
		return nil
	}
}

func configFileKeyPaths(files []ApplicationConfigFile) []corev1.KeyToPath {
	items := make([]corev1.KeyToPath, 0, len(files))
	for _, file := range files {
		items = append(items, corev1.KeyToPath{Key: file.Key, Path: file.Key})
	}
	return items
}

func appSelectorLabels(spec ApplicationResourcesSpec) map[string]string {
	labels := baseManagedLabels(spec.Name)
	setLabel(labels, DeploymentTargetIDLabel, spec.DeploymentTargetID)
	return labels
}

func appObjectLabels(spec ApplicationResourcesSpec) map[string]string {
	labels := appSelectorLabels(spec)
	setLabel(labels, ProjectIDLabel, spec.ProjectID)
	setLabel(labels, ApplicationIDLabel, spec.ApplicationID)
	setLabel(labels, EnvironmentIDLabel, spec.EnvironmentID)
	setLabel(labels, DeploymentTargetIDLabel, spec.DeploymentTargetID)
	setLabel(labels, ReleaseIDLabel, spec.ReleaseID)
	return labels
}

func appPodTemplateAnnotations(spec ApplicationResourcesSpec) map[string]string {
	annotations := map[string]string{}
	setLabel(annotations, ReleaseIDLabel, spec.ReleaseID)
	setLabel(annotations, BuildRunIDLabel, spec.BuildRunID)
	setLabel(annotations, ImageDigestLabel, spec.ImageDigest)
	return annotations
}

func appPodTemplateLabels(objectLabels map[string]string, selectorLabels map[string]string) map[string]string {
	labels := cloneStringMap(objectLabels)
	for key, value := range selectorLabels {
		labels[key] = value
	}
	return labels
}

func deploymentSelectorLabels(existing *appsv1.Deployment, fallback map[string]string) map[string]string {
	if existing != nil && existing.Spec.Selector != nil && len(existing.Spec.Selector.MatchLabels) > 0 {
		return cloneStringMap(existing.Spec.Selector.MatchLabels)
	}
	return cloneStringMap(fallback)
}

func statefulSetSelectorLabels(existing *appsv1.StatefulSet, fallback map[string]string) map[string]string {
	if existing != nil && existing.Spec.Selector != nil && len(existing.Spec.Selector.MatchLabels) > 0 {
		return cloneStringMap(existing.Spec.Selector.MatchLabels)
	}
	return cloneStringMap(fallback)
}

func applicationWorkloadType(spec ApplicationResourcesSpec) string {
	switch strings.ToLower(strings.TrimSpace(spec.WorkloadType)) {
	case "statefulset", "stateful-set":
		return "StatefulSet"
	default:
		return "Deployment"
	}
}

func (c *Client) deleteStaleApplicationDeployment(ctx context.Context, namespace string, name string) error {
	err := c.client.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) deleteStaleApplicationStatefulSet(ctx context.Context, namespace string, name string) error {
	err := c.client.AppsV1().StatefulSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func applicationEnvFrom(spec ApplicationResourcesSpec) []corev1.EnvFromSource {
	return []corev1.EnvFromSource{
		{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.Name + "-config"}}},
		{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.Name + "-secret"}}},
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func applicationImagePullPolicy(spec ApplicationResourcesSpec) corev1.PullPolicy {
	if spec.ForceImagePull {
		return corev1.PullAlways
	}
	switch strings.TrimSpace(spec.ImagePullPolicy) {
	case string(corev1.PullAlways):
		return corev1.PullAlways
	case string(corev1.PullNever):
		return corev1.PullNever
	case string(corev1.PullIfNotPresent):
		return corev1.PullIfNotPresent
	}
	return corev1.PullIfNotPresent
}

func applicationLifecycle(spec ApplicationResourcesSpec) (*corev1.Lifecycle, error) {
	raw := strings.TrimSpace(spec.Lifecycle)
	if raw == "" {
		return nil, nil
	}
	var lifecycle corev1.Lifecycle
	if err := json.Unmarshal([]byte(raw), &lifecycle); err != nil {
		return nil, fmt.Errorf("invalid lifecycle: %w", err)
	}
	return &lifecycle, nil
}

func mustApplicationLifecycle(spec ApplicationResourcesSpec) *corev1.Lifecycle {
	lifecycle, err := applicationLifecycle(spec)
	if err != nil {
		panic(err)
	}
	return lifecycle
}

func applicationAuxContainers(raw string, label string, spec ApplicationResourcesSpec, availableVolumeNames map[string]bool) ([]corev1.Container, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var input []corev1.Container
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", label, err)
	}
	output := make([]corev1.Container, 0, len(input))
	for _, item := range input {
		name := dnsLabel(item.Name)
		if name == "" || strings.TrimSpace(item.Image) == "" {
			return nil, fmt.Errorf("%s requires name and image", label)
		}
		securityContext, err := allowedAuxSecurityContext(item.SecurityContext, label)
		if err != nil {
			return nil, err
		}
		container := corev1.Container{
			Name:            name,
			Image:           strings.TrimSpace(item.Image),
			ImagePullPolicy: applicationImagePullPolicy(spec),
			Command:         compactStringList(item.Command),
			Args:            compactStringList(item.Args),
			Ports:           allowedAuxContainerPorts(item.Ports),
			Resources:       item.Resources,
			SecurityContext: securityContext,
			Env:             allowedAuxEnvVars(item.Env),
			EnvFrom:         applicationEnvFrom(spec),
			VolumeMounts:    allowedVolumeMounts(item.VolumeMounts, availableVolumeNames),
		}
		output = append(output, container)
	}
	return output, nil
}

func mustApplicationAuxContainers(raw string, label string, spec ApplicationResourcesSpec, availableVolumeNames map[string]bool) []corev1.Container {
	containers, err := applicationAuxContainers(raw, label, spec, availableVolumeNames)
	if err != nil {
		panic(err)
	}
	return containers
}

func allowedAuxContainerPorts(input []corev1.ContainerPort) []corev1.ContainerPort {
	if len(input) == 0 {
		return nil
	}
	output := make([]corev1.ContainerPort, 0, len(input))
	for _, item := range input {
		if item.ContainerPort <= 0 || item.ContainerPort > 65535 {
			continue
		}
		output = append(output, corev1.ContainerPort{
			Name:          dnsLabel(item.Name),
			ContainerPort: item.ContainerPort,
			Protocol:      item.Protocol,
		})
	}
	return output
}

func allowedAuxEnvVars(input []corev1.EnvVar) []corev1.EnvVar {
	if len(input) == 0 {
		return nil
	}
	output := make([]corev1.EnvVar, 0, len(input))
	for _, item := range input {
		name := strings.TrimSpace(item.Name)
		if name == "" || item.ValueFrom != nil {
			continue
		}
		output = append(output, corev1.EnvVar{Name: name, Value: item.Value})
	}
	return output
}

func allowedAuxSecurityContext(input *corev1.SecurityContext, label string) (*corev1.SecurityContext, error) {
	if input == nil {
		return nil, nil
	}
	if input.Privileged != nil && *input.Privileged {
		return nil, fmt.Errorf("%s cannot enable privileged", label)
	}
	if input.AllowPrivilegeEscalation != nil && *input.AllowPrivilegeEscalation {
		return nil, fmt.Errorf("%s cannot enable privilege escalation", label)
	}
	if input.Capabilities != nil && len(input.Capabilities.Add) > 0 {
		return nil, fmt.Errorf("%s cannot add Linux capabilities", label)
	}
	context := &corev1.SecurityContext{}
	hasValue := false
	if input.RunAsUser != nil {
		context.RunAsUser = input.RunAsUser
		hasValue = true
	}
	if input.RunAsGroup != nil {
		context.RunAsGroup = input.RunAsGroup
		hasValue = true
	}
	if input.RunAsNonRoot != nil {
		context.RunAsNonRoot = input.RunAsNonRoot
		hasValue = true
	}
	if input.ReadOnlyRootFilesystem != nil {
		context.ReadOnlyRootFilesystem = input.ReadOnlyRootFilesystem
		hasValue = true
	}
	if input.AllowPrivilegeEscalation != nil {
		context.AllowPrivilegeEscalation = input.AllowPrivilegeEscalation
		hasValue = true
	}
	if input.Capabilities != nil && len(input.Capabilities.Drop) > 0 {
		context.Capabilities = &corev1.Capabilities{Drop: input.Capabilities.Drop}
		hasValue = true
	}
	if input.SeccompProfile != nil {
		context.SeccompProfile = input.SeccompProfile
		hasValue = true
	}
	if !hasValue {
		return nil, nil
	}
	return context, nil
}

func allowedVolumeMounts(input []corev1.VolumeMount, available map[string]bool) []corev1.VolumeMount {
	if len(input) == 0 || len(available) == 0 {
		return nil
	}
	output := make([]corev1.VolumeMount, 0, len(input))
	for _, item := range input {
		if available[item.Name] && strings.TrimSpace(item.MountPath) != "" {
			output = append(output, corev1.VolumeMount{
				Name:      item.Name,
				ReadOnly:  item.ReadOnly,
				MountPath: item.MountPath,
				SubPath:   item.SubPath,
			})
		}
	}
	return output
}

func applicationPodSecurityContext(spec ApplicationResourcesSpec) (*corev1.PodSecurityContext, error) {
	context := &corev1.PodSecurityContext{}
	hasValue := false
	if value, ok, err := optionalInt64(spec.RunAsUser); err != nil {
		return nil, fmt.Errorf("invalid runAsUser: %w", err)
	} else if ok {
		context.RunAsUser = &value
		hasValue = true
	}
	if value, ok, err := optionalInt64(spec.RunAsGroup); err != nil {
		return nil, fmt.Errorf("invalid runAsGroup: %w", err)
	} else if ok {
		context.RunAsGroup = &value
		hasValue = true
	}
	if value, ok, err := optionalInt64(spec.FSGroup); err != nil {
		return nil, fmt.Errorf("invalid fsGroup: %w", err)
	} else if ok {
		context.FSGroup = &value
		hasValue = true
	}
	if policy := strings.TrimSpace(spec.FSGroupChangePolicy); policy != "" {
		value := corev1.PodFSGroupChangePolicy(policy)
		context.FSGroupChangePolicy = &value
		hasValue = true
	}
	if !hasValue {
		return nil, nil
	}
	return context, nil
}

func mustApplicationPodSecurityContext(spec ApplicationResourcesSpec) *corev1.PodSecurityContext {
	context, err := applicationPodSecurityContext(spec)
	if err != nil {
		panic(err)
	}
	return context
}

func applicationContainerSecurityContext(spec ApplicationResourcesSpec) (*corev1.SecurityContext, error) {
	context := &corev1.SecurityContext{}
	hasValue := false
	if spec.ReadOnlyRootFilesystem {
		context.ReadOnlyRootFilesystem = boolPtr(true)
		hasValue = true
	}
	if value, ok, err := optionalBool(spec.AllowPrivilegeEscalation); err != nil {
		return nil, fmt.Errorf("invalid allowPrivilegeEscalation: %w", err)
	} else if ok {
		context.AllowPrivilegeEscalation = &value
		hasValue = true
	}
	add, err := applicationStringList(spec.CapabilityAdd, "capability add")
	if err != nil {
		return nil, err
	}
	drop, err := applicationStringList(spec.CapabilityDrop, "capability drop")
	if err != nil {
		return nil, err
	}
	if len(add) > 0 || len(drop) > 0 {
		context.Capabilities = &corev1.Capabilities{
			Add:  capabilityNames(add),
			Drop: capabilityNames(drop),
		}
		hasValue = true
	}
	if !hasValue {
		return nil, nil
	}
	return context, nil
}

func mustApplicationContainerSecurityContext(spec ApplicationResourcesSpec) *corev1.SecurityContext {
	context, err := applicationContainerSecurityContext(spec)
	if err != nil {
		panic(err)
	}
	return context
}

func capabilityNames(values []string) []corev1.Capability {
	output := make([]corev1.Capability, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			output = append(output, corev1.Capability(value))
		}
	}
	return output
}

func applicationProbe(raw string, label string) (*corev1.Probe, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var probe corev1.Probe
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", label, err)
	}
	return &probe, nil
}

func mustApplicationProbe(raw string, label string) *corev1.Probe {
	probe, err := applicationProbe(raw, label)
	if err != nil {
		panic(err)
	}
	return probe
}

func applicationNodeSelector(spec ApplicationResourcesSpec) (map[string]string, error) {
	return stringMapFromJSONOrLines(spec.NodeSelector, "node selector")
}

func mustApplicationNodeSelector(spec ApplicationResourcesSpec) map[string]string {
	values, err := applicationNodeSelector(spec)
	if err != nil {
		panic(err)
	}
	return values
}

func applicationTolerations(spec ApplicationResourcesSpec) ([]corev1.Toleration, error) {
	raw := strings.TrimSpace(spec.Tolerations)
	if raw == "" {
		return nil, nil
	}
	var values []corev1.Toleration
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("invalid tolerations: %w", err)
	}
	return values, nil
}

func mustApplicationTolerations(spec ApplicationResourcesSpec) []corev1.Toleration {
	values, err := applicationTolerations(spec)
	if err != nil {
		panic(err)
	}
	return values
}

func applicationAffinity(spec ApplicationResourcesSpec) (*corev1.Affinity, error) {
	raw := strings.TrimSpace(spec.Affinity)
	if raw == "" {
		return nil, nil
	}
	var value corev1.Affinity
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("invalid affinity: %w", err)
	}
	return &value, nil
}

func mustApplicationAffinity(spec ApplicationResourcesSpec) *corev1.Affinity {
	value, err := applicationAffinity(spec)
	if err != nil {
		panic(err)
	}
	return value
}

func applicationTopologySpreadConstraints(spec ApplicationResourcesSpec) ([]corev1.TopologySpreadConstraint, error) {
	raw := strings.TrimSpace(spec.TopologySpreadConstraints)
	if raw == "" {
		return nil, nil
	}
	var values []corev1.TopologySpreadConstraint
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("invalid topology spread constraints: %w", err)
	}
	return values, nil
}

func mustApplicationTopologySpreadConstraints(spec ApplicationResourcesSpec) []corev1.TopologySpreadConstraint {
	values, err := applicationTopologySpreadConstraints(spec)
	if err != nil {
		panic(err)
	}
	return values
}

func resourceRequirements(spec ApplicationResourcesSpec) (corev1.ResourceRequirements, error) {
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}
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
	if spec.CPULimit != "" {
		quantity, err := resource.ParseQuantity(spec.CPULimit)
		if err != nil {
			return corev1.ResourceRequirements{}, fmt.Errorf("invalid cpu limit: %w", err)
		}
		limits[corev1.ResourceCPU] = quantity
	}
	if spec.MemoryLimit != "" {
		quantity, err := resource.ParseQuantity(spec.MemoryLimit)
		if err != nil {
			return corev1.ResourceRequirements{}, fmt.Errorf("invalid memory limit: %w", err)
		}
		limits[corev1.ResourceMemory] = quantity
	}
	requirements := corev1.ResourceRequirements{Requests: requests}
	if len(limits) > 0 {
		requirements.Limits = limits
	}
	return requirements, nil
}

func mustResourceRequirements(spec ApplicationResourcesSpec) corev1.ResourceRequirements {
	requirements, err := resourceRequirements(spec)
	if err != nil {
		panic(err)
	}
	return requirements
}
