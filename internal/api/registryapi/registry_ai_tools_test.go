package registryapi

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
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type registryAITestHost struct {
	Host
	db         *gorm.DB
	store      secret.Store
	canInspect bool
}

func (h *registryAITestHost) DBFor(*gin.Context) *gorm.DB { return h.db }
func (h *registryAITestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return h.db.WithContext(ctx)
}
func (h *registryAITestHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	v, ok := ctx.Get("luna.devops.current_user")
	if !ok {
		return model.User{}, false
	}
	u, ok := v.(model.User)
	return u, ok
}
func (h *registryAITestHost) ResolveListVisibility(_ *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	if authz.IsPlatformAdmin(user.Role) {
		return projectservice.ListVisibilityAll, true
	}
	return projectservice.ListVisibilityRelated, true
}
func (h *registryAITestHost) ApplyScopedResourceListVisibility(_ *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	if projectID != "" {
		return h.ApplyScopedResourceVisibilityForProject(query, resourceType, user, projectID, context.Background()), true
	}
	if visibility == projectservice.ListVisibilityAll {
		return query, true
	}
	return h.ApplyScopedResourceVisibilityForUser(query, resourceType, user, context.Background()), true
}
func (h *registryAITestHost) ApplyScopedResourceVisibilityForUser(query *gorm.DB, _ string, user model.User, _ context.Context) *gorm.DB {
	return query.Where("scope = 'global' OR (scope = 'user' AND owner_ref = ?) OR scope = 'project'", user.ID)
}
func (h *registryAITestHost) ApplyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, _ context.Context) *gorm.DB {
	sub := h.db.Model(&model.ScopedResourceProjectBinding{}).
		Select("resource_id").
		Where("resource_type = ? AND project_id = ?", resourceType, projectID)
	return query.Where("scope = 'global' OR (scope = 'user' AND owner_ref = ?) OR (scope = 'project' AND id IN (?))", user.ID, sub)
}
func (h *registryAITestHost) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, rawScope, rawOwnerRef string, rawProjectIDs []string, globalError string) (string, string, []string, bool) {
	switch strings.ToLower(strings.TrimSpace(rawScope)) {
	case "global":
		if !authz.IsPlatformAdmin(user.Role) {
			writeError(ctx, http.StatusForbidden, globalError)
			return "", "", nil, false
		}
		return "global", "", nil, true
	case "project":
		if len(rawProjectIDs) == 0 {
			writeError(ctx, http.StatusBadRequest, "projectIds required")
			return "", "", nil, false
		}
		return "project", "", sortedProjectIDs(rawProjectIDs), true
	default:
		if rawOwnerRef == "" {
			rawOwnerRef = user.ID
		}
		return "user", rawOwnerRef, nil, true
	}
}
func (h *registryAITestHost) NormalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, rawScope string, rawProjectIDs []string, _ string, _ []string, globalError string) (string, string, []string, bool) {
	return h.NormalizeScopedOwnerWithProjects(ctx, user, rawScope, user.ID, rawProjectIDs, globalError)
}
func (h *registryAITestHost) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, _, _, errMsg string) bool {
	if authz.IsPlatformAdmin(user.Role) || (scope == "user" && ownerRef == user.ID) || scope == "project" {
		return true
	}
	writeError(ctx, http.StatusForbidden, errMsg)
	return false
}
func (h *registryAITestHost) CanInspectScopedResourceConfigByID(_ model.User, _, _, _, _ string, _ context.Context) (bool, error) {
	return h.canInspect, nil
}
func (h *registryAITestHost) CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, _ context.Context) bool {
	if scope == "global" || (scope == "user" && ownerRef == user.ID) {
		return true
	}
	if scope == "project" {
		var cnt int64
		_ = h.db.Model(&model.ScopedResourceProjectBinding{}).Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).Count(&cnt).Error
		return cnt > 0
	}
	return false
}
func (h *registryAITestHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	if err := tx.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
		return err
	}
	defSet := map[string]bool{}
	for _, d := range defaultProjectIDs {
		defSet[d] = true
	}
	for _, pid := range projectIDs {
		row := model.ScopedResourceProjectBinding{
			ID:           resourceType + ":" + resourceID + ":" + pid,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			ProjectID:    pid,
			IsDefault:    defSet[pid],
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
func (h *registryAITestHost) ScopedResourceProjectIDs(resourceType, resourceID string, _ context.Context) []string {
	var ids []string
	_ = h.db.Model(&model.ScopedResourceProjectBinding{}).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("project_id asc").
		Pluck("project_id", &ids).Error
	return ids
}
func (h *registryAITestHost) ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, _ context.Context) map[string][]string {
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
func (h *registryAITestHost) ScopedResourceDefaultProjectIDMap(resourceType string, resourceIDs []string, _ context.Context) map[string][]string {
	out := map[string][]string{}
	if len(resourceIDs) == 0 {
		return out
	}
	var rows []model.ScopedResourceProjectBinding
	_ = h.db.Where("resource_type = ? AND resource_id IN ? AND is_default = ?", resourceType, resourceIDs, true).Find(&rows).Error
	for _, r := range rows {
		out[r.ResourceID] = append(out[r.ResourceID], r.ProjectID)
	}
	return out
}
func (h *registryAITestHost) AuthorizeProjectRef(ctx *gin.Context, projectRef string, _ authz.Action) (model.User, model.Project, bool) {
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
func (h *registryAITestHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return h.AuthorizeProjectRef(ctx, projectID, action)
}
func (h *registryAITestHost) FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	_, p, ok := h.AuthorizeProjectByID(ctx, projectID, authz.ActionProjectRead)
	return p, ok
}
func (h *registryAITestHost) ProjectIDsForUser(_ context.Context, _ string) []string {
	return []string{"prj_a"}
}
func (h *registryAITestHost) AuditWithContext(string, string, string, bool, string, context.Context) {
}
func (h *registryAITestHost) SecretAvailable() bool { return h.store.Available() }
func (h *registryAITestHost) StoreSecret(ctx context.Context, value, userID, resource string) string {
	return h.store.StoreContext(ctx, value, userID, resource)
}
func (h *registryAITestHost) StoreSecretWithDB(ctx context.Context, db *gorm.DB, value, userID, resource string) (string, error) {
	return h.store.StoreContextWithDB(ctx, db, value, userID, resource)
}
func (h *registryAITestHost) DeleteSecretRefWithDB(ctx context.Context, db *gorm.DB, ref, resource string) error {
	return h.store.DeleteRefContextWithDB(ctx, db, ref, resource)
}
func (h *registryAITestHost) ResolveSecret(ctx context.Context, ref string) string {
	return h.store.ResolveContext(ctx, ref)
}
func (h *registryAITestHost) EgressPolicyForUser(model.User, context.Context) security.EgressPolicy {
	return security.AdminEgressPolicy()
}
func (h *registryAITestHost) AllowRegistrySearch(*gin.Context, string) bool { return true }

func TestRegistryAIToolsCatalogContract(t *testing.T) {
	reg := aitool.NewRegistry()
	h := New(&registryAITestHost{})
	h.RegisterAITools(reg)

	if reg.Len() != 7 {
		t.Fatalf("expected 7 registered registry AI tools, got %d", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}

	expected := map[string]struct {
		tier           string
		effect         string
		sensitivePaths []string
	}{
		"build_list_registries":            {aitool.ApprovalTierL0, aitool.EffectRead, nil},
		"build_search_registry_images":     {aitool.ApprovalTierL0, aitool.EffectRead, nil},
		"build_save_registry":              {aitool.ApprovalTierL2, aitool.EffectWrite, nil},
		"build_delete_registry":            {aitool.ApprovalTierL2, aitool.EffectWrite, nil},
		"build_save_registry_credential":   {aitool.ApprovalTierL2, aitool.EffectWrite, []string{"password", "token"}},
		"build_delete_registry_credential": {aitool.ApprovalTierL2, aitool.EffectWrite, nil},
		"build_register_container_image":   {aitool.ApprovalTierL1, aitool.EffectWrite, nil},
	}

	for name, exp := range expected {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != aitool.DomainBuild || cat.ApprovalTier != exp.tier || cat.Effect != exp.effect || cat.AdminOnly {
			t.Fatalf("tool %s metadata mismatch: %+v", name, cat)
		}
		if exp.tier == aitool.ApprovalTierL2 && !tool.HasPreview {
			t.Fatalf("L2 tool %s must provide Preview", name)
		}
		if !slices.Equal(cat.SensitivePaths, exp.sensitivePaths) {
			t.Fatalf("tool %s sensitivePaths = %v, want %v", name, cat.SensitivePaths, exp.sensitivePaths)
		}
		for _, sc := range cat.RequiredScopes {
			if norm := authz.NormalizeOAuthScope(sc); norm != sc || !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleUser, sc) {
				t.Fatalf("tool %s has invalid OAuth scope %q", name, sc)
			}
		}
	}
}

func TestRegistryAIToolsLifecycleAndDefaultPreservation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "registry_ai_tools_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.Project{},
				&model.ProjectMember{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.ArtifactRegistry{},
				&model.RegistryCredential{},
				&model.ContainerImage{},
				&model.ScopedResourceProjectBinding{},
				&model.SecretValue{},
			)
		},
	})
	codec, err := secret.NewCodec("registry-ai-tools-test-secret-key")
	if err != nil {
		t.Fatal(err)
	}
	store := secret.NewStore(db, nil, codec)

	mockReg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v2/team/api/tags/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"team/api","tags":["v1.0.0","v1.2.0","v2.0.0-rc1"]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockReg.Close()

	host := &registryAITestHost{db: db, store: store, canInspect: false}
	h := New(host)
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	owner := model.User{ID: "usr_owner", Email: "owner@example.test", Role: authz.PlatformRoleUser}
	prjA := model.Project{ID: "prj_a", Identifier: "alpha", Name: "Alpha", DeleteStatus: "active"}
	prjB := model.Project{ID: "prj_b", Identifier: "beta", Name: "Beta", DeleteStatus: "active"}
	app := model.Application{ID: "app_a", ProjectID: prjA.ID, Identifier: "api", Name: "API"}
	for _, row := range []any{&owner, &prjA, &prjB, &app} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	makeTC := func(convProj string) *aitool.ToolContext {
		parentRec := httptest.NewRecorder()
		parentGin, _ := gin.CreateTestContext(parentRec)
		parentGin.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)
		return aitool.NewToolContext(parentGin, db, owner, "ses_1", "conv_1", convProj, "run_1", "call_1", "", "model", nil)
	}

	saveRegTool, _ := reg.Get("build_save_registry")
	// Reject embedded credentials in endpoint
	if _, err := saveRegTool.Preview(makeTC(""), json.RawMessage(`{"name":"Bad","endpoint":"https://user:pass@registry.example.com"}`)); err == nil {
		t.Fatal("expected embedded endpoint credentials to be rejected")
	}

	// Create project-scoped default registry on prj_a & prj_b
	regCreateRaw, err := saveRegTool.Execute(makeTC(""), json.RawMessage(`{
		"name":"SharedHarbor","provider":"generic-oci","endpoint":"`+mockReg.URL+`",
		"scope":"project","addProjects":["alpha","beta"],"isDefault":true
	}`))
	if err != nil {
		t.Fatalf("build_save_registry create failed: %v", err)
	}
	registryID := regCreateRaw.(map[string]any)["id"].(string)

	// Partial update omitting `isDefault` while removing `beta` MUST preserve `isDefault` on `alpha`!
	if _, err := saveRegTool.Execute(makeTC(""), json.RawMessage(`{
		"registry":"SharedHarbor","name":"AlphaHarbor","removeProjects":["beta"]
	}`)); err != nil {
		t.Fatalf("build_save_registry partial patch failed: %v", err)
	}
	defMap := host.ScopedResourceDefaultProjectIDMap(scopedResourceArtifactRegistry, []string{registryID}, t.Context())
	if !slices.Equal(defMap[registryID], []string{"prj_a"}) {
		t.Fatalf("expected DefaultProjectIDs=[prj_a] preserved after partial patch omitting isDefault, got %v", defMap[registryID])
	}

	// Create & patch credential preserving secret when omitted
	saveCredTool, _ := reg.Get("build_save_registry_credential")
	credCreateRaw, err := saveCredTool.Execute(makeTC("prj_a"), json.RawMessage(`{
		"registry":"AlphaHarbor","name":"push-bot","username":"bot","password":"secret-password","scope":"project","addProjects":["alpha"]
	}`))
	if err != nil {
		t.Fatalf("build_save_registry_credential create failed: %v", err)
	}
	credID := credCreateRaw.(map[string]any)["id"].(string)

	credUpdRaw, err := saveCredTool.Execute(makeTC("prj_a"), json.RawMessage(`{
		"registry":"AlphaHarbor","credential":"`+credID+`","tagTemplate":"{branch}-{short_sha}"
	}`))
	if err != nil {
		t.Fatalf("build_save_registry_credential partial patch failed: %v", err)
	}
	credUpd := credUpdRaw.(map[string]any)
	if credUpd["passwordSet"] != true || credUpd["scope"] != "project" {
		t.Fatalf("partial credential patch wiped password or scope: %#v", credUpd)
	}

	// build_list_registries with testRegistry redacts endpoint when canInspect == false
	listRegTool, _ := reg.Get("build_list_registries")
	listRaw, err := listRegTool.Execute(makeTC("prj_a"), json.RawMessage(`{
		"project":"alpha","application":"api","testRegistry":"AlphaHarbor"
	}`))
	if err != nil {
		t.Fatalf("build_list_registries failed: %v", err)
	}
	listMap := listRaw.(map[string]any)
	if listMap["projectId"] != "prj_a" || listMap["defaultRegistryId"] != registryID {
		t.Fatalf("expected projectId and defaultRegistryId in list output: %#v", listMap)
	}
	testRes := listMap["testResult"].(map[string]any)
	if testRes["success"] != true || testRes["endpoint"] != "" {
		t.Fatalf("expected successful ping with redacted endpoint, got %#v", testRes)
	}

	// build_register_container_image + build_search_registry_images (recorded_images & remote_tags substring filter)
	regImgTool, _ := reg.Get("build_register_container_image")
	imgOutRaw, err := regImgTool.Execute(makeTC("prj_a"), json.RawMessage(`{
		"project":"alpha","application":"api","registry":"AlphaHarbor","repository":"team/api","tag":"v1.2.0"
	}`))
	if err != nil {
		t.Fatalf("build_register_container_image failed: %v", err)
	}
	imgOut := imgOutRaw.(model.ContainerImage)
	if imgOut.ProjectID != "prj_a" || imgOut.ApplicationID != "app_a" {
		t.Fatalf("unexpected registered image: %+v", imgOut)
	}

	searchImgTool, _ := reg.Get("build_search_registry_images")
	recSearchRaw, err := searchImgTool.Execute(makeTC("prj_a"), json.RawMessage(`{
		"project":"alpha","imageId":"`+imgOut.ID+`","repository":"/team/api/"
	}`))
	if err != nil || recSearchRaw.(map[string]any)["total"] != int64(1) {
		t.Fatalf("recorded_images search failed: %v %#v", err, recSearchRaw)
	}

	remoteTagsRaw, err := searchImgTool.Execute(makeTC("prj_a"), json.RawMessage(`{
		"source":"remote_tags","registry":"AlphaHarbor","repository":"team/api","search":"v1."
	}`))
	if err != nil || remoteTagsRaw.(map[string]any)["total"] != 2 {
		t.Fatalf("remote_tags substring search failed: %v %#v", err, remoteTagsRaw)
	}

	// Referenced registry blocks deletion with 409 registry.in_use
	target := model.DeploymentTarget{
		ID: "dplt_reg", ProjectID: prjA.ID, ApplicationID: app.ID, Name: "Dev", Stage: "dev",
		KubernetesName: "api-dev", TargetRegistryID: registryID, SourceType: "repository", Enabled: true, DeleteStatus: "active",
	}
	_ = db.Create(&target).Error
	delCredTool, _ := reg.Get("build_delete_registry_credential")
	if _, err := delCredTool.Preview(makeTC("prj_a"), json.RawMessage(`{"registry":"AlphaHarbor","credential":"`+credID+`"}`)); err == nil {
		t.Fatal("expected 409 registry.credential_in_use when deleting sole push credential for active deployment target")
	}
	delRegTool, _ := reg.Get("build_delete_registry")
	if _, err := delRegTool.Preview(makeTC("prj_a"), json.RawMessage(`{"registry":"AlphaHarbor"}`)); err == nil {
		t.Fatal("expected 409 registry.in_use when deleting referenced registry")
	}

	_ = db.Delete(&target).Error
	delRegOut, err := delRegTool.Execute(makeTC("prj_a"), json.RawMessage(`{"registry":"AlphaHarbor"}`))
	if err != nil || delRegOut.(map[string]any)["deleted"] != true {
		t.Fatalf("build_delete_registry failed: %v %#v", err, delRegOut)
	}
}
