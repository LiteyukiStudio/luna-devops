package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
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

func TestBootstrapStatusIncludesDevLoginHintInDevelopment(t *testing.T) {
	t.Setenv("LOCAL_ADMIN_EMAIL", "Admin@Example.com")
	t.Setenv("LOCAL_ADMIN_PASSWORD", "secret-password")

	status := bootstrapStatusResponse("development", true)

	if status["devLoginEnabled"] != true {
		t.Fatalf("expected dev login enabled in development, got %v", status["devLoginEnabled"])
	}
	hint, ok := status["devLoginHint"].(gin.H)
	if !ok {
		t.Fatalf("expected devLoginHint map, got %T", status["devLoginHint"])
	}
	if hint["email"] != "admin@example.com" {
		t.Fatalf("expected normalized dev email, got %q", hint["email"])
	}
	if hint["password"] != "secret-password" {
		t.Fatalf("expected configured dev password, got %q", hint["password"])
	}
}

func TestAuthProviderResponseHidesStoredClientSecret(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	provider := model.AuthProvider{ClientSecretRef: secret.Encrypt("super-secret")}

	output := authProviderResponse(provider)

	if output.ClientSecretRef != "" {
		t.Fatalf("expected stored client secret ref to be hidden, got %q", output.ClientSecretRef)
	}
	if !output.ClientSecretSet {
		t.Fatal("expected clientSecretSet to be true")
	}
}

func TestBuildVariableSetResponseHidesVariablesWithoutInspectPermission(t *testing.T) {
	h := &Handlers{}
	set := model.BuildVariableSet{
		ID:        "bvs_test",
		Scope:     "global",
		Variables: `{"PUBLIC_FLAG":"true","API_URL":"https://api.example.com"}`,
	}

	output := h.buildVariableSetResponseForUser(model.User{ID: "usr_member", Role: "user"}, set)

	if output.CanInspectVariables {
		t.Fatal("expected regular user to be unable to inspect global build variables")
	}
	if output.Variables != "{}" {
		t.Fatalf("expected variables to be hidden, got %q", output.Variables)
	}
	if output.VariableCount != 2 {
		t.Fatalf("expected variable count to remain visible, got %d", output.VariableCount)
	}
}

func TestBuildVariableSetResponseShowsVariablesWithInspectPermission(t *testing.T) {
	h := &Handlers{}
	set := model.BuildVariableSet{
		ID:        "bvs_test",
		Scope:     "user",
		OwnerRef:  "usr_owner",
		Variables: `{"PUBLIC_FLAG":"true"}`,
	}

	output := h.buildVariableSetResponseForUser(model.User{ID: "usr_owner", Role: "user"}, set)

	if !output.CanInspectVariables {
		t.Fatal("expected owner to inspect personal build variables")
	}
	if output.Variables != set.Variables {
		t.Fatalf("expected variables to be visible, got %q", output.Variables)
	}
	if output.VariableCount != 1 {
		t.Fatalf("expected variable count to be 1, got %d", output.VariableCount)
	}
}

func TestAuthProviderFromInputPreservesExistingSecret(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	existingSecretRef := secret.Encrypt("old-secret")
	provider, ok := authProviderFromInput(authProviderInput{
		Type:      "oidc",
		Name:      "Casdoor",
		IssuerURL: "https://sso.example.com",
		ClientID:  "devops",
	}, "ap_existing", existingSecretRef)

	if !ok {
		t.Fatal("expected auth provider input to be valid")
	}
	if provider.ClientSecretRef != existingSecretRef {
		t.Fatalf("expected existing secret ref to be preserved, got %q", provider.ClientSecretRef)
	}
}

func TestResolveSecretSupportsStoredAndEnvRefsOnly(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	t.Setenv("OIDC_TEST_SECRET", "env-secret")
	h := &Handlers{}

	if secret := h.resolveSecret(secret.Encrypt("stored-secret")); secret != "stored-secret" {
		t.Fatalf("expected stored secret, got %q", secret)
	}
	if secret := h.resolveSecret("literal:literal-secret"); secret != "" {
		t.Fatalf("expected literal secret ref to be rejected, got %q", secret)
	}
	if secret := h.resolveSecret("plain-secret"); secret != "" {
		t.Fatalf("expected bare secret ref to be rejected, got %q", secret)
	}
	if secret := h.resolveSecret("env:OIDC_TEST_SECRET"); secret != "env-secret" {
		t.Fatalf("expected env secret, got %q", secret)
	}
}

