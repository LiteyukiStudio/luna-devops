package runtimeapi

import "testing"

func TestGroupWorkloadPodResponses(t *testing.T) {
	items := []clusterResourceResponse{
		{ID: "pod-a", Kind: "Pod", Name: "dplt-alpha-5d7f9", Namespace: "ns-a", DeploymentTargetID: "target-a"},
		{ID: "deploy-a", Kind: "Deployment", Name: "dplt-alpha", Namespace: "ns-a", DeploymentTargetID: "target-a"},
		{ID: "deploy-b", Kind: "Deployment", Name: "dplt-beta", Namespace: "ns-a", DeploymentTargetID: "target-b"},
		{ID: "pod-orphan", Kind: "Pod", Name: "manual-pod", Namespace: "ns-a", DeploymentTargetID: "target-missing"},
	}

	grouped := groupWorkloadPodResponses(items)
	if len(grouped) != 3 {
		t.Fatalf("expected 3 top-level resources, got %d", len(grouped))
	}
	if grouped[0].Kind != "Deployment" || grouped[0].Name != "dplt-alpha" {
		t.Fatalf("expected first top-level resource to be dplt-alpha deployment, got %s/%s", grouped[0].Kind, grouped[0].Name)
	}
	if len(grouped[0].Children) != 1 || grouped[0].Children[0].Name != "dplt-alpha-5d7f9" {
		t.Fatalf("expected dplt-alpha pod child, got %#v", grouped[0].Children)
	}
	if len(grouped[1].Children) != 0 {
		t.Fatalf("expected dplt-beta to have no children, got %#v", grouped[1].Children)
	}
	if grouped[2].Kind != "Pod" || grouped[2].Name != "manual-pod" {
		t.Fatalf("expected unmatched pod to stay top-level, got %s/%s", grouped[2].Kind, grouped[2].Name)
	}
}
