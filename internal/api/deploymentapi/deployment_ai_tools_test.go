package deploymentapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	runtimeapi "github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type aiToolsTestHost struct {
	Host
	db                   *gorm.DB
	secrets              secret.Store
	user                 model.User
	project              model.Project
	allowSecretViewValue bool
	queueAvailable       bool
	dispatched           []model.Release
	audits               []string
}

func (h *aiToolsTestHost) DBFor(ctx *gin.Context) *gorm.DB {
	if ctx != nil && ctx.Request != nil {
		return h.db.WithContext(ctx.Request.Context())
	}
	return h.db
}

func (h *aiToolsTestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return h.db.WithContext(ctx)
}

func (h *aiToolsTestHost) SecretStore() secret.Store {
	return h.secrets
}

func (h *aiToolsTestHost) AuthorizeProjectRef(_ *gin.Context, ref string, _ authz.Action) (model.User, model.Project, bool) {
	if ref == h.project.ID || ref == h.project.Identifier {
		return h.user, h.project, true
	}
	return model.User{}, model.Project{}, false
}

func (h *aiToolsTestHost) AuthorizeProjectByID(_ *gin.Context, projectID string, _ authz.Action) (model.User, model.Project, bool) {
	if projectID == h.project.ID {
		return h.user, h.project, true
	}
	return model.User{}, model.Project{}, false
}

func (h *aiToolsTestHost) EnsureProjectCanMutate(_ *gin.Context, project model.Project) bool {
	return model.DeleteStatusActive(project.DeleteStatus)
}

func (h *aiToolsTestHost) EnsureDeploymentTargetCanMutate(_ *gin.Context, target model.DeploymentTarget) bool {
	return model.DeleteStatusActive(target.DeleteStatus)
}

func (h *aiToolsTestHost) ResourceCanMutateDuringDelete(status string) bool {
	return model.DeleteStatusActive(status)
}

func (h *aiToolsTestHost) EnsureBillingAllowsDeployChange(_ *gin.Context, _ string) bool {
	return true
}

func (h *aiToolsTestHost) ProjectRoleActionAllowed(_ context.Context, _ model.User, _ string, action authz.Action) (bool, error) {
	if action == authz.ActionSecretViewValue {
		return h.allowSecretViewValue, nil
	}
	return true, nil
}

func (h *aiToolsTestHost) FindBuildEnvironmentConfig(db *gorm.DB, scope, scopeRef string) (model.BuildEnvironmentConfig, error) {
	var cfg model.BuildEnvironmentConfig
	err := db.First(&cfg, "scope = ? AND scope_ref = ?", scope, scopeRef).Error
	return cfg, err
}

func (h *aiToolsTestHost) AuditWithContext(_, action, resource string, _ bool, _ string, _ context.Context) {
	h.audits = append(h.audits, action+":"+resource)
}

func (h *aiToolsTestHost) EnqueueNetworkPolicyProjectReconcile(_ context.Context, _ string) bool {
	return true
}

func (h *aiToolsTestHost) EnqueueDeployRun(_ context.Context, release model.Release) bool {
	h.dispatched = append(h.dispatched, release)
	return h.queueAvailable
}

func (h *aiToolsTestHost) RuntimeClusterForDeploymentTargetValue(target model.DeploymentTarget, ctx context.Context) (model.RuntimeCluster, error) {
	var c model.RuntimeCluster
	err := h.db.WithContext(ctx).First(&c, "id = ?", target.ClusterID).Error
	return c, err
}

func (h *aiToolsTestHost) RuntimeClusterForProjectUse(_ *gin.Context, _ model.User, _ string, clusterID string) (model.RuntimeCluster, bool) {
	var c model.RuntimeCluster
	if err := h.db.First(&c, "id = ?", clusterID).Error; err != nil {
		return model.RuntimeCluster{}, false
	}
	return c, true
}

func (h *aiToolsTestHost) RuntimeClusterForDeploymentTarget(_ *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	var c model.RuntimeCluster
	if err := h.db.First(&c, "id = ?", target.ClusterID).Error; err != nil {
		return model.RuntimeCluster{}, false
	}
	return c, true
}

