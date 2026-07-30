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
	Name      string
	Capacity  string
	CreatedAt time.Time
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
			Name:      item.Name,
			Capacity:  capacity.String(),
			CreatedAt: item.CreationTimestamp.Time,
		})
	}
	return output, nil
}
