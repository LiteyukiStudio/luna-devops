package api

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/LiteyukiStudio/devops/internal/variables"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

func TestPaginationFromQueryWithSortReturnsEffectiveWhitelistValue(t *testing.T) {
	allowed := map[string]string{"createdAt": "created_at", "name": "name"}
	for _, testCase := range []struct {
		query string
		want  string
	}{
		{query: "", want: "createdAt"},
		{query: "?sortBy=unknown", want: "createdAt"},
		{query: "?sortBy=name", want: "name"},
	} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodGet, "/"+testCase.query, nil)
		if got := paginationFromQueryWithSort(ctx, allowed, "createdAt").SortBy; got != testCase.want {
			t.Fatalf("sortBy = %q, want %q", got, testCase.want)
		}
	}
}

func TestPriorityListPageQueriesApplyNormalizedLimitAndOffset(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}

	builders := map[string]func(*gorm.DB, paginationParams) *gorm.DB{
		"build-runs":         buildRunPageQuery,
		"container-images":   containerImagePageQuery,
		"deployment-targets": deploymentTargetPageQuery,
		"projects":           projectPageQuery,
	}
	cases := []struct {
		name       string
		query      string
		wantPage   int
		wantLimit  int
		wantOffset int
	}{
		{name: "defaults", query: "", wantPage: 1, wantLimit: defaultPageSize, wantOffset: 0},
		{name: "maximum cap", query: "?page=2&pageSize=101", wantPage: 2, wantLimit: maxPageSize, wantOffset: maxPageSize},
		{name: "invalid fallback", query: "?page=-2&pageSize=0", wantPage: 1, wantLimit: defaultPageSize, wantOffset: 0},
	}

	for name, build := range builders {
		for _, testCase := range cases {
			t.Run(name+"/"+testCase.name, func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest(http.MethodGet, "/"+testCase.query, nil)
				pagination := paginationFromQuery(ctx)
				if pagination.Page != testCase.wantPage {
					t.Fatalf("page = %d, want %d", pagination.Page, testCase.wantPage)
				}

				query := build(db.Table("list_items"), pagination)
				limit, ok := query.Statement.Clauses["LIMIT"].Expression.(clause.Limit)
				if !ok || limit.Limit == nil {
					t.Fatalf("query has no LIMIT clause: %#v", query.Statement.Clauses)
				}
				if *limit.Limit != testCase.wantLimit || limit.Offset != testCase.wantOffset {
					t.Fatalf("LIMIT/OFFSET = %d/%d, want %d/%d", *limit.Limit, limit.Offset, testCase.wantLimit, testCase.wantOffset)
				}

				assertPaginationEnvelope(t, paginatedResponse([]string{"item"}, 101, pagination))
			})
		}
	}
}

func TestHighGrowthListHandlersCannotBypassPaginationContract(t *testing.T) {
	contracts := []struct {
		file     string
		function string
	}{
		{file: "application_handlers.go", function: "ListApplications"},
		{file: "build_job_log_handlers.go", function: "ListBuildJobs"},
		{file: "build_run_handlers.go", function: "ListBuildRuns"},
		{file: "build_variable_handlers.go", function: "ListBuildVariableSets"},
		{file: "container_image_handlers.go", function: "ListContainerImages"},
		{file: "deployment_target_handlers.go", function: "ListDeploymentTargets"},
		{file: "gateway_handlers.go", function: "ListGatewayRoutes"},
		{file: "git_account_handlers.go", function: "ListGitAccounts"},
		{file: "git_provider_handlers.go", function: "ListGitProviders"},
		{file: "git_repository_handlers.go", function: "ListGitRepositories"},
		{file: "git_repository_handlers.go", function: "ListRepositoryBindings"},
		{file: "project_handlers.go", function: "ListProjects"},
		{file: "project_handlers.go", function: "ListProjectPins"},
		{file: "project_handlers.go", function: "ListProjectMembers"},
		{file: "project_hook_handlers.go", function: "ListProjectHookConfigs"},
		{file: "project_hook_handlers.go", function: "ListProjectHookRuns"},
		{file: "registries.go", function: "ListArtifactRegistries"},
		{file: "registry_credential_handlers.go", function: "listRegistryCredentials"},
		{file: "release_handlers.go", function: "ListReleases"},
		{file: "runtime_cluster_handlers.go", function: "ListRuntimeClusters"},
		{file: "runtime_cluster_resource_handlers.go", function: "ListRuntimeClusterResources"},
		{file: "runtime_cluster_resource_handlers.go", function: "ListRuntimeClusterResourceEvents"},
		{file: "runtime_config_handlers.go", function: "ListProjectRuntimeConfigSets"},
	}

	for _, contract := range contracts {
		t.Run(contract.function, func(t *testing.T) {
			function := parseAPIFunction(t, contract.file, contract.function)
			calls := calledFunctions(function.Body)
			if !calls["paginatedResponse"] {
				t.Fatalf("%s must return the shared pagination envelope", contract.function)
			}
			sortNormalized := calls["paginationFromQueryWithSort"] || calls["normalizeClusterResourceSortBy"] || calls["gitRepositoryPagination"]
			if !sortNormalized {
				t.Fatalf("%s must normalize sortBy to an effective whitelist value", contract.function)
			}
			bounded := calls["paginateSlice"] || (calls["Limit"] && calls["Offset"]) ||
				calls["buildRunPageQuery"] || calls["containerImagePageQuery"] ||
				calls["deploymentTargetPageQuery"] || calls["projectPageQuery"] ||
				calls["ListRepositories"] || calls["SearchPublicRepositories"] ||
				calls["ListManagedResourcesPage"] || calls["ListManagedResourceEventsPage"]
			if !bounded {
				t.Fatalf("%s must apply LIMIT/OFFSET or bounded slice pagination", contract.function)
			}
		})
	}
}

