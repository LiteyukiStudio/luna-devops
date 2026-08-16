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
	Phase             string
	Message           string
	CreatedAt         time.Time
	DesiredReplicas   int32
	UpdatedReplicas   int32
	ReadyReplicas     int32
	AvailableReplicas int32
	ObservedAt        time.Time
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
		Message:           fmt.Sprintf("rollout 进行中：updated=%d ready=%d available=%d desired=%d", deployment.Status.UpdatedReplicas, deployment.Status.ReadyReplicas, deployment.Status.AvailableReplicas, desired),
		CreatedAt:         deployment.CreationTimestamp.Time,
		DesiredReplicas:   desired,
		UpdatedReplicas:   deployment.Status.UpdatedReplicas,
		ReadyReplicas:     deployment.Status.ReadyReplicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		ObservedAt:        time.Now().UTC(),
	}

	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse && condition.Reason == "ProgressDeadlineExceeded" {
			snapshot.Phase = DeploymentFailed
			snapshot.Message = firstNonEmpty(condition.Message, "Deployment rollout exceeded progress deadline")
			return snapshot
		}
	}
	if deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas >= desired &&
		deployment.Status.ReadyReplicas >= desired &&
		deployment.Status.AvailableReplicas >= desired {
		snapshot.Phase = DeploymentSucceeded
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
	snapshot := DeploymentSnapshot{
		Phase:             DeploymentRunning,
		Message:           fmt.Sprintf("StatefulSet rollout 进行中：updated=%d ready=%d available=%d desired=%d", statefulSet.Status.UpdatedReplicas, statefulSet.Status.ReadyReplicas, statefulSet.Status.AvailableReplicas, desired),
		CreatedAt:         statefulSet.CreationTimestamp.Time,
		DesiredReplicas:   desired,
		UpdatedReplicas:   statefulSet.Status.UpdatedReplicas,
		ReadyReplicas:     statefulSet.Status.ReadyReplicas,
		AvailableReplicas: statefulSet.Status.AvailableReplicas,
		ObservedAt:        time.Now().UTC(),
	}
	if statefulSet.Status.ObservedGeneration >= statefulSet.Generation &&
		statefulSet.Status.UpdatedReplicas >= desired &&
		statefulSet.Status.ReadyReplicas >= desired {
		snapshot.Phase = DeploymentSucceeded
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
