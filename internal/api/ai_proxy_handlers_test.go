package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		`{"modelId":"aimod_test","input":{"parts":[{"type":"text","text":"diagnose"}]},"pageContext":{"projectId":"prj_visible"}}`,
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
	if runID == "" {
		t.Fatal("API must preallocate a stable runId")
	}
	if _, leaked := forwarded["runActorGrant"]; leaked {
		t.Fatalf("legacy Run Actor Grant leaked into Agent request: %#v", forwarded)
	}
	pageContext, _ := forwarded["pageContext"].(map[string]any)
	serverContext, _ := pageContext["server"].(map[string]any)
	if pageContext["locale"] != "zh-CN" || serverContext["locale"] != "zh-CN" ||
		serverContext["projectContextPresent"] != true || serverContext["requestTimestamp"] == "" {
		t.Fatalf("enriched page context = %#v", pageContext)
	}
}

func TestAIProxyForwardsInteractionCardToolActionWithoutConvertingArgumentsToChatInput(t *testing.T) {
	t.Setenv("AI_INTERNAL_SECRET", "test-ai-internal-secret-32-bytes-minimum")
	gin.SetMode(gin.TestMode)
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"turnId":"aitrn_1","runId":"airun_1"}`)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.POST("/api/v1/ai/conversations/:conversationId/tool-actions", handler.ProxyAIRequest)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/aicnv_owned/tool-actions", strings.NewReader(
		`{"operationId":"saveConfig","arguments":{"environment":[{"key":"DATABASE_PASSWORD","value":"database-password"}]},"message":"提交配置"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "tool-action-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fake.request.Path != "/internal/v1/conversations/aicnv_owned/tool-actions" || fake.request.IdempotencyKey != "tool-action-1" {
		t.Fatalf("agent request = %#v", fake.request)
	}
	var forwarded map[string]any
	if err := json.Unmarshal(fake.request.Body, &forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded["runId"] == "" || forwarded["runActorGrant"] == "" {
		t.Fatalf("secure action was not bound to a run grant: %#v", forwarded)
	}
	arguments, _ := forwarded["arguments"].(map[string]any)
	environment, _ := arguments["environment"].([]any)
	entry, _ := environment[0].(map[string]any)
	if entry["value"] != "database-password" {
		t.Fatalf("tool action arguments were filtered before Agent execution: %#v", forwarded)
	}
}

func TestAIProxyForwardsTimelineCursorPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"conversation":{"id":"aicnv_owned"},"contextUsage":{"status":"reported","runId":"airun_previous","modelId":"aimod_test","usedTokens":26112,"maxContextTokensSnapshot":128000,"recordedAt":"2026-08-24T00:00:00Z"},"turns":[],"eventCursors":[],"pageInfo":{"hasOlder":true,"olderCursor":"next-opaque-cursor"}}`,
		)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/conversations/:conversationId/timeline", handler.ProxyAIRequest)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/ai/conversations/aicnv_owned/timeline?before=opaque-cursor&limit=30",
		nil,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fake.request.Path != "/internal/v1/conversations/aicnv_owned/timeline" {
		t.Fatalf("agent request path = %q", fake.request.Path)
	}
	if fake.request.Query.Get("before") != "opaque-cursor" || fake.request.Query.Get("limit") != "30" {
		t.Fatalf("agent request query = %#v", fake.request.Query)
	}
	if !strings.Contains(response.Body.String(), `"olderCursor":"next-opaque-cursor"`) {
		t.Fatalf("timeline page response = %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"usedTokens":26112`) {
		t.Fatalf("timeline context usage was not forwarded: %s", response.Body.String())
	}
}

func TestAIProxyForwardsConversationDirectorySearchAndSort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"items":[],"page":2,"pageSize":20,"sortBy":"updatedAt","sortOrder":"asc","total":0,"totalPages":0}`)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/conversations", handler.ProxyAIRequest)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/ai/conversations?page=2&pageSize=20&search=deploy&sortBy=updatedAt&sortOrder=asc",
		nil,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fake.request.Path != "/internal/v1/conversations" || fake.request.Query.Get("page") != "2" ||
		fake.request.Query.Get("pageSize") != "20" || fake.request.Query.Get("search") != "deploy" ||
		fake.request.Query.Get("sortBy") != "updatedAt" || fake.request.Query.Get("sortOrder") != "asc" {
		t.Fatalf("agent request = %#v", fake.request)
	}
}

func TestAIProxyForwardsConversationScopedModelUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"aicnv_owned","modelId":"aimod_deep"}`)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.PATCH("/api/v1/ai/conversations/:conversationId", handler.ProxyAIRequest)

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/ai/conversations/aicnv_owned", strings.NewReader(`{"modelId":"aimod_deep"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || fake.request.Path != "/internal/v1/conversations/aicnv_owned" {
		t.Fatalf("status = %d, request = %#v, body = %s", response.Code, fake.request, response.Body.String())
	}
	if string(fake.request.Body) != `{"modelId":"aimod_deep"}` {
		t.Fatalf("forwarded body = %s", fake.request.Body)
	}
}

func TestAIProxyRequiresInitialConversationModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeAIAgentClient{}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.POST("/api/v1/ai/conversations", handler.ProxyAIRequest)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations", strings.NewReader(`{"title":"Missing model"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "ai.model_required") || fake.calls != 0 {
		t.Fatalf("status = %d, calls = %d, body = %s", response.Code, fake.calls, response.Body.String())
	}
}

func TestAIProxyKeepsSessionActorWhenPayloadContainsIdentityShapedData(t *testing.T) {
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusCreated,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"aicnv_created"}`)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.POST("/api/v1/ai/conversations", handler.ProxyAIRequest)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations", strings.NewReader(
		`{"title":"identity data","modelId":"aimod_test","metadata":{"userId":"usr_other"}}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fake.calls != 1 || fake.actor.UserID != "usr_session_owner" || !strings.Contains(string(fake.request.Body), `"userId":"usr_other"`) {
		t.Fatalf("calls=%d actor=%#v body=%s", fake.calls, fake.actor, fake.request.Body)
	}
}

func TestAIProxyTreatsPageProjectAsContextInsteadOfAuthorizationBoundary(t *testing.T) {
	t.Setenv("AI_INTERNAL_SECRET", "test-ai-internal-secret-32-bytes-minimum")
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"turnId":"aitrn_context","runId":"airun_context"}`)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.POST("/api/v1/ai/conversations/:conversationId/turns", handler.ProxyAIRequest)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/aicnv_old/turns", strings.NewReader(
		`{"modelId":"aimod_test","input":{"parts":[{"type":"text","text":"read another project"}]},"pageContext":{"projectId":"prj_hidden"}}`,
	))
	request.Header.Set("Idempotency-Key", "msg_hidden")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || fake.calls != 1 {
		t.Fatalf("status = %d, agent calls = %d, body = %s", response.Code, fake.calls, response.Body.String())
	}
	var forwarded map[string]any
	if err := json.Unmarshal(fake.request.Body, &forwarded); err != nil {
		t.Fatal(err)
	}
	if _, leaked := forwarded["runActorGrant"]; leaked {
		t.Fatalf("page project must remain context, not a portable grant: %#v", forwarded)
	}
}

func TestAICapabilitiesDependOnPlatformConfigurationNotAgentHealth(t *testing.T) {
	fake := &fakeAIAgentClient{err: aiagent.ErrUnavailable}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/capabilities", handler.GetAICapabilities)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/capabilities", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":true,"maxInputBytes":131072}` {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fake.calls != 0 {
		t.Fatalf("capabilities probed Agent %d times", fake.calls)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q", response.Header().Get("Cache-Control"))
	}

	handler.configs.set(aiAssistantEnabledConfigKey, "false")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/capabilities", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":false,"maxInputBytes":131072}` {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	handler.configs.set(aiAssistantEnabledConfigKey, "true")
	handler.aiDeploymentEnabled = false
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/capabilities", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":false,"maxInputBytes":131072}` {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAIProxyUsesFixedEnvelopeBodyLimit(t *testing.T) {
	handler := aiTestHandlers(nil, true)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations", strings.NewReader(strings.Repeat("x", aiRequestBodyLimitBytes)))
	if _, ok := handler.readAIBody(ctx); !ok {
		t.Fatalf("body at transport limit rejected: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations", strings.NewReader(strings.Repeat("x", aiRequestBodyLimitBytes+1)))
	if _, ok := handler.readAIBody(ctx); ok || recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body accepted: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestAIProxyRejectsOversizedTextInsideAllowedEnvelope(t *testing.T) {
	oversized := strings.Repeat("x", aiTextInputLimitBytes+1)
	tests := []struct {
		name         string
		routePattern string
		path         string
		body         any
	}{
		{name: "turn", routePattern: "/api/v1/ai/conversations/:conversationId/turns", path: "/api/v1/ai/conversations/aicnv_1/turns", body: map[string]any{
			"modelId": "aimod_test", "input": map[string]any{"parts": []any{map[string]any{"type": "text", "text": oversized}}},
		}},
		{name: "turn part separator", routePattern: "/api/v1/ai/conversations/:conversationId/turns", path: "/api/v1/ai/conversations/aicnv_1/turns", body: map[string]any{
			"modelId": "aimod_test", "input": map[string]any{"parts": []any{
				map[string]any{"type": "text", "text": strings.Repeat("x", aiTextInputLimitBytes-1)},
				map[string]any{"type": "text", "text": "y"},
			}},
		}},
		{name: "tool action", routePattern: "/api/v1/ai/conversations/:conversationId/tool-actions", path: "/api/v1/ai/conversations/aicnv_1/tool-actions", body: map[string]any{
			"operationId": "saveConfig", "arguments": map[string]any{}, "message": oversized,
		}},
		{name: "run input", routePattern: "/api/v1/ai/runs/:runId/input", path: "/api/v1/ai/runs/airun_1/input", body: map[string]any{
			"text": oversized, "expectedVersion": 1,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatal(err)
			}
			fake := &fakeAIAgentClient{}
			handler := aiTestHandlers(fake, true)
			router := gin.New()
			router.POST(tt.routePattern, handler.ProxyAIRequest)
			request := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(string(encoded)))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "input-limit-test")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusRequestEntityTooLarge || fake.calls != 0 || !strings.Contains(response.Body.String(), "ai.input_too_large") {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, fake.calls, response.Body.String())
			}
		})
	}
}

