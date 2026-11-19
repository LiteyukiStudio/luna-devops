package gatewayapi

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
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type aiGatewayTestHost struct {
	Host
	db            *gorm.DB
	user          model.User
	cluster       model.RuntimeCluster
	appliedRoutes []string
	cleanedRoutes []string
	audits        []string
}

func (host *aiGatewayTestHost) DBFor(ctx *gin.Context) *gorm.DB {
	if host.db == nil {
		return nil
	}
	if ctx != nil && ctx.Request != nil {
		return host.db.WithContext(ctx.Request.Context())
	}
	return host.db
}

func (host *aiGatewayTestHost) DBWithContext(ctx context.Context) *gorm.DB {
	if host.db == nil {
		return nil
	}
	return host.db.WithContext(ctx)
}

func (host *aiGatewayTestHost) CurrentUser(*gin.Context) (model.User, bool) {
	return host.user, host.user.ID != ""
}

func (host *aiGatewayTestHost) AuthorizeProjectRef(ctx *gin.Context, projectRef string, _ authz.Action) (model.User, model.Project, bool) {
	proj, err := aitool.ResolveProject(host.DBFor(ctx), projectRef)
	if err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "project.not_found", err.Error())
		return model.User{}, model.Project{}, false
	}
	return host.user, proj, true
}

func (host *aiGatewayTestHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return host.AuthorizeProjectRef(ctx, projectID, action)
}

func (host *aiGatewayTestHost) EnsureProjectCanMutate(*gin.Context, model.Project) bool { return true }
func (host *aiGatewayTestHost) EnsureGatewayRouteCanMutate(*gin.Context, model.GatewayRoute) bool {
	return true
}
func (host *aiGatewayTestHost) DeleteStatusCanStart(status string) bool {
	return status == "" || status == "active" || status == "delete_failed"
}
func (host *aiGatewayTestHost) ResourceDeleteAlreadyStarted(error) bool { return false }
func (host *aiGatewayTestHost) MarkResourceDeleting(tx *gorm.DB, resource any, resourceID string) error {
	return tx.Model(resource).Where("id = ?", resourceID).Update("delete_status", "deleting").Error
}
func (host *aiGatewayTestHost) MarkResourceDeleteFailed(db *gorm.DB, resource any, resourceID, msg string) error {
	return db.Model(resource).Where("id = ?", resourceID).Updates(map[string]any{"delete_status": "delete_failed", "delete_message": msg}).Error
}
func (host *aiGatewayTestHost) EnqueueResourceCleanup(_ context.Context, _, resourceID, _, _ string) bool {
	host.cleanedRoutes = append(host.cleanedRoutes, resourceID)
	return true
}
func (host *aiGatewayTestHost) EnqueueGatewayApply(_ context.Context, route model.GatewayRoute, _ string) bool {
	host.appliedRoutes = append(host.appliedRoutes, route.ID)
	return true
}
func (host *aiGatewayTestHost) AuditWithContext(_, action, resource string, _ bool, _ string, _ context.Context) {
	host.audits = append(host.audits, action+":"+resource)
}
func (host *aiGatewayTestHost) ProjectMemberActionAllowed(*gin.Context, string, string, authz.Action) (bool, bool) {
	return true, true
}
func (host *aiGatewayTestHost) SecretStore() secret.Store      { return secret.Store{} }
func (host *aiGatewayTestHost) NormalizeStage(v string) string { return v }
func (host *aiGatewayTestHost) NormalizeGatewayPublicScheme(v string) string {
	if v == "https" {
		return "https"
	}
	return "http"
}
func (host *aiGatewayTestHost) NormalizePort(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}
func (host *aiGatewayTestHost) ApplicationCanMutate(app model.Application) bool {
	return app.DeleteStatus == "" || app.DeleteStatus == "active"
}
func (host *aiGatewayTestHost) RuntimeProjectNamespace(proj model.Project) string {
	return "luna-" + proj.Identifier
}
func (host *aiGatewayTestHost) DeploymentTargetResourceName(target model.DeploymentTarget) string {
	return target.KubernetesName
}
func (host *aiGatewayTestHost) RuntimeClusterForDeploymentTarget(context.Context, model.DeploymentTarget) (model.RuntimeCluster, error) {
	return host.cluster, nil
}

