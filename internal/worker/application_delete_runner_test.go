package worker

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func openApplicationDeleteTestDB(t *testing.T, prefix string) *gorm.DB {
	t.Helper()
	return testdb.Open(t, testdb.Options{
		SchemaPrefix: prefix,
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{}, &model.RuntimeCluster{}, &model.Application{}, &model.DeploymentTarget{},
				&model.DeploymentVolumeMount{}, &model.ServiceBinding{}, &model.ProjectTopologyEdge{}, &model.NetworkServiceShare{},
				&model.DeploymentTargetHookBinding{}, &model.GatewayRoute{}, &model.RepositoryBinding{},
			)
		},
	})
}

func applicationDeleteTask(t *testing.T, payload tasks.ApplicationDeletePayload) *asynq.Task {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return asynq.NewTask(tasks.TypeApplicationDelete, data)
}

func seedApplicationDeleteFixture(t *testing.T, db *gorm.DB) (model.Project, model.Application, model.DeploymentTarget) {
	t.Helper()
	startedAt := time.Now()
	project := model.Project{ID: "prj_appdel", Identifier: "appdel", Name: "App delete", KubernetesNamespace: "ns-appdel"}
	cluster := model.RuntimeCluster{ID: "clu_appdel", Name: "Primary", Scope: "global", IsDefault: true, DeleteStatus: "active"}
	database := model.Application{ID: "app_db", ProjectID: project.ID, Identifier: "db", Name: "Database", DeleteStatus: "deleting", DeleteStartedAt: &startedAt}
	target := model.DeploymentTarget{
		ID: "dplt_db", ProjectID: project.ID, ApplicationID: database.ID, Name: "default", Stage: "prod", ClusterID: cluster.ID,
		KubernetesName: "db-prod", ServicePorts: model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 5432}}, 5432),
	}
	for _, row := range []any{&project, &cluster, &database, &target} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create fixture %T: %v", row, err)
		}
	}
	return project, database, target
}

func TestApplicationDeleteWithIncomingBindingFailsBeforeRuntimeCleanup(t *testing.T) {
	db := openApplicationDeleteTestDB(t, "application_delete_incoming_test")
	project, database, target := seedApplicationDeleteFixture(t, db)
	api := model.Application{ID: "app_api", ProjectID: project.ID, Identifier: "api", Name: "API", DeleteStatus: "active"}
	apiTarget := model.DeploymentTarget{ID: "dplt_api", ProjectID: project.ID, ApplicationID: api.ID, Name: "default", Stage: "prod", ClusterID: target.ClusterID}
	binding := model.ServiceBinding{
		ID: "sbind_api_db", ProjectID: project.ID, SourceApplicationID: api.ID, SourceDeploymentTargetID: apiTarget.ID,
		TargetApplicationID: database.ID, TargetDeploymentTargetID: target.ID, TargetPortName: "http", TargetPort: 5432,
		Protocol: "tcp", InjectionMode: "host_port", HostEnvVar: "DB_HOST", PortEnvVar: "DB_PORT", Enabled: true, CreatedBy: "usr_demo",
	}
	for _, row := range []any{&api, &apiTarget, &binding} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create fixture %T: %v", row, err)
		}
	}
	runner := newRunner(db, Options{})
	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		t.Fatal("runtime resources were cleaned up although the application is still referenced")
		return nil, nil
	}

	err := runner.handleApplicationDelete(t.Context(), applicationDeleteTask(t, tasks.ApplicationDeletePayload{ApplicationID: database.ID, ProjectID: project.ID}))
	if !errors.Is(err, asynq.SkipRetry) || !errors.Is(err, errApplicationReferencedByServiceBinding) {
		t.Fatalf("handleApplicationDelete() error = %v, want non-retryable binding conflict", err)
	}
	var stored model.Application
	if err := db.First(&stored, "id = ?", database.ID).Error; err != nil {
		t.Fatalf("load application: %v", err)
	}
	if stored.DeleteStatus != "delete_failed" || stored.DeleteCode != applicationDeleteCodeServiceBindingInUse || stored.DeleteFinishedAt == nil {
		t.Fatalf("stored application = %#v", stored)
	}
}

