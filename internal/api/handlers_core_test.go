package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
)

func TestBootstrapStatusHidesDevLoginHintInProduction(t *testing.T) {
	t.Setenv("LOCAL_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("LOCAL_ADMIN_PASSWORD", "secret-password")

	status := bootstrapStatusResponse("production", false)

	if status["devLoginEnabled"] != false {
		t.Fatalf("expected dev login disabled in production, got %v", status["devLoginEnabled"])
	}
	if _, ok := status["devLoginHint"]; ok {
		t.Fatal("expected production bootstrap status to omit devLoginHint")
	}
}

func TestPaginationFromQueryDefaultsAndCapsPageSize(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/access-tokens?page=0&pageSize=999&sortBy=name&sortOrder=asc", nil)

	pagination := paginationFromQuery(ctx)

	if pagination.Page != 1 {
		t.Fatalf("Page = %d", pagination.Page)
	}
	if pagination.PageSize != 100 {
		t.Fatalf("PageSize = %d", pagination.PageSize)
	}
	if pagination.Offset() != 0 {
		t.Fatalf("Offset = %d", pagination.Offset())
	}
	if pagination.SortBy != "name" {
		t.Fatalf("SortBy = %q", pagination.SortBy)
	}
	if pagination.SortOrder != "asc" {
		t.Fatalf("SortOrder = %q", pagination.SortOrder)
	}
}

func TestPaginatedResponseCalculatesTotalPages(t *testing.T) {
	response := paginatedResponse([]string{"a", "b"}, 21, paginationParams{Page: 2, PageSize: 10, SortBy: "name", SortOrder: "asc"})

	if response["totalPages"] != 3 {
		t.Fatalf("totalPages = %v", response["totalPages"])
	}
	if response["total"] != int64(21) {
		t.Fatalf("total = %v", response["total"])
	}
	if response["sortBy"] != "name" || response["sortOrder"] != "asc" {
		t.Fatalf("sort response = %v/%v", response["sortBy"], response["sortOrder"])
	}
}

func TestOrderByClauseUsesWhitelist(t *testing.T) {
	pagination := paginationParams{SortBy: "name", SortOrder: "asc"}
	orderBy := orderByClause(pagination, map[string]string{"name": "name"}, "created_at")
	if orderBy != "name asc" {
		t.Fatalf("orderBy = %q", orderBy)
	}

	pagination = paginationParams{SortBy: "name;drop table users", SortOrder: "wat"}
	orderBy = orderByClause(pagination, map[string]string{"name": "name"}, "created_at")
	if orderBy != "created_at desc" {
		t.Fatalf("fallback orderBy = %q", orderBy)
	}
}

func TestNormalizedProjectOrderIDsDeduplicatesAndTrims(t *testing.T) {
	got := normalizedProjectOrderIDs([]string{" prj_1 ", "", "prj_2", "prj_1"})
	if len(got) != 2 || got[0] != "prj_1" || got[1] != "prj_2" {
		t.Fatalf("ids = %#v", got)
	}
}

func TestNormalizeRepositoryBindingIdentity(t *testing.T) {
	if owner := normalizeRepositoryBindingOwner(" SnowyKami "); owner != "snowykami" {
		t.Fatalf("owner = %q", owner)
	}
	if repo := normalizeRepositoryBindingRepo(" Neo-Blog.GIT "); repo != "neo-blog" {
		t.Fatalf("repo = %q", repo)
	}
}

func TestResourceCanMutateDuringDeleteAllowsOnlyStableStates(t *testing.T) {
	for _, status := range []string{"", "active", "delete_failed"} {
		if !resourceCanMutateDuringDelete(status) {
			t.Fatalf("expected status %q to allow mutation", status)
		}
	}
	for _, status := range []string{"deleting", "deleted"} {
		if resourceCanMutateDuringDelete(status) {
			t.Fatalf("expected status %q to block mutation", status)
		}
	}
}

func TestProjectPinResponseIncludesDashboardOrder(t *testing.T) {
	project := model.Project{ID: "prj_1", Slug: "demo", Name: "Demo"}
	pin := model.ProjectPin{ProjectID: "prj_1"}
	response := projectPinResponseFrom(project, pin, 3)
	if response.DashboardOrder != 3 {
		t.Fatalf("dashboardOrder = %d", response.DashboardOrder)
	}
}

func TestDefaultUserProjectNameUsesLanguage(t *testing.T) {
	zh := defaultUserProjectName(model.User{Name: "轻雪", Language: "zh-CN"})
	if zh != "轻雪 的项目空间" {
		t.Fatalf("zh project name = %q", zh)
	}

	en := defaultUserProjectName(model.User{Name: "Liteyuki", Language: "en-US"})
	if en != "Liteyuki's Project Space" {
		t.Fatalf("en project name = %q", en)
	}
}

func TestPlatformAdminBypassesProjectMemberRoleChecks(t *testing.T) {
	allowedRoles := []string{"owner"}
	if !projectUserRoleAllowed(model.User{Role: "platform_admin"}, "", allowedRoles) {
		t.Fatal("expected platform admin to bypass project member role checks")
	}
	if projectUserRoleAllowed(model.User{Role: "user"}, "viewer", allowedRoles) {
		t.Fatal("expected regular viewer to be blocked from owner-only project operation")
	}
	if !projectUserRoleAllowed(model.User{Role: "user"}, "owner", allowedRoles) {
		t.Fatal("expected project owner to be allowed")
	}
}

func TestUserProjectSlugHelpersNormalizeAndLimitLength(t *testing.T) {
	if slug := dnsSafeProjectSlug("Alice.Dev_Ops"); slug != "alice-dev-ops" {
		t.Fatalf("normalized slug = %q", slug)
	}

	slug := slugWithNumericSuffix(strings.Repeat("a", 80), 1)
	if len(slug) > 48 || !strings.HasSuffix(slug, "-2") {
		t.Fatalf("suffixed slug = %q", slug)
	}
}

func TestBuildImageRefOmitsDockerHubDomainAndRendersTagTemplate(t *testing.T) {
	registry := model.ArtifactRegistry{Provider: "dockerhub", Endpoint: "https://registry-1.docker.io", Namespace: "snowykami"}
	project := model.Project{Slug: "demo"}
	application := model.Application{Slug: "blog"}
	run := model.BuildRun{
		TargetRepository: buildTargetImageRepository(registry, project, application),
		TargetTag:        "${{ github.ref_name }}-{short_sha}",
		SourceBranch:     "main",
		SourceCommit:     "1234567890abcdef",
	}

	if ref := buildImageRef(registry, run); ref != "snowykami/demo-blog:main-1234567890ab" {
		t.Fatalf("dockerhub image ref = %q", ref)
	}
}

func TestBuildImageRefAddsNonDockerHubDomainPrefix(t *testing.T) {
	registry := model.ArtifactRegistry{Provider: "harbor", Endpoint: "https://harbor.example.com", Namespace: "team"}
	project := model.Project{Slug: "demo"}
	application := model.Application{Slug: "api"}
	run := model.BuildRun{
		TargetRepository: buildTargetImageRepository(registry, project, application),
		TargetTag:        "release/${{ github.ref_name }}",
		SourceBranch:     "feature/login",
	}

	if ref := buildImageRef(registry, run); ref != "harbor.example.com/team/demo-api:release-feature-login" {
		t.Fatalf("harbor image ref = %q", ref)
	}
}

func TestBuildTargetImageRepositoryFallsBackToProjectSlugNamespace(t *testing.T) {
	registry := model.ArtifactRegistry{Provider: "harbor", Endpoint: "https://harbor.example.com"}
	project := model.Project{Slug: "demo"}
	application := model.Application{Slug: "api"}

	if repository := buildTargetImageRepository(registry, project, application); repository != "harbor.example.com/demo/demo-api" {
		t.Fatalf("repository = %q", repository)
	}
}

func TestSplitTargetImageRef(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		repository string
		tag        string
	}{
		{name: "repository and tag", value: "snowykami/neo-blog-front:latest", repository: "snowykami/neo-blog-front", tag: "latest"},
		{name: "template tag", value: "team/api:${{ github.ref_name }}-{short_sha}", repository: "team/api", tag: "${{ github.ref_name }}-{short_sha}"},
		{name: "no tag", value: "team/api", repository: "team/api", tag: "latest"},
		{name: "registry host", value: "registry.example.com/team/api:dev", repository: "registry.example.com/team/api", tag: "dev"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, tag := splitTargetImageRef(test.value)
			if repository != test.repository || tag != test.tag {
				t.Fatalf("splitTargetImageRef(%q) = %q/%q", test.value, repository, tag)
			}
		})
	}
}

