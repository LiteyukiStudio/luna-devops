package applicationapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type aiAppTestHost struct {
	Host
	db             *gorm.DB
	user           model.User
	secrets        secret.Store
	cluster        model.RuntimeCluster
	reconciledProj []string
	enqueuedRel    []string
	enqueuedDelApp []string
	audits         []string
}

func (host *aiAppTestHost) DBFor(ctx *gin.Context) *gorm.DB {
	if host.db == nil {
		return nil
	}
	if ctx != nil && ctx.Request != nil {
		return host.db.WithContext(ctx.Request.Context())
	}
	return host.db
}

func (host *aiAppTestHost) DBWithContext(ctx context.Context) *gorm.DB {
	if host.db == nil {
		return nil
	}
	return host.db.WithContext(ctx)
}

func (host *aiAppTestHost) CurrentUser(*gin.Context) (model.User, bool) {
	return host.user, host.user.ID != ""
}

func (host *aiAppTestHost) AuthorizeProjectRef(ctx *gin.Context, projectRef string, _ authz.Action) (model.User, model.Project, bool) {
	proj, err := aitool.ResolveProject(host.DBFor(ctx), projectRef)
	if err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "project.not_found", err.Error())
		return model.User{}, model.Project{}, false
	}
	return host.user, proj, true
}

func (host *aiAppTestHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return host.AuthorizeProjectRef(ctx, projectID, action)
}

func (host *aiAppTestHost) EnsureProjectCanMutate(*gin.Context, model.Project) bool { return true }
func (host *aiAppTestHost) EnsureBillingAllowsDeployChange(*gin.Context, string) bool {
	return true
}
func (host *aiAppTestHost) AuditWithContext(_, action, resource string, _ bool, _ string, _ context.Context) {
	host.audits = append(host.audits, action+":"+resource)
}
func (host *aiAppTestHost) SecretStore() secret.Store                           { return host.secrets }
func (host *aiAppTestHost) PublicBaseURL() string                               { return "https://devops.example.com" }
func (host *aiAppTestHost) NotificationEnqueuer() notification.DeliveryEnqueuer { return nil }
func (host *aiAppTestHost) EnqueueNetworkPolicyProjectReconcile(_ context.Context, projectID string) bool {
	host.reconciledProj = append(host.reconciledProj, projectID)
	return true
}
func (host *aiAppTestHost) SyncDeploymentTargetVolumeMounts(_ context.Context, _ *gorm.DB, _ model.DeploymentTarget, _ []DeploymentTargetDataVolumeInput) (DeploymentVolumeMountChanges, error) {
	return DeploymentVolumeMountChanges{}, nil
}
func (host *aiAppTestHost) NextReleaseRevisionFor(_ *gorm.DB, _, _, _ string) (int, error) {
	return 1, nil
}
func (host *aiAppTestHost) AuditDeploymentVolumeMountFailure(_ context.Context, _ string, _ DeploymentVolumeMountChanges, _ error) {
}
func (host *aiAppTestHost) AuditDeploymentVolumeMountChanges(_ context.Context, _ string, _ model.DeploymentTarget, _ DeploymentVolumeMountChanges) {
}
func (host *aiAppTestHost) EnqueueDeployRun(_ context.Context, rel model.Release) bool {
	host.enqueuedRel = append(host.enqueuedRel, rel.ID)
	return true
}
func (host *aiAppTestHost) DeploymentTargetVolumeMountsByTarget(_ context.Context, _ []model.DeploymentTarget) (map[string][]model.DeploymentVolumeMount, error) {
	return map[string][]model.DeploymentVolumeMount{}, nil
}
func (host *aiAppTestHost) NormalizePublicStage(v string) (string, bool) {
	switch v {
	case "dev", "test", "staging", "prod":
		return v, true
	default:
		return "", false
	}
}
func (host *aiAppTestHost) WriteDeploymentStageInvalid(ctx *gin.Context, _, detail string) {
	writeErrorCode(ctx, http.StatusBadRequest, "deployment_target.stage_invalid", detail)
}
func (host *aiAppTestHost) NormalizeBuildResourceQuantity(_ *gin.Context, val, fallback, _ string) (string, bool) {
	if val == "" {
		return fallback, true
	}
	return val, true
}
func (host *aiAppTestHost) RuntimeClusterForProjectUse(_ *gin.Context, _ model.User, _, _ string) (model.RuntimeCluster, bool) {
	return host.cluster, host.cluster.ID != ""
}
func (host *aiAppTestHost) RuntimeProjectNamespace(project model.Project) string {
	return "luna-" + project.Identifier
}
func (host *aiAppTestHost) NormalizeDataVolumes(_ *gin.Context, inputs []DeploymentTargetDataVolumeInput) ([]DeploymentTargetDataVolumeInput, bool) {
	return inputs, true
}
func (host *aiAppTestHost) NormalizeRuntimeConfigFilesInput(_ *gin.Context, val string) (string, bool) {
	return val, true
}
func (host *aiAppTestHost) NormalizeRuntimeConfigFilePathInput(_ *gin.Context, val string) (string, bool) {
	return val, true
}
func (host *aiAppTestHost) IsBuildEnvKey(val string) bool { return val != "" }
func (host *aiAppTestHost) ObserveDeploymentTargets(_ context.Context, _ model.Project, _ []model.DeploymentTarget) {
}
func (host *aiAppTestHost) GatewayRouteAccessURL(_ context.Context, route model.GatewayRoute, _ model.DeploymentTarget) (string, error) {
	return "https://" + route.Host, nil
}
func (host *aiAppTestHost) EnqueueApplicationDelete(_ context.Context, app model.Application, _ string, _ bool) bool {
	host.enqueuedDelApp = append(host.enqueuedDelApp, app.ID)
	return true
}

