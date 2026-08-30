package api

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAppTemplateInstallOpenAPIContractIsAgentRepairable(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	operation := paths["/api/v1/projects/{projectId}/app-templates/{templateId}/install"].(map[string]any)["post"].(map[string]any)
	cli := operation["x-luna-cli"].(map[string]any)
	scopes, _ := schemaStringList(cli["requiredScopes"])
	if cli["command"] != "app-template.install" || cli["classification"] != "business-command" || cli["risk"] != "medium" || cli["agentAllowed"] != true || !reflect.DeepEqual(scopes, []string{"project:write"}) {
		t.Fatalf("installAppTemplate CLI metadata = %#v", cli)
	}
	agent := operation["x-luna-agent"].(map[string]any)
	for _, field := range []string{"purpose", "aliases", "avoidWhen", "preconditions", "successEvidence"} {
		if agent[field] == nil {
			t.Fatalf("installAppTemplate Agent metadata is missing %s: %#v", field, agent)
		}
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	input := schemas["AppTemplateInstallInput"].(map[string]any)
	properties := input["properties"].(map[string]any)
	stage := properties["stage"].(map[string]any)
	values, ok := schemaStringList(stage["enum"])
	if !ok || !reflect.DeepEqual(values, publicDeploymentStages) || !strings.Contains(stage["description"].(string), "default") {
		t.Fatalf("AppTemplateInstallInput.stage = %#v", stage)
	}
	projectVolume := properties["projectVolumeId"].(map[string]any)
	description := projectVolume["description"].(string)
	for _, required := range []string{"lifecycleState=ready", "lifecycleState=provisioning", "pendingOperation=provision", "pendingOperation=expand", "pendingOperation=import"} {
		if !strings.Contains(description, required) {
			t.Fatalf("AppTemplateInstallInput.projectVolumeId is missing %q: %#v", required, projectVolume)
		}
	}
	if strings.Contains(description, "availability") {
		t.Fatalf("AppTemplateInstallInput.projectVolumeId = %#v", projectVolume)
	}
}

func TestInstallAppTemplateAcceptsPendingProvisionAndRejectsPendingImport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name              string
		pendingOperation  string
		wantStatus        int
		wantCode          string
		wantInstallation  bool
		wantVolumeBinding bool
	}{
		{
			name: "pending provision is attachable", pendingOperation: volume.OperationProvision,
			wantStatus: http.StatusCreated, wantInstallation: true, wantVolumeBinding: true,
		},
		{
			name: "pending import is rejected", pendingOperation: volume.OperationImport,
			wantStatus: http.StatusConflict, wantCode: "project_volume.not_attachable",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			observation := &appTemplateInstallDBObservation{projectVolume: model.ProjectVolume{
				ID: "pvol_pending", ProjectID: "prj_template", DisplayName: "Redis data",
				ClusterID: "clu_template", Namespace: "project-template", ClaimName: "pvc-redis-data",
				OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
				LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: test.pendingOperation,
				CapacityRequest: "10Gi", CapacityBytes: 10 * 1024 * 1024 * 1024,
				StorageClassName: "fast", AccessMode: model.ProjectVolumeAccessReadWriteOnce,
				VolumeMode: model.ProjectVolumeModeFilesystem, CreatedBy: "usr_template", Revision: 1,
			}}
			db := newAppTemplateInstallTestDB(t, observation)
			handlers := &Handlers{
				db: db,
				configs: &configCache{values: map[string]string{
					"billing.blockDeployChangesWhenInsufficient": "false",
				}},
			}

			installNow := false
			body, err := json.Marshal(appTemplateInstallInput{
				ApplicationName: "Redis", ApplicationIdentifier: "redis-pending-volume",
				DeploymentName: "default", Stage: "prod", ClusterID: "clu_template",
				ImageRef: "redis:7-alpine", Replicas: 1, CPURequest: "500m", MemoryRequest: "512Mi",
				ProjectVolumeID: observation.projectVolume.ID, InstallNow: &installNow, Values: map[string]string{},
			})
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_template/app-templates/redis/install", strings.NewReader(string(body)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Params = gin.Params{{Key: "projectId", Value: "prj_template"}, {Key: "templateId", Value: "redis"}}
			ctx.Set(currentUserContextKey, model.User{ID: "usr_template", Role: authz.PlatformRoleAdmin})

			handlers.InstallAppTemplate(ctx)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if test.wantCode != "" && !strings.Contains(recorder.Body.String(), test.wantCode) {
				t.Fatalf("body=%s, want code %q", recorder.Body.String(), test.wantCode)
			}
			if (observation.createdInstallation != nil) != test.wantInstallation {
				t.Fatalf("created installation=%#v, want=%t", observation.createdInstallation, test.wantInstallation)
			}
			if (observation.createdMount != nil) != test.wantVolumeBinding {
				t.Fatalf("created mount=%#v, want=%t", observation.createdMount, test.wantVolumeBinding)
			}
			if !test.wantVolumeBinding {
				return
			}
			if observation.createdMount.ActivationState != model.DeploymentVolumeActivationReserved ||
				observation.createdMount.ProjectVolumeID == nil || *observation.createdMount.ProjectVolumeID != observation.projectVolume.ID {
				t.Fatalf("created mount=%#v", observation.createdMount)
			}
			var response appTemplateInstallResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Installation.ID == "" || len(response.DeploymentTarget.DataVolumes) != 1 ||
				response.DeploymentTarget.DataVolumes[0].ProjectVolumeID != observation.projectVolume.ID {
				t.Fatalf("install response=%#v", response)
			}
		})
	}
}

