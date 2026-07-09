package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
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
		if !h.requireStepUp(ctx, user, stepUpPurposeKubeconfigUpdate) {
			return model.RuntimeCluster{}, false
		}
		kubeconfig, err := flattenKubeconfig(input.Kubeconfig)
		if err != nil {
			writeError(ctx, http.StatusBadRequest, err.Error())
			return model.RuntimeCluster{}, false
		}
		kubeconfigRef = h.secrets.Store(kubeconfig, user.ID, "runtime_cluster:"+clusterID+":kubeconfig")
	}
	platformAdmin := user.Role == "platform_admin"
	if _, err := parseGatewayHeaderMap(input.GatewayDefaultRequestHeaders, platformAdmin); err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("默认请求头配置无效: %s", err.Error()))
		return model.RuntimeCluster{}, false
	}
	if _, err := parseGatewayHeaderMap(input.GatewayDefaultResponseHeaders, platformAdmin); err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("默认响应头配置无效: %s", err.Error()))
		return model.RuntimeCluster{}, false
	}
	if err := validateTrustedProxyCIDRs(input.GatewayTrustedProxyCIDRs); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return model.RuntimeCluster{}, false
	}
	gatewayDomainSuffixes := normalizeGatewayDomainSuffixes(input.GatewayDomainSuffixes, input.GatewayRootDomain, h.legacyGatewayRootDomain())
	return model.RuntimeCluster{
		ID:                            clusterID,
		Name:                          strings.TrimSpace(input.Name),
		Type:                          normalizeRuntimeClusterType(input.Type),
		Endpoint:                      strings.TrimSpace(input.Endpoint),
		Scope:                         scope,
		OwnerRef:                      ownerRef,
		ProjectIDs:                    projectIDs,
		KubeconfigRef:                 kubeconfigRef,
		IsDefault:                     input.IsDefault,
		MaxConcurrentBuilds:           normalizeBuildConcurrency(input.MaxConcurrentBuilds, defaultClusterBuildConcurrency),
		GatewayProvider:               normalizeGatewayProvider(input.GatewayProvider),
		GatewayRootDomain:             gatewayDomainSuffixes[0],
		GatewayDomainSuffixesRaw:      encodeGatewayDomainSuffixes(gatewayDomainSuffixes),
		GatewayDomainSuffixes:         gatewayDomainSuffixes,
		GatewayPublicScheme:           normalizeGatewayPublicScheme(input.GatewayPublicScheme),
		GatewayPublicPort:             normalizeGatewayPublicPort(input.GatewayPublicPort, input.GatewayPublicScheme),
		GatewayControllerType:         normalizeGatewayControllerType(input.GatewayControllerType),
		GatewayClassName:              fallback(strings.TrimSpace(input.GatewayClassName), "traefik"),
		GatewayName:                   fallback(dnsLabelName(input.GatewayName), "liteyuki-gateway"),
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
		GatewayWildcardCertDomain:     normalizeGatewayDomainSuffixValue(input.GatewayWildcardCertDomain),
		GatewayWildcardCertSecretName: dnsLabelName(input.GatewayWildcardCertSecretName),
		GatewayExternalTLSMode:        normalizeGatewayExternalTLSMode(input.GatewayExternalTLSMode),
		GatewayForwardedHeadersMode:   normalizeGatewayForwardedHeadersMode(input.GatewayForwardedHeadersMode),
		GatewayTrustedProxyCIDRs:      strings.TrimSpace(input.GatewayTrustedProxyCIDRs),
		GatewayDefaultRequestHeaders:  strings.TrimSpace(input.GatewayDefaultRequestHeaders),
		GatewayDefaultResponseHeaders: strings.TrimSpace(input.GatewayDefaultResponseHeaders),
		Status:                        fallback(strings.TrimSpace(input.Status), "unknown"),
		CreatedBy:                     user.ID,
	}, true
}

func flattenKubeconfig(kubeconfig string) (string, error) {
	config, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return "", fmt.Errorf("kubeconfig 无效，请检查格式")
	}
	if err := api.FlattenConfig(config); err != nil {
		return "", fmt.Errorf("kubeconfig 引用了当前 API 无法读取的证书文件，请导入已内联证书的 kubeconfig: %w", err)
	}
	output, err := clientcmd.Write(*config)
	if err != nil {
		return "", fmt.Errorf("kubeconfig 序列化失败")
	}
	return string(output), nil
}

func (h *Handlers) saveRuntimeClusterWithDefault(cluster model.RuntimeCluster) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
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
	})
}

func normalizeRuntimeClusterType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "docker-compose":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "kubernetes"
	}
}

type runtimeClusterInput struct {
	Name                          string   `json:"name" binding:"required"`
	Type                          string   `json:"type"`
	Endpoint                      string   `json:"endpoint"`
	Scope                         string   `json:"scope"`
	OwnerRef                      string   `json:"ownerRef"`
	ProjectIDs                    []string `json:"projectIds"`
	Kubeconfig                    string   `json:"kubeconfig"`
	IsDefault                     bool     `json:"isDefault"`
	MaxConcurrentBuilds           int      `json:"maxConcurrentBuilds"`
	GatewayProvider               string   `json:"gatewayProvider"`
	GatewayRootDomain             string   `json:"gatewayRootDomain"`
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
	Status                        string   `json:"status"`
}

func normalizeGatewayRootDomain(value string, fallbackValue string) string {
	rootDomain := strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".")
	if rootDomain == "" {
		rootDomain = strings.Trim(strings.ToLower(strings.TrimSpace(fallbackValue)), ".")
	}
	if rootDomain == "" {
		return "apps.local"
	}
	return rootDomain
}

func normalizeGatewayDomainSuffixes(values []string, legacyValue string, fallbackValue string) []string {
	if output := normalizeGatewayDomainSuffixList(values); len(output) > 0 {
		return output
	}
	output := normalizeGatewayDomainSuffixList([]string{legacyValue, fallbackValue, "apps.local"})
	if len(output) == 0 {
		return []string{"apps.local"}
	}
	return output
}

func normalizeGatewayDomainSuffixList(values []string) []string {
	seen := map[string]bool{}
	output := make([]string, 0, len(values))
	for _, value := range values {
		suffix := normalizeGatewayDomainSuffixValue(value)
		if suffix == "" || seen[suffix] {
			continue
		}
		seen[suffix] = true
		output = append(output, suffix)
	}
	return output
}

func normalizeGatewayDomainSuffixValue(value string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".")
}

func encodeGatewayDomainSuffixes(values []string) string {
	return strings.Join(normalizeGatewayDomainSuffixList(values), "\n")
}

func decodeGatewayDomainSuffixes(raw string, legacyValue string, fallbackValue string) []string {
	return normalizeGatewayDomainSuffixes(strings.FieldsFunc(raw, func(char rune) bool {
		return char == '\n' || char == ',' || char == ';'
	}), legacyValue, fallbackValue)
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

func normalizeGatewayProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "gateway-api":
		return "gateway-api"
	default:
		return "gateway-api"
	}
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
