package networkpolicy

import (
	"context"
	"net/netip"
	"strings"
)

type Provider interface {
	EnsureBuildPolicy(ctx context.Context, policy BuildPolicy) error
}

type BuildPolicy struct {
	Name      string
	Namespace string
	PodLabels map[string]string
	Egress    []EgressRule
}

type EgressRule struct {
	To    []Peer
	Ports []Port
}

type Peer struct {
	CIDR            string
	Except          []string
	NamespaceLabels map[string]string
	PodLabels       map[string]string
}

type Port struct {
	Protocol string
	Number   int
}

func BuildPolicyWithPublicSources(namespace string) BuildPolicy {
	policy := RestrictedBuildPolicy(namespace)
	policy.Egress = append(policy.Egress, PublicSourceEgressRules()...)
	return policy
}

func PermissiveBuildPolicy(namespace string) BuildPolicy {
	return BuildPolicy{
		Name:      "luna-build-egress",
		Namespace: namespace,
		PodLabels: map[string]string{
			"luna.devops/scope": "build",
		},
		Egress: []EgressRule{{}},
	}
}

func BuildPolicyWithPrivateEgress(namespace string, privateCIDRs []string) BuildPolicy {
	return BuildPolicyWithPrivateEgressPorts(namespace, privateCIDRs, nil)
}

func BuildPolicyWithPrivateEgressPorts(namespace string, privateCIDRs []string, privatePorts []int) BuildPolicy {
	return BuildPolicyWithEgressControlsAndPorts(namespace, privateCIDRs, privatePorts, nil)
}

func BuildPolicyWithEgressControls(namespace string, privateCIDRs []string, blockedCIDRs []string) BuildPolicy {
	return BuildPolicyWithEgressControlsAndPorts(namespace, privateCIDRs, nil, blockedCIDRs)
}

func BuildPolicyWithEgressControlsAndPorts(namespace string, privateCIDRs []string, privatePorts []int, blockedCIDRs []string) BuildPolicy {
	policy := BuildPolicyWithPublicSources(namespace)
	policy.Egress = RestrictedBuildPolicy(namespace).Egress
	policy.Egress = append(policy.Egress, PublicSourceEgressRulesWithBlocked(blockedCIDRs)...)
	policy.Egress = append(policy.Egress, PrivateRegistryEgressRulesWithPortsAndBlocked(privateCIDRs, privatePorts, blockedCIDRs)...)
	return policy
}

func RestrictedBuildPolicy(namespace string) BuildPolicy {
	return BuildPolicy{
		Name:      "luna-build-egress",
		Namespace: namespace,
		PodLabels: map[string]string{
			"luna.devops/scope": "build",
		},
		Egress: []EgressRule{
			{
				Ports: []Port{
					{Protocol: "UDP", Number: 53},
					{Protocol: "TCP", Number: 53},
				},
			},
		},
	}
}

func PublicSourceEgressRules() []EgressRule {
	return PublicSourceEgressRulesWithBlocked(nil)
}

func PublicSourceEgressRulesWithBlocked(blockedCIDRs []string) []EgressRule {
	return []EgressRule{
		{
			To: []Peer{{
				CIDR:   "0.0.0.0/0",
				Except: appendCIDRs(ReservedIPv4CIDRs(), filterCIDRsForFamily(blockedCIDRs, false)...),
			}},
			Ports: []Port{
				{Protocol: "TCP", Number: 443},
				{Protocol: "TCP", Number: 80},
			},
		},
		{
			To: []Peer{{
				CIDR:   "::/0",
				Except: appendCIDRs(ReservedIPv6CIDRs(), filterCIDRsForFamily(blockedCIDRs, true)...),
			}},
			Ports: []Port{
				{Protocol: "TCP", Number: 443},
				{Protocol: "TCP", Number: 80},
			},
		},
	}
}

func PrivateRegistryEgressRules(cidrs []string) []EgressRule {
	return PrivateRegistryEgressRulesWithPorts(cidrs, nil)
}

func PrivateRegistryEgressRulesWithPorts(cidrs []string, privatePorts []int) []EgressRule {
	return PrivateRegistryEgressRulesWithPortsAndBlocked(cidrs, privatePorts, nil)
}

func PrivateRegistryEgressRulesWithBlocked(cidrs []string, blockedCIDRs []string) []EgressRule {
	return PrivateRegistryEgressRulesWithPortsAndBlocked(cidrs, nil, blockedCIDRs)
}

func PrivateRegistryEgressRulesWithPortsAndBlocked(cidrs []string, privatePorts []int, blockedCIDRs []string) []EgressRule {
	rules := make([]EgressRule, 0, len(cidrs))
	ports := privateEgressPorts(privatePorts)
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		rules = append(rules, EgressRule{
			To:    []Peer{{CIDR: cidr, Except: containedBlockedCIDRs(cidr, blockedCIDRs)}},
			Ports: ports,
		})
	}
	return rules
}

func privateEgressPorts(values []int) []Port {
	seen := map[int]bool{}
	ports := make([]Port, 0, len(values))
	for _, value := range values {
		if value < 1 || value > 65535 || seen[value] {
			continue
		}
		seen[value] = true
		ports = append(ports, Port{Protocol: "TCP", Number: value})
	}
	if len(ports) == 0 {
		return []Port{{Protocol: "TCP", Number: 443}}
	}
	return ports
}

func appendCIDRs(base []string, extra ...string) []string {
	result := append([]string(nil), base...)
	result = append(result, extra...)
	return result
}

func filterCIDRsForFamily(cidrs []string, ipv6 bool) []string {
	result := make([]string, 0, len(cidrs))
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
		if err != nil {
			continue
		}
		if prefix.Addr().Is6() == ipv6 {
			result = append(result, prefix.String())
		}
	}
	return result
}

func containedBlockedCIDRs(cidr string, blockedCIDRs []string) []string {
	parent, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(blockedCIDRs))
	for _, blocked := range blockedCIDRs {
		child, err := netip.ParsePrefix(strings.TrimSpace(blocked))
		if err != nil || child.Addr().Is6() != parent.Addr().Is6() {
			continue
		}
		if parent.Contains(child.Addr()) {
			result = append(result, child.String())
		}
	}
	return result
}

func ReservedIPv4CIDRs() []string {
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
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
	}
}

func ReservedIPv6CIDRs() []string {
	return []string{
		"::1/128",
		"::/128",
		"64:ff9b::/96",
		"100::/64",
		"2001:db8::/32",
		"fc00::/7",
		"fe80::/10",
		"ff00::/8",
	}
}
