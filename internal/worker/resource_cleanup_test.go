package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestStaleResourceCleanupPayloadsOnlyRecoverTimedOutDeletingRows(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "resource_cleanup_recovery_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{},
				&model.DeploymentTarget{},
				&model.GatewayRoute{},
				&model.ProjectRuntimeConfigSet{},
				&model.RuntimeCluster{},
			)
		},
	})
	oldStartedAt := time.Now().Add(-2 * resourceCleanupRecoveryAfter)
	recentStartedAt := time.Now()
	rows := []model.Project{
		{ID: "prj_stale", Identifier: "stale-project", Name: "Stale", DeleteStatus: "deleting", DeleteStartedAt: &oldStartedAt},
		{ID: "prj_recent", Identifier: "recent-project", Name: "Recent", DeleteStatus: "deleting", DeleteStartedAt: &recentStartedAt},
		{ID: "prj_failed", Identifier: "failed-project", Name: "Failed", DeleteStatus: "delete_failed", DeleteStartedAt: &oldStartedAt},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create cleanup fixtures: %v", err)
	}
	if err := db.Create(&model.RuntimeCluster{
		ID: "rcl_stale", Name: "Stale cluster", DeleteStatus: "deleting", DeleteStartedAt: &oldStartedAt,
	}).Error; err != nil {
		t.Fatalf("create stale runtime cluster: %v", err)
	}

	payloads, err := (&Runner{db: db}).staleResourceCleanupPayloads(t.Context(), time.Now().Add(-resourceCleanupRecoveryAfter))
	if err != nil {
		t.Fatalf("staleResourceCleanupPayloads() error = %v", err)
	}
	if len(payloads) != 2 || payloads[0].ResourceType != "project" || payloads[0].ResourceID != "prj_stale" || payloads[0].ActorID != "system:cleanup-recovery" || payloads[1].ResourceType != "runtime_cluster" || payloads[1].ResourceID != "rcl_stale" {
		t.Fatalf("recovery payloads = %#v", payloads)
	}
}

func TestStaleApplicationDeletePayloadsOnlyRecoverTimedOutDeletingRows(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "application_delete_recovery_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Application{})
		},
	})
	oldStartedAt := time.Now().Add(-2 * resourceCleanupRecoveryAfter)
	recentStartedAt := time.Now()
	applications := []model.Application{
		{ID: "app_stale", ProjectID: "prj_stale", Identifier: "stale", Name: "Stale", DeleteStatus: "deleting", DeleteStartedAt: &oldStartedAt},
		{ID: "app_recent", ProjectID: "prj_recent", Identifier: "recent", Name: "Recent", DeleteStatus: "deleting", DeleteStartedAt: &recentStartedAt},
		{ID: "app_failed", ProjectID: "prj_failed", Identifier: "failed", Name: "Failed", DeleteStatus: "delete_failed", DeleteStartedAt: &oldStartedAt},
	}
	if err := db.Create(&applications).Error; err != nil {
		t.Fatalf("create application deletion fixtures: %v", err)
	}

	payloads, err := (&Runner{db: db}).staleApplicationDeletePayloads(t.Context(), time.Now().Add(-resourceCleanupRecoveryAfter))
	if err != nil {
		t.Fatalf("staleApplicationDeletePayloads() error = %v", err)
	}
	want := []tasks.ApplicationDeletePayload{{
		ApplicationID: "app_stale",
		ProjectID:     "prj_stale",
		ActorID:       "system:cleanup-recovery",
	}}
	if len(payloads) != len(want) || payloads[0] != want[0] {
		t.Fatalf("application recovery payloads = %#v, want %#v", payloads, want)
	}
}

func TestProjectCleanupClusterTargetsIncludeEveryActiveCluster(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "project_cleanup_clusters_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{})
		},
	})
	clusters := []model.RuntimeCluster{
		{ID: "clu_default", Name: "Default", Scope: "global", IsDefault: true, DeleteStatus: "active"},
		{ID: "clu_project", Name: "Project", Scope: "project", DeleteStatus: "active"},
		{ID: "clu_deleting", Name: "Deleting", Scope: "global", DeleteStatus: "deleting"},
	}
	if err := db.Create(&clusters).Error; err != nil {
		t.Fatalf("create runtime clusters: %v", err)
	}

	targets, err := (&Runner{db: db}).projectCleanupClusterTargets(t.Context(), "prj_cleanup")
	if err != nil {
		t.Fatalf("projectCleanupClusterTargets() error = %v", err)
	}
	clusterIDs := make(map[string]bool, len(targets))
	for _, target := range targets {
		clusterIDs[target.ClusterID] = true
	}
	if !clusterIDs["clu_default"] || !clusterIDs["clu_project"] || clusterIDs["clu_deleting"] || len(clusterIDs) != 2 {
		t.Fatalf("cleanup cluster IDs = %#v", clusterIDs)
	}
}

