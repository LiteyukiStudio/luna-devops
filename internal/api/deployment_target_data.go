package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const maxDeploymentDataVolumes = 32

func normalizeDataMountPath(ctx *gin.Context, value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" || !strings.HasPrefix(normalized, "/") {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "deployment volume mountPath must be absolute")
		return "", false
	}
	cleaned := path.Clean(normalized)
	if cleaned == "/" {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "deployment volume mountPath cannot be root")
		return "", false
	}
	return cleaned, true
}

func normalizeDataDevicePath(ctx *gin.Context, value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" || !strings.HasPrefix(normalized, "/") {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "deployment volume devicePath must be absolute")
		return "", false
	}
	cleaned := path.Clean(normalized)
	if cleaned == "/" {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "deployment volume devicePath cannot be root")
		return "", false
	}
	return cleaned, true
}

func normalizeDataVolumes(ctx *gin.Context, raw []deploymentTargetDataVolumeInput) ([]deploymentTargetDataVolumeInput, bool) {
	if len(raw) > maxDeploymentDataVolumes {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "deployment dataVolumes exceeds the maximum item count")
		return nil, false
	}
	seenNames := make(map[string]bool, len(raw))
	seenMountPaths := make([]string, 0, len(raw))
	seenDevicePaths := make(map[string]bool, len(raw))
	result := make([]deploymentTargetDataVolumeInput, 0, len(raw))
	for index, item := range raw {
		sourceType := strings.TrimSpace(item.SourceType)
		if sourceType != "projectVolume" && sourceType != "emptyDir" {
			writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "deployment volume sourceType must be projectVolume or emptyDir")
			return nil, false
		}

		mountPath := strings.TrimSpace(item.MountPath)
		devicePath := strings.TrimSpace(item.DevicePath)
		var ok bool
		switch sourceType {
		case "projectVolume":
			if strings.TrimSpace(item.ProjectVolumeID) == "" || (mountPath == "") == (devicePath == "") || item.EmptyDir != nil {
				writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "projectVolume requires projectVolumeId and exactly one mountPath or devicePath")
				return nil, false
			}
			if mountPath != "" {
				mountPath, ok = normalizeDataMountPath(ctx, mountPath)
			} else {
				devicePath, ok = normalizeDataDevicePath(ctx, devicePath)
			}
		case "emptyDir":
			if strings.TrimSpace(item.ProjectVolumeID) != "" || devicePath != "" || item.ReadOnly {
				writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "emptyDir cannot contain projectVolumeId, devicePath, or readOnly")
				return nil, false
			}
			mountPath, ok = normalizeDataMountPath(ctx, mountPath)
		}
		if !ok {
			return nil, false
		}

		logicalPath := mountPath
		if logicalPath == "" {
			logicalPath = devicePath
		}
		logicalName := normalizeDataVolumeName(item.LogicalName, logicalPath, index)
		if seenNames[logicalName] {
			writeErrorCode(ctx, http.StatusBadRequest, volume.CodeBindingConflict, "deployment volume logicalName conflicts")
			return nil, false
		}
		if mountPath != "" {
			for _, current := range seenMountPaths {
				if mountPath == current || strings.HasPrefix(mountPath, current+"/") || strings.HasPrefix(current, mountPath+"/") {
					writeErrorCode(ctx, http.StatusBadRequest, volume.CodeBindingConflict, "deployment volume mountPath conflicts")
					return nil, false
				}
			}
			seenMountPaths = append(seenMountPaths, mountPath)
		} else if seenDevicePaths[devicePath] {
			writeErrorCode(ctx, http.StatusBadRequest, volume.CodeBindingConflict, "deployment volume devicePath conflicts")
			return nil, false
		} else {
			seenDevicePaths[devicePath] = true
		}

		var emptyDir *deploymentTargetEmptyDirInput
		if sourceType == "emptyDir" {
			emptyDir = &deploymentTargetEmptyDirInput{}
			if item.EmptyDir != nil {
				emptyDir.Medium = normalizeEmptyDirMedium(item.EmptyDir.Medium)
				emptyDir.SizeLimit = strings.TrimSpace(item.EmptyDir.SizeLimit)
				if emptyDir.SizeLimit != "" {
					quantity, err := resource.ParseQuantity(emptyDir.SizeLimit)
					if err != nil || quantity.Sign() <= 0 {
						writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "emptyDir sizeLimit must be a positive Kubernetes quantity")
						return nil, false
					}
				}
			}
		}

		seenNames[logicalName] = true
		result = append(result, deploymentTargetDataVolumeInput{
			LogicalName: logicalName, SourceType: sourceType,
			ProjectVolumeID: strings.TrimSpace(item.ProjectVolumeID), MountPath: mountPath,
			DevicePath: devicePath, ReadOnly: item.ReadOnly, EmptyDir: emptyDir,
		})
	}
	return result, true
}

func normalizeEmptyDirMedium(value string) string {
	if strings.TrimSpace(value) == string(corev1.StorageMediumMemory) {
		return string(corev1.StorageMediumMemory)
	}
	return ""
}

func normalizeDataVolumeName(value string, logicalPath string, index int) string {
	if strings.TrimSpace(value) != "" {
		return runtimeDNSLabel(value)
	}
	base := path.Base(logicalPath)
	if base == "." || base == "/" || base == "" {
		base = fmt.Sprintf("data-%d", index+1)
	}
	return runtimeDNSLabel(base)
}

func runtimeDataPathConflicts(mountPath string, configValues ...string) bool {
	if strings.TrimSpace(mountPath) == "" {
		return false
	}
	for _, value := range configValues {
		for _, filePath := range runtimeConfigFilePaths(value) {
			if filePath == mountPath || strings.HasPrefix(filePath, mountPath+"/") || strings.HasPrefix(mountPath, filePath+"/") {
				return true
			}
		}
	}
	return false
}

func runtimeConfigFilePaths(value string) []string {
	normalized := strings.TrimSpace(value)
	if normalized == "" || normalized == "[]" || normalized == "{}" || !strings.HasPrefix(normalized, "[") {
		return nil
	}
	var raw []runtimeConfigFileInput
	if err := json.Unmarshal([]byte(normalized), &raw); err != nil {
		return nil
	}
	paths := make([]string, 0, len(raw))
	for _, item := range raw {
		filePath := strings.TrimSpace(item.Path)
		if filePath != "" && strings.HasPrefix(filePath, "/") {
			paths = append(paths, path.Clean(filePath))
		}
	}
	return paths
}