func TestGitRepositoryPaginationUsesUnifiedBoundsAndEffectiveSort(t *testing.T) {
	cases := []struct {
		query      string
		wantPage   int
		wantSize   int
		wantOffset int
	}{
		{query: "", wantPage: 1, wantSize: defaultPageSize, wantOffset: 0},
		{query: "?page=2&pageSize=101&sortBy=unknown&sortOrder=asc", wantPage: 2, wantSize: maxPageSize, wantOffset: maxPageSize},
		{query: "?page=-1&pageSize=0", wantPage: 1, wantSize: defaultPageSize, wantOffset: 0},
	}
	for _, testCase := range cases {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodGet, "/repositories"+testCase.query, nil)
		pagination := gitRepositoryPagination(ctx)
		if pagination.Page != testCase.wantPage || pagination.PageSize != testCase.wantSize || pagination.Offset() != testCase.wantOffset {
			t.Fatalf("pagination = %#v, want page/size/offset %d/%d/%d", pagination, testCase.wantPage, testCase.wantSize, testCase.wantOffset)
		}
		if pagination.SortBy != "updatedAt" || pagination.SortOrder != "desc" {
			t.Fatalf("effective sort = %s/%s", pagination.SortBy, pagination.SortOrder)
		}
		assertPaginationEnvelope(t, paginatedResponse([]string{"repository"}, remotePageTotal(pagination, 1), pagination))
	}
}

func assertPaginationEnvelope(t *testing.T, response gin.H) {
	t.Helper()
	for _, key := range []string{"items", "page", "pageSize", "sortBy", "sortOrder", "total", "totalPages"} {
		if _, exists := response[key]; !exists {
			t.Fatalf("paginated response is missing %q: %#v", key, response)
		}
	}
	if len(response) != 7 {
		t.Fatalf("paginated response has unexpected fields: %#v", response)
	}
}

func parseAPIFunction(t *testing.T, fileName string, functionName string) *ast.FuncDecl {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	path := filepath.Join(filepath.Dir(currentFile), fileName)
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, declaration := range parsed.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == functionName {
			return function
		}
	}
	t.Fatalf("function %s was not found in %s", functionName, path)
	return nil
}

func calledFunctions(node ast.Node) map[string]bool {
	calls := map[string]bool{}
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch function := call.Fun.(type) {
		case *ast.Ident:
			calls[function.Name] = true
		case *ast.SelectorExpr:
			calls[function.Sel.Name] = true
		}
		return true
	})
	return calls
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

