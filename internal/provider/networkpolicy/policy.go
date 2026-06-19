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
	CIDR   string
	Except []string
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

func BuildPolicyWithPrivateEgress(namespace string, privateCIDRs []string) BuildPolicy {
	return BuildPolicyWithEgressControls(namespace, privateCIDRs, nil)
}

func BuildPolicyWithEgressControls(namespace string, privateCIDRs []string, blockedCIDRs []string) BuildPolicy {
	policy := BuildPolicyWithPublicSources(namespace)
	policy.Egress = RestrictedBuildPolicy(namespace).Egress
	policy.Egress = append(policy.Egress, PublicSourceEgressRulesWithBlocked(blockedCIDRs)...)
	policy.Egress = append(policy.Egress, PrivateRegistryEgressRulesWithBlocked(privateCIDRs, blockedCIDRs)...)
	return policy
}

func RestrictedBuildPolicy(namespace string) BuildPolicy {
	return BuildPolicy{
		Name:      "liteyuki-build-egress",
		Namespace: namespace,
		PodLabels: map[string]string{
			"liteyuki.devops/scope": "build",
		},
		Egress: []EgressRule{
			{
				To: []Peer{
					{CIDR: "0.0.0.0/0"},
					{CIDR: "::/0"},
				},
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
				Except: appendCIDRs(reservedIPv4CIDRs(), filterCIDRsForFamily(blockedCIDRs, false)...),
			}},
			Ports: []Port{
				{Protocol: "TCP", Number: 443},
				{Protocol: "TCP", Number: 80},
			},
		},
		{
			To: []Peer{{
				CIDR:   "::/0",
				Except: appendCIDRs(reservedIPv6CIDRs(), filterCIDRsForFamily(blockedCIDRs, true)...),
			}},
			Ports: []Port{
				{Protocol: "TCP", Number: 443},
				{Protocol: "TCP", Number: 80},
			},
		},
	}
}

func PrivateRegistryEgressRules(cidrs []string) []EgressRule {
	return PrivateRegistryEgressRulesWithBlocked(cidrs, nil)
}

func PrivateRegistryEgressRulesWithBlocked(cidrs []string, blockedCIDRs []string) []EgressRule {
	rules := make([]EgressRule, 0, len(cidrs))
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		rules = append(rules, EgressRule{
			To: []Peer{{CIDR: cidr, Except: containedBlockedCIDRs(cidr, blockedCIDRs)}},
			Ports: []Port{
				{Protocol: "TCP", Number: 443},
			},
		})
	}
	return rules
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
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
	}
}

func reservedIPv6CIDRs() []string {
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
