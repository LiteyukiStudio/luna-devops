package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/gin-gonic/gin"
)

func TestCrossProjectListVisibilityPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	db := aiProjectScopeIntegrationDB(t)

	admin := model.User{ID: "usr_visibility_admin", Email: "visibility-admin@example.test", Name: "Visibility Admin", Role: authz.PlatformRoleAdmin}
	other := model.User{ID: "usr_visibility_other", Email: "visibility-other@example.test", Name: "Visibility Other", Role: authz.PlatformRoleUser}
	developer := model.User{ID: "usr_visibility_developer", Email: "visibility-developer@example.test", Name: "Visibility Developer", Role: authz.PlatformRoleUser}
	viewer := model.User{ID: "usr_visibility_viewer", Email: "visibility-viewer@example.test", Name: "Visibility Viewer", Role: authz.PlatformRoleUser}
	projects := []model.Project{
		{ID: "prj_visibility_related", Identifier: "visibility-related", Name: "Related", NamespaceStrategy: "project", DeleteStatus: "active"},
		{ID: "prj_visibility_other", Identifier: "visibility-other", Name: "Other", NamespaceStrategy: "project", DeleteStatus: "active"},
		{ID: "prj_visibility_developer", Identifier: "visibility-developer", Name: "Developer", NamespaceStrategy: "project", DeleteStatus: "active"},
		{ID: "prj_visibility_viewer", Identifier: "visibility-viewer", Name: "Viewer", NamespaceStrategy: "project", DeleteStatus: "active"},
	}
	members := []model.ProjectMember{
		{ID: "prjm_visibility_admin", ProjectID: projects[0].ID, UserID: admin.ID, Role: authz.ProjectRoleOwner},
		{ID: "prjm_visibility_developer", ProjectID: projects[2].ID, UserID: developer.ID, Role: authz.ProjectRoleDeveloper},
		{ID: "prjm_visibility_viewer", ProjectID: projects[3].ID, UserID: viewer.ID, Role: authz.ProjectRoleViewer},
	}
	for _, value := range []any{&admin, &other, &developer, &viewer, &projects, &members} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	clusters := []model.RuntimeCluster{
		{ID: "rcl_visibility_global", Name: "Global", Type: "kubernetes", Scope: "global"},
		{ID: "rcl_visibility_user", Name: "Current user", Type: "kubernetes", Scope: "user", OwnerRef: admin.ID},
		{ID: "rcl_visibility_other_user", Name: "Other user", Type: "kubernetes", Scope: "user", OwnerRef: other.ID},
		{ID: "rcl_visibility_related", Name: "Related project", Type: "kubernetes", Scope: "project"},
		{ID: "rcl_visibility_other", Name: "Other project", Type: "kubernetes", Scope: "project"},
		{ID: "rcl_visibility_unbound", Name: "Unbound project scope", Type: "kubernetes", Scope: "project"},
	}
	bindings := []model.ScopedResourceProjectBinding{
		{ID: "srpb_visibility_related", ResourceType: scopedResourceRuntimeCluster, ResourceID: clusters[3].ID, ProjectID: projects[0].ID},
		{ID: "srpb_visibility_other", ResourceType: scopedResourceRuntimeCluster, ResourceID: clusters[4].ID, ProjectID: projects[1].ID},
	}
	if err := db.Create(&clusters).Error; err != nil {
		t.Fatalf("seed runtime clusters: %v", err)
	}
	if err := db.Create(&bindings).Error; err != nil {
		t.Fatalf("seed scoped resource bindings: %v", err)
	}

	handlers := &Handlers{db: db, projects: repository.NewProjectRepository(db)}
	t.Run("shared scoped resources", func(t *testing.T) {
		related := listRuntimeClustersByVisibility(t, handlers, admin, projectservice.ListVisibilityRelated)
		assertStringIDs(t, related, []string{clusters[0].ID, clusters[1].ID, clusters[3].ID})

		all := listRuntimeClustersByVisibility(t, handlers, admin, projectservice.ListVisibilityAll)
		assertStringIDs(t, all, []string{clusters[0].ID, clusters[1].ID, clusters[3].ID, clusters[4].ID, clusters[5].ID})
	})

	t.Run("container images", func(t *testing.T) {
		registry := model.ArtifactRegistry{ID: "areg_visibility", Name: "Visibility", Provider: "generic", Endpoint: "https://registry.example.test", Scope: "global"}
		if err := db.Create(&registry).Error; err != nil {
			t.Fatalf("seed registry: %v", err)
		}
		images := []model.ContainerImage{
			{ID: "img_visibility_user", RegistryID: registry.ID, Repository: "current", Tag: "latest", ImageRef: "registry.example.test/current:latest", CreatedBy: admin.ID},
			{ID: "img_visibility_other_user", RegistryID: registry.ID, Repository: "other-user", Tag: "latest", ImageRef: "registry.example.test/other-user:latest", CreatedBy: other.ID},
			{ID: "img_visibility_related", ProjectID: projects[0].ID, RegistryID: registry.ID, Repository: "related", Tag: "latest", ImageRef: "registry.example.test/related:latest", CreatedBy: other.ID},
			{ID: "img_visibility_other", ProjectID: projects[1].ID, RegistryID: registry.ID, Repository: "other", Tag: "latest", ImageRef: "registry.example.test/other:latest", CreatedBy: other.ID},
		}
		if err := db.Create(&images).Error; err != nil {
			t.Fatalf("seed container images: %v", err)
		}
		assertStringIDs(t, listContainerImageIDs(t, handlers, admin, ""), []string{images[0].ID, images[2].ID})
		assertStringIDs(t, listContainerImageIDs(t, handlers, admin, "all"), []string{images[0].ID, images[2].ID, images[3].ID})
	})

	t.Run("cluster resources include every member role", func(t *testing.T) {
		items := []kubeprovider.ResourceSnapshot{
			{ID: "kres_developer", ProjectID: projects[2].ID},
			{ID: "kres_viewer", ProjectID: projects[3].ID},
			{ID: "kres_other", ProjectID: projects[1].ID},
			{ID: "kres_unscoped"},
		}
		developerItems := handlers.filterClusterResourceSnapshots(visibilityContext(developer, ""), developer, items, projectservice.ListVisibilityRelated, "")
		assertSnapshotIDs(t, developerItems, []string{"kres_developer"})
		viewerItems := handlers.filterClusterResourceSnapshots(visibilityContext(viewer, ""), viewer, items, projectservice.ListVisibilityRelated, "")
		assertSnapshotIDs(t, viewerItems, []string{"kres_viewer"})
		adminRelated := handlers.filterClusterResourceSnapshots(visibilityContext(admin, ""), admin, items, projectservice.ListVisibilityRelated, "")
		assertSnapshotIDs(t, adminRelated, nil)
		adminAll := handlers.filterClusterResourceSnapshots(visibilityContext(admin, "all"), admin, items, projectservice.ListVisibilityAll, "")
		assertSnapshotIDs(t, adminAll, []string{"kres_developer", "kres_viewer", "kres_other", "kres_unscoped"})
		explicit := handlers.filterClusterResourceSnapshots(visibilityContext(admin, ""), admin, items, projectservice.ListVisibilityRelated, projects[1].ID)
		assertSnapshotIDs(t, explicit, []string{"kres_other"})
	})
}

