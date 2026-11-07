package worker

import (
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func openBuildConcurrencyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.Open(t, testdb.Options{
		SchemaPrefix: "build_concurrency_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.BuildRun{}, &model.BuildJob{}, &model.Release{}, &model.AuditLog{})
		},
	})
}

func seedBuildRun(t *testing.T, db *gorm.DB, id string, status string, createdAt time.Time) model.BuildRun {
	t.Helper()
	run := model.BuildRun{ID: id, ProjectID: "prj_cc", ApplicationID: "app_cc", DeploymentTargetID: "dplt_cc", Status: status, CreatedBy: "usr_cc", CreatedAt: createdAt}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create build run %s: %v", id, err)
	}
	job := model.BuildJob{ID: "job_" + id, BuildRunID: id, ProjectID: run.ProjectID, Status: status}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create build job %s: %v", id, err)
	}
	return run
}

func TestQueueBuildConcurrencyWaitsForEarlierOrRunningBuilds(t *testing.T) {
	db := openBuildConcurrencyTestDB(t)
	runner := &Runner{db: db}
	target := model.DeploymentTarget{ID: "dplt_cc", BuildConcurrencyPolicy: model.BuildConcurrencyQueue}
	base := time.Now().Add(-time.Hour).UTC()
	older := seedBuildRun(t, db, "bld_older", "queued", base)
	newer := seedBuildRun(t, db, "bld_newer", "queued", base.Add(time.Minute))

	if err := runner.applyBuildConcurrencyPolicy(t.Context(), older, target); err != nil {
		t.Fatalf("oldest queued build should start: %v", err)
	}
	if err := runner.applyBuildConcurrencyPolicy(t.Context(), newer, target); !errors.Is(err, errBuildCapacityUnavailable) {
		t.Fatalf("newer build error = %v, want capacity wait", err)
	}

	// 已运行的更新构建同样会阻塞排队中的旧构建，保证同一部署配置同时只有一个构建。
	if err := db.Model(&model.BuildRun{}).Where("id = ?", newer.ID).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	if err := runner.applyBuildConcurrencyPolicy(t.Context(), older, target); !errors.Is(err, errBuildCapacityUnavailable) {
		t.Fatalf("older build error = %v, want wait for running build", err)
	}

	if err := db.Model(&model.BuildRun{}).Where("id = ?", newer.ID).Update("status", "succeeded").Error; err != nil {
		t.Fatal(err)
	}
	if err := runner.applyBuildConcurrencyPolicy(t.Context(), older, target); err != nil {
		t.Fatalf("build should start once others finish: %v", err)
	}
}

func TestCancelInProgressBuildConcurrencySupersedesOnlyEarlierBuilds(t *testing.T) {
	db := openBuildConcurrencyTestDB(t)
	runner := &Runner{db: db}
	target := model.DeploymentTarget{ID: "dplt_cc", BuildConcurrencyPolicy: model.BuildConcurrencyCancelInProgress}
	base := time.Now().Add(-time.Hour).UTC()
	seedBuildRun(t, db, "bld_running", "running", base)
	seedBuildRun(t, db, "bld_queued", "queued", base.Add(time.Minute))
	seedBuildRun(t, db, "bld_done", "succeeded", base.Add(2*time.Minute))
	current := seedBuildRun(t, db, "bld_current", "queued", base.Add(3*time.Minute))
	seedBuildRun(t, db, "bld_later", "queued", base.Add(4*time.Minute))

	if err := runner.applyBuildConcurrencyPolicy(t.Context(), current, target); err != nil {
		t.Fatalf("cancel in progress: %v", err)
	}
	want := map[string]string{"bld_running": "canceled", "bld_queued": "canceled", "bld_done": "succeeded", "bld_current": "queued", "bld_later": "queued"}
	for runID, status := range want {
		var run model.BuildRun
		if err := db.First(&run, "id = ?", runID).Error; err != nil {
			t.Fatal(err)
		}
		var job model.BuildJob
		if err := db.First(&job, "build_run_id = ?", runID).Error; err != nil {
			t.Fatal(err)
		}
		if run.Status != status || job.Status != status {
			t.Fatalf("%s status run=%s job=%s, want %s", runID, run.Status, job.Status, status)
		}
		if status == "canceled" && (run.FailureCode != model.BuildFailureSuperseded || job.FailureCode != model.BuildFailureSuperseded) {
			t.Fatalf("%s failure code run=%s job=%s, want superseded", runID, run.FailureCode, job.FailureCode)
		}
	}
	var audits int64
	if err := db.Model(&model.AuditLog{}).Where("action = ? and user_id = ?", "build_run.supersede", "usr_cc").Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("supersede audits = %d, want 2", audits)
	}
}

func TestParallelBuildConcurrencyDoesNotWait(t *testing.T) {
	db := openBuildConcurrencyTestDB(t)
	base := time.Now().Add(-time.Hour).UTC()
	seedBuildRun(t, db, "bld_running", "running", base)
	current := seedBuildRun(t, db, "bld_current", "queued", base.Add(time.Minute))
	target := model.DeploymentTarget{ID: "dplt_cc", BuildConcurrencyPolicy: model.BuildConcurrencyParallel}
	if err := (&Runner{db: db}).applyBuildConcurrencyPolicy(t.Context(), current, target); err != nil {
		t.Fatalf("parallel build should start: %v", err)
	}
}

func TestNewerBuildReleasedPreventsAutoDeployRollback(t *testing.T) {
	db := openBuildConcurrencyTestDB(t)
	runner := &Runner{db: db}
	base := time.Now().Add(-time.Hour).UTC()
	older := seedBuildRun(t, db, "bld_older", "succeeded", base)
	newer := seedBuildRun(t, db, "bld_newer", "succeeded", base.Add(time.Minute))

	if superseded, err := runner.newerBuildReleased(older, "dplt_cc"); err != nil || superseded {
		t.Fatalf("no release yet: superseded=%v err=%v", superseded, err)
	}
	release := model.Release{ID: "rel_newer", ProjectID: "prj_cc", ApplicationID: "app_cc", DeploymentTargetID: "dplt_cc", BuildRunID: newer.ID, Status: "succeeded", Type: "deploy"}
	if err := db.Create(&release).Error; err != nil {
		t.Fatal(err)
	}
	if superseded, err := runner.newerBuildReleased(older, "dplt_cc"); err != nil || !superseded {
		t.Fatalf("older build after newer release: superseded=%v err=%v", superseded, err)
	}
	if superseded, err := runner.newerBuildReleased(newer, "dplt_cc"); err != nil || superseded {
		t.Fatalf("newest build should still deploy: superseded=%v err=%v", superseded, err)
	}
}
