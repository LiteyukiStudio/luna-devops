package worker

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func deploymentTargetCleanupTask(t *testing.T, target model.DeploymentTarget) *asynq.Task {
	t.Helper()
	data, err := json.Marshal(tasks.ResourceCleanupPayload{ResourceType: "deployment_target", ResourceID: target.ID, ProjectID: target.ProjectID, ActorID: "usr_demo"})
	if err != nil {
		t.Fatal(err)
	}
	return asynq.NewTask(tasks.TypeResourceCleanup, data)
}

// seedDeletingDeploymentTarget 准备一个 API 已标记 deleting 的部署配置及其随同进入 deleting 的访问入口。
func seedDeletingDeploymentTarget(t *testing.T, db *gorm.DB) (model.Project, model.DeploymentTarget, model.GatewayRoute) {
	t.Helper()
	project, database, target := seedApplicationDeleteFixture(t, db)
	startedAt := time.Now()
	if err := db.Model(&model.Application{}).Where("id = ?", database.ID).Update("delete_status", "active").Error; err != nil {
		t.Fatalf("activate application: %v", err)
	}
	if err := db.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Updates(map[string]any{"delete_status": "deleting", "delete_started_at": &startedAt}).Error; err != nil {
		t.Fatalf("mark target deleting: %v", err)
	}
	route := model.GatewayRoute{
		ID: "grt_db", ProjectID: project.ID, ApplicationID: database.ID, DeploymentTargetID: target.ID,
		Host: "db.example.test", Path: "/", ServicePort: 5432, Enabled: true, DeleteStatus: "deleting", DeleteStartedAt: &startedAt,
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create gateway route: %v", err)
	}
	return project, target, route
}

func TestDeploymentTargetDeleteWithIncomingBindingFailsBeforeRuntimeCleanup(t *testing.T) {
	db := openApplicationDeleteTestDB(t, "deployment_target_delete_incoming_test")
	project, target, route := seedDeletingDeploymentTarget(t, db)
	api := model.Application{ID: "app_api", ProjectID: project.ID, Identifier: "api", Name: "API", DeleteStatus: "active"}
	apiTarget := model.DeploymentTarget{ID: "dplt_api", ProjectID: project.ID, ApplicationID: api.ID, Name: "default", Stage: "prod", ClusterID: target.ClusterID}
	binding := model.ServiceBinding{
		ID: "sbind_api_db", ProjectID: project.ID, SourceApplicationID: api.ID, SourceDeploymentTargetID: apiTarget.ID,
		TargetApplicationID: target.ApplicationID, TargetDeploymentTargetID: target.ID, TargetPortName: "http", TargetPort: 5432,
		Protocol: "tcp", InjectionMode: "host_port", HostEnvVar: "DB_HOST", PortEnvVar: "DB_PORT", Enabled: true, CreatedBy: "usr_demo",
	}
	for _, row := range []any{&api, &apiTarget, &binding} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create fixture %T: %v", row, err)
		}
	}
	runner := newRunner(db, Options{})
	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		t.Fatal("runtime resources were cleaned up although the deployment target is still referenced")
		return nil, nil
	}

	err := runner.handleResourceCleanup(t.Context(), deploymentTargetCleanupTask(t, target))
	if !errors.Is(err, asynq.SkipRetry) || !errors.Is(err, errDeploymentTargetReferencedByServiceBinding) {
		t.Fatalf("handleResourceCleanup() error = %v, want non-retryable binding conflict", err)
	}
	var stored model.DeploymentTarget
	if err := db.First(&stored, "id = ?", target.ID).Error; err != nil {
		t.Fatalf("load deployment target: %v", err)
	}
	if stored.DeleteStatus != "delete_failed" || stored.DeleteCode != deploymentTargetDeleteCodeServiceBindingInUse || stored.DeleteFinishedAt == nil {
		t.Fatalf("stored deployment target = %#v", stored)
	}
	var storedRoute model.GatewayRoute
	if err := db.First(&storedRoute, "id = ?", route.ID).Error; err != nil || storedRoute.DeleteStatus != "delete_failed" {
		t.Fatalf("stored gateway route = %#v, %v", storedRoute, err)
	}
	var remaining int64
	if err := db.Model(&model.ServiceBinding{}).Where("id = ?", binding.ID).Count(&remaining).Error; err != nil || remaining != 1 {
		t.Fatalf("remaining bindings = %d, %v", remaining, err)
	}
}

