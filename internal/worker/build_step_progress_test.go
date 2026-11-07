package worker

import (
	"strings"
	"testing"
	"unicode/utf8"

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
		progress := buildStepProgress{counter: builder.NewStepCounter(), sensitiveValues: []string{"s3cret"}}
		for _, line := range []string{"#5 [1/3] FROM alpine", "#5 DONE 0.1s", "#6 [2/3] RUN echo s3cret"} {
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
	if got.CurrentStep == "" || strings.Contains(got.CurrentStep, "s3cret") || !strings.HasPrefix(got.CurrentStep, "[2/3] RUN echo ") {
		t.Fatalf("running job current step = %q, want redacted instruction", got.CurrentStep)
	}
	var untouched model.BuildJob
	if err := db.First(&untouched, "id = ?", finished.ID).Error; err != nil {
		t.Fatal(err)
	}
	if untouched.StepsCompleted != 0 || untouched.StepsTotal != 0 {
		t.Fatalf("finished job steps = %d/%d, want untouched", untouched.StepsCompleted, untouched.StepsTotal)
	}
}

func TestDecodeSourceCommitMessageFromExecutorResult(t *testing.T) {
	if got := decodeSourceCommitMessage("ZmVhdDogYWRkIGxvZ2luCgpCb2R5CXRhYgo="); got != "feat: add login\n\nBody\ttab" {
		t.Fatalf("decoded message = %q", got)
	}
	if got := decodeSourceCommitMessage("not base64!"); got != "" {
		t.Fatalf("invalid payload decoded to %q", got)
	}
}

func TestBuildCurrentStepTextTruncatesWithEllipsis(t *testing.T) {
	long := "[builder 2/2] RUN " + strings.Repeat("构建", maxBuildCurrentStepBytes)
	got := buildCurrentStepText(long, nil)
	if len(got) > maxBuildCurrentStepBytes || !strings.HasSuffix(got, "…") || !utf8.ValidString(got) {
		t.Fatalf("truncated step len=%d suffix=%v valid=%v", len(got), strings.HasSuffix(got, "…"), utf8.ValidString(got))
	}
	if short := buildCurrentStepText("[1/2] RUN true", nil); short != "[1/2] RUN true" {
		t.Fatalf("short step = %q", short)
	}
}
