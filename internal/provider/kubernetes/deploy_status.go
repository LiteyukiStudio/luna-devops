package kubernetes

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DeploymentRunning   = "running"
	DeploymentSucceeded = "succeeded"
	DeploymentFailed    = "failed"
)

type DeploymentSnapshot struct {
	Phase   string
	Message string
	// ReleaseID is the release recorded on the workload pod template, i.e. the
	// release whose desired state the workload currently converges to.
	ReleaseID         string
	CreatedAt         time.Time
	DesiredReplicas   int32
	UpdatedReplicas   int32
	ReadyReplicas     int32
	AvailableReplicas int32
	// RolloutSettled is true when the current generation's rollout has already
	// converged (NewReplicaSetAvailable / current revision reached), so any
	// missing Ready/Available replicas indicate post-rollout runtime degradation
	// rather than an active rollout in progress.
	RolloutSettled bool
	// ReplicaFailure is true when Kubernetes reports DeploymentReplicaFailure=True.
	ReplicaFailure bool
	// Resources is the per-replica envelope rendered into the current Pod template.
	Resources  PodTemplateResources
	ObservedAt time.Time
}

func (c *Client) GetDeploymentSnapshot(ctx context.Context, namespace, name string) (DeploymentSnapshot, error) {
	deployment, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return c.getStatefulSetSnapshot(ctx, namespace, name)
	}
	if err != nil {
		return DeploymentSnapshot{}, err
	}
	return deploymentStatusSnapshot(deployment), nil
}

// GetWorkloadSnapshot observes the exact workload type referenced by the platform.
// Unlike GetDeploymentSnapshot, it does not fall back to another resource kind.
func (c *Client) GetWorkloadSnapshot(ctx context.Context, namespace, name, workloadType string) (DeploymentSnapshot, error) {
	if workloadType == "StatefulSet" {
		return c.getStatefulSetSnapshot(ctx, namespace, name)
	}
	deployment, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return DeploymentSnapshot{}, err
	}
	return deploymentStatusSnapshot(deployment), nil
}

func deploymentStatusSnapshot(deployment *appsv1.Deployment) DeploymentSnapshot {
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	snapshot := DeploymentSnapshot{
		Phase:             DeploymentRunning,
		Message:           fmt.Sprintf("rollout 进行中：updated=%d ready=%d available=%d total=%d desired=%d", deployment.Status.UpdatedReplicas, deployment.Status.ReadyReplicas, deployment.Status.AvailableReplicas, deployment.Status.Replicas, desired),
		CreatedAt:         deployment.CreationTimestamp.Time,
		DesiredReplicas:   desired,
		UpdatedReplicas:   deployment.Status.UpdatedReplicas,
		ReadyReplicas:     deployment.Status.ReadyReplicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		Resources:         podTemplateResources(deployment.Spec.Template),
		ReleaseID:         deployment.Spec.Template.Annotations[ReleaseIDLabel],
		ObservedAt:        time.Now().UTC(),
	}

	if deployment.Status.ObservedGeneration >= deployment.Generation {
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionTrue && condition.Reason == "NewReplicaSetAvailable" {
				snapshot.RolloutSettled = true
			}
			if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue {
				snapshot.ReplicaFailure = true
			}
		}
	}

	if desired > 0 {
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse && condition.Reason == "ProgressDeadlineExceeded" {
				snapshot.Phase = DeploymentFailed
				snapshot.Message = firstNonEmpty(condition.Message, "Deployment rollout exceeded progress deadline")
				return snapshot
			}
		}
	}
	// 与 kubectl rollout status 一致：滚动更新期间新 ReplicaSet 的未就绪 Pod 已计入 updated，
	// 旧 Pod 仍计入 ready/available，因此还要求旧副本全部退出且可用数追上新版本副本数。
	replicasReady := deployment.Status.UpdatedReplicas >= desired &&
		deployment.Status.Replicas <= deployment.Status.UpdatedReplicas &&
		deployment.Status.ReadyReplicas >= desired &&
		deployment.Status.AvailableReplicas >= deployment.Status.UpdatedReplicas
	if desired == 0 {
		replicasReady = deployment.Status.Replicas == 0 &&
			deployment.Status.UpdatedReplicas == 0 &&
			deployment.Status.ReadyReplicas == 0 &&
			deployment.Status.AvailableReplicas == 0
	}
	if deployment.Status.ObservedGeneration >= deployment.Generation && replicasReady {
		snapshot.Phase = DeploymentSucceeded
		snapshot.RolloutSettled = true
		snapshot.Message = "Deployment rollout completed"
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
				snapshot.Message = firstNonEmpty(condition.Message, snapshot.Message)
				break
			}
		}
	}
	return snapshot
}