func (h *aiToolsTestHost) EnqueueResourceCleanup(_ context.Context, _, _, _, _ string) bool {
	return h.queueAvailable
}

func (h *aiToolsTestHost) ObserveGatewayRouteDiagnostics(_ context.Context, routes []model.GatewayRoute) []model.GatewayRoute {
	return routes
}

func (h *aiToolsTestHost) ApplyScopedResourceVisibilityForProject(query *gorm.DB, _ string, _ model.User, _ string, _ context.Context) *gorm.DB {
	return query
}

func (h *aiToolsTestHost) RegistryPushCredentialForProject(_ model.User, _ model.ArtifactRegistry, _ string, _ context.Context) (model.RegistryCredential, bool) {
	return model.RegistryCredential{}, false
}

func (h *aiToolsTestHost) ApplyDeploymentTargetRuntimeSecretsTx(
	_ context.Context,
	tx *gorm.DB,
	_ model.User,
	_, _, targetID string,
	input runtimeapi.RuntimeSecretMutationInput,
) (string, error) {
	var cur model.DeploymentTarget
	if err := tx.Select("env_vars", "secret_refs").First(&cur, "id = ?", targetID).Error; err != nil {
		return "", err
	}
	refs := decodeSecretRefs(cur.SecretRefs)
	if refs == nil {
		refs = map[string]string{}
	}
	for k, v := range input.Values {
		refs[k] = "secret-id:mock-" + k + "-" + v[:1]
	}
	for k := range input.Generate {
		refs[k] = "secret-id:gen-" + k
	}
	for _, k := range input.Clear {
		delete(refs, k)
	}
	keySet := map[string]struct{}{}
	for k := range refs {
		keySet[k] = struct{}{}
	}
	if len(runtimeapi.RuntimeEnvironmentKeyConflicts(cur.EnvVars, keySet)) > 0 {
		return "", errors.New("conflict")
	}
	encoded := encodeStringMap(refs)
	if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", targetID).UpdateColumn("secret_refs", encoded).Error; err != nil {
		return "", err
	}
	return encoded, nil
}