func TestProjectCleanupClusterTargetsRequireAnObservableCluster(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "project_cleanup_no_cluster_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{}, &model.DeploymentTarget{}, &model.ProjectVolume{})
		},
	})
	target := model.DeploymentTarget{
		ID: "dplt_history", ProjectID: "prj_empty", ApplicationID: "app_history", Name: "History", Stage: "prod",
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create historical runtime reference: %v", err)
	}
	if err := db.Delete(&target).Error; err != nil {
		t.Fatalf("soft-delete historical runtime reference: %v", err)
	}
	if _, err := (&Runner{db: db}).projectCleanupClusterTargets(t.Context(), "prj_empty"); err == nil {
		t.Fatal("project cleanup without an observable runtime cluster must fail")
	}
}

func TestProjectCleanupWithoutClustersOrRuntimeReferencesCanFinish(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "empty_project_cleanup_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{}, &model.DeploymentTarget{}, &model.ProjectVolume{})
		},
	})
	targets, err := (&Runner{db: db}).projectCleanupClusterTargets(t.Context(), "prj_empty")
	if err != nil {
		t.Fatalf("projectCleanupClusterTargets() error = %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("cleanup targets = %#v, want none", targets)
	}
}

func TestFinishProjectDeleteSoftDeletesRuntimeDesiredState(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "finish_project_delete_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.ProjectVolume{},
				&model.DeploymentVolumeMount{},
				&model.NetworkEgressException{},
				&model.NetworkEgressExceptionGrant{},
				&model.NetworkServiceShare{},
				&model.ScopedResourceProjectBinding{},
				&model.NotificationChannel{},
				&model.NotificationRule{},
				&model.SecretValue{},
				&model.DeploymentTargetHookBinding{},
				&model.GatewayRoute{},
				&model.RepositoryBinding{},
			)
		},
	})
	project := model.Project{
		ID: "prj_delete", Identifier: "delete", Name: "Delete", DeleteStatus: "deleting",
	}
	application := model.Application{
		ID: "app_delete", ProjectID: project.ID, Identifier: "delete", Name: "Delete",
	}
	target := model.DeploymentTarget{
		ID: "dplt_delete", ProjectID: project.ID, ApplicationID: application.ID, Name: "Production", Stage: "prod",
	}
	volume := model.ProjectVolume{
		ID: "pvol_delete", ProjectID: project.ID, DisplayName: "Data", ClusterID: "clu_delete",
		Namespace: "ns-delete", ClaimName: "data", OwnershipMode: model.ProjectVolumeOwnershipManaged,
		SourceKind: model.ProjectVolumeSourceBlank, LifecycleState: model.ProjectVolumeLifecycleReady,
		CapacityRequest: "1Gi", CapacityBytes: 1 << 30, AccessMode: model.ProjectVolumeAccessReadWriteOnce,
		VolumeMode: model.ProjectVolumeModeFilesystem, CreatedBy: "usr_delete",
	}
	volumeID := volume.ID
	mount := model.DeploymentVolumeMount{
		ID: "dvm_delete", ProjectID: project.ID, ApplicationID: application.ID,
		DeploymentTargetID: target.ID, SourceType: model.DeploymentVolumeSourceProjectVolume,
		ProjectVolumeID: &volumeID, LogicalName: "data",
	}
	fixtures := []struct {
		name string
		row  any
	}{
		{name: "project", row: &project},
		{name: "application", row: &application},
		{name: "deployment target", row: &target},
		{name: "project volume", row: &volume},
		{name: "deployment volume mount", row: &mount},
	}
	for _, fixture := range fixtures {
		if err := db.Create(fixture.row).Error; err != nil {
			t.Fatalf("create %s: %v", fixture.name, err)
		}
	}

	if err := (&Runner{db: db}).finishProjectDelete(context.Background(), project); err != nil {
		t.Fatalf("finishProjectDelete() error = %v", err)
	}

	for name, row := range map[string]any{
		"project": &model.Project{}, "application": &model.Application{},
		"deployment target": &model.DeploymentTarget{}, "project volume": &model.ProjectVolume{},
		"deployment volume mount": &model.DeploymentVolumeMount{},
	} {
		var count int64
		if err := db.Model(row).Count(&count).Error; err != nil {
			t.Fatalf("count active %s: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("active %s count = %d, want 0", name, count)
		}
	}

	var deletedVolume model.ProjectVolume
	if err := db.Unscoped().First(&deletedVolume, "id = ?", volume.ID).Error; err != nil {
		t.Fatalf("load deleted project volume: %v", err)
	}
	if !deletedVolume.DeletedAt.Valid {
		t.Fatal("project volume deleted_at was not set")
	}
	var deletedTarget model.DeploymentTarget
	if err := db.Unscoped().First(&deletedTarget, "id = ?", target.ID).Error; err != nil {
		t.Fatalf("load deleted deployment target: %v", err)
	}
	if !deletedTarget.DeletedAt.Valid || deletedTarget.DeleteStatus != "deleted" || deletedTarget.DeleteFinishedAt == nil {
		t.Fatalf("deleted deployment target = %#v", deletedTarget)
	}
	var deletedApplication model.Application
	if err := db.Unscoped().First(&deletedApplication, "id = ?", application.ID).Error; err != nil {
		t.Fatalf("load deleted application: %v", err)
	}
	if !deletedApplication.DeletedAt.Valid || deletedApplication.DeleteStatus != "deleted" || deletedApplication.DeleteFinishedAt == nil {
		t.Fatalf("deleted application = %#v", deletedApplication)
	}
	var deletedMount model.DeploymentVolumeMount
	if err := db.Unscoped().First(&deletedMount, "id = ?", mount.ID).Error; err != nil {
		t.Fatalf("load deleted deployment volume mount: %v", err)
	}
	if !deletedMount.DeletedAt.Valid {
		t.Fatal("deployment volume mount deleted_at was not set")
	}
}

