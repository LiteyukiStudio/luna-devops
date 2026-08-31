package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type KubectlRuntimeObservationOptions struct {
	RuntimeClusterID string
	ProjectID        string
	ApplicationID    string
	Namespace        string
}

type KubectlRuntimeWorkload struct {
	RuntimeClusterID       string    `json:"runtimeClusterId"`
	ProjectID              string    `json:"projectId"`
	ApplicationID          string    `json:"applicationId,omitempty"`
	Namespace              string    `json:"namespace"`
	Name                   string    `json:"name"`
	Kind                   string    `json:"kind"`
	ResourceUID            string    `json:"resourceUid"`
	ManagementSource       string    `json:"managementSource"`
	DesiredReplicas        int32     `json:"desiredReplicas"`
	UpdatedReplicas        int32     `json:"updatedReplicas"`
	ReadyReplicas          int32     `json:"readyReplicas"`
	AvailableReplicas      int32     `json:"availableReplicas"`
	EffectiveCPURequest    string    `json:"effectiveCpuRequest"`
	EffectiveMemoryRequest string    `json:"effectiveMemoryRequest"`
	Status                 string    `json:"status"`
	CreatedAt              time.Time `json:"createdAt"`
	ObservedAt             time.Time `json:"observedAt"`
	PodNames               []string  `json:"-"`
}

func (w KubectlRuntimeWorkload) SyntheticDeploymentTargetID() string {
	return KubectlRuntimeSyntheticTargetID(w.RuntimeClusterID, w.ProjectID, w.ApplicationID, w.ResourceUID)
}

func (w KubectlRuntimeWorkload) RuntimeMetricsOptions() RuntimeMetricsOptions {
	return RuntimeMetricsOptions{
		Namespace:          w.Namespace,
		DeploymentTargetID: w.SyntheticDeploymentTargetID(),
		WorkloadName:       w.Name,
		WorkloadType:       w.Kind,
		ExactPodNames:      append([]string{}, w.PodNames...),
		ProjectID:          w.ProjectID,
		ApplicationID:      w.ApplicationID,
		ManagementSource:   w.ManagementSource,
		DesiredReplicas:    w.DesiredReplicas,
		UpdatedReplicas:    w.UpdatedReplicas,
		ReadyReplicas:      w.ReadyReplicas,
		AvailableReplicas:  w.AvailableReplicas,
	}
}

func KubectlRuntimeSyntheticTargetID(clusterID, projectID, applicationID, resourceUID string) string {
	return strings.Join([]string{
		"kubectl",
		strings.TrimSpace(clusterID),
		strings.TrimSpace(projectID),
		strings.TrimSpace(applicationID),
		strings.TrimSpace(resourceUID),
	}, ":")
}

func ParseKubectlRuntimeSyntheticTargetID(value string) (clusterID, projectID, applicationID, resourceUID string, ok bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 5 || parts[0] != "kubectl" {
		return "", "", "", "", false
	}
	return parts[1], parts[2], parts[3], parts[4], parts[4] != ""
}

