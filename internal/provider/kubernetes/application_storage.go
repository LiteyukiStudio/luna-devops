package kubernetes

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func validateApplicationDataVolume(volume ApplicationDataVolume) error {
	mountPath := strings.TrimSpace(volume.MountPath)
	devicePath := strings.TrimSpace(volume.DevicePath)
	switch dataVolumeSourceType(volume) {
	case "projectVolume":
		if (mountPath == "") == (devicePath == "") {
			return fmt.Errorf("project volume %s requires exactly one mount path or device path", persistentDataVolumeName(volume))
		}
		if strings.TrimSpace(volume.ProjectVolumeID) == "" || strings.TrimSpace(volume.ClaimName) == "" {
			return fmt.Errorf("project volume %s requires project volume id and authoritative claim name", persistentDataVolumeName(volume))
		}
	case "emptyDir":
		if mountPath == "" || devicePath != "" || strings.TrimSpace(volume.ProjectVolumeID) != "" || strings.TrimSpace(volume.ClaimName) != "" || volume.ReadOnly {
			return fmt.Errorf("emptyDir %s requires a mount path and cannot contain project-volume fields", persistentDataVolumeName(volume))
		}
		if sizeLimit := strings.TrimSpace(volume.EmptyDirSizeLimit); sizeLimit != "" {
			quantity, err := resource.ParseQuantity(sizeLimit)
			if err != nil || quantity.Sign() <= 0 {
				return fmt.Errorf("emptyDir size limit must be a positive resource quantity")
			}
		}
	default:
		return fmt.Errorf("data volume %s source type must be projectVolume or emptyDir", persistentDataVolumeName(volume))
	}
	return nil
}

func persistentDataVolumes(spec ApplicationResourcesSpec) []ApplicationDataVolume {
	volumes := make([]ApplicationDataVolume, 0, len(spec.DataVolumes))
	for _, volume := range spec.DataVolumes {
		volume.Name = firstNonEmpty(volume.Name, "data")
		volume.SourceType = dataVolumeSourceType(volume)
		volume.MountPath = strings.TrimSpace(volume.MountPath)
		volume.DevicePath = strings.TrimSpace(volume.DevicePath)
		volume.ProjectVolumeID = strings.TrimSpace(volume.ProjectVolumeID)
		volume.ClaimName = strings.TrimSpace(volume.ClaimName)
		volume.EmptyDirMedium = strings.TrimSpace(volume.EmptyDirMedium)
		volume.EmptyDirSizeLimit = strings.TrimSpace(volume.EmptyDirSizeLimit)
		volumes = append(volumes, volume)
	}
	return volumes
}

func dataVolumeSourceType(volume ApplicationDataVolume) string {
	switch strings.TrimSpace(volume.SourceType) {
	case "projectVolume":
		return "projectVolume"
	case "emptyDir":
		return "emptyDir"
	default:
		return ""
	}
}

func applicationDataVolumeSource(volume ApplicationDataVolume, name string) corev1.Volume {
	if dataVolumeSourceType(volume) == "projectVolume" {
		return corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: strings.TrimSpace(volume.ClaimName),
				ReadOnly:  volume.ReadOnly,
			}},
		}
	}
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
}

func persistentDataVolumeName(volume ApplicationDataVolume) string {
	return dnsLabel(firstNonEmpty(volume.Name, "data"))
}
