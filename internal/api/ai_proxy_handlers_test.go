package api

import (
	"context"
	"encoding/base64"
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

type trackingReadCloser struct {
	reader *strings.Reader
	read   int
	closed bool
}

func (r *trackingReadCloser) Read(buffer []byte) (int, error) {
	count, err := r.reader.Read(buffer)
	r.read += count
	return count, err
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
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
		`{"input":{"parts":[{"type":"text","text":"diagnose"}]},"pageContext":{"projectId":"prj_visible"},"clientInstanceId":"browser-client-instance-1"}`,
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
		`{"operationId":"saveConfig","arguments":{"environment":[{"key":"DATABASE_PASSWORD","value":"database-password"}]},"message":"提交配置","clientInstanceId":"browser-client-instance-1"}`,
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
			`{"conversation":{"id":"aicnv_owned"},"turns":[],"eventCursors":[],"pageInfo":{"hasOlder":true,"olderCursor":"next-opaque-cursor"}}`,
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
		`{"input":{"parts":[{"type":"text","text":"read another project"}]},"pageContext":{"projectId":"prj_hidden"},"clientInstanceId":"browser-client-instance-2"}`,
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
	rawGrant, _ := forwarded["runActorGrant"].(string)
	internalKeys, err := aiagent.LoadInternalKeys()
	if err != nil {
		t.Fatal(err)
	}
	grant, err := aiagent.VerifyRunActorGrant(rawGrant, internalKeys.RunActorGrantSigningKey, time.Now())
	if err != nil {
		t.Fatalf("invalid authorization grant: %#v, error = %v", grant, err)
	}
	grantParts := strings.Split(rawGrant, ".")
	if len(grantParts) != 3 {
		t.Fatalf("invalid compact grant: %s", rawGrant)
	}
	payload, err := base64.RawURLEncoding.DecodeString(grantParts[1])
	if err != nil || strings.Contains(string(payload), `"projectId"`) {
		t.Fatalf("page project leaked into authorization grant: %s", payload)
	}
}

func TestAICapabilitiesDependOnPlatformConfigurationNotAgentHealth(t *testing.T) {
	fake := &fakeAIAgentClient{err: aiagent.ErrUnavailable}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/capabilities", handler.GetAICapabilities)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/capabilities", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":true,"maxInputBytes":1048576}` {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fake.calls != 0 {
		t.Fatalf("capabilities probed Agent %d times", fake.calls)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q", response.Header().Get("Cache-Control"))
	}

	handler.configs.set(aiMaxInputBytesConfigKey, "64")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/capabilities", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":true,"maxInputBytes":65536}` {
		t.Fatalf("configured capability limits not returned: status = %d, body = %s", response.Code, response.Body.String())
	}

	handler.configs.set(aiAssistantEnabledConfigKey, "false")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/capabilities", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":false,"maxInputBytes":65536}` {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	handler.configs.set(aiAssistantEnabledConfigKey, "true")
	handler.aiDeploymentEnabled = false
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/capabilities", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":false,"maxInputBytes":65536}` {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAIProxyBodyLimitUsesPlatformInputConfiguration(t *testing.T) {
	handler := aiTestHandlers(nil, true)
	handler.configs.set(aiMaxInputBytesConfigKey, "64")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations", strings.NewReader(strings.Repeat("x", 48*1024)))
	if _, ok := handler.readAIBody(ctx); !ok {
		t.Fatalf("48 KiB body rejected under 64 KiB limit: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	handler.configs.set(aiMaxInputBytesConfigKey, "8")
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations", strings.NewReader(strings.Repeat("x", 8*1024+1)))
	if _, ok := handler.readAIBody(ctx); ok || recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body accepted: status = %d, body = %s", recorder.Code, recorder.Body.String())
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

func TestPendingAIUIActionsDegradesWhenAgentIsUnavailable(t *testing.T) {
	tests := []struct {
		name   string
		client aiagent.Client
	}{
		{
			name:   "client is not configured",
			client: nil,
		},
		{
			name: "agent connection is unavailable",
			client: func() *fakeAIAgentClient {
				return &fakeAIAgentClient{err: aiagent.ErrUnavailable}
			}(),
		},
		{
			name: "agent reports a temporary server failure",
			client: func() *fakeAIAgentClient {
				return &fakeAIAgentClient{response: &aiagent.Response{
					StatusCode: http.StatusServiceUnavailable,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"code":"temporary"}`)),
				}}
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := aiTestHandlers(test.client, true)
			router := gin.New()
			router.GET("/api/v1/ai/ui-actions/pending", handler.ProxyAIRequest)

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/ai/ui-actions/pending?clientInstanceId=browser-client-instance-1", nil)
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Retry-After") != "" {
				t.Fatalf("status = %d, headers = %v, body = %s", response.Code, response.Header(), response.Body.String())
			}
			var body struct {
				Items             []any `json:"items"`
				AgentAvailable    bool  `json:"agentAvailable"`
				RetryAfterSeconds int   `json:"retryAfterSeconds"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Items == nil || len(body.Items) != 0 || body.AgentAvailable || body.RetryAfterSeconds != aiPendingUIActionsRetrySeconds {
				t.Fatalf("unexpected degraded response: %#v", body)
			}
		})
	}
}

func TestPendingAIUIActionsForwardsHealthyAgentResponse(t *testing.T) {
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"items":[{"actionId":"aiuia_1"}]}`)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/ui-actions/pending", handler.ProxyAIRequest)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ai/ui-actions/pending?clientInstanceId=browser-client-instance-1", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != `{"items":[{"actionId":"aiuia_1"}]}` {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Retry-After") != "" || fake.calls != 1 || fake.request.Path != "/internal/v1/ui-actions/pending" {
		t.Fatalf("headers = %v, calls = %d, request = %#v", response.Header(), fake.calls, fake.request)
	}
}