// ListKubectlRuntimeWorkloads returns only top-level runtime consumers. Pods
// created by a Deployment/ReplicaSet, StatefulSet, Job, or CronJob are attached
// to that owner and never returned as a second billable workload.
func (c *Client) ListKubectlRuntimeWorkloads(ctx context.Context, options KubectlRuntimeObservationOptions) ([]KubectlRuntimeWorkload, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("kubectl runtime observation requires a Kubernetes client")
	}
	options.RuntimeClusterID = strings.TrimSpace(options.RuntimeClusterID)
	options.ProjectID = strings.TrimSpace(options.ProjectID)
	options.ApplicationID = strings.TrimSpace(options.ApplicationID)
	options.Namespace = strings.TrimSpace(options.Namespace)
	if options.RuntimeClusterID == "" || options.ProjectID == "" || options.Namespace == "" {
		return nil, fmt.Errorf("kubectl runtime observation requires cluster, project, and namespace ownership")
	}
	selector := kubectlRuntimeObservationSelector(options)
	listOptions := metav1.ListOptions{LabelSelector: selector}
	deployments, err := c.client.AppsV1().Deployments(options.Namespace).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	statefulSets, err := c.client.AppsV1().StatefulSets(options.Namespace).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	replicaSets, err := c.client.AppsV1().ReplicaSets(options.Namespace).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	jobs, err := c.client.BatchV1().Jobs(options.Namespace).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	cronJobs, err := c.client.BatchV1().CronJobs(options.Namespace).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	pods, err := c.client.CoreV1().Pods(options.Namespace).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	observedAt := time.Now().UTC()
	items := make([]KubectlRuntimeWorkload, 0, len(deployments.Items)+len(statefulSets.Items)+len(replicaSets.Items)+len(jobs.Items)+len(cronJobs.Items)+len(pods.Items))
	workloadIndexes := make(map[string]int)
	addWorkload := func(workload KubectlRuntimeWorkload) bool {
		if workload.ResourceUID == "" || !kubectlRuntimeWorkloadMatches(options, workload) {
			return false
		}
		workload.PodNames = make([]string, 0)
		workloadIndexes[kubectlRuntimeOwnerKey(workload.Kind, workload.ResourceUID)] = len(items)
		items = append(items, workload)
		return true
	}

	for i := range deployments.Items {
		addWorkload(kubectlRuntimeDeploymentWorkload(options.RuntimeClusterID, deployments.Items[i], observedAt))
	}
	for i := range statefulSets.Items {
		addWorkload(kubectlRuntimeStatefulSetWorkload(options.RuntimeClusterID, statefulSets.Items[i], observedAt))
	}
	for i := range replicaSets.Items {
		if controller := metav1.GetControllerOf(&replicaSets.Items[i]); controller != nil {
			continue
		}
		addWorkload(kubectlRuntimeReplicaSetWorkload(options.RuntimeClusterID, replicaSets.Items[i], observedAt))
	}
	for i := range jobs.Items {
		if controller := metav1.GetControllerOf(&jobs.Items[i]); controller != nil {
			continue
		}
		addWorkload(kubectlRuntimeJobWorkload(options.RuntimeClusterID, jobs.Items[i], observedAt))
	}
	for i := range cronJobs.Items {
		addWorkload(kubectlRuntimeCronJobWorkload(options.RuntimeClusterID, cronJobs.Items[i], observedAt))
	}

	replicaSetOwners := make(map[string]string, len(replicaSets.Items))
	for i := range replicaSets.Items {
		item := &replicaSets.Items[i]
		if item.UID == "" || !kubectlRuntimeLabelsMatch(options, item.Labels) {
			continue
		}
		controller := metav1.GetControllerOf(item)
		if controller != nil && controller.Kind == "Deployment" {
			if _, exists := workloadIndexes[kubectlRuntimeOwnerKey("Deployment", string(controller.UID))]; exists {
				replicaSetOwners[string(item.UID)] = kubectlRuntimeOwnerKey("Deployment", string(controller.UID))
			}
			continue
		}
		if controller == nil {
			if _, exists := workloadIndexes[kubectlRuntimeOwnerKey("ReplicaSet", string(item.UID))]; exists {
				replicaSetOwners[string(item.UID)] = kubectlRuntimeOwnerKey("ReplicaSet", string(item.UID))
			}
		}
	}
	jobOwners := make(map[string]string, len(jobs.Items))
	for i := range jobs.Items {
		item := &jobs.Items[i]
		if item.UID == "" || !kubectlRuntimeLabelsMatch(options, item.Labels) {
			continue
		}
		controller := metav1.GetControllerOf(item)
		if controller != nil && controller.Kind == "CronJob" {
			if _, exists := workloadIndexes[kubectlRuntimeOwnerKey("CronJob", string(controller.UID))]; exists {
				jobOwners[string(item.UID)] = kubectlRuntimeOwnerKey("CronJob", string(controller.UID))
			}
			continue
		}
		if controller == nil {
			if _, exists := workloadIndexes[kubectlRuntimeOwnerKey("Job", string(item.UID))]; exists {
				jobOwners[string(item.UID)] = kubectlRuntimeOwnerKey("Job", string(item.UID))
			}
		}
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.UID == "" || !kubectlRuntimeLabelsMatch(options, pod.Labels) {
			continue
		}
		controller := metav1.GetControllerOf(pod)
		if controller == nil {
			if addWorkload(kubectlRuntimePodWorkload(options.RuntimeClusterID, *pod, observedAt)) {
				items[len(items)-1].PodNames = append(items[len(items)-1].PodNames, pod.Name)
			}
			continue
		}
		ownerKey := ""
		switch controller.Kind {
		case "Deployment", "StatefulSet":
			ownerKey = kubectlRuntimeOwnerKey(controller.Kind, string(controller.UID))
		case "ReplicaSet":
			ownerKey = replicaSetOwners[string(controller.UID)]
		case "Job":
			ownerKey = jobOwners[string(controller.UID)]
		}
		index, exists := workloadIndexes[ownerKey]
		if !exists || !kubectlRuntimePodMatchesWorkload(*pod, items[index]) {
			continue
		}
		items[index].PodNames = append(items[index].PodNames, pod.Name)
	}

	for index := range items {
		sort.Strings(items[index].PodNames)
		if items[index].Kind == "Job" || items[index].Kind == "CronJob" {
			active, ready := kubectlRuntimeActivePodCounts(items[index].PodNames, pods.Items)
			items[index].DesiredReplicas = max(items[index].DesiredReplicas, active)
			items[index].UpdatedReplicas = max(items[index].UpdatedReplicas, active)
			items[index].ReadyReplicas = max(items[index].ReadyReplicas, ready)
			items[index].AvailableReplicas = max(items[index].AvailableReplicas, ready)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].ResourceUID < items[j].ResourceUID
	})
	return items, nil
}

