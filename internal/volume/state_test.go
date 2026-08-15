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

func TestVolumeTransferStateMachineHasNoTerminalRollback(t *testing.T) {
	t.Parallel()
	if !CanTransitionVolumeTransfer(model.VolumeTransferStateCreated, model.VolumeTransferStateUploading) ||
		!CanTransitionVolumeTransfer(model.VolumeTransferStateUploading, model.VolumeTransferStateQueued) ||
		!CanTransitionVolumeTransfer(model.VolumeTransferStateQueued, model.VolumeTransferStateRunning) ||
		!CanTransitionVolumeTransfer(model.VolumeTransferStateRunning, model.VolumeTransferStateSucceeded) {
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
		if CanTransitionVolumeTransfer(terminal, model.VolumeTransferStateQueued) {
			t.Fatalf("terminal state %q must not transition back to queued", terminal)
		}
	}
}