func TestAccessTokenUnknownRouteIsDenied(t *testing.T) {
	if accessTokenAllows("*", "system:unmapped") {
		t.Fatal("expected unmapped route to be denied even for wildcard legacy token")
	}
}

func TestEnqueueDeployRunPassesStablePayload(t *testing.T) {
	fake := &fakeBuildTaskEnqueuer{}
	h := &Handlers{taskClient: fake}
	release := model.Release{ID: "rel_1", ProjectID: "prj_1", CreatedBy: "usr_1"}

	if !h.enqueueDeployRun(context.Background(), release) {
		t.Fatal("expected enqueueDeployRun to succeed")
	}

	want := tasks.DeployRunPayload{
		ReleaseID: "rel_1",
		ProjectID: "prj_1",
		ActorID:   "usr_1",
	}
	if fake.deployPayload != want {
		t.Fatalf("payload = %#v", fake.deployPayload)
	}
}

func TestEnqueueGatewayApplyPassesStablePayload(t *testing.T) {
	fake := &fakeBuildTaskEnqueuer{}
	h := &Handlers{taskClient: fake}
	route := model.GatewayRoute{ID: "gwr_1", ProjectID: "prj_1", CreatedBy: "usr_1"}

	if !h.enqueueGatewayApply(context.Background(), route) {
		t.Fatal("expected enqueueGatewayApply to succeed")
	}

	want := tasks.GatewayApplyPayload{
		GatewayRouteID: "gwr_1",
		ProjectID:      "prj_1",
		ActorID:        "usr_1",
	}
	if fake.gatewayPayload != want {
		t.Fatalf("payload = %#v", fake.gatewayPayload)
	}
}

func TestRollbackReleaseFromTargetUsesPreviousSuccessfulRelease(t *testing.T) {
	source := model.Release{
		ID:            "rel_current",
		ProjectID:     "prj_1",
		ApplicationID: "app_1",
		EnvironmentID: "env_1",
		ImageRef:      "registry.example.com/acme/api:v3",
		Revision:      3,
	}
	target := model.Release{
		ID:       "rel_previous",
		ImageRef: "registry.example.com/acme/api:v2",
		Revision: 2,
	}

	release := rollbackReleaseFromTarget(source, target, "usr_1", 4)
	if release.ImageRef != target.ImageRef || release.RollbackFromID != target.ID {
		t.Fatalf("release = %#v", release)
	}
	if release.Type != "rollback" || release.Status != "pending" || release.Revision != 4 {
		t.Fatalf("rollback metadata = %#v", release)
	}
}

func TestDeploymentTargetMatchesBuildRunUsesTargetPatterns(t *testing.T) {
	run := model.BuildRun{SourceBranch: "main", SourceTag: "v1.2.3"}
	if !deploymentTargetMatchesBuildRun(model.DeploymentTarget{BranchPattern: "main", TagPattern: "v*"}, run) {
		t.Fatal("expected target patterns to match build run")
	}
	if deploymentTargetMatchesBuildRun(model.DeploymentTarget{BranchPattern: "release-*"}, run) {
		t.Fatal("expected unmatched target branch pattern to skip auto deploy")
	}
}

