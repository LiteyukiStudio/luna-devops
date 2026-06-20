package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (r *Runner) kubernetesManager(environment model.Environment) (kubeprovider.NamespaceManager, error) {
	if r.kubernetesManagerFactory != nil {
		return r.kubernetesManagerFactory(environment)
	}
	kubeconfig, err := r.kubeconfigForEnvironment(environment)
	if err != nil {
		return nil, err
	}
	manager, err := r.namespaceFactory(kubeconfig)
	if err != nil {
		return nil, runtimeClusterKubeconfigError(err)
	}
	return manager, nil
}

func (r *Runner) kubeconfigForEnvironment(environment model.Environment) (string, error) {
	cluster, err := r.runtimeClusterForEnvironment(environment)
	if err != nil {
		return "", err
	}

	kubeconfig := r.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		return "", errors.New("runtime cluster kubeconfig is missing")
	}
	return kubeconfig, nil
}

func (r *Runner) runtimeClusterForEnvironment(environment model.Environment) (model.RuntimeCluster, error) {
	var cluster model.RuntimeCluster
	if clusterID := strings.TrimSpace(environment.ClusterID); clusterID != "" {
		query, args := environmentClusterLookup(clusterID)
		err := r.db.First(&cluster, append([]any{query}, args...)...).Error
		if err != nil {
			return cluster, fmt.Errorf("runtime cluster %s not found: %w", clusterID, err)
		}
		return cluster, nil
	}
	err := r.db.Where("scope = ? and is_default = ? and type in ?", "global", true, []string{"kubernetes", "k3s"}).First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = r.db.Where("scope = ? and type in ?", "global", []string{"kubernetes", "k3s"}).Order("created_at asc").First(&cluster).Error
	}
	if err != nil {
		return cluster, fmt.Errorf("runtime cluster not found: %w", err)
	}
	return cluster, nil
}

func runtimeClusterKubeconfigError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if strings.Contains(message, "unable to read client-cert") ||
		strings.Contains(message, "unable to read client-key") ||
		strings.Contains(message, "unable to read certificate-authority") {
		return fmt.Errorf("运行集群 kubeconfig 引用了当前 Worker 无法读取的本地证书文件，请在集群页面重新保存已内联证书的 kubeconfig 后再部署: %w", err)
	}
	return fmt.Errorf("运行集群 kubeconfig 无效，无法创建 Kubernetes 客户端: %w", err)
}

func isKubernetesNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}

func environmentClusterLookup(clusterID string) (string, []any) {
	return "id = ? and type in ?", []any{strings.TrimSpace(clusterID), []string{"kubernetes", "k3s"}}
}

func projectNamespace(project model.Project) string {
	return idResourceName("ns", project.ID)
}

func deploymentNamespace(project model.Project, _ model.Environment) string {
	return projectNamespace(project)
}

func applicationResourceName(deploymentTarget model.DeploymentTarget) string {
	return idResourceName("dplt", deploymentTarget.ID)
}

func hookJobName(run model.HookRun) string {
	return idResourceName("hook", run.ID)
}

func normalizePositive(value int, fallbackValue int) int {
	if value > 0 {
		return value
	}
	return fallbackValue
}

func shortCommit(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func idResourceName(prefix string, value string) string {
	suffix := shortID(value)
	if suffix == "" {
		return dnsLabel(prefix)
	}
	return dnsLabel(prefix + "-" + suffix)
}

func shortID(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, "_"); index >= 0 {
		value = value[index+1:]
	}
	value = dnsLabelOptionalSegment(value)
	if len(value) > 10 {
		return value[:10]
	}
	return value
}

func gatewayIngressName(route model.GatewayRoute) string {
	return buildResourceName(route.ID, "liteyuki-gateway-")
}

func gatewayTLSSecretName(route model.GatewayRoute) string {
	if strings.TrimSpace(route.TLSMode) == "http-only" {
		return ""
	}
	return dnsLabel("tls-" + route.Host)
}

func gatewayIngressSpec(route model.GatewayRoute, project model.Project, application model.Application, environment model.Environment, namespace string, serviceName string) kubeprovider.GatewayIngressSpec {
	servicePort := route.ServicePort
	if servicePort <= 0 {
		servicePort = 80
	}
	return kubeprovider.GatewayIngressSpec{
		Name:               gatewayIngressName(route),
		Namespace:          namespace,
		ProjectID:          project.ID,
		ApplicationID:      application.ID,
		EnvironmentID:      environment.ID,
		DeploymentTargetID: route.DeploymentTargetID,
		RouteID:            route.ID,
		Host:               strings.TrimSpace(route.Host),
		Path:               route.Path,
		ServiceName:        firstNonEmpty(serviceName, dnsLabel(application.Slug)),
		ServicePort:        int32(servicePort),
		TLSSecretName:      gatewayTLSSecretName(route),
	}
}