func TestConfigValueToStringAcceptsStructuredValues(t *testing.T) {
	text, err := configValueToString("Liteyuki")
	if err != nil || text != "Liteyuki" {
		t.Fatalf("string value = %q, %v", text, err)
	}

	text, err = configValueToString(true)
	if err != nil || text != "true" {
		t.Fatalf("bool value = %q, %v", text, err)
	}

	text, err = configValueToString(map[string]any{"url": "/liteyuki-logo.svg"})
	if err != nil || text != `{"url":"/liteyuki-logo.svg"}` {
		t.Fatalf("object value = %q, %v", text, err)
	}
}

func TestIPBlockListDefinitionDefaultsToReservedRanges(t *testing.T) {
	var definition configDefinition
	for _, item := range configDefinitions {
		if item.Key == "security.egress.ipBlockList" {
			definition = item
			break
		}
	}

	if definition.Key == "" {
		t.Fatal("ip block list definition not found")
	}
	for _, expected := range []string{"0.0.0.0/8", "10.0.0.0/8", "127.0.0.0/8", "192.168.0.0/16", "::1/128", "fc00::/7", "fe80::/10"} {
		if !strings.Contains(definition.Default, expected) {
			t.Fatalf("expected default ip block list to include %s, got %q", expected, definition.Default)
		}
	}
}

func TestDefaultIPBlockListOverridesAdminPrivateNetworkAccess(t *testing.T) {
	h := &Handlers{
		configs: &configCache{values: map[string]string{
			"security.egress.domainAllowList": "",
			"security.egress.domainBlockList": "",
			"security.egress.ipAllowList":     "",
			"security.egress.ipBlockList":     security.ReservedIPBlockListText(),
			"security.egress.allowedPorts":    "",
		}},
	}

	policy := h.egressPolicyForUser(model.User{Role: "platform_admin"})
	if _, err := policy.ValidateURL("http://127.0.0.1:8080"); !errors.Is(err, security.ErrBlockedByPolicy) {
		t.Fatalf("expected default explicit block list to block loopback even for admin policy, got %v", err)
	}

	h.configs.values["security.egress.ipBlockList"] = ""
	policy = h.egressPolicyForUser(model.User{Role: "platform_admin"})
	if _, err := policy.ValidateURL("http://127.0.0.1:8080"); err != nil {
		t.Fatalf("expected edited empty block list to allow admin private network access, got %v", err)
	}
}
