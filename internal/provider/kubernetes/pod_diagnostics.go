package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PodStatusReady                      = "ready"
	PodStatusNotReady                   = "not-ready"
	PodStatusPending                    = "pending"
	PodStatusContainerCreating          = "container-creating"
	PodStatusTerminating                = "terminating"
	PodStatusSucceeded                  = "succeeded"
	PodStatusFailed                     = "failed"
	PodStatusCrashLoopBackOff           = "crash-loop-back-off"
	PodStatusOOMKilled                  = "oom-killed"
	PodStatusImagePullBackOff           = "image-pull-back-off"
	PodStatusErrImagePull               = "err-image-pull"
	PodStatusCreateContainerConfigError = "create-container-config-error"
	PodStatusCreateContainerError       = "create-container-error"
	PodStatusInitError                  = "init-error"
	PodStatusUnschedulable              = "unschedulable"
	PodStatusEvicted                    = "evicted"
	PodStatusUnknown                    = "unknown"
)

type RuntimeContainerStateSnapshot struct {
	State      string     `json:"state"`
	Reason     string     `json:"reason,omitempty"`
	Message    string     `json:"message,omitempty"`
	ExitCode   *int32     `json:"exitCode,omitempty"`
	Signal     *int32     `json:"signal,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type RuntimeContainerTerminationSnapshot struct {
	Reason     string     `json:"reason,omitempty"`
	Message    string     `json:"message,omitempty"`
	ExitCode   int32      `json:"exitCode"`
	Signal     int32      `json:"signal,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type RuntimeContainerStatusSnapshot struct {
	Name            string                               `json:"name"`
	Image           string                               `json:"image"`
	Init            bool                                 `json:"init"`
	Ready           bool                                 `json:"ready"`
	RestartCount    int32                                `json:"restartCount"`
	CurrentState    RuntimeContainerStateSnapshot        `json:"currentState"`
	LastTermination *RuntimeContainerTerminationSnapshot `json:"lastTermination,omitempty"`
}

type RuntimePodConditionSnapshot struct {
	Type               string     `json:"type"`
	Status             string     `json:"status"`
	Reason             string     `json:"reason,omitempty"`
	Message            string     `json:"message,omitempty"`
	LastTransitionTime *time.Time `json:"lastTransitionTime,omitempty"`
}

type DeploymentTargetEventSnapshot struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Reason       string    `json:"reason"`
	Message      string    `json:"message"`
	Source       string    `json:"source"`
	Count        int32     `json:"count"`
	FirstSeen    time.Time `json:"firstSeen"`
	LastSeen     time.Time `json:"lastSeen"`
	InvolvedKind string    `json:"involvedKind"`
	InvolvedName string    `json:"involvedName"`
}

type WorkloadRuntimeObservation struct {
	Workload        DeploymentSnapshot
	Status          string
	ObservationCode string
	Pods            []RuntimePodSnapshot
}

// ObserveWorkloadRuntime combines workload rollout state with live Pod-level
// failure inspection so runtime crashes/OOM/image-pull/probe degradation are
// surfaced immediately without breaking active rollout polling in deploy_runner.
func (c *Client) ObserveWorkloadRuntime(
	ctx context.Context,
	namespace string,
	deploymentTargetID string,
	workloadName string,
	workloadType string,
) (WorkloadRuntimeObservation, error) {
	snapshot, err := c.GetWorkloadSnapshot(ctx, namespace, workloadName, workloadType)
	if err != nil {
		return WorkloadRuntimeObservation{}, err
	}
	pods, podsErr := c.RuntimePods(ctx, namespace, deploymentTargetID, snapshot.ReleaseID)
	if podsErr != nil {
		pods = nil
	}
	return EvaluateWorkloadRuntime(snapshot, pods), nil
}

