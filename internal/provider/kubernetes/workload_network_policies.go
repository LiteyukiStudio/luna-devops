package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/retry"
)

const (
	// WorkloadNetworkPolicyScope 标记平台为项目空间工作负载渲染的 NetworkPolicy 集合。
	WorkloadNetworkPolicyScope  = "workload-network"
	NetworkPolicyHashAnnotation = "luna.devops/policy-hash"

	workloadIngressPolicyName        = "luna-workload-ingress"
	workloadEgressPolicyName         = "luna-workload-egress"
	buildEgressPolicyName            = "luna-build-egress"
	gatewayIngressPolicyName         = "luna-gateway-ingress"
	egressExceptionPolicyPrefix      = "luna-egress-exception-"
	buildEgressExceptionPolicyPrefix = "luna-build-egress-exception-"
	dependencyIngressPolicyPrefix    = "luna-dependency-ingress-"
	sharedIngressPolicyPrefix        = "luna-shared-ingress-"
	sharedEgressPolicyPrefix         = "luna-shared-egress-"
	buildPodScope                    = "build"
	systemEgressPolicyPrefix         = "luna-system-egress-"
	namespaceNameLabel               = "kubernetes.io/metadata.name"

	WorkloadNetworkPolicySynced  = "synced"
	WorkloadNetworkPolicyDrifted = "drifted"
)

// WorkloadNetworkPolicyInput 是项目空间 Namespace 网络隔离的全部期望输入。
// 渲染是纯函数：相同输入总是得到相同的策略集合。
type WorkloadNetworkPolicyInput struct {
	Namespace        string
	GatewayNamespace string
	// GatewayPorts 是网关控制器接收流量的端口；租户 Pod 回环访问网关只放行这些端口。
	GatewayPorts []int
	// BlockedCIDRs 是集群管理员在平台保留网段之外追加的出站拒绝网段。
	BlockedCIDRs []string
	// NodeAddresses 来自集群 Node 与 kube-apiserver Endpoint，出站拒绝、入站放行（宿主机网络组件）。
	NodeAddresses []string
	// TrustedDeploymentTargetIDs 是可信系统组件的部署配置，其 Pod 出站不受项目隔离限制。
	TrustedDeploymentTargetIDs []string
	// BuildEgress 是构建 Pod 的出站策略输入。
	BuildEgress BuildEgressInput
	// IngressMode：isolated 允许同项目空间互访；dependencies 只放行服务引用声明的来源与同一部署配置的副本之间。
	IngressMode string
	// DeploymentTargetIDs 是项目空间全部部署配置；Dependencies 为目标部署配置到其服务引用来源的映射。
	DeploymentTargetIDs []string
	Dependencies        map[string][]string
	// InternalTargetIDs 是“内部服务”部署配置，其 Pod 不接受网关流量。
	InternalTargetIDs []string
	// EgressMode：internet 放行公网；allowlist 只放行 EgressAllowlist；none 只允许同项目空间与集群 DNS。
	EgressMode      string
	EgressAllowlist []EgressAllowRule
	// EgressExceptions 是管理员授权给本项目空间的集群级出站例外，始终生效，不受项目出站模式影响。
	EgressExceptions []EgressException
	// SharedIngress 是本项目空间作为提供方已生效的跨项目共享：部署配置 ID 到可访问它的使用方 Namespace。
	SharedIngress map[string][]string
	// SharedEgress 是本项目空间作为使用方已生效的跨项目共享，不受项目出站模式影响。
	SharedEgress []SharedService
}

// SharedService 是另一个项目空间共享给本项目空间的部署配置。
type SharedService struct {
	Namespace string
	TargetID  string
}

// EgressException 是渲染一条出站例外所需的输入；TargetIDs 为空表示整个项目空间的工作负载。
type EgressException struct {
	ID            string
	CIDR          string
	Ports         []int
	Protocol      string
	TargetIDs     []string
	IncludeBuilds bool
}

const (
	EgressExceptionProtocolTCP = "tcp"
	EgressExceptionProtocolUDP = "udp"
	EgressExceptionProtocolAny = "any"
)

// EgressAllowRule 是项目空间出站白名单的一条：CIDR 与可选端口（为空表示全部端口）。
type EgressAllowRule struct {
	CIDR  string
	Ports []int
}

