package runtimeapi

import (
	"fmt"
	"testing"
)

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

func TestClusterResourcePaginatedResponseUsesCompleteSortedSet(t *testing.T) {
	filteredItems := make([]clusterResourceResponse, 0, 25)
	for index := 24; index >= 0; index-- {
		filteredItems = append(filteredItems, clusterResourceResponse{Name: fmt.Sprintf("resource-%02d", index)})
	}

	response := clusterResourcePaginatedResponse(filteredItems, paginationParams{
		Page:      2,
		PageSize:  10,
		SortBy:    "name",
		SortOrder: "asc",
	})

	if response.Page != 2 || response.Total != 25 || response.TotalPages != 3 {
		t.Fatalf("pagination = page %d, total %d, totalPages %d", response.Page, response.Total, response.TotalPages)
	}
	if len(response.Items) != 10 || response.Items[0].Name != "resource-10" || response.Items[9].Name != "resource-19" {
		t.Fatalf("second page = %#v", response.Items)
	}
}
