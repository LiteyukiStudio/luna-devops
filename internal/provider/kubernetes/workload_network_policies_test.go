package kubernetes

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func testWorkloadPolicyInput() WorkloadNetworkPolicyInput {
	return WorkloadNetworkPolicyInput{
		Namespace:                  "prj-demo",
		GatewayNamespace:           "kube-system",
		GatewayPorts:               []int{8443, 8080, 8080},
		BlockedCIDRs:               []string{"203.0.113.7/32", "2001:db8:1::/48"},
		NodeAddresses:              []string{"198.51.100.10/32"},
		TrustedDeploymentTargetIDs: []string{"dt_probe"},
	}
}

func TestRenderWorkloadNetworkPoliciesIsolatesProjectAndBlocksPrivateEgress(t *testing.T) {
	policies, err := RenderWorkloadNetworkPolicies(testWorkloadPolicyInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 5 {
		t.Fatalf("policy count = %d", len(policies))
	}
	ingress := policies[0]
	if ingress.Name != workloadIngressPolicyName || len(ingress.Spec.PodSelector.MatchLabels)+len(ingress.Spec.PodSelector.MatchExpressions) != 0 {
		t.Fatalf("ingress policy must select every pod: %#v", ingress.Spec.PodSelector)
	}
	from := ingress.Spec.Ingress[0].From
	if from[0].PodSelector == nil || from[0].NamespaceSelector != nil {
		t.Fatalf("first ingress peer must be same namespace: %#v", from[0])
	}
	if from[1].IPBlock == nil || from[1].IPBlock.CIDR != "198.51.100.10/32" {
		t.Fatalf("node ingress peer = %#v", from[1])
	}
	gatewayIngress := policies[4]
	if gatewayIngress.Name != gatewayIngressPolicyName || gatewayIngress.Spec.Ingress[0].From[0].NamespaceSelector.MatchLabels[namespaceNameLabel] != "kube-system" {
		t.Fatalf("gateway ingress policy = %#v", gatewayIngress.Spec)
	}

	egress := policies[1]
	expr := egress.Spec.PodSelector.MatchExpressions
	if len(expr) != 1 || expr[0].Operator != metav1.LabelSelectorOpNotIn || !slices.Equal(expr[0].Values, []string{"build", volumeTransferScope}) {
		t.Fatalf("egress policy must exclude build and volume transfer pods: %#v", expr)
	}
	dns := egress.Spec.Egress[0]
	if len(dns.To) != 1 || dns.To[0].PodSelector.MatchLabels["k8s-app"] != "kube-dns" || dns.To[0].NamespaceSelector.MatchLabels[namespaceNameLabel] != "kube-system" {
		t.Fatalf("DNS egress must target cluster DNS only: %#v", dns)
	}
	gateway := egress.Spec.Egress[2]
	if len(gateway.Ports) != 2 || gateway.Ports[0].Port.IntVal != 8080 || gateway.Ports[1].Port.IntVal != 8443 {
		t.Fatalf("gateway egress must be limited to listener ports: %#v", gateway.Ports)
	}
	for _, rule := range egress.Spec.Egress {
		if len(rule.To) == 0 {
			t.Fatalf("egress rule without destination allows every address: %#v", rule)
		}
	}
	public := egress.Spec.Egress[3].To
	if slices.Contains(public[0].IPBlock.Except, "198.18.0.0/15") {
		t.Fatalf("198.18.0.0/15 must not be denied by default (fake-ip DNS resolves public names there)")
	}
	for _, want := range []string{"10.0.0.0/8", "169.254.0.0/16", "192.168.0.0/16", "203.0.113.7/32", "198.51.100.10/32"} {
		if !slices.Contains(public[0].IPBlock.Except, want) {
			t.Fatalf("IPv4 except missing %s: %v", want, public[0].IPBlock.Except)
		}
	}
	for _, want := range []string{"fc00::/7", "2001:db8:1::/48"} {
		if !slices.Contains(public[1].IPBlock.Except, want) {
			t.Fatalf("IPv6 except missing %s: %v", want, public[1].IPBlock.Except)
		}
	}
	if slices.Contains(public[0].IPBlock.Except, "2001:db8:1::/48") {
		t.Fatalf("IPv4 except must not contain IPv6 ranges")
	}
	system := policies[3]
	if system.Spec.PodSelector.MatchLabels[DeploymentTargetIDLabel] != "dt_probe" || len(system.Spec.Egress) != 1 || len(system.Spec.Egress[0].To) != 0 {
		t.Fatalf("trusted system egress = %#v", system.Spec)
	}

	again, _ := RenderWorkloadNetworkPolicies(testWorkloadPolicyInput())
	if again[1].Annotations[NetworkPolicyHashAnnotation] != egress.Annotations[NetworkPolicyHashAnnotation] {
		t.Fatalf("render must be deterministic")
	}
}

func TestRenderWorkloadNetworkPoliciesRejectsInvalidInput(t *testing.T) {
	input := testWorkloadPolicyInput()
	input.BlockedCIDRs = []string{"not-a-cidr"}
	if _, err := RenderWorkloadNetworkPolicies(input); err == nil {
		t.Fatal("invalid CIDR should fail")
	}
	input = testWorkloadPolicyInput()
	input.GatewayNamespace = ""
	if _, err := RenderWorkloadNetworkPolicies(input); err == nil {
		t.Fatal("missing gateway namespace should fail")
	}
	input = testWorkloadPolicyInput()
	input.GatewayPorts = nil
	if _, err := RenderWorkloadNetworkPolicies(input); err == nil {
		t.Fatal("missing gateway ports should fail")
	}
}

func TestSyncWorkloadNetworkPoliciesRepairsDriftAndRemovesStale(t *testing.T) {
	stale := networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name: systemEgressPolicyPrefix + "old", Namespace: "prj-demo",
		Labels: map[string]string{ManagedByLabel: ManagedByValue, ScopeLabel: WorkloadNetworkPolicyScope},
	}}
	foreign := networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "user-policy", Namespace: "prj-demo"}}
	client := NewClientForInterface(fake.NewSimpleClientset(&stale, &foreign))
	ctx := context.Background()
	desired, err := RenderWorkloadNetworkPolicies(testWorkloadPolicyInput())
	if err != nil {
		t.Fatal(err)
	}

	if state, _ := client.WorkloadNetworkPolicyState(ctx, "prj-demo", desired); state != WorkloadNetworkPolicyDrifted {
		t.Fatalf("state before sync = %s", state)
	}
	if err := client.SyncWorkloadNetworkPolicies(ctx, "prj-demo", desired); err != nil {
		t.Fatal(err)
	}
	if state, _ := client.WorkloadNetworkPolicyState(ctx, "prj-demo", desired); state != WorkloadNetworkPolicySynced {
		t.Fatalf("state after sync = %s", state)
	}
	policies := client.client.NetworkingV1().NetworkPolicies("prj-demo")
	if _, err := policies.Get(ctx, stale.Name, metav1.GetOptions{}); err == nil {
		t.Fatal("stale managed policy must be deleted")
	}
	if _, err := policies.Get(ctx, foreign.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("unmanaged policy must be kept: %v", err)
	}

	// 手动修改受管策略（放开所有出站）会被识别为漂移并被改回。
	current, _ := policies.Get(ctx, workloadEgressPolicyName, metav1.GetOptions{})
	current.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
	if _, err := policies.Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if state, _ := client.WorkloadNetworkPolicyState(ctx, "prj-demo", desired); state != WorkloadNetworkPolicyDrifted {
		t.Fatalf("manual edit must be drift, got %s", state)
	}
	if err := client.SyncWorkloadNetworkPolicies(ctx, "prj-demo", desired); err != nil {
		t.Fatal(err)
	}
	repaired, _ := policies.Get(ctx, workloadEgressPolicyName, metav1.GetOptions{})
	if len(repaired.Spec.Egress) != 4 {
		t.Fatalf("drift not repaired: %#v", repaired.Spec.Egress)
	}
}

