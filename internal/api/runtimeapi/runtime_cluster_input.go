package runtimeapi

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/apimachinery/pkg/util/validation"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handlers) runtimeClusterFromInput(ctx *gin.Context, user model.User, input runtimeClusterInput, clusterID string) (model.RuntimeCluster, string, bool) {
	scope, ownerRef, projectIDs, ok := h.normalizeScopedOwnerWithProjects(ctx, user, input.Scope, input.OwnerRef, input.ProjectIDs, "只有平台管理员可以维护全局运行集群")
	if !ok {
		return model.RuntimeCluster{}, "", false
	}
	if input.IsDefault && scope != "global" {
		writeError(ctx, http.StatusBadRequest, "只有全局运行集群可以设为默认集群")
		return model.RuntimeCluster{}, "", false
	}
	flattenedKubeconfig := ""
	if strings.TrimSpace(input.Kubeconfig) != "" {
		kubeconfig, err := flattenKubeconfig(input.Kubeconfig)
		if err != nil {
			writeError(ctx, http.StatusBadRequest, err.Error())
			return model.RuntimeCluster{}, "", false
		}
		flattenedKubeconfig = kubeconfig
	}
	platformAdmin := user.Role == authz.PlatformRoleAdmin
	requestHeaders, err := parseGatewayHeaderMap(input.GatewayDefaultRequestHeaders, platformAdmin)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("默认请求头配置无效: %s", err.Error()))
		return model.RuntimeCluster{}, "", false
	}
	responseHeaders, err := parseGatewayHeaderMap(input.GatewayDefaultResponseHeaders, platformAdmin)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("默认响应头配置无效: %s", err.Error()))
		return model.RuntimeCluster{}, "", false
	}
	if err := validateTrustedProxyCIDRs(input.GatewayTrustedProxyCIDRs); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return model.RuntimeCluster{}, "", false
	}
	networkBlockedCIDRs, err := normalizeNetworkBlockedCIDRs(input.NetworkBlockedCIDRs)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.network_blocked_cidrs_invalid", err.Error())
		return model.RuntimeCluster{}, "", false
	}
	buildEgressMode := strings.TrimSpace(input.BuildEgressMode)
	if buildEgressMode == "" {
		buildEgressMode = runtimecluster.BuildEgressRestricted
	}
	if buildEgressMode != runtimecluster.BuildEgressRestricted && buildEgressMode != runtimecluster.BuildEgressPermissive {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.build_egress_mode_invalid", "build egress mode must be restricted or permissive")
		return model.RuntimeCluster{}, "", false
	}
	buildPrivateEgressCIDRs, err := normalizeBuildPrivateEgressCIDRs(input.BuildPrivateEgressCIDRs)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.build_private_egress_cidrs_invalid", err.Error())
		return model.RuntimeCluster{}, "", false
	}
	buildPrivateEgressPorts, err := normalizeBuildPrivateEgressPorts(input.BuildPrivateEgressPorts)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.build_private_egress_ports_invalid", err.Error())
		return model.RuntimeCluster{}, "", false
	}
	networkGatewayNamespace := strings.TrimSpace(input.NetworkGatewayNamespace)
	if networkGatewayNamespace != "" && len(validation.IsDNS1123Label(networkGatewayNamespace)) > 0 {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.network_gateway_namespace_invalid", "network gateway namespace must be a DNS-1123 label")
		return model.RuntimeCluster{}, "", false
	}
	gatewayDomainSuffixes := runtimecluster.NormalizeGatewayDomainSuffixes(input.GatewayDomainSuffixes)
	policy := runtimeClusterResourcePolicy(input)
	if err := policy.Validate(); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime.resource_policy_invalid", err.Error())
		return model.RuntimeCluster{}, "", false
	}
	return model.RuntimeCluster{
		ID:                            clusterID,
		Name:                          strings.TrimSpace(input.Name),
		Endpoint:                      strings.TrimSpace(input.Endpoint),
		Scope:                         scope,
		OwnerRef:                      ownerRef,
		ProjectIDs:                    projectIDs,
		IsDefault:                     input.IsDefault,
		MaxConcurrentBuilds:           normalizeBuildConcurrency(input.MaxConcurrentBuilds, defaultClusterBuildConcurrency),
		CPURequestPercent:             policy.CPURequestPercent,
		MemoryRequestPercent:          policy.MemoryRequestPercent,
		CPULimitPercent:               policy.CPULimitPercent,
		MemoryLimitPercent:            policy.MemoryLimitPercent,
		GatewayDomainSuffixesRaw:      runtimecluster.EncodeGatewayDomainSuffixes(gatewayDomainSuffixes),
		GatewayDomainSuffixes:         gatewayDomainSuffixes,
		GatewayPublicScheme:           normalizeGatewayPublicScheme(input.GatewayPublicScheme),
		GatewayPublicPort:             normalizeGatewayPublicPort(input.GatewayPublicPort, input.GatewayPublicScheme),
		GatewayControllerType:         normalizeGatewayControllerType(input.GatewayControllerType),
		GatewayClassName:              fallback(strings.TrimSpace(input.GatewayClassName), "traefik"),
		GatewayName:                   fallback(dnsLabelName(input.GatewayName), "luna-gateway"),
		GatewayNamespace:              fallback(dnsLabelName(input.GatewayNamespace), "kube-system"),
		GatewayHTTPListenerName:       fallback(dnsLabelName(input.GatewayHTTPListenerName), "web"),
		GatewayHTTPListenerPort:       normalizePort(input.GatewayHTTPListenerPort, 8080),
		GatewayHTTPSListenerName:      fallback(dnsLabelName(input.GatewayHTTPSListenerName), "websecure"),
		GatewayHTTPSListenerPort:      normalizePort(input.GatewayHTTPSListenerPort, 8443),
		GatewayTLSSecretName:          dnsLabelName(input.GatewayTLSSecretName),
		GatewayTLSSecretNamespace:     dnsLabelName(input.GatewayTLSSecretNamespace),
		GatewayCertIssuerKind:         normalizeGatewayCertIssuerKind(input.GatewayCertIssuerKind),
		GatewayCertIssuerName:         dnsLabelName(input.GatewayCertIssuerName),
		GatewayCertificateNamespace:   dnsLabelName(input.GatewayCertificateNamespace),
		GatewayWildcardCertEnabled:    input.GatewayWildcardCertEnabled,
		GatewayWildcardCertDomain:     runtimecluster.NormalizeGatewayDomainSuffix(input.GatewayWildcardCertDomain),
		GatewayWildcardCertSecretName: dnsLabelName(input.GatewayWildcardCertSecretName),
		GatewayExternalTLSMode:        normalizeGatewayExternalTLSMode(input.GatewayExternalTLSMode),
		GatewayForwardedHeadersMode:   normalizeGatewayForwardedHeadersMode(input.GatewayForwardedHeadersMode),
		GatewayTrustedProxyCIDRs:      strings.TrimSpace(input.GatewayTrustedProxyCIDRs),
		NetworkBlockedCIDRs:           networkBlockedCIDRs,
		NetworkGatewayNamespace:       networkGatewayNamespace,
		BuildEgressMode:               buildEgressMode,
		BuildPrivateEgressCIDRs:       buildPrivateEgressCIDRs,
		BuildPrivateEgressPorts:       buildPrivateEgressPorts,
		GatewayDefaultRequestHeaders:  encodeGatewayHeaderMap(requestHeaders),
		GatewayDefaultResponseHeaders: encodeGatewayHeaderMap(responseHeaders),
		CreatedBy:                     user.ID,
	}, flattenedKubeconfig, true
}

