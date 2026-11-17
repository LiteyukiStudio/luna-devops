package aiapi

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestBuildRunProgressUsesAuthoritativeState(t *testing.T) {
	now := time.Now().UTC()
	snapshot := buildRunProgress(model.BuildRun{
		ID:        "bldrun_1",
		Status:    "running",
		StartedAt: &now,
		UpdatedAt: now,
	})

	if snapshot.OperationID != "bldrun_1" || snapshot.OperationType != "build_run" || snapshot.Revision != aiProgressRevision(now) {
		t.Fatalf("unexpected identity: %#v", snapshot)
	}
	if snapshot.State != "running" || snapshot.Progress.Mode != "indeterminate" || snapshot.Progress.Value != nil {
		t.Fatalf("unexpected running progress: %#v", snapshot)
	}
	if len(snapshot.Steps) != 3 || snapshot.Steps[1].Status != "running" {
		t.Fatalf("unexpected steps: %#v", snapshot.Steps)
	}
}

func TestReleaseProgressOnlyReportsDeterminateCompletion(t *testing.T) {
	now := time.Now().UTC()
	snapshot := releaseProgress(model.Release{
		ID:         "rel_1",
		Status:     "succeeded",
		StartedAt:  &now,
		FinishedAt: &now,
		UpdatedAt:  now,
	})

	if snapshot.State != "succeeded" || snapshot.Progress.Mode != "determinate" || snapshot.Progress.Value == nil || *snapshot.Progress.Value != 100 {
		t.Fatalf("unexpected completed progress: %#v", snapshot)
	}
	if !aiProgressTerminal(snapshot.State) || snapshot.Error != nil {
		t.Fatalf("unexpected completed terminal state: %#v", snapshot)
	}

	failed := releaseProgress(model.Release{ID: "rel_2", Status: "failed", UpdatedAt: now})
	if failed.Progress.Mode != "indeterminate" || failed.Progress.Value != nil || failed.Error == nil {
		t.Fatalf("failed progress must not invent a percentage: %#v", failed)
	}
}

func TestAppTemplateProgressFollowsLinkedRelease(t *testing.T) {
	now := time.Now().UTC()
	snapshot := appTemplateReleaseProgress(
		model.AppTemplateInstallation{ID: "inst_1", ReleaseID: "rel_1"},
		[]model.Release{{ID: "rel_1", Status: "succeeded", FinishedAt: &now, UpdatedAt: now}},
	)

	if snapshot.OperationID != "inst_1" || snapshot.OperationType != "app_template_installation" {
		t.Fatalf("the card binding identity must remain the installation: %#v", snapshot)
	}
	if snapshot.State != "succeeded" || snapshot.FinishedAt == nil {
		t.Fatalf("the linked release terminal state was not projected: %#v", snapshot)
	}
}

