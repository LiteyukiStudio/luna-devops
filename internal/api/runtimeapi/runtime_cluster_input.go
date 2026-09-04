package runtimeapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) runtimeClusterFromInput(ctx *gin.Context, user model.User, input runtimeClusterInput, clusterID string) (model.RuntimeCluster, bool) {
	scope, ownerRef, projectIDs, ok := h.normalizeScopedOwnerWithProjects(ctx, user, input.Scope, input.OwnerRef, input.ProjectIDs, "只有平台管理员可以维护全局运行集群")
	if !ok {
		return model.RuntimeCluster{}, false
	}
	if input.IsDefault && scope != "global" {
		writeError(ctx, http.StatusBadRequest, "只有全局运行集群可以设为默认集群")
		return model.RuntimeCluster{}, false
	}
	kubeconfigRef := ""
	if strings.TrimSpace(input.Kubeconfig) != "" {
		kubeconfig, err := flattenKubeconfig(input.Kubeconfig)
		if err != nil {
			writeError(ctx, http.StatusBadRequest, err.Error())
			return model.RuntimeCluster{}, false
		}
		kubeconfigRef = h.secrets.StoreContext(ctx.Request.Context(), kubeconfig, user.ID, "runtime_cluster:"+clusterID+":kubeconfig")
	}
	platformAdmin := user.Role == authz.PlatformRoleAdmin
	requestHeaders, err := parseGatewayHeaderMap(input.GatewayDefaultRequestHeaders, platformAdmin)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("默认请求头配置无效: %s", err.Error()))
		return model.RuntimeCluster{}, false
	}
	responseHeaders, err := parseGatewayHeaderMap(input.GatewayDefaultResponseHeaders, platformAdmin)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("默认响应头配置无效: %s", err.Error()))
		return model.RuntimeCluster{}, false
	}
	if err := validateTrustedProxyCIDRs(input.GatewayTrustedProxyCIDRs); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return model.RuntimeCluster{}, false
	}
	gatewayDomainSuffixes := runtimecluster.NormalizeGatewayDomainSuffixes(input.GatewayDomainSuffixes)
	policy := runtimeClusterResourcePolicy(input)
	if err := policy.Validate(); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime.resource_policy_invalid", err.Error())
		return model.RuntimeCluster{}, false
	}
	return model.RuntimeCluster{
		ID:                            clusterID,
		Name:                          strings.TrimSpace(input.Name),
		Endpoint:                      strings.TrimSpace(input.Endpoint),
		Scope:                         scope,
		OwnerRef:                      ownerRef,
		ProjectIDs:                    projectIDs,
		KubeconfigRef:                 kubeconfigRef,
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
		GatewayDefaultRequestHeaders:  encodeGatewayHeaderMap(requestHeaders),
		GatewayDefaultResponseHeaders: encodeGatewayHeaderMap(responseHeaders),
		CreatedBy:                     user.ID,
	}, true
}

func flattenKubeconfig(kubeconfig string) (string, error) {
	output, err := kubeprovider.NormalizeSafeKubeconfig(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("kubeconfig 不安全或格式无效，请仅使用内联凭据和 HTTPS API Server: %w", err)
	}
	return output, nil
}

func (h *Handlers) saveRuntimeClusterWithDefault(cluster model.RuntimeCluster, ctx context.Context) error {
	return h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return h.saveRuntimeClusterWithDefaultTx(tx, cluster)
	})
}

func (h *Handlers) saveRuntimeClusterWithDefaultTx(tx *gorm.DB, cluster model.RuntimeCluster) error {
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
	if err := tx.Save(&cluster).Error; err != nil {
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
