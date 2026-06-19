package kubernetes

import (
	"context"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/provider/networkpolicy"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureBuildNetworkPolicyCreatesDefaultDenyPolicy(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := BuildNetworkPolicySpec{
		Name:      "liteyuki-build-egress",
		Namespace: "liteyuki-build",
		Labels: map[string]string{
			"liteyuki.devops/scope": "build",
		},
	}

	if err := client.EnsureBuildNetworkPolicy(context.Background(), spec); err != nil {
		t.Fatalf("EnsureBuildNetworkPolicy returned error: %v", err)
	}

	policy, err := client.client.NetworkingV1().NetworkPolicies("liteyuki-build").Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if policy.Spec.PodSelector.MatchLabels["liteyuki.devops/scope"] != "build" {
		t.Fatalf("pod selector = %#v", policy.Spec.PodSelector.MatchLabels)
	}
	if len(policy.Spec.Egress) != 0 {
		t.Fatalf("expected default deny egress, got %#v", policy.Spec.Egress)
	}
	if len(policy.Spec.PolicyTypes) != 1 || policy.Spec.PolicyTypes[0] != networkingv1.PolicyTypeEgress {
		t.Fatalf("policy types = %#v", policy.Spec.PolicyTypes)
	}
}

func TestEnsureBuildNetworkPolicyUpdatesPolicy(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := BuildNetworkPolicySpec{
		Name:      "liteyuki-build-egress",
		Namespace: "liteyuki-build",
		Labels:    map[string]string{"liteyuki.devops/scope": "build"},
	}
	if err := client.EnsureBuildNetworkPolicy(context.Background(), spec); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	spec.Egress = []networkingv1.NetworkPolicyEgressRule{{Ports: []networkingv1.NetworkPolicyPort{TCPPort(443)}}}
	if err := client.EnsureBuildNetworkPolicy(context.Background(), spec); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	policy, err := client.client.NetworkingV1().NetworkPolicies("liteyuki-build").Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if len(policy.Spec.Egress) != 1 {
		t.Fatalf("egress = %#v", policy.Spec.Egress)
	}
}

func TestEnsureBuildPolicyTranslatesPublicSourceIPBlocks(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	if err := client.EnsureBuildPolicy(context.Background(), networkpolicy.BuildPolicyWithPublicSources("liteyuki-build")); err != nil {
		t.Fatalf("EnsureBuildPolicy returned error: %v", err)
	}

	policy, err := client.client.NetworkingV1().NetworkPolicies("liteyuki-build").Get(context.Background(), "liteyuki-build-egress", metav1.GetOptions{})
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

func TestEnsureBuildPolicyTranslatesDNSSelectorPeer(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	if err := client.EnsureBuildPolicy(context.Background(), networkpolicy.RestrictedBuildPolicy("liteyuki-build")); err != nil {
		t.Fatalf("EnsureBuildPolicy returned error: %v", err)
	}

	policy, err := client.client.NetworkingV1().NetworkPolicies("liteyuki-build").Get(context.Background(), "liteyuki-build-egress", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if len(policy.Spec.Egress) != 1 || len(policy.Spec.Egress[0].To) != 3 {
		t.Fatalf("dns egress = %#v", policy.Spec.Egress)
	}
	peer := policy.Spec.Egress[0].To[2]
	if peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "kube-system" {
		t.Fatalf("namespace selector = %#v", peer.NamespaceSelector)
	}
	if peer.PodSelector == nil || peer.PodSelector.MatchLabels["k8s-app"] != "kube-dns" {
		t.Fatalf("pod selector = %#v", peer.PodSelector)
	}
}
