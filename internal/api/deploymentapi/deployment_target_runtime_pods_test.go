package deploymentapi

import (
	"testing"
	"time"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func runtimePodNames(pods []kubeprovider.RuntimePodSnapshot) []string {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return names
}

func TestSortRuntimePodSnapshotsKeepsRecommendedOrderByDefault(t *testing.T) {
	now := time.Now()
	recommended := []kubeprovider.RuntimePodSnapshot{
		{Name: "api-c", CreatedAt: now.Add(-time.Hour)},
		{Name: "api-a", CreatedAt: now},
		{Name: "api-b", CreatedAt: now.Add(-2 * time.Hour)},
	}
	tests := []struct {
		sortBy, sortOrder string
		want              []string
	}{
		{"recommended", "desc", []string{"api-c", "api-a", "api-b"}},
		{"recommended", "asc", []string{"api-b", "api-a", "api-c"}},
		{"name", "asc", []string{"api-a", "api-b", "api-c"}},
		{"createdAt", "desc", []string{"api-a", "api-c", "api-b"}},
	}
	for _, test := range tests {
		pods := append([]kubeprovider.RuntimePodSnapshot(nil), recommended...)
		sortRuntimePodSnapshots(pods, paginationParams{Page: 1, PageSize: 20, SortBy: test.sortBy, SortOrder: test.sortOrder})
		got := runtimePodNames(pods)
		for index := range test.want {
			if got[index] != test.want[index] {
				t.Fatalf("sort %s %s = %v, want %v", test.sortBy, test.sortOrder, got, test.want)
			}
		}
	}
}

func TestFilterRuntimePodsMatchesNameCaseInsensitively(t *testing.T) {
	pods := filterRuntimePods([]kubeprovider.RuntimePodSnapshot{{Name: "API-7f9c-a"}, {Name: "worker-1"}}, " api ")
	if names := runtimePodNames(pods); len(names) != 1 || names[0] != "API-7f9c-a" {
		t.Fatalf("filtered pods = %v", names)
	}
}

func TestRuntimeTerminalAuditTargetRecordsSelectedPod(t *testing.T) {
	cases := map[string][2]string{
		"":                           {"", ""},
		"app":                        {"", "app"},
		"pod=api-a":                  {"api-a", ""},
		"pod=api-a container=worker": {"api-a", "worker"},
	}
	for want, input := range cases {
		if got := runtimeTerminalAuditTarget(input[0], input[1]); got != want {
			t.Fatalf("runtimeTerminalAuditTarget(%q, %q) = %q, want %q", input[0], input[1], got, want)
		}
	}
}