// EvaluateWorkloadRuntime classifies the live health of a workload from its
// controller snapshot and current non-terminating Pod diagnostics.
func EvaluateWorkloadRuntime(snapshot DeploymentSnapshot, pods []RuntimePodSnapshot) WorkloadRuntimeObservation {
	result := WorkloadRuntimeObservation{
		Workload: snapshot,
		Pods:     pods,
	}
	if snapshot.Phase == DeploymentSucceeded {
		if snapshot.DesiredReplicas == 0 {
			result.Status = "scaled-to-zero"
			result.ObservationCode = "deployment_target.scaled_to_zero"
			return result
		}
		result.Status = "ready"
		result.ObservationCode = "deployment_target.ready"
		return result
	}

	if code := dominantPodFailureObservationCode(pods); code != "" {
		result.Status = "degraded"
		result.ObservationCode = code
		return result
	}

	if snapshot.Phase == DeploymentFailed {
		result.Status = "degraded"
		result.ObservationCode = "runtime.workload_degraded"
		return result
	}

	if snapshot.DesiredReplicas > 0 && (snapshot.ReplicaFailure || snapshot.RolloutSettled) &&
		(snapshot.ReadyReplicas < snapshot.DesiredReplicas || snapshot.AvailableReplicas < snapshot.DesiredReplicas) {
		result.Status = "degraded"
		if hasRunningUnreadyPod(pods) {
			result.ObservationCode = "runtime.probe_unhealthy"
		} else {
			result.ObservationCode = "runtime.workload_degraded"
		}
		return result
	}

	result.Status = "progressing"
	result.ObservationCode = "deployment_target.rollout_progressing"
	return result
}

func dominantPodFailureObservationCode(pods []RuntimePodSnapshot) string {
	bestPriority := 999
	bestCode := ""
	for _, pod := range pods {
		if pod.Terminating {
			continue
		}
		priority, code := podFailureObservationCode(pod.Status)
		if code != "" && priority < bestPriority {
			bestPriority = priority
			bestCode = code
		}
	}
	return bestCode
}

func podFailureObservationCode(podStatus string) (int, string) {
	switch podStatus {
	case PodStatusOOMKilled:
		return 1, "runtime.oom_killed"
	case PodStatusCrashLoopBackOff:
		return 2, "runtime.crash_loop_back_off"
	case PodStatusImagePullBackOff, PodStatusErrImagePull:
		return 3, "runtime.image_pull_back_off"
	case PodStatusCreateContainerConfigError:
		return 4, "runtime.container_config_error"
	case PodStatusCreateContainerError, PodStatusInitError, PodStatusFailed:
		return 5, "runtime.pod_start_failed"
	case PodStatusUnschedulable:
		return 6, "runtime.unschedulable"
	case PodStatusEvicted:
		return 7, "runtime.evicted"
	default:
		return 999, ""
	}
}

func hasRunningUnreadyPod(pods []RuntimePodSnapshot) bool {
	for _, pod := range pods {
		if !pod.Terminating && pod.Status == PodStatusNotReady {
			return true
		}
	}
	return false
}

func IsAbnormalPodStatus(status string) bool {
	_, code := podFailureObservationCode(status)
	return code != "" || status == PodStatusNotReady
}