func TestRegisterAIToolsCatalogContract(t *testing.T) {
	reg := aitool.NewRegistry()
	handler := New(&aiToolsTestHost{})
	handler.RegisterAITools(reg)

	if got := reg.Len(); got != 18 {
		t.Fatalf("registered tool count = %d, want 18", got)
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}

	type expectedSpec struct {
		domain    string
		tier      string
		effect    string
		sensitive []string
	}
	expected := map[string]expectedSpec{
		"delivery_get_target":                 {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"delivery_list_releases":              {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"delivery_get_release":                {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"delivery_create_target":              {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"delivery_update_target_runtime":      {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"delivery_configure_target_config":    {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, sensitive: []string{"secretActions.*.value", "secretFileActions.*.content"}},
		"delivery_configure_target_volumes":   {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"delivery_delete_target":              {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync},
		"delivery_scale_target":               {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"delivery_deploy_target":              {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync},
		"delivery_rollback_target":            {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync},
		"delivery_restart_target":             {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectExecute},
		"delivery_cancel_release":             {domain: aitool.DomainDelivery, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"build_configure_target_pipeline":     {domain: aitool.DomainBuild, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
		"runtime_get_target_diagnostics":      {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"runtime_list_target_pods_and_events": {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"runtime_get_pod_logs":                {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"runtime_exec_command":                {domain: aitool.DomainRuntime, tier: aitool.ApprovalTierL2, effect: aitool.EffectExecute},
	}

	for name, exp := range expected {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing registered tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != exp.domain {
			t.Errorf("%s domain = %q, want %q", name, cat.Domain, exp.domain)
		}
		if cat.ApprovalTier != exp.tier {
			t.Errorf("%s approvalTier = %q, want %q", name, cat.ApprovalTier, exp.tier)
		}
		if cat.Effect != exp.effect {
			t.Errorf("%s effect = %q, want %q", name, cat.Effect, exp.effect)
		}
		if cat.AdminOnly || cat.GlobalSessionOnly {
			t.Errorf("%s unexpectedly marked adminOnly=%v globalSessionOnly=%v", name, cat.AdminOnly, cat.GlobalSessionOnly)
		}
		if !reflect.DeepEqual(cat.SensitivePaths, exp.sensitive) {
			t.Errorf("%s sensitivePaths = %v, want %v", name, cat.SensitivePaths, exp.sensitive)
		}
		if addProps, ok := cat.InputSchema["additionalProperties"].(bool); !ok || addProps {
			t.Errorf("%s InputSchema.additionalProperties = %v, want false", name, cat.InputSchema["additionalProperties"])
		}
		if !authz.UserCanAuthorizeOAuthScope("user", strings.Join(cat.RequiredScopes, ",")) {
			t.Errorf("%s requiredScopes %v rejected by UserCanAuthorizeOAuthScope", name, cat.RequiredScopes)
		}
	}
}

func TestTruncateUTF8TailAndHeadNeverSplitMultiByteRunes(t *testing.T) {
	// "你好世界🚀测试日志" contains 3-byte CJK and 4-byte emoji UTF-8 sequences
	sample := strings.Repeat("你好世界🚀测试日志\n", 100)
	for _, limit := range []int{7, 8, 9, 10, 11, 256, 511, 512, 513} {
		tail, tailTrunc, tailBytes := truncateUTF8Tail(sample, limit)
		if !tailTrunc || tailBytes <= 0 {
			t.Fatalf("truncateUTF8Tail(%d) expected truncated=true", limit)
		}
		if len(tail) > limit {
			t.Fatalf("truncateUTF8Tail(%d) len=%d exceeds limit", limit, len(tail))
		}
		if !utf8.ValidString(tail) {
			t.Fatalf("truncateUTF8Tail(%d) produced invalid UTF-8: %q", limit, tail)
		}

		head, headTrunc, headBytes := truncateUTF8Head(sample, limit)
		if !headTrunc || headBytes <= 0 {
			t.Fatalf("truncateUTF8Head(%d) expected truncated=true", limit)
		}
		if len(head) > limit {
			t.Fatalf("truncateUTF8Head(%d) len=%d exceeds limit", limit, len(head))
		}
		if !utf8.ValidString(head) {
			t.Fatalf("truncateUTF8Head(%d) produced invalid UTF-8: %q", limit, head)
		}
	}
}

func TestValidateRelativeRepoPathAndFilePathExtractors(t *testing.T) {
	validCases := []struct {
		raw      string
		allowDot bool
		want     string
	}{
		{"Dockerfile", false, "Dockerfile"},
		{"services/api/Dockerfile", false, "services/api/Dockerfile"},
		{".", true, "."},
		{"apps/web", true, "apps/web"},
	}
	for _, tc := range validCases {
		got, err := validateRelativeRepoPath(tc.raw, "path", tc.allowDot)
		if err != nil || got != tc.want {
			t.Fatalf("validateRelativeRepoPath(%q, %v) = (%q, %v), want (%q, nil)", tc.raw, tc.allowDot, got, err, tc.want)
		}
	}

	invalidCases := []struct {
		raw      string
		allowDot bool
	}{
		{"", true},
		{".", false},
		{"/etc/passwd", true},
		{"../Dockerfile", false},
		{"services/../../secret", true},
		{`services\api\Dockerfile`, false},
	}
	for _, tc := range invalidCases {
		if _, err := validateRelativeRepoPath(tc.raw, "path", tc.allowDot); err == nil {
			t.Fatalf("validateRelativeRepoPath(%q, %v) unexpectedly succeeded", tc.raw, tc.allowDot)
		}
	}

	// Verify extractAllFilePaths handles both ConfigFiles JSON array and SecretFiles JSON map
	cfgPaths := extractAllFilePaths(`[{"path":"/etc/app/a.yaml","content":"1"},{"path":"/etc/app/b.yaml","content":"2"}]`)
	if !reflect.DeepEqual(cfgPaths, []string{"/etc/app/a.yaml", "/etc/app/b.yaml"}) {
		t.Fatalf("cfgPaths = %v", cfgPaths)
	}
	secPaths := extractAllFilePaths(`{"/etc/app/z.key":"secret-id:1","/etc/app/a.crt":"secret-id:2"}`)
	if !reflect.DeepEqual(secPaths, []string{"/etc/app/a.crt", "/etc/app/z.key"}) {
		t.Fatalf("secPaths = %v", secPaths)
	}

	if !pathsOverlapOrNest("/etc/app", "/etc/app/config.yaml") || !pathsNestStrictly("/etc/app", "/etc/app/config.yaml") {
		t.Fatalf("expected parent-child nesting detected")
	}
	if pathsNestStrictly("/etc/app/config.yaml", "/etc/app/config.yaml") {
		t.Fatalf("same path must not be treated as strict parent-child nesting")
	}
}

func TestStrictArgumentsAndProbeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := aitool.NewRegistry()
	handler := New(&aiToolsTestHost{})
	handler.RegisterAITools(reg)

	// Unknown field is rejected before hitting DB
	tool, _ := reg.Get("delivery_create_target")
	_, err := tool.Preview(nil, json.RawMessage(`{"project":"p","application":"a","stage":"dev","unknownField":123}`))
	if te, ok := aitool.AsToolError(err); !ok || te.Status != 400 || te.Code != "ai.tool_arguments_invalid" {
		t.Fatalf("expected 400 ai.tool_arguments_invalid on unknown field, got %v", err)
	}

	parentRec := httptest.NewRecorder()
	parentGin, _ := gin.CreateTestContext(parentRec)
	parentGin.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	tc := aitool.NewToolContext(parentGin, nil, model.User{ID: "u1"}, "s1", "c1", "", "r1", "tc1", "", "model", nil)

	probeJSON, pErr := marshalAndValidateProbeSpec(tc, aiProbeSpec{
		HTTPGet: &aiHTTPGetAction{Path: "/healthz", Port: 8080, Scheme: "HTTP"},
	}, "probes.readiness")
	if pErr != nil || !strings.Contains(probeJSON, "/healthz") {
		t.Fatalf("valid HTTPGet probe failed: %q, %v", probeJSON, pErr)
	}

	// Reject probe specifying multiple handlers
	if _, badErr := marshalAndValidateProbeSpec(tc, aiProbeSpec{
		HTTPGet:   &aiHTTPGetAction{Path: "/healthz", Port: 8080},
		TCPSocket: &aiTCPSocketAction{Port: 8080},
	}, "probes.readiness"); badErr == nil {
		t.Fatalf("expected error when specifying both httpGet and tcpSocket")
	}
}

func TestDeliveryAndPipelineToolsPreserveExistingStateAndPreviewLocks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "deployment_ai_tools_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{},
				&model.Application{},
				&model.RuntimeCluster{},
				&model.RepositoryBinding{},
				&model.ArtifactRegistry{},
				&model.BuildVariableSet{},
				&model.ProjectHookConfig{},
				&model.DeploymentTarget{},
				&model.DeploymentTargetHookBinding{},
				&model.DeploymentVolumeMount{},
				&model.ProjectVolume{},
				&model.ProjectRuntimeConfigSet{},
				&model.BuildEnvironmentConfig{},
				&model.BuildRun{},
				&model.Release{},
				&model.ReleaseLog{},
				&model.GatewayRoute{},
				&model.ServiceBinding{},
				&model.SecretValue{},
			)
		},
	})

	codec, cErr := secret.NewCodec("0123456789abcdef0123456789abcdef")
	if cErr != nil {
		t.Fatalf("NewCodec: %v", cErr)
	}
	secretStore := secret.NewStore(db, nil, codec)
	project := model.Project{
		ID:                  "prj_ai1",
		Identifier:          "demo-proj",
		Name:                "Demo Project",
		KubernetesNamespace: "luna-demo",
		DeleteStatus:        "active",
		WebConsoleEnabled:   true,
	}
	app := model.Application{
		ID:           "app_ai1",
		ProjectID:    project.ID,
		Identifier:   "api-svc",
		Name:         "API Service",
		DeleteStatus: "active",
	}
	cluster1 := model.RuntimeCluster{ID: "clu_1", Name: "default-cluster", Scope: "global", IsDefault: true, DeleteStatus: "active"}
	cluster2 := model.RuntimeCluster{ID: "clu_2", Name: "secondary-cluster", Scope: "global", DeleteStatus: "active"}
	repoBinding := model.RepositoryBinding{ID: "rpb_1", ProjectID: project.ID, ApplicationID: app.ID, Owner: "org", Repo: "api"}
	registry := model.ArtifactRegistry{ID: "reg_1", Name: "main-reg", Provider: "harbor", Endpoint: "https://reg.example.com", Namespace: "team", Scope: "global", IsDefault: true}
	hookA := model.ProjectHookConfig{ID: "hook_a", ProjectID: project.ID, Name: "db-migrate", Shell: "/bin/sh", Script: "echo migrate"}
	target := model.DeploymentTarget{
		ID:                     "dplt_prod",
		ProjectID:              project.ID,
		ApplicationID:          app.ID,
		Name:                   "prod",
		Stage:                  "prod",
		KubernetesName:         "api-svc-prod",
		ClusterID:              cluster1.ID,
		WorkloadType:           "Deployment",
		Replicas:               2,
		CPURequest:             "500m",
		MemoryRequest:          "512Mi",
		Lifecycle:              `{"preStop":{"exec":{"command":["/bin/sh","-c","sleep 5"]}}}`,
		AutoScalingEnabled:     true,
		AutoScalingMinReplicas: 2,
		AutoScalingMaxReplicas: 6,
		AutoScalingCPUPercent:  75,
		AutoScalingBehavior:    `{"scaleDown":{"stabilizationWindowSeconds":120}}`,
		ServicePorts:           `[{"name":"http","port":8080}]`,
		SourceType:             "repository",
		RepositoryBindingID:    repoBinding.ID,
		DockerfilePath:         "Dockerfile",
		BuildContext:           ".",
		BuildEnvironmentID:     "benv_keep",
		TargetRegistryID:       registry.ID,
		TargetRepository:       "team/api-svc",
		TargetTag:              "prod",
		EnvVars:                "LOG_LEVEL=info\nPORT=8080",
		SecretRefs:             `{"DB_PASS":"secret-id:sec_1"}`,
		ConfigFiles:            `[{"path":"/etc/app/app.yaml","content":"env: prod"}]`,
		SecretFiles:            `{"/etc/app/tls.key":"secret-id:sec_tls"}`,
		Enabled:                true,
		DeleteStatus:           "active",
	}
	buildEnv := model.BuildEnvironmentConfig{
		ID:         "bec_1",
		Scope:      model.BuildEnvironmentScopeDeployment,
		ScopeRef:   target.ID,
		Variables:  buildenv.Encode(map[string]string{"NPM_REGISTRY": "https://npm.internal"}),
		SecretRefs: buildenv.Encode(map[string]string{"NPM_TOKEN": "secret-id:sec_npm"}),
		UpdatedBy:  "usr_1",
	}

	for _, rec := range []any{&project, &app, &cluster1, &cluster2, &repoBinding, &registry, &hookA, &target, &buildEnv} {
		if err := db.Create(rec).Error; err != nil {
			t.Fatalf("seed record: %v", err)
		}
	}

	host := &aiToolsTestHost{
		db:                   db,
		secrets:              secretStore,
		user:                 model.User{ID: "usr_1", Email: "alice@example.com", Name: "Alice", Role: "user"},
		project:              project,
		allowSecretViewValue: false,
		queueAvailable:       true,
	}
	handler := New(host)
	reg := aitool.NewRegistry()
	handler.RegisterAITools(reg)

	newTC := func(idempotencyKey string, lockedPayload any) *aitool.ToolContext {
		var rawLock json.RawMessage
		if lockedPayload != nil {
			rawLock, _ = json.Marshal(lockedPayload)
		}
		parentRec := httptest.NewRecorder()
		parentGin, _ := gin.CreateTestContext(parentRec)
		parentGin.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)
		return aitool.NewToolContext(parentGin, db, host.user, "sess_1", "conv_1", project.ID, "run_1", "call_1", idempotencyKey, "model", rawLock)
	}

	// 1. delivery_get_target gates deployment-level BuildEnvironmentConfig behind ActionSecretViewValue
	getTargetTool, _ := reg.Get("delivery_get_target")
	tcRead := newTC("", nil)
	rawOut, err := getTargetTool.Execute(tcRead, json.RawMessage(`{"project":"demo-proj","application":"api-svc","target":"prod"}`))
	if err != nil {
		t.Fatalf("delivery_get_target failed: %v", err)
	}
	if tcRead.ResponseHeaders.Get("Cache-Control") != "no-store" {
		t.Fatalf("expected Cache-Control: no-store, got %q", tcRead.ResponseHeaders.Get("Cache-Control"))
	}
	readOut := rawOut.(deliveryGetTargetOutput)
	if readOut.ProjectID != project.ID || readOut.ID != target.ID {
		t.Fatalf("unexpected IDs: %+v", readOut)
	}
	if !reflect.DeepEqual(readOut.SecretKeys, []string{"DB_PASS"}) || !reflect.DeepEqual(readOut.SecretFilePaths, []string{"/etc/app/tls.key"}) {
		t.Fatalf("unexpected secret keys/paths: %v / %v", readOut.SecretKeys, readOut.SecretFilePaths)
	}
	if readOut.BuildPipeline.CanInspectBuildEnvironment || readOut.BuildPipeline.BuildVariables != nil || readOut.BuildPipeline.BuildSecretKeys != nil {
		t.Fatalf("expected build environment details hidden without ActionSecretViewValue: %+v", readOut.BuildPipeline)
	}

	host.allowSecretViewValue = true
	rawOutAdmin, err := getTargetTool.Execute(newTC("", nil), json.RawMessage(`{"project":"demo-proj","application":"api-svc","target":"prod"}`))
	if err != nil {
		t.Fatalf("delivery_get_target (owner) failed: %v", err)
	}
	readOutAdmin := rawOutAdmin.(deliveryGetTargetOutput)
	if !readOutAdmin.BuildPipeline.CanInspectBuildEnvironment || readOutAdmin.BuildPipeline.BuildVariables["NPM_REGISTRY"] != "https://npm.internal" || !reflect.DeepEqual(readOutAdmin.BuildPipeline.BuildSecretKeys, []string{"NPM_TOKEN"}) {
		t.Fatalf("expected build environment details visible with ActionSecretViewValue: %+v", readOutAdmin.BuildPipeline)
	}

	// 2. delivery_update_target_runtime preserves unexposed columns & HPA thresholds when disabling HPA
	updateRuntimeTool, _ := reg.Get("delivery_update_target_runtime")
	updRes, err := updateRuntimeTool.Execute(newTC("", nil), json.RawMessage(`{
		"project":"demo-proj",
		"application":"api-svc",
		"target":"prod",
		"cpuRequest":"1",
		"autoScaling":{"enabled":false}
	}`))
	if err != nil {
		t.Fatalf("delivery_update_target_runtime failed: %v", err)
	}
	updTyped := updRes.(deliveryUpdateTargetRuntimeOutput)
	if !updTyped.RequiresRelease {
		t.Fatalf("expected requiresRelease=true when changing cpuRequest & autoScaling")
	}
	var afterRuntime model.DeploymentTarget
	if err := db.First(&afterRuntime, "id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if afterRuntime.Lifecycle != target.Lifecycle || afterRuntime.AutoScalingBehavior != target.AutoScalingBehavior || afterRuntime.AutoScalingMinReplicas != 2 || afterRuntime.AutoScalingMaxReplicas != 6 || afterRuntime.AutoScalingCPUPercent != 75 {
		t.Fatalf("pure patch wiped advanced or HPA threshold columns: %+v", afterRuntime)
	}
	if afterRuntime.EnvVars != target.EnvVars || afterRuntime.SecretRefs != target.SecretRefs {
		t.Fatalf("runtime spec patch unexpectedly modified env/secrets")
	}

	// Governance-only change returns requiresRelease=false
	govRes, err := updateRuntimeTool.Execute(newTC("", nil), json.RawMessage(`{
		"project":"demo-proj","application":"api-svc","target":"prod",
		"governance":{"requireApproval":true}
	}`))
	if err != nil {
		t.Fatalf("governance patch failed: %v", err)
	}
	if govRes.(deliveryUpdateTargetRuntimeOutput).RequiresRelease {
		t.Fatalf("expected requiresRelease=false for governance-only change")
	}

	// 3. delivery_configure_target_config redacts preview & rejects outbound ServiceBinding SecretMap collision
	if err := db.Create(&model.ServiceBinding{
		ID: "sbind_out", ProjectID: project.ID,
		SourceApplicationID: app.ID, SourceDeploymentTargetID: target.ID,
		TargetApplicationID: app.ID, TargetDeploymentTargetID: "dplt_other",
		TargetPortName: "http", TargetPort: 8080, Protocol: "http", InjectionMode: "url",
		URLEnvVar: "UPSTREAM_URL",
		SecretMap: model.EncodeSecretMap([]model.SecretMapping{{SourceEnvVar: "INJECTED_REDIS_PASS", TargetSecretKey: "REDIS_PASS"}}),
		Enabled:   true, CreatedBy: "usr_1",
	}).Error; err != nil {
		t.Fatal(err)
	}

	cfgTool, _ := reg.Get("delivery_configure_target_config")
	_, conflictErr := cfgTool.Preview(newTC("", nil), json.RawMessage(`{
		"project":"demo-proj","application":"api-svc","target":"prod",
		"setPublicEnv":[{"key":"INJECTED_REDIS_PASS","value":"oops"}]
	}`))
	if te, ok := aitool.AsToolError(conflictErr); !ok || te.Status != 409 || te.Code != "service_binding.env_conflict" {
		t.Fatalf("expected 409 service_binding.env_conflict, got %v", conflictErr)
	}

	prevCfg, err := cfgTool.Preview(newTC("", nil), json.RawMessage(`{
		"project":"demo-proj","application":"api-svc","target":"prod",
		"setPublicEnv":[{"key":"FEATURE_X","value":"on"}],
		"secretActions":[{"key":"API_KEY","operation":"set","value":"super-secret-value"}]
	}`))
	if err != nil {
		t.Fatalf("preview configure_target_config failed: %v", err)
	}
	if !slices.Contains(prevCfg.Preview.SensitiveKeys, "API_KEY") {
		t.Fatalf("expected API_KEY in SensitiveKeys, got %v", prevCfg.Preview.SensitiveKeys)
	}
	prevJSON, _ := json.Marshal(prevCfg.Preview)
	if strings.Contains(string(prevJSON), "super-secret-value") {
		t.Fatalf("ApprovalPreview leaked plaintext secret value: %s", string(prevJSON))
	}

	// 4. build_configure_target_pipeline preserves explicit RunOrder & rejects traversal / LITEYUKI_*
	pipeTool, _ := reg.Get("build_configure_target_pipeline")
	if _, pErr := pipeTool.Preview(newTC("", nil), json.RawMessage(`{
		"project":"demo-proj","application":"api-svc","target":"prod",
		"dockerfilePath":"../etc/passwd"
	}`)); pErr == nil {
		t.Fatalf("expected path traversal rejection")
	}
	if _, pErr := pipeTool.Preview(newTC("", nil), json.RawMessage(`{
		"project":"demo-proj","application":"api-svc","target":"prod",
		"upsertBuildArgs":[{"key":"LITEYUKI_INTERNAL","value":"1"}]
	}`)); pErr == nil {
		t.Fatalf("expected LITEYUKI_* reserved build arg rejection")
	}

	pipeOutRaw, err := pipeTool.Execute(newTC("", nil), json.RawMessage(`{
		"project":"demo-proj","application":"api-svc","target":"prod",
		"upsertHookBindings":[{"hook":"db-migrate","phase":"preDeployment","runOrder":42}]
	}`))
	if err != nil {
		t.Fatalf("build_configure_target_pipeline failed: %v", err)
	}
	pipeOut := pipeOutRaw.(buildConfigureTargetPipelineOutput)
	if pipeOut.RequiresRelease {
		t.Fatalf("expected requiresRelease=false when imageRef unchanged")
	}
	if len(pipeOut.BuildHookBindings) != 1 || pipeOut.BuildHookBindings[0].RunOrder != 42 {
		t.Fatalf("expected explicit runOrder=42 preserved, got %+v", pipeOut.BuildHookBindings)
	}
	var checkEnvID model.DeploymentTarget
	_ = db.First(&checkEnvID, "id = ?", target.ID).Error
	if checkEnvID.BuildEnvironmentID != "benv_keep" {
		t.Fatalf("BuildEnvironmentID was wiped: got %q", checkEnvID.BuildEnvironmentID)
	}

	// 5. delivery_deploy_target PreviewLock + Idempotency + cluster immutability after release
	if err := db.Create(&model.BuildRun{
		ID: "bldr_1", ProjectID: project.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		Status: "succeeded", ImageRef: "reg.example.com/team/api-svc:sha-abc123", CreatedBy: "usr_1",
	}).Error; err != nil {
		t.Fatal(err)
	}

	deployTool, _ := reg.Get("delivery_deploy_target")
	deployArgs := json.RawMessage(`{"project":"demo-proj","application":"api-svc","target":"prod"}`)
	deployPreview, err := deployTool.Preview(newTC("", nil), deployArgs)
	if err != nil {
		t.Fatalf("delivery_deploy_target preview failed: %v", err)
	}
	lockVal, ok := deployPreview.LockedPayload.(aiDeployPreviewLock)
	if !ok || lockVal.ResolvedBuildRunID != "bldr_1" || lockVal.ResolvedImageRef != "reg.example.com/team/api-svc:sha-abc123" {
		t.Fatalf("unexpected deploy preview lock: %#v", deployPreview.LockedPayload)
	}

	relOut1, err := deployTool.Execute(newTC("aiidem_deploy_0001", deployPreview.LockedPayload), deployArgs)
	if err != nil {
		t.Fatalf("delivery_deploy_target execute failed: %v", err)
	}
	rel1 := relOut1.(deliveryReleaseMutationOutput)
	if rel1.Revision != 1 || rel1.Status != "pending" {
		t.Fatalf("unexpected created release: %+v", rel1)
	}

	// Idempotent replay with same Idempotency-Key returns same release even while pending!
	relOut2, err := deployTool.Execute(newTC("aiidem_deploy_0001", deployPreview.LockedPayload), deployArgs)
	if err != nil {
		t.Fatalf("idempotent deploy replay failed: %v", err)
	}
	if relOut2.(deliveryReleaseMutationOutput).ID != rel1.ID {
		t.Fatalf("idempotent replay created duplicate release")
	}

	// Cluster switch on target with existing release must return 409 deployment_target.cluster_immutable_after_deploy!
	_, clusterSwitchErr := updateRuntimeTool.Preview(newTC("", nil), json.RawMessage(`{
		"project":"demo-proj","application":"api-svc","target":"prod","cluster":"secondary-cluster"
	}`))
	if te, ok := aitool.AsToolError(clusterSwitchErr); !ok || te.Status != 409 || te.Code != "deployment_target.cluster_immutable_after_deploy" {
		t.Fatalf("expected 409 deployment_target.cluster_immutable_after_deploy, got %v", clusterSwitchErr)
	}

	// 6. runtime_get_target_diagnostics degrades gracefully with metricsAvailable=false when cluster has no kubeconfig
	diagTool, _ := reg.Get("runtime_get_target_diagnostics")
	diagTC := newTC("", nil)
	diagRaw, err := diagTool.Execute(diagTC, json.RawMessage(`{"project":"demo-proj","application":"api-svc","target":"prod"}`))
	if err != nil {
		t.Fatalf("runtime_get_target_diagnostics failed: %v", err)
	}
	diagOut := diagRaw.(runtimeGetTargetDiagnosticsOutput)
	if diagOut.MetricsAvailable || diagTC.ResponseHeaders.Get("Cache-Control") != "no-store" {
		t.Fatalf("expected metricsAvailable=false and no-store, got %+v", diagOut)
	}

	// 7. runtime_exec_command returns actionable Hint when web console policy is disabled
	if err := db.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Update("web_console_policy", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	execTool, _ := reg.Get("runtime_exec_command")
	_, execDisabledErr := execTool.Preview(newTC("", nil), json.RawMessage(`{
		"project":"demo-proj","application":"api-svc","target":"prod","command":"env"
	}`))
	if te, ok := aitool.AsToolError(execDisabledErr); !ok || te.Status != 403 || te.Code != "runtime.web_console_disabled" || !strings.Contains(te.Hint, "runtime_get_target_diagnostics") {
		t.Fatalf("expected 403 runtime.web_console_disabled with Hint, got %+v (%v)", te, execDisabledErr)
	}
}
