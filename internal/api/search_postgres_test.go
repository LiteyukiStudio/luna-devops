package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/platformapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestSearchGlobalRBACScopesAndRoutingPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_ADDR", "")
	db := aiProjectScopeIntegrationDB(t)

	now := time.Now().UTC()

	admin := model.User{ID: "usr_search_admin", Email: "ops-admin@example.test", Name: "Platform Ops Admin", Role: authz.PlatformRoleAdmin}
	dev := model.User{ID: "usr_search_dev", Email: "alice-dev@example.test", Name: "Alice Developer", Role: authz.PlatformRoleUser}
	viewer := model.User{ID: "usr_search_viewer", Email: "bob-viewer@example.test", Name: "Bob Viewer", Role: authz.PlatformRoleUser}
	outsider := model.User{ID: "usr_search_outsider", Email: "mallory@example.test", Name: "Mallory Outsider", Role: authz.PlatformRoleUser}

	adminToken := "search-admin-session-plain"
	devToken := "search-dev-session-plain"
	viewerToken := "search-viewer-session-plain"
	outsiderToken := "search-outsider-session-plain"
	restrictedPATPlain := "luna_pat_search_project_only_secret"

	sessions := []model.UserSession{
		{ID: "ses_search_admin", UserID: admin.ID, TokenHash: transportapi.HashToken(adminToken), ExpiresAt: now.Add(time.Hour)},
		{ID: "ses_search_dev", UserID: dev.ID, TokenHash: transportapi.HashToken(devToken), ExpiresAt: now.Add(time.Hour)},
		{ID: "ses_search_viewer", UserID: viewer.ID, TokenHash: transportapi.HashToken(viewerToken), ExpiresAt: now.Add(time.Hour)},
		{ID: "ses_search_outsider", UserID: outsider.ID, TokenHash: transportapi.HashToken(outsiderToken), ExpiresAt: now.Add(time.Hour)},
	}
	pat := model.AccessToken{
		ID:        "tok_search_proj_only",
		UserID:    dev.ID,
		Name:      "Project Read Only PAT",
		TokenHash: transportapi.HashToken(restrictedPATPlain),
		Scope:     string(authz.ActionProjectRead),
		Source:    model.AccessTokenSourcePersonal,
	}

	projAlpha := model.Project{
		ID: "prj_search_alpha", Identifier: "alpha-space", Name: "Alpha Commerce",
		KubernetesNamespace: "luna-alpha-commerce", DeleteStatus: "active",
	}
	projSecret := model.Project{
		ID: "prj_search_secret", Identifier: "secret-vault", Name: "Secret Vault Space",
		KubernetesNamespace: "luna-secret-vault", DeleteStatus: "active",
	}

	members := []model.ProjectMember{
		{ID: "mem_search_dev_alpha", ProjectID: projAlpha.ID, UserID: dev.ID, Role: authz.ProjectRoleDeveloper},
		{ID: "mem_search_viewer_alpha", ProjectID: projAlpha.ID, UserID: viewer.ID, Role: authz.ProjectRoleViewer},
		{ID: "mem_search_admin_alpha", ProjectID: projAlpha.ID, UserID: admin.ID, Role: authz.ProjectRoleOwner},
		{ID: "mem_search_outsider_secret", ProjectID: projSecret.ID, UserID: outsider.ID, Role: authz.ProjectRoleOwner},
	}

	appAlpha := model.Application{
		ID: "app_search_checkout", ProjectID: projAlpha.ID, Name: "Alpha Checkout Service",
		Identifier: "alpha-checkout", DeleteStatus: "active",
	}
	appSecret := model.Application{
		ID: "app_search_secret", ProjectID: projSecret.ID, Name: "Alpha Shadow Internal",
		Identifier: "alpha-shadow", DeleteStatus: "active",
	}

	targetAlpha := model.DeploymentTarget{
		ID: "dplt_search_checkout_prod", ProjectID: projAlpha.ID, ApplicationID: appAlpha.ID,
		Name: "Production Deployment", Stage: "production", KubernetesName: "checkout-prod-deploy",
		WorkloadType: "Deployment", Replicas: 2, CPURequest: "500m", MemoryRequest: "512Mi",
		SourceType: "image", ImageRef: "ghcr.io/liteyuki/checkout-api:2.4.1",
		BuildConcurrencyPolicy: model.BuildConcurrencyQueue, DeleteStatus: "active",
	}
	routeAlpha := model.GatewayRoute{
		ID: "gwr_search_checkout", ProjectID: projAlpha.ID, ApplicationID: appAlpha.ID,
		DeploymentTargetID: targetAlpha.ID, Host: "pay.alpha.example.com", Path: "/v1",
		TLSMode: "auto-https", Enabled: true, DeleteStatus: "active",
	}
	gitProvider := model.GitProvider{
		ID: "gitp_search_gh", Name: "GitHub", Type: "github", BaseURL: "https://github.com",
		Scope: "global",
	}
	gitAccount := model.GitAccount{
		ID: "gita_search_gh", UserID: admin.ID, ProviderID: gitProvider.ID, Username: "liteyuki-bot",
		Scope: "global",
	}
	repoBinding := model.RepositoryBinding{
		ID: "rpb_search_checkout", ProjectID: projAlpha.ID, ApplicationID: appAlpha.ID,
		GitProviderID: gitProvider.ID, GitAccountID: gitAccount.ID,
		Owner: "LiteyukiStudio", Repo: "alpha-checkout", CloneURL: "https://github.com/LiteyukiStudio/alpha-checkout.git",
		DefaultBranch: "main",
	}

	clusterGlobal := model.RuntimeCluster{
		ID: "clu_search_global", Name: "Tokyo Edge Cluster", Endpoint: "https://k8s-tokyo.example.com:6443",
		Scope: "global", DeleteStatus: "active", CreatedBy: admin.ID,
	}
	volumeAlpha := model.ProjectVolume{
		ID: "pvol_search_pgdata", ProjectID: projAlpha.ID, DisplayName: "Alpha Invoices PVC",
		ClusterID: clusterGlobal.ID, Namespace: projAlpha.KubernetesNamespace, ClaimName: "pvc-alpha-invoices",
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		LifecycleState: model.ProjectVolumeLifecycleReady, CapacityRequest: "20Gi", CapacityBytes: 20 << 30,
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		CreatedBy: dev.ID,
	}
	runtimeSet := model.ProjectRuntimeConfigSet{
		ID: "prcs_search_redis", ProjectID: projAlpha.ID, Name: "Alpha Redis Config",
		EnvVars: `{"SUPER_INTERNAL_ENV_VALUE":"never_match_value_xyz"}`, Enabled: true, DeleteStatus: "active", CreatedBy: dev.ID,
	}
	buildVarSet := model.BuildVariableSet{
		ID: "bvs_search_maven", Name: "Alpha Maven Mirror", Scope: "project",
		Variables: `{"MAVEN_TOKEN_VALUE":"never_match_value_xyz"}`, Enabled: true, CreatedBy: dev.ID,
	}
	projNotifChannel := model.NotificationChannel{
		ID: "nch_search_proj_feishu", Scope: "project", OwnerUserID: "",
		PresetID: "feishu", Name: "Alpha OnCall Feishu", AdapterKind: "webhook", Enabled: true, CreatedBy: dev.ID,
	}
	personalNotifChannel := model.NotificationChannel{
		ID: "nch_search_personal", Scope: "user", OwnerUserID: outsider.ID,
		PresetID: "feishu", Name: "Alpha Personal Secret Webhook", AdapterKind: "webhook", Enabled: true, CreatedBy: outsider.ID,
	}
	registryGlobal := model.ArtifactRegistry{
		ID: "reg_search_harbor", Name: "Alpha Harbor Registry", Provider: "harbor",
		Endpoint: "https://harbor.example.com", Namespace: "alpha-images", Scope: "global", CreatedBy: admin.ID,
	}
	buildRun := model.BuildRun{
		ID: "bldr_search_commit01", ProjectID: projAlpha.ID, ApplicationID: appAlpha.ID,
		DeploymentTargetID: targetAlpha.ID, Status: "succeeded", TriggerType: "manual",
		SourceBranch: "main", SourceCommit: "7f4e9a1bc0ffee1234567890abcdef1234567890",
		ImageRef: "ghcr.io/liteyuki/checkout-api:7f4e9a1b", CreatedBy: dev.ID,
	}
	release := model.Release{
		ID: "rel_search_prod_001", ProjectID: projAlpha.ID, ApplicationID: appAlpha.ID,
		DeploymentTargetID: targetAlpha.ID, BuildRunID: buildRun.ID,
		ImageRef: "ghcr.io/liteyuki/checkout-api:2.4.1", Type: "deploy", Status: "succeeded", CreatedBy: dev.ID,
	}
	wildcardApp := model.Application{
		ID: "app_search_literal_pct", ProjectID: projAlpha.ID, Name: "100% Canary App",
		Identifier: "canary-pct", DeleteStatus: "active",
	}

	bindings := []model.ScopedResourceProjectBinding{
		{ID: "srpb_search_bvs", ResourceType: "build_variable_set", ResourceID: buildVarSet.ID, ProjectID: projAlpha.ID, IsDefault: true},
		{ID: "srpb_search_nch", ResourceType: "notification_channel", ResourceID: projNotifChannel.ID, ProjectID: projAlpha.ID},
	}

	seedItems := []any{
		&admin, &dev, &viewer, &outsider,
		&sessions, &pat,
		&projAlpha, &projSecret, &members,
		&appAlpha, &appSecret, &wildcardApp,
		&targetAlpha, &routeAlpha,
		&gitProvider, &gitAccount, &repoBinding,
		&clusterGlobal, &volumeAlpha,
		&runtimeSet, &buildVarSet, &projNotifChannel, &personalNotifChannel,
		&registryGlobal, &buildRun, &release, &bindings,
	}
	for _, item := range seedItems {
		if err := db.Create(item).Error; err != nil {
			t.Fatalf("seed %T: %v", item, err)
		}
	}

	router := NewRouter(db, mustTestConfig(t))

	doSearch := func(t *testing.T, sessionToken, bearerToken string, params url.Values) (int, platformapi.SearchResponse) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search?"+params.Encode(), nil)
		if sessionToken != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
		}
		if bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+bearerToken)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var body platformapi.SearchResponse
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode search response: %v; raw=%s", err, rec.Body.String())
			}
		}
		return rec.Code, body
	}

	hasItem := func(resp platformapi.SearchResponse, itemType, id string) (platformapi.SearchResultItem, bool) {
		for _, item := range resp.Items {
			if item.Type == itemType && item.ID == id {
				return item, true
			}
		}
		return platformapi.SearchResultItem{}, false
	}

	t.Run("developer searches across visible resources with strict tenant isolation", func(t *testing.T) {
		status, resp := doSearch(t, devToken, "", url.Values{"q": {"alpha"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		for _, want := range []struct {
			itemType string
			id       string
		}{
			{platformapi.SearchTypeProject, projAlpha.ID},
			{platformapi.SearchTypeApplication, appAlpha.ID},
			{platformapi.SearchTypeRepositoryBinding, repoBinding.ID},
			{platformapi.SearchTypeProjectVolume, volumeAlpha.ID},
			{platformapi.SearchTypeRuntimeConfigSet, runtimeSet.ID},
			{platformapi.SearchTypeBuildVariableSet, buildVarSet.ID},
			{platformapi.SearchTypeNotificationChannel, projNotifChannel.ID},
			{platformapi.SearchTypeArtifactRegistry, registryGlobal.ID},
		} {
			if _, ok := hasItem(resp, want.itemType, want.id); !ok {
				t.Errorf("missing expected search hit %s:%s in %#v", want.itemType, want.id, resp.Items)
			}
		}

		if _, leaked := hasItem(resp, platformapi.SearchTypeApplication, appSecret.ID); leaked {
			t.Fatal("leaked non-member tenant application")
		}
		if _, leaked := hasItem(resp, platformapi.SearchTypeNotificationChannel, personalNotifChannel.ID); leaked {
			t.Fatal("leaked personal notification channel")
		}
		for _, item := range resp.Items {
			if item.Type == platformapi.SearchTypeUser {
				t.Fatalf("non-admin received user search hit: %#v", item)
			}
		}
	})

	t.Run("pasted live URL and image tag resolve gateway route and deployment target", func(t *testing.T) {
		status, urlResp := doSearch(t, devToken, "", url.Values{"q": {"https://pay.alpha.example.com/v1/orders"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		routeHit, ok := hasItem(urlResp, platformapi.SearchTypeGatewayRoute, routeAlpha.ID)
		if !ok || routeHit.ApplicationID != appAlpha.ID || routeHit.ProjectID != projAlpha.ID {
			t.Fatalf("expected gateway route hit with project/app context, got %#v", urlResp.Items)
		}

		status, imgResp := doSearch(t, devToken, "", url.Values{"q": {"checkout-api:2.4.1"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if _, ok := hasItem(imgResp, platformapi.SearchTypeDeploymentTarget, targetAlpha.ID); !ok {
			t.Fatalf("expected deployment_target hit by imageRef, got %#v", imgResp.Items)
		}
		// Ordinary keyword search must NOT scan releases table unless explicitly requested.
		if _, scanned := hasItem(imgResp, platformapi.SearchTypeRelease, release.ID); scanned {
			t.Fatal("expected ordinary keyword search not to scan releases table")
		}
	})

	t.Run("secret and environment variable values are never indexed or matched", func(t *testing.T) {
		status, resp := doSearch(t, devToken, "", url.Values{"q": {"never_match_value_xyz"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if len(resp.Items) != 0 {
			t.Fatalf("expected 0 hits on env/variable values, got %#v", resp.Items)
		}
	})

	t.Run("viewer can search project apps and domains but not project notification channels", func(t *testing.T) {
		status, resp := doSearch(t, viewerToken, "", url.Values{"q": {"alpha"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if _, ok := hasItem(resp, platformapi.SearchTypeApplication, appAlpha.ID); !ok {
			t.Fatal("viewer should see project application")
		}
		if _, leaked := hasItem(resp, platformapi.SearchTypeNotificationChannel, projNotifChannel.ID); leaked {
			t.Fatal("viewer must not see project-scoped notification channel without ActionNotificationRead")
		}
	})

	t.Run("commit SHA and resource ID fast paths locate build runs and releases", func(t *testing.T) {
		status, shaResp := doSearch(t, devToken, "", url.Values{"q": {"7f4e9a1bc0"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if _, ok := hasItem(shaResp, platformapi.SearchTypeBuildRun, buildRun.ID); !ok {
			t.Fatalf("expected build_run hit by commit SHA prefix, got %#v", shaResp.Items)
		}

		status, relResp := doSearch(t, devToken, "", url.Values{"q": {"rel_search_prod"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if len(relResp.Items) != 1 || relResp.Items[0].ID != release.ID {
			t.Fatalf("expected single release fast-path hit, got %#v", relResp.Items)
		}
	})

	t.Run("platform admin visibility related vs all and user lookup", func(t *testing.T) {
		status, forbidden := doSearch(t, devToken, "", url.Values{"q": {"alpha"}, "visibility": {"all"}})
		if status != http.StatusForbidden {
			t.Fatalf("non-admin visibility=all status = %d, resp = %#v", status, forbidden)
		}

		status, relatedResp := doSearch(t, adminToken, "", url.Values{"q": {"alpha"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if _, leaked := hasItem(relatedResp, platformapi.SearchTypeApplication, appSecret.ID); leaked {
			t.Fatal("admin default related visibility should omit non-member project app")
		}
		if _, ok := hasItem(relatedResp, platformapi.SearchTypeUser, dev.ID); !ok {
			// "Alice Developer" matches "alice", let's query "alice" explicitly below.
		}

		status, userResp := doSearch(t, adminToken, "", url.Values{"q": {"alice-dev@example.test"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if _, ok := hasItem(userResp, platformapi.SearchTypeUser, dev.ID); !ok {
			t.Fatalf("expected admin user lookup hit, got %#v", userResp.Items)
		}

		status, allResp := doSearch(t, adminToken, "", url.Values{"q": {"alpha"}, "visibility": {"all"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		ownHit, okOwn := hasItem(allResp, platformapi.SearchTypeApplication, appAlpha.ID)
		otherHit, okOther := hasItem(allResp, platformapi.SearchTypeApplication, appSecret.ID)
		if !okOwn || !okOther {
			t.Fatalf("expected both own and global applications under visibility=all, got %#v", allResp.Items)
		}
		if !ownHit.Related || otherHit.Related {
			t.Fatalf("unexpected related flags: own=%t other=%t", ownHit.Related, otherHit.Related)
		}
	})

	t.Run("restricted PAT only returns categories permitted by its token scopes", func(t *testing.T) {
		status, resp := doSearch(t, "", restrictedPATPlain, url.Values{"q": {"alpha"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if len(resp.Items) != 1 || resp.Items[0].Type != platformapi.SearchTypeProject || resp.Items[0].ID != projAlpha.ID {
			t.Fatalf("expected project:read PAT to return only project hit, got %#v", resp.Items)
		}
	})

	t.Run("SQL LIKE wildcard characters in query are matched literally", func(t *testing.T) {
		status, resp := doSearch(t, devToken, "", url.Values{"q": {"100%"}})
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if len(resp.Items) != 1 || resp.Items[0].ID != wildcardApp.ID {
			t.Fatalf("expected only literal '100%%' application, got %#v", resp.Items)
		}
	})
}
