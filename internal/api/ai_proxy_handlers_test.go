package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/gin-gonic/gin"
)

type fakeAIAgentClient struct {
	actor    aiagent.ActorContext
	request  aiagent.Request
	response *aiagent.Response
	err      error
	calls    int
}

func (f *fakeAIAgentClient) Do(_ context.Context, actor aiagent.ActorContext, request aiagent.Request) (*aiagent.Response, error) {
	f.calls++
	f.actor = actor
	f.request = request
	return f.response, f.err
}

func TestAIProxyUsesSessionActorAndForwardsIdempotencyKey(t *testing.T) {
	t.Setenv("AI_INTERNAL_SECRET", "test-ai-internal-secret-32-bytes-minimum")
	gin.SetMode(gin.TestMode)
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"turnId":"aitrn_1","runId":"airun_1"}`)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.POST("/api/v1/ai/conversations/:conversationId/turns", handler.ProxyAIRequest)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/aicnv_owned/turns", strings.NewReader(
		`{"input":{"parts":[{"type":"text","text":"diagnose"}]},"pageContext":{"projectId":"prj_visible"}}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "msg_1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fake.actor.UserID != "usr_session_owner" || fake.actor.SessionID != "sess_owned" {
		t.Fatalf("agent actor = %#v", fake.actor)
	}
	if fake.actor.ProjectID != "prj_visible" {
		t.Fatalf("project ID = %q", fake.actor.ProjectID)
	}
	if fake.request.IdempotencyKey != "msg_1" || fake.request.Path != "/internal/v1/conversations/aicnv_owned/turns" {
		t.Fatalf("agent request = %#v", fake.request)
	}
	var forwarded map[string]any
	if err := json.Unmarshal(fake.request.Body, &forwarded); err != nil {
		t.Fatal(err)
	}
	runID, _ := forwarded["runId"].(string)
	rawGrant, _ := forwarded["runActorGrant"].(string)
	internalKeys, keyErr := aiagent.LoadInternalKeys()
	if keyErr != nil {
		t.Fatal(keyErr)
	}
	grant, err := aiagent.VerifyRunActorGrant(rawGrant, internalKeys.RunActorGrantSigningKey, time.Now())
	if err != nil || runID == "" || grant.RunID != runID || grant.UserID != "usr_session_owner" {
		t.Fatalf("forwarded Run Actor Grant = %#v, error = %v", grant, err)
	}
	pageContext, _ := forwarded["pageContext"].(map[string]any)
	serverContext, _ := pageContext["server"].(map[string]any)
	if pageContext["locale"] != "zh-CN" || serverContext["locale"] != "zh-CN" ||
		serverContext["projectAuthorized"] != true || serverContext["requestTimestamp"] == "" {
		t.Fatalf("enriched page context = %#v", pageContext)
	}
}

func TestAIProxyRejectsBrowserSuppliedActorIdentity(t *testing.T) {
	fake := &fakeAIAgentClient{}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.POST("/api/v1/ai/conversations", handler.ProxyAIRequest)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations", strings.NewReader(
		`{"title":"forged","userId":"usr_other"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "ai.actor_field_forbidden") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fake.calls != 0 {
		t.Fatalf("agent called %d times for forged actor", fake.calls)
	}
}

func TestAIProxyRejectsProjectOutsideCurrentUserBoundary(t *testing.T) {
	t.Setenv("AI_INTERNAL_SECRET", "test-ai-internal-secret-32-bytes-minimum")
	fake := &fakeAIAgentClient{}
	handler := aiTestHandlers(fake, true)
	handler.aiProjectAuthorizer = func(ctx *gin.Context, projectID string) bool {
		if projectID != "prj_hidden" {
			t.Fatalf("project ID = %q", projectID)
		}
		writeErrorCode(ctx, http.StatusNotFound, "project.not_found", "project not found")
		return false
	}
	router := gin.New()
	router.POST("/api/v1/ai/conversations/:conversationId/turns", handler.ProxyAIRequest)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/aicnv_old/turns", strings.NewReader(
		`{"input":{"parts":[{"type":"text","text":"read another project"}]},"pageContext":{"projectId":"prj_hidden"}}`,
	))
	request.Header.Set("Idempotency-Key", "msg_hidden")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound || fake.calls != 0 {
		t.Fatalf("status = %d, agent calls = %d, body = %s", response.Code, fake.calls, response.Body.String())
	}
}

func TestAICapabilitiesFailClosedWithoutAgent(t *testing.T) {
	handler := aiTestHandlers(nil, true)
	router := gin.New()
	router.GET("/api/v1/ai/capabilities", handler.GetAICapabilities)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/capabilities", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":false`) ||
		!strings.Contains(response.Body.String(), `"reasonCode":"ai.agent_unavailable"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAIProxyFlushesSSEChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("event: content.delta\ndata: {\"delta\":\"hello\"}\n\n")),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/runs/:runId/events", handler.ProxyAIRequest)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ai/runs/airun_stream/events", nil)
	request.Header.Set("Accept", "text/event-stream")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !response.Flushed {
		t.Fatalf("status = %d, flushed = %v, body = %s", response.Code, response.Flushed, response.Body.String())
	}
	if fake.request.LastEventID != "" || !fake.request.Stream {
		t.Fatalf("agent stream request = %#v", fake.request)
	}
}

func TestAIProxyDropsJSONContentTypeForBodylessCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"airun_cancel","status":"canceled"}`)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.POST("/api/v1/ai/runs/:runId/cancel", handler.ProxyAIRequest)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/runs/airun_cancel/cancel", nil)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fake.request.ContentType != "" || len(fake.request.Body) != 0 {
		t.Fatalf("agent cancel request = %#v", fake.request)
	}
}

func TestAIRouteContractContainsP0Endpoints(t *testing.T) {
	expected := []string{
		"GET /api/v1/ai/conversations",
		"POST /api/v1/ai/conversations",
		"GET /api/v1/ai/conversations/:conversationId/timeline",
		"POST /api/v1/ai/conversations/:conversationId/turns",
		"POST /api/v1/ai/turns/:turnId/runs",
		"GET /api/v1/ai/runs/:runId/events",
		"POST /api/v1/ai/runs/:runId/cancel",
	}
	for _, operation := range expected {
		if _, ok := aiProxyRoutes[operation]; !ok {
			t.Errorf("missing AI proxy contract %s", operation)
		}
	}
}

func aiTestHandlers(client aiagent.Client, enabled bool) *Handlers {
	return &Handlers{
		configs:             &configCache{values: map[string]string{aiAssistantEnabledConfigKey: "true"}},
		aiAgent:             client,
		aiDeploymentEnabled: enabled,
		aiActorResolver: func(*gin.Context) (aiagent.ActorContext, bool) {
			return aiagent.ActorContext{
				UserID: "usr_session_owner", SessionID: "sess_owned", Locale: "zh-CN", RequestID: "req_test",
			}, true
		},
		aiProjectAuthorizer: func(*gin.Context, string) bool { return true },
	}
}
