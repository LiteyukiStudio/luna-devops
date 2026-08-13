package kubernetes

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type PersistentVolumeClaimSnapshot struct {
	Name             string    `json:"name"`
	Capacity         string    `json:"capacity"`
	StorageClassName string    `json:"storageClassName"`
	AccessMode       string    `json:"accessMode"`
	VolumeMode       string    `json:"volumeMode"`
	CreatedAt        time.Time `json:"createdAt"`
}

// ListManagedPersistentVolumeClaims returns live PVCs created and managed for a
// deployment target. Existing claims supplied by users are intentionally not
// included because the platform does not own or bill their declared capacity.
func (c *Client) ListManagedPersistentVolumeClaims(ctx context.Context, namespace, deploymentTargetID string) ([]PersistentVolumeClaimSnapshot, error) {
	selector := labels.Set{
		ManagedByLabel:          ManagedByValue,
		DeploymentTargetIDLabel: strings.TrimSpace(deploymentTargetID),
	}.AsSelector().String()
	items, err := c.client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	output := make([]PersistentVolumeClaimSnapshot, 0, len(items.Items))
	for _, item := range items.Items {
		capacity := item.Status.Capacity[corev1.ResourceStorage]
		if capacity.IsZero() {
			capacity = item.Spec.Resources.Requests[corev1.ResourceStorage]
		}
		if capacity.IsZero() {
			continue
		}
		output = append(output, PersistentVolumeClaimSnapshot{
			Name: item.Name, Capacity: capacity.String(), CreatedAt: item.CreationTimestamp.Time,
			StorageClassName: valueOrEmpty(item.Spec.StorageClassName),
			AccessMode:       firstAccessMode(item.Spec.AccessModes), VolumeMode: valueOrEmpty(item.Spec.VolumeMode),
		})
	}
	return output, nil
}

func valueOrEmpty[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func firstAccessMode(values []corev1.PersistentVolumeAccessMode) string {
	if len(values) == 0 {
		return ""
	}
	return string(values[0])
}

// RetainManagedPersistentVolumeClaim transfers a PVC from an application
// lifecycle to a retained-volume lifecycle without changing its Kubernetes name.
func (c *Client) RetainManagedPersistentVolumeClaim(ctx context.Context, namespace, claimName, deploymentTargetID, retainedVolumeID string) error {
	claim, err := c.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("PersistentVolumeClaim", claim, map[string]string{DeploymentTargetIDLabel: deploymentTargetID}); err != nil {
		return err
	}
	labels := claim.GetLabels()
	delete(labels, ApplicationIDLabel)
	delete(labels, EnvironmentIDLabel)
	delete(labels, DeploymentTargetIDLabel)
	delete(labels, ReleaseIDLabel)
	labels[RetainedVolumeIDLabel] = retainedVolumeID
	claim.SetLabels(labels)
	_, err = c.client.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, claim, metav1.UpdateOptions{})
	return err
}
