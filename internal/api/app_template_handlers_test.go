package api

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	"github.com/LiteyukiStudio/devops/internal/api/buildapi"
	"github.com/LiteyukiStudio/devops/internal/api/deploymentapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	secretstore "github.com/LiteyukiStudio/devops/internal/secret"
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
		password          string
		wantStatus        int
		wantCode          string
		wantInstallation  bool
		wantVolumeBinding bool
		wantSecret        bool
	}{
		{
			name: "pending provision is attachable without a password", pendingOperation: volume.OperationProvision,
			wantStatus: http.StatusCreated, wantInstallation: true, wantVolumeBinding: true,
		},
		{
			name: "pending provision stores an optional password", pendingOperation: volume.OperationProvision,
			password: "test-password", wantStatus: http.StatusCreated, wantInstallation: true, wantVolumeBinding: true, wantSecret: true,
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
			codec, err := secretstore.NewCodec("app-template-test-encryption-key")
			if err != nil {
				t.Fatal(err)
			}
			handlers := &Handlers{
				db: db,
				configs: &configCache{values: map[string]string{
					"billing.blockDeployChangesWhenInsufficient": "false",
				}},
				secrets: secretstore.NewStore(db, nil, codec),
			}
			handlers.domains = newDomainHandlers(handlers)

			installNow := false
			body, err := json.Marshal(applicationapi.AppTemplateInstallInput{
				ApplicationName: "Redis", ApplicationIdentifier: "redis-pending-volume",
				DeploymentName: "default", Stage: "prod", ClusterID: "clu_template",
				ImageRef: "redis:7-alpine", Replicas: 1, CPURequest: "500m", MemoryRequest: "512Mi",
				ProjectVolumeID: observation.projectVolume.ID, InstallNow: &installNow, Values: map[string]string{"password": test.password},
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

			handlers.domains.application.InstallAppTemplate(ctx)

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
			if observation.createdTarget == nil || observation.createdTarget.ContainerCommand != "/bin/sh\n-ec" || !strings.Contains(observation.createdTarget.ContainerArgs, "--requirepass") {
				t.Fatalf("created redis target=%#v", observation.createdTarget)
			}
			refs := buildapi.DecodeSecretRefs(observation.createdTarget.SecretRefs)
			var valuesSnapshot map[string]string
			if err := json.Unmarshal([]byte(observation.createdInstallation.ValuesSnapshot), &valuesSnapshot); err != nil {
				t.Fatal(err)
			}
			if test.wantSecret {
				if !strings.HasPrefix(refs["REDIS_PASSWORD"], "secret-id:") || len(observation.createdSecrets) != 1 {
					t.Fatalf("redis secret refs=%#v secret values=%#v", refs, observation.createdSecrets)
				}
				if got := codec.ResolveInline(observation.createdSecrets[0].CipherRef); got != test.password {
					t.Fatalf("stored redis password = %q", got)
				}
				if valuesSnapshot["password"] != "set" {
					t.Fatalf("redis values snapshot = %#v", valuesSnapshot)
				}
				storedMetadata := observation.createdTarget.SecretRefs + observation.createdTarget.ContainerArgs + observation.createdInstallation.ValuesSnapshot
				if strings.Contains(storedMetadata, test.password) {
					t.Fatalf("redis password leaked into deployment metadata: %q", storedMetadata)
				}
			} else {
				if _, exists := refs["REDIS_PASSWORD"]; exists || len(observation.createdSecrets) != 0 {
					t.Fatalf("blank redis password produced secret state: refs=%#v values=%#v", refs, observation.createdSecrets)
				}
				if _, exists := valuesSnapshot["password"]; exists {
					t.Fatalf("blank redis password was marked as set: %#v", valuesSnapshot)
				}
			}
			var response struct {
				Installation     model.AppTemplateInstallation          `json:"installation"`
				DeploymentTarget deploymentapi.DeploymentTargetResponse `json:"deploymentTarget"`
			}
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
	createdApplications []model.Application
	createdTargets      []model.DeploymentTarget
	createdComponents   []model.AppTemplateInstallationComponent
	createdReleases     []model.Release
	createdTarget       *model.DeploymentTarget
	createdMount        *model.DeploymentVolumeMount
	createdInstallation *model.AppTemplateInstallation
	createdSecrets      []model.SecretValue
	createdBindings     []model.ServiceBinding
	// distinctVolumes 让每次项目卷查询返回不同 ID，模拟多组件各自选择独立数据卷。
	distinctVolumes bool
	volumeQueries   int
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
			*destination = model.RuntimeCluster{ID: "clu_template", Name: "Primary", Scope: "global"}
			query.RowsAffected = 1
		case *[]model.ScopedResourceProjectBinding:
			*destination = []model.ScopedResourceProjectBinding{}
		case *model.ProjectVolume:
			*destination = observation.projectVolume
			if observation.distinctVolumes {
				observation.volumeQueries++
				destination.ID = fmt.Sprintf("%s_%d", observation.projectVolume.ID, observation.volumeQueries)
			}
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
			// 多组件场景下伪库无法按部署配置过滤挂载，返回空列表避免把上一个组件的挂载当作冲突。
			if observation.createdMount != nil && !observation.distinctVolumes {
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
		case *model.Application:
			observation.createdApplications = append(observation.createdApplications, *destination)
		case *model.DeploymentTarget:
			created := *destination
			observation.createdTarget = &created
			observation.createdTargets = append(observation.createdTargets, created)
		case *model.AppTemplateInstallationComponent:
			observation.createdComponents = append(observation.createdComponents, *destination)
		case *model.Release:
			observation.createdReleases = append(observation.createdReleases, *destination)
		case *model.DeploymentVolumeMount:
			created := *destination
			observation.createdMount = &created
		case *model.AppTemplateInstallation:
			created := *destination
			observation.createdInstallation = &created
		case *model.SecretValue:
			observation.createdSecrets = append(observation.createdSecrets, *destination)
		case *model.ServiceBinding:
			observation.createdBindings = append(observation.createdBindings, *destination)
		}
		query.RowsAffected = 1
	}); err != nil {
		t.Fatalf("replace app template install create callback: %v", err)
	}
	return db
}

func TestInstallNeoBlogTemplateCreatesComponentsWithSharedSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	observation := &appTemplateInstallDBObservation{projectVolume: model.ProjectVolume{
		ID: "pvol_blog", ProjectID: "prj_template", DisplayName: "Blog data",
		ClusterID: "clu_template", Namespace: "project-template", ClaimName: "pvc-blog-data",
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		LifecycleState: model.ProjectVolumeLifecycleReady, CapacityRequest: "10Gi", CapacityBytes: 10 * 1024 * 1024 * 1024,
		StorageClassName: "fast", AccessMode: model.ProjectVolumeAccessReadWriteOnce,
		VolumeMode: model.ProjectVolumeModeFilesystem, CreatedBy: "usr_template", Revision: 1,
	}, distinctVolumes: true}
	db := newAppTemplateInstallTestDB(t, observation)
	codec, err := secretstore.NewCodec("app-template-test-encryption-key")
	if err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{
		db:      db,
		configs: &configCache{values: map[string]string{"billing.blockDeployChangesWhenInsufficient": "false"}},
		secrets: secretstore.NewStore(db, nil, codec),
	}
	handlers.domains = newDomainHandlers(handlers)
	install := func(input applicationapi.AppTemplateInstallInput) *httptest.ResponseRecorder {
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_template/app-templates/neo-blog/install", strings.NewReader(string(body)))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "projectId", Value: "prj_template"}, {Key: "templateId", Value: "neo-blog"}}
		ctx.Set(currentUserContextKey, model.User{ID: "usr_template", Role: authz.PlatformRoleAdmin})
		handlers.domains.application.InstallAppTemplate(ctx)
		return recorder
	}
	installNow := false
	base := applicationapi.AppTemplateInstallInput{
		ApplicationName: "Blog", ApplicationIdentifier: "blog", DeploymentName: "default", Stage: "prod",
		ClusterID: "clu_template", InstallNow: &installNow,
		ComponentProjectVolumeIDs: map[string]string{"api": "pvol_blog_api", "postgres": "pvol_blog_db"},
	}

	override := base
	override.ImageRef = "example/override:latest"
	if recorder := install(override); recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "app_template.component_override_unsupported") {
		t.Fatalf("single-image override status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	duplicated := base
	duplicated.ComponentProjectVolumeIDs = map[string]string{"api": "pvol_same", "postgres": "pvol_same"}
	if recorder := install(duplicated); recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "app_template.project_volume_duplicated") {
		t.Fatalf("duplicated volume status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	missing := base
	missing.ComponentProjectVolumeIDs = map[string]string{"api": "pvol_blog_api"}
	if recorder := install(missing); recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "app_template.project_volume_required") {
		t.Fatalf("missing volume status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(observation.createdTargets) != 0 || observation.createdInstallation != nil {
		t.Fatalf("rejected installs created state: %#v", observation)
	}

	recorder := install(base)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	identifiers := []string{}
	for _, application := range observation.createdApplications {
		identifiers = append(identifiers, application.Identifier)
	}
	if !reflect.DeepEqual(identifiers, []string{"blog-web", "blog-api", "blog-postgres", "blog-redis"}) {
		t.Fatalf("created application identifiers = %v", identifiers)
	}
	if len(observation.createdTargets) != 4 || len(observation.createdComponents) != 4 || len(observation.createdReleases) != 0 {
		t.Fatalf("targets=%d components=%d releases=%d", len(observation.createdTargets), len(observation.createdComponents), len(observation.createdReleases))
	}
	if observation.createdInstallation.ApplicationID != observation.createdApplications[0].ID || observation.createdComponents[0].ComponentKey != "web" {
		t.Fatalf("entry component is not the installation anchor: %#v %#v", observation.createdInstallation, observation.createdComponents)
	}
	targets := map[string]model.DeploymentTarget{}
	for index, target := range observation.createdTargets {
		targets[observation.createdComponents[index].ComponentKey] = target
	}
	if targets["api"].FSGroup != "1000" || targets["web"].KubernetesName != "luna-blog-web-prod" {
		t.Fatalf("api target=%#v web target=%#v", targets["api"], targets["web"])
	}
	if env := buildapi.DecodeSecretRefs(targets["web"].EnvVars); env["BACKEND_URL"] != "http://luna-blog-api-prod:8888" {
		t.Fatalf("web env = %#v", env)
	}
	edges := map[string]model.ServiceBinding{}
	for _, binding := range observation.createdBindings {
		var sourceKey, targetKey string
		for key, target := range targets {
			if target.ID == binding.SourceDeploymentTargetID {
				sourceKey = key
			}
			if target.ID == binding.TargetDeploymentTargetID {
				targetKey = key
			}
		}
		edges[sourceKey+"->"+targetKey] = binding
	}
	if len(observation.createdBindings) != 4 || len(edges) != 4 {
		t.Fatalf("component service bindings = %#v", observation.createdBindings)
	}
	for edge, want := range map[string][3]any{
		"web->api": {"API_HOST", "API_PORT", 8888}, "web->redis": {"REDIS_HOST", "REDIS_PORT", 6379},
		"api->postgres": {"POSTGRES_HOST", "POSTGRES_PORT", 5432}, "api->redis": {"REDIS_HOST", "REDIS_PORT", 6379},
	} {
		binding, exists := edges[edge]
		if !exists || binding.InjectionMode != "host_port" || binding.Protocol != "tcp" || !binding.Enabled ||
			binding.HostEnvVar != want[0] || binding.PortEnvVar != want[1] || binding.TargetPort != want[2] || binding.ProjectID != "prj_template" {
			t.Fatalf("binding %s = %#v", edge, binding)
		}
	}
	if len(observation.createdReleases) != 0 {
		t.Fatalf("installNow=false created releases: %#v", observation.createdReleases)
	}
	var installationFields struct {
		Installation map[string]any `json:"installation"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &installationFields); err != nil {
		t.Fatal(err)
	}
	if _, exists := installationFields.Installation["status"]; exists {
		t.Fatalf("installation response must not carry a persisted status: %#v", installationFields.Installation)
	}
	resolved := map[string]string{}
	for _, entry := range observation.createdSecrets {
		resolved["secret-id:"+entry.ID] = codec.ResolveInline(entry.CipherRef)
	}
	secretValue := func(component, key string) string {
		ref := buildapi.DecodeSecretRefs(targets[component].SecretRefs)[key]
		if !strings.HasPrefix(ref, "secret-id:") {
			t.Fatalf("%s %s ref = %q", component, key, ref)
		}
		return resolved[ref]
	}
	databasePassword := secretValue("postgres", "POSTGRES_PASSWORD")
	redisPassword := secretValue("redis", "REDIS_PASSWORD")
	if len(databasePassword) < 32 || len(redisPassword) < 32 || databasePassword == redisPassword {
		t.Fatalf("generated passwords are invalid")
	}
	if secretValue("api", "DB_URL") != "postgres://neoblog:"+databasePassword+"@luna-blog-postgres-prod:5432/neoblog?sslmode=disable" {
		t.Fatal("backend DB_URL does not use the PostgreSQL password")
	}
	redisURL := "redis://:" + redisPassword + "@luna-blog-redis-prod:6379/0"
	if secretValue("api", "REDIS_URL") != redisURL || secretValue("web", "REDIS_URL") != redisURL {
		t.Fatal("frontend and backend REDIS_URL do not use the Redis password")
	}
	stored := observation.createdInstallation.ValuesSnapshot
	for _, target := range observation.createdTargets {
		stored += target.EnvVars + target.SecretRefs + target.ContainerArgs
	}
	if strings.Contains(stored, databasePassword) || strings.Contains(stored, redisPassword) {
		t.Fatal("generated password leaked into deployment metadata or values snapshot")
	}
	var response struct {
		Components []struct {
			Key string `json:"key"`
		} `json:"components"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Components) != 4 || strings.Contains(recorder.Body.String(), databasePassword) {
		t.Fatalf("install response = %s", recorder.Body.String())
	}
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
	handlers.domains = newDomainHandlers(handlers)

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?query=transactional&category=database", nil)
	handlers.domains.application.ListAppTemplates(ctx)

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
	if items[0]["featured"] != true {
		t.Fatalf("postgresql summary featured = %#v, want true", items[0]["featured"])
	}

	response = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?query=Dify", nil)
	handlers.domains.application.ListAppTemplates(ctx)
	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("Dify no-match response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?category=storage", nil)
	handlers.domains.application.ListAppTemplates(ctx)
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
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates/redis", nil)
	ctx.Params = gin.Params{{Key: "templateId", Value: "redis"}}
	handlers := &Handlers{}
	handlers.domains = newDomainHandlers(handlers)
	handlers.domains.application.GetAppTemplate(ctx)

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
	password := values[0].(map[string]any)
	if password["key"] != "password" || password["secret"] != true || password["required"] != false || password["autoGenerate"] != false || password["default"] != "" {
		t.Fatalf("redis password definition = %#v", password)
	}
	for _, raw := range values {
		value := raw.(map[string]any)
		if value["secret"] == true && value["default"] != "" {
			t.Fatalf("secret default leaked: %#v", value)
		}
	}
	for _, internal := range []string{"env", "secretEnv", "configFiles", "secretFiles", "containerCommand", "containerArgs"} {
		if _, exists := template[internal]; exists {
			t.Fatalf("internal rendering field %s leaked", internal)
		}
	}
}