func TestClusterNodeAddressesAndProjectNamespaces(t *testing.T) {
	node := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{
		{Type: corev1.NodeInternalIP, Address: "192.168.31.20"},
		{Type: corev1.NodeExternalIP, Address: "198.51.100.10"},
		{Type: corev1.NodeHostName, Address: "n1"},
	}}}
	endpoints := corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "kubernetes", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "203.0.113.5"}}}}}
	project := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prj-demo", Labels: ProjectNamespaceLabels("prj_demo")}}
	other := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	client := NewClientForInterface(fake.NewSimpleClientset(&node, &endpoints, &project, &other))

	addresses, err := client.ClusterNodeAddresses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(addresses, []string{"192.168.31.20/32", "198.51.100.10/32", "203.0.113.5/32"}) {
		t.Fatalf("addresses = %v", addresses)
	}
	namespaces, err := client.ListProjectNamespaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(namespaces) != 1 || namespaces["prj-demo"] != "prj_demo" {
		t.Fatalf("project namespaces = %v", namespaces)
	}
}

func TestSyncWorkloadNetworkPoliciesToleratesConcurrentCreate(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	created := false
	// 模拟另一个同步者在本次 Get 之后抢先创建同名策略。
	clientset.PrependReactor("create", "networkpolicies", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if created {
			return false, nil, nil
		}
		created = true
		policy := action.(k8stesting.CreateAction).GetObject().(*networkingv1.NetworkPolicy).DeepCopy()
		policy.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
		if err := clientset.Tracker().Create(networkingv1.SchemeGroupVersion.WithResource("networkpolicies"), policy, policy.Namespace); err != nil {
			return true, nil, err
		}
		return true, nil, apierrors.NewAlreadyExists(networkingv1.Resource("networkpolicies"), policy.Name)
	})
	client := NewClientForInterface(clientset)
	desired, err := RenderWorkloadNetworkPolicies(testWorkloadPolicyInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SyncWorkloadNetworkPolicies(context.Background(), "prj-demo", desired); err != nil {
		t.Fatalf("concurrent create must be retried, got %v", err)
	}
	if state, _ := client.WorkloadNetworkPolicyState(context.Background(), "prj-demo", desired); state != WorkloadNetworkPolicySynced {
		t.Fatalf("state after retry = %s", state)
	}
}

