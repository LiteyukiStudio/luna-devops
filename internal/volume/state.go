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

// CanAttachProjectVolume reports whether a deployment may become the first
// consumer of the claim. A newly provisioned or expanding PVC can remain
// Pending with WaitForFirstConsumer until Kubernetes sees that deployment.
func CanAttachProjectVolume(item model.ProjectVolume) bool {
	if item.LifecycleState == model.ProjectVolumeLifecycleReady {
		return true
	}
	if item.LifecycleState != model.ProjectVolumeLifecycleProvisioning {
		return false
	}
	return item.PendingOperation == OperationProvision || item.PendingOperation == OperationExpand
}

func CanTransitionVolumeTransfer(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case model.VolumeTransferStateCreated:
		return to == model.VolumeTransferStatePreparing || isTransferFailureTerminal(to)
	case model.VolumeTransferStatePreparing:
		return to == model.VolumeTransferStateReady || isTransferFailureTerminal(to)
	case model.VolumeTransferStateReady:
		return to == model.VolumeTransferStateStreaming || isTransferFailureTerminal(to)
	case model.VolumeTransferStateStreaming:
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
