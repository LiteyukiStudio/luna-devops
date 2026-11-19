package gitapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

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

type gitAITestHost struct {
	Host
	db            *gorm.DB
	store         secret.Store
	publicBaseURL string
	audits        []string
}

func (h *gitAITestHost) DBFor(*gin.Context) *gorm.DB { return h.db }
func (h *gitAITestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return h.db.WithContext(ctx)
}
func (h *gitAITestHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	v, ok := ctx.Get("luna.devops.current_user")
	if !ok {
		return model.User{}, false
	}
	u, ok := v.(model.User)
	return u, ok
}
func (h *gitAITestHost) RequirePlatformAdmin(ctx *gin.Context) bool {
	u, ok := h.CurrentUser(ctx)
	if !ok || !authz.IsPlatformAdmin(u.Role) {
		writeErrorCode(ctx, http.StatusForbidden, "auth.platform_admin_required", "admin required")
		return false
	}
	return true
}
func (h *gitAITestHost) ResolveListVisibility(_ *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	if authz.IsPlatformAdmin(user.Role) {
		return projectservice.ListVisibilityAll, true
	}
	return projectservice.ListVisibilityRelated, true
}
func (h *gitAITestHost) ApplyScopedResourceListVisibility(_ *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	if projectID != "" {
		return h.ApplyScopedResourceVisibilityForProject(query, resourceType, user, projectID, context.Background()), true
	}
	if visibility == projectservice.ListVisibilityAll && authz.IsPlatformAdmin(user.Role) {
		return query, true
	}
	return query.Where("scope = 'global' OR (scope = 'user' AND owner_ref = ?)", user.ID), true
}
func (h *gitAITestHost) ApplyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, _ context.Context) *gorm.DB {
	sub := h.db.Model(&model.ScopedResourceProjectBinding{}).
		Select("resource_id").
		Where("resource_type = ? AND project_id = ?", resourceType, projectID)
	return query.Where("scope = 'global' OR (scope = 'user' AND owner_ref = ?) OR (scope = 'project' AND id IN (?))", user.ID, sub)
}
func (h *gitAITestHost) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool) {
	switch scope {
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
func (h *gitAITestHost) NormalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, scope string, projectIDs []string, _ string, _ []string, globalError string) (string, string, []string, bool) {
	return h.NormalizeScopedOwnerWithProjects(ctx, user, scope, user.ID, projectIDs, globalError)
}
func (h *gitAITestHost) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, _, _, errMsg string) bool {
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	if scope == "user" && ownerRef == user.ID {
		return true
	}
	if scope == "project" {
		return true
	}
	writeError(ctx, http.StatusForbidden, errMsg)
	return false
}
func (h *gitAITestHost) CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, _, _ string, _ context.Context) (bool, error) {
	if authz.IsPlatformAdmin(user.Role) || (scope == "user" && ownerRef == user.ID) {
		return true, nil
	}
	return false, nil
}
func (h *gitAITestHost) CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, _ context.Context) bool {
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
func (h *gitAITestHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
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
func (h *gitAITestHost) ScopedResourceProjectIDs(resourceType, resourceID string, _ context.Context) []string {
	var ids []string
	_ = h.db.Model(&model.ScopedResourceProjectBinding{}).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("project_id asc").
		Pluck("project_id", &ids).Error
	return ids
}
func (h *gitAITestHost) ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, _ context.Context) map[string][]string {
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
func (h *gitAITestHost) AuthorizeProjectRef(ctx *gin.Context, projectRef string, _ authz.Action) (model.User, model.Project, bool) {
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
func (h *gitAITestHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return h.AuthorizeProjectRef(ctx, projectID, action)
}
func (h *gitAITestHost) FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	_, proj, ok := h.AuthorizeProjectByID(ctx, projectID, authz.ActionProjectRead)
	return proj, ok
}
func (h *gitAITestHost) AuditWithContext(_, action, resource string, _ bool, _ string, _ context.Context) {
	h.audits = append(h.audits, action+":"+resource)
}
func (h *gitAITestHost) SecretStore() secret.Store        { return h.store }
func (h *gitAITestHost) OAuthStateStore() OAuthStateStore { return nil }
func (h *gitAITestHost) PublicBaseURL() string            { return h.publicBaseURL }
func (h *gitAITestHost) EgressPolicyForUser(model.User, context.Context) security.EgressPolicy {
	return security.AdminEgressPolicy()
}
func (h *gitAITestHost) EgressContextForUser(ctx context.Context, _ model.User, _ time.Duration) context.Context {
	return ctx
}

func TestGitAIToolsCatalogContract(t *testing.T) {
	reg := aitool.NewRegistry()
	h := New(&gitAITestHost{})
	h.RegisterAITools(reg)

	if reg.Len() != 12 {
		t.Fatalf("expected 12 registered git AI tools, got %d", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}

	expected := map[string]struct {
		domain        string
		tier          string
		effect        string
		adminOnly     bool
		sensitivePath []string
	}{
		"build_list_git_sources":          {aitool.DomainBuild, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"build_search_git_repositories":   {aitool.DomainBuild, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"build_inspect_repository_code":   {aitool.DomainBuild, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"build_list_repository_bindings":  {aitool.DomainBuild, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"build_save_git_account":          {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, []string{"accessToken"}},
		"build_delete_git_account":        {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"build_bind_repository":           {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"build_update_repository_binding": {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"build_unbind_repository":         {aitool.DomainBuild, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"admin_list_git_providers":        {aitool.DomainAdmin, aitool.ApprovalTierL0, aitool.EffectRead, true, nil},
		"admin_save_git_provider":         {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectWrite, true, []string{"clientSecret"}},
		"admin_delete_git_provider":       {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectWrite, true, nil},
	}

	for name, exp := range expected {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != exp.domain || cat.ApprovalTier != exp.tier || cat.Effect != exp.effect || cat.AdminOnly != exp.adminOnly {
			t.Fatalf("tool %s metadata mismatch: got domain=%s tier=%s effect=%s adminOnly=%v",
				name, cat.Domain, cat.ApprovalTier, cat.Effect, cat.AdminOnly)
		}
		if exp.adminOnly && !cat.GlobalSessionOnly {
			t.Fatalf("admin tool %s must set GlobalSessionOnly=true", name)
		}
		if exp.tier == aitool.ApprovalTierL2 && !tool.HasPreview {
			t.Fatalf("L2 tool %s must provide Preview handler", name)
		}
		if !slices.Equal(cat.SensitivePaths, exp.sensitivePath) {
			t.Fatalf("tool %s sensitivePaths = %v, want %v", name, cat.SensitivePaths, exp.sensitivePath)
		}
		if cat.InputSchema["additionalProperties"] != false {
			t.Fatalf("tool %s inputSchema must enforce additionalProperties=false", name)
		}
		roleForCheck := authz.PlatformRoleUser
		if cat.AdminOnly {
			roleForCheck = authz.PlatformRoleAdmin
		}
		for _, sc := range cat.RequiredScopes {
			if norm := authz.NormalizeOAuthScope(sc); norm != sc || !authz.UserCanAuthorizeOAuthScope(roleForCheck, sc) {
				t.Fatalf("tool %s has invalid or non-authorizable OAuth scope %q for role %s", name, sc, roleForCheck)
			}
		}
	}
}

func TestGitAIToolsEndToEndLifecycleAndSecurityGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "git_ai_tools_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.Project{},
				&model.ProjectMember{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.GitProvider{},
				&model.GitAccount{},
				&model.RepositoryBinding{},
				&model.ScopedResourceProjectBinding{},
				&model.SecretValue{},
			)
		},
	})
	codec, err := secret.NewCodec("git-ai-tools-test-encryption-key")
	if err != nil {
		t.Fatal(err)
	}
	store := secret.NewStore(db, nil, codec)

	readmeContent := base64.StdEncoding.EncodeToString([]byte("hello 世界! Dockerfile content line"))
	binaryContent := base64.StdEncoding.EncodeToString([]byte{0x00, 0xff, 0xfe, 0x01})
	mockGit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/api/v1/user"):
			_, _ = w.Write([]byte(`{"id":1,"username":"alice"}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/hooks"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":99,"active":true,"config":{"url":"https://luna.example.test/api/v1/git/webhooks/x"}}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/hooks/"):
			_, _ = w.Write([]byte(`{"id":99,"active":true,"config":{"url":"https://luna.example.test/api/v1/git/webhooks/x"}}`))
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/hooks/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/branches"):
			_, _ = w.Write([]byte(`[{"name":"main","commit":{"sha":"abcdef1"}}]`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/contents/README.md"):
			_, _ = w.Write([]byte(`{"name":"README.md","path":"README.md","sha":"111","type":"file","encoding":"base64","content":"` + readmeContent + `"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/contents/bin.dat"):
			_, _ = w.Write([]byte(`{"name":"bin.dat","path":"bin.dat","sha":"222","type":"file","encoding":"base64","content":"` + binaryContent + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockGit.Close()

	host := &gitAITestHost{db: db, store: store, publicBaseURL: "https://luna.example.test"}
	h := New(host)
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	admin := model.User{ID: "usr_admin", Email: "admin@example.test", Name: "Admin", Role: authz.PlatformRoleAdmin}
	owner := model.User{ID: "usr_owner", Email: "owner@example.test", Name: "Owner", Role: authz.PlatformRoleUser}
	viewer := model.User{ID: "usr_viewer", Email: "viewer@example.test", Name: "Viewer", Role: authz.PlatformRoleUser}
	proj := model.Project{ID: "prj_1", Identifier: "demo", Name: "Demo", DeleteStatus: "active"}
	app := model.Application{ID: "app_1", ProjectID: proj.ID, Identifier: "web", Name: "Web"}
	for _, row := range []any{&admin, &owner, &viewer, &proj, &app} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	makeTC := func(u model.User, convProj string) *aitool.ToolContext {
		parentRec := httptest.NewRecorder()
		parentGin, _ := gin.CreateTestContext(parentRec)
		parentGin.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)
		return aitool.NewToolContext(parentGin, db, u, "ses_1", "conv_1", convProj, "run_1", "call_1", "", "model", nil)
	}

	// 1. admin_save_git_provider creates Gitea provider & preserves ClientSecretRef on partial patch
	saveProvTool, _ := reg.Get("admin_save_git_provider")
	provOutRaw, err := saveProvTool.Execute(makeTC(admin, ""), json.RawMessage(`{
		"type":"gitea","name":"CorpGitea","baseUrl":"`+mockGit.URL+`","scope":"global","clientId":"cid_1","clientSecret":"sec_1"
	}`))
	if err != nil {
		t.Fatalf("admin_save_git_provider create failed: %v", err)
	}
	provOut := provOutRaw.(map[string]any)
	providerID := provOut["id"].(string)
	if provOut["clientSecretSet"] != true {
		t.Fatalf("expected clientSecretSet=true, got %#v", provOut)
	}

	// Partial update omitting clientSecret & clientId must preserve both!
	provUpdRaw, err := saveProvTool.Execute(makeTC(admin, ""), json.RawMessage(`{"provider":"CorpGitea","enabled":true}`))
	if err != nil {
		t.Fatalf("admin_save_git_provider patch failed: %v", err)
	}
	provUpd := provUpdRaw.(map[string]any)
	if provUpd["clientId"] != "cid_1" || provUpd["clientSecretSet"] != true {
		t.Fatalf("partial patch wiped clientId or clientSecretRef: %#v", provUpd)
	}

	// 2. build_save_git_account creates account, then manual PAT replacement clears ExpiresAt & deletes old RefreshTokenRef
	saveAccTool, _ := reg.Get("build_save_git_account")
	accOutRaw, err := saveAccTool.Execute(makeTC(owner, ""), json.RawMessage(`{
		"provider":"CorpGitea","username":"alice","accessToken":"pat_initial"
	}`))
	if err != nil {
		t.Fatalf("build_save_git_account create failed: %v", err)
	}
	accountID := accOutRaw.(map[string]any)["id"].(string)

	// Simulate an expired OAuth account with refresh token ref in DB
	exp := time.Now().Add(-time.Hour)
	oldRefreshRef, err := store.StoreContextWithDB(t.Context(), db, "refresh_old", owner.ID, "git_account:"+accountID+":refresh")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.GitAccount{}).Where("id = ?", accountID).Updates(map[string]any{
		"expires_at":        &exp,
		"refresh_token_ref": oldRefreshRef,
		"external_user_id":  "ext_42",
		"scopes":            "repo,write:repo_hook",
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Replace with static PAT via build_save_git_account
	if _, err := saveAccTool.Execute(makeTC(owner, ""), json.RawMessage(`{"gitAccount":"`+accountID+`","accessToken":"pat_rotated"}`)); err != nil {
		t.Fatalf("build_save_git_account PAT rotation failed: %v", err)
	}
	var reloadedAcc model.GitAccount
	if err := db.First(&reloadedAcc, "id = ?", accountID).Error; err != nil {
		t.Fatal(err)
	}
	if reloadedAcc.ExpiresAt != nil || reloadedAcc.RefreshTokenRef != "" {
		t.Fatalf("expected ExpiresAt=nil and RefreshTokenRef=\"\" after static PAT replacement, got %+v", reloadedAcc)
	}
	if reloadedAcc.ExternalUserID != "ext_42" || reloadedAcc.Scopes != "repo,write:repo_hook" {
		t.Fatalf("expected ExternalUserID and Scopes preserved, got %+v", reloadedAcc)
	}
	var secretCount int64
	_ = db.Model(&model.SecretValue{}).Where("resource LIKE ?", "git_account:"+accountID+":%").Count(&secretCount).Error
	if secretCount != 1 {
		t.Fatalf("expected exactly 1 secret_values row remaining after atomic cleanup, got %d", secretCount)
	}

	// Deleting GitProvider while GitAccount exists must fail with 409 git.provider_in_use
	delProvTool, _ := reg.Get("admin_delete_git_provider")
	if _, err := delProvTool.Preview(makeTC(admin, ""), json.RawMessage(`{"provider":"`+providerID+`"}`)); err == nil {
		t.Fatal("expected 409 git.provider_in_use when deleting provider with existing GitAccount")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Status != http.StatusConflict || te.Code != "git.provider_in_use" {
		t.Fatalf("unexpected error deleting referenced provider: %#v", err)
	}

	// 3. build_bind_repository blocks cross-origin cloneUrl, succeeds on same-origin
	bindTool, _ := reg.Get("build_bind_repository")
	if _, err := bindTool.Preview(makeTC(owner, proj.ID), json.RawMessage(`{
		"project":"demo","application":"web","gitAccount":"alice","owner":"team","repo":"svc",
		"cloneUrl":"https://evil.attacker.example/team/svc.git","webhookEnabled":false
	}`)); err == nil {
		t.Fatal("expected cross-origin cloneUrl to be rejected")
	}

	bindResRaw, err := bindTool.Execute(makeTC(owner, proj.ID), json.RawMessage(`{
		"project":"demo","application":"web","gitAccount":"alice","owner":"team","repo":"svc","webhookEnabled":true
	}`))
	if err != nil {
		t.Fatalf("build_bind_repository failed: %v", err)
	}
	bindRes := bindResRaw.(map[string]any)
	bindingID := bindRes["id"].(string)
	if bindRes["projectId"] != proj.ID || bindRes["webhookId"] != "99" {
		t.Fatalf("unexpected bind result: %#v", bindRes)
	}

	// 4. build_inspect_repository_code blocks viewer from borrowing owner's personal scope=user credential!
	inspectTool, _ := reg.Get("build_inspect_repository_code")
	if _, err := inspectTool.Execute(makeTC(viewer, proj.ID), json.RawMessage(`{
		"project":"demo","repositoryBinding":"`+bindingID+`","path":"README.md"
	}`)); err == nil {
		t.Fatal("expected viewer to be forbidden from reading code via owner's personal GitAccount")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Status != http.StatusForbidden {
		t.Fatalf("expected 403 forbidden for cross-user personal credential, got %#v", err)
	}

	// Owner can inspect README.md with UTF-8 boundary truncation, and binary file is rejected
	inspectRaw, err := inspectTool.Execute(makeTC(owner, proj.ID), json.RawMessage(`{
		"project":"demo","repositoryBinding":"team/svc","path":"README.md","maxFileBytes":256
	}`))
	if err != nil {
		t.Fatalf("owner inspect README.md failed: %v", err)
	}
	inspectMap := inspectRaw.(map[string]any)
	if inspectMap["projectId"] != proj.ID || inspectMap["pathType"] != "file" {
		t.Fatalf("unexpected inspect output: %#v", inspectMap)
	}
	if _, err := inspectTool.Execute(makeTC(owner, proj.ID), json.RawMessage(`{
		"project":"demo","repositoryBinding":"team/svc","path":"bin.dat"
	}`)); err == nil {
		t.Fatal("expected binary file inspection to be blocked")
	}

	// 5. build_update_repository_binding recomputes cloneUrl on repo rename & disables webhook
	updBindTool, _ := reg.Get("build_update_repository_binding")
	updBindRaw, err := updBindTool.Execute(makeTC(owner, proj.ID), json.RawMessage(`{
		"project":"demo","binding":"`+bindingID+`","repo":"svc-v2","webhookEnabled":false
	}`))
	if err != nil {
		t.Fatalf("build_update_repository_binding failed: %v", err)
	}
	updBind := updBindRaw.(map[string]any)
	if !strings.HasSuffix(updBind["cloneUrl"].(string), "/team/svc-v2.git") || updBind["webhookEnabled"] != false {
		t.Fatalf("expected recomputed cloneUrl and webhookEnabled=false, got %#v", updBind)
	}

	// Disabled webhook must be ignored by enqueueBuildRunsForWebhook
	var persistedBinding model.RepositoryBinding
	_ = db.First(&persistedBinding, "id = ?", bindingID).Error
	whCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	whCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/git/webhooks/"+bindingID, nil)
	whRes := h.enqueueBuildRunsForWebhook(whCtx, persistedBinding, gitWebhookPushPayload{SourceBranch: "main", CommitSHA: "abc1234"}, true, "deliv-12345678")
	if whRes.IgnoredReason != "webhook_disabled" {
		t.Fatalf("expected ignoredReason=webhook_disabled, got %+v", whRes)
	}

	// 6. build_unbind_repository returns 409 when referenced by DeploymentTarget, then succeeds
	target := model.DeploymentTarget{
		ID: "dplt_1", ProjectID: proj.ID, ApplicationID: app.ID, Name: "Dev", Stage: "dev",
		KubernetesName: "web-dev", RepositoryBindingID: bindingID, Enabled: true, DeleteStatus: "active",
	}
	_ = db.Create(&target).Error
	unbindTool, _ := reg.Get("build_unbind_repository")
	if _, err := unbindTool.Preview(makeTC(owner, proj.ID), json.RawMessage(`{"project":"demo","binding":"`+bindingID+`"}`)); err == nil {
		t.Fatal("expected 409 when unbinding referenced repository")
	}
	_ = db.Delete(&target).Error
	unbindRaw, err := unbindTool.Execute(makeTC(owner, proj.ID), json.RawMessage(`{"project":"demo","binding":"`+bindingID+`"}`))
	if err != nil {
		t.Fatalf("build_unbind_repository failed: %v", err)
	}
	unbindMap := unbindRaw.(map[string]any)
	if unbindMap["unbound"] != true || unbindMap["deleted"] != true || unbindMap["projectId"] != proj.ID {
		t.Fatalf("unexpected unbind response: %#v", unbindMap)
	}

	// 7. build_delete_git_account & admin_delete_git_provider succeed once unreferenced
	delAccTool, _ := reg.Get("build_delete_git_account")
	delAccRaw, err := delAccTool.Execute(makeTC(owner, ""), json.RawMessage(`{"gitAccount":"alice"}`))
	if err != nil || delAccRaw.(map[string]any)["deleted"] != true {
		t.Fatalf("build_delete_git_account failed: %v %#v", err, delAccRaw)
	}
	delProvRaw, err := delProvTool.Execute(makeTC(admin, ""), json.RawMessage(`{"provider":"`+providerID+`"}`))
	if err != nil || delProvRaw.(map[string]any)["deleted"] != true {
		t.Fatalf("admin_delete_git_provider failed: %v %#v", err, delProvRaw)
	}
}
