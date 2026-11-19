package runtimeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type runtimeAITestHost struct {
	db             *gorm.DB
	secrets        secret.Store
	currentUserVal model.User
	projects       map[string]model.Project
	audits         []string
	deployRuns     []model.Release
	cleanups       []string
	reconciles     []string
}

func (m *runtimeAITestHost) DBFor(_ *gin.Context) *gorm.DB              { return m.db }
func (m *runtimeAITestHost) DBWithContext(ctx context.Context) *gorm.DB { return m.db.WithContext(ctx) }
func (m *runtimeAITestHost) CurrentUser(_ *gin.Context) (model.User, bool) {
	return m.currentUserVal, m.currentUserVal.ID != ""
}
func (m *runtimeAITestHost) RequirePlatformAdmin(ctx *gin.Context) bool {
	if !authz.IsPlatformAdmin(m.currentUserVal.Role) {
		writeErrorCode(ctx, http.StatusForbidden, "config.admin.required", "admin required")
		return false
	}
	return true
}
func (m *runtimeAITestHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return m.AuthorizeProjectRef(ctx, ctx.Param("projectId"), action)
}
func (m *runtimeAITestHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return m.AuthorizeProjectRef(ctx, projectID, action)
}
func (m *runtimeAITestHost) AuthorizeProjectRef(ctx *gin.Context, projectRef string, _ authz.Action) (model.User, model.Project, bool) {
	proj, err := aitool.ResolveProject(m.db, projectRef)
	if err != nil {
		if te, ok := aitool.AsToolError(err); ok {
			writeErrorCode(ctx, te.Status, te.Code, te.Message)
		} else {
			writeErrorCode(ctx, http.StatusNotFound, "project.not_found", err.Error())
		}
		return model.User{}, model.Project{}, false
	}
	return m.currentUserVal, proj, true
}
func (m *runtimeAITestHost) EnsurePlatformSystemProject(_ model.User, _ context.Context) (model.Project, error) {
	sysProj := model.Project{ID: "prj_system", Identifier: "luna-system", Name: "Luna System", KubernetesNamespace: "luna-system", DeleteStatus: "active"}
	_ = m.db.Where("id = ?", sysProj.ID).FirstOrCreate(&sysProj).Error
	return sysProj, nil
}
func (m *runtimeAITestHost) EnsureProjectCanMutate(_ *gin.Context, project model.Project) bool {
	return resourceCanMutateDuringDelete(project.DeleteStatus)
}
func (m *runtimeAITestHost) EnsureRuntimeConfigSetCanMutate(ctx *gin.Context, set model.ProjectRuntimeConfigSet) bool {
	if !resourceCanMutateDuringDelete(set.DeleteStatus) {
		writeErrorCode(ctx, http.StatusConflict, "runtime_config.delete_in_progress", "deleting")
		return false
	}
	return true
}
func (m *runtimeAITestHost) DeleteStatusCanStart(status string) bool {
	return resourceCanMutateDuringDelete(status)
}
func (m *runtimeAITestHost) ResourceDeleteAlreadyStarted(_ error) bool { return false }
func (m *runtimeAITestHost) MarkResourceDeleting(tx *gorm.DB, resource any, resourceID string) error {
	return tx.Model(resource).Where("id = ?", resourceID).Update("delete_status", "deleting").Error
}
func (m *runtimeAITestHost) MarkResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error {
	return db.Model(resource).Where("id = ?", resourceID).Updates(map[string]any{"delete_status": "delete_failed", "delete_message": message}).Error
}
func (m *runtimeAITestHost) FindProjectForCurrentUserByID(_ *gin.Context, projectID string) (model.Project, bool) {
	var p model.Project
	if err := m.db.First(&p, "id = ?", projectID).Error; err != nil {
		return model.Project{}, false
	}
	return p, true
}
func (m *runtimeAITestHost) ProjectAuthorizer(_ context.Context) authz.ProjectAuthorizer { return nil }
func (m *runtimeAITestHost) ApplyScopedResourceVisibility(_ *gin.Context, query *gorm.DB, _ string, _ model.User, _ string) (*gorm.DB, bool) {
	return query, true
}
func (m *runtimeAITestHost) ApplyScopedResourceListVisibility(_ *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	if visibility == projectservice.ListVisibilityAll && projectID == "" {
		return query, true
	}
	if projectID != "" {
		return query.Where(
			`(runtime_clusters.scope = 'global' OR (runtime_clusters.scope = 'user' AND runtime_clusters.owner_ref = ?) OR (runtime_clusters.scope = 'project' AND EXISTS (SELECT 1 FROM scoped_resource_project_bindings b WHERE b.resource_type = ? AND b.resource_id = runtime_clusters.id AND b.project_id = ?)))`,
			user.ID, resourceType, projectID,
		), true
	}
	return query, true
}
func (m *runtimeAITestHost) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool) {
	s := strings.ToLower(strings.TrimSpace(scope))
	switch s {
	case "global":
		if !authz.IsPlatformAdmin(user.Role) {
			writeErrorCode(ctx, http.StatusForbidden, "runtime_cluster.global_forbidden", globalError)
			return "", "", nil, false
		}
		return "global", "", nil, true
	case "project":
		if len(projectIDs) == 0 {
			writeErrorCode(ctx, http.StatusBadRequest, "argument_invalid", "projectIds required")
			return "", "", nil, false
		}
		return "project", "", normalizeStringList(projectIDs), true
	case "user":
		if ownerRef == "" {
			ownerRef = user.ID
		}
		return "user", ownerRef, nil, true
	default:
		writeErrorCode(ctx, http.StatusBadRequest, "argument_invalid", "invalid scope")
		return "", "", nil, false
	}
}
func (m *runtimeAITestHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, _ []string) error {
	if err := tx.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
		return err
	}
	for _, pid := range normalizeStringList(projectIDs) {
		if err := tx.Create(&model.ScopedResourceProjectBinding{
			ResourceType: resourceType,
			ResourceID:   resourceID,
			ProjectID:    pid,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
func (m *runtimeAITestHost) ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	var ids []string
	_ = m.db.WithContext(ctx).Model(&model.ScopedResourceProjectBinding{}).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("project_id asc").
		Pluck("project_id", &ids).Error
	return ids
}
func (m *runtimeAITestHost) ProjectIDsForUser(_ context.Context, _ string) []string {
	var ids []string
	_ = m.db.Model(&model.Project{}).Pluck("id", &ids).Error
	return ids
}
func (m *runtimeAITestHost) CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, _, _ string, _ context.Context) (bool, error) {
	if authz.IsPlatformAdmin(user.Role) {
		return true, nil
	}
	if scope == "user" && ownerRef == user.ID {
		return true, nil
	}
	if scope == "project" {
		return true, nil
	}
	return false, nil
}
func (m *runtimeAITestHost) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, _, _, errorMessage string) bool {
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	if scope == "global" || (scope == "user" && ownerRef != user.ID) {
		writeErrorCode(ctx, http.StatusForbidden, "runtime_cluster.forbidden", errorMessage)
		return false
	}
	return true
}
func (m *runtimeAITestHost) CanUseScopedResourceByID(user model.User, scope, ownerRef, _, _ string, _ context.Context) bool {
	if authz.IsPlatformAdmin(user.Role) || scope == "global" || scope == "project" {
		return true
	}
	return scope == "user" && ownerRef == user.ID
}
func (m *runtimeAITestHost) CurrentSessionFromCookie(_ *gin.Context) (model.UserSession, bool) {
	return model.UserSession{}, false
}
func (m *runtimeAITestHost) AuditWithContext(_, action, resource string, _ bool, _ string, _ context.Context) {
	m.audits = append(m.audits, action+":"+resource)
}
func (m *runtimeAITestHost) AuditWithSafeMetadata(_, action, resource string, _ bool, _ string, _ any, _ context.Context) {
	m.audits = append(m.audits, action+":"+resource)
}
func (m *runtimeAITestHost) EnqueueDeployRun(_ context.Context, release model.Release) bool {
	m.deployRuns = append(m.deployRuns, release)
	return true
}
func (m *runtimeAITestHost) EnqueueResourceCleanup(_ context.Context, resourceType, resourceID, _, _ string) bool {
	m.cleanups = append(m.cleanups, resourceType+":"+resourceID)
	return true
}
func (m *runtimeAITestHost) EnqueueNetworkPolicyReconcile(_ context.Context, clusterID string) bool {
	m.reconciles = append(m.reconciles, clusterID)
	return true
}
func (m *runtimeAITestHost) EnqueueNetworkPolicyProbe(_ context.Context, _ string) bool { return true }
func (m *runtimeAITestHost) ObserveDeploymentTarget(_ context.Context, _ model.Project, target model.DeploymentTarget) model.DeploymentTarget {
	target.Status = "ready"
	return target
}
func (m *runtimeAITestHost) MonitorContinuousAuthorization(_ context.Context, _ continuousAuthorizationBinding, _ func(context.Context, model.User) bool, _ func()) (<-chan struct{}, bool) {
	return nil, false
}
func (m *runtimeAITestHost) ContinuousAuthorizationActive(_ context.Context, _ continuousAuthorizationBinding, _ func(context.Context, model.User) bool) bool {
	return false
}
func (m *runtimeAITestHost) SecretStore() secret.Store                   { return m.secrets }
func (m *runtimeAITestHost) RuntimeTerminalRedis() redis.UniversalClient { return nil }
func (m *runtimeAITestHost) Mode() string                                { return "test" }
func (m *runtimeAITestHost) AllowedOrigin(_ string) bool                 { return true }
func (m *runtimeAITestHost) TaskQueueAvailable() bool                    { return true }
func (m *runtimeAITestHost) TemplateApplicationIcon(_ appstore.Template) string {
	return "activity"
}
func (m *runtimeAITestHost) ShortID(value string) string {
	if len(value) > 8 {
		return value[len(value)-6:]
	}
	return value
}
func (m *runtimeAITestHost) NextReleaseRevision(_ *gorm.DB, _, _, _ string) (int, error) {
	return 1, nil
}
func (m *runtimeAITestHost) DeploymentTargetResponse(target model.DeploymentTarget) any {
	return map[string]any{"id": target.ID, "name": target.Name, "stage": target.Stage}
}

