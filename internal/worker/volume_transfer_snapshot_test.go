package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

type snapshotObservationProviderStub struct {
	kubeprovider.ProjectVolumeProvider
	observations int
}

func (stub *snapshotObservationProviderStub) ObserveVolumeSnapshot(context.Context, string, string) (kubeprovider.VolumeSnapshotObservation, error) {
	stub.observations++
	return kubeprovider.VolumeSnapshotObservation{}, nil
}

func TestVolumeTransferSnapshotWaitStopsAtItsOwnDeadline(t *testing.T) {
	provider := &snapshotObservationProviderStub{}
	started := time.Now()
	err := waitForVolumeTransferSnapshotReady(t.Context(), provider, "ns-demo", "vtx-snapshot-demo", kubeprovider.VolumeSnapshotObservation{}, 20*time.Millisecond)
	if volume.ErrorCode(err) != volume.CodeTransferSnapshotTimeout {
		t.Fatalf("snapshot wait error = %v, want %s", err, volume.CodeTransferSnapshotTimeout)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("snapshot wait ignored its deadline: %s", elapsed)
	}
}

func TestVolumeTransferSnapshotWaitPreservesTaskCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := waitForVolumeTransferSnapshotReady(ctx, &snapshotObservationProviderStub{}, "ns-demo", "vtx-snapshot-demo", kubeprovider.VolumeSnapshotObservation{}, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled snapshot wait error = %v, want context.Canceled", err)
	}
}

func TestVolumeTransferSnapshotWaitReportsSnapshotFailure(t *testing.T) {
	err := waitForVolumeTransferSnapshotReady(t.Context(), &snapshotObservationProviderStub{}, "ns-demo", "vtx-snapshot-demo",
		kubeprovider.VolumeSnapshotObservation{ErrorCode: "SnapshotFailed"}, time.Minute)
	if volume.ErrorCode(err) != volume.CodeTransferJobFailed {
		t.Fatalf("failed snapshot error = %v", err)
	}
	if err := waitForVolumeTransferSnapshotReady(t.Context(), &snapshotObservationProviderStub{}, "ns-demo", "vtx-snapshot-demo",
		kubeprovider.VolumeSnapshotObservation{ReadyToUse: true}, time.Minute); err != nil {
		t.Fatalf("ready snapshot error = %v", err)
	}
}