const (
	NetworkIngressIsolated     = "isolated"
	NetworkIngressDependencies = "dependencies"
	NetworkEgressInternet      = "internet"
	NetworkEgressAllowlist     = "allowlist"
	NetworkEgressNone          = "none"
)

// BuildEgressInput 描述构建 Pod 出站：受限模式只放行集群 DNS、公网 80/443 和管理员放行的私网镜像站。
type BuildEgressInput struct {
	Permissive   bool
	PrivateCIDRs []string
	PrivatePorts []int
}

func RenderWorkloadNetworkPolicies(input WorkloadNetworkPolicyInput) ([]networkingv1.NetworkPolicy, error) {
	input.Namespace = strings.TrimSpace(input.Namespace)
	input.GatewayNamespace = strings.TrimSpace(input.GatewayNamespace)
	if errs := validation.IsDNS1123Label(input.Namespace); len(errs) > 0 {
		return nil, fmt.Errorf("invalid project namespace %q: %s", input.Namespace, strings.Join(errs, "; "))
	}
	if errs := validation.IsDNS1123Label(input.GatewayNamespace); len(errs) > 0 {
		return nil, fmt.Errorf("invalid gateway namespace %q: %s", input.GatewayNamespace, strings.Join(errs, "; "))
	}
	blocked, err := normalizeCIDRs(input.BlockedCIDRs)
	if err != nil {
		return nil, err
	}
	nodes, err := normalizeCIDRs(input.NodeAddresses)
	if err != nil {
		return nil, err
	}
	gatewayPorts := []networkingv1.NetworkPolicyPort{}
	for _, port := range sortedUniquePorts(input.GatewayPorts) {
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid gateway port %d", port)
		}
		gatewayPorts = append(gatewayPorts, tcpPort(port))
	}
	if len(gatewayPorts) == 0 {
		return nil, fmt.Errorf("gateway ports are required")
	}
	gatewayPeer := networkingv1.NetworkPolicyPeer{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{namespaceNameLabel: input.GatewayNamespace}}}
	samePeer := networkingv1.NetworkPolicyPeer{PodSelector: &metav1.LabelSelector{}}

	ingressMode := firstNonEmpty(input.IngressMode, NetworkIngressIsolated)
	egressMode := firstNonEmpty(input.EgressMode, NetworkEgressInternet)
	if ingressMode != NetworkIngressIsolated && ingressMode != NetworkIngressDependencies {
		return nil, fmt.Errorf("invalid ingress mode %q", ingressMode)
	}
	if egressMode != NetworkEgressInternet && egressMode != NetworkEgressAllowlist && egressMode != NetworkEgressNone {
		return nil, fmt.Errorf("invalid egress mode %q", egressMode)
	}

	// 宿主机网络组件（kubelet 探针、apiserver proxy）始终可达；同项目空间互访只在 isolated 模式整体放行。
	ingressFrom := []networkingv1.NetworkPolicyPeer{}
	if ingressMode == NetworkIngressIsolated {
		ingressFrom = append(ingressFrom, samePeer)
	}
	for _, cidr := range nodes {
		ingressFrom = append(ingressFrom, networkingv1.NetworkPolicyPeer{IPBlock: &networkingv1.IPBlock{CIDR: cidr}})
	}
	ingress := workloadNetworkPolicy(input.Namespace, workloadIngressPolicyName, metav1.LabelSelector{}, networkingv1.NetworkPolicySpec{
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		Ingress:     ingressRulesFrom(ingressFrom),
	})

	denied := append(append([]string(nil), blocked...), nodes...)
	// DNS 只放行集群 DNS，避免任意地址的 53 端口绕过拒绝网段。
	clusterDNSPeer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{namespaceNameLabel: "kube-system"}},
		PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "kube-dns"}},
	}
	egressRules := []networkingv1.NetworkPolicyEgressRule{
		{To: []networkingv1.NetworkPolicyPeer{clusterDNSPeer}, Ports: []networkingv1.NetworkPolicyPort{udpPort(53), tcpPort(53)}},
		{To: []networkingv1.NetworkPolicyPeer{samePeer}},
	}
	switch egressMode {
	case NetworkEgressInternet:
		egressRules = append(egressRules,
			networkingv1.NetworkPolicyEgressRule{To: []networkingv1.NetworkPolicyPeer{gatewayPeer}, Ports: gatewayPorts},
			networkingv1.NetworkPolicyEgressRule{To: []networkingv1.NetworkPolicyPeer{
				{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0", Except: exceptCIDRs(reservedIPv4CIDRs(), denied, false)}},
				{IPBlock: &networkingv1.IPBlock{CIDR: "::/0", Except: exceptCIDRs(reservedIPv6CIDRs(), denied, true)}},
			}})
	case NetworkEgressAllowlist:
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{To: []networkingv1.NetworkPolicyPeer{gatewayPeer}, Ports: gatewayPorts})
		allowRules, err := renderEgressAllowlist(input.EgressAllowlist, denied)
		if err != nil {
			return nil, err
		}
		egressRules = append(egressRules, allowRules...)
	}
	// 构建与卷迁移 Pod 有各自更严格的出站策略（下方的 luna-build-egress、卷迁移的拒绝全部出站策略）；
	// NetworkPolicy 放行规则取并集，不能再被工作负载策略选中。
	workloadPods := metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
		Key: ScopeLabel, Operator: metav1.LabelSelectorOpNotIn, Values: []string{buildPodScope, volumeTransferScope},
	}}}
	egress := workloadNetworkPolicy(input.Namespace, workloadEgressPolicyName, workloadPods, networkingv1.NetworkPolicySpec{
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		Egress:      egressRules,
	})

	build, err := renderBuildEgressPolicy(input.Namespace, input.BuildEgress, clusterDNSPeer, denied)
	if err != nil {
		return nil, err
	}
	policies := []networkingv1.NetworkPolicy{ingress, egress, build}
	for _, targetID := range sortedUnique(input.TrustedDeploymentTargetIDs) {
		policies = append(policies, workloadNetworkPolicy(input.Namespace, systemEgressPolicyPrefix+shortHash(targetID),
			// 只选中系统组件的工作负载 Pod：同部署配置的构建 Pod 与 Hook Job Pod 不享受全放行。
			metav1.LabelSelector{
				MatchLabels: map[string]string{DeploymentTargetIDLabel: targetID},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: ScopeLabel, Operator: metav1.LabelSelectorOpNotIn, Values: []string{buildPodScope, volumeTransferScope}},
					{Key: HookRunIDLabel, Operator: metav1.LabelSelectorOpDoesNotExist},
				},
			},
			networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				Egress:      []networkingv1.NetworkPolicyEgressRule{{}},
			}))
	}
	exceptionPolicies, err := renderEgressExceptions(input.Namespace, input.EgressExceptions, nodes)
	if err != nil {
		return nil, err
	}
	policies = append(policies, exceptionPolicies...)

	gatewayPods := metav1.LabelSelector{}
	if internal := sortedUnique(input.InternalTargetIDs); len(internal) > 0 {
		gatewayPods.MatchExpressions = []metav1.LabelSelectorRequirement{{Key: DeploymentTargetIDLabel, Operator: metav1.LabelSelectorOpNotIn, Values: internal}}
	}
	policies = append(policies, workloadNetworkPolicy(input.Namespace, gatewayIngressPolicyName, gatewayPods, networkingv1.NetworkPolicySpec{
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		Ingress:     []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{gatewayPeer}}},
	}))
	shared, err := renderSharedServices(input.Namespace, input.SharedIngress, input.SharedEgress)
	if err != nil {
		return nil, err
	}
	policies = append(policies, shared...)
	if ingressMode == NetworkIngressDependencies {
		for _, targetID := range sortedUnique(input.DeploymentTargetIDs) {
			sources := sortedUnique(append([]string{targetID}, input.Dependencies[targetID]...))
			policies = append(policies, workloadNetworkPolicy(input.Namespace, dependencyIngressPolicyPrefix+shortHash(targetID),
				metav1.LabelSelector{MatchLabels: map[string]string{DeploymentTargetIDLabel: targetID}},
				networkingv1.NetworkPolicySpec{
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
					Ingress: []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{{
						PodSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key: DeploymentTargetIDLabel, Operator: metav1.LabelSelectorOpIn, Values: sources,
						}}},
					}}}},
				}))
		}
	}
	return policies, nil
}

