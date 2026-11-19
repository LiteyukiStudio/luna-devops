package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestMCPStreamableHTTPEndToEnd(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(
		&model.ProjectPin{},
		&model.Application{},
		&model.DeploymentTarget{},
		&model.RuntimeCluster{},
		&model.NetworkEgressException{},
		&model.NetworkEgressExceptionGrant{},
		&model.NetworkServiceShare{},
	); err != nil {
		t.Fatalf("migrate MCP integration tables: %v", err)
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "https://devops.example.com")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mcp-integration-secret-key-32bytes")

	// Install an in-memory span recorder to verify W3C propagation, bounded Span names, and zero secret leakage.
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

	user := model.User{
		ID:       "usr_mcp_owner",
		Email:    "mcp-owner@example.com",
		Name:     "MCP Owner",
		Role:     authz.PlatformRoleUser,
		Language: "zh-CN",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionPlain := "sess_mcp_cookie_must_not_auth"
	if err := db.Create(&model.UserSession{
		ID:        "ses_mcp_1",
		UserID:    user.ID,
		TokenHash: transportapi.HashToken(sessionPlain),
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}

	project := model.Project{
		ID:                 "prj_mcp_alpha",
		Name:               "Alpha 服务",
		Identifier:         "alpha-svc",
		Description:        "before-mcp",
		BillingOwnerUserID: user.ID,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ProjectMember{
		ID:        "pm_mcp_alpha",
		ProjectID: project.ID,
		UserID:    user.ID,
		Role:      authz.ProjectRoleOwner,
	}).Error; err != nil {
		t.Fatal(err)
	}

	readOnlyPAT := "lyd_readonly_secret_pat_token_123456"
	if err := db.Create(&model.AccessToken{
		ID:        "tok_mcp_ro",
		UserID:    user.ID,
		Name:      "MCP ReadOnly PAT",
		Scope:     "project:read",
		TokenHash: transportapi.HashToken(readOnlyPAT),
		Source:    model.AccessTokenSourcePersonal,
	}).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(db, mustTestConfig(t))

	// 1. Ambient cookie without Bearer MUST fail 401 with RFC 9728 WWW-Authenticate header.
	cookieReq := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	cookieReq.Header.Set("Content-Type", "application/json")
	cookieReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionPlain})
	cookieRec := httptest.NewRecorder()
	router.ServeHTTP(cookieRec, cookieReq)
	if cookieRec.Code != http.StatusUnauthorized {
		t.Fatalf("cookie-only /api/v1/mcp status = %d, want 401", cookieRec.Code)
	}
	wwwAuth := cookieRec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, `resource_metadata="https://devops.example.com/.well-known/oauth-protected-resource"`) {
		t.Fatalf("WWW-Authenticate = %q", wwwAuth)
	}

	// 2. RFC 9728 Protected Resource Metadata + RFC 8414 Authorization Server Metadata.
	for _, wellKnownPath := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/api/v1/mcp"} {
		wkRec := httptest.NewRecorder()
		router.ServeHTTP(wkRec, httptest.NewRequest(http.MethodGet, wellKnownPath, nil))
		if wkRec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", wellKnownPath, wkRec.Code, wkRec.Body.String())
		}
		var meta map[string]any
		if err := json.Unmarshal(wkRec.Body.Bytes(), &meta); err != nil {
			t.Fatal(err)
		}
		if meta["resource"] != "https://devops.example.com/api/v1/mcp" {
			t.Fatalf("protected resource = %#v", meta["resource"])
		}
	}

	callMCP := func(bearerToken, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+bearerToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 3. Initialize + Notifications (202 Accepted) + Tools/Resources/Prompts list.
	initRec := callMCP(readOnlyPAT, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if initRec.Code != http.StatusOK || !strings.Contains(initRec.Body.String(), `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("initialize = %d %s", initRec.Code, initRec.Body.String())
	}

	notifRec := callMCP(readOnlyPAT, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if notifRec.Code != http.StatusAccepted {
		t.Fatalf("notification status = %d, want 202", notifRec.Code)
	}

	listToolsRec := callMCP(readOnlyPAT, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if listToolsRec.Code != http.StatusOK {
		t.Fatalf("tools/list = %d %s", listToolsRec.Code, listToolsRec.Body.String())
	}
	for _, metaTool := range []string{"load_skill", "search_tools", "get_tool_details", "preview_tool", "execute_tool", "search_docs", "read_doc"} {
		if !strings.Contains(listToolsRec.Body.String(), `"`+metaTool+`"`) {
			t.Fatalf("tools/list missing %q: %s", metaTool, listToolsRec.Body.String())
		}
	}

	resListRec := callMCP(readOnlyPAT, `{"jsonrpc":"2.0","id":3,"method":"resources/list"}`)
	if resListRec.Code != http.StatusOK || !strings.Contains(resListRec.Body.String(), "luna://skills/domains/platform-admin") {
		t.Fatalf("resources/list = %d %s", resListRec.Code, resListRec.Body.String())
	}

	resReadRec := callMCP(readOnlyPAT, `{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"luna://skills/luna-devops"}}`)
	if resReadRec.Code != http.StatusOK || !strings.Contains(resReadRec.Body.String(), "previewLock") {
		t.Fatalf("resources/read = %d %s", resReadRec.Code, resReadRec.Body.String())
	}

	// 4. Non-admin search_tools hides adminOnly tools, marks L0 project:read allowed and write tools disallowed on readOnlyPAT.
	searchAdminRec := callMCP(readOnlyPAT, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_tools","arguments":{"domain":"admin"}}}`)
	if searchAdminRec.Code != http.StatusOK || !strings.Contains(searchAdminRec.Body.String(), `"total":0`) {
		t.Fatalf("non-admin saw admin tools: %d %s", searchAdminRec.Code, searchAdminRec.Body.String())
	}

	readProjRec := callMCP(readOnlyPAT, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_list_projects","arguments":{}}}}`)
	if readProjRec.Code != http.StatusOK || !strings.Contains(readProjRec.Body.String(), "alpha-svc") {
		t.Fatalf("L0 workspace_list_projects via PAT = %d %s", readProjRec.Code, readProjRec.Body.String())
	}

	deniedWriteRec := callMCP(readOnlyPAT, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_update_project","arguments":{"project":"alpha-svc","description":"blocked"}}}}`)
	if deniedWriteRec.Code != http.StatusOK || !strings.Contains(deniedWriteRec.Body.String(), "auth.token.scope_insufficient") {
		t.Fatalf("expected auth.token.scope_insufficient on read-only PAT, got %d %s", deniedWriteRec.Code, deniedWriteRec.Body.String())
	}

	// 5. Docs search & read via embedded docs.
	docSearchRec := callMCP(readOnlyPAT, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"search_docs","arguments":{"query":"MCP","locale":"zh"}}}`)
	if docSearchRec.Code != http.StatusOK || !strings.Contains(docSearchRec.Body.String(), "zh/use/mcp") {
		t.Fatalf("search_docs = %d %s", docSearchRec.Code, docSearchRec.Body.String())
	}

	// 6. Standard OAuth 2.0 Authorization Code + PKCE token -> L2 two-step previewLock enforcement + AuditLog.
	clientSecret := "mcp-oauth-client-secret-999"
	redirectURI := "http://127.0.0.1:44123/callback"
	oauthApp := model.OAuthApplication{
		ID:                      "oapp_mcp_client",
		OwnerUserID:             &user.ID,
		Name:                    "Claude Code MCP",
		ClientID:                "claude-code-mcp",
		ClientSecretHash:        transportapi.HashToken(clientSecret),
		RedirectURIs:            encodeStringList([]string{redirectURI}),
		AllowedScopes:           "project:read,project:write,project:manage",
		AccessTokenLifetimeDays: 7,
	}
	if err := db.Create(&oauthApp).Error; err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("k", 64)
	sum := sha256.Sum256([]byte(verifier))
	authBody, _ := json.Marshal(oauthAuthorizationDecisionInput{
		Approved:            true,
		ClientID:            oauthApp.ClientID,
		RedirectURI:         redirectURI,
		Scope:               "project:read,project:write,project:manage",
		CodeChallenge:       base64.RawURLEncoding.EncodeToString(sum[:]),
		CodeChallengeMethod: "S256",
	})
	consentRec := performCookieJSONRequest(router, http.MethodPost, "/api/v1/oauth/authorize", sessionPlain, string(authBody))
	if consentRec.Code != http.StatusOK {
		t.Fatalf("OAuth authorize = %d %s", consentRec.Code, consentRec.Body.String())
	}
	parsedRedirect, err := url.Parse(jsonString(t, consentRec.Body.Bytes(), "redirectUrl"))
	if err != nil {
		t.Fatal(err)
	}
	tokenRec := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {oauthApp.ClientID},
		"client_secret": {clientSecret},
		"code":          {parsedRedirect.Query().Get("code")},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("OAuth token exchange = %d %s", tokenRec.Code, tokenRec.Body.String())
	}
	var oauthTokens oauthTokenResponse
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &oauthTokens); err != nil {
		t.Fatal(err)
	}

	// workspace_create_config_set / delivery_create_config_set is L2 — or check workspace_update_project tier!
	detailsRec := callMCP(oauthTokens.AccessToken, `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"get_tool_details","arguments":{"toolNames":["workspace_update_project"]}}}`)
	if detailsRec.Code != http.StatusOK || !strings.Contains(detailsRec.Body.String(), `"tokenScopeAllowed":true`) {
		t.Fatalf("get_tool_details = %d %s", detailsRec.Code, detailsRec.Body.String())
	}

	// workspace_update_project is L2 (or L1); let's test an explicit L2 tool (`workspace_configure_network_policy`, requires project:manage -> let's also grant project:manage on a write PAT to test full L2 lock!).
	writePAT := "lyd_write_secret_pat_token_654321"
	if err := db.Create(&model.AccessToken{
		ID:        "tok_mcp_rw",
		UserID:    user.ID,
		Name:      "MCP ReadWrite PAT",
		Scope:     "project:read,project:write,project:manage",
		TokenHash: transportapi.HashToken(writePAT),
		Source:    model.AccessTokenSourcePersonal,
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Calling L2 workspace_configure_network_policy WITHOUT previewLock MUST fail with ai.tool_preview_lock_required.
	noLockRec := callMCP(writePAT, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"dependencies"}}}}`)
	if noLockRec.Code != http.StatusOK || !strings.Contains(noLockRec.Body.String(), "ai.tool_preview_lock_required") {
		t.Fatalf("expected ai.tool_preview_lock_required, got %d %s", noLockRec.Code, noLockRec.Body.String())
	}

	// Step 1: preview_tool issues previewLock + before/after Diff.
	previewRec := callMCP(writePAT, `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"preview_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"dependencies"}}}}`)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("preview_tool = %d %s", previewRec.Code, previewRec.Body.String())
	}
	var previewRPC struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				PreviewLock string `json:"previewLock"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(previewRec.Body.Bytes(), &previewRPC); err != nil || previewRPC.Result.IsError || previewRPC.Result.StructuredContent.PreviewLock == "" {
		t.Fatalf("unexpected preview_tool body: %s", previewRec.Body.String())
	}
	lock := previewRPC.Result.StructuredContent.PreviewLock

	// Tampered arguments with the same previewLock MUST fail with ai.preview_lock_mismatch.
	tamperRec := callMCP(writePAT, `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"isolated"},"previewLock":"`+lock+`"}}}`)
	if !strings.Contains(tamperRec.Body.String(), "ai.preview_lock_mismatch") {
		t.Fatalf("expected ai.preview_lock_mismatch on tampered args, got %s", tamperRec.Body.String())
	}

	// Cross-token reuse of previewLock (using OAuth token with writePAT's lock) MUST fail!
	crossTokenRec := callMCP(oauthTokens.AccessToken, `{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"dependencies"},"previewLock":"`+lock+`"}}}`)
	if !strings.Contains(crossTokenRec.Body.String(), "ai.preview_lock_mismatch") {
		t.Fatalf("expected cross-token previewLock rejection, got %s", crossTokenRec.Body.String())
	}

	// Step 2: execute_tool with matching previewLock succeeds and records AuditLog!
	execRec := callMCP(writePAT, `{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_configure_network_policy","arguments":{"project":"alpha-svc","ingressMode":"dependencies"},"previewLock":"`+lock+`"}}}`)
	if execRec.Code != http.StatusOK || strings.Contains(execRec.Body.String(), `"isError":true`) {
		t.Fatalf("L2 execute_tool failed: %d %s", execRec.Code, execRec.Body.String())
	}

	var updatedProj model.Project
	if err := db.First(&updatedProj, "id = ?", project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedProj.NetworkIngressMode != "dependencies" {
		t.Fatalf("updated NetworkIngressMode = %q, want dependencies", updatedProj.NetworkIngressMode)
	}

	var auditCount int64
	if err := db.Model(&model.AuditLog{}).Where("user_id = ? AND action = ? AND success = ?", user.ID, "mcp.tool.execute", true).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount == 0 {
		t.Fatal("expected mcp.tool.execute audit log entry")
	}

	// 7. Verify OTel spans: bounded Span names, propagated TraceID, and zero leaked PATs/secrets!
	ended := spanRecorder.Ended()
	sawMCPToolsCall := false
	for _, sp := range ended {
		if sp.Name() == "mcp.tools_call" {
			sawMCPToolsCall = true
			if sp.SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
				t.Fatalf("mcp.tools_call traceID = %s, want propagated traceID", sp.SpanContext().TraceID().String())
			}
		}
		for _, attr := range sp.Attributes() {
			val := attr.Value.Emit()
			if strings.Contains(val, readOnlyPAT) || strings.Contains(val, writePAT) || strings.Contains(val, clientSecret) {
				t.Fatalf("OTel span %q attribute %q leaked credential: %s", sp.Name(), attr.Key, val)
			}
		}
	}
	if !sawMCPToolsCall {
		t.Fatal("expected mcp.tools_call child span")
	}
}