func buildRuntimePodSnapshot(pod corev1.Pod) RuntimePodSnapshot {
	containers := make([]string, 0, len(pod.Spec.Containers))
	specImages := make(map[string]string, len(pod.Spec.Containers)+len(pod.Spec.InitContainers))
	for _, container := range pod.Spec.Containers {
		containers = append(containers, container.Name)
		specImages[container.Name] = container.Image
	}
	initContainers := make([]string, 0, len(pod.Spec.InitContainers))
	for _, container := range pod.Spec.InitContainers {
		initContainers = append(initContainers, container.Name)
		specImages[container.Name] = container.Image
	}

	var restartCount int32
	var readyContainers int32
	containerStatuses := make([]RuntimeContainerStatusSnapshot, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))

	initByName := make(map[string]corev1.ContainerStatus, len(pod.Status.InitContainerStatuses))
	for _, status := range pod.Status.InitContainerStatuses {
		initByName[status.Name] = status
	}
	seenInit := make(map[string]bool, len(pod.Spec.InitContainers))
	for _, spec := range pod.Spec.InitContainers {
		seenInit[spec.Name] = true
		if status, ok := initByName[spec.Name]; ok {
			restartCount += status.RestartCount
			containerStatuses = append(containerStatuses, toRuntimeContainerStatusSnapshot(status, spec.Image, true))
		} else {
			containerStatuses = append(containerStatuses, RuntimeContainerStatusSnapshot{
				Name:         spec.Name,
				Image:        spec.Image,
				Init:         true,
				CurrentState: RuntimeContainerStateSnapshot{State: "waiting", Reason: "PodInitializing"},
			})
		}
	}
	for _, status := range pod.Status.InitContainerStatuses {
		if seenInit[status.Name] {
			continue
		}
		restartCount += status.RestartCount
		containerStatuses = append(containerStatuses, toRuntimeContainerStatusSnapshot(status, specImages[status.Name], true))
	}

	appByName := make(map[string]corev1.ContainerStatus, len(pod.Status.ContainerStatuses))
	for _, status := range pod.Status.ContainerStatuses {
		appByName[status.Name] = status
	}
	seenApp := make(map[string]bool, len(pod.Spec.Containers))
	for _, spec := range pod.Spec.Containers {
		seenApp[spec.Name] = true
		if status, ok := appByName[spec.Name]; ok {
			restartCount += status.RestartCount
			if status.Ready {
				readyContainers++
			}
			containerStatuses = append(containerStatuses, toRuntimeContainerStatusSnapshot(status, spec.Image, false))
		} else {
			containerStatuses = append(containerStatuses, RuntimeContainerStatusSnapshot{
				Name:         spec.Name,
				Image:        spec.Image,
				Init:         false,
				CurrentState: RuntimeContainerStateSnapshot{State: "waiting", Reason: "ContainerCreating"},
			})
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if seenApp[status.Name] {
			continue
		}
		restartCount += status.RestartCount
		if status.Ready {
			readyContainers++
		}
		containerStatuses = append(containerStatuses, toRuntimeContainerStatusSnapshot(status, specImages[status.Name], false))
	}

	totalContainers := int32(len(pod.Spec.Containers))
	if totalContainers == 0 {
		totalContainers = int32(len(pod.Status.ContainerStatuses))
	}
	ready := podReady(pod)
	if totalContainers == 0 {
		totalContainers = 1
		if ready {
			readyContainers = 1
		}
	}

	conditions := make([]RuntimePodConditionSnapshot, 0, len(pod.Status.Conditions))
	for _, condition := range pod.Status.Conditions {
		var transitioned *time.Time
		if !condition.LastTransitionTime.IsZero() {
			value := condition.LastTransitionTime.Time
			transitioned = &value
		}
		conditions = append(conditions, RuntimePodConditionSnapshot{
			Type:               string(condition.Type),
			Status:             string(condition.Status),
			Reason:             strings.TrimSpace(condition.Reason),
			Message:            strings.TrimSpace(condition.Message),
			LastTransitionTime: transitioned,
		})
	}

	diagStatus, reason, message := classifyPodDiagnosticStatus(pod, ready)

	return RuntimePodSnapshot{
		Name:              pod.Name,
		Phase:             string(pod.Status.Phase),
		Status:            diagStatus,
		Ready:             ready,
		ReadyContainers:   readyContainers,
		TotalContainers:   totalContainers,
		Terminating:       pod.DeletionTimestamp != nil,
		RestartCount:      restartCount,
		ReleaseID:         pod.Labels[ReleaseIDLabel],
		NodeName:          strings.TrimSpace(pod.Spec.NodeName),
		Reason:            reason,
		Message:           message,
		Containers:        containers,
		InitContainers:    initContainers,
		ContainerStatuses: containerStatuses,
		Conditions:        conditions,
		CreatedAt:         pod.CreationTimestamp.Time,
	}
}

func toRuntimeContainerStatusSnapshot(status corev1.ContainerStatus, fallbackImage string, isInit bool) RuntimeContainerStatusSnapshot {
	image := strings.TrimSpace(status.Image)
	if image == "" {
		image = strings.TrimSpace(fallbackImage)
	}
	item := RuntimeContainerStatusSnapshot{
		Name:         status.Name,
		Image:        image,
		Init:         isInit,
		Ready:        status.Ready,
		RestartCount: status.RestartCount,
		CurrentState: toRuntimeContainerStateSnapshot(status.State),
	}
	if terminated := status.LastTerminationState.Terminated; terminated != nil {
		item.LastTermination = toRuntimeContainerTerminationSnapshot(terminated)
	}
	return item
}