type cleanupObservationManager struct {
	fakeNamespaceManager
	lists     [][]kubeprovider.ResourceSnapshot
	deletions []string
	options   []kubeprovider.ResourceListOptions
	releases  []kubeprovider.GatewayCertificateRelease
	deleteErr error
}

func (manager *cleanupObservationManager) ReleaseGatewayCertificate(_ context.Context, release kubeprovider.GatewayCertificateRelease) (kubeprovider.GatewayCertificateReleaseResult, error) {
	manager.releases = append(manager.releases, release)
	return kubeprovider.GatewayCertificateReleaseResult{}, nil
}

func (manager *cleanupObservationManager) ListManagedResources(_ context.Context, options kubeprovider.ResourceListOptions) ([]kubeprovider.ResourceSnapshot, error) {
	manager.options = append(manager.options, options)
	if len(manager.lists) == 0 {
		return nil, nil
	}
	items := manager.lists[0]
	manager.lists = manager.lists[1:]
	return items, nil
}

func (manager *cleanupObservationManager) DeleteManagedResource(_ context.Context, kind, namespace, name string) error {
	manager.deletions = append(manager.deletions, kind+"/"+namespace+"/"+name)
	return manager.deleteErr
}

func TestApplicationCleanupCoversDeletedImplicitTargetAcrossDefaultChange(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "application_cleanup_clusters_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{},
				&model.RuntimeCluster{},
				&model.DeploymentTarget{},
				&model.DeploymentVolumeMount{},
				&model.GatewayRoute{},
			)
		},
	})
	project := model.Project{ID: "prj_cleanup", Identifier: "cleanup", Name: "Cleanup", KubernetesNamespace: "ns-cleanup"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	clusters := []model.RuntimeCluster{
		{ID: "clu_old_default", Name: "Old default", Scope: "global", DeleteStatus: "active"},
		{ID: "clu_new_default", Name: "New default", Scope: "global", IsDefault: true, DeleteStatus: "active"},
	}
	if err := db.Create(&clusters).Error; err != nil {
		t.Fatalf("create runtime clusters: %v", err)
	}
	target := model.DeploymentTarget{
		ID: "dplt_implicit", ProjectID: project.ID, ApplicationID: "app_cleanup", Name: "Production", Stage: "prod",
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create implicit target: %v", err)
	}
	if err := db.Delete(&target).Error; err != nil {
		t.Fatalf("soft-delete implicit target: %v", err)
	}
	route := model.GatewayRoute{
		ID: "gwr_cleanup", ProjectID: project.ID, ApplicationID: target.ApplicationID, DeploymentTargetID: target.ID,
		Host: "cleanup.example.com", TLSMode: "http-challenge",
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create gateway route: %v", err)
	}

	runner := newRunner(db, Options{})
	managers := map[string]*cleanupObservationManager{}
	runner.kubernetesManagerFactory = func(target model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		manager := managers[target.ClusterID]
		if manager == nil {
			manager = &cleanupObservationManager{}
			managers[target.ClusterID] = manager
		}
		return manager, nil
	}
	payload := tasks.ApplicationDeletePayload{ApplicationID: target.ApplicationID, ProjectID: project.ID}
	if err := runner.cleanupApplicationRuntimeResources(t.Context(), payload); err != nil {
		t.Fatalf("cleanupApplicationRuntimeResources() error = %v", err)
	}
	for _, clusterID := range []string{"clu_old_default", "clu_new_default"} {
		manager := managers[clusterID]
		if manager == nil || len(manager.options) != 6 {
			t.Fatalf("cluster %s resource observations = %#v", clusterID, manager)
		}
		for _, options := range manager.options {
			if options.ProjectID != project.ID || options.ApplicationID != target.ApplicationID || options.Namespace != project.KubernetesNamespace {
				t.Fatalf("cluster %s resource filter = %#v", clusterID, options)
			}
		}
		if len(manager.releases) != 1 || manager.releases[0].Name != gatewayRuntimeName(route) || manager.releases[0].SecretName != gatewayTLSSecretName(route) {
			t.Fatalf("cluster %s certificate releases = %#v", clusterID, manager.releases)
		}
	}
	if managers[""] == nil {
		t.Fatal("soft-deleted implicit target was not loaded for volume mount release")
	}
}