func TestApplicationDeleteFinalizeConflictConvergesToDeleteFailed(t *testing.T) {
	db := openApplicationDeleteTestDB(t, "application_delete_finalize_test")
	project, database, target := seedApplicationDeleteFixture(t, db)
	runner := newRunner(db, Options{})
	// 模拟清理运行资源期间并发写入的入站关系：finalize 必须落 delete_failed 而不是保持 deleting 等待无限重投。
	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		api := model.Application{ID: "app_late", ProjectID: project.ID, Identifier: "late", Name: "Late", DeleteStatus: "active"}
		apiTarget := model.DeploymentTarget{ID: "dplt_late", ProjectID: project.ID, ApplicationID: api.ID, Name: "default", Stage: "prod", ClusterID: target.ClusterID}
		binding := model.ServiceBinding{
			ID: "sbind_late", ProjectID: project.ID, SourceApplicationID: api.ID, SourceDeploymentTargetID: apiTarget.ID,
			TargetApplicationID: database.ID, TargetDeploymentTargetID: target.ID, TargetPortName: "http", TargetPort: 5432,
			Protocol: "tcp", InjectionMode: "host_port", HostEnvVar: "DB_HOST", PortEnvVar: "DB_PORT", Enabled: true, CreatedBy: "usr_demo",
		}
		for _, row := range []any{&api, &apiTarget, &binding} {
			if err := db.FirstOrCreate(row).Error; err != nil {
				t.Fatalf("create late fixture %T: %v", row, err)
			}
		}
		return &cleanupObservationManager{}, nil
	}

	err := runner.handleApplicationDelete(t.Context(), applicationDeleteTask(t, tasks.ApplicationDeletePayload{ApplicationID: database.ID, ProjectID: project.ID}))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("handleApplicationDelete() error = %v, want non-retryable finalize conflict", err)
	}
	var stored model.Application
	if err := db.First(&stored, "id = ?", database.ID).Error; err != nil {
		t.Fatalf("load application: %v", err)
	}
	if stored.DeleteStatus != "delete_failed" || stored.DeleteCode != applicationDeleteCodeServiceBindingInUse {
		t.Fatalf("stored application = %#v", stored)
	}
}

func TestApplicationDeleteIgnoresBindingsBetweenItsOwnTargets(t *testing.T) {
	db := openApplicationDeleteTestDB(t, "application_delete_self_binding_test")
	project, database, target := seedApplicationDeleteFixture(t, db)
	replica := model.DeploymentTarget{ID: "dplt_db_worker", ProjectID: project.ID, ApplicationID: database.ID, Name: "worker", Stage: "dev", ClusterID: target.ClusterID}
	binding := model.ServiceBinding{
		ID: "sbind_self", ProjectID: project.ID, SourceApplicationID: database.ID, SourceDeploymentTargetID: replica.ID,
		TargetApplicationID: database.ID, TargetDeploymentTargetID: target.ID, TargetPortName: "http", TargetPort: 5432,
		Protocol: "tcp", InjectionMode: "host_port", HostEnvVar: "DB_HOST", PortEnvVar: "DB_PORT", Enabled: true, CreatedBy: "usr_demo",
	}
	for _, row := range []any{&replica, &binding} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create fixture %T: %v", row, err)
		}
	}
	runner := newRunner(db, Options{})
	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		return &cleanupObservationManager{}, nil
	}

	if err := runner.handleApplicationDelete(t.Context(), applicationDeleteTask(t, tasks.ApplicationDeletePayload{ApplicationID: database.ID, ProjectID: project.ID})); err != nil {
		t.Fatalf("handleApplicationDelete() error = %v", err)
	}
	var stored model.Application
	if err := db.Unscoped().First(&stored, "id = ?", database.ID).Error; err != nil {
		t.Fatalf("load application: %v", err)
	}
	if stored.DeleteStatus != "deleted" || !stored.DeletedAt.Valid {
		t.Fatalf("stored application = %#v", stored)
	}
	var remaining int64
	if err := db.Model(&model.ServiceBinding{}).Where("target_application_id = ?", database.ID).Count(&remaining).Error; err != nil || remaining != 0 {
		t.Fatalf("remaining bindings = %d, %v", remaining, err)
	}
}

type recordingReconcileEnqueuer struct{ projects []string }

