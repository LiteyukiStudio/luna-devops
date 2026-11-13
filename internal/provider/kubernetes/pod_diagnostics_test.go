package kubernetes

import (
	"context"
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestBuildRuntimePodSnapshotExtractsOOMAndCrashDetails(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	finished := now.Add(-30 * time.Second)
	started := now.Add(-2 * time.Minute)
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "order-api-7f89d5b6c-x9k2p",
			Namespace:         "ns-demo",
			Labels:            map[string]string{ReleaseIDLabel: "rel_1"},
			CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Minute)),
		},
		Spec: corev1.PodSpec{
			NodeName:       "node-1",
			InitContainers: []corev1.Container{{Name: "migrate", Image: "repo/migrate:1.0"}},
			Containers:     []corev1.Container{{Name: "app", Image: "repo/order-api:1.2"}, {Name: "sidecar", Image: "repo/envoy:1.0"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady", Message: "containers with unready status: [app]"},
			},
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "migrate",
					Image: "repo/migrate:1.0",
					Ready: true,
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, Reason: "Completed"}},
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					Image:        "repo/order-api:1.2",
					Ready:        false,
					RestartCount: 4,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "back-off 2m40s restarting failed container=app",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode:   137,
							Reason:     "OOMKilled",
							StartedAt:  metav1.NewTime(started),
							FinishedAt: metav1.NewTime(finished),
						},
					},
				},
				{
					Name:  "sidecar",
					Image: "repo/envoy:1.0",
					Ready: true,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(started)}},
				},
			},
		},
	}

	snap := buildRuntimePodSnapshot(pod)
	if snap.Status != PodStatusOOMKilled {
		t.Fatalf("status = %q, want %q", snap.Status, PodStatusOOMKilled)
	}
	if snap.ReadyContainers != 1 || snap.TotalContainers != 2 || snap.RestartCount != 4 {
		t.Fatalf("counts = ready %d/%d restarts %d", snap.ReadyContainers, snap.TotalContainers, snap.RestartCount)
	}
	if snap.NodeName != "node-1" || len(snap.InitContainers) != 1 || len(snap.ContainerStatuses) != 3 {
		t.Fatalf("unexpected container details: %+v", snap)
	}
	var appStatus *RuntimeContainerStatusSnapshot
	for i := range snap.ContainerStatuses {
		if snap.ContainerStatuses[i].Name == "app" {
			appStatus = &snap.ContainerStatuses[i]
			break
		}
	}
	if appStatus == nil || appStatus.LastTermination == nil {
		t.Fatalf("app lastTermination missing: %+v", appStatus)
	}
	if appStatus.LastTermination.Reason != "OOMKilled" || appStatus.LastTermination.ExitCode != 137 {
		t.Fatalf("lastTermination = %+v", appStatus.LastTermination)
	}
}

func TestEvaluateWorkloadRuntimeDistinguishesRolloutFromDegradation(t *testing.T) {
	desired := int32(2)
	// Active fresh rollout: ObservedGeneration == Generation, NewReplicaSetAvailable not yet reached, pods ContainerCreating.
	rollingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Generation: 3},
		Spec:       appsv1.DeploymentSpec{Replicas: &desired},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 3,
			Replicas:           3,
			UpdatedReplicas:    1,
			ReadyReplicas:      1,
			AvailableReplicas:  1,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "ReplicaSetUpdated"},
			},
		},
	}
	rollingObs := EvaluateWorkloadRuntime(deploymentStatusSnapshot(rollingDeploy), []RuntimePodSnapshot{
		{Name: "api-new", Status: PodStatusContainerCreating},
		{Name: "api-old", Status: PodStatusReady, Ready: true},
	})
	if rollingObs.Status != "progressing" || rollingObs.ObservationCode != "deployment_target.rollout_progressing" {
		t.Fatalf("rolling observation = %+v, want progressing", rollingObs)
	}

	// Post-rollout runtime crash: NewReplicaSetAvailable already true, ready dropped to 0, pod CrashLoopBackOff.
	degradedDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Generation: 3},
		Spec:       appsv1.DeploymentSpec{Replicas: &desired},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 3,
			Replicas:           2,
			UpdatedReplicas:    2,
			ReadyReplicas:      0,
			AvailableReplicas:  0,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse, Reason: "MinimumReplicasUnavailable"},
			},
		},
	}
	crashObs := EvaluateWorkloadRuntime(deploymentStatusSnapshot(degradedDeploy), []RuntimePodSnapshot{
		{Name: "api-1", Status: PodStatusCrashLoopBackOff},
		{Name: "api-2", Status: PodStatusNotReady},
	})
	if crashObs.Status != "degraded" || crashObs.ObservationCode != "runtime.crash_loop_back_off" {
		t.Fatalf("crash observation = %+v, want degraded / runtime.crash_loop_back_off", crashObs)
	}

	// Post-rollout probe unhealthy without container crash.
	probeObs := EvaluateWorkloadRuntime(deploymentStatusSnapshot(degradedDeploy), []RuntimePodSnapshot{
		{Name: "api-1", Status: PodStatusNotReady},
	})
	if probeObs.Status != "degraded" || probeObs.ObservationCode != "runtime.probe_unhealthy" {
		t.Fatalf("probe observation = %+v, want degraded / runtime.probe_unhealthy", probeObs)
	}
}

