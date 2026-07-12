package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
)

type deploymentKubernetesAdvancedInput struct {
	ImagePullPolicy              string
	ContainerCommand             string
	ContainerArgs                string
	Lifecycle                    string
	InitContainers               string
	SidecarContainers            string
	ReadinessProbe               string
	LivenessProbe                string
	StartupProbe                 string
	RunAsUser                    string
	RunAsGroup                   string
	FSGroup                      string
	FSGroupChangePolicy          string
	ReadOnlyRootFilesystem       bool
	AllowPrivilegeEscalation     string
	CapabilityAdd                string
	CapabilityDrop               string
	NodeSelector                 string
	Tolerations                  string
	Affinity                     string
	TopologySpreadConstraints    string
	PriorityClassName            string
	ServiceType                  string
	ServiceAnnotations           string
	ServiceExternalTrafficPolicy string
	ServiceSessionAffinity       string
	DataStorageClassName         string
	DataAccessMode               string
	DataVolumeMode               string
}

func normalizeDeploymentKubernetesAdvanced(ctx *gin.Context, input deploymentTargetInput) (deploymentKubernetesAdvancedInput, bool) {
	lifecycle, ok := normalizeLifecycleJSON(ctx, input.Lifecycle)
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	initContainers, ok := normalizeAuxContainersJSON(ctx, input.InitContainers, "初始化容器")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	sidecarContainers, ok := normalizeAuxContainersJSON(ctx, input.SidecarContainers, "Sidecar 容器")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	readinessProbe, ok := normalizeProbeJSON(ctx, input.ReadinessProbe, "就绪探针")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	livenessProbe, ok := normalizeProbeJSON(ctx, input.LivenessProbe, "存活探针")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	startupProbe, ok := normalizeProbeJSON(ctx, input.StartupProbe, "启动探针")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	runAsUser, ok := normalizeOptionalNonNegativeInteger(ctx, input.RunAsUser, "运行用户 UID")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	runAsGroup, ok := normalizeOptionalNonNegativeInteger(ctx, input.RunAsGroup, "运行用户组 GID")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	fsGroup, ok := normalizeOptionalNonNegativeInteger(ctx, input.FSGroup, "文件系统组 GID")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	tolerations, ok := normalizeTolerationsJSON(ctx, input.Tolerations)
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	affinity, ok := normalizeAffinityJSON(ctx, input.Affinity)
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	topologySpreadConstraints, ok := normalizeTopologySpreadConstraintsJSON(ctx, input.TopologySpreadConstraints)
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	nodeSelector, ok := normalizeMapJSONOrLines(ctx, input.NodeSelector, "节点选择器")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	serviceAnnotations, ok := normalizeMapJSONOrLines(ctx, input.ServiceAnnotations, "Service 注解")
	if !ok {
		return deploymentKubernetesAdvancedInput{}, false
	}
	return deploymentKubernetesAdvancedInput{
		ImagePullPolicy:              normalizeImagePullPolicyValue(input.ImagePullPolicy),
		ContainerCommand:             normalizeStringArrayText(input.ContainerCommand),
		ContainerArgs:                normalizeStringArrayText(input.ContainerArgs),
		Lifecycle:                    lifecycle,
		InitContainers:               initContainers,
		SidecarContainers:            sidecarContainers,
		ReadinessProbe:               readinessProbe,
		LivenessProbe:                livenessProbe,
		StartupProbe:                 startupProbe,
		RunAsUser:                    runAsUser,
		RunAsGroup:                   runAsGroup,
		FSGroup:                      fsGroup,
		FSGroupChangePolicy:          normalizeFSGroupChangePolicy(input.FSGroupChangePolicy),
		ReadOnlyRootFilesystem:       input.ReadOnlyRootFilesystem,
		AllowPrivilegeEscalation:     normalizeTriStateBool(input.AllowPrivilegeEscalation),
		CapabilityAdd:                normalizeStringArrayText(input.CapabilityAdd),
		CapabilityDrop:               normalizeStringArrayText(input.CapabilityDrop),
		NodeSelector:                 nodeSelector,
		Tolerations:                  tolerations,
		Affinity:                     affinity,
		TopologySpreadConstraints:    topologySpreadConstraints,
		PriorityClassName:            strings.TrimSpace(input.PriorityClassName),
		ServiceType:                  normalizeServiceType(input.ServiceType),
		ServiceAnnotations:           serviceAnnotations,
		ServiceExternalTrafficPolicy: normalizeServiceExternalTrafficPolicy(input.ServiceExternalTrafficPolicy),
		ServiceSessionAffinity:       normalizeServiceSessionAffinity(input.ServiceSessionAffinity),
		DataStorageClassName:         strings.TrimSpace(input.DataStorageClassName),
		DataAccessMode:               normalizePersistentVolumeAccessMode(input.DataAccessMode),
		DataVolumeMode:               normalizePersistentVolumeMode(input.DataVolumeMode),
	}, true
}

