package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type snapshotNamespaceManager struct {
	fakeNamespaceManager
	snapshot kubeprovider.DeploymentSnapshot
}

func (m snapshotNamespaceManager) GetDeploymentSnapshot(context.Context, string, string) (kubeprovider.DeploymentSnapshot, error) {
	return m.snapshot, nil
}

func openDeliveryStateDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.Open(t, testdb.Options{
		SchemaPrefix: "delivery_state",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{}, &model.Application{}, &model.DeploymentTarget{},
				&model.Release{}, &model.ReleaseLog{}, &model.BuildRun{}, &model.BuildJob{},
			)
		},
	})
}

func seedDeliveryTarget(t *testing.T, db *gorm.DB, projectDeleteStatus, targetDeleteStatus string) (model.Project, model.Application, model.DeploymentTarget) {
	t.Helper()
	project := model.Project{ID: "prj_state", Name: "State", Identifier: "state", DeleteStatus: projectDeleteStatus}
	application := model.Application{ID: "app_state", ProjectID: project.ID, Name: "State", Identifier: "state", DeleteStatus: "active"}
	target := model.DeploymentTarget{
		ID: "dplt_state", ProjectID: project.ID, ApplicationID: application.ID, Name: "Dev", Stage: "dev",
		KubernetesName: "state-dev", Replicas: 1, DeleteStatus: targetDeleteStatus,
	}
	for _, value := range []any{&project, &application, &target} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	return project, application, target
}

func createRelease(t *testing.T, db *gorm.DB, id, status string, createdAt time.Time, startedAt *time.Time) model.Release {
	t.Helper()
	release := model.Release{
		ID: id, ProjectID: "prj_state", ApplicationID: "app_state", DeploymentTargetID: "dplt_state",
		ImageRef: "registry.example.test/state:" + id, Status: status, Revision: 1,
		StartedAt: startedAt, CreatedAt: createdAt,
	}
	if err := db.Create(&release).Error; err != nil {
		t.Fatalf("create release %s: %v", id, err)
	}
	return release
}

func readRelease(t *testing.T, db *gorm.DB, id string) model.Release {
	t.Helper()
	var release model.Release
	if err := db.First(&release, "id = ?", id).Error; err != nil {
		t.Fatalf("read release %s: %v", id, err)
	}
	return release
}

func TestFinishDeployReleaseDoesNotOverwriteTerminalState(t *testing.T) {
	db := openDeliveryStateDB(t)
	runner := &Runner{db: db}
	canceled := createRelease(t, db, "rel_canceled", "canceled", time.Now(), nil)
	transitioned, err := runner.finishDeployRelease(t.Context(), canceled, "succeeded", "", "rollout completed")
	if err != nil || transitioned {
		t.Fatalf("finish canceled release = %v, %v; want no transition", transitioned, err)
	}
	if got := readRelease(t, db, canceled.ID); got.Status != "canceled" {
		t.Fatalf("canceled release status = %q", got.Status)
	}

	running := createRelease(t, db, "rel_running", "running", time.Now(), nil)
	transitioned, err = runner.finishDeployRelease(t.Context(), running, "failed", model.ReleaseFailureRolloutFailed, "probe failed")
	if err != nil || !transitioned {
		t.Fatalf("finish running release = %v, %v; want transition", transitioned, err)
	}
	transitioned, err = runner.finishDeployRelease(t.Context(), running, "succeeded", "", "late success")
	if err != nil || transitioned {
		t.Fatalf("late success = %v, %v; want ignored", transitioned, err)
	}
	if got := readRelease(t, db, running.ID); got.Status != "failed" || got.FailureCode != model.ReleaseFailureRolloutFailed {
		t.Fatalf("running release = %s/%s, want failed/%s", got.Status, got.FailureCode, model.ReleaseFailureRolloutFailed)
	}
}

func TestSyncReleaseRuntimeStatusOnlyJudgesAppliedReleases(t *testing.T) {
	db := openDeliveryStateDB(t)
	seedDeliveryTarget(t, db, "active", "active")
	now := time.Now()
	old := now.Add(-40 * time.Minute)
	recent := now.Add(-2 * time.Minute)
	pending := createRelease(t, db, "rel_pending", "pending", old, nil)
	unapplied := createRelease(t, db, "rel_unapplied", "running", old, &old)
	inWorkerWindow := createRelease(t, db, "rel_recent", "running", recent, &recent)

	// A healthy workload that still carries a previous release must not be
	// read as success for releases the Worker has not applied yet.
	runner := &Runner{db: db, kubernetesManagerFactory: func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		return snapshotNamespaceManager{snapshot: kubeprovider.DeploymentSnapshot{Phase: kubeprovider.DeploymentSucceeded, ReleaseID: "rel_previous"}}, nil
	}}
	if err := runner.syncReleaseRuntimeStatus(t.Context()); err != nil {
		t.Fatalf("sync release status: %v", err)
	}
	for _, release := range []model.Release{pending, unapplied, inWorkerWindow} {
		if got := readRelease(t, db, release.ID); got.Status != release.Status {
			t.Fatalf("release %s status = %q, want %q", release.ID, got.Status, release.Status)
		}
	}

	runner.kubernetesManagerFactory = func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		return snapshotNamespaceManager{snapshot: kubeprovider.DeploymentSnapshot{Phase: kubeprovider.DeploymentSucceeded, ReleaseID: unapplied.ID}}, nil
	}
	if err := runner.syncReleaseRuntimeStatus(t.Context()); err != nil {
		t.Fatalf("sync applied release status: %v", err)
	}
	if got := readRelease(t, db, unapplied.ID); got.Status != "succeeded" {
		t.Fatalf("applied release status = %q, want succeeded", got.Status)
	}
	if got := readRelease(t, db, inWorkerWindow.ID); got.Status != "running" {
		t.Fatalf("release inside the worker window = %q, want running", got.Status)
	}
	if got := readRelease(t, db, pending.ID); got.Status != "pending" {
		t.Fatalf("pending release = %q, want pending", got.Status)
	}
}

