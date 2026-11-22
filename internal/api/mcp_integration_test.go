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

func TestMCPDynamicClientRegistrationPublicClientEndToEnd(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(&model.ProjectPin{}); err != nil {
		t.Fatalf("migrate ProjectPin: %v", err)
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "https://devops.example.com")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mcp-dcr-integration-secret-32byt")

	user := model.User{
		ID:       "usr_mcp_dcr_dev",
		Email:    "mcp-dcr-dev@example.com",
		Name:     "MCP Developer",
		Role:     authz.PlatformRoleUser,
		Language: "zh-CN",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionPlain := "sess_mcp_dcr_browser_login"
	if err := db.Create(&model.UserSession{
		ID:        "ses_mcp_dcr_1",
		UserID:    user.ID,
		TokenHash: transportapi.HashToken(sessionPlain),
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	project := model.Project{
		ID:                 "prj_mcp_dcr",
		Name:               "DCR 项目",
		Identifier:         "dcr-proj",
		BillingOwnerUserID: user.ID,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ProjectMember{
		ID:        "pm_mcp_dcr",
		ProjectID: project.ID,
		UserID:    user.ID,
		Role:      authz.ProjectRoleOwner,
	}).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(db, mustTestConfig(t))

	// 1. Discover protected resource metadata -> authorization server metadata.
	prmRec := httptest.NewRecorder()
	router.ServeHTTP(prmRec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil))
	if prmRec.Code != http.StatusOK {
		t.Fatalf("protected resource metadata = %d %s", prmRec.Code, prmRec.Body.String())
	}
	var prm oauthProtectedResourceMetadataResponse
	if err := json.Unmarshal(prmRec.Body.Bytes(), &prm); err != nil {
		t.Fatal(err)
	}
	if len(prm.AuthorizationServers) != 1 || prm.AuthorizationServers[0] != "https://devops.example.com" {
		t.Fatalf("authorization_servers = %#v", prm.AuthorizationServers)
	}

	asmRec := httptest.NewRecorder()
	router.ServeHTTP(asmRec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if asmRec.Code != http.StatusOK {
		t.Fatalf("authorization server metadata = %d %s", asmRec.Code, asmRec.Body.String())
	}
	var asm oauthAuthorizationServerMetadataResponse
	if err := json.Unmarshal(asmRec.Body.Bytes(), &asm); err != nil {
		t.Fatal(err)
	}
	if asm.RegistrationEndpoint != "https://devops.example.com/api/v1/oauth/register" {
		t.Fatalf("registration_endpoint = %q", asm.RegistrationEndpoint)
	}
	if !containsString(asm.TokenEndpointAuthMethodsSupported, "none") ||
		!containsString(asm.TokenEndpointAuthMethodsSupported, "client_secret_basic") ||
		!containsString(asm.TokenEndpointAuthMethodsSupported, "client_secret_post") {
		t.Fatalf("token_endpoint_auth_methods_supported = %#v", asm.TokenEndpointAuthMethodsSupported)
	}

	// 2. RFC 7591 Dynamic Client Registration as a public client ("none").
	redirectURI := "http://127.0.0.1:53124/oauth/callback"
	regPayload := `{"client_name":"Antigravity MCP","redirect_uris":["` + redirectURI + `"],"token_endpoint_auth_method":"none","grant_types":["authorization_code","refresh_token"],"response_types":["code"]}`
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/register", strings.NewReader(regPayload))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/oauth/register = %d %s", regRec.Code, regRec.Body.String())
	}
	if regRec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", regRec.Header().Get("Cache-Control"))
	}
	var regResp oauthDynamicClientRegistrationResponse
	if err := json.Unmarshal(regRec.Body.Bytes(), &regResp); err != nil {
		t.Fatal(err)
	}
	if regResp.ClientID == "" || regResp.ClientSecret != "" || regResp.ClientSecretExpiresAt != nil || regResp.TokenEndpointAuthMethod != "none" {
		t.Fatalf("unexpected public client registration response: %#v", regResp)
	}

	// Anonymous dynamically registered clients must not pollute user's owned OAuth applications list.
	appsRec := performCookieJSONRequest(router, http.MethodGet, "/api/v1/oauth/applications", sessionPlain, "")
	if appsRec.Code != http.StatusOK || strings.Contains(appsRec.Body.String(), regResp.ClientID) {
		t.Fatalf("ListOAuthApplications leaked dynamic client: %d %s", appsRec.Code, appsRec.Body.String())
	}

	// 3. Browser /oauth/authorize with PKCE S256 (even when MCP client passes full scopes_supported for a non-admin user, or omits scope).
	verifier := strings.Repeat("p", 64)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authPreviewQuery := url.Values{
		"response_type":         {"code"},
		"client_id":             {regResp.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(prm.ScopesSupported, " ")},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	previewAuthRec := performCookieJSONRequest(router, http.MethodGet, "/api/v1/oauth/authorize?"+authPreviewQuery.Encode(), sessionPlain, "")
	if previewAuthRec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/oauth/authorize = %d %s", previewAuthRec.Code, previewAuthRec.Body.String())
	}
	var authPreview oauthAuthorizationRequest
	if err := json.Unmarshal(previewAuthRec.Body.Bytes(), &authPreview); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(authPreview.Scope, "auth:manage") || !strings.Contains(authPreview.Scope, "project:read") {
		t.Fatalf("expected non-admin scope narrowing on dynamic client, got %q", authPreview.Scope)
	}
	if authPreview.Application.RegistrationSource != "dynamic" {
		t.Fatalf("expected registrationSource = dynamic on consent preview, got %q", authPreview.Application.RegistrationSource)
	}

	// Missing response_type on GET /api/v1/oauth/authorize must fail 400.
	noRespTypeQuery := url.Values{
		"client_id":             {regResp.ClientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	if noRT := performCookieJSONRequest(router, http.MethodGet, "/api/v1/oauth/authorize?"+noRespTypeQuery.Encode(), sessionPlain, ""); noRT.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on missing response_type, got %d %s", noRT.Code, noRT.Body.String())
	}

	// Missing PKCE must be rejected even for public clients.
	missingPKCEBody, _ := json.Marshal(oauthAuthorizationDecisionInput{
		Approved:            true,
		ClientID:            regResp.ClientID,
		RedirectURI:         redirectURI,
		Scope:               authPreview.Scope,
		CodeChallenge:       "short",
		CodeChallengeMethod: "S256",
	})
	if badPKCE := performCookieJSONRequest(router, http.MethodPost, "/api/v1/oauth/authorize", sessionPlain, string(missingPKCEBody)); badPKCE.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid PKCE, got %d %s", badPKCE.Code, badPKCE.Body.String())
	}

	// POST /api/v1/oauth/authorize with Approved=true must reject un-narrowed admin scopes for non-admin user (no silent expansion).
	escalatedApproveBody, _ := json.Marshal(oauthAuthorizationDecisionInput{
		Approved:            true,
		ClientID:            regResp.ClientID,
		RedirectURI:         redirectURI,
		Scope:               strings.Join(prm.ScopesSupported, " "),
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
	})
	if escRec := performCookieJSONRequest(router, http.MethodPost, "/api/v1/oauth/authorize", sessionPlain, string(escalatedApproveBody)); escRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on un-narrowed admin scope approval, got %d %s", escRec.Code, escRec.Body.String())
	}

	// RFC 8252 §7.3: public client re-authenticating on a new ephemeral loopback port succeeds, while binding the issued code to that exact port.
	ephemeralRedirectURI := "http://127.0.0.1:61888/oauth/callback"
	approveBody, _ := json.Marshal(oauthAuthorizationDecisionInput{
		Approved:            true,
		ClientID:            regResp.ClientID,
		RedirectURI:         ephemeralRedirectURI,
		Scope:               authPreview.Scope,
		State:               "mcp-state-123",
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
	})
	consentRec := performCookieJSONRequest(router, http.MethodPost, "/api/v1/oauth/authorize", sessionPlain, string(approveBody))
	if consentRec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/oauth/authorize = %d %s", consentRec.Code, consentRec.Body.String())
	}
	parsedRedirect, err := url.Parse(jsonString(t, consentRec.Body.Bytes(), "redirectUrl"))
	if err != nil {
		t.Fatal(err)
	}
	authCode := parsedRedirect.Query().Get("code")
	if authCode == "" || parsedRedirect.Query().Get("state") != "mcp-state-123" || parsedRedirect.Host != "127.0.0.1:61888" {
		t.Fatalf("unexpected redirect URL: %s", parsedRedirect.String())
	}

	// 4. Negative: public client sending extraneous client_secret, Basic auth, or malformed Authorization header MUST get 401 invalid_client + WWW-Authenticate without consuming code.
	extraSecretRec := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {regResp.ClientID},
		"client_secret": {"unexpected-secret"},
		"code":          {authCode},
		"redirect_uri":  {ephemeralRedirectURI},
		"code_verifier": {verifier},
	})
	if extraSecretRec.Code != http.StatusUnauthorized || jsonString(t, extraSecretRec.Body.Bytes(), "error") != "invalid_client" || extraSecretRec.Header().Get("WWW-Authenticate") != "Basic" {
		t.Fatalf("public client with extra secret = %d %s, want 401 invalid_client", extraSecretRec.Code, extraSecretRec.Body.String())
	}

	malformedAuthReq := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {regResp.ClientID},
		"code":          {authCode},
		"redirect_uri":  {ephemeralRedirectURI},
		"code_verifier": {verifier},
	}.Encode()))
	malformedAuthReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	malformedAuthReq.Header.Set("Authorization", "Bearer bogus")
	malformedAuthRec := httptest.NewRecorder()
	router.ServeHTTP(malformedAuthRec, malformedAuthReq)
	if malformedAuthRec.Code != http.StatusUnauthorized {
		t.Fatalf("public client with non-Basic Authorization header = %d, want 401", malformedAuthRec.Code)
	}

	basicOnPublicReq := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authCode},
		"redirect_uri":  {ephemeralRedirectURI},
		"code_verifier": {verifier},
	}.Encode()))
	basicOnPublicReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	basicOnPublicReq.SetBasicAuth(regResp.ClientID, "unexpected-secret")
	basicOnPublicRec := httptest.NewRecorder()
	router.ServeHTTP(basicOnPublicRec, basicOnPublicReq)
	if basicOnPublicRec.Code != http.StatusUnauthorized || jsonString(t, basicOnPublicRec.Body.Bytes(), "error") != "invalid_client" {
		t.Fatalf("public client with Basic auth = %d %s, want 401 invalid_client", basicOnPublicRec.Code, basicOnPublicRec.Body.String())
	}

	// Port mismatch at token exchange (using original port instead of ephemeral port bound to code) MUST fail 400 invalid_grant.
	wrongPortRec := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {regResp.ClientID},
		"code":          {authCode},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	if wrongPortRec.Code != http.StatusBadRequest || jsonString(t, wrongPortRec.Body.Bytes(), "error") != "invalid_grant" {
		t.Fatalf("wrong port token exchange = %d %s, want 400 invalid_grant", wrongPortRec.Code, wrongPortRec.Body.String())
	}

	// 5. Exchange code for tokens as public client (client_id only + PKCE code_verifier + exact ephemeral redirect_uri).
	tokenRec := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {regResp.ClientID},
		"code":          {authCode},
		"redirect_uri":  {ephemeralRedirectURI},
		"code_verifier": {verifier},
	})
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("public client token exchange = %d %s", tokenRec.Code, tokenRec.Body.String())
	}
	var tokens oauthTokenResponse
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatalf("expected access_token and refresh_token, got %#v", tokens)
	}

	// 6. Access /api/v1/mcp (initialize, tools/list, and L0 workspace_list_projects) using the issued Bearer token.
	callMCP := func(bearer, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := callMCP(tokens.AccessToken, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`); rec.Code != http.StatusOK {
		t.Fatalf("MCP initialize = %d %s", rec.Code, rec.Body.String())
	}
	if rec := callMCP(tokens.AccessToken, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"search_tools"`) {
		t.Fatalf("MCP tools/list = %d %s", rec.Code, rec.Body.String())
	}
	if rec := callMCP(tokens.AccessToken, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"execute_tool","arguments":{"toolName":"workspace_list_projects","arguments":{}}}}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "dcr-proj") {
		t.Fatalf("MCP workspace_list_projects = %d %s", rec.Code, rec.Body.String())
	}

	// Dynamic MCP OAuth token MUST NOT bypass /api/v1/mcp onto REST /api/v1/* or mint personal access tokens!
	if restBypass := performBearerRequest(router, http.MethodGet, "/api/v1/projects", tokens.AccessToken, ""); restBypass.Code != http.StatusForbidden || !strings.Contains(restBypass.Body.String(), "auth.token.mcp_only") {
		t.Fatalf("dynamic MCP token REST bypass = %d %s, want 403 auth.token.mcp_only", restBypass.Code, restBypass.Body.String())
	}
	if patMint := performBearerRequest(router, http.MethodPost, "/api/v1/access-tokens", tokens.AccessToken, `{"name":"escaped-pat","scope":"project:read"}`); patMint.Code != http.StatusForbidden {
		t.Fatalf("dynamic MCP token PAT mint = %d %s, want 403", patMint.Code, patMint.Body.String())
	}

	// User can still inspect and manage the authorized grant in /api/v1/oauth/grants.
	grantsRec := performCookieJSONRequest(router, http.MethodGet, "/api/v1/oauth/grants", sessionPlain, "")
	if grantsRec.Code != http.StatusOK || !strings.Contains(grantsRec.Body.String(), "Antigravity MCP") {
		t.Fatalf("ListMyOAuthGrants missing dynamic client grant: %d %s", grantsRec.Code, grantsRec.Body.String())
	}

	// 7. Refresh token rotation & RFC 6749 §6 scope validation for public client (client_id only).
	badScopeRefresh := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {regResp.ClientID},
		"refresh_token": {tokens.RefreshToken},
		"scope":         {"auth:manage"},
	})
	if badScopeRefresh.Code != http.StatusBadRequest || jsonString(t, badScopeRefresh.Body.Bytes(), "error") != "invalid_scope" {
		t.Fatalf("escalated refresh scope = %d %s, want 400 invalid_scope", badScopeRefresh.Code, badScopeRefresh.Body.String())
	}

	refreshRec := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {regResp.ClientID},
		"refresh_token": {tokens.RefreshToken},
		"scope":         {"project:read"},
	})
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("public client refresh_token = %d %s", refreshRec.Code, refreshRec.Body.String())
	}
	var rotated oauthTokenResponse
	if err := json.Unmarshal(refreshRec.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.AccessToken == "" || rotated.RefreshToken == "" || rotated.RefreshToken == tokens.RefreshToken || rotated.Scope != "project:read" {
		t.Fatalf("unexpected rotated tokens: %#v", rotated)
	}

	// 8. Revoke token family for public client (client_id only).
	badRevoke := performFormRequest(router, http.MethodPost, "/api/v1/oauth/revoke", url.Values{
		"client_id":     {regResp.ClientID},
		"client_secret": {"wrong"},
		"token":         {rotated.AccessToken},
	})
	if badRevoke.Code != http.StatusUnauthorized {
		t.Fatalf("public client revoke with secret = %d, want 401", badRevoke.Code)
	}

	revokeRec := performFormRequest(router, http.MethodPost, "/api/v1/oauth/revoke", url.Values{
		"client_id": {regResp.ClientID},
		"token":     {rotated.AccessToken},
	})
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("public client /api/v1/oauth/revoke = %d %s", revokeRec.Code, revokeRec.Body.String())
	}
	if rec := callMCP(rotated.AccessToken, `{"jsonrpc":"2.0","id":4,"method":"ping"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on revoked MCP Bearer token, got %d", rec.Code)
	}
	revokedRefresh := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {regResp.ClientID},
		"refresh_token": {rotated.RefreshToken},
	})
	if revokedRefresh.Code != http.StatusBadRequest || jsonString(t, revokedRefresh.Body.Bytes(), "error") != "invalid_grant" {
		t.Fatalf("refresh after revoke = %d %s, want 400 invalid_grant", revokedRefresh.Code, revokedRefresh.Body.String())
	}
}

func TestMCPDynamicClientRegistrationConfidentialClientCompatibility(t *testing.T) {
	db := authIntegrationDB(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "https://devops.example.com")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mcp-dcr-confidential-secret-32by")

	user := model.User{
		ID:       "usr_mcp_dcr_conf",
		Email:    "mcp-dcr-conf@example.com",
		Name:     "Confidential Tester",
		Role:     authz.PlatformRoleUser,
		Language: "en-US",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionPlain := "sess_mcp_dcr_conf_1"
	if err := db.Create(&model.UserSession{
		ID:        "ses_mcp_dcr_conf",
		UserID:    user.ID,
		TokenHash: transportapi.HashToken(sessionPlain),
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(db, mustTestConfig(t))

	// Omitted token_endpoint_auth_method defaults to client_secret_basic and issues one-time secret; explicit grant_types=["authorization_code"] omits refresh_token.
	redirectURI := "https://confidential.example.com/oauth/callback"
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/register", strings.NewReader(`{"client_name":"Confidential App","redirect_uris":["`+redirectURI+`"],"grant_types":["authorization_code"],"scope":"project:read"}`))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("confidential registration = %d %s", regRec.Code, regRec.Body.String())
	}
	var regResp oauthDynamicClientRegistrationResponse
	if err := json.Unmarshal(regRec.Body.Bytes(), &regResp); err != nil {
		t.Fatal(err)
	}
	if regResp.TokenEndpointAuthMethod != "client_secret_basic" || regResp.ClientSecret == "" || regResp.ClientSecretExpiresAt == nil || *regResp.ClientSecretExpiresAt != 0 {
		t.Fatalf("unexpected confidential registration response: %#v", regResp)
	}

	// Verify DB never stores plaintext client_secret.
	var stored model.OAuthApplication
	if err := db.First(&stored, "client_id = ?", regResp.ClientID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ClientSecretHash == "" || stored.ClientSecretHash == regResp.ClientSecret {
		t.Fatalf("stored secret hash invalid: %q", stored.ClientSecretHash)
	}

	verifier := strings.Repeat("c", 64)
	sum := sha256.Sum256([]byte(verifier))
	approveBody, _ := json.Marshal(oauthAuthorizationDecisionInput{
		Approved:            true,
		ClientID:            regResp.ClientID,
		RedirectURI:         redirectURI,
		Scope:               "project:read",
		CodeChallenge:       base64.RawURLEncoding.EncodeToString(sum[:]),
		CodeChallengeMethod: "S256",
	})
	consentRec := performCookieJSONRequest(router, http.MethodPost, "/api/v1/oauth/authorize", sessionPlain, string(approveBody))
	if consentRec.Code != http.StatusOK {
		t.Fatalf("authorize = %d %s", consentRec.Code, consentRec.Body.String())
	}
	parsedRedirect, _ := url.Parse(jsonString(t, consentRec.Body.Bytes(), "redirectUrl"))
	code := parsedRedirect.Query().Get("code")

	// Confidential client without secret MUST fail 401 invalid_client.
	noSecretRec := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {regResp.ClientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	if noSecretRec.Code != http.StatusUnauthorized || jsonString(t, noSecretRec.Body.Bytes(), "error") != "invalid_client" {
		t.Fatalf("confidential client without secret = %d %s, want 401 invalid_client", noSecretRec.Code, noSecretRec.Body.String())
	}

	// Dynamic client_secret_basic client attempting client_secret_post MUST fail 401 invalid_client.
	postMethodRec := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {regResp.ClientID},
		"client_secret": {regResp.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	if postMethodRec.Code != http.StatusUnauthorized || jsonString(t, postMethodRec.Body.Bytes(), "error") != "invalid_client" {
		t.Fatalf("client_secret_basic client using post secret = %d %s, want 401 invalid_client", postMethodRec.Code, postMethodRec.Body.String())
	}

	// Valid Basic auth succeeds, and because grant_types excluded refresh_token, RefreshToken is omitted.
	basicReq := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}.Encode()))
	basicReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	basicReq.SetBasicAuth(regResp.ClientID, regResp.ClientSecret)
	basicRec := httptest.NewRecorder()
	router.ServeHTTP(basicRec, basicReq)
	if basicRec.Code != http.StatusOK {
		t.Fatalf("confidential Basic token exchange = %d %s", basicRec.Code, basicRec.Body.String())
	}
	var tokens oauthTokenResponse
	if err := json.Unmarshal(basicRec.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken != "" {
		t.Fatalf("expected access_token only for authorization_code-only client, got %#v", tokens)
	}
}
