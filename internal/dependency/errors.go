package dependency

import "errors"

const (
	CodeInvalidInput      = "dependency_invalid_input"
	CodeNotFound          = "dependency_not_found"
	CodeCrossProject      = "service_binding_cross_project"
	CodeCrossCluster      = "service_binding_cross_cluster"
	CodeSourceTargetSame  = "service_binding_source_target_same"
	CodePortNotFound      = "service_binding_port_not_found"
	CodeEnvConflict       = "service_binding_env_conflict"
	CodeTargetDeleting    = "service_binding_target_deleting"
	CodeReservedEnv       = "service_binding_reserved_env"
	CodeTopologyDuplicate = "topology_edge_duplicate"
	CodeTopologyTruncated = "topology_truncated"
	CodeDependencyCycle   = "dependency_cycle"
)

type DomainError struct {
	Code string
	Err  error
}

func (err *DomainError) Error() string {
	if err == nil || err.Err == nil {
		return "dependency operation failed"
	}
	return err.Err.Error()
}

func (err *DomainError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func domainError(code, message string) error {
	return &DomainError{Code: code, Err: errors.New(message)}
}

func ErrorCode(err error) string {
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return domainErr.Code
	}
	return ""
}
