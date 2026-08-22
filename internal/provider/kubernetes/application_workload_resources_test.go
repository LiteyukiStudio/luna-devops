package kubernetes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResourceRequirementsOmitDisabledFields(t *testing.T) {
	resources, err := resourceRequirements(ApplicationResourcesSpec{})
	if err != nil {
		t.Fatalf("resourceRequirements() error = %v", err)
	}
	payload, err := json.Marshal(resources)
	if err != nil {
		t.Fatalf("marshal resources: %v", err)
	}
	if strings.Contains(string(payload), "requests") || strings.Contains(string(payload), "limits") {
		t.Fatalf("disabled resources were serialized: %s", payload)
	}
}

func TestResourceRequirementsRenderPolicyOutput(t *testing.T) {
	resources, err := resourceRequirements(ApplicationResourcesSpec{CPURequest: "100m", MemoryRequest: "256Mi", CPULimit: "1", MemoryLimit: "1Gi"})
	if err != nil {
		t.Fatalf("resourceRequirements() error = %v", err)
	}
	if resources.Requests.Cpu().String() != "100m" || resources.Requests.Memory().String() != "256Mi" ||
		resources.Limits.Cpu().String() != "1" || resources.Limits.Memory().String() != "1Gi" {
		t.Fatalf("resources = %#v", resources)
	}
}