func TestRenderBuildEgressPolicy(t *testing.T) {
	input := testWorkloadPolicyInput()
	input.BlockedCIDRs = []string{"10.20.5.0/24"}
	input.NodeAddresses = []string{"10.20.0.10/32", "198.51.100.10/32"}
	input.BuildEgress = BuildEgressInput{PrivateCIDRs: []string{"10.20.0.0/16"}, PrivatePorts: []int{5000, 443}}
	policies, err := RenderWorkloadNetworkPolicies(input)
	if err != nil {
		t.Fatal(err)
	}
	build := policies[2]
	if build.Name != buildEgressPolicyName || build.Spec.PodSelector.MatchLabels[ScopeLabel] != "build" || build.Labels[ScopeLabel] != WorkloadNetworkPolicyScope {
		t.Fatalf("build policy identity = %#v", build.ObjectMeta)
	}
	rules := build.Spec.Egress
	if len(rules) != 3 {
		t.Fatalf("expected DNS, public HTTP(S) and private registry rules, got %#v", rules)
	}
	if rules[0].To[0].PodSelector.MatchLabels["k8s-app"] != "kube-dns" {
		t.Fatalf("build DNS must target cluster DNS: %#v", rules[0])
	}
	if len(rules[1].Ports) != 2 || rules[1].Ports[0].Port.IntVal != 80 || rules[1].Ports[1].Port.IntVal != 443 {
		t.Fatalf("public build egress must be HTTP(S) only: %#v", rules[1].Ports)
	}
	if !slices.Contains(rules[1].To[0].IPBlock.Except, "198.51.100.10/32") {
		t.Fatalf("public build egress must deny node addresses: %v", rules[1].To[0].IPBlock.Except)
	}
	private := rules[2]
	if private.To[0].IPBlock.CIDR != "10.20.0.0/16" || !slices.Equal(private.To[0].IPBlock.Except, []string{"10.20.0.10/32", "10.20.5.0/24"}) {
		t.Fatalf("private registry rule must keep contained denied ranges: %#v", private.To[0].IPBlock)
	}
	if len(private.Ports) != 2 || private.Ports[0].Port.IntVal != 443 || private.Ports[1].Port.IntVal != 5000 {
		t.Fatalf("private registry ports = %#v", private.Ports)
	}

	input.BuildEgress = BuildEgressInput{Permissive: true}
	policies, err = RenderWorkloadNetworkPolicies(input)
	if err != nil {
		t.Fatal(err)
	}
	if egress := policies[2].Spec.Egress; len(egress) != 1 || len(egress[0].To) != 0 {
		t.Fatalf("permissive build egress must allow all: %#v", egress)
	}
}