func TestAIProxyStillReportsUnavailableAgentWhenEnabled(t *testing.T) {
	handler := aiTestHandlers(nil, true)
	router := gin.New()
	router.GET("/api/v1/ai/conversations", handler.ProxyAIRequest)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/conversations", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"ai.agent_unavailable"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAIAccessModeDefaultsToAuthenticatedUsersAndCanRestrictAdmins(t *testing.T) {
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"enabled":true}`)),
	}}
	handler := aiTestHandlers(fake, true)
	if !handler.aiAccessAllowed("user") || !handler.aiAccessAllowed("platform_admin") {
		t.Fatal("default authenticated access did not allow valid platform roles")
	}

	handler.configs.set(aiAccessModeConfigKey, "admins")
	if handler.aiAccessAllowed("user") || !handler.aiAccessAllowed("platform_admin") {
		t.Fatal("admin-only access mode was not enforced")
	}

	router := gin.New()
	router.GET("/api/v1/ai/capabilities", handler.GetAICapabilities)
	router.GET("/api/v1/ai/conversations", handler.ProxyAIRequest)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/capabilities", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":false,"maxInputBytes":131072}` || fake.calls != 0 {
		t.Fatalf("status = %d, calls = %d, body = %s", response.Code, fake.calls, response.Body.String())
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/conversations", nil))
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"code":"ai.user_not_allowed"`) || fake.calls != 0 {
		t.Fatalf("status = %d, calls = %d, body = %s", response.Code, fake.calls, response.Body.String())
	}
}

func TestAIProxyFlushesSSEChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	streamBody := "event: stream.heartbeat\ndata: {\"version\":1,\"type\":\"stream.heartbeat\",\"runId\":\"airun_stream\",\"conversationId\":\"aicnv_stream\",\"occurredAt\":\"2026-08-24T00:00:00Z\"}\n\nevent: content.delta\ndata: {\"delta\":\"hello\"}\n\n"
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
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
	if response.Body.String() != streamBody {
		t.Fatalf("SSE frames changed while proxying: %q", response.Body.String())
	}
}

func TestAIProxySanitizesNonSuccessSSEBeforeStreamingInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	gin.SetMode(gin.TestMode)
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("event: error\ndata: provider token=secret url=http://internal-agent.local\n\n")),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/conversations", handler.ProxyAIRequest)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/conversations", nil))

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Flushed || strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("production error started an SSE response: headers=%v", response.Header())
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode safe error: %v", err)
	}
	if body["code"] != "ai.agent_unavailable" || !strings.HasPrefix(body["requestId"], "req_") {
		t.Fatalf("unexpected safe error: %#v", body)
	}
	if strings.Contains(response.Body.String(), "token=secret") || strings.Contains(response.Body.String(), "internal-agent") {
		t.Fatalf("production proxy leaked upstream body: %s", response.Body.String())
	}
}

func TestAIProxyKeepsNonSuccessBodyInDevelopment(t *testing.T) {
	rawBody := `{"error":"provider connection failed","detail":"dial tcp internal-agent.local"}`
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(rawBody)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.Use(runtimeModeMiddleware("development"))
	router.GET("/api/v1/ai/conversations", handler.ProxyAIRequest)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/conversations", nil))

	if response.Code != http.StatusBadGateway || response.Body.String() != rawBody {
		t.Fatalf("development proxy response = %d %s", response.Code, response.Body.String())
	}
}

func TestAIProviderConnectionResponseSanitizesExternalErrorInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	response := &aiagent.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":"provider rejected token=secret"}`)),
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/configs/ai/provider/test", nil)

	(&Handlers{}).copyAIResponse(ctx, response, http.StatusOK, "ai.provider_unavailable")

	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode safe provider error: %v", err)
	}
	if recorder.Code != http.StatusServiceUnavailable || body["code"] != "ai.provider_unavailable" ||
		!strings.HasPrefix(body["requestId"], "req_") {
		t.Fatalf("unexpected provider error response: %d %#v", recorder.Code, body)
	}
	if strings.Contains(recorder.Body.String(), "token=secret") {
		t.Fatalf("production provider test leaked upstream body: %s", recorder.Body.String())
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
		"PATCH /api/v1/ai/conversations/:conversationId",
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
		configs:             &configCache{values: map[string]string{aiAssistantEnabledConfigKey: "true", aiAccessModeConfigKey: "all_authenticated"}},
		aiAgent:             client,
		aiDeploymentEnabled: enabled,
		aiActorResolver: func(*gin.Context) (aiagent.ActorContext, string, bool) {
			return aiagent.ActorContext{
				UserID: "usr_session_owner", SessionID: "sess_owned", Locale: "zh-CN", RequestID: "req_test",
			}, "user", true
		},
	}
}
