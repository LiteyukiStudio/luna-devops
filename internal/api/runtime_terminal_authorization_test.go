package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

func TestRuntimeTerminalAuthorizationState(t *testing.T) {
	now := time.Now()
	binding := runtimeTerminalAuthorizationBinding{
		UserID:                    "usr_test",
		SubjectID:                 "ses_test",
		AssertionID:               "mfaas_test",
		AssertionRequired:         true,
		AssertionAbsoluteDeadline: now.Add(20 * time.Minute),
		Deadline:                  now.Add(20 * time.Minute),
	}
	activeState := runtimeTerminalAuthorizationState{
		Session: model.UserSession{ID: binding.SubjectID, UserID: binding.UserID, ExpiresAt: now.Add(time.Hour)},
		User:    model.User{ID: binding.UserID},
		Assertion: model.StepUpAssertion{
			ID:                binding.AssertionID,
			UserID:            binding.UserID,
			SessionID:         binding.SubjectID,
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
	binding := runtimeTerminalAuthorizationBinding{UserID: "usr_test", SubjectID: "ses_test", Deadline: now.Add(time.Hour)}
	state := runtimeTerminalAuthorizationState{
		Session:              model.UserSession{ID: binding.SubjectID, UserID: binding.UserID, ExpiresAt: now.Add(time.Hour)},
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

func TestRuntimeClusterPodTerminalTicketIsOneTimeAndResourceBound(t *testing.T) {
	handlers := &Handlers{mode: "test"}
	now := time.Now()
	binding := runtimeTerminalAuthorizationBinding{
		UserID:    "usr_runtime_pod_ticket",
		SubjectID: "ses_runtime_pod_ticket",
		Deadline:  now.Add(time.Hour),
	}
	reference := runtimeClusterPodTerminalAuthorizationReference{
		ClusterID:          "rcl_runtime_pod_ticket",
		ClusterKubeconfig:  "sec_runtime_pod_ticket",
		Namespace:          "ns-runtime-pod-ticket",
		Name:               "pod-runtime-pod-ticket",
		ProjectID:          "prj_runtime_pod_ticket",
		ApplicationID:      "app_runtime_pod_ticket",
		DeploymentTargetID: "dplt_runtime_pod_ticket",
		ReleaseID:          "rel_runtime_pod_ticket",
	}

	ticket, expiresAt, err := handlers.issueRuntimeTerminalTicket(context.Background(), binding, "runtime_pod", reference)
	if err != nil {
		t.Fatal(err)
	}
	if expiresAt.Before(now.Add(runtimeTerminalTicketTTL-time.Second)) ||
		expiresAt.After(now.Add(runtimeTerminalTicketTTL+time.Second)) {
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

func TestReleaseRuntimeTerminalTicketBindsOAuthGrantAssertionAndResource(t *testing.T) {
	handlers := &Handlers{mode: "test"}
	now := time.Now()
	grantID := "oagr_release_terminal_ticket"
	binding := runtimeTerminalAuthorizationBinding{
		UserID:                    "usr_release_terminal_ticket",
		SubjectID:                 oauthAssertionSubject(grantID),
		AssertionID:               "mfaas_release_terminal_ticket",
		AssertionRequired:         true,
		AssertionAbsoluteDeadline: now.Add(20 * time.Minute),
		Deadline:                  now.Add(20 * time.Minute),
	}
	reference := releaseRuntimeTerminalAuthorizationReference{
		ProjectID:          "prj_release_terminal_ticket",
		ApplicationID:      "app_release_terminal_ticket",
		ReleaseID:          "rel_release_terminal_ticket",
		DeploymentTargetID: "dplt_release_terminal_ticket",
		ClusterID:          "rcl_release_terminal_ticket",
		ClusterKubeconfig:  "sec_release_terminal_ticket",
		Namespace:          "ns-release-terminal-ticket",
	}

	ticket, _, err := handlers.issueRuntimeTerminalTicket(context.Background(), binding, "release", reference)
	if err != nil {
		t.Fatal(err)
	}
	value, ok, err := handlers.consumeRuntimeTerminalTicket(context.Background(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected the release terminal ticket to be consumable")
	}
	if value.Authorization.SubjectID != oauthAssertionSubject(grantID) ||
		value.Authorization.AssertionID != binding.AssertionID ||
		!value.Authorization.AssertionRequired {
		t.Fatalf("release ticket lost its OAuth grant or MFA assertion binding: %#v", value.Authorization)
	}
	if !value.matches("release", reference) {
		t.Fatal("ticket did not match its release resource")
	}
	otherReference := reference
	otherReference.ReleaseID = "rel_other"
	if value.matches("release", otherReference) {
		t.Fatal("ticket must not match another release")
	}

	activeState := runtimeTerminalAuthorizationState{
		OAuthGrant: model.OAuthGrant{
			ID:            grantID,
			ApplicationID: lunaCLIApplicationID,
			UserID:        binding.UserID,
		},
		OAuthApplication: model.OAuthApplication{ID: lunaCLIApplicationID},
		User:             model.User{ID: binding.UserID},
		Assertion: model.StepUpAssertion{
			ID:                binding.AssertionID,
			UserID:            binding.UserID,
			SessionID:         binding.SubjectID,
			Purpose:           stepUpPurposeRuntimeTerminal,
			IdleExpiresAt:     now.Add(10 * time.Minute),
			AbsoluteExpiresAt: binding.AssertionAbsoluteDeadline,
		},
		AuthorizationAllowed: true,
	}
	if !activeState.active(binding, now) {
		t.Fatal("expected an active Luna CLI OAuth grant and assertion to authorize the ticket")
	}
	activeState.OAuthGrant.RevokedAt = &now
	if activeState.active(binding, now) {
		t.Fatal("revoking the OAuth grant must revoke release terminal authorization")
	}
	if _, ok, err := handlers.consumeRuntimeTerminalTicket(context.Background(), ticket); err != nil || ok {
		t.Fatalf("consuming a release terminal ticket twice = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestRuntimeTerminalAuthorizationPollDoesNotRefreshIdleExpiry(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	binding, assertion := createRuntimeTerminalAuthorizationFixture(t, db, now)
	handlers := runtimeTerminalTestHandlers(db, true, "10")

	if !handlers.runtimeTerminalAuthorizationActive(context.Background(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("expected terminal authorization to be active before idle expiry")
	}
	var stored model.StepUpAssertion
	if err := db.First(&stored, "id = ?", assertion.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.LastActivityAt.Equal(assertion.LastActivityAt) || !stored.IdleExpiresAt.Equal(assertion.IdleExpiresAt) {
		t.Fatalf("authorization polling refreshed activity: got last=%s idle=%s, want last=%s idle=%s", stored.LastActivityAt, stored.IdleExpiresAt, assertion.LastActivityAt, assertion.IdleExpiresAt)
	}

	if err := db.Model(&model.StepUpAssertion{}).Where("id = ?", assertion.ID).Update("idle_expires_at", now.Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if handlers.runtimeTerminalAuthorizationActive(context.Background(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("an idle terminal must be revoked after its assertion expires")
	}
}

func TestRuntimeTerminalAuthorizationMonitorRevokesIdleTerminal(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	binding, assertion := createRuntimeTerminalAuthorizationFixture(t, db, now)
	if err := db.Model(&model.StepUpAssertion{}).Where("id = ?", assertion.ID).Update("idle_expires_at", now.Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	handlers := runtimeTerminalTestHandlers(db, true, "10")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	revoked := handlers.monitorRuntimeTerminalAuthorizationAtInterval(ctx, binding, func(context.Context, model.User) bool { return true }, cancel, 5*time.Millisecond)

	select {
	case <-revoked:
	case <-time.After(time.Second):
		t.Fatal("idle terminal authorization was not revoked")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("revoked terminal did not cancel its session context")
	}
}

func TestRuntimeTerminalInputRefreshesIdleWithoutExtendingAbsoluteExpiry(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	binding, assertion := createRuntimeTerminalAuthorizationFixture(t, db, now)
	handlers := runtimeTerminalTestHandlers(db, true, "10")
	tracker := handlers.newRuntimeTerminalActivityTracker(binding)
	refreshAt := now.Add(30 * time.Second)

	if !tracker.Record(context.Background(), refreshAt) {
		t.Fatal("expected real terminal input to refresh the assertion")
	}
	var stored model.StepUpAssertion
	if err := db.First(&stored, "id = ?", assertion.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.LastActivityAt.Equal(refreshAt) {
		t.Fatalf("last activity = %s, want %s", stored.LastActivityAt, refreshAt)
	}
	if !stored.IdleExpiresAt.Equal(binding.AssertionAbsoluteDeadline) {
		t.Fatalf("idle expiry = %s, want absolute deadline %s", stored.IdleExpiresAt, binding.AssertionAbsoluteDeadline)
	}
	if !stored.AbsoluteExpiresAt.Equal(assertion.AbsoluteExpiresAt) {
		t.Fatalf("absolute expiry changed from %s to %s", assertion.AbsoluteExpiresAt, stored.AbsoluteExpiresAt)
	}

	throttledAt := refreshAt.Add(time.Second)
	if !tracker.Record(context.Background(), throttledAt) {
		t.Fatal("a throttled input should preserve the active assertion")
	}
	if err := db.First(&stored, "id = ?", assertion.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.LastActivityAt.Equal(refreshAt) {
		t.Fatalf("throttled input unexpectedly wrote activity at %s", stored.LastActivityAt)
	}
	if tracker.Record(context.Background(), binding.AssertionAbsoluteDeadline) {
		t.Fatal("input at the absolute deadline must not revive the terminal assertion")
	}
	if err := db.First(&stored, "id = ?", assertion.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.AbsoluteExpiresAt.Equal(assertion.AbsoluteExpiresAt) || !stored.IdleExpiresAt.Equal(binding.AssertionAbsoluteDeadline) {
		t.Fatalf("absolute deadline changed after an expired input: idle=%s absolute=%s", stored.IdleExpiresAt, stored.AbsoluteExpiresAt)
	}
}

func TestRuntimeTerminalInputClassification(t *testing.T) {
	queue := newRuntimeTerminalSizeQueue()
	resize, err := json.Marshal(runtimeTerminalClientMessage{Type: "resize", Cols: 120, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := runtimeTerminalInputPayload(websocket.TextMessage, resize, queue); ok {
		t.Fatal("resize messages must not count as terminal activity")
	}
	size := queue.Next()
	if size == nil || size.Width != 120 || size.Height != 40 {
		t.Fatalf("resize was not forwarded to the terminal size queue: %#v", size)
	}
	if _, ok := runtimeTerminalInputPayload(websocket.PingMessage, []byte("ping"), queue); ok {
		t.Fatal("ping messages must not count as terminal activity")
	}
	if _, ok := runtimeTerminalInputPayload(websocket.TextMessage, nil, queue); ok {
		t.Fatal("empty messages must not count as terminal activity")
	}
	payload := []byte("echo hello\n")
	got, ok := runtimeTerminalInputPayload(websocket.TextMessage, payload, queue)
	if !ok || string(got) != string(payload) {
		t.Fatalf("stdin payload = %q, %v; want %q, true", got, ok, payload)
	}
}

func runtimeTerminalTestHandlers(db *gorm.DB, mfaEnabled bool, idleMinutes string) *Handlers {
	values := map[string]string{
		"security.stepUpMfa.enabled":            "false",
		"security.stepUpMfa.idleTimeoutMinutes": idleMinutes,
	}
	if mfaEnabled {
		values["security.stepUpMfa.enabled"] = "true"
	}
	return &Handlers{db: db, configs: &configCache{values: values}}
}

func createRuntimeTerminalAuthorizationFixture(t *testing.T, db *gorm.DB, now time.Time) (runtimeTerminalAuthorizationBinding, model.StepUpAssertion) {
	t.Helper()
	user := model.User{ID: "usr_runtime_terminal", Email: "runtime-terminal@example.com", Name: "Runtime Terminal", Role: authz.PlatformRoleAdmin, Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	session := model.UserSession{
		ID:        "ses_runtime_terminal",
		UserID:    user.ID,
		TokenHash: "runtime-terminal-token-hash",
		ExpiresAt: now.Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	assertion := model.StepUpAssertion{
		ID:                "mfaas_runtime_terminal",
		UserID:            user.ID,
		SessionID:         session.ID,
		Purpose:           stepUpPurposeRuntimeTerminal,
		VerifiedAt:        now,
		LastActivityAt:    now,
		IdleExpiresAt:     now.Add(time.Minute),
		AbsoluteExpiresAt: now.Add(2 * time.Minute),
	}
	if err := db.Create(&assertion).Error; err != nil {
		t.Fatal(err)
	}
	return runtimeTerminalAuthorizationBinding{
		UserID:                    user.ID,
		SubjectID:                 session.ID,
		AssertionID:               assertion.ID,
		AssertionRequired:         true,
		AssertionAbsoluteDeadline: assertion.AbsoluteExpiresAt,
		Deadline:                  assertion.AbsoluteExpiresAt,
	}, assertion
}