func (e *recordingReconcileEnqueuer) EnqueueNetworkPolicyReconcile(_ context.Context, payload tasks.NetworkPolicyReconcilePayload) (*asynq.TaskInfo, error) {
	e.projects = append(e.projects, payload.ProjectID)
	return &asynq.TaskInfo{}, nil
}

// 删除应用、部署配置或项目空间时，相关共享随之删除，并立即重新下发对端项目空间的策略。
func TestDeletesRemoveNetworkSharesAndResyncPeers(t *testing.T) {
	db := openApplicationDeleteTestDB(t, "delete_network_shares_test")
	if err := db.AutoMigrate(&model.ProjectVolume{}, &model.NetworkEgressException{}, &model.NetworkEgressExceptionGrant{}); err != nil {
		t.Fatal(err)
	}
	seed := []any{
		&model.Project{ID: "prj_share_provider", Identifier: "provider", Name: "Provider", DeleteStatus: "active"},
		&model.Project{ID: "prj_share_consumer", Identifier: "consumer", Name: "Consumer", DeleteStatus: "deleting"},
		&model.Application{ID: "app_share_db", ProjectID: "prj_share_provider", Identifier: "db", Name: "db"},
		&model.Application{ID: "app_share_cache", ProjectID: "prj_share_provider", Identifier: "cache", Name: "cache"},
		&model.DeploymentTarget{ID: "dplt_share_db", ProjectID: "prj_share_provider", ApplicationID: "app_share_db", Name: "prod", Stage: "prod"},
		&model.DeploymentTarget{ID: "dplt_share_cache", ProjectID: "prj_share_provider", ApplicationID: "app_share_cache", Name: "prod", Stage: "prod"},
		&model.DeploymentTarget{ID: "dplt_share_other", ProjectID: "prj_share_provider", ApplicationID: "app_share_cache", Name: "staging", Stage: "staging"},
		&model.NetworkServiceShare{ID: "nss_db", ProviderProjectID: "prj_share_provider", DeploymentTargetID: "dplt_share_db", ConsumerProjectID: "prj_share_consumer"},
		&model.NetworkServiceShare{ID: "nss_cache", ProviderProjectID: "prj_share_provider", DeploymentTargetID: "dplt_share_cache", ConsumerProjectID: "prj_share_consumer"},
		&model.NetworkServiceShare{ID: "nss_other", ProviderProjectID: "prj_share_provider", DeploymentTargetID: "dplt_share_other", ConsumerProjectID: "prj_share_consumer"},
	}
	for _, row := range seed {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
	remaining := func() []string {
		var ids []string
		db.Model(&model.NetworkServiceShare{}).Order("id").Pluck("id", &ids)
		return ids
	}
	enqueuer := &recordingReconcileEnqueuer{}
	runner := &Runner{db: db, networkPolicyReconcileEnqueuer: enqueuer}
	ctx := context.Background()

	var target model.DeploymentTarget
	db.First(&target, "id = ?", "dplt_share_other")
	if err := runner.finishDeploymentTargetDelete(ctx, target); err != nil {
		t.Fatal(err)
	}
	if got := remaining(); !slices.Equal(got, []string{"nss_cache", "nss_db"}) {
		t.Fatalf("after target delete = %v", got)
	}
	var app model.Application
	db.First(&app, "id = ?", "app_share_db")
	if err := runner.finishApplicationDelete(ctx, app); err != nil {
		t.Fatal(err)
	}
	if got := remaining(); !slices.Equal(got, []string{"nss_cache"}) {
		t.Fatalf("after application delete = %v", got)
	}
	if !slices.Contains(enqueuer.projects, "prj_share_consumer") || !slices.Contains(enqueuer.projects, "prj_share_provider") {
		t.Fatalf("both sides must be resynced, got %v", enqueuer.projects)
	}
	enqueuer.projects = nil
	if err := runner.finishProjectDelete(ctx, model.Project{ID: "prj_share_consumer", DeleteStatus: "deleting"}); err != nil {
		t.Fatal(err)
	}
	if got := remaining(); len(got) != 0 {
		t.Fatalf("after consumer project delete = %v", got)
	}
	if !slices.Equal(enqueuer.projects, []string{"prj_share_provider"}) {
		t.Fatalf("only the surviving provider must be resynced, got %v", enqueuer.projects)
	}
}
