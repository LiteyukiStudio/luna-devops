package kubernetes

import (
	"context"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/provider/networkpolicy"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureBuildPolicyCreatesDefaultDenyPolicy(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	buildPolicy := networkpolicy.BuildPolicy{
		Name:      "luna-build-egress",
		Namespace: "luna-build",
		PodLabels: map[string]string{
			"luna.devops/scope": "build",
		},
	}

	if err := client.EnsureBuildPolicy(context.Background(), buildPolicy); err != nil {
		t.Fatalf("EnsureBuildPolicy returned error: %v", err)
	}

	policy, err := client.client.NetworkingV1().NetworkPolicies("luna-build").Get(context.Background(), buildPolicy.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if policy.Spec.PodSelector.MatchLabels["luna.devops/scope"] != "build" {
		t.Fatalf("pod selector = %#v", policy.Spec.PodSelector.MatchLabels)
	}
	if len(policy.Spec.Egress) != 0 {
		t.Fatalf("expected default deny egress, got %#v", policy.Spec.Egress)
	}
	if len(policy.Spec.PolicyTypes) != 1 || policy.Spec.PolicyTypes[0] != networkingv1.PolicyTypeEgress {
		t.Fatalf("policy types = %#v", policy.Spec.PolicyTypes)
	}
}

func TestEnsureBuildPolicyUpdatesPolicy(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	buildPolicy := networkpolicy.BuildPolicy{
		Name:      "luna-build-egress",
		Namespace: "luna-build",
		PodLabels: map[string]string{"luna.devops/scope": "build"},
	}
	if err := client.EnsureBuildPolicy(context.Background(), buildPolicy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	buildPolicy.Egress = []networkpolicy.EgressRule{{Ports: []networkpolicy.Port{{Protocol: "TCP", Number: 443}}}}
	if err := client.EnsureBuildPolicy(context.Background(), buildPolicy); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	policy, err := client.client.NetworkingV1().NetworkPolicies("luna-build").Get(context.Background(), buildPolicy.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if len(policy.Spec.Egress) != 1 {
		t.Fatalf("egress = %#v", policy.Spec.Egress)
	}
}

func TestEnsureBuildPolicyTranslatesPublicSourceIPBlocks(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	if err := client.EnsureBuildPolicy(context.Background(), networkpolicy.BuildPolicyWithPublicSources("luna-build")); err != nil {
		t.Fatalf("EnsureBuildPolicy returned error: %v", err)
	}

	policy, err := client.client.NetworkingV1().NetworkPolicies("luna-build").Get(context.Background(), "luna-build-egress", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if len(policy.Spec.Egress) != 3 {
		t.Fatalf("egress = %#v", policy.Spec.Egress)
	}
	publicRule := policy.Spec.Egress[1]
	if publicRule.To[0].IPBlock == nil || publicRule.To[0].IPBlock.CIDR != "0.0.0.0/0" {
		t.Fatalf("public rule peer = %#v", publicRule.To)
	}
	if len(publicRule.To[0].IPBlock.Except) == 0 {
		t.Fatalf("expected CIDR exceptions in public rule")
	}
}