type deploymentAutoScalingInput struct {
	Enabled       bool
	MinReplicas   int
	MaxReplicas   int
	CPUPercent    int
	MemoryPercent int
	Behavior      string
}

func normalizeDeploymentAutoScaling(ctx *gin.Context, input deploymentTargetInput, replicas int) (deploymentAutoScalingInput, bool) {
	if !input.AutoScalingEnabled {
		return deploymentAutoScalingInput{MinReplicas: 1, MaxReplicas: fallbackInt(replicas, 1)}, true
	}
	minReplicas := input.AutoScalingMinReplicas
	if minReplicas <= 0 {
		minReplicas = fallbackInt(replicas, 1)
	}
	maxReplicas := input.AutoScalingMaxReplicas
	if maxReplicas <= 0 {
		maxReplicas = minReplicas
	}
	if maxReplicas < minReplicas {
		writeError(ctx, http.StatusBadRequest, "自动伸缩最大副本数不能小于最小副本数")
		return deploymentAutoScalingInput{}, false
	}
	cpuPercent := input.AutoScalingCPUPercent
	memoryPercent := input.AutoScalingMemoryPercent
	if cpuPercent < 0 || cpuPercent > 1000 || memoryPercent < 0 || memoryPercent > 1000 {
		writeError(ctx, http.StatusBadRequest, "自动伸缩目标利用率必须在 1 到 1000 之间")
		return deploymentAutoScalingInput{}, false
	}
	if cpuPercent == 0 && memoryPercent == 0 {
		writeError(ctx, http.StatusBadRequest, "启用自动伸缩后至少需要配置 CPU 或内存目标利用率")
		return deploymentAutoScalingInput{}, false
	}
	behavior, ok := normalizeHPABehaviorJSON(ctx, input.AutoScalingBehavior)
	if !ok {
		return deploymentAutoScalingInput{}, false
	}
	return deploymentAutoScalingInput{
		Enabled:       true,
		MinReplicas:   minReplicas,
		MaxReplicas:   maxReplicas,
		CPUPercent:    cpuPercent,
		MemoryPercent: memoryPercent,
		Behavior:      behavior,
	}, true
}

func normalizeWorkloadType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "statefulset", "stateful-set":
		return "StatefulSet"
	default:
		return "Deployment"
	}
}

func normalizeImagePullPolicyValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "always":
		return "Always"
	case "never":
		return "Never"
	case "ifnotpresent", "if-not-present":
		return "IfNotPresent"
	default:
		return ""
	}
}

func normalizeFSGroupChangePolicy(value string) string {
	switch strings.TrimSpace(value) {
	case string(corev1.FSGroupChangeOnRootMismatch):
		return string(corev1.FSGroupChangeOnRootMismatch)
	case string(corev1.FSGroupChangeAlways):
		return string(corev1.FSGroupChangeAlways)
	default:
		return ""
	}
}

func normalizeTriStateBool(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return "true"
	case "false":
		return "false"
	default:
		return ""
	}
}

func normalizeServiceType(value string) string {
	switch strings.TrimSpace(value) {
	case string(corev1.ServiceTypeNodePort):
		return string(corev1.ServiceTypeNodePort)
	case string(corev1.ServiceTypeLoadBalancer):
		return string(corev1.ServiceTypeLoadBalancer)
	case string(corev1.ServiceTypeClusterIP):
		return string(corev1.ServiceTypeClusterIP)
	default:
		return ""
	}
}

func normalizeServiceExternalTrafficPolicy(value string) string {
	switch strings.TrimSpace(value) {
	case string(corev1.ServiceExternalTrafficPolicyLocal):
		return string(corev1.ServiceExternalTrafficPolicyLocal)
	case string(corev1.ServiceExternalTrafficPolicyCluster):
		return string(corev1.ServiceExternalTrafficPolicyCluster)
	default:
		return ""
	}
}

func normalizeServiceSessionAffinity(value string) string {
	switch strings.TrimSpace(value) {
	case string(corev1.ServiceAffinityClientIP):
		return string(corev1.ServiceAffinityClientIP)
	case string(corev1.ServiceAffinityNone):
		return string(corev1.ServiceAffinityNone)
	default:
		return ""
	}
}

func normalizePersistentVolumeAccessMode(value string) string {
	switch strings.TrimSpace(value) {
	case string(corev1.ReadWriteMany):
		return string(corev1.ReadWriteMany)
	case string(corev1.ReadOnlyMany):
		return string(corev1.ReadOnlyMany)
	case string(corev1.ReadWriteOnce):
		return string(corev1.ReadWriteOnce)
	default:
		return ""
	}
}

