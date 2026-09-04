package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourcename"
	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"github.com/LiteyukiStudio/devops/internal/variables"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (r *Runner) kubernetesManager(ctx context.Context, target model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
	if r.kubernetesManagerFactory != nil {
		return r.kubernetesManagerFactory(target)
	}
	kubeconfig, err := r.kubeconfigForDeploymentTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	manager, err := r.namespaceFactory(kubeconfig)
	if err != nil {
		return nil, runtimeClusterKubeconfigError(err)
	}
	return manager, nil
}

func applyRuntimeClusterResourcePolicy(spec *kubeprovider.ApplicationResourcesSpec, cluster model.RuntimeCluster) error {
	policy := resourcepolicy.Policy{
		CPURequestPercent: cluster.CPURequestPercent, MemoryRequestPercent: cluster.MemoryRequestPercent,
		CPULimitPercent: cluster.CPULimitPercent, MemoryLimitPercent: cluster.MemoryLimitPercent,
	}
	effective, err := resourcepolicy.Calculate(spec.CPURequest, spec.MemoryRequest, policy)
	if err != nil {
		return fmt.Errorf("runtime.resource_policy_render_failed: cluster %s deployment target %s: %w", cluster.ID, spec.DeploymentTargetID, err)
	}
	spec.CPURequest = effective.CPURequest
	spec.MemoryRequest = effective.MemoryRequest
	spec.CPULimit = effective.CPULimit
	spec.MemoryLimit = effective.MemoryLimit
	return nil
}

func (r *Runner) kubeconfigForDeploymentTarget(ctx context.Context, target model.DeploymentTarget) (string, error) {
	return r.kubeconfigForRuntimeClusterID(ctx, target.ClusterID)
}

func (r *Runner) kubeconfigForRuntimeClusterID(ctx context.Context, clusterID string) (string, error) {
	cluster, err := r.runtimeClusterForClusterID(ctx, clusterID)
	if err != nil {
		return "", err
	}

	kubeconfig := r.secrets.ResolveContext(ctx, cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		return "", errors.New("runtime cluster kubeconfig is missing")
	}
	return kubeconfig, nil
}

func (r *Runner) runtimeClusterForDeploymentTarget(ctx context.Context, target model.DeploymentTarget) (model.RuntimeCluster, error) {
	return r.runtimeClusterForClusterID(ctx, target.ClusterID)
}