func TestSyncTakesOverLegacyBuildPolicy(t *testing.T) {
	legacy := networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name: buildEgressPolicyName, Namespace: "prj-demo",
		Labels: map[string]string{ManagedByLabel: ManagedByValue, ScopeLabel: "build"},
	}, Spec: networkingv1.NetworkPolicySpec{Egress: []networkingv1.NetworkPolicyEgressRule{{}}}}
	client := NewClientForInterface(fake.NewSimpleClientset(&legacy))
	desired, err := RenderWorkloadNetworkPolicies(testWorkloadPolicyInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SyncWorkloadNetworkPolicies(context.Background(), "prj-demo", desired); err != nil {
		t.Fatal(err)
	}
	if state, _ := client.WorkloadNetworkPolicyState(context.Background(), "prj-demo", desired); state != WorkloadNetworkPolicySynced {
		t.Fatalf("legacy build policy must be taken over, state = %s", state)
	}
}

func TestBuildPrivateEgressAlwaysExcludesCloudMetadata(t *testing.T) {
	input := testWorkloadPolicyInput()
	input.BuildEgress = BuildEgressInput{PrivateCIDRs: []string{"169.254.0.0/16", "100.64.0.0/10", "fd00::/8"}, PrivatePorts: []int{80}}
	policies, err := RenderWorkloadNetworkPolicies(input)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"169.254.0.0/16": "169.254.169.254/32", "100.64.0.0/10": "100.100.100.200/32", "fd00::/8": "fd00:ec2::254/128"}
	for _, rule := range policies[2].Spec.Egress[2:] {
		block := rule.To[0].IPBlock
		if !slices.Contains(block.Except, want[block.CIDR]) {
			t.Fatalf("private rule %s must exclude metadata %s: %v", block.CIDR, want[block.CIDR], block.Except)
		}
	}
	input.BuildEgress = BuildEgressInput{PrivateCIDRs: []string{"169.254.169.254/32"}}
	if _, err := RenderWorkloadNetworkPolicies(input); err == nil {
		t.Fatal("a private rule equal to a metadata address must fail closed")
	}
}

func TestTrustedSystemEgressExcludesBuildAndHookPods(t *testing.T) {
	policies, err := RenderWorkloadNetworkPolicies(testWorkloadPolicyInput())
	if err != nil {
		t.Fatal(err)
	}
	selector := policies[3].Spec.PodSelector
	if len(selector.MatchExpressions) != 2 ||
		selector.MatchExpressions[0].Key != ScopeLabel || !slices.Contains(selector.MatchExpressions[0].Values, "build") ||
		selector.MatchExpressions[1].Key != HookRunIDLabel || selector.MatchExpressions[1].Operator != metav1.LabelSelectorOpDoesNotExist {
		t.Fatalf("trusted system egress selector = %#v", selector)
	}
}