func gatewayCertificateSpec(route model.GatewayRoute, project model.Project, namespace string, clusterIssuer string) kubeprovider.CertificateSpec {
	return kubeprovider.CertificateSpec{
		Name:          gatewayIngressName(route),
		Namespace:     namespace,
		ProjectID:     project.ID,
		RouteID:       route.ID,
		Host:          strings.TrimSpace(route.Host),
		SecretName:    gatewayTLSSecretName(route),
		ClusterIssuer: strings.TrimSpace(clusterIssuer),
	}
}

func applicationResourcesSpec(release model.Release, project model.Project, application model.Application, environment model.Environment, deploymentTarget model.DeploymentTarget, runtimeConfigSets []model.ProjectRuntimeConfigSet, namespace string, rolloutTimeoutSeconds int64) (kubeprovider.ApplicationResourcesSpec, error) {
	configValues := make([]string, 0, len(runtimeConfigSets)+4)
	secretValues := make([]string, 0, len(runtimeConfigSets)+2)
	configFileValues := make([]string, 0, len(runtimeConfigSets)+1)
	secretFileValues := make([]string, 0, len(runtimeConfigSets)+1)
	for _, set := range runtimeConfigSets {
		configValues = append(configValues, set.EnvVars)
		secretValues = append(secretValues, set.SecretRefs)
		configFileValues = append(configFileValues, set.ConfigFiles)
		secretFileValues = append(secretFileValues, set.SecretFiles)
	}
	configValues = append(configValues, environment.EnvVars, environment.ConfigRefs, deploymentTarget.EnvVars, deploymentTarget.ConfigRefs)
	secretValues = append(secretValues, environment.SecretRefs, deploymentTarget.SecretRefs)
	configFileValues = append(configFileValues, deploymentTarget.ConfigFiles)
	secretFileValues = append(secretFileValues, deploymentTarget.SecretFiles)
	configData, err := mergeKeyValueMaps(configValues...)
	if err != nil {
		return kubeprovider.ApplicationResourcesSpec{}, err
	}
	secretData, err := mergeKeyValueMaps(secretValues...)
	if err != nil {
		return kubeprovider.ApplicationResourcesSpec{}, err
	}
	configFiles, err := mergeRuntimeConfigFiles(configFileValues...)
	if err != nil {
		return kubeprovider.ApplicationResourcesSpec{}, err
	}
	secretFiles, err := mergeRuntimeConfigFiles(secretFileValues...)
	if err != nil {
		return kubeprovider.ApplicationResourcesSpec{}, err
	}
	servicePort := deploymentTarget.ServicePort
	if servicePort <= 0 {
		servicePort = 8080
	}
	replicas := environment.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	return kubeprovider.ApplicationResourcesSpec{
		Name:                  applicationResourceName(deploymentTarget),
		Namespace:             namespace,
		ProjectID:             project.ID,
		ApplicationID:         application.ID,
		EnvironmentID:         environment.ID,
		DeploymentTargetID:    deploymentTarget.ID,
		ReleaseID:             release.ID,
		BuildRunID:            release.BuildRunID,
		Image:                 strings.TrimSpace(release.ImageRef),
		Replicas:              int32(replicas),
		ServicePort:           int32(servicePort),
		CPURequest:            strings.TrimSpace(environment.CPURequest),
		MemoryRequest:         strings.TrimSpace(environment.MemoryRequest),
		RolloutTimeoutSeconds: int32(rolloutTimeoutSeconds),
		ConfigData:            configData,
		SecretData:            secretData,
		ConfigFiles:           configFiles,
		SecretFiles:           secretFiles,
		DataRetentionEnabled:  deploymentTarget.DataRetentionEnabled,
		DataCapacity:          deploymentTarget.DataCapacity,
		DataMountPath:         deploymentTarget.DataMountPath,
		DataVolumes:           deploymentTargetDataVolumes(deploymentTarget),
	}, nil
}

func deploymentTargetDataVolumes(target model.DeploymentTarget) []kubeprovider.ApplicationDataVolume {
	normalized := strings.TrimSpace(target.DataVolumes)
	if normalized == "" || normalized == "[]" {
		if !target.DataRetentionEnabled {
			return nil
		}
		return []kubeprovider.ApplicationDataVolume{{
			Name:      "data",
			MountPath: firstNonEmpty(target.DataMountPath, "/data"),
			Capacity:  firstNonEmpty(target.DataCapacity, "1Gi"),
		}}
	}
	var volumes []kubeprovider.ApplicationDataVolume
	if err := json.Unmarshal([]byte(normalized), &volumes); err != nil {
		return nil
	}
	return volumes
}

