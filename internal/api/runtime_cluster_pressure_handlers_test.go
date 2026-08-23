package api

import (
	"testing"
	"time"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestRuntimeClusterPressureScoreAndLevels(t *testing.T) {
	cpuUsage := 80.0
	memoryUsage := 60.0
	if score := runtimeClusterPressureScore(50, 70, &cpuUsage, &memoryUsage); score != 72 {
		t.Fatalf("weighted score with saturation guard = %.1f, want 72", score)
	}
	if score := runtimeClusterPressureScore(100, 0, nil, nil); score != 90 {
		t.Fatalf("single saturated resource score = %.1f, want 90", score)
	}
	cases := []struct {
		score float64
		want  string
	}{{0, "idle"}, {20, "light"}, {45, "moderate"}, {70, "heavy"}, {90, "full"}}
	for _, item := range cases {
		if got := runtimeClusterPressureLevel(item.score); got != item.want {
			t.Fatalf("level(%v) = %q, want %q", item.score, got, item.want)
		}
	}
}

func TestRuntimeClusterPressureHidesDetailsFromOrdinaryUsers(t *testing.T) {
	snapshot := kubeprovider.ClusterPressureSnapshot{
		CPURequestsMilli: 500, CPUAllocatableMilli: 1000, CPUUsageMilli: 250,
		MemoryRequestsBytes: 512, MemoryAllocatableBytes: 1024, MemoryUsageBytes: 256,
		MetricsAvailable: true, NodeCount: 1, PodCount: 2, ObservedAt: time.Now().UTC(),
	}
	public := runtimeClusterPressureFromSnapshot("clu_demo", snapshot, false)
	if public.Details != nil || public.PressureScore != nil || public.PressureLevel != "moderate" {
		t.Fatalf("public pressure response = %#v", public)
	}
	admin := runtimeClusterPressureFromSnapshot("clu_demo", snapshot, true)
	if admin.Details == nil || admin.PressureScore == nil || admin.Details.CPU.Usage == nil {
		t.Fatalf("admin pressure response = %#v", admin)
	}
}

func TestRuntimeClusterPressureIDsAreBoundedAndDeduplicated(t *testing.T) {
	ids, ok := runtimeClusterPressureIDs([]string{"clu_a", "clu_a", "clu_b"})
	if !ok || len(ids) != 2 || ids[0] != "clu_a" || ids[1] != "clu_b" {
		t.Fatalf("ids = %#v, ok = %v", ids, ok)
	}
	if _, ok := runtimeClusterPressureIDs([]string{"invalid"}); ok {
		t.Fatal("invalid cluster identifier was accepted")
	}
	tooMany := make([]string, maxRuntimeClusterPressureBatch+1)
	for index := range tooMany {
		tooMany[index] = "clu_duplicate"
	}
	if _, ok := runtimeClusterPressureIDs(tooMany); ok {
		t.Fatal("more than 100 query values were accepted")
	}
}