func TestDeleteManagedResourcesDoesNotConfirmBeforeResourcesDisappear(t *testing.T) {
	resource := kubeprovider.ResourceSnapshot{Kind: "Deployment", Namespace: "ns-demo", Name: "api"}
	manager := &cleanupObservationManager{lists: [][]kubeprovider.ResourceSnapshot{{resource}, {resource}}}
	err := deleteManagedResourcesAndConfirm(t.Context(), manager, kubeprovider.ResourceListOptions{Kind: "workloads", Namespace: "ns-demo"})
	if err == nil || !strings.Contains(err.Error(), "still deleting") {
		t.Fatalf("deleteManagedResourcesAndConfirm() error = %v", err)
	}
	if len(manager.deletions) != 1 || manager.deletions[0] != "Deployment/ns-demo/api" {
		t.Fatalf("resource deletions = %#v", manager.deletions)
	}
}

func TestDeleteManagedNamespaceDoesNotConfirmBeforeNamespaceDisappears(t *testing.T) {
	namespace := kubeprovider.ResourceSnapshot{Kind: "Namespace", Name: "ns-demo", ProjectID: "prj_demo"}
	manager := &cleanupObservationManager{lists: [][]kubeprovider.ResourceSnapshot{{namespace}, {namespace}}}
	err := deleteManagedNamespace(t.Context(), manager, namespace.Name, namespace.ProjectID)
	if err == nil || !strings.Contains(err.Error(), "still in progress") {
		t.Fatalf("deleteManagedNamespace() error = %v", err)
	}
	if len(manager.deletions) != 1 || manager.deletions[0] != "Namespace//ns-demo" {
		t.Fatalf("namespace deletions = %#v", manager.deletions)
	}
}

func TestDeleteManagedNamespaceSkipsNamespaceOwnedByAnotherProject(t *testing.T) {
	namespace := kubeprovider.ResourceSnapshot{Kind: "Namespace", Name: "ns-demo", ProjectID: "prj_other"}
	manager := &cleanupObservationManager{lists: [][]kubeprovider.ResourceSnapshot{{namespace}}}
	if err := deleteManagedNamespace(t.Context(), manager, namespace.Name, "prj_demo"); err != nil {
		t.Fatalf("deleteManagedNamespace() error = %v", err)
	}
	if len(manager.deletions) != 0 {
		t.Fatalf("namespace deletions = %#v", manager.deletions)
	}
	if len(manager.options) != 1 || manager.options[0].ProjectID != "prj_demo" {
		t.Fatalf("namespace ownership filter = %#v", manager.options)
	}
}

