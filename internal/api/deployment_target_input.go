package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	defaultBuildCPURequest    = "1"
	defaultBuildMemoryRequest = "1Gi"
)

func (h *Handlers) deploymentTargetFromInput(ctx *gin.Context, user model.User, app model.Application, input deploymentTargetInput, targetID string, existingSecretFiles map[string]string) (model.DeploymentTarget, bool) {
	var environment model.Environment
	if err := h.db.First(&environment, "id = ? and project_id = ?", strings.TrimSpace(input.EnvironmentID), app.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "环境不存在或不属于当前项目空间")
		return model.DeploymentTarget{}, false
	}
	sourceType := normalizeDeploymentSourceType(input.SourceType)
	repositoryBindingID := strings.TrimSpace(input.RepositoryBindingID)
	if sourceType == "repository" {
		if repositoryBindingID == "" {
			writeError(ctx, http.StatusBadRequest, "代码仓库不能为空")
			return model.DeploymentTarget{}, false
		}
		var binding model.RepositoryBinding
		if err := h.db.First(&binding, "id = ? and project_id = ? and application_id = ?", repositoryBindingID, app.ProjectID, app.ID).Error; err != nil {
			writeError(ctx, http.StatusBadRequest, "代码仓库绑定不存在")
			return model.DeploymentTarget{}, false
		}
	}
	targetRepository, targetTag := splitTargetImageRef(input.TargetImageRef)
	if targetRepository == "" {
		targetRepository = strings.Trim(strings.TrimSpace(input.TargetRepository), "/")
		targetTag = strings.TrimSpace(input.TargetTag)
	}
	buildHooksEnabled := true
	if input.BuildHooksEnabled != nil {
		buildHooksEnabled = *input.BuildHooksEnabled
	}
	dataCapacity, ok := normalizeDataCapacity(ctx, input.DataCapacity, input.DataRetentionEnabled)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	dataMountPath, ok := normalizeDataMountPath(ctx, input.DataMountPath, input.DataRetentionEnabled)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	dataVolumes, ok := normalizeDataVolumes(ctx, input.DataVolumes, input.DataRetentionEnabled, dataMountPath, dataCapacity)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	if len(dataVolumes) > 0 {
		dataMountPath = dataVolumes[0].MountPath
		dataCapacity = dataVolumes[0].Capacity
	}
	servicePort := fallbackInt(input.ServicePort, 8080)
	if servicePort <= 0 || servicePort > 65535 {
		writeError(ctx, http.StatusBadRequest, "服务端口必须在 1 到 65535 之间")
		return model.DeploymentTarget{}, false
	}
	buildEnvironmentID := strings.TrimSpace(input.BuildEnvironmentID)
	if buildEnvironmentID == "" {
		buildEnvironmentID = environment.ID
	}
	var buildEnvironment model.Environment
	if err := h.db.First(&buildEnvironment, "id = ? and project_id = ?", buildEnvironmentID, app.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "构建环境不存在或不属于当前项目空间")
		return model.DeploymentTarget{}, false
	}
	buildCPURequest, ok := normalizeBuildResourceQuantity(ctx, buildEnvironment.CPURequest, defaultBuildCPURequest, "构建 CPU")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	buildMemoryRequest, ok := normalizeBuildResourceQuantity(ctx, buildEnvironment.MemoryRequest, defaultBuildMemoryRequest, "构建内存")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	runtimeConfigSetIDs := normalizeStringList(input.RuntimeConfigSetIDs)
	if len(runtimeConfigSetIDs) > 0 {
		var count int64
		if err := h.db.Model(&model.ProjectRuntimeConfigSet{}).Where("project_id = ? and id in ?", app.ProjectID, runtimeConfigSetIDs).Count(&count).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return model.DeploymentTarget{}, false
		}
		if count != int64(len(runtimeConfigSetIDs)) {
			writeError(ctx, http.StatusBadRequest, "运行配置集不存在或不属于当前项目空间")
			return model.DeploymentTarget{}, false
		}
	}
	configFiles, ok := normalizeRuntimeConfigFilesInput(ctx, input.ConfigFiles)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	secretFiles, ok := h.runtimeSecretFilesFromInput(ctx, user, targetID, input.SecretFiles, existingSecretFiles)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	secretFilesContent, err := json.Marshal(secretFiles)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.DeploymentTarget{}, false
	}
	for _, volume := range dataVolumes {
		if runtimeDataPathConflicts(volume.MountPath, configFiles, string(secretFilesContent)) {
			writeError(ctx, http.StatusBadRequest, "运行数据目录不能与配置文件或密钥文件挂载路径重叠")
			return model.DeploymentTarget{}, false
		}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = environment.Name
	}
	return model.DeploymentTarget{
		ID:                   targetID,
		ProjectID:            app.ProjectID,
		ApplicationID:        app.ID,
		EnvironmentID:        environment.ID,
		Name:                 name,
		ServicePort:          servicePort,
		SourceType:           sourceType,
		RepositoryBindingID:  repositoryBindingID,
		DockerfilePath:       fallback(strings.TrimSpace(input.DockerfilePath), "Dockerfile"),
		BuildContext:         fallback(strings.TrimSpace(input.BuildContext), "."),
		BuildDirectory:       strings.TrimSpace(input.BuildDirectory),
		BuildEnvironmentID:   buildEnvironment.ID,
		BuildCPURequest:      buildCPURequest,
		BuildMemoryRequest:   buildMemoryRequest,
		TargetRegistryID:     strings.TrimSpace(input.TargetRegistryID),
		TargetRepository:     targetRepository,
		TargetTag:            fallback(targetTag, "latest"),
		ImageRef:             strings.TrimSpace(input.ImageRef),
		BuildLabels:          strings.Join(normalizeBuildSelectorList(strings.Split(input.BuildLabels, ",")), ","),
		BuildVariableSetIDs:  encodeBuildVariableSetIDs(input.BuildVariableSetIDs),
		BuildHooksEnabled:    buildHooksEnabled,
		AutoDeploy:           input.AutoDeploy,
		BranchPattern:        strings.TrimSpace(input.BranchPattern),
		TagPattern:           strings.TrimSpace(input.TagPattern),
		ConcurrencyPolicy:    normalizeBuildConcurrencyPolicy(input.ConcurrencyPolicy),
		RuntimeConfigSetIDs:  encodeBuildVariableSetIDs(runtimeConfigSetIDs),
		EnvVars:              strings.TrimSpace(input.EnvVars),
		ConfigRefs:           strings.TrimSpace(input.ConfigRefs),
		SecretRefs:           normalizeSecretRefsInput(input.SecretRefs),
		ConfigFiles:          configFiles,
		SecretFiles:          string(secretFilesContent),
		DataRetentionEnabled: input.DataRetentionEnabled,
		DataCapacity:         dataCapacity,
		DataMountPath:        dataMountPath,
		DataVolumes:          encodeDataVolumes(dataVolumes),
		RequireApproval:      input.RequireApproval,
		Enabled:              input.Enabled,
		CreatedBy:            user.ID,
	}, true
}

func normalizeDeploymentSourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image":
		return "image"
	default:
		return "repository"
	}
}

func normalizeBuildResourceQuantity(ctx *gin.Context, value string, fallbackValue string, label string) (string, bool) {
	normalized, err := normalizeBuildResourceQuantityValue(value, fallbackValue, label)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return "", false
	}
	return normalized, true
}

func normalizeBuildResourceQuantityValue(value string, fallbackValue string, label string) (string, error) {
	normalized := fallback(strings.TrimSpace(value), fallbackValue)
	quantity, err := resource.ParseQuantity(normalized)
	if err != nil || quantity.Sign() <= 0 {
		return "", fmt.Errorf("%s必须是有效的正数资源规格", label)
	}
	return normalized, nil
}

func normalizeSecretRefsInput(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "{}" {
		return ""
	}
	return normalized
}
