package api

import (
	"bytes"
	"context"
	"encoding/json"
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
	if configuration.SecretRefs != "" || configuration.SecretFiles != "" || configuration.BuildSecrets != nil || configuration.Enabled {
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
	base.Configuration.SecretRefs = `{"TOKEN":"plaintext"}`
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
			Name: "Imported image", Stage: "preview", SourceType: "image", ImageRef: "registry.example.test/service:v1",
		},
		References:         []deploymentBundleReference{},
		SecretRequirements: []deploymentBundleSecretRequirement{},
		Omissions:          []string{"runtimeState", "history", "secretValues"},
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
	if preview.Status != deploymentBundleStatusReady || strings.TrimSpace(preview.Digest) == "" {
		t.Fatalf("preview = %#v", preview)
	}
	assertDeploymentBundleSideEffectCounts(t, db, 0, 0, 0)

	importPayload, err := json.Marshal(deploymentTargetBundleImportRequest{Bundle: bundle, Digest: preview.Digest})
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

	var target model.DeploymentTarget
	if err := db.First(&target, "project_id = ? and application_id = ?", project.ID, app.ID).Error; err != nil {
		t.Fatal(err)
	}
	if target.SourceType != "image" || target.ImageRef != bundle.Configuration.ImageRef || !target.Enabled {
		t.Fatalf("imported target = %#v", target)
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

	failedPayload, err := json.Marshal(deploymentTargetBundleImportRequest{Bundle: bundle, Digest: strings.Repeat("0", 64), Overrides: deploymentTargetBundleOverrides{Stage: "failed"}})
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
