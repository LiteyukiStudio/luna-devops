package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

func TestValidateDeploymentBundleJSONRejectsDuplicateAndDeepValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		payload string
	}{
		{name: "duplicate nested key", payload: `{"bundle":{"kind":"first","kind":"second"}}`},
		{name: "additional root value", payload: `{"bundle":{}} {"bundle":{}}`},
		{name: "invalid json", payload: `{"bundle":`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validateDeploymentBundleJSON([]byte(test.payload)); err == nil {
				t.Fatal("validateDeploymentBundleJSON() accepted invalid input")
			}
		})
	}

	var deep strings.Builder
	for range deploymentBundleMaxDepth + 2 {
		deep.WriteByte('[')
	}
	deep.WriteString("null")
	for range deploymentBundleMaxDepth + 2 {
		deep.WriteByte(']')
	}
	if err := validateDeploymentBundleJSON([]byte(deep.String())); err == nil {
		t.Fatal("validateDeploymentBundleJSON() accepted excessive nesting")
	}
}

func TestBindDeploymentBundlePayloadDoesNotExposeDuplicateKey(t *testing.T) {
	t.Parallel()
	marker := "user-controlled-secret-key"
	ctx, recorder := deploymentBundleRequestContext(http.MethodPost, "/preview", []byte(`{"bundle":{"`+marker+`":1,"`+marker+`":2}}`), model.User{}, "", "")
	var request deploymentTargetBundleImportRequest
	if bindDeploymentBundleJSON(ctx, &request) {
		t.Fatal("bindDeploymentBundleJSON() accepted duplicate key")
	}
	if recorder.Code != http.StatusBadRequest || strings.Contains(recorder.Body.String(), marker) {
		t.Fatalf("duplicate-key response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDeploymentBundleErrorBoundaryUsesRegisteredSafeResponses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "forbidden reference", err: &deploymentBundleError{Code: "deployment_bundle.reference_forbidden", Message: "private-resource-id"}, wantStatus: http.StatusForbidden, wantCode: "deployment_bundle.reference_forbidden"},
		{name: "unknown dependency", err: errors.New("postgres-marker-secret-value"), wantStatus: http.StatusInternalServerError, wantCode: "deployment_bundle.internal_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := deploymentBundleRequestContext(http.MethodPost, "/preview", nil, model.User{}, "", "")
			writeDeploymentBundleError(ctx, test.err)
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
			}
			for _, forbidden := range []string{"private-resource-id", "postgres-marker-secret-value"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("response leaked %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestAuditResourceTypeUsesStableActionCategory(t *testing.T) {
	t.Parallel()
	if got := auditResourceType("deployment_bundle.import"); got != "deployment_bundle" {
		t.Fatalf("auditResourceType() = %q", got)
	}
	if got := auditResourceType("user-controlled.resource-id"); got != "unknown" {
		t.Fatalf("unknown auditResourceType() = %q", got)
	}
}

func TestAuditWriteFailureTelemetryOmitsResourceAndMessage(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	callbackName := "deployment_bundle_test:reject_audit"
	marker := "postgres-marker-private-diagnostic"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "AuditLog" {
			tx.AddError(errors.New(marker))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	resourceMarker := "dplt-private-resource-id"
	messageMarker := "sha256-private-bundle-digest"
	(&Handlers{db: db}).auditWithContext("usr_audit", "deployment_bundle.import", resourceMarker, false, messageMarker, context.Background())

	output := logs.String()
	for _, forbidden := range []string{resourceMarker, messageMarker, marker} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("audit failure telemetry leaked %q: %s", forbidden, output)
		}
	}
	for _, required := range []string{`"event.name":"audit.write.failed"`, `"audit.action":"deployment_bundle.import"`, `"resource.type":"deployment_bundle"`, `"error.type":"*errors.errorString"`} {
		if !strings.Contains(output, required) {
			t.Fatalf("audit failure telemetry missing %q: %s", required, output)
		}
	}
}

func TestDeploymentBundleConfigurationOmitsDestinationIdentifiersAndSecrets(t *testing.T) {
	t.Parallel()
	target := model.DeploymentTarget{
		ID: "dplt_source", ProjectID: "prj_source", ApplicationID: "app_source", EnvironmentID: "env_source",
		Name: "Production", Stage: "prod", KubernetesName: "source-prod", ClusterID: "cluster_source",
		SourceType: "repository", RepositoryBindingID: "binding_source", TargetRegistryID: "registry_source",
		BuildVariableSetIDs: `["vars_source"]`, RuntimeConfigSetIDs: `["runtime_source"]`,
		SecretRefs: `{"TOKEN":"secret-id:sec_runtime"}`, SecretFiles: `{"/run/key":"secret-id:sec_file"}`,
		Enabled: true,
	}
	mountID := "pvol_source"
	configuration, err := deploymentBundleConfiguration(target, []model.DeploymentVolumeMount{{
		LogicalName: "data", SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: &mountID,
		MountPath: deploymentBundleStringPointer("/data"),
	}}, model.BuildEnvironmentConfig{
		Variables:  buildenv.Encode(map[string]string{"LOG_LEVEL": "info"}),
		SecretRefs: buildenv.Encode(map[string]string{"TOKEN": "secret-id:sec_build"}),
	})
	if err != nil {
		t.Fatalf("deploymentBundleConfiguration() error = %v", err)
	}
	if configuration.EnvironmentID != "" || configuration.ClusterID != "" || configuration.RepositoryBindingID != "" || configuration.TargetRegistryID != "" {
		t.Fatalf("portable configuration contains destination identifiers: %#v", configuration)
	}
	if configuration.SecretFiles != "" || configuration.BuildSecrets != nil || configuration.Enabled {
		t.Fatalf("portable configuration contains secrets or enabled state: %#v", configuration)
	}
	if len(configuration.DataVolumes) != 1 || configuration.DataVolumes[0].ProjectVolumeID != "" || configuration.DataVolumes[0].MountPath != "/data" {
		t.Fatalf("portable volume = %#v", configuration.DataVolumes)
	}
	if configuration.BuildVariables == nil || (*configuration.BuildVariables)["LOG_LEVEL"] != "info" {
		t.Fatalf("portable build variables = %#v", configuration.BuildVariables)
	}
	payload, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"dplt_source", "prj_source", "app_source", "binding_source", "registry_source", "sec_runtime", "sec_file", "sec_build", "pvol_source"} {
		if bytes.Contains(payload, []byte(forbidden)) {
			t.Fatalf("portable configuration leaked %q: %s", forbidden, payload)
		}
	}
}

func TestValidateResolvedDeploymentBundleAllowsImageOnlyDestination(t *testing.T) {
	t.Parallel()
	if err := validateResolvedDeploymentBundle(deploymentTargetInput{SourceType: "image", ImageRef: "registry.example/app:v1"}, nil, nil); err != nil {
		t.Fatalf("image-only bundle should import into an empty application: %v", err)
	}
	err := validateResolvedDeploymentBundle(deploymentTargetInput{SourceType: "repository"}, nil, nil)
	if deploymentBundleErrorCode(err) != "deployment_bundle.repository_binding_missing" {
		t.Fatalf("repository bundle error code = %q", deploymentBundleErrorCode(err))
	}
}

func TestDeploymentBundleValidationRejectsEmbeddedIdentifiersAndSecrets(t *testing.T) {
	t.Parallel()
	base := deploymentTargetBundle{SchemaVersion: 1, Kind: deploymentBundleKind, Configuration: deploymentTargetInput{SourceType: "image", ImageRef: "example/app:v1"}}
	if err := validateDeploymentTargetBundle(base); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	base.Configuration.ClusterID = "cluster_source"
	if code := deploymentBundleErrorCode(validateDeploymentTargetBundle(base)); code != "deployment_bundle.invalid_json" {
		t.Fatalf("embedded identifier error code = %q", code)
	}
	base.Configuration.ClusterID = ""
	base.Configuration.SecretFiles = `[ {"path":"/run/token","content":"plaintext"} ]`
	if code := deploymentBundleErrorCode(validateDeploymentTargetBundle(base)); code != "deployment_bundle.invalid_json" {
		t.Fatalf("embedded secret error code = %q", code)
	}
}

func TestDeploymentBundleValidationRejectsDuplicateSecretDestinations(t *testing.T) {
	t.Parallel()
	bundle := deploymentTargetBundle{
		SchemaVersion: 1,
		Kind:          deploymentBundleKind,
		Configuration: deploymentTargetInput{SourceType: "image", ImageRef: "example/app:v1"},
		SecretRequirements: []deploymentBundleSecretRequirement{
			{Key: "secret:runtime:0", Target: deploymentBundleSecretRuntimeEnv, Name: "TOKEN"},
			{Key: "secret:runtime:1", Target: deploymentBundleSecretRuntimeEnv, Name: "TOKEN"},
		},
	}
	if code := deploymentBundleErrorCode(validateDeploymentTargetBundle(bundle)); code != "deployment_bundle.invalid_json" {
		t.Fatalf("duplicate secret destination error code = %q", code)
	}
}

func TestValidateResolvedDeploymentBundleRequiresProjectVolumeMapping(t *testing.T) {
	t.Parallel()
	input := deploymentTargetInput{
		SourceType: "image",
		ImageRef:   "example/app:v1",
		DataVolumes: []deploymentTargetDataVolumeInput{{
			LogicalName: "data", SourceType: "projectVolume", MountPath: "/data",
		}},
	}
	if code := deploymentBundleErrorCode(validateResolvedDeploymentBundle(input, nil, nil)); code != "deployment_bundle.reference_missing" {
		t.Fatalf("unmapped project volume error code = %q", code)
	}
}

func TestDeploymentBundleVolumeDestinationCompatibility(t *testing.T) {
	t.Parallel()
	input := deploymentTargetInput{ClusterID: "cluster_destination", Namespace: "app-space"}
	if !deploymentBundleVolumeDestinationCompatible(input, model.ProjectVolume{ClusterID: "cluster_destination", Namespace: "app-space"}) {
		t.Fatal("matching destination volume was rejected")
	}
	if deploymentBundleVolumeDestinationCompatible(input, model.ProjectVolume{ClusterID: "cluster_source", Namespace: "app-space"}) {
		t.Fatal("cross-cluster destination volume was accepted")
	}
	if deploymentBundleVolumeDestinationCompatible(input, model.ProjectVolume{ClusterID: "cluster_destination", Namespace: "other-space"}) {
		t.Fatal("cross-namespace destination volume was accepted")
	}
	if deploymentBundleVolumeDestinationCompatible(deploymentTargetInput{}, model.ProjectVolume{ClusterID: "cluster_destination"}) {
		t.Fatal("project volume without an explicit destination cluster was accepted")
	}
}

func TestBuildDeploymentTargetImportPlanRejectsOversizedSecretValue(t *testing.T) {
	t.Parallel()
	handlers := &Handlers{}
	request := deploymentTargetBundleImportRequest{
		Bundle: deploymentTargetBundle{
			SchemaVersion: 1,
			Kind:          deploymentBundleKind,
			Configuration: deploymentTargetInput{SourceType: "image", ImageRef: "example/app:v1", Stage: "*"},
			SecretRequirements: []deploymentBundleSecretRequirement{{
				Key: "secret:runtime:0", Target: deploymentBundleSecretRuntimeEnv, Name: "TOKEN",
			}},
		},
		Digest:       "placeholder",
		SecretValues: map[string]string{"secret:runtime:0": strings.Repeat("x", deploymentBundleSecretMaxBytes+1)},
	}
	digest, digestErr := deploymentBundleDigest(request.Bundle)
	if digestErr != nil {
		t.Fatal(digestErr)
	}
	request.Digest = digest
	_, err := handlers.buildDeploymentTargetImportPlan(nil, model.User{}, model.Project{}, model.Application{}, request, true)
	if code := deploymentBundleErrorCode(err); code != "deployment_bundle.secret_requirement_invalid" {
		t.Fatalf("oversized secret error code = %q", code)
	}
}

func TestBuildDeploymentTargetImportPlanRequiresPreviewDigestAndKnownKeys(t *testing.T) {
	t.Parallel()
	handlers := &Handlers{}
	bundle := deploymentTargetBundle{
		SchemaVersion: 1,
		Kind:          deploymentBundleKind,
		Configuration: deploymentTargetInput{SourceType: "image", ImageRef: "example/app:v1", Stage: "*"},
	}
	if _, err := handlers.buildDeploymentTargetImportPlan(nil, model.User{}, model.Project{}, model.Application{}, deploymentTargetBundleImportRequest{Bundle: bundle}, true); deploymentBundleErrorCode(err) != "deployment_bundle.digest_mismatch" {
		t.Fatalf("missing digest error code = %q", deploymentBundleErrorCode(err))
	}
	digest, err := deploymentBundleDigest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	request := deploymentTargetBundleImportRequest{Bundle: bundle, Digest: digest, Mappings: map[string]string{"unknown": "resource"}}
	if _, err := handlers.buildDeploymentTargetImportPlan(nil, model.User{}, model.Project{}, model.Application{}, request, true); deploymentBundleErrorCode(err) != "deployment_bundle.invalid_json" {
		t.Fatalf("unknown mapping error code = %q", deploymentBundleErrorCode(err))
	}
	request.Mappings = nil
	request.SecretValues = map[string]string{"unknown": "value"}
	if _, err := handlers.buildDeploymentTargetImportPlan(nil, model.User{}, model.Project{}, model.Application{}, request, true); deploymentBundleErrorCode(err) != "deployment_bundle.secret_requirement_invalid" {
		t.Fatalf("unknown secret error code = %q", deploymentBundleErrorCode(err))
	}
}

func TestDeploymentBundleFilenamePart(t *testing.T) {
	t.Parallel()
	if got := deploymentBundleFilenamePart(" Prod\r\nInjected.JSON "); got != "prodinjectedjson" {
		t.Fatalf("deploymentBundleFilenamePart() = %q", got)
	}
}

func TestNormalizeDeploymentBundleCandidateQuery(t *testing.T) {
	t.Parallel()
	query := normalizeDeploymentBundleCandidateQuery(deploymentBundleCandidateQuery{
		Pagination: paginationParams{Page: -1, PageSize: 1000, SortBy: "unsafe", SortOrder: "sideways"},
	})
	if query.Pagination.Page != 1 || query.Pagination.PageSize != maxPageSize || query.Pagination.SortBy != "name" || query.Pagination.SortOrder != "asc" {
		t.Fatalf("normalized query = %#v", query)
	}
}

func TestDeploymentBundleCompatibleMatchesIgnoreIncompatibleCandidates(t *testing.T) {
	t.Parallel()
	candidates := []deploymentBundleCandidate{
		{Public: deploymentBundleReferenceCandidate{ID: "incompatible", Matched: true, Compatible: false}},
		{Public: deploymentBundleReferenceCandidate{ID: "compatible", Matched: true, Compatible: true}},
		{Public: deploymentBundleReferenceCandidate{ID: "not-matched", Matched: false, Compatible: true}},
	}
	matches := appendCompatibleDeploymentBundleMatches(nil, candidates)
	if len(matches) != 1 || matches[0].Public.ID != "compatible" {
		t.Fatalf("compatible matches = %#v", matches)
	}
	source := deploymentBundleReferenceDescriptor{Name: "data", AccessMode: "ReadWriteOnce", VolumeMode: "Filesystem", StorageClassName: "source-class"}
	candidate := deploymentBundleReferenceDescriptor{Name: "data", AccessMode: "ReadWriteOnce", VolumeMode: "Filesystem", StorageClassName: "destination-class"}
	if !deploymentBundleReferenceDescriptorMatches(source, candidate) {
		t.Fatal("storage class must not change canonical descriptor matching semantics")
	}
}

func TestDeploymentBundleCandidateOpenAPIContract(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	path := paths["/api/v1/projects/{projectId}/applications/{applicationId}/deployment-target-imports/reference-candidates"].(map[string]any)
	operation := path["post"].(map[string]any)
	if operation["operationId"] != "listDeploymentTargetBundleReferenceCandidates" {
		t.Fatalf("candidate operationId = %#v", operation["operationId"])
	}
	parameters := operation["parameters"].([]any)
	seen := map[string]map[string]any{}
	for _, raw := range parameters {
		parameter := raw.(map[string]any)
		if name, ok := parameter["name"].(string); ok {
			seen[name] = parameter["schema"].(map[string]any)
		}
	}
	if seen["pageSize"]["maximum"] != float64(100) {
		t.Fatalf("candidate pageSize = %#v", seen["pageSize"])
	}
	for _, name := range []string{"page", "pageSize", "search", "sortBy", "sortOrder"} {
		if seen[name] == nil {
			t.Fatalf("candidate query is missing %s", name)
		}
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	pageSchema := schemas["DeploymentBundleReferenceCandidatePage"].(map[string]any)
	required, ok := schemaStringList(pageSchema["required"])
	if !ok || strings.Join(required, ",") != "items,page,pageSize,sortBy,sortOrder,total,totalPages" {
		t.Fatalf("candidate page required = %#v", pageSchema["required"])
	}
	errorCodes := schemas["DeploymentBundleErrorCode"].(map[string]any)["enum"].([]any)
	registered := make(map[string]bool, len(errorCodes))
	for _, rawCode := range errorCodes {
		code := rawCode.(string)
		if _, ok := deploymentBundleErrorSpecFor(code); !ok {
			t.Fatalf("OpenAPI deployment bundle error code %q is not registered", code)
		}
		registered[code] = true
	}
	for code := range deploymentBundleErrorCatalog {
		if !registered[code] {
			t.Fatalf("registered deployment bundle error code %q is missing from OpenAPI", code)
		}
	}
}

func TestDeploymentBundleCandidatePaginationAndScopedMapping(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(&model.Application{}, &model.ProjectRuntimeConfigSet{}, &model.RuntimeCluster{}, &model.GitProvider{}, &model.RepositoryBinding{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate candidate integration schema: %v", err)
	}
	user := model.User{ID: "usr_candidate", Email: "candidate@example.test", Name: "Candidate User", Role: authz.PlatformRoleUser, Language: "en-US"}
	project := model.Project{ID: "prj_candidate", Identifier: "candidate-project", Name: "Candidate Project", NamespaceStrategy: "project", DeleteStatus: "active"}
	otherProject := model.Project{ID: "prj_candidate_other", Identifier: "candidate-other", Name: "Other Project", NamespaceStrategy: "project", DeleteStatus: "active"}
	app := model.Application{ID: "app_candidate", ProjectID: project.ID, Identifier: "candidate-app", Name: "Candidate App", DeleteStatus: "active"}
	member := model.ProjectMember{ID: "pm_candidate", ProjectID: project.ID, UserID: user.ID, Role: authz.ProjectRoleDeveloper}
	for _, value := range []any{&user, &project, &otherProject, &app, &member} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	sets := make([]model.ProjectRuntimeConfigSet, 0, 150)
	for index := 1; index <= 150; index++ {
		sets = append(sets, model.ProjectRuntimeConfigSet{
			ID: "prcs_candidate_" + fmt.Sprintf("%03d", index), ProjectID: project.ID,
			Name: fmt.Sprintf("Config %03d", index), Enabled: true, DeleteStatus: "active", CreatedBy: user.ID,
		})
	}
	if err := db.CreateInBatches(sets, 50).Error; err != nil {
		t.Fatal(err)
	}
	otherSet := model.ProjectRuntimeConfigSet{ID: "prcs_candidate_other", ProjectID: otherProject.ID, Name: "Config 120", Enabled: true, DeleteStatus: "active", CreatedBy: user.ID}
	if err := db.Create(&otherSet).Error; err != nil {
		t.Fatal(err)
	}
	wrongType := model.RuntimeCluster{ID: "clu_candidate_wrong_type", Name: "Config 120", Type: "kubernetes", Scope: "global", CreatedBy: user.ID}
	if err := db.Create(&wrongType).Error; err != nil {
		t.Fatal(err)
	}
	provider := model.GitProvider{ID: "gp_candidate", Name: "GitHub", Type: "github", Scope: "global", Enabled: true}
	binding := model.RepositoryBinding{ID: "rpb_candidate", ProjectID: project.ID, ApplicationID: app.ID, GitProviderID: provider.ID, GitAccountID: "ga_candidate", Owner: "foo", Repo: "bar", CloneURL: "https://example.test/foo/bar.git"}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}

	handlers := &Handlers{db: db, mode: "production"}
	reference := deploymentBundleReference{Key: "runtimeConfigSet:0", Kind: deploymentBundleReferenceRuntimeConfigSet, Required: true, Usage: "runtimeConfig", Source: deploymentBundleReferenceDescriptor{Name: "Config 120"}}
	page, candidates, err := handlers.deploymentBundleCandidates(context.Background(), user, project, app, reference, deploymentBundleCandidateQuery{
		Pagination: paginationParams{Page: 6, PageSize: 20, SortBy: "name", SortOrder: "asc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 150 || page.TotalPages != 8 || len(candidates) != 20 || candidates[0].Public.Name != "Config 101" || candidates[19].Public.Name != "Config 120" {
		t.Fatalf("candidate page = %#v, first/last = %#v/%#v", page, candidates[0].Public, candidates[len(candidates)-1].Public)
	}
	searchPage, searchCandidates, err := handlers.deploymentBundleCandidates(context.Background(), user, project, app, reference, deploymentBundleCandidateQuery{
		Pagination: paginationParams{Page: 1, PageSize: 20, SortBy: "createdAt", SortOrder: "desc"}, Search: "120",
	})
	if err != nil || searchPage.Total != 1 || len(searchCandidates) != 1 || searchCandidates[0].Public.ID != "prcs_candidate_120" {
		t.Fatalf("search page = %#v candidates = %#v err = %v", searchPage, searchCandidates, err)
	}
	repositoryReference := deploymentBundleReference{Key: "repository", Kind: deploymentBundleReferenceRepositoryBinding, Required: true, Usage: "source", Source: deploymentBundleReferenceDescriptor{Name: "foo/bar", Owner: "foo", Repository: "bar", Type: "github"}}
	repositoryPage, repositoryCandidates, err := handlers.deploymentBundleCandidates(context.Background(), user, project, app, repositoryReference, deploymentBundleCandidateQuery{
		Pagination: paginationParams{Page: 1, PageSize: 20, SortBy: "name", SortOrder: "asc"}, Search: "foo/bar",
	})
	if err != nil || repositoryPage.Total != 1 || len(repositoryCandidates) != 1 || repositoryCandidates[0].Public.Name != "foo/bar" {
		t.Fatalf("repository display-name search page = %#v candidates = %#v err = %v", repositoryPage, repositoryCandidates, err)
	}

	requestContext, _ := deploymentBundleRequestContext(http.MethodPost, "/preview", nil, user, project.ID, app.ID)
	resolution, _, err := handlers.resolveDeploymentBundleReference(requestContext, user, project, app, reference, "prcs_candidate_120")
	if err != nil || resolution.Status != deploymentBundleReferenceResolved || resolution.ResolvedID != "prcs_candidate_120" {
		t.Fatalf("120th mapping resolution = %#v err = %v", resolution, err)
	}
	for name, mappedID := range map[string]string{"cross project": otherSet.ID, "not found": "prcs_missing", "wrong type": wrongType.ID} {
		resolution, _, err = handlers.resolveDeploymentBundleReference(requestContext, user, project, app, reference, mappedID)
		if err != nil || resolution.Status != deploymentBundleReferenceForbidden || resolution.Code != "deployment_bundle.reference_forbidden" {
			t.Fatalf("%s resolution = %#v err = %v", name, resolution, err)
		}
	}

	payload, err := json.Marshal(deploymentBundleReferenceCandidatesRequest{Reference: reference})
	if err != nil {
		t.Fatal(err)
	}
	handlerContext, recorder := deploymentBundleRequestContext(http.MethodPost, "/reference-candidates?page=2&pageSize=50&search=Config&sortBy=name&sortOrder=asc", payload, user, project.ID, app.ID)
	handlers.ListDeploymentTargetBundleReferenceCandidates(handlerContext)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("candidate handler status/header = %d/%q body=%s", recorder.Code, recorder.Header().Get("Cache-Control"), recorder.Body.String())
	}
	var response deploymentBundleReferenceCandidatePage
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 150 || response.Page != 2 || response.PageSize != 50 || response.TotalPages != 3 || len(response.Items) != 50 {
		t.Fatalf("candidate response = %#v", response)
	}
	var candidateAudits int64
	if err := db.Model(&model.AuditLog{}).Where("action = ?", "deployment_bundle.reference_candidates").Count(&candidateAudits).Error; err != nil {
		t.Fatal(err)
	}
	if candidateAudits != 0 {
		t.Fatalf("candidate pagination wrote %d audit rows", candidateAudits)
	}
}

func TestDeploymentBundleUnknownDatabaseFailureIsSafeInResponseAuditAndSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})
	parentContext, parentSpan := provider.Tracer("deployment-bundle-db-failure-test").Start(context.Background(), "test.request")

	db := authIntegrationDB(t)
	if err := db.AutoMigrate(&model.Application{}, &model.DeploymentTarget{}, &model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "usr_bundle_db_failure", Email: "bundle-db-failure@example.test", Role: authz.PlatformRoleUser}
	project := model.Project{ID: "prj_bundle_db_failure", Identifier: "bundle-db-failure", Name: "Bundle DB Failure", NamespaceStrategy: "project", DeleteStatus: "active"}
	app := model.Application{ID: "app_bundle_db_failure", ProjectID: project.ID, Identifier: "bundle-db-failure", Name: "Bundle DB Failure", DeleteStatus: "active"}
	member := model.ProjectMember{ID: "pm_bundle_db_failure", ProjectID: project.ID, UserID: user.ID, Role: authz.ProjectRoleDeveloper}
	for _, value := range []any{&user, &project, &app, &member} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	marker := "postgres-marker-private-diagnostic"
	callbackName := "deployment_bundle_test:fail_target_query"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "DeploymentTarget" {
			tx.AddError(errors.New(marker))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	bundle := deploymentTargetBundle{SchemaVersion: 1, Kind: deploymentBundleKind, ExportedAt: time.Now().UTC(), Configuration: deploymentTargetInput{Name: "Safe failure", Stage: "dev", SourceType: "image", ImageRef: "example.test/safe:v1"}, References: []deploymentBundleReference{}, SecretRequirements: []deploymentBundleSecretRequirement{}, Omissions: []string{}}
	payload, err := json.Marshal(deploymentTargetBundleImportRequest{Bundle: bundle})
	if err != nil {
		t.Fatal(err)
	}
	ctx, response := deploymentBundleRequestContext(http.MethodPost, "/preview", payload, user, project.ID, app.ID)
	ctx.Request = ctx.Request.WithContext(parentContext)
	(&Handlers{db: db, mode: "production"}).PreviewDeploymentTargetBundleImport(ctx)
	parentSpan.End()
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "deployment_bundle.internal_error") || strings.Contains(response.Body.String(), marker) {
		t.Fatalf("database failure response = %d %s", response.Code, response.Body.String())
	}
	var audit model.AuditLog
	if err := db.Where("action = ?", "deployment_bundle.preview").First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.Message != "deployment_bundle.internal_error" || strings.Contains(audit.Message, marker) {
		t.Fatalf("database failure audit = %#v", audit)
	}
	for _, span := range recorder.Ended() {
		if span.Name() != "deployment.bundle_preview" {
			continue
		}
		if span.Status().Code != codes.Error {
			t.Fatalf("database failure span status = %#v", span.Status())
		}
		for _, event := range span.Events() {
			for _, attr := range event.Attributes {
				if strings.Contains(attr.Value.AsString(), marker) {
					t.Fatalf("database failure span leaked marker: %#v", span.Events())
				}
			}
		}
		return
	}
	t.Fatal("database failure span not found")
}

func TestDeploymentBundleExportFailureDoesNotExposeDatabaseDiagnostic(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(&model.Application{}, &model.DeploymentTarget{}, &model.DeploymentTargetHookBinding{}, &model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "usr_bundle_export_failure", Email: "bundle-export-failure@example.test", Role: authz.PlatformRoleUser}
	project := model.Project{ID: "prj_bundle_export_failure", Identifier: "bundle-export-failure", Name: "Bundle Export Failure", NamespaceStrategy: "project", DeleteStatus: "active"}
	app := model.Application{ID: "app_bundle_export_failure", ProjectID: project.ID, Identifier: "bundle-export-failure", Name: "Bundle Export Failure", DeleteStatus: "active"}
	member := model.ProjectMember{ID: "pm_bundle_export_failure", ProjectID: project.ID, UserID: user.ID, Role: authz.ProjectRoleDeveloper}
	target := model.DeploymentTarget{ID: "dplt_bundle_export_failure", ProjectID: project.ID, ApplicationID: app.ID, Name: "Export failure", Stage: "qa", SourceType: "image", ImageRef: "example.test/export:v1"}
	for _, value := range []any{&user, &project, &app, &member, &target} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	marker := "gorm-marker-private-export-diagnostic"
	callbackName := "deployment_bundle_test:fail_hook_binding_query"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "DeploymentTargetHookBinding" {
			tx.AddError(errors.New(marker))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	ctx, response := deploymentBundleRequestContext(http.MethodGet, "/export", nil, user, project.ID, app.ID)
	ctx.Params = append(ctx.Params, gin.Param{Key: "targetId", Value: target.ID})
	(&Handlers{db: db, mode: "production"}).ExportDeploymentTargetBundle(ctx)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "deployment_bundle.export_failed") || strings.Contains(response.Body.String(), marker) {
		t.Fatalf("export failure response = %d %s", response.Code, response.Body.String())
	}
	var audit model.AuditLog
	if err := db.Where("action = ? and resource = ?", "deployment_bundle.export", target.ID).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.Message != "deployment_bundle.export_failed" || strings.Contains(audit.Message, marker) {
		t.Fatalf("export failure audit = %#v", audit)
	}
}

func TestDeploymentBundlePreviewAndImportCreatesOnlyDeploymentTarget(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})
	parentContext, parentSpan := provider.Tracer("deployment-bundle-integration-test").Start(context.Background(), "test.request")
	defer parentSpan.End()

	db := authIntegrationDB(t)
	if err := db.AutoMigrate(
		&model.Application{},
		&model.DeploymentTarget{},
		&model.DeploymentVolumeMount{},
		&model.DeploymentTargetHookBinding{},
		&model.ProjectHookConfig{},
		&model.BuildEnvironmentConfig{},
		&model.SecretValue{},
		&model.AuditLog{},
		&model.BuildRun{},
		&model.Release{},
	); err != nil {
		t.Fatalf("migrate deployment bundle integration schema: %v", err)
	}

	user := model.User{ID: "usr_bundle", Email: "bundle@example.test", Name: "Bundle User", Role: authz.PlatformRoleUser, Language: "en-US"}
	project := model.Project{ID: "prj_bundle", Identifier: "bundle-project", Name: "Bundle Project", NamespaceStrategy: "project", DeleteStatus: "active"}
	app := model.Application{ID: "app_bundle", ProjectID: project.ID, Identifier: "empty-app", Name: "Empty App", DeleteStatus: "active"}
	member := model.ProjectMember{ID: "pm_bundle", ProjectID: project.ID, UserID: user.ID, Role: authz.ProjectRoleDeveloper}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&app).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	hook := model.ProjectHookConfig{ID: "hook_bundle", ProjectID: project.ID, Name: "Verify bundle", Script: "true", Shell: "sh", CreatedBy: user.ID}
	if err := db.Create(&hook).Error; err != nil {
		t.Fatal(err)
	}

	handlers := &Handlers{
		db: db,
		configs: &configCache{values: map[string]string{
			"billing.blockDeployChangesWhenInsufficient": "false",
		}},
		mode: "production",
	}
	bundle := deploymentTargetBundle{
		SchemaVersion: deploymentBundleSchemaVersion,
		Kind:          deploymentBundleKind,
		ExportedAt:    time.Now().UTC(),
		Configuration: deploymentTargetInput{
			Name: "Imported image", Stage: "production", SourceType: "image", ImageRef: "registry.example.test/service:v1",
		},
		References: []deploymentBundleReference{{
			Key: "hookConfig:0", Kind: deploymentBundleReferenceHookConfig, Required: true, Usage: "buildHook",
			Source: deploymentBundleReferenceDescriptor{Name: hook.Name, Type: hook.Shell, Phase: "preBuild", RunOrder: 7},
		}},
		SecretRequirements: []deploymentBundleSecretRequirement{},
		Omissions:          []string{"runtimeState", "history", "secretValues"},
	}
	buildVariables := map[string]string{"PUBLIC_VALUE": "safe"}
	invalidBundle := bundle
	invalidBundle.Configuration.Stage = "qa"
	invalidBundle.Configuration.BuildVariables = &buildVariables
	invalidBundle.Configuration.DataVolumes = []deploymentTargetDataVolumeInput{{LogicalName: "cache", SourceType: "emptyDir", MountPath: "/cache"}}
	invalidBundle.SecretRequirements = []deploymentBundleSecretRequirement{{Key: "secret:runtime:0", Target: deploymentBundleSecretRuntimeEnv, Name: "TOKEN"}}
	invalidPreviewPayload, err := json.Marshal(deploymentTargetBundleImportRequest{Bundle: invalidBundle})
	if err != nil {
		t.Fatal(err)
	}
	invalidPreviewCtx, invalidPreviewRecorder := deploymentBundleRequestContext(http.MethodPost, "/preview", invalidPreviewPayload, user, project.ID, app.ID)
	invalidPreviewCtx.Request = invalidPreviewCtx.Request.WithContext(parentContext)
	handlers.PreviewDeploymentTargetBundleImport(invalidPreviewCtx)
	if invalidPreviewRecorder.Code != http.StatusOK {
		t.Fatalf("invalid-stage preview status = %d body=%s", invalidPreviewRecorder.Code, invalidPreviewRecorder.Body.String())
	}
	var invalidPreview deploymentTargetBundlePreview
	if err := json.Unmarshal(invalidPreviewRecorder.Body.Bytes(), &invalidPreview); err != nil {
		t.Fatal(err)
	}
	if invalidPreview.Status != deploymentBundleStatusInvalid || !containsString(invalidPreview.Warnings, "deployment.stage_invalid") {
		t.Fatalf("invalid-stage preview = %#v", invalidPreview)
	}
	invalidImportPayload, err := json.Marshal(deploymentTargetBundleImportRequest{
		Bundle: invalidBundle, Digest: invalidPreview.Digest,
		SecretValues: map[string]string{"secret:runtime:0": "must-not-persist"},
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidImportCtx, invalidImportRecorder := deploymentBundleRequestContext(http.MethodPost, "/imports", invalidImportPayload, user, project.ID, app.ID)
	invalidImportCtx.Request = invalidImportCtx.Request.WithContext(parentContext)
	handlers.ImportDeploymentTargetBundle(invalidImportCtx)
	if invalidImportRecorder.Code != http.StatusConflict || !strings.Contains(invalidImportRecorder.Body.String(), "deployment_bundle.not_ready") {
		t.Fatalf("invalid-stage import = %d %s", invalidImportRecorder.Code, invalidImportRecorder.Body.String())
	}
	assertDeploymentBundleSideEffectCounts(t, db, 0, 0, 0)
	invalidOverridePayload, err := json.Marshal(deploymentTargetBundleImportRequest{Bundle: invalidBundle, Overrides: deploymentTargetBundleOverrides{Stage: "qa"}})
	if err != nil {
		t.Fatal(err)
	}
	invalidOverrideCtx, invalidOverrideRecorder := deploymentBundleRequestContext(http.MethodPost, "/preview", invalidOverridePayload, user, project.ID, app.ID)
	handlers.PreviewDeploymentTargetBundleImport(invalidOverrideCtx)
	if invalidOverrideRecorder.Code != http.StatusOK || !strings.Contains(invalidOverrideRecorder.Body.String(), "deployment.stage_invalid") {
		t.Fatalf("invalid override preview = %d %s", invalidOverrideRecorder.Code, invalidOverrideRecorder.Body.String())
	}

	previewPayload, err := json.Marshal(deploymentTargetBundleImportRequest{Bundle: bundle})
	if err != nil {
		t.Fatal(err)
	}
	previewCtx, previewRecorder := deploymentBundleRequestContext(http.MethodPost, "/preview", previewPayload, user, project.ID, app.ID)
	previewCtx.Request = previewCtx.Request.WithContext(parentContext)
	handlers.PreviewDeploymentTargetBundleImport(previewCtx)
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", previewRecorder.Code, previewRecorder.Body.String())
	}
	var preview deploymentTargetBundlePreview
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.Status != deploymentBundleStatusReady || preview.Summary.Stage != "prod" || strings.TrimSpace(preview.Digest) == "" {
		t.Fatalf("preview = %#v", preview)
	}
	assertDeploymentBundleSideEffectCounts(t, db, 0, 0, 0)
	repairBundle := bundle
	repairBundle.Configuration.Stage = "qa"
	repairPreviewPayload, err := json.Marshal(deploymentTargetBundleImportRequest{Bundle: repairBundle, Overrides: deploymentTargetBundleOverrides{Stage: "staging"}})
	if err != nil {
		t.Fatal(err)
	}
	repairPreviewCtx, repairPreviewRecorder := deploymentBundleRequestContext(http.MethodPost, "/preview", repairPreviewPayload, user, project.ID, app.ID)
	repairPreviewCtx.Request = repairPreviewCtx.Request.WithContext(parentContext)
	handlers.PreviewDeploymentTargetBundleImport(repairPreviewCtx)
	if repairPreviewRecorder.Code != http.StatusOK {
		t.Fatalf("repair preview status = %d body=%s", repairPreviewRecorder.Code, repairPreviewRecorder.Body.String())
	}
	if err := json.Unmarshal(repairPreviewRecorder.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Status != deploymentBundleStatusReady || preview.Summary.Stage != "staging" {
		t.Fatalf("repair preview = %#v", preview)
	}

	importPayload, err := json.Marshal(deploymentTargetBundleImportRequest{Bundle: repairBundle, Digest: preview.Digest, Overrides: deploymentTargetBundleOverrides{Stage: "staging"}})
	if err != nil {
		t.Fatal(err)
	}
	importCtx, importRecorder := deploymentBundleRequestContext(http.MethodPost, "/imports", importPayload, user, project.ID, app.ID)
	importCtx.Request = importCtx.Request.WithContext(parentContext)
	handlers.ImportDeploymentTargetBundle(importCtx)
	if importRecorder.Code != http.StatusCreated {
		t.Fatalf("import status = %d, body = %s", importRecorder.Code, importRecorder.Body.String())
	}
	assertDeploymentBundleSideEffectCounts(t, db, 1, 0, 0)
	var imported deploymentTargetResponse
	if err := json.Unmarshal(importRecorder.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}

	var target model.DeploymentTarget
	if err := db.First(&target, "project_id = ? and application_id = ?", project.ID, app.ID).Error; err != nil {
		t.Fatal(err)
	}
	if target.SourceType != "image" || target.ImageRef != bundle.Configuration.ImageRef || target.Stage != "staging" || !target.Enabled {
		t.Fatalf("imported target = %#v", target)
	}
	var persistedBindings []model.DeploymentTargetHookBinding
	if err := db.Where("target_id = ?", target.ID).Order("run_order asc, created_at asc").Find(&persistedBindings).Error; err != nil {
		t.Fatal(err)
	}
	if len(imported.BuildHookBindings) != 1 || len(persistedBindings) != 1 || imported.BuildHookBindings[0].ID == "" || imported.BuildHookBindings[0].ID != persistedBindings[0].ID || imported.BuildHookBindings[0].Phase != persistedBindings[0].Phase || imported.BuildHookBindings[0].RunOrder != persistedBindings[0].RunOrder {
		t.Fatalf("response bindings = %#v, persisted = %#v", imported.BuildHookBindings, persistedBindings)
	}
	var successfulImports int64
	if err := db.Model(&model.AuditLog{}).
		Where("user_id = ? and action = ? and resource = ? and success = ?", user.ID, "deployment_bundle.import", target.ID, true).
		Count(&successfulImports).Error; err != nil {
		t.Fatal(err)
	}
	if successfulImports != 1 {
		t.Fatalf("successful import audits = %d, want 1", successfulImports)
	}

	failedPayload, err := json.Marshal(deploymentTargetBundleImportRequest{Bundle: repairBundle, Digest: strings.Repeat("0", 64), Overrides: deploymentTargetBundleOverrides{Stage: "prod"}})
	if err != nil {
		t.Fatal(err)
	}
	failedCtx, failedRecorder := deploymentBundleRequestContext(http.MethodPost, "/imports", failedPayload, user, project.ID, app.ID)
	failedCtx.Request = failedCtx.Request.WithContext(parentContext)
	handlers.ImportDeploymentTargetBundle(failedCtx)
	if failedRecorder.Code != http.StatusConflict {
		t.Fatalf("failed import status = %d, body = %s", failedRecorder.Code, failedRecorder.Body.String())
	}
	assertDeploymentBundleSideEffectCounts(t, db, 1, 0, 0)
	assertDeploymentBundleOperationSpans(t, recorder.Ended(), parentSpan.SpanContext().SpanID())
}

func deploymentBundleRequestContext(method, path string, payload []byte, user model.User, projectID, applicationID string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "projectId", Value: projectID}, {Key: "applicationId", Value: applicationID}}
	ctx.Set(currentUserContextKey, user)
	return ctx, recorder
}