func flattenKubeconfig(kubeconfig string) (string, error) {
	output, err := kubeprovider.NormalizeSafeKubeconfig(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("kubeconfig 不安全或格式无效，请仅使用内联凭据和 HTTPS API Server: %w", err)
	}
	return output, nil
}

var (
	errRuntimeClusterInUse     = errors.New("runtime cluster is still referenced by active resources")
	errRuntimeClusterNotActive = errors.New("runtime cluster is not active")
)

func (h *Handlers) createRuntimeClusterWithDefault(cluster model.RuntimeCluster, flattenedKubeconfig, userID string, ctx context.Context) (model.RuntimeCluster, error) {
	err := h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if strings.TrimSpace(flattenedKubeconfig) != "" && h.secrets.Available() {
			ref, storeErr := h.secrets.StoreContextWithDB(ctx, tx, flattenedKubeconfig, userID, "runtime_cluster:"+cluster.ID+":kubeconfig")
			if storeErr != nil {
				return storeErr
			}
			cluster.KubeconfigRef = ref
		}
		return h.saveRuntimeClusterWithDefaultTx(tx, cluster, false)
	})
	return cluster, err
}

func (h *Handlers) updateRuntimeClusterWithDefault(clusterID string, next model.RuntimeCluster, flattenedKubeconfig string, resetProbe bool, userID string, ctx context.Context) (model.RuntimeCluster, model.RuntimeCluster, bool, bool, error) {
	var previous model.RuntimeCluster
	var updated model.RuntimeCluster
	grantsPruned := false
	networkChanged := false
	err := h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.RuntimeCluster
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", clusterID).Error; err != nil {
			return err
		}
		if !runtimecluster.IsActive(locked) {
			return errRuntimeClusterNotActive
		}
		previous = locked
		if resetProbe {
			locked.NetworkPolicyEnforcement = kubeprovider.NetworkPolicyEnforcementUnknown
			locked.NetworkPolicyProbeCode = ""
			locked.NetworkPolicyProbedAt = nil
		}
		networkChanged = locked.NetworkBlockedCIDRs != next.NetworkBlockedCIDRs ||
			locked.NetworkGatewayNamespace != next.NetworkGatewayNamespace ||
			locked.GatewayNamespace != next.GatewayNamespace ||
			locked.GatewayHTTPListenerPort != next.GatewayHTTPListenerPort ||
			locked.GatewayHTTPSListenerPort != next.GatewayHTTPSListenerPort ||
			locked.BuildEgressMode != next.BuildEgressMode ||
			locked.BuildPrivateEgressCIDRs != next.BuildPrivateEgressCIDRs ||
			locked.BuildPrivateEgressPorts != next.BuildPrivateEgressPorts
		locked.Name = next.Name
		locked.Endpoint = next.Endpoint
		locked.Scope = next.Scope
		locked.OwnerRef = next.OwnerRef
		locked.ProjectIDs = next.ProjectIDs
		if strings.TrimSpace(flattenedKubeconfig) != "" && h.secrets.Available() {
			resource := "runtime_cluster:" + locked.ID + ":kubeconfig"
			newRef, storeErr := h.secrets.StoreContextWithDB(ctx, tx, flattenedKubeconfig, userID, resource)
			if storeErr != nil {
				return storeErr
			}
			if strings.TrimSpace(previous.KubeconfigRef) != "" && previous.KubeconfigRef != newRef {
				if delErr := h.secrets.DeleteRefContextWithDB(ctx, tx, previous.KubeconfigRef, resource); delErr != nil {
					return delErr
				}
			}
			locked.KubeconfigRef = newRef
		}
		locked.IsDefault = next.IsDefault
		locked.MaxConcurrentBuilds = next.MaxConcurrentBuilds
		locked.CPURequestPercent = next.CPURequestPercent
		locked.MemoryRequestPercent = next.MemoryRequestPercent
		locked.CPULimitPercent = next.CPULimitPercent
		locked.MemoryLimitPercent = next.MemoryLimitPercent
		locked.GatewayDomainSuffixesRaw = next.GatewayDomainSuffixesRaw
		locked.GatewayDomainSuffixes = next.GatewayDomainSuffixes
		locked.GatewayPublicScheme = next.GatewayPublicScheme
		locked.GatewayPublicPort = next.GatewayPublicPort
		locked.GatewayControllerType = next.GatewayControllerType
		locked.GatewayClassName = next.GatewayClassName
		locked.GatewayName = next.GatewayName
		locked.GatewayNamespace = next.GatewayNamespace
		locked.GatewayHTTPListenerName = next.GatewayHTTPListenerName
		locked.GatewayHTTPListenerPort = next.GatewayHTTPListenerPort
		locked.GatewayHTTPSListenerName = next.GatewayHTTPSListenerName
		locked.GatewayHTTPSListenerPort = next.GatewayHTTPSListenerPort
		locked.GatewayTLSSecretName = next.GatewayTLSSecretName
		locked.GatewayTLSSecretNamespace = next.GatewayTLSSecretNamespace
		locked.GatewayCertIssuerKind = next.GatewayCertIssuerKind
		locked.GatewayCertIssuerName = next.GatewayCertIssuerName
		locked.GatewayCertificateNamespace = next.GatewayCertificateNamespace
		locked.GatewayWildcardCertEnabled = next.GatewayWildcardCertEnabled
		locked.GatewayWildcardCertDomain = next.GatewayWildcardCertDomain
		locked.GatewayWildcardCertSecretName = next.GatewayWildcardCertSecretName
		locked.GatewayExternalTLSMode = next.GatewayExternalTLSMode
		locked.GatewayForwardedHeadersMode = next.GatewayForwardedHeadersMode
		locked.NetworkBlockedCIDRs = next.NetworkBlockedCIDRs
		locked.NetworkGatewayNamespace = next.NetworkGatewayNamespace
		locked.BuildEgressMode = next.BuildEgressMode
		locked.BuildPrivateEgressCIDRs = next.BuildPrivateEgressCIDRs
		locked.BuildPrivateEgressPorts = next.BuildPrivateEgressPorts
		locked.GatewayTrustedProxyCIDRs = next.GatewayTrustedProxyCIDRs
		locked.GatewayDefaultRequestHeaders = next.GatewayDefaultRequestHeaders
		locked.GatewayDefaultResponseHeaders = next.GatewayDefaultResponseHeaders
		if err := h.saveRuntimeClusterWithDefaultTx(tx, locked, resetProbe); err != nil {
			return err
		}
		if err := ensureUpdatedRuntimeClusterReferencesAccessible(tx, previous, locked); err != nil {
			return err
		}
		var pruneErr error
		grantsPruned, pruneErr = pruneUnusableClusterEgressExceptionGrants(tx, locked)
		if pruneErr != nil {
			return pruneErr
		}
		updated = locked
		return nil
	})
	return previous, updated, grantsPruned, networkChanged, err
}