func listRuntimeClustersByVisibility(t *testing.T, handlers *Handlers, user model.User, visibility projectservice.ListVisibility) []string {
	t.Helper()
	ctx := visibilityContext(user, string(visibility))
	query, ok := handlers.applyScopedResourceListVisibility(ctx, handlers.db.Model(&model.RuntimeCluster{}), scopedResourceRuntimeCluster, user, "", visibility)
	if !ok {
		t.Fatal("scoped resource visibility unexpectedly rejected")
	}
	var clusters []model.RuntimeCluster
	if err := query.Find(&clusters).Error; err != nil {
		t.Fatalf("list runtime clusters: %v", err)
	}
	ids := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		ids = append(ids, cluster.ID)
	}
	return ids
}

func listContainerImageIDs(t *testing.T, handlers *Handlers, user model.User, visibility string) []string {
	t.Helper()
	path := "/api/v1/container-images?page=1&pageSize=100"
	if visibility != "" {
		path += "&visibility=" + visibility
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	ctx.Set(currentUserContextKey, user)
	handlers.ListContainerImages(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list container images: %d %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []model.ContainerImage `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode container images: %v", err)
	}
	ids := make([]string, 0, len(response.Items))
	for _, item := range response.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

func visibilityContext(user model.User, visibility string) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	path := "/api/v1/runtime/clusters"
	if visibility != "" {
		path += "?visibility=" + visibility
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	ctx.Set(currentUserContextKey, user)
	return ctx
}

func assertSnapshotIDs(t *testing.T, items []kubeprovider.ResourceSnapshot, want []string) {
	t.Helper()
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.ID)
	}
	assertStringIDs(t, got, want)
}

func assertStringIDs(t *testing.T, got, want []string) {
	t.Helper()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("IDs = %v, want %v", got, want)
	}
}
