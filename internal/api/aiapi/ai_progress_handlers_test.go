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
		model.AppTemplateInstallation{ID: "inst_1", Status: "deploying", ReleaseID: "rel_1"},
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
	installation := model.AppTemplateInstallation{ID: "inst_multi", Status: "deploying", ReleaseID: "rel_web"}
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
		{ID: "rel_api", Status: "failed", FinishedAt: &now, UpdatedAt: now},
	})
	if failed.State != "failed" || failed.FinishedAt == nil {
		t.Fatalf("any failed component release must fail the installation: %#v", failed)
	}
}
