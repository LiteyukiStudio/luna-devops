package kubernetes

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validateApplicationDataVolume(volume ApplicationDataVolume) error {
	switch dataVolumeSourceType(volume) {
	case "existingClaim", "retainedClaim":
		if strings.TrimSpace(volume.ExistingClaimName) == "" {
			return fmt.Errorf("existing claim data volume %s requires claim name", persistentDataVolumeName(volume))
		}
		if dataVolumeSourceType(volume) == "retainedClaim" && strings.TrimSpace(volume.RetainedVolumeID) == "" {
			return fmt.Errorf("retained claim data volume %s requires retained volume id", persistentDataVolumeName(volume))
		}
	case "emptyDir":
		if sizeLimit := strings.TrimSpace(volume.EmptyDirSizeLimit); sizeLimit != "" {
			quantity, err := resource.ParseQuantity(sizeLimit)
			if err != nil || quantity.Sign() <= 0 {
				return fmt.Errorf("emptyDir size limit must be a positive resource quantity")
			}
		}
	}
	return nil
}

func (c *Client) ApplyPersistentDataVolume(ctx context.Context, spec ApplicationResourcesSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" {
		return fmt.Errorf("application resource name and namespace are required")
	}
	if strings.TrimSpace(spec.ProjectID) == "" || strings.TrimSpace(spec.ApplicationID) == "" || strings.TrimSpace(spec.DeploymentTargetID) == "" {
		return fmt.Errorf("project, application, and deployment target ids are required")
	}
	if err := c.ensureApplicationStorageOwnership(ctx, spec); err != nil {
		return err
	}
	for _, volume := range persistentDataVolumes(spec) {
		if !dataVolumeNeedsPVC(volume) {
			continue
		}
		if _, err := persistentDataCapacity(volume); err != nil {
			return err
		}
		if err := c.applyPersistentDataVolume(ctx, spec, volume, appObjectLabels(spec)); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyPersistentDataVolume(ctx context.Context, spec ApplicationResourcesSpec, volume ApplicationDataVolume, labels map[string]string) error {
	if dataVolumeSourceType(volume) == "retainedClaim" {
		claimName := persistentDataPVCName(spec, volume)
		existing, err := c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(ctx, claimName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if strings.TrimSpace(existing.Labels[RetainedVolumeIDLabel]) != strings.TrimSpace(volume.RetainedVolumeID) {
			return fmt.Errorf("retained volume %s no longer owns PersistentVolumeClaim %s/%s", volume.RetainedVolumeID, spec.Namespace, claimName)
		}
		existing.Labels = labels
		_, err = c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
		return err
	}
	capacity, err := persistentDataCapacity(volume)
	if err != nil {
		return err
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: persistentDataPVCName(spec, volume), Namespace: spec.Namespace, Labels: labels},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{persistentDataAccessMode(spec)},
			Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: capacity}},
		},
	}
	if mode := persistentDataVolumeMode(spec); mode != "" {
		pvc.Spec.VolumeMode = &mode
	}
	if storageClassName := strings.TrimSpace(spec.DataStorageClassName); storageClassName != "" {
		pvc.Spec.StorageClassName = &storageClassName
	}
	existing, err := c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("PersistentVolumeClaim", existing, labels); err != nil {
		return err
	}
	existing.Labels = pvc.Labels
	existing.Spec.Resources.Requests[corev1.ResourceStorage] = capacity
	_, err = c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func persistentDataAccessMode(spec ApplicationResourcesSpec) corev1.PersistentVolumeAccessMode {
	switch strings.TrimSpace(spec.DataAccessMode) {
	case string(corev1.ReadWriteMany):
		return corev1.ReadWriteMany
	case string(corev1.ReadOnlyMany):
		return corev1.ReadOnlyMany
	default:
		return corev1.ReadWriteOnce
	}
}

func persistentDataVolumeMode(spec ApplicationResourcesSpec) corev1.PersistentVolumeMode {
	switch strings.TrimSpace(spec.DataVolumeMode) {
	case string(corev1.PersistentVolumeBlock):
		return corev1.PersistentVolumeBlock
	case string(corev1.PersistentVolumeFilesystem):
		return corev1.PersistentVolumeFilesystem
	default:
		return ""
	}
}

func persistentDataVolumes(spec ApplicationResourcesSpec) []ApplicationDataVolume {
	if len(spec.DataVolumes) > 0 {
		volumes := make([]ApplicationDataVolume, 0, len(spec.DataVolumes))
		for _, volume := range spec.DataVolumes {
			name := firstNonEmpty(volume.Name, "data")
			volumes = append(volumes, ApplicationDataVolume{
				Name:              name,
				MountPath:         firstNonEmpty(volume.MountPath, "/data"),
				Capacity:          firstNonEmpty(volume.Capacity, "1Gi"),
				SourceType:        dataVolumeSourceType(volume),
				ExistingClaimName: strings.TrimSpace(volume.ExistingClaimName),
				RetainedVolumeID:  strings.TrimSpace(volume.RetainedVolumeID),
				EmptyDirMedium:    strings.TrimSpace(volume.EmptyDirMedium),
				EmptyDirSizeLimit: strings.TrimSpace(volume.EmptyDirSizeLimit),
			})
		}
		return volumes
	}
	return []ApplicationDataVolume{{
		Name:       "data",
		MountPath:  persistentDataMountPath(spec),
		Capacity:   firstNonEmpty(spec.DataCapacity, "1Gi"),
		SourceType: "managed",
	}}
}

func dataVolumeSourceType(volume ApplicationDataVolume) string {
	switch strings.TrimSpace(volume.SourceType) {
	case "existingClaim":
		return "existingClaim"
	case "retainedClaim":
		return "retainedClaim"
	case "emptyDir":
		return "emptyDir"
	default:
		return "managed"
	}
}

func dataVolumeNeedsPVC(volume ApplicationDataVolume) bool {
	return dataVolumeSourceType(volume) == "managed" || dataVolumeSourceType(volume) == "retainedClaim"
}

func applicationDataVolumeSource(spec ApplicationResourcesSpec, volume ApplicationDataVolume, name string) corev1.Volume {
	switch dataVolumeSourceType(volume) {
	case "existingClaim", "retainedClaim":
		return corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: strings.TrimSpace(volume.ExistingClaimName),
			}},
		}
	case "emptyDir":
		emptyDir := &corev1.EmptyDirVolumeSource{}
		if medium := strings.TrimSpace(volume.EmptyDirMedium); medium != "" {
			emptyDir.Medium = corev1.StorageMedium(medium)
		}
		if sizeLimit := strings.TrimSpace(volume.EmptyDirSizeLimit); sizeLimit != "" {
			if quantity, err := resource.ParseQuantity(sizeLimit); err == nil {
				emptyDir.SizeLimit = &quantity
			}
		}
		return corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{EmptyDir: emptyDir}}
	default:
		return corev1.Volume{
			Name:         name,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: persistentDataPVCName(spec, volume)}},
		}
	}
}

func persistentDataPVCName(spec ApplicationResourcesSpec, volume ApplicationDataVolume) string {
	if dataVolumeSourceType(volume) == "retainedClaim" {
		return strings.TrimSpace(volume.ExistingClaimName)
	}
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