func normalizePersistentVolumeMode(value string) string {
	switch strings.TrimSpace(value) {
	case string(corev1.PersistentVolumeBlock):
		return string(corev1.PersistentVolumeBlock)
	case string(corev1.PersistentVolumeFilesystem):
		return string(corev1.PersistentVolumeFilesystem)
	default:
		return ""
	}
}

func normalizeOptionalNonNegativeInteger(ctx *gin.Context, value string, label string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	parsed, err := strconv.ParseInt(normalized, 10, 64)
	if err != nil || parsed < 0 {
		writeError(ctx, http.StatusBadRequest, label+"必须是非负整数")
		return "", false
	}
	return strconv.FormatInt(parsed, 10), true
}

func normalizeProbeJSON(ctx *gin.Context, value string, label string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	var probe corev1.Probe
	if err := json.Unmarshal([]byte(normalized), &probe); err != nil {
		writeError(ctx, http.StatusBadRequest, label+"必须是合法的 Kubernetes Probe JSON")
		return "", false
	}
	return normalized, true
}

func normalizeLifecycleJSON(ctx *gin.Context, value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	var lifecycle corev1.Lifecycle
	if err := json.Unmarshal([]byte(normalized), &lifecycle); err != nil {
		writeError(ctx, http.StatusBadRequest, "生命周期钩子必须是合法的 Kubernetes Lifecycle JSON")
		return "", false
	}
	return normalized, true
}

func normalizeHPABehaviorJSON(ctx *gin.Context, value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	var behavior autoscalingv2.HorizontalPodAutoscalerBehavior
	if err := json.Unmarshal([]byte(normalized), &behavior); err != nil {
		writeError(ctx, http.StatusBadRequest, "HPA 行为必须是合法的 Kubernetes HorizontalPodAutoscalerBehavior JSON")
		return "", false
	}
	return normalized, true
}

func normalizeAuxContainersJSON(ctx *gin.Context, value string, label string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	var containers []corev1.Container
	if err := json.Unmarshal([]byte(normalized), &containers); err != nil {
		writeError(ctx, http.StatusBadRequest, label+"必须是合法的 Kubernetes Container JSON 数组")
		return "", false
	}
	if len(containers) > 8 {
		writeError(ctx, http.StatusBadRequest, label+"最多配置 8 个")
		return "", false
	}
	for _, container := range containers {
		if strings.TrimSpace(container.Name) == "" || strings.TrimSpace(container.Image) == "" {
			writeError(ctx, http.StatusBadRequest, label+"必须填写 name 和 image")
			return "", false
		}
		if container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
			writeError(ctx, http.StatusBadRequest, label+"不允许启用 privileged")
			return "", false
		}
	}
	return normalized, true
}

func normalizeTolerationsJSON(ctx *gin.Context, value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	var tolerations []corev1.Toleration
	if err := json.Unmarshal([]byte(normalized), &tolerations); err != nil {
		writeError(ctx, http.StatusBadRequest, "Tolerations 必须是合法的 Kubernetes Toleration JSON 数组")
		return "", false
	}
	return normalized, true
}

func normalizeAffinityJSON(ctx *gin.Context, value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	var affinity corev1.Affinity
	if err := json.Unmarshal([]byte(normalized), &affinity); err != nil {
		writeError(ctx, http.StatusBadRequest, "Affinity 必须是合法的 Kubernetes Affinity JSON")
		return "", false
	}
	return normalized, true
}

func normalizeTopologySpreadConstraintsJSON(ctx *gin.Context, value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	var constraints []corev1.TopologySpreadConstraint
	if err := json.Unmarshal([]byte(normalized), &constraints); err != nil {
		writeError(ctx, http.StatusBadRequest, "拓扑分布约束必须是合法的 Kubernetes TopologySpreadConstraint JSON 数组")
		return "", false
	}
	return normalized, true
}

func normalizeMapJSONOrLines(ctx *gin.Context, value string, label string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", true
	}
	if strings.HasPrefix(normalized, "{") {
		var values map[string]string
		if err := json.Unmarshal([]byte(normalized), &values); err != nil {
			writeError(ctx, http.StatusBadRequest, label+"必须是合法的 JSON 对象或 KEY=VALUE 多行文本")
			return "", false
		}
		return normalized, true
	}
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "=") {
			writeError(ctx, http.StatusBadRequest, label+"必须是合法的 JSON 对象或 KEY=VALUE 多行文本")
			return "", false
		}
	}
	return normalized, true
}

func normalizeStringArrayText(value string) string {
	return strings.TrimSpace(value)
}

func normalizeSecretRefsInput(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "{}" {
		return ""
	}
	return normalized
}