func mergeKeyValueMaps(values ...string) (map[string]string, error) {
	merged := map[string]string{}
	for _, value := range values {
		parsed, err := parseKeyValueMap(value)
		if err != nil {
			return nil, err
		}
		for key, item := range parsed {
			merged[key] = item
		}
	}
	return merged, nil
}

type runtimeConfigFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func mergeRuntimeConfigFiles(values ...string) ([]kubeprovider.ApplicationConfigFile, error) {
	merged := map[string]kubeprovider.ApplicationConfigFile{}
	order := []string{}
	for _, value := range values {
		files, err := parseRuntimeConfigFiles(value)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if _, ok := merged[file.Path]; !ok {
				order = append(order, file.Path)
			}
			merged[file.Path] = file
		}
	}
	output := make([]kubeprovider.ApplicationConfigFile, 0, len(order))
	for index, itemPath := range order {
		file := merged[itemPath]
		file.Key = runtimeConfigFileKey(index, file.Path)
		output = append(output, file)
	}
	return output, nil
}

func parseRuntimeConfigFiles(value string) ([]kubeprovider.ApplicationConfigFile, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "[]" {
		return nil, nil
	}
	if !strings.HasPrefix(value, "[") {
		return nil, fmt.Errorf("runtime config files must be an array")
	}
	var raw []runtimeConfigFileInput
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, err
	}
	files := make([]kubeprovider.ApplicationConfigFile, 0, len(raw))
	seenPaths := map[string]bool{}
	for _, item := range raw {
		filePath, err := normalizeRuntimeConfigFilePath(item.Path)
		if err != nil {
			return nil, err
		}
		if seenPaths[filePath] {
			return nil, fmt.Errorf("runtime config file path %q is duplicated", filePath)
		}
		seenPaths[filePath] = true
		files = append(files, kubeprovider.ApplicationConfigFile{Path: filePath, Content: item.Content})
	}
	return files, nil
}

func normalizeRuntimeConfigFilePath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("runtime config file path must be absolute")
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "/" || strings.Contains(cleaned, "/../") || strings.HasSuffix(cleaned, "/..") {
		return "", fmt.Errorf("runtime config file path is invalid")
	}
	return cleaned, nil
}

func runtimeConfigFileKey(index int, filePath string) string {
	name := strings.Trim(path.Base(filePath), ". ")
	if name == "" || name == "/" {
		name = "file"
	}
	var builder strings.Builder
	for _, char := range strings.ToLower(name) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	key := strings.Trim(builder.String(), "-.")
	if key == "" {
		key = "file"
	}
	return fmt.Sprintf("%02d-%s", index+1, key)
}

func runtimeConfigSetIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err == nil {
		return compactStringList(ids)
	}
	return compactStringList(strings.Split(raw, ","))
}

func compactStringList(values []string) []string {
	seen := map[string]bool{}
	output := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		output = append(output, item)
	}
	return output
}

func parseKeyValueMap(value string) (map[string]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return map[string]string{}, nil
	}
	if strings.HasPrefix(value, "{") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(value), &raw); err != nil {
			return nil, err
		}
		parsed := make(map[string]string, len(raw))
		for key, item := range raw {
			parsed[strings.TrimSpace(key)] = fmt.Sprint(item)
		}
		return compactKeyValueMap(parsed), nil
	}
	parsed := map[string]string{}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, item, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid key-value line %q", line)
		}
		parsed[strings.TrimSpace(key)] = strings.TrimSpace(item)
	}
	return compactKeyValueMap(parsed), nil
}

func compactKeyValueMap(values map[string]string) map[string]string {
	compacted := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		compacted[key] = value
	}
	return compacted
}

func buildResourceName(buildRunID, prefix string) string {
	id := strings.ToLower(strings.TrimSpace(buildRunID))
	id = strings.TrimPrefix(id, "bldr_")
	var builder strings.Builder
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('-')
	}
	suffix := strings.Trim(builder.String(), "-")
	if suffix == "" {
		suffix = "run"
	}
	maxSuffix := 63 - len(prefix)
	if maxSuffix < 1 {
		maxSuffix = 1
	}
	if len(suffix) > maxSuffix {
		suffix = suffix[:maxSuffix]
	}
	return prefix + suffix
}

func dnsLabel(value string) string {
	label := dnsLabelOptionalSegment(value)
	if len(label) > 63 {
		label = strings.TrimRight(label[:63], "-")
	}
	if label == "" {
		return "liteyuki"
	}
	return label
}

func dnsLabelOptionalSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	previousDash := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			previousDash = false
			continue
		}
		if !previousDash {
			builder.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
