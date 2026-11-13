package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/api/buildapi"
	"github.com/LiteyukiStudio/devops/internal/api/deploymentapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/repository"
	secretstore "github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
)

func TestSystemComponentInstallOpenAPIContractExposesImageAndProvisionAccess(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	input := schemas["SystemComponentInstallInput"].(map[string]any)
	properties := input["properties"].(map[string]any)
	imageProp, ok := properties["image"].(map[string]any)
	if !ok || imageProp["type"] != "string" {
		t.Fatalf("SystemComponentInstallInput.image = %#v", properties["image"])
	}
	provisionProp, ok := properties["provisionAccess"].(map[string]any)
	if !ok || provisionProp["type"] != "boolean" {
		t.Fatalf("SystemComponentInstallInput.provisionAccess = %#v", properties["provisionAccess"])
	}
}

func TestSystemComponentReinstallAndProjectSpaceUpdatePostgres(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := aiProjectScopeIntegrationDB(t)

	admin := model.User{
		ID:    "usr_probe_admin",
		Email: "probe-admin@example.test",
		Name:  "Probe Admin",
		Role:  authz.PlatformRoleAdmin,
	}
	developer := model.User{
		ID:    "usr_probe_dev",
		Email: "probe-dev@example.test",
		Name:  "Probe Developer",
		Role:  authz.PlatformRoleUser,
	}
	cluster := model.RuntimeCluster{
		ID:                    "clu_6b0377ad1d6f04eddfa72a3d",
		Name:                  "Production Cluster",
		Scope:                 "global",
		IsDefault:             true,
		GatewayControllerType: "traefik",
		GatewayNamespace:      "kube-system",
	}
	for _, value := range []any{&admin, &developer, &cluster} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	codec, err := secretstore.NewCodec("system-component-test-encryption-key")
	if err != nil {
		t.Fatal(err)
	}
	enqueuer := &fakeBuildTaskEnqueuer{}
	handlers := &Handlers{
		db:       db,
		projects: repository.NewProjectRepository(db),
		configs: &configCache{values: map[string]string{
			"billing.blockDeployChangesWhenInsufficient": "true",
		}},
		secrets:    secretstore.NewStore(db, nil, codec),
		taskClient: enqueuer,
	}
	handlers.domains = newDomainHandlers(handlers)

	invoke := func(user model.User, method, path string, params gin.Params, body any, fn gin.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		payload, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(method, path, bytes.NewReader(payload))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = params
		ctx.Set(currentUserContextKey, user)
		fn(ctx)
		ctx.Writer.WriteHeaderNow()
		return recorder
	}

	// 1. Initial system install (omitting provisionAccess to verify CLI/API default = true).
	installBody := map[string]any{
		"clusterId":  cluster.ID,
		"apiBaseUrl": "https://devops.example.test",
	}
	initialResp := invoke(
		admin,
		http.MethodPost,
		"/api/v1/app-templates/luna-gateway-traffic-probe/system-install",
		gin.Params{{Key: "templateId", Value: "luna-gateway-traffic-probe"}},
		installBody,
		handlers.domains.runtime.InstallSystemAppTemplate,
	)
	if initialResp.Code != http.StatusCreated {
		t.Fatalf("initial system install = %d %s", initialResp.Code, initialResp.Body.String())
	}

	var initialPayload struct {
		Installation     model.SystemComponentInstallation      `json:"installation"`
		Application      model.Application                      `json:"application"`
		DeploymentTarget deploymentapi.DeploymentTargetResponse `json:"deploymentTarget"`
		Release          model.Release                          `json:"release"`
	}
	if err := json.Unmarshal(initialResp.Body.Bytes(), &initialPayload); err != nil {
		t.Fatalf("decode initial install response: %v", err)
	}
	if !initialPayload.Release.ForceImagePull {
		t.Fatal("expected initial system component release to set forceImagePull=true")
	}

	var storedTarget model.DeploymentTarget
	if err := db.First(&storedTarget, "id = ?", initialPayload.DeploymentTarget.ID).Error; err != nil {
		t.Fatalf("load stored deployment target: %v", err)
	}
	if storedTarget.ServiceAccountName != "luna-gateway-traffic-probe" || storedTarget.AutomountServiceAccountToken != "true" {
		t.Fatalf("initial service account = (%q, %q), want (luna-gateway-traffic-probe, true)", storedTarget.ServiceAccountName, storedTarget.AutomountServiceAccountToken)
	}
	if storedTarget.BuildConcurrencyPolicy != model.BuildConcurrencyQueue {
		t.Fatalf("buildConcurrencyPolicy = %q, want %q", storedTarget.BuildConcurrencyPolicy, model.BuildConcurrencyQueue)
	}

	// 2. Grant an ordinary non-admin user developer role in the platform system project.
	member := model.ProjectMember{
		ID:        "prjm_probe_dev",
		ProjectID: initialPayload.Installation.ProjectID,
		UserID:    developer.ID,
		Role:      authz.ProjectRoleDeveloper,
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("seed developer project member: %v", err)
	}

	// 3. Ordinary developer updates the probe deployment target directly in the project space.
	updateBody := map[string]any{
		"name":          storedTarget.Name,
		"stage":         storedTarget.Stage,
		"clusterId":     storedTarget.ClusterID,
		"sourceType":    "image",
		"imageRef":      "liteyukistudio/devops-gateway-traffic-probe:v2",
		"replicas":      1,
		"cpuRequest":    "250m",
		"memoryRequest": "256Mi",
		"servicePorts":  []map[string]any{{"name": "metrics", "port": 9090}},
		"enabled":       true,
		"environmentVariables": []map[string]any{
			{"key": "API_BASE_URL", "value": "https://devops.example.test", "valueMode": "public"},
		},
	}
	updateResp := invoke(
		developer,
		http.MethodPut,
		"/api/v1/projects/"+initialPayload.Installation.ProjectID+"/applications/"+initialPayload.Application.ID+"/deployment-targets/"+storedTarget.ID,
		gin.Params{
			{Key: "projectId", Value: initialPayload.Installation.ProjectID},
			{Key: "applicationId", Value: initialPayload.Application.ID},
			{Key: "targetId", Value: storedTarget.ID},
		},
		updateBody,
		handlers.domains.deployment.UpdateDeploymentTarget,
	)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("developer project-space deployment update = %d %s", updateResp.Code, updateResp.Body.String())
	}

	var updatedTarget model.DeploymentTarget
	if err := db.First(&updatedTarget, "id = ?", storedTarget.ID).Error; err != nil {
		t.Fatalf("reload updated target: %v", err)
	}
	if updatedTarget.ServiceAccountName != "luna-gateway-traffic-probe" || updatedTarget.AutomountServiceAccountToken != "true" {
		t.Fatalf("project-space update wiped service account = (%q, %q)", updatedTarget.ServiceAccountName, updatedTarget.AutomountServiceAccountToken)
	}
	if updatedTarget.CPURequest != "250m" || updatedTarget.MemoryRequest != "256Mi" || updatedTarget.ImageRef != "liteyukistudio/devops-gateway-traffic-probe:v2" {
		t.Fatalf("updated target = (%q, %q, %q)", updatedTarget.CPURequest, updatedTarget.MemoryRequest, updatedTarget.ImageRef)
	}
	if refs := buildapi.DecodeSecretRefs(updatedTarget.SecretRefs); refs["REPORT_TOKEN"] == "" {
		t.Fatalf("project-space update dropped REPORT_TOKEN secret ref: %#v", refs)
	}

	// 4. Ordinary developer triggers a release directly in the project space.
	releaseResp := invoke(
		developer,
		http.MethodPost,
		"/api/v1/projects/"+initialPayload.Installation.ProjectID+"/releases",
		gin.Params{
			{Key: "projectId", Value: initialPayload.Installation.ProjectID},
		},
		map[string]any{
			"applicationId":      initialPayload.Application.ID,
			"deploymentTargetId": storedTarget.ID,
			"forceImagePull":     true,
			"message":            "update probe from project space",
		},
		handlers.domains.deployment.CreateRelease,
	)
	if releaseResp.Code != http.StatusCreated {
		t.Fatalf("developer project-space release = %d %s", releaseResp.Code, releaseResp.Body.String())
	}
	var projectRelease model.Release
	if err := json.Unmarshal(releaseResp.Body.Bytes(), &projectRelease); err != nil {
		t.Fatalf("decode project release: %v", err)
	}
	if !projectRelease.ForceImagePull || enqueuer.deployPayload.ReleaseID != projectRelease.ID {
		t.Fatalf("project release = %#v, enqueued = %#v", projectRelease, enqueuer.deployPayload)
	}

	// 5. Reinstall via system-install succeeds against real PostgreSQL CHECK constraints and preserves tuned resources.
	reinstallResp := invoke(
		admin,
		http.MethodPost,
		"/api/v1/app-templates/luna-gateway-traffic-probe/system-install",
		gin.Params{{Key: "templateId", Value: "luna-gateway-traffic-probe"}},
		map[string]any{
			"clusterId":  cluster.ID,
			"apiBaseUrl": "https://devops.example.test",
			"image":      "liteyukistudio/devops-gateway-traffic-probe:nightly",
		},
		handlers.domains.runtime.InstallSystemAppTemplate,
	)
	if reinstallResp.Code != http.StatusCreated {
		t.Fatalf("reinstall system component = %d %s", reinstallResp.Code, reinstallResp.Body.String())
	}

	var reinstallPayload struct {
		Installation model.SystemComponentInstallation `json:"installation"`
		Release      model.Release                     `json:"release"`
	}
	if err := json.Unmarshal(reinstallResp.Body.Bytes(), &reinstallPayload); err != nil {
		t.Fatalf("decode reinstall response: %v", err)
	}
	if !reinstallPayload.Release.ForceImagePull {
		t.Fatal("expected reinstall release to set forceImagePull=true")
	}

	var reinstalledTarget model.DeploymentTarget
	if err := db.First(&reinstalledTarget, "id = ?", storedTarget.ID).Error; err != nil {
		t.Fatalf("reload reinstalled target: %v", err)
	}
	if reinstalledTarget.CPURequest != "250m" || reinstalledTarget.MemoryRequest != "256Mi" {
		t.Fatalf("reinstall did not preserve customized resources = (%q, %q)", reinstalledTarget.CPURequest, reinstalledTarget.MemoryRequest)
	}
	if reinstalledTarget.ServiceAccountName != "luna-gateway-traffic-probe" || reinstalledTarget.AutomountServiceAccountToken != "true" {
		t.Fatalf("reinstall service account = (%q, %q)", reinstalledTarget.ServiceAccountName, reinstalledTarget.AutomountServiceAccountToken)
	}
	if reinstalledTarget.ImageRef != "liteyukistudio/devops-gateway-traffic-probe:nightly" {
		t.Fatalf("reinstall imageRef = %q", reinstalledTarget.ImageRef)
	}
}
