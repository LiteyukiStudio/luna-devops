package kubeproxy

import (
	"context"
	"fmt"
	"time"
)

type AuditEvent struct {
	ActorID          string
	CredentialID     string
	BindingID        string
	ProjectID        string
	ApplicationID    string
	RuntimeClusterID string
	Namespace        string
	APIGroup         string
	APIVersion       string
	Resource         string
	Subresource      string
	Verb             string
	ObjectName       string
	Transport        TransportClass
	RequestID        string
	TraceID          string
	StartedAt        time.Time
}

type AuditAttempt struct {
	ID string
}

type AuditResult struct {
	Allowed        bool
	StatusCode     int
	Outcome        string
	ErrorCode      string
	StreamTerminal string
	Duration       time.Duration
	FinishedAt     time.Time
}

type AuditRecorder interface {
	Begin(context.Context, AuditEvent) (AuditAttempt, error)
	Finish(context.Context, AuditAttempt, AuditResult) error
	RecordDenial(context.Context, AuditEvent, AuditResult) error
}

type AuditCoordinator struct {
	Recorder AuditRecorder
	Timeout  time.Duration
}

func (coordinator AuditCoordinator) Begin(ctx context.Context, event AuditEvent, required bool) (AuditAttempt, error) {
	if !required {
		return AuditAttempt{}, nil
	}
	if coordinator.Recorder == nil {
		return AuditAttempt{}, Unavailable(CodeAuditUnavailable, fmt.Errorf("audit recorder is unavailable"))
	}
	attempt, err := coordinator.Recorder.Begin(ctx, event)
	if err != nil {
		return AuditAttempt{}, Unavailable(CodeAuditUnavailable, err)
	}
	if attempt.ID == "" {
		return AuditAttempt{}, Unavailable(CodeAuditUnavailable, fmt.Errorf("audit attempt ID is missing"))
	}
	return attempt, nil
}

func (coordinator AuditCoordinator) Finish(ctx context.Context, attempt AuditAttempt, result AuditResult) error {
	if attempt.ID == "" || coordinator.Recorder == nil {
		return nil
	}
	timeout := coordinator.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	return coordinator.Recorder.Finish(finishCtx, attempt, result)
}

func ShouldPersistAudit(info RequestInfo, decision Decision) bool {
	if info.Transport != "" && info.Transport != TransportNormal {
		return true
	}
	if info.Verb != "get" && info.Verb != "list" && info.Verb != "watch" {
		return true
	}
	for _, action := range decision.Actions {
		if action == "secret:view_value" || action == "deployment:exec" {
			return true
		}
	}
	return false
}