// renderSharedServices 渲染跨项目共享的两侧放行：提供方部署配置的 Pod 接受使用方 Namespace 的入站，
// 使用方工作负载 Pod（不含构建与卷迁移）可出站访问提供方部署配置的 Pod。Namespace 只按 Kubernetes
// 自动写入、租户无法修改的 kubernetes.io/metadata.name 标签选择。
func renderSharedServices(namespace string, ingress map[string][]string, egress []SharedService) ([]networkingv1.NetworkPolicy, error) {
	policies := []networkingv1.NetworkPolicy{}
	targetIDs := make([]string, 0, len(ingress))
	for targetID := range ingress {
		targetIDs = append(targetIDs, targetID)
	}
	for _, targetID := range sortedUnique(targetIDs) {
		consumers := sortedUnique(ingress[targetID])
		if len(consumers) == 0 {
			continue
		}
		for _, consumer := range consumers {
			if errs := validation.IsDNS1123Label(consumer); len(errs) > 0 {
				return nil, fmt.Errorf("invalid shared service consumer namespace %q", consumer)
			}
		}
		policies = append(policies, workloadNetworkPolicy(namespace, sharedIngressPolicyPrefix+shortHash(targetID),
			metav1.LabelSelector{MatchLabels: map[string]string{DeploymentTargetIDLabel: targetID}},
			networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				Ingress: []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key: namespaceNameLabel, Operator: metav1.LabelSelectorOpIn, Values: consumers,
					}}},
				}}}},
			}))
	}
	seen := map[string]bool{}
	egress = append([]SharedService(nil), egress...)
	sort.Slice(egress, func(i, j int) bool {
		return egress[i].Namespace+"/"+egress[i].TargetID < egress[j].Namespace+"/"+egress[j].TargetID
	})
	for _, service := range egress {
		key := service.Namespace + "/" + service.TargetID
		if seen[key] {
			continue
		}
		seen[key] = true
		if errs := validation.IsDNS1123Label(service.Namespace); len(errs) > 0 {
			return nil, fmt.Errorf("invalid shared service provider namespace %q", service.Namespace)
		}
		policies = append(policies, workloadNetworkPolicy(namespace, sharedEgressPolicyPrefix+shortHash(key),
			metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: ScopeLabel, Operator: metav1.LabelSelectorOpNotIn, Values: []string{buildPodScope, volumeTransferScope},
			}}},
			networkingv1.NetworkPolicySpec{
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				Egress: []networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{namespaceNameLabel: service.Namespace}},
					PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{DeploymentTargetIDLabel: service.TargetID}},
				}}}},
			}))
	}
	return policies, nil
}

