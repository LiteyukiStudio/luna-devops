package volume

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestProjectVolumeLifecycleTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		from string
		to   string
		want bool
	}{
		{model.ProjectVolumeLifecycleProvisioning, model.ProjectVolumeLifecycleReady, true},
		{model.ProjectVolumeLifecycleProvisioning, model.ProjectVolumeLifecycleError, true},
		{model.ProjectVolumeLifecycleReady, model.ProjectVolumeLifecycleDeleting, true},
		{model.ProjectVolumeLifecycleError, model.ProjectVolumeLifecycleProvisioning, true},
		{model.ProjectVolumeLifecycleDeleting, model.ProjectVolumeLifecycleError, true},
		{model.ProjectVolumeLifecycleReady, model.ProjectVolumeLifecycleProvisioning, false},
		{model.ProjectVolumeLifecycleDeleting, model.ProjectVolumeLifecycleReady, false},
		{"unknown", model.ProjectVolumeLifecycleReady, false},
	}
	for _, test := range tests {
		if got := CanTransitionProjectVolume(test.from, test.to); got != test.want {
			t.Errorf("CanTransitionProjectVolume(%q, %q) = %t, want %t", test.from, test.to, got, test.want)
		}
	}
}

func TestProjectVolumeAttachabilityAllowsFirstConsumerProvisioning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		item model.ProjectVolume
		want bool
	}{
		{name: "ready", item: model.ProjectVolume{LifecycleState: model.ProjectVolumeLifecycleReady}, want: true},
		{name: "first consumer provision", item: model.ProjectVolume{LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: OperationProvision}, want: true},
		{name: "expansion keeps existing claim attachable", item: model.ProjectVolume{LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: OperationExpand}, want: true},
		{name: "archive import is incomplete", item: model.ProjectVolume{LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: OperationImport}, want: false},
		{name: "failed", item: model.ProjectVolume{LifecycleState: model.ProjectVolumeLifecycleError, PendingOperation: OperationProvision}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := CanAttachProjectVolume(test.item); got != test.want {
				t.Fatalf("CanAttachProjectVolume(%#v) = %t, want %t", test.item, got, test.want)
			}
		})
	}
}

func TestVolumeTransferStateMachineHasNoTerminalRollback(t *testing.T) {
	t.Parallel()
	if !CanTransitionVolumeTransfer(model.VolumeTransferStateCreated, model.VolumeTransferStatePreparing) ||
		!CanTransitionVolumeTransfer(model.VolumeTransferStatePreparing, model.VolumeTransferStateReady) ||
		!CanTransitionVolumeTransfer(model.VolumeTransferStateReady, model.VolumeTransferStateStreaming) ||
		!CanTransitionVolumeTransfer(model.VolumeTransferStateStreaming, model.VolumeTransferStateSucceeded) {
		t.Fatal("expected transfer success path is not allowed")
	}
	for _, terminal := range []string{
		model.VolumeTransferStateSucceeded,
		model.VolumeTransferStateFailed,
		model.VolumeTransferStateCancelled,
		model.VolumeTransferStateExpired,
	} {
		if !IsVolumeTransferTerminal(terminal) {
			t.Fatalf("state %q must be terminal", terminal)
		}
		if CanTransitionVolumeTransfer(terminal, model.VolumeTransferStatePreparing) {
			t.Fatalf("terminal state %q must not transition back to queued", terminal)
		}
	}
}