func TestDeploymentTargetDeleteFinalizeConflictConvergesToDeleteFailed(t *testing.T) {
	db := openApplicationDeleteTestDB(t, "deployment_target_delete_finalize_test")
	project, target, _ := seedDeletingDeploymentTarget(t, db)
	runner := newRunner(db, Options{})
	// 模拟清理运行资源期间并发写入的入站关系：finalize 必须立即落 delete_failed，而不是在 deleting 中等待重试。
	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		api := model.Application{ID: "app_late", ProjectID: project.ID, Identifier: "late", Name: "Late", DeleteStatus: "active"}
		apiTarget := model.DeploymentTarget{ID: "dplt_late", ProjectID: project.ID, ApplicationID: api.ID, Name: "default", Stage: "prod", ClusterID: target.ClusterID}
		binding := model.ServiceBinding{
			ID: "sbind_late", ProjectID: project.ID, SourceApplicationID: api.ID, SourceDeploymentTargetID: apiTarget.ID,
			TargetApplicationID: target.ApplicationID, TargetDeploymentTargetID: target.ID, TargetPortName: "http", TargetPort: 5432,
			Protocol: "tcp", InjectionMode: "host_port", HostEnvVar: "DB_HOST", PortEnvVar: "DB_PORT", Enabled: true, CreatedBy: "usr_demo",
		}
		for _, row := range []any{&api, &apiTarget, &binding} {
			if err := db.FirstOrCreate(row).Error; err != nil {
				t.Fatalf("create late fixture %T: %v", row, err)
			}
		}
		return &cleanupObservationManager{}, nil
	}

	err := runner.handleResourceCleanup(t.Context(), deploymentTargetCleanupTask(t, target))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("handleResourceCleanup() error = %v, want non-retryable finalize conflict", err)
	}
	var stored model.DeploymentTarget
	if err := db.First(&stored, "id = ?", target.ID).Error; err != nil {
		t.Fatalf("load deployment target: %v", err)
	}
	if stored.DeleteStatus != "delete_failed" || stored.DeleteCode != deploymentTargetDeleteCodeServiceBindingInUse {
		t.Fatalf("stored deployment target = %#v", stored)
	}
}

func TestDeploymentTargetDeleteCleanupFailureRecordsStableCode(t *testing.T) {
	db := openApplicationDeleteTestDB(t, "deployment_target_delete_cleanup_failed_test")
	_, target, _ := seedDeletingDeploymentTarget(t, db)
	runner := newRunner(db, Options{})
	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		return nil, errors.New("cluster unreachable")
	}

	// 直接调用没有队列元数据，按最后一次尝试处理：重试耗尽后必须落稳定码，而不是停留在 deleting。
	if err := runner.handleResourceCleanup(t.Context(), deploymentTargetCleanupTask(t, target)); err == nil {
		t.Fatal("handleResourceCleanup() error = nil, want cleanup failure")
	}
	var stored model.DeploymentTarget
	if err := db.First(&stored, "id = ?", target.ID).Error; err != nil {
		t.Fatalf("load deployment target: %v", err)
	}
	if stored.DeleteStatus != "delete_failed" || stored.DeleteCode != deploymentTargetDeleteCodeCleanupFailed || stored.DeleteMessage == "" {
		t.Fatalf("stored deployment target = %#v", stored)
	}
}

func TestDeploymentTargetDeleteWithoutIncomingBindingsFinishes(t *testing.T) {
	db := openApplicationDeleteTestDB(t, "deployment_target_delete_success_test")
	project, target, route := seedDeletingDeploymentTarget(t, db)
	disabled := model.ServiceBinding{
		ID: "sbind_disabled", ProjectID: project.ID, SourceApplicationID: target.ApplicationID, SourceDeploymentTargetID: target.ID,
		TargetApplicationID: target.ApplicationID, TargetDeploymentTargetID: target.ID, TargetPortName: "http", TargetPort: 5432,
		Protocol: "tcp", InjectionMode: "host_port", HostEnvVar: "DB_HOST", PortEnvVar: "DB_PORT", Enabled: true, CreatedBy: "usr_demo",
	}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if err := db.Model(&model.ServiceBinding{}).Where("id = ?", disabled.ID).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable binding: %v", err)
	}
	runner := newRunner(db, Options{})
	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		return &cleanupObservationManager{}, nil
	}

	if err := runner.handleResourceCleanup(t.Context(), deploymentTargetCleanupTask(t, target)); err != nil {
		t.Fatalf("handleResourceCleanup() error = %v", err)
	}
	var stored model.DeploymentTarget
	if err := db.Unscoped().First(&stored, "id = ?", target.ID).Error; err != nil {
		t.Fatalf("load deployment target: %v", err)
	}
	if stored.DeleteStatus != "deleted" || stored.DeleteCode != "" || !stored.DeletedAt.Valid {
		t.Fatalf("stored deployment target = %#v", stored)
	}
	var routes int64
	if err := db.Model(&model.GatewayRoute{}).Where("id = ?", route.ID).Count(&routes).Error; err != nil || routes != 0 {
		t.Fatalf("remaining gateway routes = %d, %v", routes, err)
	}
	var bindings int64
	if err := db.Model(&model.ServiceBinding{}).Where("id = ?", disabled.ID).Count(&bindings).Error; err != nil || bindings != 0 {
		t.Fatalf("remaining disabled bindings = %d, %v", bindings, err)
	}
}