func (c *Client) getStatefulSetSnapshot(ctx context.Context, namespace, name string) (DeploymentSnapshot, error) {
	statefulSet, err := c.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return DeploymentSnapshot{}, err
	}
	desired := int32(1)
	if statefulSet.Spec.Replicas != nil {
		desired = *statefulSet.Spec.Replicas
	}
	revisionSettled := statefulSet.Status.ObservedGeneration >= statefulSet.Generation &&
		statefulSet.Status.UpdatedReplicas >= desired &&
		(statefulSet.Status.UpdateRevision == "" || statefulSet.Status.CurrentRevision == statefulSet.Status.UpdateRevision)
	snapshot := DeploymentSnapshot{
		Phase:             DeploymentRunning,
		Message:           fmt.Sprintf("StatefulSet rollout 进行中：updated=%d ready=%d available=%d desired=%d", statefulSet.Status.UpdatedReplicas, statefulSet.Status.ReadyReplicas, statefulSet.Status.AvailableReplicas, desired),
		CreatedAt:         statefulSet.CreationTimestamp.Time,
		DesiredReplicas:   desired,
		UpdatedReplicas:   statefulSet.Status.UpdatedReplicas,
		ReadyReplicas:     statefulSet.Status.ReadyReplicas,
		AvailableReplicas: statefulSet.Status.AvailableReplicas,
		RolloutSettled:    revisionSettled,
		Resources:         podTemplateResources(statefulSet.Spec.Template),
		ReleaseID:         statefulSet.Spec.Template.Annotations[ReleaseIDLabel],
		ObservedAt:        time.Now().UTC(),
	}
	// 滚动更新完成需要所有 Pod 都切到新 revision；仅按数量判断会把旧 revision 的就绪 Pod 算作新版本。
	replicasReady := statefulSet.Status.UpdatedReplicas >= desired &&
		statefulSet.Status.ReadyReplicas >= desired &&
		(statefulSet.Status.UpdateRevision == "" || statefulSet.Status.CurrentRevision == statefulSet.Status.UpdateRevision)
	if desired == 0 {
		replicasReady = statefulSet.Status.Replicas == 0 &&
			statefulSet.Status.UpdatedReplicas == 0 &&
			statefulSet.Status.ReadyReplicas == 0 &&
			statefulSet.Status.AvailableReplicas == 0
	}
	if statefulSet.Status.ObservedGeneration >= statefulSet.Generation && replicasReady {
		snapshot.Phase = DeploymentSucceeded
		snapshot.RolloutSettled = true
		snapshot.Message = "StatefulSet rollout completed"
	}
	return snapshot, nil
}

func (c *Client) RestartDeployment(ctx context.Context, namespace, name string) error {
	deployment, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return c.restartStatefulSet(ctx, namespace, name)
	}
	if err != nil {
		return err
	}
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	_, err = c.client.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

func (c *Client) restartStatefulSet(ctx context.Context, namespace, name string) error {
	statefulSet, err := c.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if statefulSet.Spec.Template.Annotations == nil {
		statefulSet.Spec.Template.Annotations = map[string]string{}
	}
	statefulSet.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	_, err = c.client.AppsV1().StatefulSets(namespace).Update(ctx, statefulSet, metav1.UpdateOptions{})
	return err
}
