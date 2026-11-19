package buildapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type buildAITestHost struct {
	Host
	db         *gorm.DB
	store      secret.Store
	canInspect bool
	enqueued   []tasks.BuildRunPayload
}

func (h *buildAITestHost) DBFor(*gin.Context) *gorm.DB { return h.db }
func (h *buildAITestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return h.db.WithContext(ctx)
}
func (h *buildAITestHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	v, ok := ctx.Get("luna.devops.current_user")
	if !ok {
		return model.User{}, false
	}
	u, ok := v.(model.User)
	return u, ok
}
func (h *buildAITestHost) RequirePlatformAdmin(ctx *gin.Context) bool {
	u, ok := h.CurrentUser(ctx)
	return ok && authz.IsPlatformAdmin(u.Role)
}
func (h *buildAITestHost) AuthorizeProjectRef(ctx *gin.Context, projectRef string, _ authz.Action) (model.User, model.Project, bool) {
	u, ok := h.CurrentUser(ctx)
	if !ok {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.unauthorized", "unauthorized")
		return model.User{}, model.Project{}, false
	}
	proj, err := aitool.ResolveProject(h.db, projectRef)
	if err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "project.not_found", err.Error())
		return model.User{}, model.Project{}, false
	}
	return u, proj, true
}
func (h *buildAITestHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return h.AuthorizeProjectRef(ctx, projectID, action)
}
func (h *buildAITestHost) FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	_, p, ok := h.AuthorizeProjectByID(ctx, projectID, authz.ActionProjectRead)
	return p, ok
}
func (h *buildAITestHost) ResolveListVisibility(_ *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	if authz.IsPlatformAdmin(user.Role) {
		return projectservice.ListVisibilityAll, true
	}
	return projectservice.ListVisibilityRelated, true
}
func (h *buildAITestHost) ApplyScopedResourceListVisibility(_ *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	if projectID != "" {
		sub := h.db.Model(&model.ScopedResourceProjectBinding{}).
			Select("resource_id").
			Where("resource_type = ? AND project_id = ?", resourceType, projectID)
		return query.Where("scope = 'global' OR (scope = 'user' AND owner_ref = ?) OR (scope = 'project' AND id IN (?))", user.ID, sub), true
	}
	if visibility == projectservice.ListVisibilityAll {
		return query, true
	}
	return query.Where("scope = 'global' OR (scope = 'user' AND owner_ref = ?) OR scope = 'project'", user.ID), true
}
func (h *buildAITestHost) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "global":
		if !authz.IsPlatformAdmin(user.Role) {
			writeError(ctx, http.StatusForbidden, globalError)
			return "", "", nil, false
		}
		return "global", "", nil, true
	case "project":
		if len(projectIDs) == 0 {
			writeError(ctx, http.StatusBadRequest, "projectIds required")
			return "", "", nil, false
		}
		return "project", "", sortedProjectIDs(projectIDs), true
	default:
		if ownerRef == "" {
			ownerRef = user.ID
		}
		return "user", ownerRef, nil, true
	}
}
func (h *buildAITestHost) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, _, _, errMsg string) bool {
	if authz.IsPlatformAdmin(user.Role) || (scope == "user" && ownerRef == user.ID) || scope == "project" {
		return true
	}
	writeError(ctx, http.StatusForbidden, errMsg)
	return false
}
func (h *buildAITestHost) CanInspectScopedResourceConfigByID(_ model.User, _, _, _, _ string, _ context.Context) (bool, error) {
	return h.canInspect, nil
}
func (h *buildAITestHost) CanUseScopedResourceByID(_ model.User, _, _, _, _ string, _ context.Context) bool {
	return true
}
func (h *buildAITestHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, _ []string) error {
	if err := tx.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
		return err
	}
	for _, pid := range projectIDs {
		row := model.ScopedResourceProjectBinding{
			ID:           resourceType + ":" + resourceID + ":" + pid,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			ProjectID:    pid,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
func (h *buildAITestHost) ScopedResourceProjectIDs(resourceType, resourceID string, _ context.Context) []string {
	var ids []string
	_ = h.db.Model(&model.ScopedResourceProjectBinding{}).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("project_id asc").
		Pluck("project_id", &ids).Error
	return ids
}
func (h *buildAITestHost) ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, _ context.Context) map[string][]string {
	out := map[string][]string{}
	if len(resourceIDs) == 0 {
		return out
	}
	var rows []model.ScopedResourceProjectBinding
	_ = h.db.Where("resource_type = ? AND resource_id IN ?", resourceType, resourceIDs).Find(&rows).Error
	for _, r := range rows {
		out[r.ResourceID] = append(out[r.ResourceID], r.ProjectID)
	}
	return out
}
func (h *buildAITestHost) ProjectRoleActionAllowed(_ context.Context, user model.User, _ string, _ authz.Action) (bool, error) {
	return user.ID == "usr_owner" || user.ID == "usr_dev2" || authz.IsPlatformAdmin(user.Role), nil
}
func (h *buildAITestHost) AuditWithContext(string, string, string, bool, string, context.Context) {}
func (h *buildAITestHost) EnsureProjectCanMutate(_ *gin.Context, proj model.Project) bool {
	return model.DeleteStatusActive(proj.DeleteStatus)
}
func (h *buildAITestHost) EnsureBillingAllowsNewBuild(*gin.Context, string) bool { return true }
func (h *buildAITestHost) RegistryPushCredentialForProject(_ model.User, reg model.ArtifactRegistry, _ string, _ context.Context) (model.RegistryCredential, bool) {
	return model.RegistryCredential{ID: "regc_push", RegistryID: reg.ID, Usage: "push-pull"}, true
}
func (h *buildAITestHost) SecretAvailable() bool { return h.store.Available() }
func (h *buildAITestHost) StoreSecret(ctx context.Context, value, userID, resource string) string {
	return h.store.StoreContext(ctx, value, userID, resource)
}
func (h *buildAITestHost) StoreSecretWithDB(ctx context.Context, db *gorm.DB, value, userID, resource string) (string, error) {
	return h.store.StoreContextWithDB(ctx, db, value, userID, resource)
}
func (h *buildAITestHost) DeleteSecretRefWithDB(ctx context.Context, db *gorm.DB, ref, resource string) error {
	return h.store.DeleteRefContextWithDB(ctx, db, ref, resource)
}
func (h *buildAITestHost) ResolveSecret(ctx context.Context, ref string) string {
	return h.store.ResolveContext(ctx, ref)
}
func (h *buildAITestHost) BuildQueueAvailable() bool { return true }
func (h *buildAITestHost) EnqueueBuildRun(_ context.Context, payload tasks.BuildRunPayload) error {
	h.enqueued = append(h.enqueued, payload)
	return nil
}
func (h *buildAITestHost) ApplicationCanMutate(model.Application) bool { return true }
func (h *buildAITestHost) NormalizeDeploymentSourceType(v string) string {
	if strings.TrimSpace(v) == "" {
		return "repository"
	}
	return strings.ToLower(strings.TrimSpace(v))
}
func (h *buildAITestHost) NormalizeBuildResourceQuantityValue(v, fb, _ string) (string, error) {
	if strings.TrimSpace(v) == "" {
		return fb, nil
	}
	return strings.TrimSpace(v), nil
}
func (h *buildAITestHost) NormalizeBuildTimeoutSecondsValue(v int) int {
	if v <= 0 {
		return 1800
	}
	return v
}

func TestBuildAIToolsCatalogContract(t *testing.T) {
	reg := aitool.NewRegistry()
	h := New(&buildAITestHost{})
	h.RegisterAITools(reg)

	if reg.Len() != 14 {
		t.Fatalf("expected 14 registered build AI tools, got %d", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}

	expected := map[string]struct {
		domain         string
		tier           string
		effect         string
		adminOnly      bool
		sensitivePaths []string
	}{
		"build_list_templates":             {aitool.DomainBuild, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"build_list_variable_sets":         {aitool.DomainBuild, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"build_get_scoped_env":             {aitool.DomainBuild, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"build_list_runs":                  {aitool.DomainBuild, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"build_get_run":                    {aitool.DomainBuild, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"build_save_variable_set":          {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, []string{"secretActions.*.value"}},
		"build_delete_variable_set":        {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"build_configure_scoped_env":       {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, []string{"secretActions.*.value"}},
		"build_trigger_run":                {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectAsync, false, nil},
		"build_retry_run":                  {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectAsync, false, nil},
		"build_cancel_run":                 {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"build_delete_run":                 {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"admin_get_global_build_env":       {aitool.DomainAdmin, aitool.ApprovalTierL0, aitool.EffectRead, true, nil},
		"admin_configure_global_build_env": {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectWrite, true, []string{"secretActions.*.value"}},
	}

	for name, exp := range expected {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != exp.domain || cat.ApprovalTier != exp.tier || cat.Effect != exp.effect || cat.AdminOnly != exp.adminOnly {
			t.Fatalf("tool %s metadata mismatch: %+v", name, cat)
		}
		if exp.adminOnly && !cat.GlobalSessionOnly {
			t.Fatalf("admin tool %s must set GlobalSessionOnly=true", name)
		}
		if exp.tier == aitool.ApprovalTierL2 && !tool.HasPreview {
			t.Fatalf("L2 tool %s must provide Preview", name)
		}
		if !slices.Equal(cat.SensitivePaths, exp.sensitivePaths) {
			t.Fatalf("tool %s sensitivePaths = %v, want %v", name, cat.SensitivePaths, exp.sensitivePaths)
		}
		roleForCheck := authz.PlatformRoleUser
		if cat.AdminOnly {
			roleForCheck = authz.PlatformRoleAdmin
		}
		for _, sc := range cat.RequiredScopes {
			if norm := authz.NormalizeOAuthScope(sc); norm != sc || !authz.UserCanAuthorizeOAuthScope(roleForCheck, sc) {
				t.Fatalf("tool %s has invalid OAuth scope %q for role %s", name, sc, roleForCheck)
			}
		}
	}
}

func TestBuildAIToolsVariableSetsScopedEnvAndRunLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "build_ai_tools_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.Project{},
				&model.ProjectMember{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.RepositoryBinding{},
				&model.ArtifactRegistry{},
				&model.ContainerImage{},
				&model.BuildVariableSet{},
				&model.BuildEnvironmentConfig{},
				&model.BuildRun{},
				&model.BuildJob{},
				&model.BuildLog{},
				&model.HookRun{},
				&model.HookRunLog{},
				&model.ScopedResourceProjectBinding{},
				&model.SecretValue{},
			)
		},
	})
	codec, err := secret.NewCodec("build-ai-tools-test-secret-key")
	if err != nil {
		t.Fatal(err)
	}
	store := secret.NewStore(db, nil, codec)

	host := &buildAITestHost{db: db, store: store, canInspect: true}
	h := New(host)
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	admin := model.User{ID: "usr_admin", Email: "admin@example.test", Name: "Admin", Role: authz.PlatformRoleAdmin}
	owner := model.User{ID: "usr_owner", Email: "owner@example.test", Name: "Owner", Role: authz.PlatformRoleUser}
	dev2 := model.User{ID: "usr_dev2", Email: "dev2@example.test", Name: "Dev2", Role: authz.PlatformRoleUser}
	proj := model.Project{ID: "prj_1", Identifier: "demo", Name: "Demo", DeleteStatus: "active"}
	app := model.Application{ID: "app_1", ProjectID: proj.ID, Identifier: "api", Name: "API"}
	binding := model.RepositoryBinding{ID: "rpb_1", ProjectID: proj.ID, ApplicationID: app.ID, Owner: "team", Repo: "api", DefaultBranch: "develop"}
	artifactReg := model.ArtifactRegistry{ID: "reg_1", Name: "Harbor", Provider: "harbor", Endpoint: "https://registry.example.test", Scope: "global"}
	targetDev := model.DeploymentTarget{
		ID: "dplt_dev", ProjectID: proj.ID, ApplicationID: app.ID, Name: "Dev", Stage: "dev",
		KubernetesName: "api-dev", SourceType: "repository", RepositoryBindingID: binding.ID,
		TargetRegistryID: artifactReg.ID, TargetRepository: "team/api", TargetTag: "default-tag",
		BuildArgs: `{"BASE":"alpine"}`, Enabled: true, DeleteStatus: "active",
	}
	for _, row := range []any{&admin, &owner, &dev2, &proj, &app, &binding, &artifactReg, &targetDev} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	makeTC := func(u model.User, convProj, idemKey string) *aitool.ToolContext {
		parentRec := httptest.NewRecorder()
		parentGin, _ := gin.CreateTestContext(parentRec)
		parentGin.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)
		return aitool.NewToolContext(parentGin, db, u, "ses_1", "conv_1", convProj, "run_1", "call_1", idemKey, "model", nil)
	}

	// 1. build_list_templates in-memory Dockerfile preview
	tplTool, _ := reg.Get("build_list_templates")
	tplOutRaw, err := tplTool.Execute(makeTC(owner, "", ""), json.RawMessage(`{"template":"node-service","previewValues":{"port":"4000"}}`))
	if err != nil {
		t.Fatalf("build_list_templates preview failed: %v", err)
	}
	tplPreview := tplOutRaw.(map[string]any)["preview"]
	if tplPreview == nil {
		t.Fatalf("expected rendered preview in build_list_templates output: %#v", tplOutRaw)
	}

	// 2. build_save_variable_set guards LITEYUKI_* & bidirectional key conflicts, preserves existing secrets
	saveSetTool, _ := reg.Get("build_save_variable_set")
	if _, err := saveSetTool.Preview(makeTC(owner, "prj_1", ""), json.RawMessage(`{
		"name":"BadSet","setVariables":[{"key":"LITEYUKI_INTERNAL","value":"1"}]
	}`)); err == nil {
		t.Fatal("expected LITEYUKI_* reserved prefix to be rejected")
	}
	if _, err := saveSetTool.Preview(makeTC(owner, "prj_1", ""), json.RawMessage(`{
		"name":"ConflictSet",
		"setVariables":[{"key":"NPM_TOKEN","value":"plain"}],
		"secretActions":[{"key":"NPM_TOKEN","operation":"set","value":"secret"}]
	}`)); err == nil {
		t.Fatal("expected bidirectional key conflict 409 build.variable_key_conflict")
	}

	setOutRaw, err := saveSetTool.Execute(makeTC(owner, "prj_1", ""), json.RawMessage(`{
		"name":"NodeSecrets",
		"setVariables":[{"key":"NODE_OPTIONS","value":"--max-old-space-size=4096"}],
		"secretActions":[{"key":"NPM_TOKEN","value":"npm_secret_1"}]
	}`))
	if err != nil {
		t.Fatalf("build_save_variable_set create failed: %v", err)
	}
	personalSet := setOutRaw.(aiBuildVariableSetOutput)
	if !slices.Equal(personalSet.SecretKeys, []string{"NPM_TOKEN"}) || personalSet.Scope != "user" {
		t.Fatalf("unexpected variable set output: %+v", personalSet)
	}

	// Redaction check on build_list_variable_sets when canInspect == false
	host.canInspect = false
	listSetTool, _ := reg.Get("build_list_variable_sets")
	listSetRaw, err := listSetTool.Execute(makeTC(owner, "prj_1", ""), json.RawMessage(`{"variableSet":"`+personalSet.ID+`"}`))
	if err != nil {
		t.Fatalf("build_list_variable_sets failed: %v", err)
	}
	listItems := listSetRaw.(map[string]any)["items"].([]aiBuildVariableSetOutput)
	if len(listItems) != 1 || listItems[0].CanInspectVariables || len(listItems[0].Variables) != 0 || listItems[0].VariableCount != 1 {
		t.Fatalf("expected redacted variables with variableCount=1, got %+v", listItems)
	}
	host.canInspect = true

	// 3. build_configure_scoped_env & admin_configure_global_build_env atomic secret cleanup
	cfgScopedTool, _ := reg.Get("build_configure_scoped_env")
	scopedResRaw, err := cfgScopedTool.Execute(makeTC(owner, "prj_1", ""), json.RawMessage(`{
		"scope":"deployment","project":"demo","application":"api","target":"dev",
		"setVariables":[{"key":"GOPROXY","value":"https://goproxy.cn"}],
		"secretActions":[{"key":"SENTRY_AUTH_TOKEN","value":"tok_v1"}]
	}`))
	if err != nil {
		t.Fatalf("build_configure_scoped_env failed: %v", err)
	}
	scopedRes := scopedResRaw.(map[string]any)
	if scopedRes["projectId"] != proj.ID || scopedRes["requiresRelease"] != false {
		t.Fatalf("unexpected scoped env output: %#v", scopedRes)
	}
	// Clear SENTRY_AUTH_TOKEN -> must physically delete row from secret_values!
	if _, err := cfgScopedTool.Execute(makeTC(owner, "prj_1", ""), json.RawMessage(`{
		"scope":"deployment","project":"demo","application":"api","target":"dev",
		"secretActions":[{"key":"SENTRY_AUTH_TOKEN","operation":"clear"}]
	}`)); err != nil {
		t.Fatalf("build_configure_scoped_env clear secret failed: %v", err)
	}
	var envSecretCount int64
	_ = db.Model(&model.SecretValue{}).Where("resource LIKE ?", "build_environment:deployment:%").Count(&envSecretCount).Error
	if envSecretCount != 0 {
		t.Fatalf("expected physical deletion of cleared build_environment secret_values, got %d", envSecretCount)
	}

	cfgGlobalTool, _ := reg.Get("admin_configure_global_build_env")
	if _, err := cfgGlobalTool.Execute(makeTC(admin, "", ""), json.RawMessage(`{
		"setVariables":[{"key":"GLOBAL_GOPROXY","value":"direct"}]
	}`)); err != nil {
		t.Fatalf("admin_configure_global_build_env failed: %v", err)
	}

	// 4. Attach personalSet to targetDev, test build_trigger_run (overrideBuildArgs + targetTag + idempotency)
	_ = db.Model(&model.DeploymentTarget{}).Where("id = ?", targetDev.ID).Update("build_variable_set_ids", `["`+personalSet.ID+`"]`).Error

	// Add second active target -> omitting target must fail with 409 ai.resource_ambiguous!
	targetProd := model.DeploymentTarget{
		ID: "dplt_prod", ProjectID: proj.ID, ApplicationID: app.ID, Name: "Prod", Stage: "prod",
		KubernetesName: "api-prod", SourceType: "repository", RepositoryBindingID: binding.ID,
		TargetRegistryID: artifactReg.ID, Enabled: true, DeleteStatus: "active",
	}
	_ = db.Create(&targetProd).Error

	triggerTool, _ := reg.Get("build_trigger_run")
	if _, err := triggerTool.Preview(makeTC(owner, "prj_1", ""), json.RawMessage(`{"project":"demo","application":"api"}`)); err == nil {
		t.Fatal("expected 409 ai.resource_ambiguous when target is omitted and 2 active targets exist")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "ai.resource_ambiguous" {
		t.Fatalf("expected ai.resource_ambiguous, got %#v", err)
	}

	trigRaw, err := triggerTool.Execute(makeTC(owner, "prj_1", "aiidem_trigger_0001"), json.RawMessage(`{
		"project":"demo","application":"api","target":"dev",
		"targetTag":"v9.9.9","overrideBuildArgs":{"EXTRA":"yes"}
	}`))
	if err != nil {
		t.Fatalf("build_trigger_run failed: %v", err)
	}
	trigMap := trigRaw.(map[string]any)
	runID := trigMap["buildRunId"].(string)
	if trigMap["targetTag"] != "v9.9.9" || trigMap["sourceBranch"] != "develop" || trigMap["projectId"] != proj.ID {
		t.Fatalf("unexpected triggered build run: %#v", trigMap)
	}
	// Replay with same Idempotency-Key returns exact same buildRunId without creating a second BuildRun or BuildJob
	replayRaw, err := triggerTool.Execute(makeTC(owner, "prj_1", "aiidem_trigger_0001"), json.RawMessage(`{
		"project":"demo","application":"api","target":"dev",
		"targetTag":"v9.9.9","overrideBuildArgs":{"EXTRA":"yes"}
	}`))
	var runRowCount int64
	_ = db.Model(&model.BuildRun{}).Count(&runRowCount).Error
	if err != nil || replayRaw.(map[string]any)["buildRunId"] != runID || runRowCount != 1 || len(host.enqueued) != 2 || host.enqueued[0] != host.enqueued[1] {
		t.Fatalf("idempotent replay failed: err=%v replay=%#v runRowCount=%d enqueued=%#v", err, replayRaw, runRowCount, host.enqueued)
	}

	// 5. build_retry_run blocks dev2 from retrying a build that captured owner's personal scope=user BuildVariableSet!
	retryTool, _ := reg.Get("build_retry_run")
	if _, err := retryTool.Preview(makeTC(dev2, "prj_1", ""), json.RawMessage(`{
		"project":"demo","buildRunId":"`+runID+`"
	}`)); err == nil {
		t.Fatal("expected dev2 retry of build with owner's personal variable set to be rejected with 403")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Status != http.StatusForbidden {
		t.Fatalf("expected 403 forbidden on cross-user retry, got %#v", err)
	}

	// 6. Seed BuildLog (with multibyte UTF-8) & HookRun (with large ScriptSnapshot) and verify build_get_run
	_ = db.Create(&model.BuildLog{
		ID: "bll_1", BuildRunID: runID, BuildJobID: "bldj_x", ProjectID: proj.ID,
		Content: strings.Repeat("构建日志行🚀\n", 100),
	}).Error
	_ = db.Create(&model.HookRun{
		ID: "hkr_1", ProjectID: proj.ID, BuildRunID: runID, Name: "pre-build", Phase: "preBuild",
		Status: "succeeded", ScriptSnapshot: "SECRET_LARGE_SCRIPT_SNAPSHOT_MUST_NOT_APPEAR",
	}).Error

	getRunTool, _ := reg.Get("build_get_run")
	getRunRaw, err := getRunTool.Execute(makeTC(owner, "prj_1", ""), json.RawMessage(`{
		"project":"demo","buildRunId":"`+runID+`","logTailBytes":256
	}`))
	if err != nil {
		t.Fatalf("build_get_run failed: %v", err)
	}
	encodedGetRun, _ := json.Marshal(getRunRaw)
	if strings.Contains(string(encodedGetRun), "SECRET_LARGE_SCRIPT_SNAPSHOT_MUST_NOT_APPEAR") {
		t.Fatalf("build_get_run leaked HookRun.scriptSnapshot: %s", string(encodedGetRun))
	}

	// 7. build_delete_run rejects active run, build_cancel_run cancels it, then build_delete_run deletes it
	delRunTool, _ := reg.Get("build_delete_run")
	if _, err := delRunTool.Preview(makeTC(owner, "prj_1", ""), json.RawMessage(`{"project":"demo","buildRunId":"`+runID+`"}`)); err == nil {
		t.Fatal("expected 409 build.not_deletable on queued build run")
	}
	cancelTool, _ := reg.Get("build_cancel_run")
	cancelRaw, err := cancelTool.Execute(makeTC(owner, "prj_1", ""), json.RawMessage(`{"project":"demo","buildRunId":"`+runID+`"}`))
	if err != nil || cancelRaw.(map[string]any)["status"] != "canceled" {
		t.Fatalf("build_cancel_run failed: %v %#v", err, cancelRaw)
	}
	// Idempotent second cancel succeeds
	if _, err := cancelTool.Execute(makeTC(owner, "prj_1", ""), json.RawMessage(`{"project":"demo","buildRunId":"`+runID+`"}`)); err != nil {
		t.Fatalf("idempotent build_cancel_run on already canceled run failed: %v", err)
	}

	delRunRaw, err := delRunTool.Execute(makeTC(owner, "prj_1", ""), json.RawMessage(`{"project":"demo","buildRunId":"`+runID+`"}`))
	if err != nil {
		t.Fatalf("build_delete_run failed: %v", err)
	}
	delRunMap := delRunRaw.(map[string]any)
	if delRunMap["deleted"] != true || delRunMap["projectId"] != proj.ID {
		t.Fatalf("unexpected delete run output: %#v", delRunMap)
	}

	// 8. build_delete_variable_set detaches from targetDev and deletes set
	delSetTool, _ := reg.Get("build_delete_variable_set")
	delSetRaw, err := delSetTool.Execute(makeTC(owner, "prj_1", ""), json.RawMessage(`{"variableSet":"NodeSecrets"}`))
	if err != nil || delSetRaw.(map[string]any)["deleted"] != true {
		t.Fatalf("build_delete_variable_set failed: %v %#v", err, delSetRaw)
	}
	var refreshedDev model.DeploymentTarget
	_ = db.First(&refreshedDev, "id = ?", targetDev.ID).Error
	if strings.Contains(refreshedDev.BuildVariableSetIDs, personalSet.ID) {
		t.Fatalf("expected variable set ID unbound from targetDev, got %q", refreshedDev.BuildVariableSetIDs)
	}
}