// renderEgressExceptions 为每条授权渲染工作负载（以及按需的构建）出站放行。云元数据与节点地址是平台硬底线：
// 网段内包含它们时排除，网段整体落在它们之内时不下发该条例外。
func renderEgressExceptions(namespace string, exceptions []EgressException, nodes []string) ([]networkingv1.NetworkPolicy, error) {
	hardFloor := append(append([]string(nil), cloudMetadataCIDRs()...), nodes...)
	policies := []networkingv1.NetworkPolicy{}
	for _, exception := range exceptions {
		prefix, err := ParseNetworkCIDR(exception.CIDR)
		if err != nil || prefix.Bits() == 0 {
			return nil, fmt.Errorf("invalid egress exception CIDR %q", exception.CIDR)
		}
		cidr := prefix.String()
		if cidrWithinAny(prefix, hardFloor) {
			continue
		}
		ports, err := exceptionPorts(exception.Ports, exception.Protocol)
		if err != nil {
			return nil, err
		}
		rule := networkingv1.NetworkPolicyEgressRule{
			To:    []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: cidr, Except: containedCIDRs(cidr, hardFloor)}}},
			Ports: ports,
		}
		spec := networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}, Egress: []networkingv1.NetworkPolicyEgressRule{rule}}
		targets := sortedUnique(exception.TargetIDs)
		workloadSelector := metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key: ScopeLabel, Operator: metav1.LabelSelectorOpNotIn, Values: []string{buildPodScope, volumeTransferScope},
		}}}
		buildSelector := metav1.LabelSelector{MatchLabels: map[string]string{ScopeLabel: buildPodScope}}
		if len(targets) > 0 {
			narrow := metav1.LabelSelectorRequirement{Key: DeploymentTargetIDLabel, Operator: metav1.LabelSelectorOpIn, Values: targets}
			workloadSelector.MatchExpressions = append(workloadSelector.MatchExpressions, narrow)
			buildSelector.MatchExpressions = []metav1.LabelSelectorRequirement{narrow}
		}
		policies = append(policies, workloadNetworkPolicy(namespace, egressExceptionPolicyPrefix+shortHash(exception.ID), workloadSelector, spec))
		if exception.IncludeBuilds {
			policies = append(policies, workloadNetworkPolicy(namespace, buildEgressExceptionPolicyPrefix+shortHash(exception.ID), buildSelector, spec))
		}
	}
	return policies, nil
}