func kubectlRuntimeObservationSelector(options KubectlRuntimeObservationOptions) string {
	values := map[string]string{
		ManagedByLabel:                      ManagedByValue,
		KubectlGatewayManagementSourceLabel: KubectlGatewayManagementSourceValue,
		ProjectIDLabel:                      strings.TrimSpace(options.ProjectID),
	}
	if applicationID := strings.TrimSpace(options.ApplicationID); applicationID != "" {
		values[ApplicationIDLabel] = applicationID
	}
	return labels.Set(values).AsSelector().String()
}

func kubectlRuntimeDeploymentWorkload(clusterID string, item appsv1.Deployment, observedAt time.Time) KubectlRuntimeWorkload {
	snapshot := deploymentSnapshot(item)
	cpu, memory := kubectlRuntimePodSpecRequests(item.Spec.Template.Spec)
	desired := int32(1)
	if item.Spec.Replicas != nil {
		desired = *item.Spec.Replicas
	}
	return kubectlRuntimeWorkloadFromMetadata(clusterID, "Deployment", item.ObjectMeta, desired, item.Status.UpdatedReplicas, item.Status.ReadyReplicas, item.Status.AvailableReplicas, cpu, memory, snapshot.Status, observedAt)
}

func kubectlRuntimeStatefulSetWorkload(clusterID string, item appsv1.StatefulSet, observedAt time.Time) KubectlRuntimeWorkload {
	snapshot := statefulSetSnapshot(item)
	cpu, memory := kubectlRuntimePodSpecRequests(item.Spec.Template.Spec)
	desired := int32(1)
	if item.Spec.Replicas != nil {
		desired = *item.Spec.Replicas
	}
	return kubectlRuntimeWorkloadFromMetadata(clusterID, "StatefulSet", item.ObjectMeta, desired, item.Status.UpdatedReplicas, item.Status.ReadyReplicas, item.Status.AvailableReplicas, cpu, memory, snapshot.Status, observedAt)
}

func kubectlRuntimeReplicaSetWorkload(clusterID string, item appsv1.ReplicaSet, observedAt time.Time) KubectlRuntimeWorkload {
	cpu, memory := kubectlRuntimePodSpecRequests(item.Spec.Template.Spec)
	desired := int32(1)
	if item.Spec.Replicas != nil {
		desired = *item.Spec.Replicas
	}
	status := "progressing"
	if desired == 0 {
		status = "scaled-to-zero"
	} else if item.Status.ReadyReplicas >= desired {
		status = "ready"
	}
	return kubectlRuntimeWorkloadFromMetadata(clusterID, "ReplicaSet", item.ObjectMeta, desired, item.Status.FullyLabeledReplicas, item.Status.ReadyReplicas, item.Status.AvailableReplicas, cpu, memory, status, observedAt)
}

func kubectlRuntimeJobWorkload(clusterID string, item batchv1.Job, observedAt time.Time) KubectlRuntimeWorkload {
	cpu, memory := kubectlRuntimePodSpecRequests(item.Spec.Template.Spec)
	ready := int32(0)
	if item.Status.Ready != nil {
		ready = *item.Status.Ready
	}
	status := "pending"
	if item.Status.Active > 0 {
		status = "running"
	} else if item.Status.Failed > 0 {
		status = "failed"
	} else if item.Status.Succeeded > 0 {
		status = "succeeded"
	}
	return kubectlRuntimeWorkloadFromMetadata(clusterID, "Job", item.ObjectMeta, item.Status.Active, item.Status.Active, ready, ready, cpu, memory, status, observedAt)
}

func kubectlRuntimeCronJobWorkload(clusterID string, item batchv1.CronJob, observedAt time.Time) KubectlRuntimeWorkload {
	cpu, memory := kubectlRuntimePodSpecRequests(item.Spec.JobTemplate.Spec.Template.Spec)
	status := "idle"
	if item.Spec.Suspend != nil && *item.Spec.Suspend {
		status = "suspended"
	} else if len(item.Status.Active) > 0 {
		status = "running"
	}
	active := int32(len(item.Status.Active))
	return kubectlRuntimeWorkloadFromMetadata(clusterID, "CronJob", item.ObjectMeta, active, active, 0, 0, cpu, memory, status, observedAt)
}