func projectCoveredByClusterScope(tx *gorm.DB, cluster model.RuntimeCluster, projectID string) (bool, error) {
	var projectCount int64
	if err := tx.Model(&model.Project{}).Where("id = ?", projectID).Count(&projectCount).Error; err != nil {
		return false, err
	}
	if projectCount == 0 {
		return false, nil
	}
	var count int64
	switch cluster.Scope {
	case runtimecluster.ScopeGlobal:
		return true, nil
	case "project":
		err := tx.Model(&model.ScopedResourceProjectBinding{}).
			Where("resource_type = ? and resource_id = ? and project_id = ?", scopedResourceRuntimeCluster, cluster.ID, projectID).
			Count(&count).Error
		return count > 0, err
	case "user":
		err := tx.Model(&model.ProjectMember{}).
			Where("project_id = ? and user_id = ?", projectID, cluster.OwnerRef).
			Count(&count).Error
		return count > 0, err
	default:
		return false, nil
	}
}

func ensureUpdatedRuntimeClusterReferencesAccessible(tx *gorm.DB, previous, cluster model.RuntimeCluster) error {
	if previous.Scope == "global" && cluster.Scope != "global" {
		if _, err := runtimecluster.ResolveForTarget(tx, ""); err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			var implicitTargetCount int64
			if err := tx.Model(&model.DeploymentTarget{}).
				Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
				Where("cluster_id = ''").
				Count(&implicitTargetCount).Error; err != nil {
				return err
			}
			if implicitTargetCount > 0 {
				return errRuntimeClusterInUse
			}
		}
	}
	if previous.Scope == cluster.Scope && previous.OwnerRef == cluster.OwnerRef && cluster.Scope == "global" {
		return nil
	}
	referencedProjects := map[string]bool{}
	var targetProjectIDs []string
	if err := tx.Model(&model.DeploymentTarget{}).
		Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
		Where("deployment_targets.cluster_id = ?", cluster.ID).
		Distinct("deployment_targets.project_id").
		Pluck("deployment_targets.project_id", &targetProjectIDs).Error; err != nil {
		return err
	}
	for _, projectID := range targetProjectIDs {
		referencedProjects[projectID] = true
	}
	var volumeProjectIDs []string
	if err := tx.Model(&model.ProjectVolume{}).
		Joins("join projects on projects.id = project_volumes.project_id and projects.deleted_at is null").
		Where("project_volumes.cluster_id = ?", cluster.ID).
		Distinct("project_volumes.project_id").
		Pluck("project_volumes.project_id", &volumeProjectIDs).Error; err != nil {
		return err
	}
	for _, projectID := range volumeProjectIDs {
		referencedProjects[projectID] = true
	}
	for projectID := range referencedProjects {
		covered, err := projectCoveredByClusterScope(tx, cluster, projectID)
		if err != nil {
			return err
		}
		if !covered {
			return errRuntimeClusterInUse
		}
	}
	return nil
}

