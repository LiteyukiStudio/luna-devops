package model

import (
	"testing"
	"time"
)

func TestDeploymentRuntimeConfigRefsNormalizeAndLiveIDs(t *testing.T) {
	refs := []DeploymentRuntimeConfigRef{
		{SetID: " set_live ", Mode: "LIVE"},
		{SetID: "set_snapshot", Mode: RuntimeConfigRefModeSnapshot, Snapshot: &DeploymentRuntimeConfigSnapshot{Name: "snapshot", Enabled: true}},
		{SetID: "set_live", Mode: RuntimeConfigRefModeSnapshot},
		{SetID: "", Mode: RuntimeConfigRefModeLive},
	}

	encoded := EncodeDeploymentRuntimeConfigRefs(refs)
	decoded := DecodeDeploymentRuntimeConfigRefs(encoded)

	if len(decoded) != 2 {
		t.Fatalf("decoded refs = %#v", decoded)
	}
	if decoded[0].SetID != "set_live" || decoded[0].Mode != RuntimeConfigRefModeLive {
		t.Fatalf("first ref = %#v", decoded[0])
	}
	if decoded[1].SetID != "set_snapshot" || decoded[1].Mode != RuntimeConfigRefModeSnapshot || decoded[1].Snapshot == nil {
		t.Fatalf("second ref = %#v", decoded[1])
	}

	liveIDs := DeploymentRuntimeConfigLiveSetIDs(decoded)
	if len(liveIDs) != 1 || liveIDs[0] != "set_live" {
		t.Fatalf("live IDs = %#v", liveIDs)
	}
}

func TestProjectRuntimeConfigSetSnapshotStoresSecretRefsOnly(t *testing.T) {
	capturedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	snapshot := ProjectRuntimeConfigSetSnapshot(ProjectRuntimeConfigSet{
		Name:        "shared",
		EnvVars:     "LOG_LEVEL=info",
		ConfigFiles: `[{"path":"/app/config.yaml","content":"debug: false"}]`,
		SecretRefs:  `{"TOKEN":"secret-id"}`,
		SecretFiles: `{"/app/key.pem":"secret-file-id"}`,
		Enabled:     true,
	}, capturedAt)

	if snapshot.SecretRefs != `{"TOKEN":"secret-id"}` || snapshot.SecretFiles != `{"/app/key.pem":"secret-file-id"}` {
		t.Fatalf("snapshot secrets = %#v", snapshot)
	}
	if !snapshot.CapturedAt.Equal(capturedAt) {
		t.Fatalf("capturedAt = %s", snapshot.CapturedAt)
	}
}