func TestFinishRuntimeClusterDeleteRechecksActiveProjectReferences(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_cluster_cleanup_refs_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.RuntimeCluster{}, &model.DeploymentTarget{}, &model.ProjectVolume{})
		},
	})
	project := model.Project{ID: "prj_runtime_refs", Identifier: "runtime-refs", Name: "Runtime refs"}
	cluster := model.RuntimeCluster{ID: "clu_runtime_refs", Name: "Runtime refs", Scope: "global", DeleteStatus: "deleting"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatalf("create runtime cluster: %v", err)
	}
	if err := db.Create(&model.DeploymentTarget{
		ID: "dplt_runtime_refs", ProjectID: project.ID, ApplicationID: "app_runtime_refs",
		Name: "Production", Stage: "prod", ClusterID: cluster.ID,
	}).Error; err != nil {
		t.Fatalf("create deployment target: %v", err)
	}
	if err := db.Create(&model.ProjectVolume{
		ID: "pvol_runtime_refs", ProjectID: project.ID, DisplayName: "data", ClusterID: cluster.ID,
		Namespace: "luna-runtime-refs", ClaimName: "data", OwnershipMode: model.ProjectVolumeOwnershipManaged,
		SourceKind: model.ProjectVolumeSourceBlank, LifecycleState: model.ProjectVolumeLifecycleReady,
		CapacityRequest: "1Gi", CapacityBytes: 1024 * 1024 * 1024,
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		CreatedBy: "usr_test",
	}).Error; err != nil {
		t.Fatalf("create project volume: %v", err)
	}

	err := newRunner(db, Options{}).finishRuntimeClusterDelete(t.Context(), cluster)
	if err == nil || !strings.Contains(err.Error(), "1 active deployment targets and 1 active project volumes") {
		t.Fatalf("finishRuntimeClusterDelete() error = %v", err)
	}
	var persisted model.RuntimeCluster
	if err := db.Unscoped().First(&persisted, "id = ?", cluster.ID).Error; err != nil {
		t.Fatalf("load runtime cluster after blocked deletion: %v", err)
	}
	if persisted.DeletedAt.Valid || persisted.DeleteStatus != "deleting" {
		t.Fatalf("runtime cluster changed after blocked deletion: %#v", persisted)
	}
}

func TestResourceCleanupAuditActionUsesStableResourceCategory(t *testing.T) {
	tests := map[string]string{
		"project":           "project.delete",
		"deployment_target": "deployment.delete",
		"gateway_route":     "gateway.delete",
		"runtime_config":    "runtime_config.delete",
		"runtime_cluster":   "runtime_cluster.delete",
		"unknown":           "",
	}
	for resourceType, want := range tests {
		if got := resourceCleanupAuditAction(resourceType); got != want {
			t.Fatalf("resourceCleanupAuditAction(%q) = %q, want %q", resourceType, got, want)
		}
	}
}

func TestResourceCleanupFailureAuditSurvivesCancelledTaskContext(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "resource_cleanup_audit_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.AuditLog{})
		},
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	payload := tasks.ResourceCleanupPayload{
		ResourceType: "deployment_target",
		ResourceID:   "dplt_audit",
		ProjectID:    "prj_audit",
		ActorID:      "usr_audit",
	}
	if err := (&Runner{db: db}).auditResourceCleanupFailure(ctx, payload); err != nil {
		t.Fatalf("auditResourceCleanupFailure() error = %v", err)
	}
	var audit model.AuditLog
	if err := db.First(&audit, "resource = ?", payload.ResourceID).Error; err != nil {
		t.Fatalf("load cleanup failure audit: %v", err)
	}
	if audit.UserID != payload.ActorID || audit.Action != "deployment.delete" || audit.Success || audit.Message != "cleanup_failed" {
		t.Fatalf("cleanup failure audit = %#v", audit)
	}
}

func TestResourceCleanupAttemptWithoutQueueMetadataIsTerminal(t *testing.T) {
	if !resourceCleanupAttemptExhausted(t.Context()) {
		t.Fatal("direct cleanup invocation must fail closed as a terminal attempt")
	}
}