func newGatewayTestToolContext(t *testing.T, db *gorm.DB, user model.User, locked any) *aitool.ToolContext {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	parent, _ := gin.CreateTestContext(rec)
	parent.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)
	var lockedRaw json.RawMessage
	if locked != nil {
		b, err := json.Marshal(locked)
		if err != nil {
			t.Fatalf("marshal locked payload: %v", err)
		}
		lockedRaw = b
	}
	return aitool.NewToolContext(parent, db, user, "sess_1", "conv_1", "", "run_1", "call_1", "aiidem_gw", "model", lockedRaw)
}

func TestGatewayAPIRegistersAll5AIToolsWithValidSchemasAndScopes(t *testing.T) {
	reg := aitool.NewRegistry()
	handler := New(&aiGatewayTestHost{})
	handler.RegisterAITools(reg)

	if reg.Len() != 5 {
		t.Fatalf("registered tool count = %d, want 5", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}

	expected := map[string]struct {
		tier   string
		effect string
		scope  string
	}{
		"network_list_gateway_routes":  {aitool.ApprovalTierL0, aitool.EffectRead, string(authz.ActionGatewayRead)},
		"network_check_gateway_domain": {aitool.ApprovalTierL0, aitool.EffectRead, string(authz.ActionGatewayRead)},
		"network_create_gateway_route": {aitool.ApprovalTierL2, aitool.EffectAsync, string(authz.ActionGatewayManage)},
		"network_update_gateway_route": {aitool.ApprovalTierL2, aitool.EffectAsync, string(authz.ActionGatewayManage)},
		"network_delete_gateway_route": {aitool.ApprovalTierL2, aitool.EffectAsync, string(authz.ActionGatewayDelete)},
	}

	for name, want := range expected {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing expected gateway tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != aitool.DomainNetwork || cat.ApprovalTier != want.tier || cat.Effect != want.effect {
			t.Fatalf("%s metadata = domain:%s tier:%s effect:%s, want %+v", name, cat.Domain, cat.ApprovalTier, cat.Effect, want)
		}
		if len(cat.RequiredScopes) != 1 || cat.RequiredScopes[0] != want.scope {
			t.Fatalf("%s scopes = %v, want [%s]", name, cat.RequiredScopes, want.scope)
		}
		for _, scope := range cat.RequiredScopes {
			if norm := authz.NormalizeOAuthScope(scope); norm != scope {
				t.Fatalf("%s scope %q rejected by NormalizeOAuthScope", name, scope)
			}
			if !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleUser, scope) {
				t.Fatalf("%s scope %q not authorizable for tenant users", name, scope)
			}
		}
	}

	// Ensure hostnameAliases is physically omitted from gateway route output schema
	createTool, _ := reg.Get("network_create_gateway_route")
	outProps, _ := createTool.Catalog.OutputSchema["properties"].(map[string]any)
	if _, leaked := outProps["hostnameAliases"]; leaked {
		t.Fatalf("OutputSchema must omit unimplemented hostnameAliases column")
	}
}