func pruneUnusableClusterEgressExceptionGrants(tx *gorm.DB, cluster model.RuntimeCluster) (bool, error) {
	if cluster.Scope == "global" {
		return false, nil
	}
	var grants []model.NetworkEgressExceptionGrant
	exceptionsSubquery := tx.Model(&model.NetworkEgressException{}).Select("id").Where("runtime_cluster_id = ?", cluster.ID)
	if err := tx.Where("exception_id in (?)", exceptionsSubquery).Find(&grants).Error; err != nil {
		return false, err
	}
	pruned := false
	for _, grant := range grants {
		covered, err := projectCoveredByClusterScope(tx, cluster, grant.ProjectID)
		if err != nil {
			return false, err
		}
		if covered {
			continue
		}
		if err := tx.Delete(&grant).Error; err != nil {
			return false, err
		}
		pruned = true
	}
	return pruned, nil
}

// saveRuntimeClusterWithDefaultTx 保存集群配置。探测结论由 Worker 写入，普通保存不得用请求开始时读到的旧值覆盖；
// 只有替换 kubeconfig 时才随保存把结论重置为 unknown。
func saveRuntimeClusterRecord(tx *gorm.DB, cluster model.RuntimeCluster, resetProbe bool) error {
	if !resetProbe {
		tx = tx.Omit("network_policy_enforcement", "network_policy_probe_code", "network_policy_probed_at")
	}
	return tx.Save(&cluster).Error
}

