package runtimeapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestRuntimeClusterKubeGatewayResponseKeepsArrayContract(t *testing.T) {
	for _, raw := range []string{"", "[]", "null"} {
		rules, err := decodeRuntimeClusterKubeGatewayRules(raw)
		if err != nil {
			t.Fatalf("decodeRuntimeClusterKubeGatewayRules(%q): %v", raw, err)
		}
		if rules == nil {
			t.Fatalf("decodeRuntimeClusterKubeGatewayRules(%q) returned a nil slice", raw)
		}
		encoded, err := json.Marshal(runtimeClusterKubeGatewayResponse{ExtraResourceRules: rules, Status: "disabled"})
		if err != nil {
			t.Fatalf("marshal empty gateway response: %v", err)
		}
		if !strings.Contains(string(encoded), `"extraResourceRules":[]`) {
			t.Fatalf("empty gateway response = %s, want an array", encoded)
		}
	}

	rules := normalizeRuntimeClusterKubeGatewayRules([]runtimeClusterKubeGatewayRule{{
		APIGroup: "example.io", APIVersion: "v1", Resource: "widgets", Verbs: []string{"get"}, Action: "project:read",
	}})
	encoded, err := json.Marshal(runtimeClusterKubeGatewayResponse{ExtraResourceRules: rules, Status: "ready"})
	if err != nil {
		t.Fatalf("marshal gateway rule response: %v", err)
	}
	if !strings.Contains(string(encoded), `"subresources":[]`) {
		t.Fatalf("gateway rule response = %s, want an explicit subresources array", encoded)
	}
}

func TestRuntimeClusterKubeGatewayStatusIDsAreBoundedAndDeduplicated(t *testing.T) {
	ids, ok := runtimeClusterKubeGatewayStatusIDs([]string{"clu_first", " clu_first ", "clu_second"})
	if !ok || len(ids) != 2 || ids[0] != "clu_first" || ids[1] != "clu_second" {
		t.Fatalf("ids = %#v, ok = %v", ids, ok)
	}
	for _, invalid := range [][]string{nil, {}, {"invalid"}, {"clu_valid", ""}} {
		if _, ok := runtimeClusterKubeGatewayStatusIDs(invalid); ok {
			t.Fatalf("invalid cluster IDs were accepted: %#v", invalid)
		}
	}
	tooMany := make([]string, maxRuntimeClusterKubeGatewayStatusBatch+1)
	for index := range tooMany {
		tooMany[index] = "clu_duplicate"
	}
	if _, ok := runtimeClusterKubeGatewayStatusIDs(tooMany); ok {
		t.Fatal("more than 100 query values were accepted")
	}
}

func TestRuntimeClusterKubeGatewayStatusesPreserveOrderAndSanitizeObservations(t *testing.T) {
	checkedAt := time.Date(2026, time.September, 1, 8, 30, 0, 0, time.UTC)
	clusters := []model.RuntimeCluster{
		{ID: "clu_ready", KubeGatewayEnabled: true},
		{ID: "clu_reconciling", KubeGatewayEnabled: true},
		{ID: "clu_failed", KubeGatewayEnabled: false},
	}
	items := observeRuntimeClusterKubeGatewayStatuses(t.Context(), clusters, time.Second, func(_ context.Context, cluster model.RuntimeCluster) runtimeClusterKubeGatewayResponse {
		switch cluster.ID {
		case "clu_ready":
			return runtimeClusterKubeGatewayResponse{Status: "ready", ObservationCode: "raw upstream detail", LastCheckedAt: &checkedAt}
		case "clu_reconciling":
			return runtimeClusterKubeGatewayResponse{Status: "reconciling", ObservationCode: "unexpected code", LastCheckedAt: &checkedAt}
		default:
			return runtimeClusterKubeGatewayResponse{Status: "unknown", ObservationCode: "secret failure", LastCheckedAt: &checkedAt}
		}
	})

	if len(items) != len(clusters) {
		t.Fatalf("items = %#v", items)
	}
	wantStatuses := []string{"ready", "reconciling", "unavailable"}
	wantCodes := []string{"", "kube_gateway.reconciling", "kube_gateway.unavailable"}
	for index, item := range items {
		if item.ClusterID != clusters[index].ID || item.Enabled != clusters[index].KubeGatewayEnabled {
			t.Fatalf("item %d identity = %#v", index, item)
		}
		if item.Status != wantStatuses[index] || item.ObservationCode != wantCodes[index] {
			t.Fatalf("item %d observation = %#v", index, item)
		}
		if !item.LastCheckedAt.Equal(checkedAt) {
			t.Fatalf("item %d lastCheckedAt = %s, want %s", index, item.LastCheckedAt, checkedAt)
		}
	}
}

