package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/resource"
)

func normalizeDataCapacity(ctx *gin.Context, value string, enabled bool) (string, bool) {
	normalized := strings.TrimSpace(value)
	if !enabled {
		return "", true
	}
	if normalized == "" {
		normalized = "1Gi"
	}
	quantity, err := resource.ParseQuantity(normalized)
	if err != nil || quantity.Sign() <= 0 {
		writeError(ctx, http.StatusBadRequest, "运行数据容量格式无效，例如 1Gi 或 10Gi")
		return "", false
	}
	return normalized, true
}

func normalizeDataMountPath(ctx *gin.Context, value string, enabled bool) (string, bool) {
	normalized := strings.TrimSpace(value)
	if !enabled {
		return "", true
	}
	if normalized == "" {
		normalized = "/data"
	}
	if !strings.HasPrefix(normalized, "/") {
		writeError(ctx, http.StatusBadRequest, "运行数据目录必须使用容器内绝对路径，例如 /data")
		return "", false
	}
	cleaned := path.Clean(normalized)
	if cleaned == "/" {
		writeError(ctx, http.StatusBadRequest, "运行数据目录不能是根目录")
		return "", false
	}
	return cleaned, true
}

func normalizeDataVolumes(ctx *gin.Context, value string, enabled bool, fallbackMountPath string, fallbackCapacity string) ([]deploymentTargetDataVolumeInput, bool) {
	if !enabled {
		return nil, true
	}
	normalized := strings.TrimSpace(value)
	if normalized == "" || normalized == "[]" {
		return []deploymentTargetDataVolumeInput{{
			Name:      "data",
			MountPath: fallback(fallbackMountPath, "/data"),
			Capacity:  fallback(fallbackCapacity, "1Gi"),
		}}, true
	}
	if !strings.HasPrefix(normalized, "[") {
		writeError(ctx, http.StatusBadRequest, "运行数据卷必须使用数组格式")
		return nil, false
	}
	var raw []deploymentTargetDataVolumeInput
	if err := json.Unmarshal([]byte(normalized), &raw); err != nil {
		writeError(ctx, http.StatusBadRequest, "运行数据卷格式无效")
		return nil, false
	}
	if len(raw) == 0 {
		writeError(ctx, http.StatusBadRequest, "启用运行数据后至少需要一个数据卷")
		return nil, false
	}
	seenNames := map[string]bool{}
	seenMountPaths := []string{}
	volumes := make([]deploymentTargetDataVolumeInput, 0, len(raw))
	for index, item := range raw {
		mountPath, ok := normalizeDataMountPath(ctx, item.MountPath, true)
		if !ok {
			return nil, false
		}
		name := normalizeDataVolumeName(item.Name, mountPath, index)
		if seenNames[name] {
			writeError(ctx, http.StatusBadRequest, "运行数据卷标识不能重复")
			return nil, false
		}
		for _, existingPath := range seenMountPaths {
			if mountPath == existingPath || strings.HasPrefix(mountPath, existingPath+"/") || strings.HasPrefix(existingPath, mountPath+"/") {
				writeError(ctx, http.StatusBadRequest, "运行数据目录不能重复或互相嵌套")
				return nil, false
			}
		}
		capacity := fallback(strings.TrimSpace(item.Capacity), "1Gi")
		if _, ok := normalizeDataCapacity(ctx, capacity, true); !ok {
			return nil, false
		}
		seenNames[name] = true
		seenMountPaths = append(seenMountPaths, mountPath)
		volumes = append(volumes, deploymentTargetDataVolumeInput{Name: name, MountPath: mountPath, Capacity: capacity})
	}
	return volumes, true
}

func normalizeDataVolumeName(value string, mountPath string, index int) string {
	if strings.TrimSpace(value) != "" {
		return runtimeDNSLabel(value)
	}
	base := path.Base(mountPath)
	if base == "." || base == "/" || base == "" {
		base = fmt.Sprintf("data-%d", index+1)
	}
	return runtimeDNSLabel(base)
}

func encodeDataVolumes(volumes []deploymentTargetDataVolumeInput) string {
	if len(volumes) == 0 {
		return ""
	}
	content, err := json.Marshal(volumes)
	if err != nil {
		return ""
	}
	return string(content)
}

