package volumemigration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"gorm.io/gorm"
)

func TestStableBackfillIDsAreDeterministic(t *testing.T) {
	first := stableProjectVolumeID(model.ProjectVolumeSourceRetained, "rvol_demo")
	second := stableProjectVolumeID(model.ProjectVolumeSourceRetained, "rvol_demo")
	changed := stableProjectVolumeID(model.ProjectVolumeSourceRetained, "rvol_other")
	if first != second || first == changed || !strings.HasPrefix(first, "pvol_") {
		t.Fatalf("stable project volume ids = %q, %q, %q", first, second, changed)
	}
	if stableMountID("dplt_demo", "data", 0) != stableMountID("dplt_demo", "data", 9) {
		t.Fatal("mount id changed when only legacy array order changed")
	}
	repair := stableRepairID("deployment_target", "dplt_demo", RepairClaimNotFound)
	if repair != stableRepairID("deployment_target", "dplt_demo", RepairClaimNotFound) || !strings.HasPrefix(repair, "vrpi_") {
		t.Fatalf("stable repair id = %q", repair)
	}
}

func TestServiceBackfillsRetainedManagedReferencedAndEmptyDirVolumesIdempotently(t *testing.T) {
	repository := newMemoryRepository()
	repository.projects = []model.Project{{ID: "prj_demo", KubernetesNamespace: "luna-demo"}}
	repository.applications["app_demo"] = model.Application{ID: "app_demo", ProjectID: "prj_demo", Name: "Demo"}
	repository.retained = []model.RetainedVolume{{
		ID: "rvol_demo", ProjectID: "prj_demo", SourceApplicationID: "app_demo", SourceApplicationName: "Demo",
		SourceDeploymentTargetID: "dplt_old", ClusterID: "rcl_demo", Namespace: "luna-demo",
		ClaimName: "retained-data", Status: model.RetainedVolumeStatusRetained, RetainedAt: time.Now().Add(-time.Hour),
	}}
	repository.targets = []model.DeploymentTarget{{
		ID: "dplt_demo", ProjectID: "prj_demo", ApplicationID: "app_demo", ClusterID: "rcl_demo",
		KubernetesName: "demo-prod", WorkloadType: "Deployment", DataRetentionEnabled: true,
		DataVolumes: `[
          {"name":"data","mountPath":"/data","capacity":"10Gi","sourceType":"managed"},
          {"name":"shared","mountPath":"/shared","sourceType":"existingClaim","existingClaimName":"shared-pvc","readOnly":true},
          {"name":"archive","mountPath":"/archive","sourceType":"retainedClaim","existingClaimName":"retained-data","retainedVolumeId":"rvol_demo"},
          {"name":"scratch","mountPath":"/tmp/work","sourceType":"emptyDir","emptyDirSizeLimit":"1Gi"}
        ]`,
	}}
	inspector := &inspectorStub{
		claims: map[string]ClaimObservation{
			"retained-data":  managedClaim("prj_demo", "10Gi"),
			"demo-prod-data": managedClaim("prj_demo", "10Gi"),
			"shared-pvc":     referencedClaim("5Gi"),
		},
		workloads: map[string]WorkloadAttachment{
			"data":    {ClaimName: "demo-prod-data", MountPath: "/data"},
			"shared":  {ClaimName: "shared-pvc", MountPath: "/shared", ReadOnly: true},
			"archive": {ClaimName: "retained-data", MountPath: "/archive"},
			"scratch": {MountPath: "/tmp/work", EmptyDir: true},
		},
	}
	service := NewService(repository, inspector)

	dryRun, err := service.Run(context.Background(), Options{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	assertBalancedPlan(t, dryRun, false, OutcomePlanned)
	if len(repository.volumes) != 0 || len(repository.mounts) != 0 {
		t.Fatal("dry-run persisted backfill records")
	}

	firstApply, err := service.Run(context.Background(), Options{Apply: true, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	assertBalancedPlan(t, firstApply, true, OutcomeApplied)
	if len(repository.volumes) != 3 || len(repository.mounts) != 4 {
		t.Fatalf("persisted volumes/mounts = %d/%d", len(repository.volumes), len(repository.mounts))
	}
	for _, mount := range repository.mounts {
		if mount.ActivationState != model.DeploymentVolumeActivationActive {
			t.Fatalf("mount %s activation = %q", mount.ID, mount.ActivationState)
		}
	}

	secondApply, err := service.Run(context.Background(), Options{Apply: true, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	assertBalancedPlan(t, secondApply, true, OutcomeUnchanged)
	if len(repository.volumes) != 3 || len(repository.mounts) != 4 {
		t.Fatal("idempotent rerun created duplicate records")
	}
}

func TestServicePaginatesProjectsAndDeploymentTargetsAtConfiguredLimit(t *testing.T) {
	repository := newMemoryRepository()
	for index := 0; index < 101; index++ {
		projectID := "prj_" + integerString(index)
		repository.projects = append(repository.projects, model.Project{ID: projectID, KubernetesNamespace: "ns-" + integerString(index)})
	}
	report, err := NewService(repository, &inspectorStub{}).Run(context.Background(), Options{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if report.Reconciliation.Projects != 101 {
		t.Fatalf("project count = %d", report.Reconciliation.Projects)
	}
	if got := repository.projectPages; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("project pages = %v", got)
	}
	for _, size := range repository.observedPageSizes {
		if size > MaxPageSize {
			t.Fatalf("repository received page size %d", size)
		}
	}
	if _, err := NewService(repository, &inspectorStub{}).Run(context.Background(), Options{PageSize: 101}); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("oversized page error = %v", err)
	}
}

func TestServiceRecordsStableRepairWithoutGuessingClaimSuccess(t *testing.T) {
	repository := repositoryWithManagedTarget()
	inspector := &inspectorStub{claimErr: ErrClaimObservation, workloadErr: ErrWorkloadNotFound}
	service := NewService(repository, inspector)
	first, err := service.Run(context.Background(), Options{Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Run(context.Background(), Options{Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Repairs) != 1 || first.Repairs[0].Code != RepairClaimObservationUnavailable || first.Repairs[0].ID != second.Repairs[0].ID {
		t.Fatalf("repairs = %+v / %+v", first.Repairs, second.Repairs)
	}
	if first.Reconciliation.ReadyForSwitch || len(repository.volumes) != 0 || len(repository.mounts) != 0 {
		t.Fatal("unobserved claim was treated as a successful backfill")
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "kubeconfig") || strings.Contains(string(encoded), "/data") || strings.Contains(string(encoded), "secret-value") {
		t.Fatalf("report leaked configuration or a filesystem path: %s", encoded)
	}
}

func TestServicePropagatesCancellationToInspector(t *testing.T) {
	repository := repositoryWithManagedTarget()
	inspector := &cancellingInspector{contextKey: testContextKey("trace"), entered: make(chan struct{})}
	service := NewService(repository, inspector)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), inspector.contextKey, "trace-parent"))
	done := make(chan error, 1)
	go func() {
		_, err := service.Run(ctx, Options{})
		done <- err
	}()
	select {
	case <-inspector.entered:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("claim inspector was not called")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled run did not stop")
	}
	if inspector.contextValue != "trace-parent" {
		t.Fatalf("inspector context value = %v", inspector.contextValue)
	}
}

func TestServiceKeepsMountReservedWhenWorkloadObservationFails(t *testing.T) {
	repository := repositoryWithManagedTarget()
	inspector := &inspectorStub{claims: map[string]ClaimObservation{"demo-data": managedClaim("prj_demo", "1Gi")}, workloadErr: ErrWorkloadObservation}
	report, err := NewService(repository, inspector).Run(context.Background(), Options{Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Repairs) != 1 || report.Repairs[0].Code != RepairWorkloadObservationUnavailable || report.Reconciliation.ReadyForSwitch {
		t.Fatalf("report = %+v", report)
	}
	for _, mount := range repository.mounts {
		if mount.ActivationState != model.DeploymentVolumeActivationReserved {
			t.Fatalf("unobserved workload mount activation = %q", mount.ActivationState)
		}
	}
}

func assertBalancedPlan(t *testing.T, report Report, apply bool, outcome string) {
	t.Helper()
	got := report.Reconciliation
	if got.SourceRetainedVolumes != 1 || got.SourcePersistentMounts != 3 || got.SourceEmptyDirMounts != 1 || got.ExpectedProjectVolumes != 3 || got.ExpectedDeploymentMounts != 4 || got.RepairItems != 0 || !got.PlanBalanced {
		t.Fatalf("reconciliation = %+v", got)
	}
	switch outcome {
	case OutcomePlanned:
		if got.PlannedProjectVolumes != 3 || got.PlannedDeploymentMounts != 4 || got.DatabaseBalanced {
			t.Fatalf("dry-run reconciliation = %+v", got)
		}
	case OutcomeApplied:
		if got.AppliedProjectVolumes != 3 || got.AppliedDeploymentMounts != 4 || !got.DatabaseBalanced {
			t.Fatalf("apply reconciliation = %+v", got)
		}
	case OutcomeUnchanged:
		if got.UnchangedProjectVolumes != 3 || got.UnchangedDeploymentMounts != 4 || !got.DatabaseBalanced {
			t.Fatalf("rerun reconciliation = %+v", got)
		}
	}
	if got.ReadyForSwitch != apply {
		t.Fatalf("readyForSwitch = %v, apply = %v", got.ReadyForSwitch, apply)
	}
}

func managedClaim(projectID, capacity string) ClaimObservation {
	bytes, _ := capacityInBytes(capacity)
	return ClaimObservation{
		Exists: true, CapacityRequest: capacity, CapacityBytes: bytes, AccessModes: []string{model.ProjectVolumeAccessReadWriteOnce},
		VolumeMode: model.ProjectVolumeModeFilesystem, ManagedBy: kubernetes.ManagedByValue, OwnerProjectID: projectID,
	}
}

func referencedClaim(capacity string) ClaimObservation {
	bytes, _ := capacityInBytes(capacity)
	return ClaimObservation{Exists: true, CapacityRequest: capacity, CapacityBytes: bytes, AccessModes: []string{model.ProjectVolumeAccessReadWriteMany}, VolumeMode: model.ProjectVolumeModeFilesystem}
}

func repositoryWithManagedTarget() *memoryRepository {
	repository := newMemoryRepository()
	repository.projects = []model.Project{{ID: "prj_demo", KubernetesNamespace: "luna-demo"}}
	repository.applications["app_demo"] = model.Application{ID: "app_demo", ProjectID: "prj_demo", Name: "Demo"}
	repository.targets = []model.DeploymentTarget{{
		ID: "dplt_demo", ProjectID: "prj_demo", ApplicationID: "app_demo", ClusterID: "rcl_demo", KubernetesName: "demo",
		DataRetentionEnabled: true, DataVolumes: `[{"name":"data","mountPath":"/data","capacity":"1Gi","sourceType":"managed"}]`,
	}}
	return repository
}

type inspectorStub struct {
	claims      map[string]ClaimObservation
	claimErr    error
	workloads   map[string]WorkloadAttachment
	workloadErr error
}

func (inspector *inspectorStub) InspectClaim(_ context.Context, input ClaimInspectionInput) (ClaimObservation, error) {
	if inspector.claimErr != nil {
		return ClaimObservation{}, inspector.claimErr
	}
	if observation, ok := inspector.claims[input.ClaimName]; ok {
		return observation, nil
	}
	return ClaimObservation{}, ErrClaimNotFound
}

func (inspector *inspectorStub) InspectWorkload(context.Context, WorkloadInspectionInput) (map[string]WorkloadAttachment, error) {
	return inspector.workloads, inspector.workloadErr
}

type testContextKey string

type cancellingInspector struct {
	contextKey   testContextKey
	contextValue any
	entered      chan struct{}
	once         sync.Once
}

func (inspector *cancellingInspector) InspectClaim(ctx context.Context, _ ClaimInspectionInput) (ClaimObservation, error) {
	inspector.contextValue = ctx.Value(inspector.contextKey)
	inspector.once.Do(func() { close(inspector.entered) })
	<-ctx.Done()
	return ClaimObservation{}, ctx.Err()
}

func (inspector *cancellingInspector) InspectWorkload(context.Context, WorkloadInspectionInput) (map[string]WorkloadAttachment, error) {
	return nil, ErrWorkloadNotFound
}

type memoryRepository struct {
	projects          []model.Project
	retained          []model.RetainedVolume
	targets           []model.DeploymentTarget
	applications      map[string]model.Application
	volumes           map[string]model.ProjectVolume
	mounts            map[string]model.DeploymentVolumeMount
	projectPages      []int
	observedPageSizes []int
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		applications: make(map[string]model.Application), volumes: make(map[string]model.ProjectVolume), mounts: make(map[string]model.DeploymentVolumeMount),
	}
}

func (repository *memoryRepository) ListProjects(_ context.Context, page, pageSize int, projectID string) ([]model.Project, error) {
	repository.projectPages = append(repository.projectPages, page)
	repository.observedPageSizes = append(repository.observedPageSizes, pageSize)
	items := repository.projects
	if projectID != "" {
		items = nil
		for _, project := range repository.projects {
			if project.ID == projectID {
				items = append(items, project)
			}
		}
	}
	return pageItems(items, page, pageSize), nil
}

func (repository *memoryRepository) ListRetainedVolumes(_ context.Context, projectID string, page, pageSize int) ([]model.RetainedVolume, error) {
	repository.observedPageSizes = append(repository.observedPageSizes, pageSize)
	items := make([]model.RetainedVolume, 0)
	for _, item := range repository.retained {
		if item.ProjectID == projectID {
			items = append(items, item)
		}
	}
	return pageItems(items, page, pageSize), nil
}

func (repository *memoryRepository) ListDeploymentTargets(_ context.Context, projectID string, page, pageSize int) ([]model.DeploymentTarget, error) {
	repository.observedPageSizes = append(repository.observedPageSizes, pageSize)
	items := make([]model.DeploymentTarget, 0)
	for _, item := range repository.targets {
		if item.ProjectID == projectID {
			items = append(items, item)
		}
	}
	return pageItems(items, page, pageSize), nil
}

func (repository *memoryRepository) ResolveRuntimeClusterID(_ context.Context, clusterID string) (string, error) {
	if clusterID == "" {
		return "rcl_default", nil
	}
	return clusterID, nil
}

func (repository *memoryRepository) GetApplication(_ context.Context, projectID, applicationID string) (model.Application, error) {
	item, ok := repository.applications[applicationID]
	if !ok || item.ProjectID != projectID {
		return model.Application{}, gorm.ErrRecordNotFound
	}
	return item, nil
}

func (repository *memoryRepository) GetProjectVolume(_ context.Context, projectID, volumeID string) (model.ProjectVolume, error) {
	item, ok := repository.volumes[volumeID]
	if !ok || item.ProjectID != projectID {
		return model.ProjectVolume{}, gorm.ErrRecordNotFound
	}
	return item, nil
}

func (repository *memoryRepository) SyncProjectVolume(_ context.Context, desired model.ProjectVolume, apply bool) (SyncResult, error) {
	if existing, ok := repository.volumes[desired.ID]; ok {
		if !projectVolumesMatch(existing, desired) {
			return SyncResult{}, ErrProjectVolumeConflict
		}
		return SyncResult{Outcome: OutcomeUnchanged, CapacityBytes: existing.CapacityBytes}, nil
	}
	for _, existing := range repository.volumes {
		if existing.ClusterID == desired.ClusterID && existing.Namespace == desired.Namespace && existing.ClaimName == desired.ClaimName {
			return SyncResult{}, ErrProjectVolumeConflict
		}
	}
	if !apply {
		return SyncResult{Outcome: OutcomePlanned}, nil
	}
	repository.volumes[desired.ID] = desired
	return SyncResult{Outcome: OutcomeApplied, CapacityBytes: desired.CapacityBytes}, nil
}

func (repository *memoryRepository) SyncDeploymentVolumeMount(_ context.Context, desired model.DeploymentVolumeMount, apply bool) (SyncResult, error) {
	if existing, ok := repository.mounts[desired.ID]; ok {
		if !deploymentMountsMatch(existing, desired) {
			return SyncResult{}, ErrDeploymentMountConflict
		}
		return SyncResult{Outcome: OutcomeUnchanged}, nil
	}
	if !apply {
		return SyncResult{Outcome: OutcomePlanned}, nil
	}
	repository.mounts[desired.ID] = desired
	return SyncResult{Outcome: OutcomeApplied}, nil
}

func pageItems[T any](items []T, page, pageSize int) []T {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	result := append([]T(nil), items[start:end]...)
	return result
}