// exceptionPorts 按协议渲染端口；未写端口时放行该协议的全部端口，any 且未写端口时放行全部端口和协议。
func exceptionPorts(values []int, protocol string) ([]networkingv1.NetworkPolicyPort, error) {
	var protocols []corev1.Protocol
	switch protocol {
	case EgressExceptionProtocolTCP:
		protocols = []corev1.Protocol{corev1.ProtocolTCP}
	case EgressExceptionProtocolUDP:
		protocols = []corev1.Protocol{corev1.ProtocolUDP}
	case EgressExceptionProtocolAny:
		protocols = []corev1.Protocol{corev1.ProtocolTCP, corev1.ProtocolUDP}
	default:
		return nil, fmt.Errorf("invalid egress exception protocol %q", protocol)
	}
	ports := []networkingv1.NetworkPolicyPort{}
	for _, port := range sortedUniquePorts(values) {
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid egress exception port %d", port)
		}
		for _, value := range protocols {
			value := value
			ports = append(ports, networkingv1.NetworkPolicyPort{Protocol: &value, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: int32(port)}})
		}
	}
	if len(ports) == 0 && protocol != EgressExceptionProtocolAny {
		value := protocols[0]
		ports = append(ports, networkingv1.NetworkPolicyPort{Protocol: &value})
	}
	return ports, nil
}

// renderEgressAllowlist 渲染项目空间出站白名单。整段落在拒绝网段内的条目本就被拒绝，直接跳过；
// 其余条目排除其中的保留网段、拒绝网段、节点地址与云元数据地址。
func renderEgressAllowlist(rules []EgressAllowRule, denied []string) ([]networkingv1.NetworkPolicyEgressRule, error) {
	blocked := append(append(append([]string(nil), reservedIPv4CIDRs()...), reservedIPv6CIDRs()...), denied...)
	blocked = append(blocked, cloudMetadataCIDRs()...)
	result := []networkingv1.NetworkPolicyEgressRule{}
	for _, rule := range rules {
		prefix, err := ParseNetworkCIDR(rule.CIDR)
		if err != nil {
			return nil, fmt.Errorf("invalid egress allowlist CIDR %q: %w", rule.CIDR, err)
		}
		cidr := prefix.String()
		if cidrWithinAny(prefix, blocked) {
			continue
		}
		ports := []networkingv1.NetworkPolicyPort{}
		for _, port := range sortedUniquePorts(rule.Ports) {
			if port < 1 || port > 65535 {
				return nil, fmt.Errorf("invalid egress allowlist port %d", port)
			}
			ports = append(ports, tcpPort(port))
		}
		result = append(result, networkingv1.NetworkPolicyEgressRule{
			To:    []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: cidr, Except: containedCIDRs(cidr, blocked)}}},
			Ports: ports,
		})
	}
	return result, nil
}

// ingressRulesFrom 把来源列表包装成入站规则。from 为空的规则在 Kubernetes 中表示允许所有来源，
// 因此没有来源时不生成规则（等价于拒绝全部入站）。
func ingressRulesFrom(from []networkingv1.NetworkPolicyPeer) []networkingv1.NetworkPolicyIngressRule {
	if len(from) == 0 {
		return nil
	}
	return []networkingv1.NetworkPolicyIngressRule{{From: from}}
}

// WithinPlatformDeniedRange 判断网段是否整体落在平台始终拒绝的私网、保留或元数据网段内；
// 这类出站白名单条目不可能生效，API 应直接拒绝。
func WithinPlatformDeniedRange(prefix netip.Prefix) bool {
	denied := append(append(reservedIPv4CIDRs(), reservedIPv6CIDRs()...), cloudMetadataCIDRs()...)
	return cidrWithinAny(prefix.Masked(), denied)
}