func TestGatewayAIToolsDTOAndFilterPatchLogicWithoutDB(t *testing.T) {
	// 1. DTO decodes JSON filter strings into typed objects and strips hostnameAliases
	route := model.GatewayRoute{
		ID:                     "gwr_1",
		ProjectID:              "prj_1",
		ApplicationID:          "app_1",
		DeploymentTargetID:     "dplt_1",
		Host:                   "api.example.com",
		Path:                   "/v1",
		PathMatchType:          "Exact",
		SectionName:            "websecure",
		ParentGatewayName:      "custom-gw",
		ParentGatewayNamespace: "gw-ns",
		RequestHeaders:         `{"X-App":"demo","X-Remove":"bye"}`,
		ResponseHeaders:        `{"X-Frame-Options":"DENY"}`,
		URLRewrite:             `{"hostname":"backend.internal","replacePrefixMatch":"/"}`,
		HostnameAliases:        "www.example.com",
		BackendWeight:          3,
		Enabled:                true,
	}
	dto := toAIGatewayRouteDTO(route)
	if dto.RouteID != "gwr_1" || dto.SectionName != "websecure" || dto.ParentGatewayName != "custom-gw" || dto.ParentGatewayNamespace != "gw-ns" {
		t.Fatalf("DTO missing gateway fields: %#v", dto)
	}
	if dto.RequestHeaders["X-App"] != "demo" || dto.ResponseHeaders["X-Frame-Options"] != "DENY" {
		t.Fatalf("DTO headers not decoded: req=%#v resp=%#v", dto.RequestHeaders, dto.ResponseHeaders)
	}
	if dto.URLRewrite == nil || *dto.URLRewrite.Hostname != "backend.internal" || *dto.URLRewrite.ReplacePrefixMatch != "/" {
		t.Fatalf("DTO urlRewrite not decoded: %#v", dto.URLRewrite)
	}
	encodedDTO, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedDTO), "hostnameAliases") || strings.Contains(string(encodedDTO), "www.example.com") {
		t.Fatalf("DTO leaked hostnameAliases: %s", string(encodedDTO))
	}

	// 2. Incremental header patch preserves untouched keys and removes case-insensitively
	patchedHeaders, err := mergeGatewayHeaderPatch(route.RequestHeaders, map[string]string{"X-New": "1"}, []string{"x-remove"})
	if err != nil {
		t.Fatalf("mergeGatewayHeaderPatch: %v", err)
	}
	decodedHeaders, _ := parseGatewayKeyValueMap(patchedHeaders)
	if decodedHeaders["X-App"] != "demo" || decodedHeaders["X-New"] != "1" || decodedHeaders["X-Remove"] != "" {
		t.Fatalf("unexpected merged headers: %#v", decodedHeaders)
	}

	// 3. Same-side urlRewrite merge preserves hostname and swaps replacePrefixMatch -> replaceFullPath
	fullPath := "/readyz"
	rewrittenRaw, err := mergeAndEncodeURLRewrite(route.URLRewrite, &aiGatewayURLRewrite{ReplaceFullPath: &fullPath}, false)
	if err != nil {
		t.Fatalf("mergeAndEncodeURLRewrite: %v", err)
	}
	rewritten := decodeGatewayURLRewrite(rewrittenRaw)
	if rewritten == nil || *rewritten.Hostname != "backend.internal" || rewritten.ReplacePrefixMatch != nil || *rewritten.ReplaceFullPath != "/readyz" {
		t.Fatalf("unexpected merged urlRewrite: %#v", rewritten)
	}

	// 4. Strict tlsMode and requestRedirect validation reject invalid enums without silent fallback
	badTLS := "self-signed"
	if _, err := validateStrictTLSMode(&badTLS, "http-only"); err == nil {
		t.Fatalf("expected invalid tlsMode to fail")
	}
	badCode := 307
	if _, err := mergeAndEncodeRequestRedirect("", &aiGatewayRequestRedirect{StatusCode: &badCode}, false); err == nil {
		t.Fatalf("expected statusCode=307 to fail strict 301|302 check")
	}
}

