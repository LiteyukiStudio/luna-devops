package runtimeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRuntimeTerminalBearerTransportRequiresOneTimeTicket(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/clusters/rcl_test/pods/terminal", nil)
	ctx.Request.Header.Set("Authorization", "Bearer lyo_test")

	if requireRuntimeTerminalTicketForBearer(ctx, "") {
		t.Fatal("bearer WebSocket transport without a one-time ticket was accepted")
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusForbidden || response["code"] != "runtime_terminal.ticket_required" {
		t.Fatalf("missing bearer terminal ticket = status %d, body %#v", recorder.Code, response)
	}

	withTicketRecorder := httptest.NewRecorder()
	withTicket, _ := gin.CreateTestContext(withTicketRecorder)
	withTicket.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/clusters/rcl_test/pods/terminal?ticket=ticket_test", nil)
	withTicket.Request.Header.Set("Authorization", "Bearer lyo_test")
	if !requireRuntimeTerminalTicketForBearer(withTicket, "ticket_test") {
		t.Fatal("bearer WebSocket transport with a one-time ticket was rejected")
	}

	browserRecorder := httptest.NewRecorder()
	browser, _ := gin.CreateTestContext(browserRecorder)
	browser.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/clusters/rcl_test/pods/terminal", nil)
	if !requireRuntimeTerminalTicketForBearer(browser, "") {
		t.Fatal("browser session-cookie WebSocket flow should remain available without a ticket")
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