// cidrWithinAny 判断 prefix 是否整体落在任一网段内（含相等）。
func cidrWithinAny(prefix netip.Prefix, values []string) bool {
	for _, value := range values {
		parent, err := netip.ParsePrefix(value)
		if err != nil || parent.Addr().Is6() != prefix.Addr().Is6() {
			continue
		}
		if parent.Bits() <= prefix.Bits() && parent.Contains(prefix.Addr()) {
			return true
		}
	}
	return false
}

func renderBuildEgressPolicy(namespace string, input BuildEgressInput, clusterDNSPeer networkingv1.NetworkPolicyPeer, denied []string) (networkingv1.NetworkPolicy, error) {
	selector := metav1.LabelSelector{MatchLabels: map[string]string{ScopeLabel: buildPodScope}}
	spec := networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}}
	if input.Permissive {
		spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
		return workloadNetworkPolicy(namespace, buildEgressPolicyName, selector, spec), nil
	}
	private, err := normalizeCIDRs(input.PrivateCIDRs)
	if err != nil {
		return networkingv1.NetworkPolicy{}, err
	}
	privatePorts := []networkingv1.NetworkPolicyPort{}
	for _, port := range sortedUniquePorts(input.PrivatePorts) {
		if port < 1 || port > 65535 {
			return networkingv1.NetworkPolicy{}, fmt.Errorf("invalid build private egress port %d", port)
		}
		privatePorts = append(privatePorts, tcpPort(port))
	}
	if len(privatePorts) == 0 {
		privatePorts = []networkingv1.NetworkPolicyPort{tcpPort(443)}
	}
	spec.Egress = []networkingv1.NetworkPolicyEgressRule{
		{To: []networkingv1.NetworkPolicyPeer{clusterDNSPeer}, Ports: []networkingv1.NetworkPolicyPort{udpPort(53), tcpPort(53)}},
		{
			To: []networkingv1.NetworkPolicyPeer{
				{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0", Except: exceptCIDRs(reservedIPv4CIDRs(), denied, false)}},
				{IPBlock: &networkingv1.IPBlock{CIDR: "::/0", Except: exceptCIDRs(reservedIPv6CIDRs(), denied, true)}},
			},
			Ports: []networkingv1.NetworkPolicyPort{tcpPort(80), tcpPort(443)},
		},
	}
	// 私网镜像站只放行指定端口；网段内的拒绝网段、节点地址与云元数据地址仍然拒绝。
	privateDenied := append(append([]string(nil), denied...), cloudMetadataCIDRs()...)
	for _, cidr := range private {
		if covered := metadataExactlyCovered(cidr); covered != "" {
			return networkingv1.NetworkPolicy{}, fmt.Errorf("build private egress CIDR %s covers cloud metadata address %s", cidr, covered)
		}
		spec.Egress = append(spec.Egress, networkingv1.NetworkPolicyEgressRule{
			To:    []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: cidr, Except: containedCIDRs(cidr, privateDenied)}}},
			Ports: privatePorts,
		})
	}
	return workloadNetworkPolicy(namespace, buildEgressPolicyName, selector, spec), nil
}

func workloadNetworkPolicy(namespace, name string, selector metav1.LabelSelector, spec networkingv1.NetworkPolicySpec) networkingv1.NetworkPolicy {
	spec.PodSelector = selector
	return networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      map[string]string{ManagedByLabel: ManagedByValue, ScopeLabel: WorkloadNetworkPolicyScope},
			Annotations: map[string]string{NetworkPolicyHashAnnotation: networkPolicySpecHash(spec)},
		},
		Spec: spec,
	}
}

