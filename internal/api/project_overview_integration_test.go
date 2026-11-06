package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestGetProjectOverviewObservesEnabledAccessRoutes(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "project_overview_api_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.Application{}, &model.Release{}, &model.DeploymentTarget{}, &model.GatewayRoute{}, &model.RuntimeCluster{})
		},
	})
	project := model.Project{ID: "prj_overview", Identifier: "overview", Name: "Overview", KubernetesNamespace: "ns-overview", DeleteStatus: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	routes := []model.GatewayRoute{
		// 部署配置不存在：入口被观察为 not-configured，计入总数但不就绪。
		{ID: "grt_orphan", ProjectID: project.ID, ApplicationID: "app_web", DeploymentTargetID: "dplt_missing", Host: "web.example.test", Path: "/", ServicePort: 80, Enabled: true, DeleteStatus: "active"},
		// 停用与删除中的入口不参与就绪统计。
		{ID: "grt_disabled", ProjectID: project.ID, ApplicationID: "app_web", DeploymentTargetID: "dplt_missing", Host: "off.example.test", Path: "/", ServicePort: 80, Enabled: true, DeleteStatus: "active"},
		{ID: "grt_deleting", ProjectID: project.ID, ApplicationID: "app_web", DeploymentTargetID: "dplt_missing", Host: "gone.example.test", Path: "/", ServicePort: 80, Enabled: true, DeleteStatus: "deleting"},
	}
	if err := db.Create(&routes).Error; err != nil {
		t.Fatalf("create gateway routes: %v", err)
	}
	if err := db.Model(&model.GatewayRoute{}).Where("id = ?", "grt_disabled").Update("enabled", false).Error; err != nil {
		t.Fatalf("disable route: %v", err)
	}
	handlers := &Handlers{db: db, configs: &configCache{values: map[string]string{}}}
	handlers.domains = newDomainHandlers(handlers)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj_overview/overview", nil)
	ctx.Params = gin.Params{{Key: "projectId", Value: project.ID}}
	ctx.Set(currentUserContextKey, model.User{ID: "usr_overview", Role: authz.PlatformRoleAdmin})
	handlers.domains.project.GetProjectOverview(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("get project overview = %d %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store for live route observation", got)
	}
	var body struct {
		AccessRoutes struct {
			Status          string `json:"status"`
			Ready           *int64 `json:"ready"`
			Total           int64  `json:"total"`
			ObservationCode string `json:"observationCode"`
			ObservedAt      string `json:"observedAt"`
		} `json:"accessRoutes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	routesSummary := body.AccessRoutes
	if routesSummary.Status != "degraded" || routesSummary.Ready == nil || *routesSummary.Ready != 0 || routesSummary.Total != 1 ||
		routesSummary.ObservationCode != "project_overview.access_routes_not_ready" || routesSummary.ObservedAt == "" {
		t.Fatalf("access routes = %+v (%s)", routesSummary, recorder.Body.String())
	}
}