func TestOrderByClauseToleratesDefaultColumnWithDirection(t *testing.T) {
	pagination := paginationParams{SortBy: "", SortOrder: "desc"}
	if got := orderByClause(pagination, map[string]string{"occurredAt": "occurred_at"}, "occurred_at desc"); got != "occurred_at desc" {
		t.Fatalf("orderBy = %q", got)
	}
	if got := orderByClause(pagination, map[string]string{}, "occurred_at asc"); got != "occurred_at desc" {
		t.Fatalf("orderBy = %q", got)
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

func TestNormalizeRegistryProviderSupportsGenericOCI(t *testing.T) {
	cases := map[string]string{
		"":                "harbor",
		"Harbor":          "harbor",
		"dockerhub":       "dockerhub",
		"gitea-registry":  "gitea-registry",
		"generic-oci":     "generic-oci",
		"docker-registry": "generic-oci",
		"custom-vendor":   "generic-oci",
	}

	for input, expected := range cases {
		if actual := normalizeRegistryProvider(input); actual != expected {
			t.Fatalf("normalizeRegistryProvider(%q) = %q, want %q", input, actual, expected)
		}
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
	project := model.Project{ID: "prj_1", Identifier: "demo", Name: "Demo"}
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

	en := defaultUserProjectName(model.User{Name: "Luna", Language: "en-US"})
	if en != "Luna's Project Space" {
		t.Fatalf("en project name = %q", en)
	}
}

func TestPlatformAdminBypassesProjectMemberRoleChecks(t *testing.T) {
	allowedRoles := []string{authz.ProjectRoleOwner}
	if !projectUserRoleAllowed(model.User{Role: authz.PlatformRoleAdmin}, "", allowedRoles) {
		t.Fatal("expected platform admin to bypass project member role checks")
	}
	if projectUserRoleAllowed(model.User{Role: authz.PlatformRoleUser}, authz.ProjectRoleViewer, allowedRoles) {
		t.Fatal("expected regular viewer to be blocked from owner-only project operation")
	}
	if !projectUserRoleAllowed(model.User{Role: authz.PlatformRoleUser}, authz.ProjectRoleOwner, allowedRoles) {
		t.Fatal("expected project owner to be allowed")
	}
}

func TestUserProjectIdentifierHelpersNormalizeAndLimitLength(t *testing.T) {
	if identifier := dnsSafeProjectIdentifier("Alice.Dev_Ops"); identifier != "alice-dev-ops" {
		t.Fatalf("normalized identifier = %q", identifier)
	}

	identifier := slugWithNumericSuffix(strings.Repeat("a", 80), 1)
	if len(identifier) > projectIdentifierMaxLength || !strings.HasSuffix(identifier, "-2") {
		t.Fatalf("suffixed identifier = %q", identifier)
	}
}

func TestBuildImageRefOmitsDockerHubDomainAndRendersTagTemplate(t *testing.T) {
	registry := model.ArtifactRegistry{Provider: "dockerhub", Endpoint: "https://registry-1.docker.io", Namespace: "snowykami"}
	project := model.Project{Identifier: "demo"}
	application := model.Application{Identifier: "blog"}
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
	project := model.Project{Identifier: "demo"}
	application := model.Application{Identifier: "api"}
	run := model.BuildRun{
		TargetRepository: buildTargetImageRepository(registry, project, application),
		TargetTag:        "release/${{ github.ref_name }}",
		SourceBranch:     "feature/login",
	}

	if ref := buildImageRef(registry, run); ref != "harbor.example.com/team/demo-api:release-feature-login" {
		t.Fatalf("harbor image ref = %q", ref)
	}
}

func TestBuildTargetImageRepositoryFallsBackToProjectIdentifierNamespace(t *testing.T) {
	registry := model.ArtifactRegistry{Provider: "harbor", Endpoint: "https://harbor.example.com"}
	project := model.Project{Identifier: "demo"}
	application := model.Application{Identifier: "api"}

	if repository := buildTargetImageRepository(registry, project, application); repository != "harbor.example.com/demo/demo-api" {
		t.Fatalf("repository = %q", repository)
	}
}

func TestCredentialRepositoryTemplateUsesStage(t *testing.T) {
	registry := model.ArtifactRegistry{Provider: "dockerhub", Endpoint: "https://registry-1.docker.io", Namespace: "snowykami"}
	credential := model.RegistryCredential{RepositoryTemplate: "devopsns/{project}-{app}-{stage}", TagTemplate: "{commit}"}
	project := model.Project{Identifier: "neo-blog"}
	application := model.Application{Identifier: "frontend"}
	target := model.DeploymentTarget{Name: "prod", Stage: "production"}

	repository, tag := splitTargetImageRef(buildTargetImageRepositoryForCredential(registry, credential, project, application, target) + ":" + buildTargetImageTagTemplateForCredential(credential))
	if repository != "devopsns/neo-blog-frontend-production" || tag != "{commit}" {
		t.Fatalf("templated image = %q:%q", repository, tag)
	}
}

func TestCredentialStaticTagTemplateOnlyUsesDeploymentContext(t *testing.T) {
	registry := model.ArtifactRegistry{Provider: "harbor", Endpoint: "https://harbor.example.com", Namespace: "team"}
	project := model.Project{Identifier: "neo-blog"}
	application := model.Application{Identifier: "frontend"}
	target := model.DeploymentTarget{Name: "prod-web", Stage: "prod"}

	staticCredential := model.RegistryCredential{TagTemplate: "{projectIdentifier}-{appIdentifier}-{stage}"}
	if tag := buildStaticTargetImageTagForCredential(registry, staticCredential, project, application, target); tag != "neo-blog-frontend-prod" {
		t.Fatalf("static tag = %q", tag)
	}

	buildVariableCredential := model.RegistryCredential{TagTemplate: "{commit}"}
	if tag := buildStaticTargetImageTagForCredential(registry, buildVariableCredential, project, application, target); tag != "latest" {
		t.Fatalf("build variable tag = %q", tag)
	}
}

func TestDefaultImageRepositoryAcceptsHostlessInput(t *testing.T) {
	registry := model.ArtifactRegistry{Provider: "harbor", Endpoint: "https://harbor.example.com"}
	project := model.Project{Identifier: "demo"}
	application := model.Application{Identifier: "api"}

	if !isDefaultImageRepository(registry, project, application, "demo/demo-api") {
		t.Fatal("expected hostless default repository to be recognized")
	}
}

func TestBuildTagTemplateSupportsFriendlyVariables(t *testing.T) {
	got := renderBuildTagTemplate("{branchSlug}-{shortSha}-{commit}", variables.Context{
		SourceBranch: "feature/Login Page",
		SourceCommit: "1234567890abcdef",
	})
	want := "feature-login-page-1234567890ab-1234567890abcdef"
	if got != want {
		t.Fatalf("tag = %q, want %q", got, want)
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
	text, err := configValueToString("Luna")
	if err != nil || text != "Luna" {
		t.Fatalf("string value = %q, %v", text, err)
	}

	text, err = configValueToString(true)
	if err != nil || text != "true" {
		t.Fatalf("bool value = %q, %v", text, err)
	}

	text, err = configValueToString(float64(2_000_000))
	if err != nil || text != "2000000" {
		t.Fatalf("large integer value = %q, %v", text, err)
	}

	text, err = configValueToString(0.9)
	if err != nil || text != "0.9" {
		t.Fatalf("decimal value = %q, %v", text, err)
	}

	text, err = configValueToString(map[string]any{"url": "/luna-devops-logo.svg"})
	if err != nil || text != `{"url":"/luna-devops-logo.svg"}` {
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

func TestConfigDefinitionResponseUsesI18nKeys(t *testing.T) {
	definition := configDefinitionResponse{
		Key:            "site.title",
		LabelKey:       "settings.configDefinitions.site.title.label",
		DescriptionKey: "settings.configDefinitions.site.title.description",
		Type:           "string",
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("marshal config definition: %v", err)
	}
	text := string(payload)
	if strings.Contains(text, `"label":`) || strings.Contains(text, `"description":`) {
		t.Fatalf("localized config text must not be returned by the backend: %s", text)
	}
	if !strings.Contains(text, `"labelKey":"settings.configDefinitions.site.title.label"`) {
		t.Fatalf("expected stable label key, got %s", text)
	}
}

func TestRetentionConfigDefinitionsAndBounds(t *testing.T) {
	expectedDefaults := map[string]string{
		"retention.platformEventsDays":         "90",
		"retention.notificationDeliveriesDays": "90",
		"retention.buildLogsDays":              "30",
		"retention.releaseLogsDays":            "90",
		"retention.hookRunLogsDays":            "90",
		"retention.expiredAuthDataDays":        "30",
	}
	for key, expectedDefault := range expectedDefaults {
		definition := configDefinitionByKey(key)
		if definition == nil {
			t.Fatalf("config definition %q not found", key)
		}
		if definition.Type != "number" || definition.Default != expectedDefault {
			t.Fatalf("config definition %q = type %q default %q", key, definition.Type, definition.Default)
		}
		for _, value := range []any{0, 3650, "90"} {
			if _, err := validateConfigValues(map[string]any{key: value}); err != nil {
				t.Fatalf("expected %q=%v to be valid: %v", key, value, err)
			}
		}
		for _, value := range []any{-1, 3651, "1.5", "invalid"} {
			if _, err := validateConfigValues(map[string]any{key: value}); err == nil {
				t.Fatalf("expected %q=%v to be rejected", key, value)
			}
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

	policy := h.egressPolicyForUser(model.User{Role: authz.PlatformRoleAdmin}, context.Background())
	if _, err := policy.ValidateURL("http://127.0.0.1:8080"); !errors.Is(err, security.ErrBlockedByPolicy) {
		t.Fatalf("expected default explicit block list to block loopback even for admin policy, got %v", err)
	}

	h.configs.values["security.egress.ipBlockList"] = ""
	policy = h.egressPolicyForUser(model.User{Role: authz.PlatformRoleAdmin}, context.Background())
	if _, err := policy.ValidateURL("http://127.0.0.1:8080"); err != nil {
		t.Fatalf("expected edited empty block list to allow admin private network access, got %v", err)
	}
}