func runtimeDataPathConflicts(mountPath string, configValues ...string) bool {
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
		if filePath == "" || !strings.HasPrefix(filePath, "/") {
			continue
		}
		paths = append(paths, path.Clean(filePath))
	}
	return paths
}

func (h *Handlers) syncDeploymentTargetDataVolume(ctx *gin.Context, target model.DeploymentTarget) bool {
	if !target.DataRetentionEnabled {
		return true
	}
	var project model.Project
	if err := h.db.First(&project, "id = ?", target.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project not found")
		return false
	}
	client, namespace, ok := h.kubernetesClientForDeploymentTarget(ctx, project, target, "运行集群不可用，无法同步运行数据容量")
	if !ok {
		return false
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()
	if err := client.EnsureNamespace(requestCtx, namespace, kubeprovider.ProjectNamespaceLabels(project.ID)); err != nil {
		writeError(ctx, http.StatusBadGateway, "运行数据命名空间同步失败，请检查集群权限")
		return false
	}
	if err := client.ApplyPersistentDataVolume(requestCtx, kubeprovider.ApplicationResourcesSpec{
		Name:                 deploymentTargetResourceName(target),
		Namespace:            namespace,
		ProjectID:            target.ProjectID,
		ApplicationID:        target.ApplicationID,
		EnvironmentID:        target.EnvironmentID,
		DeploymentTargetID:   target.ID,
		DataRetentionEnabled: true,
		DataCapacity:         target.DataCapacity,
		DataMountPath:        deploymentTargetDataMountPath(target),
		DataVolumes:          deploymentTargetKubernetesDataVolumes(target),
	}); err != nil {
		writeError(ctx, http.StatusBadGateway, "运行数据容量同步失败，请检查集群是否支持扩容")
		return false
	}
	return true
}

func deploymentTargetDataMountPath(target model.DeploymentTarget) string {
	return fallback(strings.TrimSpace(target.DataMountPath), "/data")
}

func deploymentTargetDataVolumes(target model.DeploymentTarget) []deploymentTargetDataVolumeInput {
	normalized := strings.TrimSpace(target.DataVolumes)
	if normalized == "" || normalized == "[]" {
		if !target.DataRetentionEnabled {
			return nil
		}
		return []deploymentTargetDataVolumeInput{{
			Name:      "data",
			MountPath: deploymentTargetDataMountPath(target),
			Capacity:  fallback(strings.TrimSpace(target.DataCapacity), "1Gi"),
		}}
	}
	var volumes []deploymentTargetDataVolumeInput
	if err := json.Unmarshal([]byte(normalized), &volumes); err != nil {
		return nil
	}
	return volumes
}

func deploymentTargetKubernetesDataVolumes(target model.DeploymentTarget) []kubeprovider.ApplicationDataVolume {
	volumes := deploymentTargetDataVolumes(target)
	output := make([]kubeprovider.ApplicationDataVolume, 0, len(volumes))
	for _, volume := range volumes {
		output = append(output, kubeprovider.ApplicationDataVolume{
			Name:      volume.Name,
			MountPath: volume.MountPath,
			Capacity:  volume.Capacity,
		})
	}
	return output
}

func deploymentTargetDataExportVolumes(target model.DeploymentTarget) []kubeprovider.DataExportVolume {
	resourceName := deploymentTargetResourceName(target)
	volumes := deploymentTargetDataVolumes(target)
	if len(volumes) == 0 && target.DataRetentionEnabled {
		volumes = []deploymentTargetDataVolumeInput{{Name: "data"}}
	}
	output := make([]kubeprovider.DataExportVolume, 0, len(volumes))
	for _, volume := range volumes {
		name := normalizeDataVolumeName(volume.Name, volume.MountPath, len(output))
		pvcName := resourceName + "-data"
		if name != "data" {
			pvcName = runtimeDNSLabel(resourceName + "-" + name + "-data")
		}
		output = append(output, kubeprovider.DataExportVolume{Name: name, PVCName: pvcName})
	}
	return output
}