func (r *Runner) runtimeClusterForClusterID(ctx context.Context, clusterID string) (model.RuntimeCluster, error) {
	var cluster model.RuntimeCluster
	db := r.db.WithContext(ctx)
	if clusterID = strings.TrimSpace(clusterID); clusterID != "" {
		query, args := runtimeClusterLookup(clusterID)
		err := runtimecluster.ActiveScope(db).First(&cluster, append([]any{query}, args...)...).Error
		if err != nil {
			return cluster, fmt.Errorf("runtime cluster %s not found: %w", clusterID, err)
		}
		return cluster, nil
	}
	err := runtimecluster.ActiveScope(db).Where("scope = ? and is_default = ?", "global", true).First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = runtimecluster.ActiveScope(db).Where("scope = ?", "global").Order("created_at asc").First(&cluster).Error
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

func runtimeClusterLookup(clusterID string) (string, []any) {
	return "id = ?", []any{strings.TrimSpace(clusterID)}
}

func projectNamespace(project model.Project) string {
	return strings.TrimSpace(project.KubernetesNamespace)
}

func deploymentNamespace(project model.Project) string {
	return projectNamespace(project)
}

func applicationResourceName(deploymentTarget model.DeploymentTarget) string {
	return strings.TrimSpace(deploymentTarget.KubernetesName)
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
	return resourcename.FromID(prefix, value)
}

func shortID(value string) string {
	return resourcename.ShortID(value)
}

func gatewayRuntimeName(route model.GatewayRoute) string {
	return resourcename.GatewayRoute(route.ID)
}

func gatewayTLSSecretName(route model.GatewayRoute) string {
	if strings.TrimSpace(route.TLSMode) == "http-only" {
		return ""
	}
	return dnsLabel("tls-" + route.Host)
}

func gatewaySpec(cluster model.RuntimeCluster, projectID string) kubeprovider.GatewaySpec {
	return kubeprovider.GatewaySpec{
		Name:               firstNonEmpty(cluster.GatewayName, "luna-gateway"),
		Namespace:          firstNonEmpty(cluster.GatewayNamespace, "kube-system"),
		GatewayClassName:   firstNonEmpty(cluster.GatewayClassName, "traefik"),
		ExternalTLSMode:    firstNonEmpty(cluster.GatewayExternalTLSMode, "none"),
		HTTPListenerName:   firstNonEmpty(cluster.GatewayHTTPListenerName, "web"),
		HTTPListenerPort:   int32(normalizePositive(cluster.GatewayHTTPListenerPort, 8080)),
		HTTPSListenerName:  firstNonEmpty(cluster.GatewayHTTPSListenerName, "websecure"),
		HTTPSListenerPort:  int32(normalizePositive(cluster.GatewayHTTPSListenerPort, 8443)),
		TLSSecretName:      strings.TrimSpace(cluster.GatewayTLSSecretName),
		TLSSecretNamespace: strings.TrimSpace(cluster.GatewayTLSSecretNamespace),
		ProjectID:          projectID,
	}
}

func gatewayCertificateNamespace(cluster model.RuntimeCluster, fallbackNamespace string) string {
	return firstNonEmpty(cluster.GatewayCertificateNamespace, cluster.GatewayNamespace, fallbackNamespace)
}

func gatewayCertificateIssuerKind(cluster model.RuntimeCluster) string {
	if strings.EqualFold(strings.TrimSpace(cluster.GatewayCertIssuerKind), "Issuer") {
		return "Issuer"
	}
	return "ClusterIssuer"
}

func gatewayCertificateIssuerName(cluster model.RuntimeCluster, fallbackIssuer string) string {
	return firstNonEmpty(cluster.GatewayCertIssuerName, fallbackIssuer)
}

func gatewayWildcardCertificateDomain(cluster model.RuntimeCluster) string {
	return firstNonEmpty(cluster.GatewayWildcardCertDomain, runtimecluster.DecodeGatewayDomainSuffixes(cluster.GatewayDomainSuffixesRaw)[0])
}

func gatewayWildcardCertificateSecretName(cluster model.RuntimeCluster) string {
	if name := strings.TrimSpace(cluster.GatewayWildcardCertSecretName); name != "" {
		return name
	}
	domain := strings.TrimPrefix(gatewayWildcardCertificateDomain(cluster), "*.")
	if domain == "" {
		return ""
	}
	return dnsLabel("wildcard-" + domain)
}

func gatewayWildcardCertificateSpec(cluster model.RuntimeCluster, project model.Project, namespace string, issuerName string) (kubeprovider.CertificateSpec, bool) {
	if !cluster.GatewayWildcardCertEnabled {
		return kubeprovider.CertificateSpec{}, false
	}
	domain := gatewayWildcardCertificateDomain(cluster)
	secretName := gatewayWildcardCertificateSecretName(cluster)
	if domain == "" || secretName == "" {
		return kubeprovider.CertificateSpec{}, false
	}
	domain = strings.TrimPrefix(domain, "*.")
	return kubeprovider.CertificateSpec{
		Name:          secretName,
		Namespace:     gatewayCertificateNamespace(cluster, namespace),
		ProjectID:     project.ID,
		Host:          domain,
		DNSNames:      []string{"*." + domain},
		SecretName:    secretName,
		IssuerKind:    gatewayCertificateIssuerKind(cluster),
		ClusterIssuer: issuerName,
	}, true
}

func httpRouteSpec(route model.GatewayRoute, project model.Project, application model.Application, cluster model.RuntimeCluster, namespace string, serviceName string) (kubeprovider.HTTPRouteSpec, error) {
	servicePort := route.ServicePort
	if servicePort <= 0 {
		servicePort = 80
	}
	requestHeaders, err := mergeKeyValueMaps(cluster.GatewayDefaultRequestHeaders, route.RequestHeaders)
	if err != nil {
		return kubeprovider.HTTPRouteSpec{}, err
	}
	responseHeaders, err := mergeKeyValueMaps(cluster.GatewayDefaultResponseHeaders, route.ResponseHeaders)
	if err != nil {
		return kubeprovider.HTTPRouteSpec{}, err
	}
	for key, value := range forwardedHeaderOverrides(cluster) {
		requestHeaders[key] = value
	}
	return kubeprovider.HTTPRouteSpec{
		Name:                   gatewayRuntimeName(route),
		Namespace:              namespace,
		ProjectID:              project.ID,
		ApplicationID:          application.ID,
		DeploymentTargetID:     route.DeploymentTargetID,
		RouteID:                route.ID,
		Host:                   strings.TrimSpace(route.Host),
		Path:                   route.Path,
		PathMatchType:          firstNonEmpty(route.PathMatchType, "PathPrefix"),
		ParentGatewayName:      firstNonEmpty(route.ParentGatewayName, cluster.GatewayName, "luna-gateway"),
		ParentGatewayNamespace: firstNonEmpty(route.ParentGatewayNamespace, cluster.GatewayNamespace, "kube-system"),
		SectionName:            gatewayRouteSectionName(route, cluster),
		ServiceName:            firstNonEmpty(serviceName, dnsLabel(application.Identifier)),
		ServicePort:            int32(servicePort),
		BackendWeight:          int32(normalizePositive(route.BackendWeight, 1)),
		RequestHeaders:         requestHeaders,
		ResponseHeaders:        responseHeaders,
		URLRewrite:             route.URLRewrite,
		RequestRedirect:        route.RequestRedirect,
	}, nil
}

func gatewayRouteSectionName(route model.GatewayRoute, cluster model.RuntimeCluster) string {
	if sectionName := strings.TrimSpace(route.SectionName); sectionName != "" {
		return sectionName
	}
	if strings.TrimSpace(cluster.GatewayExternalTLSMode) == "gateway" {
		return firstNonEmpty(cluster.GatewayHTTPSListenerName, "websecure")
	}
	return firstNonEmpty(cluster.GatewayHTTPListenerName, "web")
}

func forwardedHeaderOverrides(cluster model.RuntimeCluster) map[string]string {
	if strings.TrimSpace(cluster.GatewayExternalTLSMode) != "upstream" || strings.TrimSpace(cluster.GatewayForwardedHeadersMode) != "overwrite" {
		return map[string]string{}
	}
	return map[string]string{
		"X-Forwarded-Proto": "https",
		"X-Forwarded-Port":  "443",
	}
}

func gatewayCertificateSpec(route model.GatewayRoute, project model.Project, namespace string, issuerKind string, issuerName string) kubeprovider.CertificateSpec {
	return kubeprovider.CertificateSpec{
		Name:          gatewayRuntimeName(route),
		Namespace:     namespace,
		ProjectID:     project.ID,
		RouteID:       route.ID,
		Host:          strings.TrimSpace(route.Host),
		SecretName:    gatewayTLSSecretName(route),
		IssuerKind:    strings.TrimSpace(issuerKind),
		ClusterIssuer: strings.TrimSpace(issuerName),
	}
}

func applicationResourcesSpec(release model.Release, project model.Project, application model.Application, deploymentTarget model.DeploymentTarget, runtimeConfigSets []model.ProjectRuntimeConfigSet, dataVolumes []kubeprovider.ApplicationDataVolume, namespace string, rolloutTimeoutSeconds int64) (kubeprovider.ApplicationResourcesSpec, error) {
	configValues := make([]string, 0, len(runtimeConfigSets)+2)
	secretValues := make([]string, 0, len(runtimeConfigSets)+2)
	configFileValues := make([]string, 0, len(runtimeConfigSets)+1)
	secretFileValues := make([]string, 0, len(runtimeConfigSets)+1)
	for _, set := range runtimeConfigSets {
		configValues = append(configValues, set.EnvVars)
		secretValues = append(secretValues, set.SecretRefs)
		configFileValues = append(configFileValues, set.ConfigFiles)
		secretFileValues = append(secretFileValues, set.SecretFiles)
	}
	configValues = append(configValues, deploymentTarget.EnvVars)
	secretValues = append(secretValues, deploymentTarget.SecretRefs)
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
	// A key must have one authoritative value mode in the rendered workload.
	// Secret wins across configuration layers so plaintext can never shadow an
	// already configured secret through ConfigMap precedence.
	for key := range secretData {
		delete(configData, key)
	}
	expandEnvRefsCrossBoundary(configData, secretData)
	configFiles, err := mergeRuntimeConfigFiles(configFileValues...)
	if err != nil {
		return kubeprovider.ApplicationResourcesSpec{}, err
	}
	secretFiles, err := mergeRuntimeConfigFiles(secretFileValues...)
	if err != nil {
		return kubeprovider.ApplicationResourcesSpec{}, err
	}
	configuredServicePorts := model.DeploymentTargetServicePorts(deploymentTarget)
	if len(configuredServicePorts) == 0 {
		return kubeprovider.ApplicationResourcesSpec{}, errors.New("deployment service ports are required")
	}
	resourceName := applicationResourceName(deploymentTarget)
	if resourceName == "" {
		return kubeprovider.ApplicationResourcesSpec{}, errors.New("deployment kubernetes name is required")
	}
	servicePort := configuredServicePorts[0].Port
	servicePorts := deploymentTargetApplicationServicePorts(deploymentTarget)
	replicas := deploymentTarget.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	return kubeprovider.ApplicationResourcesSpec{
		Name:                         resourceName,
		Namespace:                    namespace,
		ProjectID:                    project.ID,
		ApplicationID:                application.ID,
		DeploymentTargetID:           deploymentTarget.ID,
		ReleaseID:                    release.ID,
		BuildRunID:                   release.BuildRunID,
		Image:                        strings.TrimSpace(release.ImageRef),
		WorkloadType:                 strings.TrimSpace(deploymentTarget.WorkloadType),
		Replicas:                     int32(replicas),
		ServicePort:                  int32(servicePort),
		ServicePorts:                 servicePorts,
		CPURequest:                   strings.TrimSpace(deploymentTarget.CPURequest),
		MemoryRequest:                strings.TrimSpace(deploymentTarget.MemoryRequest),
		CPULimit:                     "",
		MemoryLimit:                  "",
		ImagePullPolicy:              strings.TrimSpace(deploymentTarget.ImagePullPolicy),
		ContainerCommand:             strings.TrimSpace(deploymentTarget.ContainerCommand),
		ContainerArgs:                strings.TrimSpace(deploymentTarget.ContainerArgs),
		Lifecycle:                    strings.TrimSpace(deploymentTarget.Lifecycle),
		InitContainers:               strings.TrimSpace(deploymentTarget.InitContainers),
		SidecarContainers:            strings.TrimSpace(deploymentTarget.SidecarContainers),
		ReadinessProbe:               strings.TrimSpace(deploymentTarget.ReadinessProbe),
		LivenessProbe:                strings.TrimSpace(deploymentTarget.LivenessProbe),
		StartupProbe:                 strings.TrimSpace(deploymentTarget.StartupProbe),
		RunAsUser:                    strings.TrimSpace(deploymentTarget.RunAsUser),
		RunAsGroup:                   strings.TrimSpace(deploymentTarget.RunAsGroup),
		FSGroup:                      strings.TrimSpace(deploymentTarget.FSGroup),
		FSGroupChangePolicy:          strings.TrimSpace(deploymentTarget.FSGroupChangePolicy),
		ReadOnlyRootFilesystem:       deploymentTarget.ReadOnlyRootFilesystem,
		AllowPrivilegeEscalation:     "false",
		CapabilityAdd:                "",
		CapabilityDrop:               strings.TrimSpace(deploymentTarget.CapabilityDrop),
		NodeSelector:                 strings.TrimSpace(deploymentTarget.NodeSelector),
		Tolerations:                  strings.TrimSpace(deploymentTarget.Tolerations),
		Affinity:                     strings.TrimSpace(deploymentTarget.Affinity),
		TopologySpreadConstraints:    strings.TrimSpace(deploymentTarget.TopologySpreadConstraints),
		PriorityClassName:            strings.TrimSpace(deploymentTarget.PriorityClassName),
		ServiceAccountName:           "",
		AutomountServiceAccountToken: "false",
		ServiceType:                  "ClusterIP",
		ServiceAnnotations:           strings.TrimSpace(deploymentTarget.ServiceAnnotations),
		ServiceExternalTrafficPolicy: "",
		ServiceSessionAffinity:       strings.TrimSpace(deploymentTarget.ServiceSessionAffinity),
		AutoScalingEnabled:           deploymentTarget.AutoScalingEnabled,
		AutoScalingMinReplicas:       int32(deploymentTarget.AutoScalingMinReplicas),
		AutoScalingMaxReplicas:       int32(deploymentTarget.AutoScalingMaxReplicas),
		AutoScalingCPUPercent:        int32(deploymentTarget.AutoScalingCPUPercent),
		AutoScalingMemoryPercent:     int32(deploymentTarget.AutoScalingMemoryPercent),
		AutoScalingBehavior:          strings.TrimSpace(deploymentTarget.AutoScalingBehavior),
		RolloutTimeoutSeconds:        int32(rolloutTimeoutSeconds),
		ConfigData:                   configData,
		SecretData:                   secretData,
		ConfigFiles:                  configFiles,
		SecretFiles:                  secretFiles,
		DataVolumes:                  dataVolumes,
	}, nil
}

func deploymentTargetApplicationServicePorts(target model.DeploymentTarget) []kubeprovider.ApplicationServicePort {
	ports := model.DeploymentTargetServicePorts(target)
	result := make([]kubeprovider.ApplicationServicePort, 0, len(ports))
	for _, item := range ports {
		result = append(result, kubeprovider.ApplicationServicePort{Name: item.Name, Port: int32(item.Port), AppProtocol: strings.TrimSpace(item.AppProtocol)})
	}
	return result
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

func parseKeyValueMap(value string) (map[string]string, error) {
	return runtimeconfig.DecodeKeyValue(value)
}

// expandEnvRefsCrossBoundary 合并 config 和 secret 数据作为引用源，展开 config 中的 ${VAR_NAME} 引用。
// 这使得 DATABASE_URL=postgresql://${USER}:${PASSWORD}@${HOST}:${PORT}/db 可以在 PASSWORD 存在于 Secret 中时正确展开。
// config 更新为展开后的值；secret 保持原样（只作为引用源）。
func expandEnvRefsCrossBoundary(config, secret map[string]string) {
	if len(config) == 0 {
		return
	}
	// 合并 config 和 secret 作为查找源，config 优先级更高
	source := make(map[string]string, len(secret)+len(config))
	for k, v := range secret {
		source[k] = v
	}
	for k, v := range config {
		source[k] = v
	}
	expanded := variables.ExpandEnvRefs(source)
	// 只回写 config 中发生变化的 key，不修改 secret
	for key, value := range expanded {
		if current, ok := config[key]; ok && current != value {
			config[key] = value
		}
	}
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
		return "luna"
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
