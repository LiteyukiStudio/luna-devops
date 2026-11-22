package aiapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestMCPRenderCardTemplatesAllFourAndSelfContainedStyling(t *testing.T) {
	handler := New(&stubMCPHost{})
	zhUser := model.User{ID: "usr_zh", Language: "zh-CN"}
	enUser := model.User{ID: "usr_en", Language: "en-US"}

	cases := []struct {
		name           string
		user           model.User
		payload        string
		expectSnippets []string
	}{
		{
			name: "diagnostics_structured_and_string_warnings",
			user: zhUser,
			payload: `{
				"template": "diagnostics",
				"title": "api-server · prod",
				"subtitle": "项目 alpha-svc",
				"data": {
					"status": "degraded",
					"statusLabel": "CrashLoopBackOff",
					"target": "alpha-svc/api-server/prod",
					"cluster": "prod-k8s-01",
					"summary": "新版实例因缺少 REDIS_URL 启动失败，旧版 1 副本仍在线承载流量。",
					"replicas": {"ready": 1, "desired": 2, "updated": 1, "available": 1, "restarts": 5},
					"cpu": {"usage": "420m", "limit": "1000m", "percent": 42.4},
					"memory": {"usage": "490Mi", "limit": "512Mi", "percent": 95.7},
					"pvcs": [{"name": "data-pvc", "status": "Bound", "capacity": "20Gi", "mountPath": "/var/lib/data"}],
					"warnings": [
						{"reason": "BackOff", "pod": "api-server-7d9f-x2k", "message": "Back-off restarting failed container", "count": 5, "lastSeen": "30s ago"},
						"Readiness probe failed: HTTP 503"
					]
				}
			}`,
			expectSnippets: []string{
				"<!DOCTYPE html>",
				`<html lang="zh-CN">`,
				`http-equiv="Content-Security-Policy"`,
				"@media (prefers-color-scheme:dark)",
				`data-luna-card-template="diagnostics"`,
				"CrashLoopBackOff",
				"ld-tone-warning",
				"1 / 2",
				"重启 5",
				"width:42%",
				"width:96%",
				"data-pvc",
				"Back-off restarting failed container",
				"Readiness probe failed: HTTP 503",
			},
		},
		{
			name: "change_review_preview_tool_compatible_and_english_locale",
			user: enUser,
			payload: `{
				"template": "change_review",
				"title": "Update prod config & rotate credentials",
				"data": {
					"toolName": "delivery_configure_target_config",
					"approvalTier": "L2",
					"targetResource": "api-server/prod",
					"targetId": "dplt_89e66ebb87c11976a0d33667",
					"projectId": "prj_33d6a910384769b3b35984e5",
					"summary": "Adjust replicas and rotate database credentials",
					"changes": [
						{"field": "replicas", "before": 2, "after": 4, "action": "update"},
						{"field": "CUSTOM_CONN", "before": "plain-old-val", "after": "plain-new-val", "action": "update"},
						{"field": "kubeconfig", "after": "apiVersion: v1\nclusters: secret-cluster-token"},
						{"field": "smtp", "after": {"host": "smtp.example.com", "password": "nested-smtp-secret"}}
					],
					"sensitiveKeys": ["CUSTOM_CONN"],
					"impact": ["Affects 1 production target (prod)"],
					"warnings": ["Requires a release to roll out"]
				}
			}`,
			expectSnippets: []string{
				`<html lang="en">`,
				`data-luna-card-template="change_review"`,
				"prj_33d6a910384769b3b35984e5",
				"api-server/prod",
				"delivery_configure_target_config",
				"update",
				"CUSTOM_CONN",
				"Read-only preview",
				"[REDACTED]",
				"smtp.example.com",
			},
		},
		{
			name: "inventory",
			user: zhUser,
			payload: `{
				"template": "inventory",
				"title": "Alpha 项目空间资源盘点",
				"data": {
					"summary": "共 3 个服务，公网入口证书均已签发。",
					"stats": [
						{"label": "应用数", "value": 3, "status": "Healthy"},
						{"label": "异常路由", "value": 1, "status": "NotReady"}
					],
					"items": [
						{"name": "api-server", "kind": "Deployment", "stage": "prod", "status": "Healthy", "url": "https://api.alpha.example.com", "detail": "2/2 Ready"}
					]
				}
			}`,
			expectSnippets: []string{
				`data-luna-card-template="inventory"`,
				"Alpha 项目空间资源盘点",
				"ld-tone-danger",
				`href="https://api.alpha.example.com"`,
				`rel="noopener noreferrer"`,
				"2/2 Ready",
			},
		},
		{
			name: "topology",
			user: zhUser,
			payload: `{
				"template": "topology",
				"title": "Alpha 服务依赖拓扑",
				"data": {
					"entrypoints": [
						{"host": "api.alpha.example.com", "path": "v1", "protocol": "https", "tlsStatus": "Issued", "targetService": "api-server (prod)", "url": "https://api.alpha.example.com/v1", "status": "Ready"}
					],
					"nodes": [
						{"name": "api-server", "kind": "Deployment", "stage": "prod", "status": "Healthy", "replicas": {"ready": 2, "desired": 2}},
						{"name": "postgres-main", "kind": "StatefulSet", "stage": "prod", "status": "Healthy", "replicas": 1}
					],
					"bindings": [
						{"from": "api-server", "to": "postgres-main", "protocol": "tcp", "port": 5432, "injection": "DATABASE_URL", "status": "Active"}
					]
				}
			}`,
			expectSnippets: []string{
				`data-luna-card-template="topology"`,
				"api.alpha.example.com/v1",
				"2/2",
				"→ postgres-main",
				"TCP:5432",
				"DATABASE_URL",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := handler.mcpCallRenderCard(tc.user, json.RawMessage(tc.payload))
			if res.IsError {
				t.Fatalf("mcpCallRenderCard(%s) returned error: %#v", tc.name, res.StructuredContent)
			}
			// Content[0].Text must be the raw self-contained HTML without duplicate JSON-escaped bloat
			if len(res.Content) != 1 || !strings.HasPrefix(res.Content[0].Text, "<!DOCTYPE html>") {
				t.Fatalf("expected Content[0].Text to start with <!DOCTYPE html>, got: %.80q", res.Content)
			}
			sc, ok := res.StructuredContent.(map[string]any)
			if !ok {
				t.Fatalf("StructuredContent type = %T", res.StructuredContent)
			}
			htmlStr, _ := sc["html"].(string)
			if htmlStr != res.Content[0].Text {
				t.Fatal("StructuredContent.html and Content[0].Text must match")
			}
			if strings.Contains(htmlStr, "ZgotmplZ") {
				t.Fatalf("html/template emitted ZgotmplZ sanitizer sentinel in %s:\n%s", tc.name, htmlStr)
			}
			if strings.Contains(htmlStr, "<button") || strings.Contains(htmlStr, "<form") || strings.Contains(htmlStr, "<script") {
				t.Fatalf("standalone read-only card must not contain interactive <button>/<form>/<script> tags:\n%s", htmlStr)
			}
			for _, leaked := range []string{"plain-old-val", "plain-new-val", "secret-cluster-token", "nested-smtp-secret"} {
				if strings.Contains(htmlStr, leaked) {
					t.Fatalf("secret value %q leaked into HTML:\n%s", leaked, htmlStr)
				}
			}
			for _, snippet := range tc.expectSnippets {
				if !strings.Contains(htmlStr, snippet) {
					t.Errorf("expected snippet %q in %s HTML:\n%s", snippet, tc.name, htmlStr)
				}
			}
		})
	}
}