func (h *Handlers) saveRuntimeClusterWithDefaultTx(tx *gorm.DB, cluster model.RuntimeCluster, resetProbe bool) error {
	if cluster.IsDefault {
		if cluster.Scope != "global" {
			return errors.New("只有全局运行集群可以设为默认集群")
		}
		if err := tx.Model(&model.RuntimeCluster{}).Where("scope = ? and id <> ?", "global", cluster.ID).Update("is_default", false).Error; err != nil {
			return err
		}
	} else if cluster.Scope != "global" {
		cluster.IsDefault = false
	}
	if err := saveRuntimeClusterRecord(tx, cluster, resetProbe); err != nil {
		return err
	}
	return h.replaceScopedResourceProjectBindings(tx, scopedResourceRuntimeCluster, cluster.ID, sortedProjectIDs(cluster.ProjectIDs), nil)
}

type runtimeClusterInput struct {
	Name                          string   `json:"name" binding:"required"`
	Endpoint                      string   `json:"endpoint"`
	Scope                         string   `json:"scope"`
	OwnerRef                      string   `json:"ownerRef"`
	ProjectIDs                    []string `json:"projectIds"`
	Kubeconfig                    string   `json:"kubeconfig"`
	IsDefault                     bool     `json:"isDefault"`
	MaxConcurrentBuilds           int      `json:"maxConcurrentBuilds"`
	CPURequestPercent             *int     `json:"cpuRequestPercent"`
	MemoryRequestPercent          *int     `json:"memoryRequestPercent"`
	CPULimitPercent               *int     `json:"cpuLimitPercent"`
	MemoryLimitPercent            *int     `json:"memoryLimitPercent"`
	GatewayDomainSuffixes         []string `json:"gatewayDomainSuffixes"`
	GatewayPublicScheme           string   `json:"gatewayPublicScheme"`
	GatewayPublicPort             int      `json:"gatewayPublicPort"`
	GatewayControllerType         string   `json:"gatewayControllerType"`
	GatewayClassName              string   `json:"gatewayClassName"`
	GatewayName                   string   `json:"gatewayName"`
	GatewayNamespace              string   `json:"gatewayNamespace"`
	GatewayHTTPListenerName       string   `json:"gatewayHttpListenerName"`
	GatewayHTTPListenerPort       int      `json:"gatewayHttpListenerPort"`
	GatewayHTTPSListenerName      string   `json:"gatewayHttpsListenerName"`
	GatewayHTTPSListenerPort      int      `json:"gatewayHttpsListenerPort"`
	GatewayTLSSecretName          string   `json:"gatewayTlsSecretName"`
	GatewayTLSSecretNamespace     string   `json:"gatewayTlsSecretNamespace"`
	GatewayCertIssuerKind         string   `json:"gatewayCertIssuerKind"`
	GatewayCertIssuerName         string   `json:"gatewayCertIssuerName"`
	GatewayCertificateNamespace   string   `json:"gatewayCertificateNamespace"`
	GatewayWildcardCertEnabled    bool     `json:"gatewayWildcardCertEnabled"`
	GatewayWildcardCertDomain     string   `json:"gatewayWildcardCertDomain"`
	GatewayWildcardCertSecretName string   `json:"gatewayWildcardCertSecretName"`
	GatewayExternalTLSMode        string   `json:"gatewayExternalTLSMode"`
	GatewayForwardedHeadersMode   string   `json:"gatewayForwardedHeadersMode"`
	GatewayTrustedProxyCIDRs      string   `json:"gatewayTrustedProxyCIDRs"`
	NetworkBlockedCIDRs           string   `json:"networkBlockedCIDRs"`
	NetworkGatewayNamespace       string   `json:"networkGatewayNamespace"`
	BuildEgressMode               string   `json:"buildEgressMode"`
	BuildPrivateEgressCIDRs       string   `json:"buildPrivateEgressCIDRs"`
	BuildPrivateEgressPorts       string   `json:"buildPrivateEgressPorts"`
	GatewayDefaultRequestHeaders  string   `json:"gatewayDefaultRequestHeaders"`
	GatewayDefaultResponseHeaders string   `json:"gatewayDefaultResponseHeaders"`
}