type appTemplateInstallDBObservation struct {
	projectVolume       model.ProjectVolume
	createdTarget       *model.DeploymentTarget
	createdMount        *model.DeploymentVolumeMount
	createdInstallation *model.AppTemplateInstallation
}

func newAppTemplateInstallTestDB(t *testing.T, observation *appTemplateInstallDBObservation) *gorm.DB {
	t.Helper()
	sqlDB := sql.OpenDB(appTemplateInstallTestConnector{})
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB, PreferSimpleProtocol: true, WithoutReturning: true,
	}), &gorm.Config{
		DisableAutomaticPing: true, DisableNestedTransaction: true, SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open app template install test database: %v", err)
	}
	if err := db.Callback().Query().Replace("gorm:query", func(query *gorm.DB) {
		switch destination := query.Statement.Dest.(type) {
		case *model.Project:
			*destination = model.Project{
				ID: "prj_template", Identifier: "project-template", KubernetesNamespace: "project-template", DeleteStatus: "active",
			}
			query.RowsAffected = 1
		case *model.Application:
			query.AddError(gorm.ErrRecordNotFound)
		case *model.RuntimeCluster:
			*destination = model.RuntimeCluster{ID: "clu_template", Name: "Primary", Type: "kubernetes", Scope: "global"}
			query.RowsAffected = 1
		case *[]model.ScopedResourceProjectBinding:
			*destination = []model.ScopedResourceProjectBinding{}
		case *model.ProjectVolume:
			*destination = observation.projectVolume
			query.RowsAffected = 1
		case *model.DeploymentTarget:
			if observation.createdTarget == nil {
				query.AddError(gorm.ErrRecordNotFound)
				return
			}
			*destination = *observation.createdTarget
			query.RowsAffected = 1
		case *[]model.DeploymentVolumeMount:
			*destination = []model.DeploymentVolumeMount{}
			if observation.createdMount != nil {
				*destination = append(*destination, *observation.createdMount)
				query.RowsAffected = 1
			}
		default:
			t.Errorf("unexpected app template install query destination %T", query.Statement.Dest)
			query.AddError(gorm.ErrRecordNotFound)
		}
	}); err != nil {
		t.Fatalf("replace app template install query callback: %v", err)
	}
	if err := db.Callback().Create().Replace("gorm:create", func(query *gorm.DB) {
		switch destination := query.Statement.Dest.(type) {
		case *model.DeploymentTarget:
			created := *destination
			observation.createdTarget = &created
		case *model.DeploymentVolumeMount:
			created := *destination
			observation.createdMount = &created
		case *model.AppTemplateInstallation:
			created := *destination
			observation.createdInstallation = &created
		}
		query.RowsAffected = 1
	}); err != nil {
		t.Fatalf("replace app template install create callback: %v", err)
	}
	return db
}

type appTemplateInstallTestConnector struct{}

