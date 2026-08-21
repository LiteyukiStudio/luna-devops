package api

import (
	"context"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestRuntimeTerminalAuthorizationStateRequiresLiveIdentityAndAuthorization(t *testing.T) {
	now := time.Now()
	binding := runtimeTerminalAuthorizationBinding{UserID: "usr_test", SubjectID: "ses_test", Deadline: now.Add(time.Hour)}
	active := runtimeTerminalAuthorizationState{
		Session:              model.UserSession{ID: binding.SubjectID, UserID: binding.UserID, ExpiresAt: now.Add(time.Hour)},
		User:                 model.User{ID: binding.UserID},
		AuthorizationAllowed: true,
	}
	tests := []struct {
		name   string
		mutate func(*runtimeTerminalAuthorizationState, *runtimeTerminalAuthorizationBinding)
		want   bool
	}{
		{name: "active", want: true},
		{name: "session removed", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Session = model.UserSession{}
		}},
		{name: "session expired", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Session.ExpiresAt = now
		}},
		{name: "user disabled", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.User.Disabled = true
		}},
		{name: "authorization removed", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.AuthorizationAllowed = false
		}},
		{name: "ticket deadline reached", mutate: func(_ *runtimeTerminalAuthorizationState, binding *runtimeTerminalAuthorizationBinding) {
			binding.Deadline = now
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := active
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

func TestRuntimeTerminalAuthorizationStateSupportsLunaCLIOAuth(t *testing.T) {
	now := time.Now()
	grantID := "oagr_test"
	binding := runtimeTerminalAuthorizationBinding{UserID: "usr_test", SubjectID: oauthGrantSubject(grantID), Deadline: now.Add(time.Hour)}
	state := runtimeTerminalAuthorizationState{
		OAuthGrant:       model.OAuthGrant{ID: grantID, ApplicationID: lunaCLIApplicationID, UserID: binding.UserID},
		OAuthApplication: model.OAuthApplication{ID: lunaCLIApplicationID},
		User:             model.User{ID: binding.UserID}, AuthorizationAllowed: true,
	}
	if !state.active(binding, now) {
		t.Fatal("active Luna CLI OAuth grant should authorize the terminal")
	}
	state.OAuthGrant.RevokedAt = &now
	if state.active(binding, now) {
		t.Fatal("revoked Luna CLI OAuth grant must revoke terminal authorization")
	}
}

func TestRuntimeClusterPodTerminalTicketIsOneTimeAndResourceBound(t *testing.T) {
	handlers := &Handlers{mode: "test"}
	now := time.Now()
	binding := runtimeTerminalAuthorizationBinding{UserID: "usr_runtime_pod_ticket", SubjectID: "ses_runtime_pod_ticket", Deadline: now.Add(time.Hour)}
	reference := runtimeClusterPodTerminalAuthorizationReference{
		ClusterID: "rcl_runtime_pod_ticket", ClusterKubeconfig: "sec_runtime_pod_ticket", Namespace: "ns-runtime-pod-ticket",
		Name: "pod-runtime-pod-ticket", ProjectID: "prj_runtime_pod_ticket", ApplicationID: "app_runtime_pod_ticket",
		DeploymentTargetID: "dplt_runtime_pod_ticket", ReleaseID: "rel_runtime_pod_ticket",
	}
	ticket, expiresAt, err := handlers.issueRuntimeTerminalTicket(context.Background(), binding, "runtime_pod", reference)
	if err != nil {
		t.Fatal(err)
	}
	if expiresAt.Before(now.Add(runtimeTerminalTicketTTL-time.Second)) || expiresAt.After(now.Add(runtimeTerminalTicketTTL+time.Second)) {
		t.Fatalf("ticket expiry %s is outside the expected short TTL", expiresAt)
	}
	if _, found := runtimeTerminalMemoryTickets.Load(ticket); found {
		t.Fatal("raw terminal ticket must not be stored")
	}
	if _, found := runtimeTerminalMemoryTickets.Load(hashToken(ticket)); !found {
		t.Fatal("hashed terminal ticket was not stored")
	}
	value, ok, err := handlers.consumeRuntimeTerminalTicket(context.Background(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value.UserID != binding.UserID || value.Authorization.SubjectID != binding.SubjectID {
		t.Fatalf("consumed ticket is not bound to the expected user/session: %#v", value)
	}
	if !value.matches("runtime_pod", reference) {
		t.Fatal("ticket did not match its runtime Pod resource")
	}
	otherReference := reference
	otherReference.Name = "another-pod"
	if value.matches("runtime_pod", otherReference) {
		t.Fatal("ticket must not match another runtime Pod")
	}
	if _, ok, err := handlers.consumeRuntimeTerminalTicket(context.Background(), ticket); err != nil || ok {
		t.Fatalf("consuming a terminal ticket twice = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestRuntimeTerminalAuthorizationRevokesDeletedSession(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now()
	user := model.User{ID: "usr_terminal_monitor", Email: "terminal-monitor@example.com", Name: "Terminal Monitor", Role: authz.PlatformRoleAdmin, Language: "en-US"}
	session := model.UserSession{ID: "ses_terminal_monitor", UserID: user.ID, TokenHash: hashToken("terminal-monitor"), ExpiresAt: now.Add(time.Hour)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	binding := runtimeTerminalAuthorizationBinding{UserID: user.ID, SubjectID: session.ID, Deadline: session.ExpiresAt}
	handlers := &Handlers{db: db}
	if !handlers.runtimeTerminalAuthorizationActive(context.Background(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("live session should authorize the terminal")
	}
	if err := db.Delete(&session).Error; err != nil {
		t.Fatal(err)
	}
	if handlers.runtimeTerminalAuthorizationActive(context.Background(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("deleted session must revoke terminal authorization")
	}
}
