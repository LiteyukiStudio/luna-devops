package api

import (
	"context"
	"testing"
	"time"
)

func TestMemoryGatewayTrafficRuntimeStateMarksHello(t *testing.T) {
	store := newMemoryGatewayTrafficRuntimeStateStore()
	if err := store.MarkHello(context.Background(), "clu_1"); err != nil {
		t.Fatalf("MarkHello returned error: %v", err)
	}

	state, ok, err := store.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected state")
	}
	if state.Status != "deployed" || state.RuntimeClusterID != "clu_1" {
		t.Fatalf("state = %#v", state)
	}
	if state.LastReportedAt != nil {
		t.Fatalf("LastReportedAt = %v, want nil", state.LastReportedAt)
	}
}

func TestMemoryGatewayTrafficRuntimeStatePrefersReady(t *testing.T) {
	store := newMemoryGatewayTrafficRuntimeStateStore()
	if err := store.MarkHello(context.Background(), "clu_deployed"); err != nil {
		t.Fatalf("MarkHello returned error: %v", err)
	}
	if err := store.MarkReport(context.Background(), "clu_ready", time.Date(2026, 7, 6, 1, 2, 0, 0, time.UTC), time.Date(2026, 7, 6, 1, 3, 0, 0, time.UTC)); err != nil {
		t.Fatalf("MarkReport returned error: %v", err)
	}
	if err := store.MarkHello(context.Background(), "clu_deployed"); err != nil {
		t.Fatalf("MarkHello returned error: %v", err)
	}

	state, ok, err := store.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected state")
	}
	if state.RuntimeClusterID != "clu_ready" || state.Status != "ready" {
		t.Fatalf("state = %#v", state)
	}
}

func TestMemoryGatewayTrafficRuntimeStateDropsExpiredState(t *testing.T) {
	store := newMemoryGatewayTrafficRuntimeStateStore()
	store.states["clu_1"] = gatewayTrafficRuntimeState{
		RuntimeClusterID: "clu_1",
		Status:           "ready",
		LastHeartbeatAt:  time.Now().UTC().Add(-time.Hour),
		ExpiresAt:        time.Now().UTC().Add(-time.Minute),
	}

	_, ok, err := store.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected expired state to be ignored")
	}
}

func TestMemoryGatewayTrafficRuntimeStateDoesNotKeepExpiredReadyOnHello(t *testing.T) {
	store := newMemoryGatewayTrafficRuntimeStateStore()
	oldReport := time.Now().UTC().Add(-time.Hour)
	store.states["clu_1"] = gatewayTrafficRuntimeState{
		RuntimeClusterID: "clu_1",
		Status:           "ready",
		LastHeartbeatAt:  oldReport,
		LastReportedAt:   &oldReport,
		ExpiresAt:        time.Now().UTC().Add(-time.Minute),
	}

	if err := store.MarkHello(context.Background(), "clu_1"); err != nil {
		t.Fatalf("MarkHello returned error: %v", err)
	}
	state, ok, err := store.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected state")
	}
	if state.Status != "deployed" || state.LastReportedAt != nil {
		t.Fatalf("state = %#v", state)
	}
}