func TestMCPRenderCardXSSEscapingURLCredentialsAndStatusClassification(t *testing.T) {
	handler := New(&stubMCPHost{})
	user := model.User{ID: "usr_xss", Language: "zh-CN"}

	xssArgs := `{
		"template": "inventory",
		"title": "<script>alert('title-xss')</script>",
		"subtitle": "postgres://dbadmin:super-secret-db-pass@10.0.0.8:5432/prod",
		"data": {
			"summary": "</style><script>alert('css-breakout')</script>",
			"stats": [
				{"label": "<b>bold</b>", "value": "<svg onload=alert(1)>", "status": "danger\" onclick=\"alert(1)"}
			],
			"items": [
				{
					"name": "evil-js-link",
					"kind": "<iframe src=//evil.example>",
					"status": "danger\" onmouseover=\"alert(2)",
					"url": "javascript:alert(document.domain)",
					"detail": "normal detail"
				},
				{
					"name": "evil-data-link",
					"status": "Warning",
					"url": "data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg=="
				},
				{
					"name": "credentialed-https-link",
					"status": "Healthy",
					"url": "https://alice:hunter2password@api.example.com/hook?access_token=tok_live_999&env=prod",
					"detail": "redis://:redis_empty_user_pass@10.0.0.5:6379/0 and https://ghp_single_token_pat@github.com/org/repo.git"
				},
				{
					"name": "non-http-query-token",
					"status": "Running",
					"url": "wss://stream.example.com/events?token=ws_query_secret_777&ch=ops"
				}
			]
		}
	}`

	res := handler.mcpCallRenderCard(user, json.RawMessage(xssArgs))
	if res.IsError {
		t.Fatalf("unexpected error: %#v", res.StructuredContent)
	}
	htmlStr := res.StructuredContent.(map[string]any)["html"].(string)

	forbiddenRaw := []string{
		"<script>",
		"</script>",
		"<img src=x",
		"<svg onload",
		"<iframe",
		`href="javascript:`,
		`href="data:`,
		`onclick="alert`,
		`onmouseover="alert`,
		"super-secret-db-pass",
		"hunter2password",
		"tok_live_999",
		"redis_empty_user_pass",
		"ghp_single_token_pat",
		"ws_query_secret_777",
		"ZgotmplZ",
	}
	for _, bad := range forbiddenRaw {
		if strings.Contains(strings.ToLower(htmlStr), strings.ToLower(bad)) {
			t.Errorf("unescaped XSS or credential vector %q found in rendered HTML:\n%s", bad, htmlStr)
		}
	}
	if !strings.Contains(htmlStr, "&lt;script&gt;alert(&#39;title-xss&#39;)&lt;/script&gt;") {
		t.Errorf("expected escaped <script> entity in HTML, got:\n%s", htmlStr)
	}
	if !strings.Contains(htmlStr, `href="https://api.example.com/hook?access_token=REDACTED&amp;env=prod"`) {
		t.Errorf("expected sanitized https URL with stripped userinfo and redacted access_token, got:\n%s", htmlStr)
	}

	// Verify status tone classification never marks negated/failure Kubernetes/TLS states as green "success"
	toneAssertions := map[string]string{
		"NotReady":         "danger",
		"not_ready":        "danger",
		"Invalid":          "danger",
		"Revoked":          "danger",
		"Expired":          "danger",
		"ImagePullBackOff": "danger",
		"ErrImagePull":     "danger",
		"Evicted":          "danger",
		"Unbound":          "warning",
		"Inactive":         "warning",
		"Pending":          "warning",
		"unavailable":      "warning",
		"Progressing":      "info",
		"Running":          "info",
		"Ready":            "success",
		"Bound":            "success",
		"Active":           "success",
		"Valid":            "success",
		"OK":               "success",
	}
	for status, wantTone := range toneAssertions {
		if got := classifyStatusTone(status); got != wantTone {
			t.Errorf("classifyStatusTone(%q) = %q, want %q", status, got, wantTone)
		}
	}
}