func TestSyncReleaseRuntimeStatusMarksSupersededRelease(t *testing.T) {
	db := openDeliveryStateDB(t)
	seedDeliveryTarget(t, db, "active", "active")
	old := time.Now().Add(-45 * time.Minute)
	newer := old.Add(5 * time.Minute)
	stale := createRelease(t, db, "rel_stale", "running", old, &old)
	createRelease(t, db, "rel_newer", "succeeded", newer, &newer)
	runner := &Runner{db: db, kubernetesManagerFactory: func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		return snapshotNamespaceManager{snapshot: kubeprovider.DeploymentSnapshot{Phase: kubeprovider.DeploymentSucceeded, ReleaseID: "rel_newer"}}, nil
	}}
	if err := runner.syncReleaseRuntimeStatus(t.Context()); err != nil {
		t.Fatalf("sync release status: %v", err)
	}
	if got := readRelease(t, db, stale.ID); got.Status != "failed" || got.FailureCode != model.ReleaseFailureSuperseded {
		t.Fatalf("superseded release = %s/%s", got.Status, got.FailureCode)
	}
}

func TestExpireStaleReleasesUsesTimeoutCode(t *testing.T) {
	db := openDeliveryStateDB(t)
	now := time.Now()
	expired := now.Add(-3 * time.Hour)
	running := createRelease(t, db, "rel_expired_running", "running", expired, &expired)
	pending := createRelease(t, db, "rel_expired_pending", "pending", expired, nil)
	canceled := createRelease(t, db, "rel_expired_canceled", "canceled", expired, &expired)
	fresh := createRelease(t, db, "rel_fresh_pending", "pending", now, nil)
	if err := (&Runner{db: db}).expireStaleReleases(t.Context(), now); err != nil {
		t.Fatalf("expire stale releases: %v", err)
	}
	for _, release := range []model.Release{running, pending} {
		if got := readRelease(t, db, release.ID); got.Status != "failed" || got.FailureCode != model.ReleaseFailureTimeout || got.FinishedAt == nil {
			t.Fatalf("expired release %s = %s/%s finished=%v", release.ID, got.Status, got.FailureCode, got.FinishedAt)
		}
	}
	if got := readRelease(t, db, canceled.ID); got.Status != "canceled" {
		t.Fatalf("canceled release = %q", got.Status)
	}
	if got := readRelease(t, db, fresh.ID); got.Status != "pending" {
		t.Fatalf("fresh release = %q", got.Status)
	}
}

func deployTask(t *testing.T, releaseID string) *asynq.Task {
	t.Helper()
	payload, err := json.Marshal(tasks.DeployRunPayload{ReleaseID: releaseID, ProjectID: "prj_state"})
	if err != nil {
		t.Fatalf("encode deploy payload: %v", err)
	}
	return asynq.NewTask(tasks.TypeDeployRun, payload)
}

func TestHandleDeployRunSkipsCanceledRelease(t *testing.T) {
	db := openDeliveryStateDB(t)
	seedDeliveryTarget(t, db, "active", "active")
	release := createRelease(t, db, "rel_skip", "canceled", time.Now(), nil)
	runner := &Runner{db: db, kubernetesManagerFactory: func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
		t.Fatal("canceled release reached the cluster")
		return nil, nil
	}}
	if err := runner.handleDeployRun(t.Context(), deployTask(t, release.ID)); err != nil {
		t.Fatalf("handle canceled release: %v", err)
	}
	if got := readRelease(t, db, release.ID); got.Status != "canceled" || got.StartedAt != nil {
		t.Fatalf("canceled release = %s started=%v", got.Status, got.StartedAt)
	}
}