func runtimeClusterResourcePolicy(input runtimeClusterInput) resourcepolicy.Policy {
	defaults := resourcepolicy.Default()
	if input.CPURequestPercent != nil {
		defaults.CPURequestPercent = *input.CPURequestPercent
	}
	if input.MemoryRequestPercent != nil {
		defaults.MemoryRequestPercent = *input.MemoryRequestPercent
	}
	if input.CPULimitPercent != nil {
		defaults.CPULimitPercent = *input.CPULimitPercent
	}
	if input.MemoryLimitPercent != nil {
		defaults.MemoryLimitPercent = *input.MemoryLimitPercent
	}
	return defaults
}

func normalizeGatewayPublicScheme(value string) string {
	if strings.ToLower(strings.TrimSpace(value)) == "https" {
		return "https"
	}
	return "http"
}

func normalizePort(value int, fallbackValue int) int {
	if value >= 1 && value <= 65535 {
		return value
	}
	return fallbackValue
}

func normalizeGatewayPublicPort(value int, scheme string) int {
	if normalizeGatewayPublicScheme(scheme) == "https" {
		return normalizePort(value, 443)
	}
	return normalizePort(value, 80)
}

func normalizeGatewayControllerType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "generic":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "traefik"
	}
}

func normalizeGatewayExternalTLSMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "gateway", "upstream":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "none"
	}
}

func normalizeGatewayCertIssuerKind(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "Issuer") {
		return "Issuer"
	}
	return "ClusterIssuer"
}

