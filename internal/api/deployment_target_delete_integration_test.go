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

func deploymentTargetDeleteIntegrationHandlers(t *testing.T, prefix string) (*Handlers, *gorm.DB) {
	t.Helper()
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: prefix,
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{}, &model.Application{}, &model.DeploymentTarget{}, &model.ServiceBinding{}, &model.GatewayRoute{},
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

func deleteDeploymentTargetRequest(handlers *Handlers, applicationID, targetID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/projects/prj_delete_api/applications/"+applicationID+"/deployment-targets/"+targetID, nil)
	ctx.Params = gin.Params{{Key: "projectId", Value: "prj_delete_api"}, {Key: "applicationId", Value: applicationID}, {Key: "targetId", Value: targetID}}
	ctx.Set(currentUserContextKey, model.User{ID: "usr_delete_api", Role: authz.PlatformRoleAdmin})
	handlers.domains.deployment.DeleteDeploymentTarget(ctx)
	return recorder
}

func TestDeleteDeploymentTargetRejectsIncomingBindingInsideTransaction(t *testing.T) {
	handlers, db := deploymentTargetDeleteIntegrationHandlers(t, "deployment_target_delete_api_incoming_test")
	seedDeleteApplicationTarget(t, db, "app_db", "dplt_db")
	seedDeleteApplicationTarget(t, db, "app_api", "dplt_api")
	seedDeleteApplicationBinding(t, db, "sbind_api_db", "app_api", "dplt_api", "app_db", "dplt_db")

	recorder := deleteDeploymentTargetRequest(handlers, "app_db", "dplt_db")
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || body["code"] != "service_binding_in_use" {
		t.Fatalf("delete referenced deployment target = %d %s", recorder.Code, recorder.Body.String())
	}
	var stored model.DeploymentTarget
	if err := db.First(&stored, "id = ?", "dplt_db").Error; err != nil || stored.DeleteStatus != "active" {
		t.Fatalf("referenced deployment target = %#v, %v", stored, err)
	}
}

func TestDeleteDeploymentTargetRecordsEnqueueFailureCode(t *testing.T) {
	handlers, db := deploymentTargetDeleteIntegrationHandlers(t, "deployment_target_delete_api_enqueue_test")
	seedDeleteApplicationTarget(t, db, "app_web", "dplt_web")
	if err := db.Model(&model.DeploymentTarget{}).Where("id = ?", "dplt_web").Updates(map[string]any{
		"delete_status": "delete_failed", "delete_code": "service_binding_in_use",
	}).Error; err != nil {
		t.Fatalf("seed previous failure: %v", err)
	}
	route := model.GatewayRoute{ID: "grt_web", ProjectID: "prj_delete_api", ApplicationID: "app_web", DeploymentTargetID: "dplt_web", Host: "web.example.test", Path: "/", ServicePort: 80, Enabled: true, DeleteStatus: "active"}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create gateway route: %v", err)
	}

	// 测试处理器没有任务队列：重试删除通过入站关系检查后，投递失败必须落 delete_failed 与新的稳定码。
	recorder := deleteDeploymentTargetRequest(handlers, "app_web", "dplt_web")
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusServiceUnavailable || body["code"] != deploymentTargetDeleteEnqueueFailedCodeForTest {
		t.Fatalf("delete deployment target = %d %s", recorder.Code, recorder.Body.String())
	}
	var stored model.DeploymentTarget
	if err := db.First(&stored, "id = ?", "dplt_web").Error; err != nil {
		t.Fatal(err)
	}
	if stored.DeleteStatus != "delete_failed" || stored.DeleteCode != deploymentTargetDeleteEnqueueFailedCodeForTest {
		t.Fatalf("stored deployment target = %#v", stored)
	}
	var storedRoute model.GatewayRoute
	if err := db.First(&storedRoute, "id = ?", route.ID).Error; err != nil || storedRoute.DeleteStatus != "delete_failed" {
		t.Fatalf("stored gateway route = %#v, %v", storedRoute, err)
	}
}

const deploymentTargetDeleteEnqueueFailedCodeForTest = "deployment_target.delete_enqueue_failed"