func assertDeploymentBundleSideEffectCounts(t *testing.T, db *gorm.DB, targetCount, buildRunCount, releaseCount int64) {
	t.Helper()
	for _, item := range []struct {
		name  string
		model any
		want  int64
	}{
		{name: "deployment targets", model: &model.DeploymentTarget{}, want: targetCount},
		{name: "deployment volume mounts", model: &model.DeploymentVolumeMount{}, want: 0},
		{name: "build environments", model: &model.BuildEnvironmentConfig{}, want: 0},
		{name: "secret values", model: &model.SecretValue{}, want: 0},
		{name: "build runs", model: &model.BuildRun{}, want: buildRunCount},
		{name: "releases", model: &model.Release{}, want: releaseCount},
	} {
		var got int64
		if err := db.Model(item.model).Count(&got).Error; err != nil {
			t.Fatalf("count %s: %v", item.name, err)
		}
		if got != item.want {
			t.Fatalf("%s = %d, want %d", item.name, got, item.want)
		}
	}
}

func assertDeploymentBundleOperationSpans(t *testing.T, spans []sdktrace.ReadOnlySpan, parentSpanID trace.SpanID) {
	t.Helper()
	var previewFound, importSuccessFound, importFailureFound bool
	for _, span := range spans {
		if span.Parent().SpanID() != parentSpanID {
			continue
		}
		switch span.Name() {
		case "deployment.bundle_preview":
			previewFound = span.Status().Code == codes.Ok
		case "deployment.bundle_import":
			if span.Status().Code == codes.Ok {
				importSuccessFound = true
			}
			if span.Status().Code == codes.Error {
				importFailureFound = true
			}
		}
	}
	if !previewFound || !importSuccessFound || !importFailureFound {
		t.Fatalf("deployment bundle spans: preview=%t importSuccess=%t importFailure=%t", previewFound, importSuccessFound, importFailureFound)
	}
}

func deploymentBundleStringPointer(value string) *string {
	return &value
}