func dnsLabelName(value string) string {
	value = strings.Trim(strings.ToLower(strings.TrimSpace(value)), "-")
	if value == "" {
		return ""
	}
	value = gatewayHostSegmentPattern.ReplaceAllString(value, "-")
	value = strings.Join(strings.FieldsFunc(value, func(char rune) bool { return char == '-' }), "-")
	return strings.Trim(value, "-")
}

func normalizeGatewayForwardedHeadersMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "overwrite", "none":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "preserve"
	}
}

func validateTrustedProxyCIDRs(value string) error {
	for _, item := range strings.FieldsFunc(value, func(char rune) bool {
		return char == '\n' || char == ',' || char == ';'
	}) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, err := netip.ParsePrefix(item); err != nil {
			return fmt.Errorf("可信代理 CIDR %q 无效", item)
		}
	}
	return nil
}

// normalizeNetworkBlockedCIDRs 校验管理员追加的出站拒绝网段：每行一个合法 CIDR，不能是 /0，规范化后去重。
func normalizeNetworkBlockedCIDRs(value string) (string, error) {
	prefixes, err := parseCIDRLines(value, "blocked")
	if err != nil {
		return "", err
	}
	return joinPrefixes(prefixes), nil
}

// normalizeBuildPrivateEgressCIDRs 校验构建可访问的私网镜像站网段：每行一个合法 CIDR，不能是 /0，
// 不能恰好是云主机元数据地址，规范化后去重。
func normalizeBuildPrivateEgressCIDRs(value string) (string, error) {
	prefixes, err := parseCIDRLines(value, "build private egress")
	if err != nil {
		return "", err
	}
	for _, prefix := range prefixes {
		for _, metadata := range kubeprovider.CloudMetadataCIDRs() {
			// 包含元数据地址的网段由渲染器排除元数据地址；网段恰好等于元数据地址时无法排除，直接拒绝。
			if prefix == netip.MustParsePrefix(metadata) {
				return "", fmt.Errorf("build private egress CIDR %q is a cloud metadata address", prefix.String())
			}
		}
	}
	return joinPrefixes(prefixes), nil
}

func parseCIDRLines(value string, kind string) ([]netip.Prefix, error) {
	seen := map[netip.Prefix]bool{}
	result := []netip.Prefix{}
	for _, item := range strings.FieldsFunc(value, func(char rune) bool {
		return char == '\n' || char == ',' || char == ';'
	}) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		prefix, err := kubeprovider.ParseNetworkCIDR(item)
		if err != nil {
			return nil, fmt.Errorf("%s CIDR %q is invalid", kind, item)
		}
		if prefix.Bits() == 0 {
			return nil, fmt.Errorf("%s CIDR %q must not be /0", kind, item)
		}
		prefix = prefix.Masked()
		if !seen[prefix] {
			seen[prefix] = true
			result = append(result, prefix)
		}
	}
	if len(result) > 256 {
		return nil, fmt.Errorf("at most 256 %s CIDRs are allowed", kind)
	}
	return result, nil
}

func joinPrefixes(prefixes []netip.Prefix) string {
	values := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		values = append(values, prefix.String())
	}
	return strings.Join(values, "\n")
}

// normalizeBuildPrivateEgressPorts 校验私网镜像站放行端口：逗号或换行分隔的 1-65535 端口，最多 16 个；为空时为 443。
func normalizeBuildPrivateEgressPorts(value string) (string, error) {
	seen := map[int]bool{}
	ports := []string{}
	for _, item := range strings.FieldsFunc(value, func(char rune) bool { return char == ',' || char == '\n' || char == ' ' }) {
		port, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || port < 1 || port > 65535 {
			return "", fmt.Errorf("build private egress port %q is invalid", item)
		}
		if !seen[port] {
			seen[port] = true
			ports = append(ports, strconv.Itoa(port))
		}
	}
	if len(ports) > 16 {
		return "", fmt.Errorf("at most 16 build private egress ports are allowed")
	}
	if len(ports) == 0 {
		return "443", nil
	}
	return strings.Join(ports, ","), nil
}
