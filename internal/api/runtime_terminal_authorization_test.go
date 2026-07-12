package api

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestRuntimeTerminalAuthorizationState(t *testing.T) {
	now := time.Now()
	binding := runtimeTerminalAuthorizationBinding{
		UserID:                    "usr_test",
		SessionID:                 "ses_test",
		AssertionID:               "mfaas_test",
		AssertionRequired:         true,
		AssertionAbsoluteDeadline: now.Add(20 * time.Minute),
		Deadline:                  now.Add(20 * time.Minute),
	}
	activeState := runtimeTerminalAuthorizationState{
		Session: model.UserSession{ID: binding.SessionID, UserID: binding.UserID, ExpiresAt: now.Add(time.Hour)},
		User:    model.User{ID: binding.UserID},
		Assertion: model.StepUpAssertion{
			ID:                binding.AssertionID,
			UserID:            binding.UserID,
			SessionID:         binding.SessionID,
			Purpose:           stepUpPurposeRuntimeTerminal,
			IdleExpiresAt:     now.Add(10 * time.Minute),
			AbsoluteExpiresAt: binding.AssertionAbsoluteDeadline,
		},
		AuthorizationAllowed: true,
	}

	tests := []struct {
		name   string
		mutate func(*runtimeTerminalAuthorizationState, *runtimeTerminalAuthorizationBinding)
		want   bool
	}{
		{name: "active", want: true},
		{name: "logout removes session", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Session = model.UserSession{}
		}},
		{name: "session expiry", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Session.ExpiresAt = now
		}},
		{name: "disabled user", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.User.Disabled = true
		}},
		{name: "role or membership removed", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.AuthorizationAllowed = false
		}},
		{name: "assertion revoked", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Assertion = model.StepUpAssertion{}
		}},
		{name: "assertion idle expiry", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Assertion.IdleExpiresAt = now
		}},
		{name: "assertion absolute expiry", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Assertion.AbsoluteExpiresAt = now
		}},
		{name: "bound deadline reached", mutate: func(_ *runtimeTerminalAuthorizationState, binding *runtimeTerminalAuthorizationBinding) {
			binding.Deadline = now
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := activeState
			currentBinding := binding
			if test.mutate != nil {
				test.mutate(&state, &currentBinding)
			}
			if got := state.active(currentBinding, now); got != test.want {
				t.Fatalf("active() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRuntimeTerminalAuthorizationStateWithoutMFAStillRequiresSessionAndAuthorization(t *testing.T) {
	now := time.Now()
	binding := runtimeTerminalAuthorizationBinding{UserID: "usr_test", SessionID: "ses_test", Deadline: now.Add(time.Hour)}
	state := runtimeTerminalAuthorizationState{
		Session:              model.UserSession{ID: binding.SessionID, UserID: binding.UserID, ExpiresAt: now.Add(time.Hour)},
		User:                 model.User{ID: binding.UserID},
		AuthorizationAllowed: true,
	}
	if !state.active(binding, now) {
		t.Fatal("expected an active browser session to remain authorized when step-up MFA is disabled")
	}
	state.AuthorizationAllowed = false
	if state.active(binding, now) {
		t.Fatal("business authorization removal must cancel a terminal even when step-up MFA is disabled")
	}
}