func kubectlRuntimePodWorkload(clusterID string, item corev1.Pod, observedAt time.Time) KubectlRuntimeWorkload {
	cpu, memory := kubectlRuntimePodSpecRequests(item.Spec)
	desired := int32(0)
	if item.Status.Phase != corev1.PodSucceeded && item.Status.Phase != corev1.PodFailed {
		desired = 1
	}
	ready := int32(0)
	if desired > 0 && podReady(item) {
		ready = 1
	}
	status := strings.ToLower(string(item.Status.Phase))
	if status == "" {
		status = "pending"
	}
	return kubectlRuntimeWorkloadFromMetadata(clusterID, "Pod", item.ObjectMeta, desired, desired, ready, ready, cpu, memory, status, observedAt)
}

func kubectlRuntimeWorkloadFromMetadata(clusterID, kind string, metadata metav1.ObjectMeta, desired, updated, ready, available int32, cpu, memory, status string, observedAt time.Time) KubectlRuntimeWorkload {
	return KubectlRuntimeWorkload{
		RuntimeClusterID:       strings.TrimSpace(clusterID),
		ProjectID:              strings.TrimSpace(metadata.Labels[ProjectIDLabel]),
		ApplicationID:          strings.TrimSpace(metadata.Labels[ApplicationIDLabel]),
		Namespace:              metadata.Namespace,
		Name:                   metadata.Name,
		Kind:                   kind,
		ResourceUID:            string(metadata.UID),
		ManagementSource:       strings.TrimSpace(metadata.Labels[KubectlGatewayManagementSourceLabel]),
		DesiredReplicas:        max(desired, 0),
		UpdatedReplicas:        max(updated, 0),
		ReadyReplicas:          max(ready, 0),
		AvailableReplicas:      max(available, 0),
		EffectiveCPURequest:    cpu,
		EffectiveMemoryRequest: memory,
		Status:                 status,
		CreatedAt:              metadata.CreationTimestamp.Time,
		ObservedAt:             observedAt,
	}
}

func kubectlRuntimeWorkloadMatches(options KubectlRuntimeObservationOptions, workload KubectlRuntimeWorkload) bool {
	return workload.RuntimeClusterID == options.RuntimeClusterID && workload.Namespace == options.Namespace && workload.ProjectID == options.ProjectID &&
		(options.ApplicationID == "" || workload.ApplicationID == options.ApplicationID) && workload.ManagementSource == KubectlGatewayManagementSourceValue
}

func kubectlRuntimeLabelsMatch(options KubectlRuntimeObservationOptions, actual map[string]string) bool {
	if actual[ManagedByLabel] != ManagedByValue || actual[KubectlGatewayManagementSourceLabel] != KubectlGatewayManagementSourceValue || actual[ProjectIDLabel] != options.ProjectID {
		return false
	}
	return options.ApplicationID == "" || actual[ApplicationIDLabel] == options.ApplicationID
}

func kubectlRuntimePodMatchesWorkload(pod corev1.Pod, workload KubectlRuntimeWorkload) bool {
	if pod.Namespace != workload.Namespace || pod.Labels[ManagedByLabel] != ManagedByValue || pod.Labels[KubectlGatewayManagementSourceLabel] != workload.ManagementSource || pod.Labels[ProjectIDLabel] != workload.ProjectID {
		return false
	}
	return pod.Labels[ApplicationIDLabel] == workload.ApplicationID
}

func kubectlRuntimeOwnerKey(kind, uid string) string {
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(uid) == "" {
		return ""
	}
	return kind + "/" + uid
}

func kubectlRuntimeActivePodCounts(names []string, pods []corev1.Pod) (active, ready int32) {
	selected := make(map[string]struct{}, len(names))
	for _, name := range names {
		selected[name] = struct{}{}
	}
	for i := range pods {
		if _, exists := selected[pods[i].Name]; !exists || pods[i].Status.Phase == corev1.PodSucceeded || pods[i].Status.Phase == corev1.PodFailed {
			continue
		}
		active++
		if podReady(pods[i]) {
			ready++
		}
	}
	return active, ready
}

func kubectlRuntimePodSpecRequests(spec corev1.PodSpec) (cpuRequest, memoryRequest string) {
	pod := &corev1.Pod{Spec: spec}
	cpuMilli := effectivePodRequest(pod, corev1.ResourceCPU, true)
	memoryBytes := effectivePodRequest(pod, corev1.ResourceMemory, false)
	if cpuMilli > 0 {
		cpuRequest = fmt.Sprintf("%dm", cpuMilli)
	}
	if memoryBytes > 0 {
		memoryRequest = fmt.Sprintf("%d", memoryBytes)
	}
	return cpuRequest, memoryRequest
}