func TestListDeploymentTargetEventsMatchesWorkloadReplicaSetsAndPodsWithoutPrefixCollision(t *testing.T) {
	now := time.Now().UTC()
	client := NewClientForInterface(fake.NewSimpleClientset(
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "api-7f89d5b6c",
				Namespace:       "ns-demo",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "api"}},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "api-worker-6a12b3c4d",
				Namespace:       "ns-demo",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "api-worker"}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-7f89d5b6c-live1",
				Namespace: "ns-demo",
				Labels: map[string]string{
					ManagedByLabel:          ManagedByValue,
					DeploymentTargetIDLabel: "dplt_api",
				},
			},
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev-deploy", Namespace: "ns-demo"},
			InvolvedObject: corev1.ObjectReference{Kind: "Deployment", Name: "api"},
			Type:           "Normal",
			Reason:         "ScalingReplicaSet",
			Message:        "Scaled up replica set api-7f89d5b6c to 1",
			LastTimestamp:  metav1.NewTime(now.Add(-2 * time.Minute)),
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev-evicted-pod", Namespace: "ns-demo"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api-7f89d5b6c-evicted0"},
			Type:           "Warning",
			Reason:         "Evicted",
			Message:        "The node was low on resource: memory.",
			LastTimestamp:  metav1.NewTime(now.Add(-1 * time.Minute)),
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev-other-workload-pod", Namespace: "ns-demo"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api-worker-6a12b3c4d-pod1"},
			Type:           "Warning",
			Reason:         "BackOff",
			Message:        "Back-off restarting failed container",
			LastTimestamp:  metav1.NewTime(now),
		},
	))

	events, err := client.ListDeploymentTargetEvents(context.Background(), "ns-demo", "dplt_api", "api", "Deployment")
	if err != nil {
		t.Fatalf("ListDeploymentTargetEvents error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2: %+v", len(events), events)
	}
	if events[0].Reason != "Evicted" || events[1].Reason != "ScalingReplicaSet" {
		t.Fatalf("events order/content = %+v", events)
	}
}

func TestRuntimePodLogsPreviousSelectsRestartedReplicaAndValidatesInitContainer(t *testing.T) {
	now := time.Now().UTC()
	healthyPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "api-healthy",
			Namespace:         "ns-demo",
			CreationTimestamp: metav1.NewTime(now),
			Labels: map[string]string{
				ManagedByLabel:          ManagedByValue,
				DeploymentTargetIDLabel: "dplt_api",
				ReleaseIDLabel:          "rel_1",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true, RestartCount: 0, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
		},
	}
	crashedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "api-crashed",
			Namespace:         "ns-demo",
			CreationTimestamp: metav1.NewTime(now.Add(-time.Minute)),
			Labels: map[string]string{
				ManagedByLabel:          ManagedByValue,
				DeploymentTargetIDLabel: "dplt_api",
				ReleaseIDLabel:          "rel_1",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "init-db"}},
			Containers:     []corev1.Container{{Name: "app"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			InitContainerStatuses: []corev1.ContainerStatus{{
				Name:  "init-db",
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}},
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:                 "app",
				Ready:                false,
				RestartCount:         2,
				State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}},
			}},
		},
	}
	client := NewClientForInterface(fake.NewSimpleClientset(healthyPod, crashedPod))

	// Automatic selection for Previous=true picks api-crashed instead of api-healthy.
	result, err := client.RuntimePodLogs(context.Background(), RuntimePodLogsOptions{
		RuntimePodSelection: RuntimePodSelection{
			Namespace:          "ns-demo",
			DeploymentTargetID: "dplt_api",
			PreferredReleaseID: "rel_1",
		},
		Previous: true,
	})
	if err != nil {
		t.Fatalf("RuntimePodLogs previous error: %v", err)
	}
	if result.Pod != "api-crashed" || !result.Previous {
		t.Fatalf("result = %+v, want pod=api-crashed previous=true", result)
	}

	// Explicit healthyPod with Previous=true returns ErrRuntimePreviousLogNotFound.
	_, err = client.RuntimePodLogs(context.Background(), RuntimePodLogsOptions{
		RuntimePodSelection: RuntimePodSelection{
			Namespace:          "ns-demo",
			DeploymentTargetID: "dplt_api",
			Pod:                "api-healthy",
		},
		Previous: true,
	})
	if !errors.Is(err, ErrRuntimePreviousLogNotFound) {
		t.Fatalf("expected ErrRuntimePreviousLogNotFound, got %v", err)
	}

	// Init container logs can be read via RuntimePodLogs.
	initResult, err := client.RuntimePodLogs(context.Background(), RuntimePodLogsOptions{
		RuntimePodSelection: RuntimePodSelection{
			Namespace:          "ns-demo",
			DeploymentTargetID: "dplt_api",
			Pod:                "api-crashed",
			Container:          "init-db",
		},
	})
	if err != nil || initResult.Container != "init-db" {
		t.Fatalf("init container logs = (%+v, %v)", initResult, err)
	}
}
