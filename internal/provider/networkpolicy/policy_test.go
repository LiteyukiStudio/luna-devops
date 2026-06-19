package networkpolicy

import "testing"

func TestBuildPolicyCarriesPodSelector(t *testing.T) {
	policy := BuildPolicy{
		Name:      "liteyuki-build-egress",
		Namespace: "liteyuki-build",
		PodLabels: map[string]string{
			"liteyuki.devops/scope": "build",
		},
	}

	if policy.PodLabels["liteyuki.devops/scope"] != "build" {
		t.Fatalf("pod labels = %#v", policy.PodLabels)
	}
}

func TestRestrictedBuildPolicyAllowsOnlyDNSByDefault(t *testing.T) {
	policy := RestrictedBuildPolicy("liteyuki-build")
	if policy.Name != "liteyuki-build-egress" || policy.Namespace != "liteyuki-build" {
		t.Fatalf("policy = %#v", policy)
	}
	if len(policy.Egress) != 1 || len(policy.Egress[0].Ports) != 2 {
		t.Fatalf("egress = %#v", policy.Egress)
	}
	if policy.Egress[0].Ports[0].Protocol != "UDP" || policy.Egress[0].Ports[0].Number != 53 {
		t.Fatalf("first port = %#v", policy.Egress[0].Ports[0])
	}
	if len(policy.Egress[0].To) != 3 || policy.Egress[0].To[0].CIDR != "0.0.0.0/0" || policy.Egress[0].To[1].CIDR != "::/0" {
		t.Fatalf("dns peers = %#v", policy.Egress[0].To)
	}
	if policy.Egress[0].To[2].NamespaceLabels["kubernetes.io/metadata.name"] != "kube-system" || policy.Egress[0].To[2].PodLabels["k8s-app"] != "kube-dns" {
		t.Fatalf("dns selector peer = %#v", policy.Egress[0].To[2])
	}
}

func TestBuildPolicyWithPublicSourcesAllowsPublicHTTPAndHTTPS(t *testing.T) {
	policy := BuildPolicyWithPublicSources("liteyuki-build")
	if len(policy.Egress) != 3 {
		t.Fatalf("egress = %#v", policy.Egress)
	}
	publicIPv4 := policy.Egress[1]
	if publicIPv4.To[0].CIDR != "0.0.0.0/0" {
		t.Fatalf("public IPv4 peer = %#v", publicIPv4.To)
	}
	if len(publicIPv4.To[0].Except) == 0 {
		t.Fatalf("expected reserved CIDR exceptions")
	}
	if publicIPv4.Ports[0].Number != 443 || publicIPv4.Ports[1].Number != 80 {
		t.Fatalf("ports = %#v", publicIPv4.Ports)
	}
}

func TestPrivateRegistryEgressRulesAllowOnlyTCP443(t *testing.T) {
	rules := PrivateRegistryEgressRules([]string{"10.20.0.0/16", "  ", "fd00::/8"})
	if len(rules) != 2 {
		t.Fatalf("rules = %#v", rules)
	}
	if rules[0].To[0].CIDR != "10.20.0.0/16" {
		t.Fatalf("cidr = %#v", rules[0].To)
	}
	if len(rules[0].Ports) != 1 || rules[0].Ports[0].Protocol != "TCP" || rules[0].Ports[0].Number != 443 {
		t.Fatalf("ports = %#v", rules[0].Ports)
	}
}

func TestBuildPolicyWithPrivateEgressDoesNotAllowPrivateNon443(t *testing.T) {
	policy := BuildPolicyWithPrivateEgress("liteyuki-build", []string{"10.20.0.0/16"})
	for _, rule := range policy.Egress {
		for _, peer := range rule.To {
			if peer.CIDR != "10.20.0.0/16" {
				continue
			}
			for _, port := range rule.Ports {
				if port.Protocol != "TCP" || port.Number != 443 {
					t.Fatalf("private egress must be TCP 443 only, got %#v", port)
				}
			}
		}
	}
}

func TestBuildPolicyWithEgressControlsBlocksMetadataAndServiceCIDR(t *testing.T) {
	policy := BuildPolicyWithEgressControls("liteyuki-build", []string{"10.0.0.0/8"}, []string{"169.254.169.254/32", "10.96.0.0/12"})
	publicIPv4 := policy.Egress[1]
	if !contains(publicIPv4.To[0].Except, "169.254.169.254/32") {
		t.Fatalf("public IPv4 exceptions = %#v", publicIPv4.To[0].Except)
	}

	privateRule := policy.Egress[3]
	if privateRule.To[0].CIDR != "10.0.0.0/8" {
		t.Fatalf("private rule = %#v", privateRule)
	}
	if !contains(privateRule.To[0].Except, "10.96.0.0/12") {
		t.Fatalf("private exceptions = %#v", privateRule.To[0].Except)
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