func TestProjectCleanupReleasesGatewayCertificatesAfterNamespaceDeletion(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "project_cleanup_certificates_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{}, &model.GatewayRoute{})
		},
	})
	cluster := model.RuntimeCluster{ID: "clu_cert", Name: "Cert", Scope: "global", IsDefault: true, DeleteStatus: "active", GatewayCertificateNamespace: "cert-system"}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatalf("create runtime cluster: %v", err)
	}
	project := model.Project{ID: "prj_cert_cleanup", Identifier: "cert-cleanup", KubernetesNamespace: "luna-cert-cleanup"}
	route := model.GatewayRoute{ID: "gwr_project", ProjectID: project.ID, ApplicationID: "app_cert", DeploymentTargetID: "dplt_cert", Host: "project.example.com", TLSMode: "http-challenge"}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create gateway route: %v", err)
	}
	ownedNamespace := kubeprovider.ResourceSnapshot{Kind: "Namespace", Name: project.KubernetesNamespace, ProjectID: project.ID}
	manager := &cleanupObservationManager{lists: [][]kubeprovider.ResourceSnapshot{{ownedNamespace}, nil}}
	runner := newRunner(db, Options{})
	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		return manager, nil
	}

	if err := runner.cleanupProjectNamespacesForDeploymentTargets(t.Context(), project, []model.DeploymentTarget{{ClusterID: cluster.ID}}, "usr_owner"); err != nil {
		t.Fatalf("cleanupProjectNamespacesForDeploymentTargets() error = %v", err)
	}
	if len(manager.deletions) != 1 || len(manager.releases) != 1 {
		t.Fatalf("namespace deletions = %#v, certificate releases = %#v", manager.deletions, manager.releases)
	}
	release := manager.releases[0]
	if release.Namespace != "cert-system" || release.Name != gatewayRuntimeName(route) || release.GatewayNamespace != "kube-system" {
		t.Fatalf("certificate release = %#v", release)
	}
}