func TestPendingAIUIActionsForwardsAgentClientErrors(t *testing.T) {
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"code":"ai.client_instance_invalid"}`)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/ui-actions/pending", handler.ProxyAIRequest)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ai/ui-actions/pending?clientInstanceId=invalid", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d, headers = %v, body = %s", response.Code, response.Header(), response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"agentAvailable":false`) {
		t.Fatalf("client error was incorrectly degraded: %s", response.Body.String())
	}
}

func TestPendingAIUIActionsDrainsTemporaryFailureBodyWithinLimit(t *testing.T) {
	body := &trackingReadCloser{reader: strings.NewReader(strings.Repeat("x", aiPendingUIActionsDrainLimit+1024))}
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       body,
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/ui-actions/pending", handler.ProxyAIRequest)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/ui-actions/pending?clientInstanceId=browser-client-instance-1", nil))

	if response.Code != http.StatusOK || body.read != aiPendingUIActionsDrainLimit || !body.closed {
		t.Fatalf("status = %d, read = %d, closed = %t", response.Code, body.read, body.closed)
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
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":false,"maxInputBytes":1048576}` || fake.calls != 0 {
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
	t.Setenv("APP_ENV", "development")
	rawBody := `{"error":"provider connection failed","detail":"dial tcp internal-agent.local"}`
	fake := &fakeAIAgentClient{response: &aiagent.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(rawBody)),
	}}
	handler := aiTestHandlers(fake, true)
	router := gin.New()
	router.GET("/api/v1/ai/conversations", handler.ProxyAIRequest)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ai/conversations", nil))

	if response.Code != http.StatusBadGateway || response.Body.String() != rawBody {
		t.Fatalf("development proxy response = %d %s", response.Code, response.Body.String())
	}
}

func TestAIProviderConnectionResponseSanitizesUpstreamFailureInProduction(t *testing.T) {
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