func newTestToolContext(t *testing.T, db *gorm.DB, user model.User, locked any) *aitool.ToolContext {
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
	return aitool.NewToolContext(parent, db, user, "sess_1", "conv_1", "", "run_1", "call_1", "aiidem_test", "model", lockedRaw)
}

func TestApplicationAPIRegistersAll17AIToolsWithValidSchemasAndScopes(t *testing.T) {
	reg := aitool.NewRegistry()
	handler := New(&aiAppTestHost{})
	handler.RegisterAITools(reg)

	if reg.Len() != 17 {
		t.Fatalf("registered tool count = %d, want 17", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}

	expected := map[string]struct {
		domain    string
		tier      string
		effect    string
		adminOnly bool
		sensitive []string
	}{
		"delivery_list_applications":         {aitool.DomainDelivery, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"delivery_get_application":           {aitool.DomainDelivery, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"delivery_list_templates":            {aitool.DomainDelivery, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"delivery_get_template":              {aitool.DomainDelivery, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"delivery_create_application":        {aitool.DomainDelivery, aitool.ApprovalTierL1, aitool.EffectWrite, false, nil},
		"delivery_update_application":        {aitool.DomainDelivery, aitool.ApprovalTierL1, aitool.EffectWrite, false, nil},
		"delivery_delete_application":        {aitool.DomainDelivery, aitool.ApprovalTierL2, aitool.EffectAsync, false, nil},
		"delivery_install_template":          {aitool.DomainDelivery, aitool.ApprovalTierL2, aitool.EffectAsync, false, []string{"secretValues.*"}},
		"network_get_topology":               {aitool.DomainNetwork, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"network_list_service_bindings":      {aitool.DomainNetwork, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"network_save_service_binding":       {aitool.DomainNetwork, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"network_delete_service_binding":     {aitool.DomainNetwork, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"network_save_topology_edge":         {aitool.DomainNetwork, aitool.ApprovalTierL1, aitool.EffectWrite, false, nil},
		"network_delete_topology_edge":       {aitool.DomainNetwork, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"admin_list_platform_app_templates":  {aitool.DomainAdmin, aitool.ApprovalTierL0, aitool.EffectRead, true, nil},
		"admin_save_platform_app_template":   {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectWrite, true, nil},
		"admin_delete_platform_app_template": {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectWrite, true, nil},
	}

	for name, want := range expected {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing expected tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != want.domain || cat.ApprovalTier != want.tier || cat.Effect != want.effect || cat.AdminOnly != want.adminOnly {
			t.Fatalf("%s metadata = domain:%s tier:%s effect:%s admin:%t, want %+v",
				name, cat.Domain, cat.ApprovalTier, cat.Effect, cat.AdminOnly, want)
		}
		if want.adminOnly && !cat.GlobalSessionOnly {
			t.Fatalf("%s must set GlobalSessionOnly=true", name)
		}
		if !slices.Equal(cat.SensitivePaths, want.sensitive) {
			t.Fatalf("%s sensitivePaths = %v, want %v", name, cat.SensitivePaths, want.sensitive)
		}
		for _, scope := range cat.RequiredScopes {
			if norm := authz.NormalizeOAuthScope(scope); norm != scope {
				t.Fatalf("%s scope %q rejected by NormalizeOAuthScope (got %q)", name, scope, norm)
			}
			role := authz.PlatformRoleUser
			if cat.AdminOnly {
				role = authz.PlatformRoleAdmin
			}
			if !authz.UserCanAuthorizeOAuthScope(role, scope) {
				t.Fatalf("%s scope %q not authorizable for role %q", name, scope, role)
			}
		}
	}

	// Verify delivery_install_template secretValues marks additionalProperties writeOnly + x-luna-sensitive
	installTool, _ := reg.Get("delivery_install_template")
	props := installTool.Catalog.InputSchema["properties"].(map[string]any)
	secProp := props["secretValues"].(map[string]any)
	addl := secProp["additionalProperties"].(map[string]any)
	if addl["writeOnly"] != true || addl["x-luna-sensitive"] != true {
		t.Fatalf("secretValues.additionalProperties = %#v, want writeOnly & x-luna-sensitive", addl)
	}
}

func TestApplicationAIToolsTemplateAndPatchLogicWithoutDB(t *testing.T) {
	adminUser := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin}
	memberUser := model.User{ID: "usr_member", Role: authz.PlatformRoleUser}
	handler := New(&aiAppTestHost{user: adminUser})
	reg := aitool.NewRegistry()
	handler.RegisterAITools(reg)

	// 1. delivery_list_templates excludes system_component and caps pageSize at 50
	listTpl, _ := reg.Get("delivery_list_templates")
	listOut, err := listTpl.Execute(newTestToolContext(t, nil, memberUser, nil), json.RawMessage(`{"pageSize":50}`))
	if err != nil {
		t.Fatalf("delivery_list_templates: %v", err)
	}
	env := listOut.(map[string]any)
	if env["pageSize"] != 50 {
		t.Fatalf("pageSize = %v, want 50", env["pageSize"])
	}
	for _, item := range env["items"].([]appTemplateSummaryResponse) {
		if item.Kind == "system_component" || item.SystemComponent != "" {
			t.Fatalf("leaked system_component template %s", item.ID)
		}
	}

	// 2. delivery_get_template returns 404 on system_component and separates values vs secretValues
	getTpl, _ := reg.Get("delivery_get_template")
	if _, err := getTpl.Execute(newTestToolContext(t, nil, memberUser, nil), json.RawMessage(`{"template":"luna-gateway-traffic-probe"}`)); err == nil {
		t.Fatalf("expected 404 on system_component")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Status != http.StatusNotFound {
		t.Fatalf("expected 404 ToolError, got %#v", err)
	}
	mongoExpRaw, err := getTpl.Execute(newTestToolContext(t, nil, memberUser, nil), json.RawMessage(`{"template":"mongo-express"}`))
	if err != nil {
		t.Fatalf("delivery_get_template(mongo-express): %v", err)
	}
	mongoExp := mongoExpRaw.(map[string]any)
	plainVals := mongoExp["values"].([]appTemplateValueResponse)
	secVals := mongoExp["secretValues"].([]appTemplateValueResponse)
	if len(plainVals) == 0 || len(secVals) == 0 {
		t.Fatalf("expected separated plain values (%d) and secretValues (%d)", len(plainVals), len(secVals))
	}
	for _, sv := range secVals {
		if sv.Default != "" {
			t.Fatalf("secretValues[%s] leaked default %q", sv.Key, sv.Default)
		}
	}

	// 3. admin_list_platform_app_templates enforces admin RBAC and supports built-in exportTemplate
	adminListTpl, _ := reg.Get("admin_list_platform_app_templates")
	if _, err := adminListTpl.Execute(newTestToolContext(t, nil, memberUser, nil), json.RawMessage(`{}`)); err == nil {
		t.Fatalf("expected 403 for non-admin")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Status != http.StatusForbidden {
		t.Fatalf("expected 403 ToolError, got %#v", err)
	}
	bundleRes, err := adminListTpl.Execute(newTestToolContext(t, nil, adminUser, nil), json.RawMessage(`{"exportTemplate":"mongo-express"}`))
	if err != nil {
		t.Fatalf("exportTemplate(mongo-express): %v", err)
	}
	if bundleRes.(map[string]any)["mode"] != "bundle" {
		t.Fatalf("unexpected exportTemplate mode: %#v", bundleRes)
	}

	// 4. Strict icon validation rejects unknown icon names instead of silently falling back to "box"
	badIcon := "unknown-icon"
	if _, _, err := validateAIApplicationIcon(&badIcon, true); err == nil {
		t.Fatalf("expected error for unknown icon name")
	}
	validIcon := "database"
	if got, changed, err := validateAIApplicationIcon(&validIcon, true); err != nil || !changed || got != "database" {
		t.Fatalf("validateAIApplicationIcon(database) = %q, %t, %v", got, changed, err)
	}

	// 5. CredentialMap decoding & incremental merge never double-escapes JSON
	dto := toAIServiceBindingDTO(model.ServiceBinding{
		ID:        "sbind_1",
		ProjectID: "prj_1",
		SecretMap: `[{"sourceEnvVar":"DB_PASS","targetSecretKey":"PASSWORD"}]`,
	}, nil)
	if len(dto.CredentialMap) != 1 || dto.CredentialMap[0].SourceEnvVar != "DB_PASS" {
		t.Fatalf("CredentialMap not decoded properly: %#v", dto.CredentialMap)
	}
	mergedCreds, err := mergeAICredentialMap(dto.CredentialMap, []model.SecretMapping{{SourceEnvVar: "api_token", TargetSecretKey: "TOKEN"}}, nil)
	if err != nil || len(mergedCreds) != 2 || mergedCreds[1].SourceEnvVar != "API_TOKEN" {
		t.Fatalf("mergeAICredentialMap = %#v, %v", mergedCreds, err)
	}

	// 6. Platform template value patch clears legacy plaintext default when switched to secret=true
	trueVal := true
	patchedVals, err := applyAIValuePatches(
		[]appstore.ValueDefinition{{Key: "token", Label: "Token", Default: "legacy-plain", Secret: false}},
		[]aiPlatformTemplateValuePatch{{Key: "token", Secret: &trueVal}},
		nil,
	)
	if err != nil || len(patchedVals) != 1 || !patchedVals[0].Secret || patchedVals[0].Default != "" {
		t.Fatalf("applyAIValuePatches failed to clear legacy default on secret switch: %#v (%v)", patchedVals, err)
	}
}

func TestApplicationAIToolsEndToEndFlows(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "app_ai_tools",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{},
				&model.RuntimeCluster{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.Release{},
				&model.GatewayRoute{},
				&model.ProjectVolume{},
				&model.ServiceBinding{},
				&model.ProjectTopologyEdge{},
				&model.PlatformAppTemplate{},
				&model.AppTemplateInstallation{},
				&model.AppTemplateInstallationComponent{},
				&model.SecretValue{},
				&model.PlatformEvent{},
				&model.NotificationRule{},
			)
		},
	})

	codec, err := secret.NewCodec("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	store := secret.NewStore(db, nil, codec)
	adminUser := model.User{ID: "usr_admin", Name: "Admin", Email: "admin@example.com", Role: authz.PlatformRoleAdmin}
	project := model.Project{ID: "prj_demo", Identifier: "demo", Name: "Demo Project"}
	cluster := model.RuntimeCluster{ID: "clu_default", Name: "default-cluster", IsDefault: true, DeleteStatus: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}

	host := &aiAppTestHost{
		db:      db,
		user:    adminUser,
		secrets: store,
		cluster: cluster,
	}
	handler := New(host)
	reg := aitool.NewRegistry()
	handler.RegisterAITools(reg)

	// 1. Tenant template discovery excludes system_component and separates values vs secretValues
	listTplTool, _ := reg.Get("delivery_list_templates")
	listTplOut, err := listTplTool.Execute(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"pageSize":50}`))
	if err != nil {
		t.Fatalf("delivery_list_templates: %v", err)
	}
	tplItems := listTplOut.(map[string]any)["items"].([]appTemplateSummaryResponse)
	for _, item := range tplItems {
		if item.Kind == "system_component" || item.SystemComponent != "" {
			t.Fatalf("delivery_list_templates leaked system_component template %s", item.ID)
		}
	}

	getTplTool, _ := reg.Get("delivery_get_template")
	if _, err := getTplTool.Execute(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"template":"luna-gateway-traffic-probe"}`)); err == nil {
		t.Fatalf("expected 404 on system_component template")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Status != http.StatusNotFound {
		t.Fatalf("expected 404 ToolError, got %#v", err)
	}

	redisTpl, err := getTplTool.Execute(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"template":"redis"}`))
	if err != nil {
		t.Fatalf("delivery_get_template(redis): %v", err)
	}
	redisMap := redisTpl.(map[string]any)
	if _, hasSecretValues := redisMap["secretValues"]; !hasSecretValues {
		t.Fatalf("delivery_get_template missing secretValues: %#v", redisMap)
	}

	// 2. Create -> Update (strict icon validation + patch semantics) -> Get -> List Applications
	createAppTool, _ := reg.Get("delivery_create_application")
	if _, err := createAppTool.Execute(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"project":"demo","identifier":"web-api","name":"Web API","icon":"not-a-valid-icon"}`)); err == nil {
		t.Fatalf("expected invalid icon error")
	}
	createdRaw, err := createAppTool.Execute(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"project":"demo","identifier":"web-api","name":"Web API","icon":"server"}`))
	if err != nil {
		t.Fatalf("delivery_create_application: %v", err)
	}
	createdMap := createdRaw.(map[string]any)
	appID := createdMap["applicationId"].(string)
	if createdMap["projectId"] != project.ID || createdMap["icon"] != "server" {
		t.Fatalf("unexpected create response: %#v", createdMap)
	}

	updateAppTool, _ := reg.Get("delivery_update_application")
	updatedRaw, err := updateAppTool.Execute(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"project":"demo","application":"web-api","name":"Web API v2"}`))
	if err != nil {
		t.Fatalf("delivery_update_application: %v", err)
	}
	updatedMap := updatedRaw.(map[string]any)
	if updatedMap["name"] != "Web API v2" || updatedMap["icon"] != "server" {
		t.Fatalf("patch wiped icon or missed name: %#v", updatedMap)
	}

	getAppTool, _ := reg.Get("delivery_get_application")
	tcGet := newTestToolContext(t, db, adminUser, nil)
	gotApp, err := getAppTool.Execute(tcGet, json.RawMessage(`{"project":"demo","application":"web-api"}`))
	if err != nil {
		t.Fatalf("delivery_get_application: %v", err)
	}
	if tcGet.ResponseHeaders.Get("Cache-Control") != "no-store" || gotApp.(map[string]any)["projectId"] != project.ID {
		t.Fatalf("expected no-store and authoritative projectId, got headers=%v body=%#v", tcGet.ResponseHeaders, gotApp)
	}

	// 3. Install Template (Preview blocks secret in values; Preview -> Execute locks identifier & creates release)
	installTool, _ := reg.Get("delivery_install_template")
	if _, err := installTool.Preview(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"project":"demo","template":"mongo-express","stage":"dev","values":{"password":"plain"}}`)); err == nil {
		t.Fatalf("expected R11 rejection when passing secret key in values")
	}
	installArgs := json.RawMessage(`{"project":"demo","template":"mongo-express","stage":"dev","values":{"username":"opsadmin"},"secretValues":{"mongodbUrl":"mongodb://root:pass@mongo:27017/"}}`)
	previewRes, err := installTool.Preview(newTestToolContext(t, db, adminUser, nil), installArgs)
	if err != nil {
		t.Fatalf("delivery_install_template Preview: %v", err)
	}
	installOut, err := installTool.Execute(newTestToolContext(t, db, adminUser, previewRes.LockedPayload), installArgs)
	if err != nil {
		t.Fatalf("delivery_install_template Execute: %v", err)
	}
	installOutMap := installOut.(map[string]any)
	backendAppID := installOutMap["primaryApplicationId"].(string)
	if installOutMap["projectId"] != project.ID || installOutMap["primaryReleaseId"] == "" || installOutMap["requiresRelease"] != false {
		t.Fatalf("unexpected delivery_install_template output: %#v", installOutMap)
	}

	// Seed target on web-api so we can test ServiceBinding + delete guard
	webTarget := model.DeploymentTarget{
		ID:            "dplt_web_dev",
		ProjectID:     project.ID,
		ApplicationID: appID,
		Name:          "dev",
		Stage:         "dev",
		ClusterID:     cluster.ID,
		ServicePorts:  model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 8080}, {Name: "grpc", Port: 9090}}, 8080),
		Enabled:       true,
		DeleteStatus:  "active",
	}
	if err := db.Create(&webTarget).Error; err != nil {
		t.Fatal(err)
	}
	// Update installed target to same clusterId so service binding cluster check passes
	backendTargetID := installOutMap["primaryDeploymentTargetId"].(string)

	// 4. ServiceBinding create + patch + decoded credentialMap + delete guard on delivery_delete_application
	saveBindTool, _ := reg.Get("network_save_service_binding")
	createBindArgs := json.RawMessage(`{
		"project":"demo",
		"sourceApplication":"` + backendAppID + `",
		"sourceTarget":"` + backendTargetID + `",
		"targetApplication":"web-api",
		"targetTarget":"dev",
		"targetPortName":"http",
		"protocol":"http",
		"injectionMode":"url",
		"urlEnvVar":"WEB_API_URL",
		"upsertCredentialMap":[{"sourceEnvVar":"WEB_SECRET","targetSecretKey":"API_KEY"}]
	}`)
	bindPreview, err := saveBindTool.Preview(newTestToolContext(t, db, adminUser, nil), createBindArgs)
	if err != nil {
		t.Fatalf("network_save_service_binding Preview: %v", err)
	}
	bindCreatedRaw, err := saveBindTool.Execute(newTestToolContext(t, db, adminUser, bindPreview.LockedPayload), createBindArgs)
	if err != nil {
		t.Fatalf("network_save_service_binding Execute: %v", err)
	}
	bindCreated := bindCreatedRaw.(map[string]any)
	bindingID := bindCreated["id"].(string)
	if bindCreated["requiresRelease"] != true {
		t.Fatalf("expected requiresRelease=true on service binding save")
	}
	creds := bindCreated["credentialMap"].([]model.SecretMapping)
	if len(creds) != 1 || creds[0].SourceEnvVar != "WEB_SECRET" {
		t.Fatalf("expected structured credentialMap slice, got %#v", bindCreated["credentialMap"])
	}

	// Patch only targetPort -> auto-resets targetPortName="grpc" without port deadlock and keeps credentialMap
	patchBindArgs := json.RawMessage(`{"project":"demo","bindingId":"` + bindingID + `","targetPort":9090}`)
	patchBindPreview, err := saveBindTool.Preview(newTestToolContext(t, db, adminUser, nil), patchBindArgs)
	if err != nil {
		t.Fatalf("patch service binding Preview: %v", err)
	}
	patchedBindRaw, err := saveBindTool.Execute(newTestToolContext(t, db, adminUser, patchBindPreview.LockedPayload), patchBindArgs)
	if err != nil {
		t.Fatalf("patch service binding Execute: %v", err)
	}
	patchedBind := patchedBindRaw.(map[string]any)
	if patchedBind["targetPort"] != 9090 || patchedBind["targetPortName"] != "grpc" || len(patchedBind["credentialMap"].([]model.SecretMapping)) != 1 {
		t.Fatalf("unexpected patched service binding: %#v", patchedBind)
	}

	// Delete Application Preview MUST fail with 409 service_binding_in_use while inbound binding exists
	delAppTool, _ := reg.Get("delivery_delete_application")
	if _, err := delAppTool.Preview(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"project":"demo","application":"web-api"}`)); err == nil {
		t.Fatalf("expected 409 service_binding_in_use on delivery_delete_application Preview")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Status != http.StatusConflict || te.Code != "service_binding_in_use" {
		t.Fatalf("expected 409 service_binding_in_use, got %#v", err)
	}

	// Delete ServiceBinding -> then Delete Application succeeds!
	delBindTool, _ := reg.Get("network_delete_service_binding")
	delBindArgs := json.RawMessage(`{"project":"demo","bindingId":"` + bindingID + `"}`)
	delBindPrev, err := delBindTool.Preview(newTestToolContext(t, db, adminUser, nil), delBindArgs)
	if err != nil {
		t.Fatalf("network_delete_service_binding Preview: %v", err)
	}
	delBindOut, err := delBindTool.Execute(newTestToolContext(t, db, adminUser, delBindPrev.LockedPayload), delBindArgs)
	if err != nil {
		t.Fatalf("network_delete_service_binding Execute: %v", err)
	}
	if delBindOut.(map[string]any)["deleted"] != true || delBindOut.(map[string]any)["projectId"] != project.ID {
		t.Fatalf("unexpected delete service binding response: %#v", delBindOut)
	}

	// Topology edge save + get + delete
	saveEdgeTool, _ := reg.Get("network_save_topology_edge")
	edgeOut, err := saveEdgeTool.Execute(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{
		"project":"demo",
		"sourceApplication":"`+backendAppID+`",
		"targetApplication":"web-api",
		"relationType":"calls",
		"protocol":"http",
		"port":8080
	}`))
	if err != nil {
		t.Fatalf("network_save_topology_edge: %v", err)
	}
	edge := edgeOut.(model.ProjectTopologyEdge)
	getTopoTool, _ := reg.Get("network_get_topology")
	topoOut, err := getTopoTool.Execute(newTestToolContext(t, db, adminUser, nil), json.RawMessage(`{"project":"demo"}`))
	if err != nil || topoOut.(map[string]any)["projectId"] != project.ID {
		t.Fatalf("network_get_topology: %v %#v", err, topoOut)
	}
	delEdgeTool, _ := reg.Get("network_delete_topology_edge")
	delEdgeArgs := json.RawMessage(`{"project":"demo","edgeId":"` + edge.ID + `"}`)
	delEdgePrev, err := delEdgeTool.Preview(newTestToolContext(t, db, adminUser, nil), delEdgeArgs)
	if err != nil {
		t.Fatalf("network_delete_topology_edge Preview: %v", err)
	}
	delEdgeRes, err := delEdgeTool.Execute(newTestToolContext(t, db, adminUser, delEdgePrev.LockedPayload), delEdgeArgs)
	if err != nil || delEdgeRes.(map[string]any)["deleted"] != true {
		t.Fatalf("network_delete_topology_edge Execute: %v %#v", err, delEdgeRes)
	}

	// Delete Application now succeeds
	delAppArgs := json.RawMessage(`{"project":"demo","application":"web-api"}`)
	delAppPrev, err := delAppTool.Preview(newTestToolContext(t, db, adminUser, nil), delAppArgs)
	if err != nil {
		t.Fatalf("delivery_delete_application Preview after unbind: %v", err)
	}
	delAppOut, err := delAppTool.Execute(newTestToolContext(t, db, adminUser, delAppPrev.LockedPayload), delAppArgs)
	if err != nil {
		t.Fatalf("delivery_delete_application Execute: %v", err)
	}
	if delAppOut.(map[string]any)["deleteStatus"] != "deleting" || len(host.enqueuedDelApp) != 1 {
		t.Fatalf("unexpected delete application result: %#v", delAppOut)
	}

	// 5. Admin Platform App Templates: create -> pure patch enabled=false preserving workload & values -> list -> delete
	savePlatTpl, _ := reg.Get("admin_save_platform_app_template")
	createPlatArgs := json.RawMessage(`{
		"slug":"custom-cache",
		"name":"Custom Cache",
		"category":"middleware",
		"workload":{"image":"redis:7-alpine","servicePort":6379,"setSecretEnv":{"REDIS_PASSWORD":"{{cachePassword}}"}},
		"upsertValues":[{"key":"cachePassword","secret":true,"autoGenerate":true}]
	}`)
	platPrev, err := savePlatTpl.Preview(newTestToolContext(t, db, adminUser, nil), createPlatArgs)
	if err != nil {
		t.Fatalf("admin_save_platform_app_template create Preview: %v", err)
	}
	platCreatedRaw, err := savePlatTpl.Execute(newTestToolContext(t, db, adminUser, platPrev.LockedPayload), createPlatArgs)
	if err != nil {
		t.Fatalf("admin_save_platform_app_template create Execute: %v", err)
	}
	platCreated := platCreatedRaw.(platformAppTemplateResponse)

	// Pure shelf toggle preserves image, servicePort, secretEnv, and values!
	disablePlatArgs := json.RawMessage(`{"template":"` + platCreated.ID + `","enabled":false}`)
	disablePrev, err := savePlatTpl.Preview(newTestToolContext(t, db, adminUser, nil), disablePlatArgs)
	if err != nil {
		t.Fatalf("admin_save_platform_app_template disable Preview: %v", err)
	}
	platDisabledRaw, err := savePlatTpl.Execute(newTestToolContext(t, db, adminUser, disablePrev.LockedPayload), disablePlatArgs)
	if err != nil {
		t.Fatalf("admin_save_platform_app_template disable Execute: %v", err)
	}
	platDisabled := platDisabledRaw.(platformAppTemplateResponse)
	if platDisabled.Enabled || platDisabled.Image != "redis:7-alpine" || platDisabled.ServicePort != 6379 || len(platDisabled.Values) != 1 {
		t.Fatalf("patch wiped platform template fields: %#v", platDisabled)
	}

	delPlatTpl, _ := reg.Get("admin_delete_platform_app_template")
	delPlatArgs := json.RawMessage(`{"template":"custom-cache"}`)
	delPlatPrev, err := delPlatTpl.Preview(newTestToolContext(t, db, adminUser, nil), delPlatArgs)
	if err != nil {
		t.Fatalf("admin_delete_platform_app_template Preview: %v", err)
	}
	delPlatOut, err := delPlatTpl.Execute(newTestToolContext(t, db, adminUser, delPlatPrev.LockedPayload), delPlatArgs)
	if err != nil || delPlatOut.(map[string]any)["deleted"] != true {
		t.Fatalf("admin_delete_platform_app_template Execute: %v %#v", err, delPlatOut)
	}
}
