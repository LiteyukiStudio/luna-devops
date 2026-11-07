package worker

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestRecordBuildStepProgressUpdatesOnlyRunningJobs(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "build_step_progress_test",
		Migrate:      func(db *gorm.DB) error { return db.AutoMigrate(&model.BuildJob{}) },
	})
	runner := &Runner{db: db}
	running := model.BuildJob{ID: "job_running", BuildRunID: "bld_1", ProjectID: "prj_1", Status: "running"}
	finished := model.BuildJob{ID: "job_finished", BuildRunID: "bld_2", ProjectID: "prj_1", Status: "succeeded"}
	if err := db.Create(&[]model.BuildJob{running, finished}).Error; err != nil {
		t.Fatal(err)
	}
	for _, job := range []model.BuildJob{running, finished} {
		progress := buildStepProgress{counter: builder.NewStepCounter()}
		for _, line := range []string{"#5 [1/3] FROM alpine", "#5 DONE 0.1s", "#6 [2/3] RUN true"} {
			progress.dirty = progress.counter.Observe(line) || progress.dirty
		}
		runner.recordBuildStepProgress(t.Context(), job, &progress)
	}

	var got model.BuildJob
	if err := db.First(&got, "id = ?", running.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.StepsCompleted != 1 || got.StepsTotal != 3 {
		t.Fatalf("running job steps = %d/%d, want 1/3", got.StepsCompleted, got.StepsTotal)
	}
	var untouched model.BuildJob
	if err := db.First(&untouched, "id = ?", finished.ID).Error; err != nil {
		t.Fatal(err)
	}
	if untouched.StepsCompleted != 0 || untouched.StepsTotal != 0 {
		t.Fatalf("finished job steps = %d/%d, want untouched", untouched.StepsCompleted, untouched.StepsTotal)
	}
}