func setupRuntimeAITestEnv(t *testing.T, user model.User) (*Handler, *runtimeAITestHost, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_ai_tools_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.Project{},
				&model.ProjectMember{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.DeploymentVolumeMount{},
				&model.ServiceBinding{},
				&model.ProjectVolume{},
				&model.ProjectRuntimeConfigSet{},
				&model.RuntimeCluster{},
				&model.ScopedResourceProjectBinding{},
				&model.NetworkEgressException{},
				&model.NetworkEgressExceptionGrant{},
				&model.NetworkServiceShare{},
				&model.SystemComponentInstallation{},
				&model.BillingUsageRecord{},
				&model.Release{},
				&model.SecretValue{},
				&model.AuditLog{},
			)
		},
	})
	codec, err := secret.NewCodec("runtime-ai-tools-test-encryption-key-32b")
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	store := secret.NewStore(db, nil, codec)
	host := &runtimeAITestHost{
		db:             db,
		secrets:        store,
		currentUserVal: user,
	}
	return New(host), host, db
}

func newTestToolContext(db *gorm.DB, user model.User, convProjectID string) *aitool.ToolContext {
	rec := httptest.NewRecorder()
	parent, _ := gin.CreateTestContext(rec)
	parent.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools", nil)
	return aitool.NewToolContext(parent, db, user, "ses_1", "conv_1", convProjectID, "run_1", "call_1", "aiidem_test_12345678", "model", nil)
}