func TestFlattenKubeconfigEmbedsCertificateFiles(t *testing.T) {
	caFile := writeTempKubeconfigFile(t, "ca.crt", "ca-data")
	certFile := writeTempKubeconfigFile(t, "client.crt", "cert-data")
	keyFile := writeTempKubeconfigFile(t, "client.key", "key-data")
	input := `
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: https://127.0.0.1:6443
    certificate-authority: ` + caFile + `
users:
- name: local
  user:
    client-certificate: ` + certFile + `
    client-key: ` + keyFile + `
contexts:
- name: local
  context:
    cluster: local
    user: local
current-context: local
`

	output, err := flattenKubeconfig(input)
	if err != nil {
		t.Fatalf("flattenKubeconfig returned error: %v", err)
	}
	if strings.Contains(output, caFile) || strings.Contains(output, certFile) || strings.Contains(output, keyFile) {
		t.Fatalf("expected file paths to be removed, got %s", output)
	}
	if !strings.Contains(output, "certificate-authority-data") || !strings.Contains(output, "client-certificate-data") || !strings.Contains(output, "client-key-data") {
		t.Fatalf("expected certificate data to be embedded, got %s", output)
	}
}

func writeTempKubeconfigFile(t *testing.T, name string, content string) string {
	t.Helper()
	path := t.TempDir() + "/" + name
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

type fakeBuildTaskEnqueuer struct {
	buildPayload             tasks.BuildRunPayload
	deployPayload            tasks.DeployRunPayload
	gatewayPayload           tasks.GatewayApplyPayload
	applicationDeletePayload tasks.ApplicationDeletePayload
	resourceCleanupPayload   tasks.ResourceCleanupPayload
}

func (f *fakeBuildTaskEnqueuer) EnqueueBuildRun(_ context.Context, payload tasks.BuildRunPayload) (*asynq.TaskInfo, error) {
	f.buildPayload = payload
	return &asynq.TaskInfo{}, nil
}

func (f *fakeBuildTaskEnqueuer) EnqueueDeployRun(_ context.Context, payload tasks.DeployRunPayload) (*asynq.TaskInfo, error) {
	f.deployPayload = payload
	return &asynq.TaskInfo{}, nil
}

func (f *fakeBuildTaskEnqueuer) EnqueueGatewayApply(_ context.Context, payload tasks.GatewayApplyPayload) (*asynq.TaskInfo, error) {
	f.gatewayPayload = payload
	return &asynq.TaskInfo{}, nil
}

func (f *fakeBuildTaskEnqueuer) EnqueueApplicationDelete(_ context.Context, payload tasks.ApplicationDeletePayload) (*asynq.TaskInfo, error) {
	f.applicationDeletePayload = payload
	return &asynq.TaskInfo{}, nil
}

func (f *fakeBuildTaskEnqueuer) EnqueueResourceCleanup(_ context.Context, payload tasks.ResourceCleanupPayload) (*asynq.TaskInfo, error) {
	f.resourceCleanupPayload = payload
	return &asynq.TaskInfo{}, nil
}

func TestNormalizeAccessTokenScopeRejectsWildcardAndUnknownScopes(t *testing.T) {
	if scope := normalizeAccessTokenScope("*"); scope != "" {
		t.Fatalf("expected wildcard scope to be rejected, got %q", scope)
	}
	if scope := normalizeAccessTokenScope("project:read,git:read"); scope != "project:read,git:read" {
		t.Fatalf("expected normalized scope list, got %q", scope)
	}
	if scope := normalizeAccessTokenScope("billing:write"); scope != "billing:write" {
		t.Fatalf("expected billing write scope, got %q", scope)
	}
	if scope := normalizeAccessTokenScope("project:read,unknown:write"); scope != "" {
		t.Fatalf("expected unknown scope to be rejected, got %q", scope)
	}
}

func TestUserCannotCreateAdministrativeAccessTokenScope(t *testing.T) {
	user := model.User{Role: "user"}
	if userCanCreateAccessTokenScope(user, "user:manage") {
		t.Fatal("expected normal user to be blocked from user:manage")
	}
	if userCanCreateAccessTokenScope(user, "config:write") {
		t.Fatal("expected normal user to be blocked from config:write")
	}
	if userCanCreateAccessTokenScope(user, "project:write") {
		t.Fatal("expected normal user to be blocked from project:write without project role context")
	}
	if userCanCreateAccessTokenScope(user, "billing:write") {
		t.Fatal("expected normal user to be blocked from billing:write")
	}
	if !userCanCreateAccessTokenScope(user, "project:read,git:read") {
		t.Fatal("expected normal user to create read scopes")
	}
	if !userCanCreateAccessTokenScope(user, "billing:read") {
		t.Fatal("expected normal user to create billing:read")
	}
}

func TestRegistryResponseExposesCredentialSetOnly(t *testing.T) {
	output := registryResponse(model.ArtifactRegistry{CredentialRef: "regc_secret"})
	if !output.CredentialSet {
		t.Fatal("expected credentialSet to be true")
	}
}

func TestVerifyGitWebhookSignatureSupportsGitHubAndGiteaHeaders(t *testing.T) {
	body := []byte(`{"after":"abc"}`)
	signature := hmacSHA256Hex(body, "secret")

	headers := http.Header{}
	headers.Set("X-Hub-Signature-256", "sha256="+signature)
	if !verifyGitWebhookSignature(headers, body, "secret") {
		t.Fatal("expected GitHub webhook signature to verify")
	}

	headers = http.Header{}
	headers.Set("X-Gitea-Signature", signature)
	if !verifyGitWebhookSignature(headers, body, "secret") {
		t.Fatal("expected Gitea webhook signature to verify")
	}

	headers.Set("X-Gitea-Signature", "bad")
	if verifyGitWebhookSignature(headers, body, "secret") {
		t.Fatal("expected invalid webhook signature to fail")
	}
}

func TestGitUpstreamErrorStatusAndCodeMapsWebhookLocalhost(t *testing.T) {
	status, code := gitUpstreamErrorStatusAndCode(&gitprovider.UpstreamError{
		StatusCode: http.StatusUnprocessableEntity,
		Message:    "Validation Failed",
		Details: []gitprovider.UpstreamErrorDetail{{
			Resource: "Hook",
			Code:     "custom",
			Field:    "url",
			Message:  "url is not supported because it isn't reachable over the public Internet (localhost)",
		}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", status, http.StatusBadRequest)
	}
	if code != "git.webhook_callback_unreachable" {
		t.Fatalf("code = %q", code)
	}
}

func TestGitUpstreamErrorStatusAndCodeMapsNetworkFailure(t *testing.T) {
	status, code := gitUpstreamErrorStatusAndCode(errors.New("dial tcp: lookup github.com: no such host"))
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", status, http.StatusBadGateway)
	}
	if code != "git.network_failed" {
		t.Fatalf("code = %q", code)
	}
}

func TestGitWebhookCommitSHAReadsAfterField(t *testing.T) {
	sha := gitWebhookCommitSHA([]byte(`{"after":"abc123","sha":"ignored"}`))
	if sha != "abc123" {
		t.Fatalf("sha = %q", sha)
	}
}

func TestParseGitWebhookPushPayloadReadsBranchAndAuthor(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-GitHub-Event", "push")
	payload, ok := parseGitWebhookPushPayload(headers, []byte(`{
		"ref":"refs/heads/main",
		"after":"abc123",
		"pusher":{"name":"snowy","email":"snowy@example.com"},
		"head_commit":{"author":{"name":"Author","email":"author@example.com"}}
	}`))
	if !ok {
		t.Fatal("expected push payload to parse")
	}
	if payload.SourceBranch != "main" || payload.SourceTag != "" || payload.CommitSHA != "abc123" {
		t.Fatalf("payload source = %#v", payload)
	}
	if payload.TriggeredByName != "snowy" || payload.SourceAuthorEmail != "author@example.com" {
		t.Fatalf("payload actor = %#v", payload)
	}
}

func TestParseGitWebhookPushPayloadReadsTagAndDeletion(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Gitea-Event", "push")
	payload, ok := parseGitWebhookPushPayload(headers, []byte(`{
		"ref":"refs/tags/v1.0.0",
		"after":"0000000000000000000000000000000000000000"
	}`))
	if !ok {
		t.Fatal("expected tag push payload to parse")
	}
	if payload.SourceTag != "v1.0.0" || !payload.Deleted {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestPathEscapePathPreservesPathSegments(t *testing.T) {
	escaped := gitprovider.PathEscapePath("docs/app yaml.yml")
	if escaped != "docs/app%20yaml.yml" {
		t.Fatalf("escaped path = %q", escaped)
	}
}

func TestFilterGitRepositoriesMatchesNameAndFullName(t *testing.T) {
	repos := []gitprovider.Repository{
		{Name: "api", FullName: "liteyuki/api"},
		{Name: "web", FullName: "liteyuki/web"},
	}
	filtered := gitprovider.FilterRepositories(repos, "API")
	if len(filtered) != 1 || filtered[0].Name != "api" {
		t.Fatalf("filtered = %#v", filtered)
	}
}

func TestGitOAuthEndpointDefaultsGitHub(t *testing.T) {
	endpoint, err := gitprovider.OAuthEndpoint(model.GitProvider{Type: "github"})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.AuthURL != "https://github.com/login/oauth/authorize" {
		t.Fatalf("auth url = %q", endpoint.AuthURL)
	}
}

func TestSanitizeFrontendOrigin(t *testing.T) {
	defaultOrigin := "http://127.0.0.1:8080"

	if got := sanitizeFrontendOrigin("", defaultOrigin); got != defaultOrigin {
		t.Fatalf("empty origin = %q", got)
	}
	if got := sanitizeFrontendOrigin("mailto://bad.example", defaultOrigin); got != defaultOrigin {
		t.Fatalf("bad scheme origin = %q", got)
	}
	if got := sanitizeFrontendOrigin("http://127.0.0.1:5173", defaultOrigin); got != "http://127.0.0.1:5173" {
		t.Fatalf("same host origin = %q", got)
	}
	if got := sanitizeFrontendOrigin("http://localhost:5173", defaultOrigin); got != "http://localhost:5173" {
		t.Fatalf("loopback host origin = %q", got)
	}
	if got := sanitizeFrontendOrigin("http://evil.example", defaultOrigin); got != defaultOrigin {
		t.Fatalf("different host origin = %q", got)
	}
}

func TestBuildFrontendRedirect(t *testing.T) {
	defaultOrigin := "http://127.0.0.1:8080"
	redirectPath := "/code-repositories"
	accountID := "gita_xxx"
	target := buildFrontendRedirect(defaultOrigin, "http://127.0.0.1:5173", redirectPath, accountID)
	if target != "http://127.0.0.1:5173/code-repositories?gitAccountId=gita_xxx" {
		t.Fatalf("redirect = %q", target)
	}

	target = buildFrontendRedirect(defaultOrigin, "http://localhost:5173", "/projects?tab=apps", accountID)
	if target != "http://localhost:5173/projects?tab=apps&gitAccountId=gita_xxx" {
		t.Fatalf("redirect with query = %q", target)
	}
}

func TestGitOAuthCallbackURL(t *testing.T) {
	if got := gitOAuthCallbackURL("http://localhost:5173/"); got != "http://localhost:5173/api/v1/git/oauth/callback" {
		t.Fatalf("callback url = %q", got)
	}
}

func TestOIDCCallbackURL(t *testing.T) {
	if got := oidcCallbackURL("https://studio.example.com/"); got != "https://studio.example.com/api/v1/auth/oidc/callback" {
		t.Fatalf("callback url = %q", got)
	}
	if got := oidcCallbackURL(""); got != "" {
		t.Fatalf("empty callback url = %q", got)
	}
}

func TestOIDCAdmissionEmailHonorsVerifiedRequirement(t *testing.T) {
	claims := oidcIdentityClaims{Email: "USER@example.com", EmailVerified: false}
	if _, ok := oidcAdmissionEmail(claims, true); ok {
		t.Fatal("expected unverified email to be rejected when verification is required")
	}

	email, ok := oidcAdmissionEmail(claims, false)
	if !ok {
		t.Fatal("expected unverified email to be accepted when verification is optional")
	}
	if email != "user@example.com" {
		t.Fatalf("email = %q", email)
	}

	if _, ok := oidcAdmissionEmail(oidcIdentityClaims{}, false); ok {
		t.Fatal("expected empty email to be rejected")
	}
}

func TestGitExternalBaseURLPrefersPublicEnv(t *testing.T) {
	h := &Handlers{}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/git/oauth/start", nil)

	t.Setenv("PUBLIC_BASE_URL", "https://studio.example.com/")

	if got := h.externalBaseURL(ctx); got != "https://studio.example.com" {
		t.Fatalf("externalBaseURL = %q", got)
	}
}

func TestGitExternalBaseURLReturnsEmptyWhenNotConfigured(t *testing.T) {
	h := &Handlers{}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/git/oauth/start", nil)

	t.Setenv("PUBLIC_BASE_URL", "")

	if got := h.externalBaseURL(ctx); got != "" {
		t.Fatalf("externalBaseURL = %q", got)
	}
}

func TestConfiguredAllowedOriginsUsesPublicBaseAndEnv(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_BASE_URL", "https://studio.example.com/app")
	t.Setenv("APP_CORS_ORIGINS", "https://admin.example.com, https://studio.example.com")

	origins := configuredAllowedOrigins()
	if !containsString(origins, "https://studio.example.com") {
		t.Fatalf("expected PUBLIC_BASE_URL origin, got %#v", origins)
	}
	if !containsString(origins, "https://admin.example.com") {
		t.Fatalf("expected APP_CORS_ORIGINS origin, got %#v", origins)
	}
	if containsString(origins, "http://localhost:5173") {
		t.Fatalf("did not expect development origin in production, got %#v", origins)
	}
}

func TestConfiguredAllowedOriginsDefaultsToProductionMode(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	t.Setenv("APP_CORS_ORIGINS", "")

	origins := configuredAllowedOrigins()
	if containsString(origins, "http://localhost:5173") {
		t.Fatalf("did not expect development origin when APP_ENV is unset, got %#v", origins)
	}
	if containsString(origins, "http://127.0.0.1:5173") {
		t.Fatalf("did not expect development origin when APP_ENV is unset, got %#v", origins)
	}
}

func TestRequestOriginAllowedRejectsUntrustedOrigin(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/users/me", nil)
	ctx.Request.Header.Set("Origin", "https://evil.example.com")

	if requestOriginAllowed(ctx, []string{"https://studio.example.com"}) {
		t.Fatal("expected untrusted origin to be rejected")
	}

	ctx.Request.Header.Set("Origin", "https://studio.example.com")
	if !requestOriginAllowed(ctx, []string{"https://studio.example.com"}) {
		t.Fatal("expected trusted origin to be accepted")
	}
}

func TestRateLimiterUsesRedisOnly(t *testing.T) {
	limiter := newRateLimiter("localhost:6379")
	if limiter.redis == nil {
		t.Fatal("expected redis client")
	}
}

func TestWriteErrorCodeHidesDetailInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeError(ctx, http.StatusBadRequest, "duplicate key value violates unique constraint users_email_key")

	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "request.invalid" {
		t.Fatalf("code = %q", body["code"])
	}
	if body["error"] == "duplicate key value violates unique constraint users_email_key" || body["detail"] != "" {
		t.Fatalf("production response leaked detail: %#v", body)
	}
}

func TestWriteErrorCodeIncludesDetailInDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeError(ctx, http.StatusBadRequest, "validation detail")

	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "validation detail" || body["detail"] != "validation detail" {
		t.Fatalf("development response should include detail: %#v", body)
	}
}

func TestPersonalGitAccountIsOnlyUsableByOwner(t *testing.T) {
	h := &Handlers{}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	account := model.GitAccount{
		UserID:      "usr_owner",
		Scope:       "project",
		OwnerRef:    "prj_demo",
		AccessScope: "personal",
	}

	if !h.canUseGitAccount(ctx, model.User{ID: "usr_owner", Role: "user"}, account) {
		t.Fatal("expected owner to use personal Git account")
	}
	if h.canUseGitAccount(ctx, model.User{ID: "usr_other", Role: "user"}, account) {
		t.Fatal("expected another project member to be blocked from personal Git account")
	}
	if h.canUseGitAccount(ctx, model.User{ID: "usr_admin", Role: "platform_admin"}, account) {
		t.Fatal("expected platform admin to be blocked from using another user's personal Git account")
	}
}