func TestCleanupPrunesNotificationRulesScopedBindingsAndEgressGrantsFailClosed(t *testing.T) {
	db := openApplicationDeleteTestDB(t, "cleanup_dangling_refs_test")
	if err := db.AutoMigrate(
		&model.ProjectVolume{},
		&model.BuildVariableSet{},
		&model.ArtifactRegistry{},
		&model.RegistryCredential{},
		&model.GitProvider{},
		&model.GitAccount{},
		&model.SecretValue{},
	); err != nil {
		t.Fatal(err)
	}
	codec, err := secret.NewCodec("worker-cleanup-test-key")
	if err != nil {
		t.Fatal(err)
	}
	secrets := secret.NewStore(db, nil, codec)

	surviving := model.Project{ID: "prj_surviving", Identifier: "surviving", Name: "Surviving", DeleteStatus: "active"}
	doomed := model.Project{ID: "prj_doomed", Identifier: "doomed", Name: "Doomed", DeleteStatus: "deleting"}
	appOnly := model.Application{ID: "app_only", ProjectID: surviving.ID, Identifier: "only", Name: "Only", DeleteStatus: "deleting"}
	appKeep := model.Application{ID: "app_keep", ProjectID: surviving.ID, Identifier: "keep", Name: "Keep", DeleteStatus: "active"}
	appDoomed := model.Application{ID: "app_doomed", ProjectID: doomed.ID, Identifier: "doomed-app", Name: "Doomed App", DeleteStatus: "deleting"}
	targetOnly := model.DeploymentTarget{ID: "dplt_only", ProjectID: surviving.ID, ApplicationID: appOnly.ID, Name: "prod", Stage: "prod"}
	targetKeep := model.DeploymentTarget{ID: "dplt_keep", ProjectID: surviving.ID, ApplicationID: appKeep.ID, Name: "prod", Stage: "prod"}
	targetDoomed := model.DeploymentTarget{ID: "dplt_doomed", ProjectID: doomed.ID, ApplicationID: appDoomed.ID, Name: "prod", Stage: "prod"}
	cluster := model.RuntimeCluster{ID: "clu_ref", Name: "Primary", Scope: "global", IsDefault: true, DeleteStatus: "active"}

	orphanBvsSecretRef, err := secrets.StoreContextWithDB(t.Context(), db, "super-secret", "usr_owner", "build_variable_set:bvs_doomed_only:TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	orphanBvs := model.BuildVariableSet{
		ID: "bvs_doomed_only", Name: "Doomed Vars", Scope: "project",
		Variables: `{}`, SecretRefs: `{"TOKEN":"` + orphanBvsSecretRef + `"}`, Enabled: true,
	}
	orphanReg := model.ArtifactRegistry{ID: "reg_doomed_only", Name: "Doomed Reg", Provider: "harbor", Endpoint: "https://reg.doomed.local", Scope: "project"}
	orphanCredRef, err := secrets.StoreContextWithDB(t.Context(), db, "reg-pass", "usr_owner", "registry_credential:"+orphanReg.ID+":password")
	if err != nil {
		t.Fatal(err)
	}
	orphanCred := model.RegistryCredential{
		ID: "regc_doomed_only", RegistryID: orphanReg.ID, Name: "default", Username: "bot",
		PasswordRef: orphanCredRef, Usage: "push-pull", Scope: "project",
	}

	sharedCh := model.NotificationChannel{ID: "nch_shared", Scope: "project", Name: "Shared", AdapterKind: "webhook", ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true}
	orphanCh := model.NotificationChannel{ID: "nch_orphan", Scope: "project", Name: "Doomed Only", AdapterKind: "webhook", ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true}
	bindings := []model.ScopedResourceProjectBinding{
		{ID: "srpb_shared_1", ResourceType: "notification_channel", ResourceID: sharedCh.ID, ProjectID: surviving.ID},
		{ID: "srpb_shared_2", ResourceType: "notification_channel", ResourceID: sharedCh.ID, ProjectID: doomed.ID},
		{ID: "srpb_orphan_1", ResourceType: "notification_channel", ResourceID: orphanCh.ID, ProjectID: doomed.ID},
		{ID: "srpb_orphan_bvs", ResourceType: "build_variable_set", ResourceID: orphanBvs.ID, ProjectID: doomed.ID},
		{ID: "srpb_orphan_reg", ResourceType: "artifact_registry", ResourceID: orphanReg.ID, ProjectID: doomed.ID},
		{ID: "srpb_orphan_regc", ResourceType: "registry_credential", ResourceID: orphanCred.ID, ProjectID: doomed.ID},
	}

	rules := []model.NotificationRule{
		{
			ID: "nrl_app_narrow", ProjectID: surviving.ID, Name: "Only App Rule",
			EventTypesJSON: `["release.failed"]`,
			FilterJSON:     `{"scope":"projects","projectIds":["prj_surviving"],"applicationIds":["app_only"]}`,
			ChannelIDsJSON: `["nch_shared"]`, Enabled: true,
		},
		{
			ID: "nrl_multi_target", ProjectID: surviving.ID, Name: "Multi Target Rule",
			EventTypesJSON: `["release.failed"]`,
			FilterJSON:     `{"scope":"projects","projectIds":["prj_surviving"],"applicationIds":["app_only","app_keep"],"deploymentTargetIds":["dplt_only","dplt_keep"]}`,
			ChannelIDsJSON: `["nch_shared"]`, Enabled: true,
		},
		{
			ID: "nrl_global_multi", ProjectID: "", Name: "Global Multi Project",
			EventTypesJSON: `["release.failed"]`,
			FilterJSON:     `{"scope":"projects","projectIds":["prj_surviving","prj_doomed"],"applicationIds":["app_doomed"],"deploymentTargetIds":["dplt_doomed","dplt_keep"]}`,
			ChannelIDsJSON: `["nch_global"]`, Enabled: true,
		},
		{
			ID: "nrl_global_doomed_only", ProjectID: "", Name: "Global Doomed Only",
			EventTypesJSON: `["release.failed"]`,
			FilterJSON:     `{"scope":"projects","projectIds":["prj_doomed"]}`,
			ChannelIDsJSON: `["nch_global"]`, Enabled: true,
		},
	}

	excNarrow := model.NetworkEgressException{ID: "nex_narrow", RuntimeClusterID: cluster.ID, Name: "Narrow", CIDR: "10.10.0.0/16", Protocol: "tcp", Reason: "db"}
	excMulti := model.NetworkEgressException{ID: "nex_multi", RuntimeClusterID: cluster.ID, Name: "Multi", CIDR: "10.20.0.0/16", Protocol: "tcp", Reason: "cache"}
	grants := []model.NetworkEgressExceptionGrant{
		{ID: "nexg_narrow", ExceptionID: excNarrow.ID, ProjectID: surviving.ID, DeploymentTargetIDs: targetOnly.ID},
		{ID: "nexg_multi", ExceptionID: excMulti.ID, ProjectID: surviving.ID, DeploymentTargetIDs: targetOnly.ID + "," + targetKeep.ID},
	}

	for _, row := range []any{
		&surviving, &doomed, &cluster, &appOnly, &appKeep, &appDoomed,
		&targetOnly, &targetKeep, &targetDoomed,
		&orphanBvs, &orphanReg, &orphanCred,
		&sharedCh, &orphanCh, &bindings, &rules, &excNarrow, &excMulti, &grants,
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	enqueuer := &recordingReconcileEnqueuer{}
	runner := &Runner{db: db, secrets: secrets, networkPolicyReconcileEnqueuer: enqueuer}
	if err := runner.finishApplicationDelete(t.Context(), appOnly); err != nil {
		t.Fatalf("finishApplicationDelete() error = %v", err)
	}

	var narrowRule model.NotificationRule
	if err := db.First(&narrowRule, "id = ?", "nrl_app_narrow").Error; err != nil {
		t.Fatal(err)
	}
	if narrowRule.Enabled || strings.Contains(narrowRule.FilterJSON, "app_only") {
		t.Fatalf("narrow rule after sole app delete = %#v, want disabled without app_only", narrowRule)
	}

	var multiRule model.NotificationRule
	if err := db.First(&multiRule, "id = ?", "nrl_multi_target").Error; err != nil {
		t.Fatal(err)
	}
	if !multiRule.Enabled || strings.Contains(multiRule.FilterJSON, "app_only") || strings.Contains(multiRule.FilterJSON, "dplt_only") || !strings.Contains(multiRule.FilterJSON, "app_keep") || !strings.Contains(multiRule.FilterJSON, "dplt_keep") {
		t.Fatalf("multi rule after app delete = %#v", multiRule)
	}

	var narrowGrantCount int64
	db.Model(&model.NetworkEgressExceptionGrant{}).Where("id = ?", "nexg_narrow").Count(&narrowGrantCount)
	if narrowGrantCount != 0 {
		t.Fatalf("narrow egress grant should be deleted when last target is deleted, count = %d", narrowGrantCount)
	}
	var multiGrant model.NetworkEgressExceptionGrant
	if err := db.First(&multiGrant, "id = ?", "nexg_multi").Error; err != nil {
		t.Fatal(err)
	}
	if multiGrant.DeploymentTargetIDs != targetKeep.ID {
		t.Fatalf("multi egress grant targets = %q, want %q", multiGrant.DeploymentTargetIDs, targetKeep.ID)
	}
	if len(enqueuer.projects) == 0 || enqueuer.projects[0] != surviving.ID {
		t.Fatalf("expected network policy reconcile for %s, got %v", surviving.ID, enqueuer.projects)
	}

	if err := runner.finishProjectDelete(t.Context(), doomed); err != nil {
		t.Fatalf("finishProjectDelete() error = %v", err)
	}
	var doomedBindings int64
	db.Model(&model.ScopedResourceProjectBinding{}).Where("project_id = ?", doomed.ID).Count(&doomedBindings)
	if doomedBindings != 0 {
		t.Fatalf("doomed project bindings remaining = %d, want 0", doomedBindings)
	}
	var activeChannels []string
	db.Model(&model.NotificationChannel{}).Order("id").Pluck("id", &activeChannels)
	if len(activeChannels) != 1 || activeChannels[0] != sharedCh.ID {
		t.Fatalf("active channels after doomed project delete = %v, want [%s]", activeChannels, sharedCh.ID)
	}
	var globalMulti model.NotificationRule
	if err := db.First(&globalMulti, "id = ?", "nrl_global_multi").Error; err != nil {
		t.Fatal(err)
	}
	if globalMulti.Enabled || strings.Contains(globalMulti.FilterJSON, doomed.ID) || strings.Contains(globalMulti.FilterJSON, "app_doomed") || strings.Contains(globalMulti.FilterJSON, "dplt_doomed") || !strings.Contains(globalMulti.FilterJSON, surviving.ID) || !strings.Contains(globalMulti.FilterJSON, "dplt_keep") {
		t.Fatalf("global multi rule after doomed project delete = %#v", globalMulti)
	}
	var orphanBvsCount, orphanRegCount, orphanCredCount, remainingSecrets int64
	db.Model(&model.BuildVariableSet{}).Where("id = ?", orphanBvs.ID).Count(&orphanBvsCount)
	db.Model(&model.ArtifactRegistry{}).Where("id = ?", orphanReg.ID).Count(&orphanRegCount)
	db.Model(&model.RegistryCredential{}).Where("id = ?", orphanCred.ID).Count(&orphanCredCount)
	db.Model(&model.SecretValue{}).Count(&remainingSecrets)
	if orphanBvsCount != 0 || orphanRegCount != 0 || orphanCredCount != 0 || remainingSecrets != 0 {
		t.Fatalf("expected sole-project scoped resources and secrets to be cleaned up, got bvs=%d reg=%d cred=%d secrets=%d", orphanBvsCount, orphanRegCount, orphanCredCount, remainingSecrets)
	}
	var doomedGlobalCount int64
	db.Model(&model.NotificationRule{}).Where("id = ?", "nrl_global_doomed_only").Count(&doomedGlobalCount)
	if doomedGlobalCount != 0 {
		t.Fatalf("global rule targeting only deleted project must be deleted, count = %d", doomedGlobalCount)
	}
}