func TestRuntimeAIToolsRegistrationCatalogAndSchemas(t *testing.T) {
	h := &Handler{}
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	if reg.Len() != 16 {
		t.Fatalf("expected 16 registered tools in runtimeapi, got %d", reg.Len())
	}

	type expectedSpec struct {
		domain        string
		tier          string
		effect        string
		adminOnly     bool
		sensitivePath []string
	}
	want := map[string]expectedSpec{
		"delivery_list_config_sets":               {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"delivery_get_config_set":                 {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"delivery_create_config_set":              {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, sensitivePath: []string{"secretFiles.*.content", "secrets.*.value"}},
		"delivery_configure_config_set":           {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, sensitivePath: []string{"secretActions.*.value", "secretFileActions.*.content"}},
		"delivery_delete_config_set":              {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync},
		"runtime_list_clusters":                   {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"runtime_inspect_cluster_governance":      {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"runtime_save_cluster":                    {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, sensitivePath: []string{"kubeconfig"}},
		"runtime_test_cluster":                    {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL2, effect: aitool.EffectExecute},
		"runtime_delete_cluster":                  {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync},
		"runtime_save_cluster_egress_exception":   {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"runtime_delete_cluster_egress_exception": {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"runtime_approve_cluster_network_share":   {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"runtime_delete_cluster_k8s_resource":     {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"admin_list_system_components":            {domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead, adminOnly: true},
		"admin_install_system_component":          {domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync, adminOnly: true},
	}

	for name, spec := range want {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing registered tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != spec.domain || cat.ApprovalTier != spec.tier || cat.Effect != spec.effect || cat.AdminOnly != spec.adminOnly {
			t.Fatalf("%s metadata mismatch: got domain=%q tier=%q effect=%q adminOnly=%v, want %+v",
				name, cat.Domain, cat.ApprovalTier, cat.Effect, cat.AdminOnly, spec)
		}
		if spec.adminOnly && !cat.GlobalSessionOnly {
			t.Fatalf("%s must set GlobalSessionOnly=true", name)
		}
		if !slices.Equal(cat.SensitivePaths, spec.sensitivePath) {
			t.Fatalf("%s sensitivePaths = %v, want %v", name, cat.SensitivePaths, spec.sensitivePath)
		}
		for _, sc := range cat.RequiredScopes {
			if !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleAdmin, sc) {
				t.Fatalf("%s declares invalid OAuth scope %q", name, sc)
			}
			if !cat.AdminOnly && !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleUser, sc) {
				t.Fatalf("tenant tool %s declares scope %q not authorizable by regular user", name, sc)
			}
		}
		if props, ok := cat.InputSchema["properties"].(map[string]any); ok {
			if ps, hasPS := props["pageSize"].(map[string]any); hasPS {
				if ps["maximum"] != int64(50) || ps["default"] != int64(20) {
					t.Fatalf("%s pageSize schema = %#v, want default=20 maximum=50", name, ps)
				}
			}
		}
	}

	// Register stub delivery_get_release to verify cross-references across the registry.
	aitool.Register(reg, aitool.Spec[struct {
		Project   string `json:"project"`
		ReleaseID string `json:"releaseId"`
	}, map[string]any]{
		Name:           "delivery_get_release",
		Domain:         aitool.DomainDelivery,
		Summary:        "stub",
		ApprovalTier:   aitool.ApprovalTierL0,
		RequiredScopes: []string{string(authz.ActionDeploymentRead)},
		Execute: func(_ *aitool.ToolContext, _ struct {
			Project   string `json:"project"`
			ReleaseID string `json:"releaseId"`
		}) (map[string]any, error) {
			return map[string]any{}, nil
		},
	})
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}
}

func TestConfigSetAIToolsLifecyclePatchAndLiveConflictValidation(t *testing.T) {
	user := model.User{ID: "usr_1", Role: authz.PlatformRoleUser}
	h, _, db := setupRuntimeAITestEnv(t, user)
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	proj := model.Project{ID: "prj_demo", Identifier: "demo", Name: "Demo Project", KubernetesNamespace: "luna-demo", DeleteStatus: "active"}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	createTool, _ := reg.Get("delivery_create_config_set")
	getTool, _ := reg.Get("delivery_get_config_set")
	listTool, _ := reg.Get("delivery_list_config_sets")
	patchTool, _ := reg.Get("delivery_configure_config_set")
	deleteTool, _ := reg.Get("delivery_delete_config_set")

	// Reject empty secret value on operation="set" (no silent skip).
	tc := newTestToolContext(db, user, proj.ID)
	if _, err := createTool.Preview(tc, json.RawMessage(`{
		"project": "demo",
		"name": "bad-empty-secret",
		"secrets": [{"key": "API_SECRET", "value": ""}]
	}`)); err == nil {
		t.Fatal("expected 400 when secret value is empty on operation=set")
	}

	// Create config set with publicEnv, secret (set + generate), configFiles, and secretFiles.
	createArgs := json.RawMessage(`{
		"project": "demo",
		"name": "shared-cfg",
		"publicEnv": [{"key": "APP_ENV", "value": "production"}, {"key": "LOG_LEVEL", "value": "info"}],
		"secrets": [
			{"key": "DB_PASSWORD", "value": "super-secret-pw"},
			{"key": "JWT_SECRET", "operation": "generate", "encoding": "hex", "length": 32}
		],
		"configFiles": [{"path": "/etc/app/config.yaml", "content": "port: 8080"}],
		"secretFiles": [{"path": "/etc/app/tls.key", "content": "PRIVATE-KEY-DATA"}]
	}`)
	prev, err := createTool.Preview(tc, createArgs)
	if err != nil {
		t.Fatalf("preview create config set: %v", err)
	}
	if prev.Preview.ProjectID != proj.ID || len(prev.Preview.SensitiveKeys) != 3 {
		t.Fatalf("unexpected create preview: %+v", prev.Preview)
	}
	createdRaw, err := createTool.Execute(tc, createArgs)
	if err != nil {
		t.Fatalf("exec create config set: %v", err)
	}
	created := createdRaw.(map[string]any)
	setID, _ := created["id"].(string)
	if created["projectId"] != proj.ID || !strings.HasPrefix(setID, "prcs_") {
		t.Fatalf("unexpected created payload: %#v", created)
	}

	// Read back via delivery_get_config_set by name: verify sorted secretKeys & secretFilePaths without refs.
	readTc := newTestToolContext(db, user, proj.ID)
	gotRaw, err := getTool.Execute(readTc, json.RawMessage(`{"project":"demo","configSet":"shared-cfg"}`))
	if err != nil {
		t.Fatalf("get config set: %v", err)
	}
	got := gotRaw.(map[string]any)
	gotJSON, _ := json.Marshal(got)
	if strings.Contains(string(gotJSON), "secret-id:") || strings.Contains(string(gotJSON), "super-secret-pw") || strings.Contains(string(gotJSON), "PRIVATE-KEY-DATA") {
		t.Fatalf("secret ref or plaintext leaked in get response: %s", string(gotJSON))
	}
	if !slices.Equal(got["secretKeys"].([]string), []string{"DB_PASSWORD", "JWT_SECRET"}) {
		t.Fatalf("unexpected secretKeys: %#v", got["secretKeys"])
	}
	if !slices.Equal(got["secretFilePaths"].([]string), []string{"/etc/app/tls.key"}) {
		t.Fatalf("unexpected secretFilePaths: %#v", got["secretFilePaths"])
	}

	// List config sets echoes projectId.
	listRaw, err := listTool.Execute(readTc, json.RawMessage(`{"project":"demo"}`))
	if err != nil {
		t.Fatalf("list config sets: %v", err)
	}
	listMap := listRaw.(map[string]any)
	if listMap["projectId"] != proj.ID || listMap["total"] != int64(1) {
		t.Fatalf("unexpected list response: %#v", listMap)
	}

	// Bind a live DeploymentTarget with an active ServiceBinding and a DeploymentVolumeMount to test §1.3 Tool 15 conflicts.
	mountPath := "/var/lib/data"
	target := model.DeploymentTarget{
		ID:                "dplt_live",
		ProjectID:         proj.ID,
		ApplicationID:     "app_1",
		Name:              "prod",
		Stage:             "prod",
		RuntimeConfigRefs: model.EncodeDeploymentRuntimeConfigRefs([]model.DeploymentRuntimeConfigRef{{SetID: setID, Mode: "live"}}),
		DeleteStatus:      "active",
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := db.Create(&model.ServiceBinding{
		ID:                       "sbind_1",
		ProjectID:                proj.ID,
		SourceApplicationID:      "app_1",
		SourceDeploymentTargetID: target.ID,
		TargetApplicationID:      "app_2",
		TargetDeploymentTargetID: "dplt_redis",
		TargetPortName:           "redis",
		TargetPort:               6379,
		Protocol:                 "tcp",
		InjectionMode:            "env",
		URLEnvVar:                "REDIS_URL",
		Enabled:                  true,
		CreatedBy:                user.ID,
	}).Error; err != nil {
		t.Fatalf("create service binding: %v", err)
	}
	if err := db.Create(&model.DeploymentVolumeMount{
		ID:                 "dvm_1",
		ProjectID:          proj.ID,
		ApplicationID:      "app_1",
		DeploymentTargetID: target.ID,
		SourceType:         model.DeploymentVolumeSourceProjectVolume,
		LogicalName:        "data",
		MountPath:          &mountPath,
		ActivationState:    model.DeploymentVolumeActivationActive,
	}).Error; err != nil {
		t.Fatalf("create volume mount: %v", err)
	}

	// 1) Public env colliding with existing secret key -> 409 deployment.runtime_environment_key_conflict
	if _, err := patchTool.Preview(tc, json.RawMessage(`{
		"project": "demo",
		"configSet": "shared-cfg",
		"setPublicEnv": [{"key": "DB_PASSWORD", "value": "oops"}]
	}`)); err == nil || !strings.Contains(err.Error(), "deployment.runtime_environment_key_conflict") {
		t.Fatalf("expected 409 deployment.runtime_environment_key_conflict, got %v", err)
	}

	// 2) Live-bound ServiceBinding env conflict -> 409 service_binding.env_conflict
	if _, err := patchTool.Preview(tc, json.RawMessage(`{
		"project": "demo",
		"configSet": "shared-cfg",
		"setPublicEnv": [{"key": "REDIS_URL", "value": "redis://custom"}]
	}`)); err == nil || !strings.Contains(err.Error(), "service_binding.env_conflict") {
		t.Fatalf("expected 409 service_binding.env_conflict, got %v", err)
	}

	// 3) Volume mount prefix conflict -> 409 deployment.runtime_config_mount_conflict
	if _, err := patchTool.Preview(tc, json.RawMessage(`{
		"project": "demo",
		"configSet": "shared-cfg",
		"upsertConfigFiles": [{"path": "/var/lib/data/seed.sql", "content": "SELECT 1;"}]
	}`)); err == nil || !strings.Contains(err.Error(), "deployment.runtime_config_mount_conflict") {
		t.Fatalf("expected 409 deployment.runtime_config_mount_conflict, got %v", err)
	}

	// 4) Pure incremental patch: add CACHE_TTL without wiping APP_ENV, DB_PASSWORD, JWT_SECRET, or /etc/app/tls.key!
	patchedRaw, err := patchTool.Execute(tc, json.RawMessage(`{
		"project": "demo",
		"configSet": "shared-cfg",
		"setPublicEnv": [{"key": "CACHE_TTL", "value": "60"}],
		"removePublicEnvKeys": ["LOG_LEVEL"]
	}`))
	if err != nil {
		t.Fatalf("configure config set: %v", err)
	}
	patched := patchedRaw.(map[string]any)
	if patched["requiresRelease"] != true || patched["affectedDeploymentTargetCount"] != 1 {
		t.Fatalf("expected requiresRelease=true and affectedDeploymentTargetCount=1, got %#v", patched)
	}
	envMap := patched["environmentVariables"].(map[string]string)
	if envMap["APP_ENV"] != "production" || envMap["CACHE_TTL"] != "60" {
		t.Fatalf("incremental patch failed to preserve APP_ENV or set CACHE_TTL: %#v", envMap)
	}
	if _, exists := envMap["LOG_LEVEL"]; exists {
		t.Fatalf("LOG_LEVEL should have been removed: %#v", envMap)
	}
	if !slices.Equal(patched["secretKeys"].([]string), []string{"DB_PASSWORD", "JWT_SECRET"}) {
		t.Fatalf("incremental patch wiped existing secrets: %#v", patched["secretKeys"])
	}
	if !slices.Equal(patched["secretFilePaths"].([]string), []string{"/etc/app/tls.key"}) {
		t.Fatalf("incremental patch wiped existing secretFiles: %#v", patched["secretFilePaths"])
	}

	// 5) Delete config set previews affected live targets and returns deleting JSON.
	delPrev, err := deleteTool.Preview(tc, json.RawMessage(`{"project":"demo","configSet":"shared-cfg"}`))
	if err != nil || len(delPrev.Preview.Impact) == 0 {
		t.Fatalf("expected delete preview with live target impact, got %+v, err=%v", delPrev, err)
	}
	delOutRaw, err := deleteTool.Execute(tc, json.RawMessage(`{"project":"demo","configSet":"shared-cfg"}`))
	if err != nil {
		t.Fatalf("delete config set: %v", err)
	}
	delOut := delOutRaw.(map[string]any)
	if delOut["id"] != setID || delOut["projectId"] != proj.ID || delOut["deleteStatus"] != "deleting" {
		t.Fatalf("unexpected delete response: %#v", delOut)
	}
}

func TestClusterAndGovernanceAndSystemComponentAITools(t *testing.T) {
	admin := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin}
	h, host, db := setupRuntimeAITestEnv(t, admin)
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	projA := model.Project{ID: "prj_a", Identifier: "proj-a", Name: "Project A", KubernetesNamespace: "luna-a", DeleteStatus: "active"}
	projB := model.Project{ID: "prj_b", Identifier: "proj-b", Name: "Project B", KubernetesNamespace: "luna-b", DeleteStatus: "active"}
	if err := db.Create(&[]model.Project{projA, projB}).Error; err != nil {
		t.Fatalf("create projects: %v", err)
	}

	saveClusterTool, _ := reg.Get("runtime_save_cluster")
	listClustersTool, _ := reg.Get("runtime_list_clusters")
	deleteClusterTool, _ := reg.Get("runtime_delete_cluster")
	saveExTool, _ := reg.Get("runtime_save_cluster_egress_exception")
	delExTool, _ := reg.Get("runtime_delete_cluster_egress_exception")
	govTool, _ := reg.Get("runtime_inspect_cluster_governance")
	listSysTool, _ := reg.Get("admin_list_system_components")
	installSysTool, _ := reg.Get("admin_install_system_component")

	globalTC := newTestToolContext(db, admin, "")
	createdClusterRaw, err := saveClusterTool.Execute(globalTC, json.RawMessage(`{
		"name": "edge-cluster",
		"scope": "global",
		"resourcePolicy": {"cpuRequestPercent": 50, "memoryRequestPercent": 80, "cpuLimitPercent": 90, "memoryLimitPercent": 95},
		"gatewayConfig": {
			"addDomainSuffixes": ["apps.example.com"],
			"addTrustedProxyCidrs": ["10.0.0.0/8"],
			"upsertDefaultRequestHeaders": {"X-Env": "edge"}
		},
		"networkSecurity": {
			"buildEgressMode": "permissive",
			"addBuildPrivateEgressPorts": ["8443"]
		}
	}`))
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	createdCluster := createdClusterRaw.(map[string]any)
	clusterID := createdCluster["id"].(string)

	// Pure incremental update of only endpoint MUST NOT reset cpuLimitPercent, domain suffixes, headers, or buildEgressMode!
	updatedClusterRaw, err := saveClusterTool.Execute(globalTC, json.RawMessage(`{
		"cluster": "edge-cluster",
		"endpoint": "https://k8s.example.com:6443"
	}`))
	if err != nil {
		t.Fatalf("incremental update cluster: %v", err)
	}
	updatedCluster := updatedClusterRaw.(map[string]any)
	if updatedCluster["cpuLimitPercent"] != 90 ||
		updatedCluster["buildEgressMode"] != "permissive" ||
		!slices.Equal(updatedCluster["gatewayDomainSuffixes"].([]string), []string{"apps.example.com"}) ||
		updatedCluster["gatewayDefaultRequestHeaders"].(map[string]string)["X-Env"] != "edge" {
		t.Fatalf("incremental cluster update lost existing fields: %#v", updatedCluster)
	}

	// List clusters sets Cache-Control: no-store and hides pressureScore/details from regular user.
	regularUser := model.User{ID: "usr_member", Role: authz.PlatformRoleUser}
	host.currentUserVal = regularUser
	userTC := newTestToolContext(db, regularUser, projA.ID)
	listRaw, err := listClustersTool.Execute(userTC, json.RawMessage(`{"project":"proj-a"}`))
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if userTC.ResponseHeaders.Get("Cache-Control") != "no-store" {
		t.Fatalf("expected Cache-Control: no-store on runtime_list_clusters")
	}
	listResp := listRaw.(map[string]any)
	if listResp["projectId"] != projA.ID {
		t.Fatalf("expected echoed projectId=%s, got %#v", projA.ID, listResp["projectId"])
	}
	firstItem := listResp["items"].([]map[string]any)[0]
	pressureMap := firstItem["pressure"].(map[string]any)
	if _, leaked := pressureMap["details"]; leaked {
		t.Fatalf("tenant user must not receive cluster pressure details: %#v", pressureMap)
	}
	host.currentUserVal = admin

	// Egress exception: create scoped to target dplt_a1, then update reason omitting targets -> targets MUST be preserved!
	if err := db.Create(&model.Application{ID: "app_a", ProjectID: projA.ID, Identifier: "api", Name: "API", DeleteStatus: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.DeploymentTarget{ID: "dplt_a1", ProjectID: projA.ID, ApplicationID: "app_a", Name: "prod", Stage: "prod", ClusterID: clusterID, DeleteStatus: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	exCreatedRaw, err := saveExTool.Execute(globalTC, json.RawMessage(`{
		"cluster": "edge-cluster",
		"name": "corp-db",
		"cidr": "192.168.10.0/24",
		"ports": "5432",
		"reason": "Connect to legacy PostgreSQL",
		"upsertGrants": [{"project": "proj-a", "targets": ["api:prod"]}]
	}`))
	if err != nil {
		t.Fatalf("create egress exception: %v", err)
	}
	exID := exCreatedRaw.(map[string]any)["id"].(string)

	// Update grant includeBuilds=true while omitting targets: must preserve dplt_a1 restriction!
	exUpdatedRaw, err := saveExTool.Execute(globalTC, json.RawMessage(`{
		"cluster": "edge-cluster",
		"exception": "corp-db",
		"upsertGrants": [{"project": "proj-a", "includeBuilds": true}]
	}`))
	if err != nil {
		t.Fatalf("update egress exception: %v", err)
	}
	var rawGrants []map[string]any
	grantsBytes, _ := json.Marshal(exUpdatedRaw.(map[string]any)["grants"])
	_ = json.Unmarshal(grantsBytes, &rawGrants)
	if len(rawGrants) != 1 || len(rawGrants[0]["deploymentTargetIds"].([]any)) != 1 || rawGrants[0]["deploymentTargetIds"].([]any)[0] != "dplt_a1" {
		t.Fatalf("R5/R7 violation: omitting targets widened existing grant: %s", string(grantsBytes))
	}

	// Inspect governance egress_exceptions.
	govOutRaw, err := govTool.Execute(globalTC, json.RawMessage(`{"cluster":"edge-cluster","section":"egress_exceptions","exception":"corp-db"}`))
	if err != nil || govOutRaw.(map[string]any)["total"] != int64(1) {
		t.Fatalf("inspect egress_exceptions failed: %#v, err=%v", govOutRaw, err)
	}

	// Single-project session check on runtime_delete_cluster_egress_exception:
	// Bound to projB -> must reject with 403 ai.project_scope_mismatch!
	projBTC := newTestToolContext(db, admin, projB.ID)
	if _, err := delExTool.Preview(projBTC, json.RawMessage(`{"cluster":"edge-cluster","exception":"corp-db"}`)); err == nil || !strings.Contains(err.Error(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch when deleting projA exception from projB session, got %v", err)
	}
	// Bound to projA -> succeeds and returns {id, deleted: true, projectId}.
	projATC := newTestToolContext(db, admin, projA.ID)
	delExOutRaw, err := delExTool.Execute(projATC, json.RawMessage(`{"cluster":"edge-cluster","exception":"corp-db"}`))
	delExOut := delExOutRaw.(map[string]any)
	if err != nil || delExOut["id"] != exID || delExOut["deleted"] != true || delExOut["projectId"] != projA.ID {
		t.Fatalf("delete egress exception failed: %#v, err=%v", delExOut, err)
	}

	// Admin system components list & install.
	sysListOutRaw, err := listSysTool.Execute(globalTC, json.RawMessage(`{}`))
	sysListOut := sysListOutRaw.(map[string]any)
	if err != nil || sysListOut["observationCode"] != "billing.gateway_traffic_probe_not_configured" {
		t.Fatalf("unexpected admin_list_system_components before install: %#v, err=%v", sysListOut, err)
	}
	sysInstallOutRaw, err := installSysTool.Execute(globalTC, json.RawMessage(`{
		"template": "luna-gateway-traffic-probe",
		"cluster": "edge-cluster",
		"apiBaseUrl": "https://devops.example.com"
	}`))
	if err != nil {
		t.Fatalf("admin_install_system_component: %v", err)
	}
	sysInstallOut := sysInstallOutRaw.(map[string]any)
	if sysInstallOut["projectId"] != "prj_system" || sysInstallOut["status"] != "pending" || sysInstallOut["releaseId"] == "" || sysInstallOut["installationId"] == "" {
		t.Fatalf("unexpected admin_install_system_component output: %#v", sysInstallOut)
	}

	// Cluster delete: blocked with 409 while dplt_a1 + system target reference it; succeeds on unreferenced cluster.
	if _, err := deleteClusterTool.Preview(globalTC, json.RawMessage(`{"cluster":"edge-cluster"}`)); err == nil || !strings.Contains(err.Error(), "runtime_cluster.in_use") {
		t.Fatalf("expected 409 runtime_cluster.in_use on referenced cluster, got %v", err)
	}
	unusedRaw, err := saveClusterTool.Execute(globalTC, json.RawMessage(`{"name":"temp-cluster","scope":"user"}`))
	if err != nil {
		t.Fatalf("create temp cluster: %v", err)
	}
	unusedID := unusedRaw.(map[string]any)["id"].(string)
	delClusterOutRaw, err := deleteClusterTool.Execute(globalTC, json.RawMessage(`{"cluster":"temp-cluster"}`))
	delClusterOut := delClusterOutRaw.(map[string]any)
	if err != nil || delClusterOut["id"] != unusedID || delClusterOut["deleteStatus"] != "deleting" {
		t.Fatalf("delete unreferenced cluster failed: %#v, err=%v", delClusterOut, err)
	}
}

func TestConfigSetIncrementalPatchAndHelperInvariantsInMemory(t *testing.T) {
	existing := model.ProjectRuntimeConfigSet{
		ID:          "prcs_mem",
		ProjectID:   "prj_mem",
		Name:        "base-cfg",
		EnvVars:     `{"APP_ENV":"prod","LOG_LEVEL":"info"}`,
		SecretRefs:  `{"DB_PASS":"secret-id:sec_1"}`,
		ConfigFiles: `[{"path":"/etc/app/app.conf","content":"a=1"}]`,
		SecretFiles: `{"/etc/app/tls.key":"secret-id:sec_2"}`,
		Enabled:     true,
	}

	// Omitted fields must be preserved 100% in pure incremental patch.
	patch, err := prepareConfigureConfigSetAI(existing, aiConfigureConfigSetInput{
		SetPublicEnv:        []aiConfigSetPublicEnvItem{{Key: "PORT", Value: "8080"}},
		RemovePublicEnvKeys: []string{"LOG_LEVEL"},
	})
	if err != nil {
		t.Fatalf("prepareConfigureConfigSetAI: %v", err)
	}
	if patch.publicEnv["APP_ENV"] != "prod" || patch.publicEnv["PORT"] != "8080" {
		t.Fatalf("expected APP_ENV preserved and PORT added, got %#v", patch.publicEnv)
	}
	if _, hasLog := patch.publicEnv["LOG_LEVEL"]; hasLog {
		t.Fatalf("expected LOG_LEVEL removed, got %#v", patch.publicEnv)
	}
	if _, hasSecret := patch.nextSecretKeys["DB_PASS"]; !hasSecret {
		t.Fatalf("expected DB_PASS secret key preserved, got %#v", patch.nextSecretKeys)
	}
	if !slices.Equal(patch.nextSecretPaths, []string{"/etc/app/tls.key"}) {
		t.Fatalf("expected /etc/app/tls.key preserved, got %#v", patch.nextSecretPaths)
	}

	// Colliding publicEnv with existing secret key -> 409 deployment.runtime_environment_key_conflict.
	if _, err := prepareConfigureConfigSetAI(existing, aiConfigureConfigSetInput{
		SetPublicEnv: []aiConfigSetPublicEnvItem{{Key: "DB_PASS", Value: "plain"}},
	}); err == nil || !strings.Contains(err.Error(), "deployment.runtime_environment_key_conflict") {
		t.Fatalf("expected 409 deployment.runtime_environment_key_conflict, got %v", err)
	}

	// Heterogeneous file path conflict (configFiles <-> secretFiles) -> 409.
	if _, err := prepareConfigureConfigSetAI(existing, aiConfigureConfigSetInput{
		UpsertConfigFiles: []aiConfigSetPlainFileItem{{Path: "/etc/app/tls.key", Content: "not-secret"}},
	}); err == nil || !strings.Contains(err.Error(), "deployment.runtime_config_mount_conflict") {
		t.Fatalf("expected 409 deployment.runtime_config_mount_conflict, got %v", err)
	}

	// Parent-child directory prefix conflict -> 409.
	if _, err := prepareConfigureConfigSetAI(existing, aiConfigureConfigSetInput{
		UpsertConfigFiles: []aiConfigSetPlainFileItem{{Path: "/etc/app/app.conf/nested.yaml", Content: "x"}},
	}); err == nil || !strings.Contains(err.Error(), "deployment.runtime_config_mount_conflict") {
		t.Fatalf("expected 409 prefix conflict, got %v", err)
	}

	// Empty value on operation="set" -> 400 argument_invalid (never silently skipped).
	empty := "   "
	if _, err := prepareConfigureConfigSetAI(existing, aiConfigureConfigSetInput{
		SecretActions: []aiConfigSetSecretActionItem{{Key: "NEW_SEC", Value: &empty}},
	}); err == nil || !strings.Contains(err.Error(), "argument_invalid") {
		t.Fatalf("expected 400 argument_invalid on empty secret value, got %v", err)
	}

	// Detail map never exposes secret-id refs.
	h := &Handler{}
	detail := h.configSetDetailMap(existing, 2)
	detailBytes, _ := json.Marshal(detail)
	if strings.Contains(string(detailBytes), "secret-id:") {
		t.Fatalf("configSetDetailMap leaked secret ref: %s", string(detailBytes))
	}
	if !slices.Equal(detail["secretKeys"].([]string), []string{"DB_PASS"}) ||
		!slices.Equal(detail["secretFilePaths"].([]string), []string{"/etc/app/tls.key"}) ||
		detail["affectedDeploymentTargetCount"] != 2 {
		t.Fatalf("unexpected configSetDetailMap: %#v", detail)
	}

	// UTF-8 safe byte truncation & K8s error classification.
	truncStr, truncated, cutBytes := truncateUTF8Bytes("你好世界Hello", 7)
	if !truncated || cutBytes <= 0 || truncStr != "你好" {
		t.Fatalf("truncateUTF8Bytes = (%q, %v, %d), want (\"你好\", true, >0)", truncStr, truncated, cutBytes)
	}
	if te := classifyClusterK8sResourceError(json.Unmarshal([]byte("{"), &struct{}{})); te.Status != http.StatusBadGateway {
		t.Fatalf("unexpected fallback status: %+v", te)
	}
	if te := classifyClusterK8sResourceError(&aitool.ToolError{Message: "Gateway API CRDs are not installed or httproutes \"demo\" does not exist"}); te.Status != http.StatusNotFound || te.Code != "cluster.resource_not_found" {
		t.Fatalf("expected 404 cluster.resource_not_found, got %+v", te)
	}
	if te := classifyClusterK8sResourceError(&aitool.ToolError{Message: "resource is not managed by Luna DevOps"}); te.Status != http.StatusForbidden || te.Code != "cluster.resource_unmanaged" {
		t.Fatalf("expected 403 cluster.resource_unmanaged, got %+v", te)
	}

	// Structured cluster DTO hides pressureScore & details from non-admin.
	score := 64.5
	pr := &runtimeClusterPressureResponse{
		ClusterID:     "clu_1",
		Status:        "ready",
		PressureLevel: "moderate",
		PressureScore: &score,
		Details:       &runtimeClusterPressureDetails{NodeCount: 5, PodCount: 80},
	}
	tenantDTO := structuredRuntimeClusterDTO(model.RuntimeCluster{
		ID:                           "clu_1",
		GatewayDefaultRequestHeaders: `{"X-Real-IP":"1.2.3.4"}`,
		GatewayTrustedProxyCIDRs:     "10.0.0.0/8\n172.16.0.0/12",
		BuildPrivateEgressPorts:      "443,8443",
		GatewayDomainSuffixesRaw:     `["app.example.com"]`,
	}, pr, false)
	if _, leaked := tenantDTO["pressure"].(map[string]any)["details"]; leaked {
		t.Fatalf("tenant DTO leaked pressure details: %#v", tenantDTO["pressure"])
	}
	if _, leaked := tenantDTO["pressure"].(map[string]any)["pressureScore"]; leaked {
		t.Fatalf("tenant DTO leaked pressureScore: %#v", tenantDTO["pressure"])
	}
	if !slices.Equal(tenantDTO["buildPrivateEgressPorts"].([]string), []string{"443", "8443"}) ||
		tenantDTO["gatewayDefaultRequestHeaders"].(map[string]string)["X-Real-IP"] != "1.2.3.4" {
		t.Fatalf("unexpected structured cluster DTO: %#v", tenantDTO)
	}
}