func TestAppTemplateProgressAggregatesComponentReleases(t *testing.T) {
	now := time.Now().UTC()
	installation := model.AppTemplateInstallation{ID: "inst_multi", ReleaseID: "rel_web"}
	components := []model.AppTemplateInstallationComponent{
		{ComponentKey: "web", ReleaseID: "rel_web"}, {ComponentKey: "api", ReleaseID: "rel_api"}, {ComponentKey: "postgres", ReleaseID: "rel_db"},
	}
	if ids := appTemplateInstallationReleaseIDs(installation, components); len(ids) != 3 {
		t.Fatalf("component release ids = %#v", ids)
	}
	running := appTemplateReleaseProgress(installation, []model.Release{
		{ID: "rel_web", Status: "succeeded", FinishedAt: &now, UpdatedAt: now},
		{ID: "rel_api", Status: "running", UpdatedAt: now},
	})
	if running.State != "running" || running.FinishedAt != nil {
		t.Fatalf("partially finished installation must stay running: %#v", running)
	}
	failed := appTemplateReleaseProgress(installation, []model.Release{
		{ID: "rel_web", Status: "succeeded", FinishedAt: &now, UpdatedAt: now},
		{ID: "rel_api", Status: "failed", FailureCode: "rollout_failed", FinishedAt: &now, UpdatedAt: now},
	})
	if failed.State != "failed" || failed.FinishedAt == nil || failed.Error == nil || failed.Error.Code != "rollout_failed" {
		t.Fatalf("any failed component release must fail the installation with real failureCode: %#v", failed)
	}
	fallbackFailed := buildRunProgress(model.BuildRun{
		ID: "bldr_empty_code", Status: "failed", FinishedAt: &now, UpdatedAt: now,
	})
	if fallbackFailed.Error == nil || fallbackFailed.Error.Code != "ai.progress.operation_failed" {
		t.Fatalf("empty failureCode must fall back to ai.progress.operation_failed: %#v", fallbackFailed)
	}
	realBuildFailed := buildRunProgress(model.BuildRun{
		ID: "bldr_job_failed", Status: "failed", FailureCode: "job_failed", FinishedAt: &now, UpdatedAt: now,
	})
	if realBuildFailed.Error == nil || realBuildFailed.Error.Code != "job_failed" {
		t.Fatalf("expected buildRunProgress to forward FailureCode: %#v", realBuildFailed)
	}
	realReleaseFailed := releaseProgress(model.Release{
		ID: "rel_apply_failed", Status: "failed", FailureCode: "apply_failed", FinishedAt: &now, UpdatedAt: now,
	})
	if realReleaseFailed.Error == nil || realReleaseFailed.Error.Code != "apply_failed" {
		t.Fatalf("expected releaseProgress to forward FailureCode: %#v", realReleaseFailed)
	}
}

func TestInternalTaskStatusNormalizationForWaitForTask(t *testing.T) {
	now := time.Now().UTC()
	runningBuild := buildRunTaskStatusInternal(model.BuildRun{
		ID:                 "bldr_running",
		ProjectID:          "prj_1",
		ApplicationID:      "app_1",
		DeploymentTargetID: "dplt_1",
		Status:             "building",
		UpdatedAt:          now,
	})
	if runningBuild.TaskStatus != "running" || runningBuild.Terminal {
		t.Fatalf("expected building build_run to normalize to non-terminal running: %#v", runningBuild)
	}

	failedBuild := buildRunTaskStatusInternal(model.BuildRun{
		ID:          "bldr_failed",
		ProjectID:   "prj_1",
		Status:      "failed",
		FailureCode: "build.dockerfile_step_failed",
		FinishedAt:  &now,
		UpdatedAt:   now,
	})
	if failedBuild.TaskStatus != "failed" || !failedBuild.Terminal || failedBuild.FailureCode != "build.dockerfile_step_failed" {
		t.Fatalf("expected failed build_run to carry failureCode and terminal=true: %#v", failedBuild)
	}

	succeededRelease := releaseTaskStatusInternal(model.Release{
		ID:                 "rel_ok",
		ProjectID:          "prj_1",
		ApplicationID:      "app_1",
		DeploymentTargetID: "dplt_1",
		Status:             "succeeded",
		ImageRef:           "registry.example/app:v1",
		Revision:           3,
		FinishedAt:         &now,
		UpdatedAt:          now,
	})
	if succeededRelease.TaskStatus != "succeeded" || !succeededRelease.Terminal || succeededRelease.ImageRef != "registry.example/app:v1" || succeededRelease.Revision != 3 {
		t.Fatalf("expected succeeded release snapshot: %#v", succeededRelease)
	}

	canceledRelease := releaseTaskStatusInternal(model.Release{
		ID:        "rel_canceled",
		ProjectID: "prj_1",
		Status:    "cancelled",
		UpdatedAt: now,
	})
	if canceledRelease.TaskStatus != "canceled" || !canceledRelease.Terminal {
		t.Fatalf("expected cancelled release to normalize to canceled terminal: %#v", canceledRelease)
	}
}
