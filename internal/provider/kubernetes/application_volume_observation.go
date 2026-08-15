package kubernetes

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplicationVolumeAttachment is an authoritative view of one volume wired
// into the current workload pod template. It deliberately excludes Secret and
// ConfigMap contents; callers only need the binding identity and container
// path when promoting a reserved project-volume relation.
type ApplicationVolumeAttachment struct {
	ClaimName  string
	MountPath  string
	DevicePath string
	ReadOnly   bool
	EmptyDir   bool
}

// ObserveApplicationVolumeAttachments reads the active Deployment or
// StatefulSet pod template. It does not cache observations because Kubernetes
// remains the single source of truth for runtime state.
func (c *Client) ObserveApplicationVolumeAttachments(ctx context.Context, namespace, name, workloadType string) (map[string]ApplicationVolumeAttachment, error) {
	var podSpec corev1.PodSpec
	switch applicationWorkloadType(ApplicationResourcesSpec{WorkloadType: workloadType}) {
	case "StatefulSet":
		item, err := c.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		podSpec = item.Spec.Template.Spec
	default:
		item, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		podSpec = item.Spec.Template.Spec
	}

	volumes := make(map[string]ApplicationVolumeAttachment, len(podSpec.Volumes))
	for _, item := range podSpec.Volumes {
		attachment := ApplicationVolumeAttachment{}
		switch {
		case item.PersistentVolumeClaim != nil:
			attachment.ClaimName = strings.TrimSpace(item.PersistentVolumeClaim.ClaimName)
			attachment.ReadOnly = item.PersistentVolumeClaim.ReadOnly
		case item.EmptyDir != nil:
			attachment.EmptyDir = true
		default:
			continue
		}
		volumes[item.Name] = attachment
	}
	if len(podSpec.Containers) == 0 {
		return nil, fmt.Errorf("application workload has no containers")
	}
	appContainer := podSpec.Containers[0]
	for _, mount := range appContainer.VolumeMounts {
		attachment, ok := volumes[mount.Name]
		if !ok {
			continue
		}
		attachment.MountPath = strings.TrimSpace(mount.MountPath)
		attachment.ReadOnly = attachment.ReadOnly || mount.ReadOnly
		volumes[mount.Name] = attachment
	}
	for _, device := range appContainer.VolumeDevices {
		attachment, ok := volumes[device.Name]
		if !ok {
			continue
		}
		attachment.DevicePath = strings.TrimSpace(device.DevicePath)
		volumes[device.Name] = attachment
	}
	return volumes, nil
}
