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

func applicationDeleteIntegrationHandlers(t *testing.T, prefix string) (*Handlers, *gorm.DB) {
	t.Helper()
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: prefix,
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{}, &model.Application{}, &model.DeploymentTarget{}, &model.ServiceBinding{},
				&model.ProjectVolume{}, &model.DeploymentVolumeMount{}, &model.AuditLog{},
			)
		},
	})
	project := model.Project{ID: "prj_delete_api", Identifier: "delete-api", Name: "Delete API", KubernetesNamespace: "ns-delete-api", DeleteStatus: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	handlers := &Handlers{db: db, configs: &configCache{values: map[string]string{}}}
	handlers.domains = newDomainHandlers(handlers)
	return handlers, db
}

func deleteApplicationRequest(handlers *Handlers, applicationID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/projects/prj_delete_api/applications/"+applicationID, nil)
	ctx.Params = gin.Params{{Key: "projectId", Value: "prj_delete_api"}, {Key: "applicationId", Value: applicationID}}
	ctx.Set(currentUserContextKey, model.User{ID: "usr_delete_api", Role: authz.PlatformRoleAdmin})
	handlers.domains.application.DeleteApplication(ctx)
	return recorder
}

func seedDeleteApplicationTarget(t *testing.T, db *gorm.DB, applicationID, targetID string) {
	t.Helper()
	application := model.Application{ID: applicationID, ProjectID: "prj_delete_api", Identifier: applicationID, Name: applicationID, DeleteStatus: "active"}
	target := model.DeploymentTarget{ID: targetID, ProjectID: "prj_delete_api", ApplicationID: applicationID, Name: targetID, Stage: "prod", ClusterID: "clu_delete_api", DeleteStatus: "active"}
	if err := db.Create(&application).Error; err != nil {
		t.Fatalf("create application: %v", err)
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create deployment target: %v", err)
	}
}

func seedDeleteApplicationBinding(t *testing.T, db *gorm.DB, id, sourceApp, sourceTarget, targetApp, targetTarget string) {
	t.Helper()
	binding := model.ServiceBinding{
		ID: id, ProjectID: "prj_delete_api", SourceApplicationID: sourceApp, SourceDeploymentTargetID: sourceTarget,
		TargetApplicationID: targetApp, TargetDeploymentTargetID: targetTarget, TargetPortName: "http", TargetPort: 5432,
		Protocol: "tcp", InjectionMode: "host_port", HostEnvVar: "DB_HOST", PortEnvVar: "DB_PORT", Enabled: true, CreatedBy: "usr_delete_api",
	}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatalf("create service binding: %v", err)
	}
}

func TestDeleteApplicationRejectsIncomingBindingInsideTransaction(t *testing.T) {
	handlers, db := applicationDeleteIntegrationHandlers(t, "application_delete_api_incoming_test")
	seedDeleteApplicationTarget(t, db, "app_db", "dplt_db")
	seedDeleteApplicationTarget(t, db, "app_api", "dplt_api")
	seedDeleteApplicationBinding(t, db, "sbind_api_db", "app_api", "dplt_api", "app_db", "dplt_db")

	recorder := deleteApplicationRequest(handlers, "app_db")
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || body["code"] != "service_binding_in_use" {
		t.Fatalf("delete referenced application = %d %s", recorder.Code, recorder.Body.String())
	}
	var stored model.Application
	if err := db.First(&stored, "id = ?", "app_db").Error; err != nil || stored.DeleteStatus != "active" {
		t.Fatalf("referenced application = %#v, %v", stored, err)
	}
}

func TestDeleteApplicationAllowsInternalBindingAndRecordsEnqueueFailureCode(t *testing.T) {
	handlers, db := applicationDeleteIntegrationHandlers(t, "application_delete_api_self_test")
	seedDeleteApplicationTarget(t, db, "app_self", "dplt_self_prod")
	worker := model.DeploymentTarget{ID: "dplt_self_worker", ProjectID: "prj_delete_api", ApplicationID: "app_self", Name: "worker", Stage: "dev", ClusterID: "clu_delete_api", DeleteStatus: "active"}
	if err := db.Create(&worker).Error; err != nil {
		t.Fatalf("create worker target: %v", err)
	}
	seedDeleteApplicationBinding(t, db, "sbind_self", "app_self", "dplt_self_worker", "app_self", "dplt_self_prod")

	// 测试处理器没有任务队列：删除通过入站关系检查后，投递失败必须落 delete_failed 与稳定码。
	recorder := deleteApplicationRequest(handlers, "app_self")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("delete self-bound application = %d %s", recorder.Code, recorder.Body.String())
	}
	var stored model.Application
	if err := db.First(&stored, "id = ?", "app_self").Error; err != nil {
		t.Fatal(err)
	}
	if stored.DeleteStatus != "delete_failed" || stored.DeleteCode != "application.delete_enqueue_failed" {
		t.Fatalf("stored application = %#v", stored)
	}
}