func toRuntimeContainerStateSnapshot(state corev1.ContainerState) RuntimeContainerStateSnapshot {
	if state.Running != nil {
		var startedAt *time.Time
		if !state.Running.StartedAt.IsZero() {
			value := state.Running.StartedAt.Time
			startedAt = &value
		}
		return RuntimeContainerStateSnapshot{
			State:     "running",
			StartedAt: startedAt,
		}
	}
	if state.Waiting != nil {
		return RuntimeContainerStateSnapshot{
			State:   "waiting",
			Reason:  strings.TrimSpace(state.Waiting.Reason),
			Message: strings.TrimSpace(state.Waiting.Message),
		}
	}
	if state.Terminated != nil {
		exitCode := state.Terminated.ExitCode
		var signal *int32
		if state.Terminated.Signal != 0 {
			sig := state.Terminated.Signal
			signal = &sig
		}
		var startedAt *time.Time
		if !state.Terminated.StartedAt.IsZero() {
			value := state.Terminated.StartedAt.Time
			startedAt = &value
		}
		var finishedAt *time.Time
		if !state.Terminated.FinishedAt.IsZero() {
			value := state.Terminated.FinishedAt.Time
			finishedAt = &value
		}
		return RuntimeContainerStateSnapshot{
			State:      "terminated",
			Reason:     strings.TrimSpace(state.Terminated.Reason),
			Message:    strings.TrimSpace(state.Terminated.Message),
			ExitCode:   &exitCode,
			Signal:     signal,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
		}
	}
	return RuntimeContainerStateSnapshot{State: "unknown"}
}

func toRuntimeContainerTerminationSnapshot(terminated *corev1.ContainerStateTerminated) *RuntimeContainerTerminationSnapshot {
	if terminated == nil {
		return nil
	}
	var startedAt *time.Time
	if !terminated.StartedAt.IsZero() {
		value := terminated.StartedAt.Time
		startedAt = &value
	}
	var finishedAt *time.Time
	if !terminated.FinishedAt.IsZero() {
		value := terminated.FinishedAt.Time
		finishedAt = &value
	}
	return &RuntimeContainerTerminationSnapshot{
		Reason:     strings.TrimSpace(terminated.Reason),
		Message:    strings.TrimSpace(terminated.Message),
		ExitCode:   terminated.ExitCode,
		Signal:     terminated.Signal,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}
}

