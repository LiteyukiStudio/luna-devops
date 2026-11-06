package model

import "strings"

// Delivery workflow states shared by the API and Worker. Status transitions are
// guarded with these sets so a late writer cannot overwrite a terminal state.
var (
	BuildActiveStatuses   = []string{"queued", "running"}
	ReleaseActiveStatuses = []string{"pending", "running"}
)

// Stable, low-cardinality failure codes persisted on BuildRun/BuildJob.
// The raw error stays in job messages, logs and traces for diagnostics; user
// interfaces localize these codes instead.
const (
	BuildFailureCanceledByUser      = "canceled_by_user"
	BuildFailureTimeout             = "timeout"
	BuildFailureQueueTimeout        = "queue_timeout"
	BuildFailureDispatchFailed      = "dispatch_failed"
	BuildFailureProjectUnavailable  = "project_unavailable"
	BuildFailureTargetUnavailable   = "deployment_target_unavailable"
	BuildFailureClusterUnavailable  = "cluster_unavailable"
	BuildFailureCapacityCheckFailed = "capacity_check_failed"
	BuildFailureNamespaceFailed     = "namespace_failed"
	BuildFailureTaskResolveFailed   = "task_resolve_failed"
	BuildFailureJobStartFailed      = "job_start_failed"
	BuildFailureJobFailed           = "job_failed"
	BuildFailureObservationFailed   = "observation_failed"
	BuildFailureInternalError       = "internal_error"
)

// Stable, low-cardinality failure codes persisted on Release.
const (
	ReleaseFailureCanceledByUser         = "canceled_by_user"
	ReleaseFailureTimeout                = "timeout"
	ReleaseFailureDispatchFailed         = "dispatch_failed"
	ReleaseFailureProjectUnavailable     = "project_unavailable"
	ReleaseFailureApplicationUnavailable = "application_unavailable"
	ReleaseFailureTargetUnavailable      = "deployment_target_unavailable"
	ReleaseFailureNamespaceFailed        = "namespace_failed"
	ReleaseFailureServiceBindingFailed   = "service_binding_failed"
	ReleaseFailurePreflightFailed        = "preflight_failed"
	ReleaseFailureRuntimeConfigFailed    = "runtime_config_failed"
	ReleaseFailurePreHookFailed          = "pre_hook_failed"
	ReleaseFailureDependenciesFailed     = "dependencies_failed"
	ReleaseFailureApplyFailed            = "apply_failed"
	ReleaseFailureRolloutFailed          = "rollout_failed"
	ReleaseFailureRolloutTimeout         = "rollout_timeout"
	ReleaseFailureVolumeMountFailed      = "volume_mount_failed"
	ReleaseFailurePostHookFailed         = "post_hook_failed"
	ReleaseFailureSuperseded             = "superseded"
	ReleaseFailureInternalError          = "internal_error"
)

// DeleteStatusActive reports whether a resource with this delete_status still
// accepts new delivery work. delete_failed resources remain usable so that
// operators can repair and retry deletion.
func DeleteStatusActive(status string) bool {
	switch strings.TrimSpace(status) {
	case "", "active", "delete_failed":
		return true
	default:
		return false
	}
}

// ActiveDeleteStatuses lists delete_status values accepted by DeleteStatusActive
// for use in SQL filters.
var ActiveDeleteStatuses = []string{"", "active", "delete_failed"}