func TestHandleDeployRunRejectsDeletingProjectAndTarget(t *testing.T) {
	for _, tc := range []struct {
		name          string
		projectStatus string
		targetStatus  string
		wantCode      string
	}{
		{name: "project", projectStatus: "deleting", targetStatus: "active", wantCode: model.ReleaseFailureProjectUnavailable},
		{name: "target", projectStatus: "active", targetStatus: "deleting", wantCode: model.ReleaseFailureTargetUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openDeliveryStateDB(t)
			seedDeliveryTarget(t, db, tc.projectStatus, tc.targetStatus)
			release := createRelease(t, db, "rel_deleting", "pending", time.Now(), nil)
			if err := (&Runner{db: db}).handleDeployRun(t.Context(), deployTask(t, release.ID)); err != nil {
				t.Fatalf("handle deploy run: %v", err)
			}
			if got := readRelease(t, db, release.ID); got.Status != "failed" || got.FailureCode != tc.wantCode {
				t.Fatalf("release = %s/%s, want failed/%s", got.Status, got.FailureCode, tc.wantCode)
			}
		})
	}
}

func createBuild(t *testing.T, db *gorm.DB, id, status string, createdAt time.Time, startedAt *time.Time) (model.BuildRun, model.BuildJob) {
	t.Helper()
	run := model.BuildRun{
		ID: id, ProjectID: "prj_state", ApplicationID: "app_state", DeploymentTargetID: "dplt_state",
		Status: status, BuildTimeoutSeconds: 600, StartedAt: startedAt, CreatedAt: createdAt,
	}
	job := model.BuildJob{ID: id + "_job", BuildRunID: id, ProjectID: "prj_state", Status: status, StartedAt: startedAt, CreatedAt: createdAt}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create build run: %v", err)
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create build job: %v", err)
	}
	return run, job
}

func readBuild(t *testing.T, db *gorm.DB, id string) (model.BuildRun, model.BuildJob) {
	t.Helper()
	var run model.BuildRun
	var job model.BuildJob
	if err := db.First(&run, "id = ?", id).Error; err != nil {
		t.Fatalf("read build run: %v", err)
	}
	if err := db.First(&job, "build_run_id = ?", id).Error; err != nil {
		t.Fatalf("read build job: %v", err)
	}
	return run, job
}

func TestReclaimExpiredBuildJobsTimesOutQueuedAndRunningBuilds(t *testing.T) {
	db := openDeliveryStateDB(t)
	now := time.Now()
	queuedLongAgo := now.Add(-30 * time.Minute) // timeout 600s, queue limit 1200s
	createBuild(t, db, "bldr_queued_expired", "queued", queuedLongAgo, nil)
	createBuild(t, db, "bldr_queued_fresh", "queued", now.Add(-5*time.Minute), nil)
	startedLongAgo := now.Add(-15 * time.Minute)
	createBuild(t, db, "bldr_running_expired", "running", startedLongAgo, &startedLongAgo)
	createBuild(t, db, "bldr_canceled", "canceled", queuedLongAgo, nil)

	if err := (&Runner{db: db}).reclaimExpiredBuildJobs(t.Context()); err != nil {
		t.Fatalf("reclaim expired builds: %v", err)
	}
	for id, want := range map[string]string{
		"bldr_queued_expired":  model.BuildFailureQueueTimeout,
		"bldr_running_expired": model.BuildFailureTimeout,
	} {
		run, job := readBuild(t, db, id)
		if run.Status != "timeout" || run.FailureCode != want || job.Status != "timeout" || job.FailureCode != want || run.FinishedAt == nil {
			t.Fatalf("%s = run %s/%s job %s/%s, want timeout/%s", id, run.Status, run.FailureCode, job.Status, job.FailureCode, want)
		}
	}
	if run, _ := readBuild(t, db, "bldr_queued_fresh"); run.Status != "queued" {
		t.Fatalf("fresh queued build = %q", run.Status)
	}
	if run, _ := readBuild(t, db, "bldr_canceled"); run.Status != "canceled" {
		t.Fatalf("canceled build = %q", run.Status)
	}
}

func TestHandleBuildRunFailsDeletingTargetWithStableCode(t *testing.T) {
	db := openDeliveryStateDB(t)
	seedDeliveryTarget(t, db, "active", "deleting")
	run, job := createBuild(t, db, "bldr_deleting_target", "queued", time.Now(), nil)
	payload, err := json.Marshal(tasks.BuildRunPayload{BuildRunID: run.ID, BuildJobID: job.ID, ProjectID: run.ProjectID})
	if err != nil {
		t.Fatalf("encode build payload: %v", err)
	}
	if err := (&Runner{db: db}).handleBuildRun(t.Context(), asynq.NewTask(tasks.TypeBuildRun, payload)); err != nil {
		t.Fatalf("handle build run: %v", err)
	}
	gotRun, gotJob := readBuild(t, db, run.ID)
	if gotRun.Status != "failed" || gotRun.FailureCode != model.BuildFailureTargetUnavailable || gotJob.FailureCode != model.BuildFailureTargetUnavailable {
		t.Fatalf("build = run %s/%s job %s/%s", gotRun.Status, gotRun.FailureCode, gotJob.Status, gotJob.FailureCode)
	}
}