func TestGatewayAIToolsEndToEndPatchAndAmbiguityFlows(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "gw_ai_tools",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{},
				&model.RuntimeCluster{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.GatewayRoute{},
				&model.NetworkEgressException{},
				&model.NetworkEgressExceptionGrant{},
			)
		},
	})

	adminUser := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin}
	project := model.Project{ID: "prj_gw", Identifier: "gwproj", Name: "GW Project"}
	cluster := model.RuntimeCluster{
		ID:                       "clu_gw",
		Name:                     "gw-cluster",
		GatewayDomainSuffixesRaw: "apps.example.com",
		GatewayPublicScheme:      "https",
		GatewayPublicPort:        443,
		GatewayHTTPListenerName:  "web",
		GatewayHTTPSListenerName: "websecure",
	}
	app := model.Application{ID: "app_web", ProjectID: project.ID, Identifier: "frontend", Name: "Frontend", DeleteStatus: "active"}
	publicTarget := model.DeploymentTarget{
		ID:            "dplt_pub",
		ProjectID:     project.ID,
		ApplicationID: app.ID,
		Name:          "prod",
		Stage:         "prod",
		ClusterID:     cluster.ID,
		ServicePorts:  model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 3000}}, 3000),
		Enabled:       true,
		DeleteStatus:  "active",
	}
	internalTarget := model.DeploymentTarget{
		ID:            "dplt_int",
		ProjectID:     project.ID,
		ApplicationID: app.ID,
		Name:          "internal-worker",
		Stage:         "dev",
		ClusterID:     cluster.ID,
		InternalOnly:  true,
		ServicePorts:  model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 8080}}, 8080),
		Enabled:       true,
		DeleteStatus:  "active",
	}
	for _, rec := range []any{&project, &cluster, &app, &publicTarget, &internalTarget} {
		if err := db.Create(rec).Error; err != nil {
			t.Fatal(err)
		}
	}

	host := &aiGatewayTestHost{db: db, user: adminUser, cluster: cluster}
	handler := New(host)
	reg := aitool.NewRegistry()
	handler.RegisterAITools(reg)

	// 1. Create Gateway Route: rejects internalOnly (409) & invalid tlsMode (400); succeeds with enabled=false
	createTool, _ := reg.Get("network_create_gateway_route")
	if _, err := createTool.Preview(newGatewayTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"project":"gwproj","application":"frontend","target":"dev"}`)); err == nil {
		t.Fatalf("expected 409 gateway_route.target_internal")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "gateway_route.target_internal" {
		t.Fatalf("expected gateway_route.target_internal, got %#v", err)
	}
	if _, err := createTool.Preview(newGatewayTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"project":"gwproj","application":"frontend","target":"prod","tlsMode":"bogus","enabled":false}`)); err == nil {
		t.Fatalf("expected strict 400 on invalid tlsMode")
	}

	createArgs := json.RawMessage(`{"project":"gwproj","application":"frontend","target":"prod","tlsMode":"http-challenge","sectionName":"websecure","enabled":false}`)
	createPrev, err := createTool.Preview(newGatewayTestToolContext(t, db, adminUser, nil), createArgs)
	if err != nil {
		t.Fatalf("network_create_gateway_route Preview: %v", err)
	}
	createdRaw, err := createTool.Execute(newGatewayTestToolContext(t, db, adminUser, createPrev.LockedPayload), createArgs)
	if err != nil {
		t.Fatalf("network_create_gateway_route Execute: %v", err)
	}
	createdDTO := createdRaw.(aiGatewayRouteDTO)
	if createdDTO.ProjectID != project.ID || createdDTO.Status != "progressing" || createdDTO.Host != "gwproj-frontend-prod.apps.example.com" {
		t.Fatalf("unexpected created DTO: %#v", createdDTO)
	}
	rawJSON, _ := json.Marshal(createdDTO)
	if strings.Contains(string(rawJSON), "hostnameAliases") {
		t.Fatalf("DTO JSON must not contain hostnameAliases: %s", string(rawJSON))
	}

	// Seed legacy HostnameAliases in DB to verify Update preserves it untouched!
	if err := db.Model(&model.GatewayRoute{}).Where("id = ?", createdDTO.ID).Update("hostname_aliases", "legacy.example.com").Error; err != nil {
		t.Fatal(err)
	}

	// 2. Check Gateway Domain recognizes current route vs conflict
	checkDomTool, _ := reg.Get("network_check_gateway_domain")
	tcDom := newGatewayTestToolContext(t, db, adminUser, nil)
	domRes, err := checkDomTool.Execute(tcDom, json.RawMessage(`{"project":"gwproj","host":"gwproj-frontend-prod.apps.example.com","route":"`+createdDTO.ID+`"}`))
	if err != nil {
		t.Fatalf("network_check_gateway_domain: %v", err)
	}
	domMap := domRes.(map[string]any)
	if domMap["status"] != "current" || domMap["available"] != true || domMap["cnameTarget"] != "*.apps.example.com" || tcDom.ResponseHeaders.Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected domain check result: %#v", domMap)
	}

	// 3. Update Gateway Route: incremental headers + urlRewrite + cross-side conflict protection + preserves untouched DB fields
	updateTool, _ := reg.Get("network_update_gateway_route")
	updArgs := json.RawMessage(`{
		"project":"gwproj",
		"route":"gwproj-frontend-prod.apps.example.com",
		"upsertRequestHeaders":{"X-Env":"prod","X-Trace":"1"},
		"urlRewrite":{"hostname":"internal.svc","replacePrefixMatch":"/v1"}
	}`)
	updPrev, err := updateTool.Preview(newGatewayTestToolContext(t, db, adminUser, nil), updArgs)
	if err != nil {
		t.Fatalf("network_update_gateway_route Preview: %v", err)
	}
	updRaw, err := updateTool.Execute(newGatewayTestToolContext(t, db, adminUser, updPrev.LockedPayload), updArgs)
	if err != nil {
		t.Fatalf("network_update_gateway_route Execute: %v", err)
	}
	updDTO := updRaw.(aiGatewayRouteDTO)
	if updDTO.SectionName != "websecure" || updDTO.TLSMode != "http-challenge" || updDTO.RequestHeaders["X-Env"] != "prod" || updDTO.URLRewrite == nil || *updDTO.URLRewrite.ReplacePrefixMatch != "/v1" {
		t.Fatalf("incremental update wiped or missed fields: %#v", updDTO)
	}

	// Cross-side requestRedirect without clearUrlRewrite=true MUST return 409 gateway_route.conflicting_filter
	if _, err := updateTool.Preview(newGatewayTestToolContext(t, db, adminUser, nil), json.RawMessage(`{
		"project":"gwproj",
		"route":"`+createdDTO.ID+`",
		"requestRedirect":{"scheme":"https","statusCode":301}
	}`)); err == nil {
		t.Fatalf("expected 409 gateway_route.conflicting_filter")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Status != http.StatusConflict || te.Code != "gateway_route.conflicting_filter" {
		t.Fatalf("expected 409 gateway_route.conflicting_filter, got %#v", err)
	}

	// Same-side urlRewrite patch merges hostname & switches replacePrefixMatch -> replaceFullPath while removing X-Trace header
	upd2Args := json.RawMessage(`{
		"project":"gwproj",
		"route":"` + createdDTO.ID + `",
		"removeRequestHeaderKeys":["x-trace"],
		"urlRewrite":{"replaceFullPath":"/healthz"}
	}`)
	upd2Prev, err := updateTool.Preview(newGatewayTestToolContext(t, db, adminUser, nil), upd2Args)
	if err != nil {
		t.Fatalf("second update Preview: %v", err)
	}
	upd2Raw, err := updateTool.Execute(newGatewayTestToolContext(t, db, adminUser, upd2Prev.LockedPayload), upd2Args)
	if err != nil {
		t.Fatalf("second update Execute: %v", err)
	}
	upd2DTO := upd2Raw.(aiGatewayRouteDTO)
	if _, stillHasTrace := upd2DTO.RequestHeaders["X-Trace"]; stillHasTrace || upd2DTO.RequestHeaders["X-Env"] != "prod" {
		t.Fatalf("header patch failed: %#v", upd2DTO.RequestHeaders)
	}
	if upd2DTO.URLRewrite == nil || *upd2DTO.URLRewrite.Hostname != "internal.svc" || upd2DTO.URLRewrite.ReplacePrefixMatch != nil || *upd2DTO.URLRewrite.ReplaceFullPath != "/healthz" {
		t.Fatalf("same-side urlRewrite merge failed: %#v", upd2DTO.URLRewrite)
	}

	var stored model.GatewayRoute
	if err := db.First(&stored, "id = ?", createdDTO.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.HostnameAliases != "legacy.example.com" {
		t.Fatalf("HostnameAliases was wiped in DB: %q", stored.HostnameAliases)
	}

	// 4. Multi-path same-host ambiguity returns 409 ai.resource_ambiguous
	secondRoute := model.GatewayRoute{
		ID:                 "gwr_second",
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: publicTarget.ID,
		Host:               createdDTO.Host,
		Path:               "/api",
		ServicePort:        3000,
		TLSMode:            "http-only",
		Enabled:            false,
		DeleteStatus:       "active",
	}
	if err := db.Create(&secondRoute).Error; err != nil {
		t.Fatal(err)
	}
	listTool, _ := reg.Get("network_list_gateway_routes")
	if _, err := listTool.Execute(newGatewayTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"project":"gwproj","route":"`+createdDTO.Host+`"}`)); err == nil {
		t.Fatalf("expected 409 ai.resource_ambiguous on duplicate host")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Status != http.StatusConflict || te.Code != "ai.resource_ambiguous" || len(te.Candidates) != 2 {
		t.Fatalf("expected 409 ai.resource_ambiguous with 2 candidates, got %#v", err)
	}

	// 5. Delete Gateway Route by exact ID
	delTool, _ := reg.Get("network_delete_gateway_route")
	delArgs := json.RawMessage(`{"project":"gwproj","route":"` + secondRoute.ID + `"}`)
	delPrev, err := delTool.Preview(newGatewayTestToolContext(t, db, adminUser, nil), delArgs)
	if err != nil {
		t.Fatalf("network_delete_gateway_route Preview: %v", err)
	}
	delOut, err := delTool.Execute(newGatewayTestToolContext(t, db, adminUser, delPrev.LockedPayload), delArgs)
	if err != nil {
		t.Fatalf("network_delete_gateway_route Execute: %v", err)
	}
	delMap := delOut.(map[string]any)
	if delMap["routeId"] != secondRoute.ID || delMap["projectId"] != project.ID || delMap["deleteStatus"] != "deleting" || len(host.cleanedRoutes) != 1 {
		t.Fatalf("unexpected delete gateway route output: %#v", delMap)
	}
}