func TestMCPRenderCardValidationAndMCPAppsResourceContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := aitool.NewRegistry()
	host := &stubMCPHost{
		stubAIToolHost: stubAIToolHost{reg: reg},
		authOK:         true,
		user:           model.User{ID: "usr_card", Role: authz.PlatformRoleUser, Language: "zh-CN"},
		token:          model.AccessToken{ID: "tok_min", UserID: "usr_card", Scope: "project:read", Source: model.AccessTokenSourceOAuth},
		baseURL:        "https://devops.example.com",
		mcpKey:         []byte("test-mcp-signing-key-0123456789"),
	}
	handler := New(host)
	router := gin.New()
	router.POST("/api/v1/mcp", handler.PostMCPStreamableHTTP)

	call := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer tok")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1) render_card is NEVER registered in internal/aitool Registry (isolated from luna-agent)
	if _, exists := reg.Get("render_card"); exists {
		t.Fatal("render_card must not be registered in internal/aitool business registry")
	}

	// 2) initialize advertises extensions["io.modelcontextprotocol/ui"] & adaptive instructions
	initRec := call(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if initRec.Code != http.StatusOK || !strings.Contains(initRec.Body.String(), `"extensions":{"io.modelcontextprotocol/ui"`) || !strings.Contains(initRec.Body.String(), "render_card") {
		t.Fatalf("initialize missing extensions UI capability or adaptive instructions: %s", initRec.Body.String())
	}

	// 3) tools/list exposes render_card with _meta.ui.resourceUri and full nested data properties schema
	listRec := call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	for _, want := range []string{`"name":"render_card"`, mcpCardViewerResourceURI, `"targetResource"`, `"entrypoints"`, `"pvcs"`} {
		if !strings.Contains(listRec.Body.String(), want) {
			t.Fatalf("tools/list missing %s: %s", want, listRec.Body.String())
		}
	}

	// 4) resources/list & resources/read serve ui://luna-devops/card-viewer.html with CSP, teardown, and source checks
	resListRec := call(`{"jsonrpc":"2.0","id":3,"method":"resources/list"}`)
	if !strings.Contains(resListRec.Body.String(), mcpCardViewerResourceURI) || !strings.Contains(resListRec.Body.String(), mcpAppHTMLMimeType) || !strings.Contains(resListRec.Body.String(), `"connectDomains":[]`) {
		t.Fatalf("resources/list missing ui:// card viewer or CSP metadata: %s", resListRec.Body.String())
	}
	resReadRec := call(`{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"ui://luna-devops/card-viewer.html"}}`)
	bodyStr := resReadRec.Body.String()
	for _, requiredViewerFeature := range []string{
		"ui/initialize",
		"ui/resource-teardown",
		"ui/notifications/tool-result",
		"ui/notifications/tool-cancelled",
		"event.source !== window.parent",
		"SAFE_VAR_NAME",
	} {
		if !strings.Contains(bodyStr, requiredViewerFeature) {
			t.Fatalf("resources/read ui:// missing %q: %s", requiredViewerFeature, bodyStr)
		}
	}
	if strings.Contains(bodyStr, "innerHTML") {
		t.Fatal("ui://luna-devops/card-viewer.html must construct DOM via textContent/createElement without innerHTML")
	}

	// 5) Strict validation rejects unknown template, empty diagnostics data, or unknown fields with actionable hint
	badTplRec := call(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"render_card","arguments":{"template":"present_card","title":"bad","data":{}}}}`)
	if !strings.Contains(badTplRec.Body.String(), `"isError":true`) || !strings.Contains(badTplRec.Body.String(), "diagnostics, change_review, inventory, topology") {
		t.Fatalf("expected invalid template error with hint, got: %s", badTplRec.Body.String())
	}
	emptyDiagRec := call(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"render_card","arguments":{"template":"diagnostics","title":"empty","data":{"pvcs":[{"name":"   "}],"warnings":[""],"replicas":null}}}}`)
	if !strings.Contains(emptyDiagRec.Body.String(), `"isError":true`) {
		t.Fatalf("expected post-filter blank diagnostics data rejection, got: %s", emptyDiagRec.Body.String())
	}

	// Verify every schema node in render_card.inputSchema defines explicit "type" or "anyOf" (Gemini/Antigravity compatibility)
	var rawToolSchema map[string]any
	defBytes, _ := json.Marshal(mcpRenderCardToolDefinition()["inputSchema"])
	if err := json.Unmarshal(defBytes, &rawToolSchema); err != nil {
		t.Fatal(err)
	}
	assertEveryJSONSchemaNodeTyped(t, "inputSchema", rawToolSchema)
	badFieldRec := call(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"render_card","arguments":{"template":"change_review","title":"bad","data":{"targetResource":"p/a/prod","unknownExtra":123}}}}`)
	if !strings.Contains(badFieldRec.Body.String(), `"isError":true`) || !strings.Contains(badFieldRec.Body.String(), "ai.tool_arguments_invalid") {
		t.Fatalf("expected strict unknown field rejection, got: %s", badFieldRec.Body.String())
	}

	okCallRec := call(`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"render_card","arguments":{"template":"inventory","title":"Quick Check","data":{"stats":[{"label":"Projects","value":2}]}}}}`)
	if strings.Contains(okCallRec.Body.String(), `"isError":true`) || (!strings.Contains(okCallRec.Body.String(), "<!DOCTYPE html>") && !strings.Contains(okCallRec.Body.String(), `\u003c!DOCTYPE html\u003e`)) {
		t.Fatalf("valid render_card tools/call failed: %s", okCallRec.Body.String())
	}
	if len(host.audits) != 0 {
		t.Fatalf("read-only render_card must not write mutation audit logs, got %v", host.audits)
	}
}

func assertEveryJSONSchemaNodeTyped(t *testing.T, path string, node map[string]any) {
	t.Helper()
	_, hasType := node["type"]
	anyOfRaw, hasAnyOf := node["anyOf"]
	if !hasType && !hasAnyOf {
		t.Fatalf("schema node %s is missing both 'type' and 'anyOf': %#v", path, node)
	}
	if hasAnyOf {
		branches, ok := anyOfRaw.([]any)
		if !ok || len(branches) == 0 {
			t.Fatalf("schema node %s has invalid anyOf: %#v", path, anyOfRaw)
		}
		for idx, br := range branches {
			brMap, ok := br.(map[string]any)
			if !ok {
				t.Fatalf("schema node %s.anyOf[%d] is not an object", path, idx)
			}
			assertEveryJSONSchemaNodeTyped(t, path+".anyOf", brMap)
		}
	}
	if node["type"] == "array" {
		itemsMap, ok := node["items"].(map[string]any)
		if !ok {
			t.Fatalf("array schema node %s is missing object 'items': %#v", path, node)
		}
		assertEveryJSONSchemaNodeTyped(t, path+".items", itemsMap)
	}
	if propsMap, ok := node["properties"].(map[string]any); ok {
		for k, v := range propsMap {
			propNode, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("property %s.properties.%s is not an object", path, k)
			}
			assertEveryJSONSchemaNodeTyped(t, path+".properties."+k, propNode)
		}
	}
}