func TestRuntimeClusterKubeGatewayStatusesLimitConcurrency(t *testing.T) {
	clusters := make([]model.RuntimeCluster, runtimeClusterKubeGatewayStatusConcurrency*2)
	for index := range clusters {
		clusters[index] = model.RuntimeCluster{ID: "clu_concurrency"}
	}

	var active atomic.Int32
	var peak atomic.Int32
	release := make(chan struct{})
	done := make(chan []runtimeClusterKubeGatewayStatusResponse, 1)
	go func() {
		done <- observeRuntimeClusterKubeGatewayStatuses(t.Context(), clusters, time.Second, func(_ context.Context, _ model.RuntimeCluster) runtimeClusterKubeGatewayResponse {
			current := active.Add(1)
			defer active.Add(-1)
			for {
				previous := peak.Load()
				if current <= previous || peak.CompareAndSwap(previous, current) {
					break
				}
			}
			<-release
			return runtimeClusterKubeGatewayResponse{Status: "ready"}
		})
	}()

	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for peak.Load() < runtimeClusterKubeGatewayStatusConcurrency {
		select {
		case <-deadline.C:
			close(release)
			t.Fatalf("peak concurrency = %d, want %d", peak.Load(), runtimeClusterKubeGatewayStatusConcurrency)
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(release)
	items := <-done
	if len(items) != len(clusters) {
		t.Fatalf("items = %d, want %d", len(items), len(clusters))
	}
	if got := peak.Load(); got != runtimeClusterKubeGatewayStatusConcurrency {
		t.Fatalf("peak concurrency = %d, want %d", got, runtimeClusterKubeGatewayStatusConcurrency)
	}
}

func TestRuntimeClusterKubeGatewayStatusesRespectCancellationAndTimeout(t *testing.T) {
	t.Run("parent cancelled before observation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		var calls atomic.Int32
		items := observeRuntimeClusterKubeGatewayStatuses(ctx, []model.RuntimeCluster{{ID: "clu_cancelled", KubeGatewayEnabled: true}}, time.Second, func(_ context.Context, _ model.RuntimeCluster) runtimeClusterKubeGatewayResponse {
			calls.Add(1)
			return runtimeClusterKubeGatewayResponse{Status: "ready"}
		})
		if calls.Load() != 0 {
			t.Fatalf("observer calls = %d, want 0", calls.Load())
		}
		assertUnavailableRuntimeClusterKubeGatewayStatus(t, items, "clu_cancelled", true)
	})

	t.Run("per cluster timeout", func(t *testing.T) {
		var timedOut atomic.Bool
		items := observeRuntimeClusterKubeGatewayStatuses(t.Context(), []model.RuntimeCluster{{ID: "clu_timeout"}}, 5*time.Millisecond, func(ctx context.Context, _ model.RuntimeCluster) runtimeClusterKubeGatewayResponse {
			<-ctx.Done()
			timedOut.Store(ctx.Err() == context.DeadlineExceeded)
			return runtimeClusterKubeGatewayResponse{Status: "ready"}
		})
		if !timedOut.Load() {
			t.Fatal("observer context did not reach its per-cluster deadline")
		}
		assertUnavailableRuntimeClusterKubeGatewayStatus(t, items, "clu_timeout", false)
	})
}

func assertUnavailableRuntimeClusterKubeGatewayStatus(t *testing.T, items []runtimeClusterKubeGatewayStatusResponse, clusterID string, enabled bool) {
	t.Helper()
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	item := items[0]
	if item.ClusterID != clusterID || item.Enabled != enabled || item.Status != "unavailable" || item.ObservationCode != "kube_gateway.unavailable" || item.LastCheckedAt.IsZero() {
		t.Fatalf("unavailable item = %#v", item)
	}
}