func classifyPodDiagnosticStatus(pod corev1.Pod, ready bool) (status string, reason string, message string) {
	podReason := strings.TrimSpace(pod.Status.Reason)
	podMessage := strings.TrimSpace(pod.Status.Message)

	if pod.DeletionTimestamp != nil {
		return PodStatusTerminating, firstNonEmpty(podReason, "Terminating"), podMessage
	}
	if strings.EqualFold(podReason, "Evicted") {
		return PodStatusEvicted, "Evicted", podMessage
	}

	// Check InitContainer failures first.
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode == 0 {
			continue
		}
		if cs.State.Terminated != nil && strings.EqualFold(cs.State.Terminated.Reason, "OOMKilled") {
			return PodStatusOOMKilled, "OOMKilled", firstNonEmpty(cs.State.Terminated.Message, fmt.Sprintf("init container %s OOMKilled (exit %d)", cs.Name, cs.State.Terminated.ExitCode))
		}
		if !cs.Ready && cs.LastTerminationState.Terminated != nil && strings.EqualFold(cs.LastTerminationState.Terminated.Reason, "OOMKilled") {
			return PodStatusOOMKilled, "OOMKilled", firstNonEmpty(cs.LastTerminationState.Terminated.Message, fmt.Sprintf("init container %s OOMKilled (exit %d)", cs.Name, cs.LastTerminationState.Terminated.ExitCode))
		}
		if cs.State.Waiting != nil {
			if mapped := classifyContainerWaitingReason(cs.State.Waiting.Reason, true); mapped != "" && mapped != PodStatusContainerCreating {
				return mapped, strings.TrimSpace(cs.State.Waiting.Reason), strings.TrimSpace(cs.State.Waiting.Message)
			}
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return PodStatusInitError, firstNonEmpty(cs.State.Terminated.Reason, "InitError"), firstNonEmpty(cs.State.Terminated.Message, fmt.Sprintf("init container %s exited with code %d", cs.Name, cs.State.Terminated.ExitCode))
		}
	}

	// Check application Container failures.
	creatingReason := ""
	creatingMessage := ""
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil && strings.EqualFold(cs.State.Terminated.Reason, "OOMKilled") {
			return PodStatusOOMKilled, "OOMKilled", firstNonEmpty(cs.State.Terminated.Message, fmt.Sprintf("container %s OOMKilled (exit %d)", cs.Name, cs.State.Terminated.ExitCode))
		}
		if !cs.Ready && cs.LastTerminationState.Terminated != nil && strings.EqualFold(cs.LastTerminationState.Terminated.Reason, "OOMKilled") {
			return PodStatusOOMKilled, "OOMKilled", firstNonEmpty(cs.LastTerminationState.Terminated.Message, fmt.Sprintf("container %s OOMKilled (exit %d)", cs.Name, cs.LastTerminationState.Terminated.ExitCode))
		}
		if cs.State.Waiting != nil {
			mapped := classifyContainerWaitingReason(cs.State.Waiting.Reason, false)
			if mapped == PodStatusContainerCreating {
				creatingReason = strings.TrimSpace(cs.State.Waiting.Reason)
				creatingMessage = strings.TrimSpace(cs.State.Waiting.Message)
				continue
			}
			if mapped != "" {
				msg := strings.TrimSpace(cs.State.Waiting.Message)
				if msg == "" && cs.LastTerminationState.Terminated != nil {
					msg = fmt.Sprintf("last exit %d (%s)", cs.LastTerminationState.Terminated.ExitCode, firstNonEmpty(cs.LastTerminationState.Terminated.Reason, "Error"))
				}
				return mapped, strings.TrimSpace(cs.State.Waiting.Reason), msg
			}
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return PodStatusFailed, firstNonEmpty(cs.State.Terminated.Reason, "Error"), firstNonEmpty(cs.State.Terminated.Message, fmt.Sprintf("container %s exited with code %d", cs.Name, cs.State.Terminated.ExitCode))
		}
	}

	// Check scheduling conditions.
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
			if strings.EqualFold(condition.Reason, "Unschedulable") {
				return PodStatusUnschedulable, "Unschedulable", strings.TrimSpace(condition.Message)
			}
			return PodStatusPending, strings.TrimSpace(condition.Reason), strings.TrimSpace(condition.Message)
		}
	}

	if creatingReason != "" {
		return PodStatusContainerCreating, creatingReason, creatingMessage
	}

	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		return PodStatusSucceeded, podReason, podMessage
	case corev1.PodFailed:
		return PodStatusFailed, podReason, podMessage
	case corev1.PodRunning:
		if ready {
			return PodStatusReady, podReason, podMessage
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionFalse {
				return PodStatusNotReady, strings.TrimSpace(condition.Reason), strings.TrimSpace(condition.Message)
			}
		}
		return PodStatusNotReady, podReason, podMessage
	case corev1.PodPending:
		for _, cs := range pod.Status.InitContainerStatuses {
			if cs.State.Running != nil || (cs.State.Waiting != nil && cs.State.Waiting.Reason == "PodInitializing") {
				return PodStatusContainerCreating, "PodInitializing", ""
			}
		}
		return PodStatusPending, podReason, podMessage
	default:
		if pod.Status.Phase == "" {
			return PodStatusUnknown, podReason, podMessage
		}
		return strings.ToLower(string(pod.Status.Phase)), podReason, podMessage
	}
}

func classifyContainerWaitingReason(reason string, isInit bool) string {
	switch strings.TrimSpace(reason) {
	case "CrashLoopBackOff":
		if isInit {
			return PodStatusInitError
		}
		return PodStatusCrashLoopBackOff
	case "ImagePullBackOff":
		return PodStatusImagePullBackOff
	case "ErrImagePull", "InvalidImageName", "RegistryUnavailable", "InspectImageError":
		return PodStatusErrImagePull
	case "CreateContainerConfigError":
		return PodStatusCreateContainerConfigError
	case "CreateContainerError", "RunContainerError":
		return PodStatusCreateContainerError
	case "ContainerCreating", "PodInitializing":
		return PodStatusContainerCreating
	default:
		if strings.TrimSpace(reason) != "" {
			if isInit {
				return PodStatusInitError
			}
			return PodStatusCreateContainerError
		}
		return ""
	}
}