// SyncWorkloadNetworkPolicies 以期望集合整体覆盖 Namespace 中的受管策略。
// 先创建或覆盖期望策略，再删除不再需要的受管策略：模式切换时新旧放行规则短暂并存（取并集），
// 不会出现两者都不在的断流窗口。
func (c *Client) SyncWorkloadNetworkPolicies(ctx context.Context, namespace string, desired []networkingv1.NetworkPolicy) error {
	existing, err := c.managedWorkloadNetworkPolicies(ctx, namespace)
	if err != nil {
		return err
	}
	wanted := map[string]networkingv1.NetworkPolicy{}
	for _, policy := range desired {
		wanted[policy.Name] = policy
	}
	for _, name := range sortedKeys(wanted) {
		policy := wanted[name]
		// 部署、构建、网关下发与周期修正可能并发同步同一 Namespace：创建撞上已存在或更新冲突时重读后重试。
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return applyWorkloadNetworkPolicy(ctx, c, namespace, policy)
		}); err != nil {
			return err
		}
	}
	policies := c.client.NetworkingV1().NetworkPolicies(namespace)
	for _, name := range sortedKeys(existing) {
		if _, ok := wanted[name]; ok {
			continue
		}
		if err := policies.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func applyWorkloadNetworkPolicy(ctx context.Context, c *Client, namespace string, policy networkingv1.NetworkPolicy) error {
	policies := c.client.NetworkingV1().NetworkPolicies(namespace)
	current, err := policies.Get(ctx, policy.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = policies.Create(ctx, &policy, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			return apierrors.NewConflict(networkingv1.Resource("networkpolicies"), policy.Name, err)
		}
		return err
	}
	if err != nil {
		return err
	}
	if workloadNetworkPolicyMatches(*current, policy) {
		return nil
	}
	current.Labels = mergeStringMaps(current.Labels, policy.Labels)
	current.Annotations = mergeStringMaps(current.Annotations, policy.Annotations)
	current.Spec = policy.Spec
	_, err = policies.Update(ctx, current, metav1.UpdateOptions{})
	return err
}

// WorkloadNetworkPolicyState 实时比对集群中的受管策略与期望集合。
func (c *Client) WorkloadNetworkPolicyState(ctx context.Context, namespace string, desired []networkingv1.NetworkPolicy) (string, error) {
	existing, err := c.managedWorkloadNetworkPolicies(ctx, namespace)
	if err != nil {
		return "", err
	}
	if len(existing) != len(desired) {
		return WorkloadNetworkPolicyDrifted, nil
	}
	for _, policy := range desired {
		current, ok := existing[policy.Name]
		if !ok || !workloadNetworkPolicyMatches(current, policy) {
			return WorkloadNetworkPolicyDrifted, nil
		}
	}
	return WorkloadNetworkPolicySynced, nil
}

func (c *Client) managedWorkloadNetworkPolicies(ctx context.Context, namespace string) (map[string]networkingv1.NetworkPolicy, error) {
	list, err := c.client.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: ManagedByLabel + "=" + ManagedByValue + "," + ScopeLabel + "=" + WorkloadNetworkPolicyScope,
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]networkingv1.NetworkPolicy, len(list.Items))
	for _, item := range list.Items {
		result[item.Name] = item
	}
	return result, nil
}

func workloadNetworkPolicyMatches(current, desired networkingv1.NetworkPolicy) bool {
	return current.Labels[ScopeLabel] == WorkloadNetworkPolicyScope &&
		current.Annotations[NetworkPolicyHashAnnotation] == desired.Annotations[NetworkPolicyHashAnnotation] &&
		apiequality.Semantic.DeepEqual(current.Spec, desired.Spec)
}

// ClusterNodeAddresses 返回 Node InternalIP/ExternalIP 与 kube-apiserver Endpoint 地址（单地址 CIDR）。
func (c *Client) ClusterNodeAddresses(ctx context.Context) ([]string, error) {
	nodes, err := c.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	addresses := []string{}
	for _, node := range nodes.Items {
		for _, address := range node.Status.Addresses {
			if address.Type == "InternalIP" || address.Type == "ExternalIP" {
				addresses = append(addresses, address.Address)
			}
		}
	}
	endpoints, err := c.client.CoreV1().Endpoints("default").Get(ctx, "kubernetes", metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if err == nil {
		for _, subset := range endpoints.Subsets {
			for _, address := range subset.Addresses {
				addresses = append(addresses, address.IP)
			}
		}
	}
	result := make([]string, 0, len(addresses))
	for _, value := range addresses {
		addr, err := netip.ParseAddr(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		result = append(result, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()).String())
	}
	return sortedUnique(result), nil
}

// ListProjectNamespaces 返回平台创建的项目空间 Namespace 及其项目 ID。
func (c *Client) ListProjectNamespaces(ctx context.Context) (map[string]string, error) {
	list, err := c.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: ManagedByLabel + "=" + ManagedByValue + "," + ScopeLabel + "=project",
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(list.Items))
	for _, item := range list.Items {
		if projectID := strings.TrimSpace(item.Labels[ProjectIDLabel]); projectID != "" && item.DeletionTimestamp == nil {
			result[item.Name] = projectID
		}
	}
	return result, nil
}

