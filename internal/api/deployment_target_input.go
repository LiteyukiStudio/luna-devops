package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	defaultBuildCPURequest     = "2"
	defaultBuildMemoryRequest  = "4Gi"
	defaultBuildTimeoutSeconds = 1800
	minBuildTimeoutSeconds     = 60
	maxBuildTimeoutSeconds     = 24 * 60 * 60
)

func (h *Handlers) deploymentTargetFromInput(ctx *gin.Context, user model.User, app model.Application, input deploymentTargetInput, targetID string, existingSecretFiles map[string]string) (model.DeploymentTarget, bool) {
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
	stage := normalizeStage(input.Stage)
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = stage
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
	servicePorts, ok := normalizeDeploymentServicePorts(ctx, input.ServicePorts, input.ServicePort)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	servicePort := servicePorts[0].Port
	replicas := input.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	runtimeCPURequest, ok := normalizeBuildResourceQuantity(ctx, input.CPURequest, "1", "运行 CPU")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	runtimeMemoryRequest, ok := normalizeBuildResourceQuantity(ctx, input.MemoryRequest, "1Gi", "运行内存")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	buildCPURequest, ok := normalizeBuildResourceQuantity(ctx, input.BuildCPURequest, defaultBuildCPURequest, "构建 CPU")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	buildMemoryRequest, ok := normalizeBuildResourceQuantity(ctx, input.BuildMemoryRequest, defaultBuildMemoryRequest, "构建内存")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	buildTimeoutSeconds, ok := normalizeBuildTimeoutSeconds(ctx, input.BuildTimeoutSeconds)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	clusterID := strings.TrimSpace(input.ClusterID)
	targetRegistryID := strings.TrimSpace(input.TargetRegistryID)
	if clusterID != "" {
		var count int64
		if err := h.db.Model(&model.RuntimeCluster{}).Where("id = ? and type in ?", clusterID, []string{"kubernetes", "k3s"}).Count(&count).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return model.DeploymentTarget{}, false
		}
		if count == 0 {
			writeError(ctx, http.StatusBadRequest, "运行集群不存在")
			return model.DeploymentTarget{}, false
		}
	}
	targetRepository, targetTag, ok = h.applyRegistryCredentialImageTemplate(ctx, user, app, sourceType, targetRegistryID, targetRepository, targetTag, model.DeploymentTarget{
		ID:    targetID,
		Name:  name,
		Stage: stage,
	})
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
	return model.DeploymentTarget{
		ID:                   targetID,
		ProjectID:            app.ProjectID,
		ApplicationID:        app.ID,
		EnvironmentID:        strings.TrimSpace(input.EnvironmentID),
		Name:                 name,
		Stage:                stage,
		ClusterID:            clusterID,
		Namespace:            strings.TrimSpace(input.Namespace),
		Replicas:             replicas,
		CPURequest:           runtimeCPURequest,
		MemoryRequest:        runtimeMemoryRequest,
		ServicePort:          servicePort,
		ServicePorts:         model.EncodeDeploymentServicePorts(servicePorts, servicePort),
		SourceType:           sourceType,
		RepositoryBindingID:  repositoryBindingID,
		DockerfilePath:       fallback(strings.TrimSpace(input.DockerfilePath), "Dockerfile"),
		BuildContext:         fallback(strings.TrimSpace(input.BuildContext), "."),
		BuildDirectory:       strings.TrimSpace(input.BuildDirectory),
		BuildEnvironmentID:   strings.TrimSpace(input.BuildEnvironmentID),
		BuildCPURequest:      buildCPURequest,
		BuildMemoryRequest:   buildMemoryRequest,
		BuildTimeoutSeconds:  buildTimeoutSeconds,
		TargetRegistryID:     targetRegistryID,
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

func normalizeBuildTimeoutSeconds(ctx *gin.Context, value int) (int, bool) {
	normalized := normalizeBuildTimeoutSecondsValue(value)
	if normalized < minBuildTimeoutSeconds || normalized > maxBuildTimeoutSeconds {
		writeError(ctx, http.StatusBadRequest, "构建超时时间必须在 1 分钟到 24 小时之间")
		return 0, false
	}
	return normalized, true
}

func normalizeBuildTimeoutSecondsValue(value int) int {
	if value <= 0 {
		return defaultBuildTimeoutSeconds
	}
	return value
}

func (h *Handlers) applyRegistryCredentialImageTemplate(ctx *gin.Context, user model.User, app model.Application, sourceType string, registryID string, repository string, tag string, target model.DeploymentTarget) (string, string, bool) {
	if sourceType != "repository" || strings.TrimSpace(registryID) == "" {
		return repository, tag, true
	}
	var project model.Project
	if err := h.db.First(&project, "id = ?", app.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "项目空间不存在")
		return repository, tag, false
	}
	var registry model.ArtifactRegistry
	if err := h.db.First(&registry, "id = ?", registryID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "目标镜像站不存在")
		return repository, tag, false
	}
	credential, ok := h.registryPushCredentialFor(user, registry)
	if !ok {
		return repository, tag, true
	}
	if strings.TrimSpace(repository) == "" || isDefaultImageRepository(registry, project, app, repository) {
		repository, _ = splitTargetImageRef(buildTargetImageRepositoryForCredential(registry, credential, project, app, target))
	}
	if strings.TrimSpace(tag) == "" || (strings.TrimSpace(tag) == "latest" && strings.TrimSpace(credential.TagTemplate) != "") {
		tag = buildTargetImageTagTemplateForCredential(credential)
	}
	return strings.Trim(strings.TrimSpace(repository), "/"), strings.TrimSpace(tag), true
}

func normalizeDeploymentServicePorts(ctx *gin.Context, input []model.DeploymentServicePort, fallbackPort int) ([]model.DeploymentServicePort, bool) {
	if len(input) == 0 {
		input = []model.DeploymentServicePort{{Name: "http", Port: fallbackInt(fallbackPort, 8080)}}
	}
	if len(input) > 16 {
		writeError(ctx, http.StatusBadRequest, "服务端口最多配置 16 个")
		return nil, false
	}
	seenNames := map[string]bool{}
	seenPorts := map[int]bool{}
	ports := make([]model.DeploymentServicePort, 0, len(input))
	for index, item := range input {
		port := item.Port
		if port <= 0 || port > 65535 {
			writeError(ctx, http.StatusBadRequest, "服务端口必须在 1 到 65535 之间")
			return nil, false
		}
		if seenPorts[port] {
			writeError(ctx, http.StatusBadRequest, "服务端口不能重复")
			return nil, false
		}
		name := normalizeDeploymentServicePortName(item.Name, port, index)
		if seenNames[name] {
			writeError(ctx, http.StatusBadRequest, "服务端口名称不能重复")
			return nil, false
		}
		seenPorts[port] = true
		seenNames[name] = true
		ports = append(ports, model.DeploymentServicePort{Name: name, Port: port})
	}
	return ports, true
}

func normalizeDeploymentServicePortName(value string, port int, index int) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' {
			builder.WriteRune(char)
		} else if char == '_' || unicode.IsSpace(char) {
			builder.WriteRune('-')
		}
	}
	name := strings.Trim(builder.String(), "-")
	if name == "" {
		if index == 0 {
			name = "http"
		} else {
			name = fmt.Sprintf("port-%d", port)
		}
	}
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}
	return name
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