// ListDeploymentTargetEvents aggregates live Kubernetes Events in the namespace
// belonging to the deployment target's workload (Deployment/StatefulSet),
// owned ReplicaSets, HPA, and Pods (including recently evicted/deleted Pods).
func (c *Client) ListDeploymentTargetEvents(
	ctx context.Context,
	namespace string,
	deploymentTargetID string,
	workloadName string,
	workloadType string,
) ([]DeploymentTargetEventSnapshot, error) {
	namespace = strings.TrimSpace(namespace)
	deploymentTargetID = strings.TrimSpace(deploymentTargetID)
	workloadName = strings.TrimSpace(workloadName)
	if namespace == "" {
		return nil, fmt.Errorf("resource namespace is required")
	}
	if deploymentTargetID == "" {
		return nil, fmt.Errorf("deployment target is required")
	}

	workloadKind := "Deployment"
	if strings.EqualFold(strings.TrimSpace(workloadType), "StatefulSet") {
		workloadKind = "StatefulSet"
	}

	livePods, err := c.listRuntimePods(ctx, namespace, deploymentTargetID)
	if err != nil {
		return nil, err
	}
	livePodNames := make(map[string]bool, len(livePods))
	for _, pod := range livePods {
		livePodNames[pod.Name] = true
	}

	replicaSetNames := make(map[string]bool)
	if workloadKind == "Deployment" {
		if replicaSets, rsErr := c.client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{}); rsErr == nil {
			for _, rs := range replicaSets.Items {
				if isReplicaSetOfTarget(rs, deploymentTargetID, workloadName) {
					replicaSetNames[rs.Name] = true
				}
			}
		}
	}

	eventList, err := c.client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	items := make([]DeploymentTargetEventSnapshot, 0)
	for _, event := range eventList.Items {
		kind := strings.TrimSpace(event.InvolvedObject.Kind)
		name := strings.TrimSpace(event.InvolvedObject.Name)
		if !eventBelongsToDeploymentTarget(kind, name, workloadKind, workloadName, livePodNames, replicaSetNames) {
			continue
		}
		base := eventSnapshot(event)
		items = append(items, DeploymentTargetEventSnapshot{
			ID:           base.ID,
			Type:         base.Type,
			Reason:       base.Reason,
			Message:      base.Message,
			Source:       base.Source,
			Count:        base.Count,
			FirstSeen:    base.FirstSeen,
			LastSeen:     base.LastSeen,
			InvolvedKind: kind,
			InvolvedName: name,
		})
	}

	sort.SliceStable(items, func(left, right int) bool {
		if !items[left].LastSeen.Equal(items[right].LastSeen) {
			return items[left].LastSeen.After(items[right].LastSeen)
		}
		return items[left].ID < items[right].ID
	})
	return items, nil
}

func isReplicaSetOfTarget(rs appsv1.ReplicaSet, deploymentTargetID, workloadName string) bool {
	if rs.Labels[ManagedByLabel] == ManagedByValue && rs.Labels[DeploymentTargetIDLabel] == deploymentTargetID {
		return true
	}
	if workloadName == "" {
		return false
	}
	for _, owner := range rs.OwnerReferences {
		if owner.Kind == "Deployment" && owner.Name == workloadName {
			return true
		}
	}
	return false
}

func eventBelongsToDeploymentTarget(
	kind string,
	name string,
	workloadKind string,
	workloadName string,
	livePodNames map[string]bool,
	replicaSetNames map[string]bool,
) bool {
	if name == "" {
		return false
	}
	switch kind {
	case "Deployment", "StatefulSet":
		return workloadName != "" && kind == workloadKind && name == workloadName
	case "HorizontalPodAutoscaler", "Service":
		return workloadName != "" && name == workloadName
	case "ReplicaSet":
		return replicaSetNames[name]
	case "Pod":
		if livePodNames[name] {
			return true
		}
		for rsName := range replicaSetNames {
			if strings.HasPrefix(name, rsName+"-") {
				return true
			}
		}
		if workloadKind == "StatefulSet" && workloadName != "" && isStatefulSetOrdinalPodName(name, workloadName) {
			return true
		}
		return false
	default:
		return false
	}
}

func isStatefulSetOrdinalPodName(podName, statefulSetName string) bool {
	prefix := statefulSetName + "-"
	if !strings.HasPrefix(podName, prefix) {
		return false
	}
	ordinal := strings.TrimPrefix(podName, prefix)
	if ordinal == "" {
		return false
	}
	_, err := strconv.Atoi(ordinal)
	return err == nil
}