func (appTemplateInstallTestConnector) Connect(context.Context) (driver.Conn, error) {
	return appTemplateInstallTestConn{}, nil
}

func (appTemplateInstallTestConnector) Driver() driver.Driver {
	return appTemplateInstallTestDriver{}
}

type appTemplateInstallTestDriver struct{}

func (appTemplateInstallTestDriver) Open(string) (driver.Conn, error) {
	return appTemplateInstallTestConn{}, nil
}

type appTemplateInstallTestConn struct{}

func (appTemplateInstallTestConn) Prepare(string) (driver.Stmt, error) {
	return appTemplateInstallTestStmt{}, nil
}

func (appTemplateInstallTestConn) Close() error {
	return nil
}

func (appTemplateInstallTestConn) Begin() (driver.Tx, error) {
	return appTemplateInstallTestTx{}, nil
}

type appTemplateInstallTestStmt struct{}

func (appTemplateInstallTestStmt) Close() error {
	return nil
}

func (appTemplateInstallTestStmt) NumInput() int {
	return -1
}

func (appTemplateInstallTestStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

func (appTemplateInstallTestStmt) Query([]driver.Value) (driver.Rows, error) {
	return appTemplateInstallTestRows{}, nil
}

type appTemplateInstallTestRows struct{}

func (appTemplateInstallTestRows) Columns() []string {
	return nil
}

func (appTemplateInstallTestRows) Close() error {
	return nil
}

func (appTemplateInstallTestRows) Next([]driver.Value) error {
	return io.EOF
}

type appTemplateInstallTestTx struct{}

func (appTemplateInstallTestTx) Commit() error {
	return nil
}

func (appTemplateInstallTestTx) Rollback() error {
	return nil
}

func TestDeploymentStageInvalidErrorIsStructuredAndNotRetryable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj/app-templates/redis/install", nil)
	writeDeploymentStageInvalid(ctx, "stage", "deployment stage must be canonical")

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || response["code"] != "deployment.stage_invalid" || response["path"] != "stage" || response["retryable"] != false {
		t.Fatalf("structured stage error = %d %#v", recorder.Code, response)
	}
	allowed, ok := schemaStringList(response["allowedValues"])
	if !ok || !reflect.DeepEqual(allowed, publicDeploymentStages) {
		t.Fatalf("allowedValues = %#v", response["allowedValues"])
	}
}

func TestListAppTemplatesFiltersSummariesByQueryAndCategory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers := &Handlers{}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?query=transactional&category=database", nil)
	handlers.ListAppTemplates(ctx)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0]["id"] != "postgresql" {
		t.Fatalf("items = %#v", items)
	}
	if _, exists := items[0]["values"]; exists {
		t.Fatal("list response must not embed full template values")
	}
	if items[0]["valueCount"] == nil || items[0]["requiredValueCount"] == nil {
		t.Fatalf("summary counts missing: %#v", items[0])
	}

	response = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?query=Dify", nil)
	handlers.ListAppTemplates(ctx)
	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("Dify no-match response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?category=storage", nil)
	handlers.ListAppTemplates(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("storage status = %d body=%s", response.Code, response.Body.String())
	}
	items = nil
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("storage items = %#v, want Garage, Verdaccio, and Docker Registry", items)
	}
	for _, item := range items {
		if item["category"] != "storage" {
			t.Fatalf("storage filter returned category %v", item["category"])
		}
	}
}

func TestGetAppTemplateReturnsSanitizedFullDefinition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates/mongodb", nil)
	ctx.Params = gin.Params{{Key: "templateId", Value: "mongodb"}}
	(&Handlers{}).GetAppTemplate(ctx)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var template map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &template); err != nil {
		t.Fatal(err)
	}
	values, ok := template["values"].([]any)
	if !ok || len(values) == 0 {
		t.Fatalf("values = %#v", template["values"])
	}
	for _, raw := range values {
		value := raw.(map[string]any)
		if value["secret"] == true && value["default"] != "" {
			t.Fatalf("secret default leaked: %#v", value)
		}
	}
	for _, internal := range []string{"env", "secretEnv", "configFiles", "secretFiles"} {
		if _, exists := template[internal]; exists {
			t.Fatalf("internal rendering field %s leaked", internal)
		}
	}
}
