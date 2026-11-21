package aiapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type stubMCPHost struct {
	stubAIToolHost
	authOK  bool
	user    model.User
	token   model.AccessToken
	audits  []string
	baseURL string
	mcpKey  []byte
}

func (h *stubMCPHost) AuthenticateMCPBearerToken(ctx *gin.Context) (model.User, model.AccessToken, bool) {
	if !h.authOK {
		return model.User{}, model.AccessToken{}, false
	}
	ctx.Set("luna.devops.current_user", h.user)
	ctx.Set("luna.devops.current_access_token", h.token)
	return h.user, h.token, true
}

func (h *stubMCPHost) ExternalBaseURL(*gin.Context) string { return h.baseURL }
func (h *stubMCPHost) MCPPreviewSigningSecret() []byte     { return h.mcpKey }
func (h *stubMCPHost) AppVersion() string                  { return "0.2.0-mcp" }
func (h *stubMCPHost) AuditWithContext(userID, action, resource string, success bool, _ string, _ context.Context) {
	if success {
		h.audits = append(h.audits, userID+":"+action+":"+resource)
	}
}

func TestStreamableHTTPMCPGatewayContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
		_ = tp.Shutdown(t.Context())
	})

	reg := aitool.NewRegistry()

	aitool.Register(reg, aitool.Spec[struct {
		Search *string `json:"search,omitempty"`
	}, map[string]any]{
		Name:           "workspace_list_projects",
		Summary:        "列出当前可见的项目空间",
		Domain:         aitool.DomainWorkspace,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{"project:read"},
		Execute: func(_ *aitool.ToolContext, _ struct {
			Search *string `json:"search,omitempty"`
		}) (map[string]any, error) {
			return map[string]any{"items": []string{"alpha-svc"}}, nil
		},
	})

	aitool.Register(reg, aitool.Spec[struct {
		Project string `json:"project"`
		Pinned  bool   `json:"pinned"`
	}, map[string]any]{
		Name:           "workspace_pin_project",
		Summary:        "置顶或取消置顶项目空间",
		Domain:         aitool.DomainWorkspace,
		ApprovalTier:   aitool.ApprovalTierL1,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{"project:read"},
		Verification:   []aitool.OperationVerification{{OperationID: "workspace_list_projects", Family: "resource"}},
		Execute: func(_ *aitool.ToolContext, in struct {
			Project string `json:"project"`
			Pinned  bool   `json:"pinned"`
		}) (map[string]any, error) {
			return map[string]any{"project": in.Project, "pinned": in.Pinned}, nil
		},
	})

	var capturedLockedRevision string
	aitool.Register(reg, aitool.Spec[struct {
		Project     string `json:"project"`
		IngressMode string `json:"ingressMode" enum:"isolated|dependencies"`
	}, map[string]any]{
		Name:           "workspace_configure_network_policy",
		Summary:        "配置项目网络入站与出站策略",
		Domain:         aitool.DomainWorkspace,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{"project:manage"},
		Verification:   []aitool.OperationVerification{{OperationID: "workspace_list_projects", Family: "resource"}},
		Preview: func(_ *aitool.ToolContext, in struct {
			Project     string `json:"project"`
			IngressMode string `json:"ingressMode" enum:"isolated|dependencies"`
		}) (*aitool.PreviewResult, error) {
			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        "Configure network policy for " + in.Project,
					TargetResource: "project_network_policy",
					Changes: []aitool.PreviewChange{
						{Field: "ingressMode", Before: "isolated", After: in.IngressMode, Action: "update"},
					},
				},
				LockedPayload: map[string]string{"expectedRevision": "rev_42"},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in struct {
			Project     string `json:"project"`
			IngressMode string `json:"ingressMode" enum:"isolated|dependencies"`
		}) (map[string]any, error) {
			var locked map[string]string
			if err := tc.UnmarshalLockedPayload(&locked); err != nil {
				return nil, err
			}
			capturedLockedRevision = locked["expectedRevision"]
			return map[string]any{"project": in.Project, "ingressMode": in.IngressMode}, nil
		},
	})

	aitool.Register(reg, aitool.Spec[struct{}, map[string]any]{
		Name:           "admin_list_ai_models",
		Summary:        "列出全局 AI 模型配置",
		Domain:         aitool.DomainAdmin,
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{"config:read"},
		Execute: func(_ *aitool.ToolContext, _ struct{}) (map[string]any, error) {
			return map[string]any{"items": []any{}}, nil
		},
	})

	host := &stubMCPHost{
		stubAIToolHost: stubAIToolHost{reg: reg},
		baseURL:        "https://devops.example.com",
		mcpKey:         []byte("unit-test-mcp-preview-secret-key"),
	}
	handler := New(host)
	router := gin.New()
	router.Use(telemetry.GinTracingMiddleware("luna-devops-api"))
	router.POST("/api/v1/mcp", handler.PostMCPStreamableHTTP)
	router.GET("/api/v1/mcp", handler.GetMCPStreamableHTTP)
	router.DELETE("/api/v1/mcp", handler.DeleteMCPStreamableHTTPSession)

	postMCP := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer lyd_secret_unit_token_do_not_leak")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1) Unauthenticated -> 401 + RFC 9728 WWW-Authenticate resource_metadata
	unauthRec := postMCP(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", unauthRec.Code)
	}
	if got := unauthRec.Header().Get("WWW-Authenticate"); !strings.Contains(got, `resource_metadata="https://devops.example.com/.well-known/oauth-protected-resource"`) {
		t.Fatalf("WWW-Authenticate = %q", got)
	}

	// Authenticate as standard user with read-only PAT (`project:read`)
	host.authOK = true
	host.user = model.User{ID: "usr_mcp_1", Role: authz.PlatformRoleUser, Language: "zh-CN"}
	host.token = model.AccessToken{ID: "tok_ro", UserID: "usr_mcp_1", Scope: "project:read", Source: model.AccessTokenSourcePersonal}

	// 2) GET -> 405, DELETE -> 204, notification -> 202, initialize -> 200
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/mcp", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/v1/mcp = %d, want 405", getRec.Code)
	}
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/mcp", nil)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/v1/mcp = %d, want 204", delRec.Code)
	}
	if rec := postMCP(`{"jsonrpc":"2.0","method":"notifications/initialized"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("notification = %d, want 202", rec.Code)
	}
	if rec := postMCP(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"luna-devops"`) || !strings.Contains(rec.Body.String(), "runtime_get_target_diagnostics") || !strings.Contains(rec.Body.String(), "delivery_deploy_target") {
		t.Fatalf("initialize = %d %s", rec.Code, rec.Body.String())
	}

	// 3) Skills & Docs via MCP meta-tools, resources, and prompts
	skillRec := postMCP(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"load_skill","arguments":{"skillIds":["gateway-topology","platform-admin"]}}}`)
	if skillRec.Code != http.StatusOK || !strings.Contains(skillRec.Body.String(), "clearUrlRewrite") {
		t.Fatalf("load_skill = %d %s", skillRec.Code, skillRec.Body.String())
	}
	resRec := postMCP(`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"luna://skills/luna-devops"}}`)
	if resRec.Code != http.StatusOK || !strings.Contains(resRec.Body.String(), "previewLock") {
		t.Fatalf("resources/read = %d %s", resRec.Code, resRec.Body.String())
	}
	promptRec := postMCP(`{"jsonrpc":"2.0","id":4,"method":"prompts/get","params":{"name":"application-deployment"}}`)
	if promptRec.Code != http.StatusOK || !strings.Contains(promptRec.Body.String(), "delivery_deploy_target") {
		t.Fatalf("prompts/get = %d %s", promptRec.Code, promptRec.Body.String())
	}
	docRec := postMCP(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_docs","arguments":{"query":"MCP","locale":"zh"}}}`)
	if docRec.Code != http.StatusOK || !strings.Contains(docRec.Body.String(), "zh/use/mcp") {
		t.Fatalf("search_docs = %d %s", docRec.Code, docRec.Body.String())
	}

	// 4) Non-admin user cannot see or execute adminOnly tools
	adminSearchRec := postMCP(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"search_tools","arguments":{"domain":"admin"}}}`)
	if !strings.Contains(adminSearchRec.Body.String(), `"total":0`) {
		t.Fatalf("non-admin saw admin tools: %s", adminSearchRec.Body.String())
	}

	// 5) L0 succeeds, L1 requires confirmed: true, L2 rejected on read-only PAT scope
	l0Rec := postMCP(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_list_projects","arguments":{}}}}`)
	if !strings.Contains(l0Rec.Body.String(), "alpha-svc") {
		t.Fatalf("L0 execute = %s", l0Rec.Body.String())
	}
	l1Unconfirmed := postMCP(`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_pin_project","arguments":{"project":"alpha-svc","pinned":true}}}}`)
	if !strings.Contains(l1Unconfirmed.Body.String(), "ai.tool_confirmation_required") {
		t.Fatalf("L1 without confirmed = %s", l1Unconfirmed.Body.String())
	}
	l1Confirmed := postMCP(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_pin_project","arguments":{"project":"alpha-svc","pinned":true},"confirmed":true}}}`)
	if strings.Contains(l1Confirmed.Body.String(), `"isError":true`) {
		t.Fatalf("L1 with confirmed failed = %s", l1Confirmed.Body.String())
	}
	l2ScopeDenied := postMCP(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"preview_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"dependencies"}}}}`)
	if !strings.Contains(l2ScopeDenied.Body.String(), "auth.token.scope_insufficient") {
		t.Fatalf("expected auth.token.scope_insufficient, got %s", l2ScopeDenied.Body.String())
	}

	// 6) Upgrade token scope -> L2 two-step preview_tool -> previewLock -> execute_tool
	host.token = model.AccessToken{ID: "tok_rw", UserID: "usr_mcp_1", Scope: "project:read,project:manage", Source: model.AccessTokenSourceOAuth}

	l2NoLock := postMCP(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"dependencies"}}}}`)
	if !strings.Contains(l2NoLock.Body.String(), "ai.tool_preview_lock_required") {
		t.Fatalf("expected ai.tool_preview_lock_required, got %s", l2NoLock.Body.String())
	}

	prevRec := postMCP(`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"preview_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"dependencies"}}}}`)
	var prevEnvelope struct {
		Result struct {
			StructuredContent struct {
				PreviewLock string `json:"previewLock"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(prevRec.Body.Bytes(), &prevEnvelope); err != nil || prevEnvelope.Result.StructuredContent.PreviewLock == "" {
		t.Fatalf("preview_tool failed: %s", prevRec.Body.String())
	}
	lock := prevEnvelope.Result.StructuredContent.PreviewLock

	// Tampered argument -> ai.preview_lock_mismatch
	tamperRec := postMCP(`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"isolated"},"previewLock":"` + lock + `"}}}`)
	if !strings.Contains(tamperRec.Body.String(), "ai.preview_lock_mismatch") {
		t.Fatalf("expected ai.preview_lock_mismatch, got %s", tamperRec.Body.String())
	}

	// Different token ID -> ai.preview_lock_mismatch
	host.token.ID = "tok_other"
	otherTokRec := postMCP(`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"dependencies"},"previewLock":"` + lock + `"}}}`)
	if !strings.Contains(otherTokRec.Body.String(), "ai.preview_lock_mismatch") {
		t.Fatalf("expected cross-token mismatch, got %s", otherTokRec.Body.String())
	}
	host.token.ID = "tok_rw"

	// Valid lock -> executes, passes LockedPayload ("rev_42"), writes AuditLog!
	execRec := postMCP(`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"dependencies"},"previewLock":"` + lock + `"}}}`)
	if strings.Contains(execRec.Body.String(), `"isError":true`) || capturedLockedRevision != "rev_42" {
		t.Fatalf("L2 execute failed: revision=%q body=%s", capturedLockedRevision, execRec.Body.String())
	}
	if len(host.audits) < 2 {
		t.Fatalf("expected audit logs for L1 & L2 writes, got %#v", host.audits)
	}

	// 7) Verify OTel Span contract (bounded names, TraceID propagation, zero credential leak)
	sawToolsCall := false
	for _, sp := range spanRecorder.Ended() {
		if sp.Name() == "mcp.tools_call" {
			sawToolsCall = true
			if sp.SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
				t.Fatalf("unexpected traceID %s", sp.SpanContext().TraceID().String())
			}
		}
		for _, attr := range sp.Attributes() {
			if strings.Contains(attr.Value.Emit(), "lyd_secret_unit_token_do_not_leak") {
				t.Fatalf("leaked bearer token in span %s attribute %s", sp.Name(), attr.Key)
			}
		}
	}
	if !sawToolsCall {
		t.Fatal("missing mcp.tools_call span")
	}
}