func TestRenderDependenciesIngressAndInternalTargets(t *testing.T) {
	input := testWorkloadPolicyInput()
	input.IngressMode = NetworkIngressDependencies
	input.DeploymentTargetIDs = []string{"dt_web", "dt_db"}
	input.Dependencies = map[string][]string{"dt_db": {"dt_web"}}
	input.InternalTargetIDs = []string{"dt_db"}
	policies, err := RenderWorkloadNetworkPolicies(input)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]networkingv1.NetworkPolicy{}
	for _, policy := range policies {
		byName[policy.Name] = policy
	}
	base := byName[workloadIngressPolicyName]
	for _, peer := range base.Spec.Ingress[0].From {
		if peer.PodSelector != nil {
			t.Fatalf("dependencies mode must not allow the whole namespace: %#v", base.Spec.Ingress)
		}
	}
	db := byName[dependencyIngressPolicyPrefix+shortHash("dt_db")]
	if db.Spec.PodSelector.MatchLabels[DeploymentTargetIDLabel] != "dt_db" ||
		!slices.Equal(db.Spec.Ingress[0].From[0].PodSelector.MatchExpressions[0].Values, []string{"dt_db", "dt_web"}) {
		t.Fatalf("db must accept only itself and its service binding source: %#v", db.Spec)
	}
	web := byName[dependencyIngressPolicyPrefix+shortHash("dt_web")]
	if !slices.Equal(web.Spec.Ingress[0].From[0].PodSelector.MatchExpressions[0].Values, []string{"dt_web"}) {
		t.Fatalf("web without bindings must accept only its own replicas: %#v", web.Spec)
	}
	gateway := byName[gatewayIngressPolicyName]
	expr := gateway.Spec.PodSelector.MatchExpressions
	if len(expr) != 1 || expr[0].Operator != metav1.LabelSelectorOpNotIn || !slices.Equal(expr[0].Values, []string{"dt_db"}) {
		t.Fatalf("internal targets must not accept gateway traffic: %#v", gateway.Spec.PodSelector)
	}
}

func TestRenderDependenciesWithoutNodesDeniesInsteadOfAllowingAll(t *testing.T) {
	input := testWorkloadPolicyInput()
	input.IngressMode = NetworkIngressDependencies
	input.NodeAddresses = nil
	policies, err := RenderWorkloadNetworkPolicies(input)
	if err != nil {
		t.Fatal(err)
	}
	if rules := policies[0].Spec.Ingress; len(rules) != 0 {
		t.Fatalf("an ingress rule without sources would allow everyone: %#v", rules)
	}
}

func TestRenderEgressModes(t *testing.T) {
	input := testWorkloadPolicyInput()
	input.EgressMode = NetworkEgressNone
	policies, err := RenderWorkloadNetworkPolicies(input)
	if err != nil {
		t.Fatal(err)
	}
	if rules := policies[1].Spec.Egress; len(rules) != 2 {
		t.Fatalf("none mode must allow only cluster DNS and the same namespace: %#v", rules)
	}

	input.EgressMode = NetworkEgressAllowlist
	input.EgressAllowlist = []EgressAllowRule{
		{CIDR: "1.1.1.0/24", Ports: []int{443}},
		{CIDR: "10.1.0.0/16"},
		{CIDR: "0.0.0.0/1"},
	}
	policies, err = RenderWorkloadNetworkPolicies(input)
	if err != nil {
		t.Fatal(err)
	}
	rules := policies[1].Spec.Egress
	if len(rules) != 5 {
		t.Fatalf("allowlist mode = DNS, same namespace, gateway and two public entries, got %#v", rules)
	}
	if rules[3].To[0].IPBlock.CIDR != "1.1.1.0/24" || len(rules[3].Ports) != 1 || rules[3].Ports[0].Port.IntVal != 443 {
		t.Fatalf("allowlist entry = %#v", rules[3])
	}
	wide := rules[4].To[0].IPBlock
	if wide.CIDR != "0.0.0.0/1" || !slices.Contains(wide.Except, "10.0.0.0/8") || !slices.Contains(wide.Except, "100.100.100.200/32") || len(rules[4].Ports) != 0 {
		t.Fatalf("wide allowlist entry must exclude private and metadata ranges: %#v", rules[4])
	}

	input.EgressMode = "everything"
	if _, err := RenderWorkloadNetworkPolicies(input); err == nil {
		t.Fatal("unknown egress mode must fail")
	}
}