// ParseNetworkCIDR 解析网络策略使用的 CIDR。拒绝 IPv4 映射的 IPv6 写法（如 ::ffff:0.0.0.0/96）：
// Kubernetes 与多数 CNI 会把它当作 IPv4 网段处理，从而绕过只按 IPv6 比较的保留网段排除。
func ParseNetworkCIDR(value string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
	if err != nil {
		return netip.Prefix{}, err
	}
	if prefix.Addr().Is4In6() || prefix.Addr().Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("CIDR %q must be written as plain IPv4 or IPv6", value)
	}
	return prefix.Masked(), nil
}

func normalizeCIDRs(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := ParseNetworkCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", value, err)
		}
		result = append(result, prefix.String())
	}
	return sortedUnique(result), nil
}

func exceptCIDRs(reserved []string, extra []string, ipv6 bool) []string {
	result := []string{}
	for _, value := range append(append([]string(nil), reserved...), extra...) {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Addr().Is6() != ipv6 || prefix.Bits() == 0 {
			continue
		}
		result = append(result, prefix.Masked().String())
	}
	return sortedUnique(result)
}

func networkPolicySpecHash(spec networkingv1.NetworkPolicySpec) string {
	content, _ := json.Marshal(spec)
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])[:16]
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// containedCIDRs 返回严格落在 parent 内的网段，作为 IPBlock.Except（Kubernetes 要求 except 在 cidr 之内）。
func containedCIDRs(parent string, values []string) []string {
	parentPrefix, err := netip.ParsePrefix(parent)
	if err != nil {
		return nil
	}
	result := []string{}
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Addr().Is6() != parentPrefix.Addr().Is6() || prefix.Bits() <= parentPrefix.Bits() {
			continue
		}
		if parentPrefix.Contains(prefix.Addr()) {
			result = append(result, prefix.Masked().String())
		}
	}
	return sortedUnique(result)
}

// CloudMetadataCIDRs 是云主机元数据服务地址，任何放行规则都不能覆盖。
func CloudMetadataCIDRs() []string { return cloudMetadataCIDRs() }

func cloudMetadataCIDRs() []string {
	return []string{"100.100.100.200/32", "169.254.169.254/32", "fd00:ec2::254/128"}
}

// metadataExactlyCovered 返回与 cidr 完全相同的元数据地址：IPBlock.Except 必须严格小于 cidr，无法排除自身。
func metadataExactlyCovered(cidr string) string {
	for _, value := range cloudMetadataCIDRs() {
		if value == cidr {
			return value
		}
	}
	return ""
}

// reservedIPv4CIDRs 是工作负载与构建默认拒绝的私网与特殊用途网段。
// 198.18.0.0/15 不在其中：fake-ip 代理会把公网域名解析到该网段。
func reservedIPv4CIDRs() []string {
	return []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.168.0.0/16",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
	}
}

// reservedIPv6CIDRs 不包含 ::ffff:0:0/96：Kubernetes 拒绝 IPv4 映射写法的 CIDR，
// 发往映射地址的流量实际是 IPv4 包，由 IPv4 规则约束；用户输入中的映射写法已在入口拒绝。
func reservedIPv6CIDRs() []string {
	return []string{
		"::1/128",
		"::/128",
		"64:ff9b:1::/48",
		"2002::/16",
		"64:ff9b::/96",
		"100::/64",
		"2001:db8::/32",
		"fc00::/7",
		"fe80::/10",
		"ff00::/8",
	}
}

func tcpPort(port int) networkingv1.NetworkPolicyPort {
	protocol := corev1.ProtocolTCP
	return networkingv1.NetworkPolicyPort{Protocol: &protocol, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: int32(port)}}
}

func udpPort(port int) networkingv1.NetworkPolicyPort {
	protocol := corev1.ProtocolUDP
	return networkingv1.NetworkPolicyPort{Protocol: &protocol, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: int32(port)}}
}

func sortedUniquePorts(values []int) []int {
	seen := map[int]bool{}
	result := make([]int, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Ints(result)
	return result
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
