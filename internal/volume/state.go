package volume

import "github.com/LiteyukiStudio/devops/internal/model"

func CanTransitionProjectVolume(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case model.ProjectVolumeLifecycleProvisioning:
		return to == model.ProjectVolumeLifecycleReady || to == model.ProjectVolumeLifecycleError || to == model.ProjectVolumeLifecycleDeleting
	case model.ProjectVolumeLifecycleReady:
		return to == model.ProjectVolumeLifecycleError || to == model.ProjectVolumeLifecycleDeleting
	case model.ProjectVolumeLifecycleError:
		return to == model.ProjectVolumeLifecycleProvisioning || to == model.ProjectVolumeLifecycleDeleting
	case model.ProjectVolumeLifecycleDeleting:
		return to == model.ProjectVolumeLifecycleError
	default:
		return false
	}
}

func CanTransitionVolumeTransfer(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case model.VolumeTransferStateCreated:
		return to == model.VolumeTransferStateUploading || to == model.VolumeTransferStateQueued || isTransferFailureTerminal(to)
	case model.VolumeTransferStateUploading:
		return to == model.VolumeTransferStateQueued || isTransferFailureTerminal(to)
	case model.VolumeTransferStateQueued:
		return to == model.VolumeTransferStateRunning || to == model.VolumeTransferStateFailed || to == model.VolumeTransferStateCancelled
	case model.VolumeTransferStateRunning:
		return to == model.VolumeTransferStateSucceeded || to == model.VolumeTransferStateFailed || to == model.VolumeTransferStateCancelled
	default:
		return false
	}
}

func IsVolumeTransferTerminal(state string) bool {
	return state == model.VolumeTransferStateSucceeded || isTransferFailureTerminal(state)
}

func isTransferFailureTerminal(state string) bool {
	return state == model.VolumeTransferStateFailed || state == model.VolumeTransferStateCancelled || state == model.VolumeTransferStateExpired
}
