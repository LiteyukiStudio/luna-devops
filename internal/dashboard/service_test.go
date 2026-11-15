package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestAggregateAttentionGroupsConsecutiveFailuresAndStopsAtRecovery(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	events := []model.PlatformEvent{
		{ID: "evt_latest_failure", Type: "build.failed", Category: "build", Severity: "error", Status: "failed", ProjectID: "prj_1", ApplicationID: "app_1", OccurredAt: now},
		{ID: "evt_previous_failure", Type: "build.failed", Category: "build", Severity: "error", Status: "failed", ProjectID: "prj_1", ApplicationID: "app_1", OccurredAt: now.Add(-time.Minute)},
		{ID: "evt_recovered", Type: "build.succeeded", Category: "build", Severity: "info", Status: "succeeded", ProjectID: "prj_1", ApplicationID: "app_1", OccurredAt: now.Add(-2 * time.Minute)},
		{ID: "evt_old_failure", Type: "build.failed", Category: "build", Severity: "error", Status: "failed", ProjectID: "prj_1", ApplicationID: "app_1", OccurredAt: now.Add(-3 * time.Minute)},
	}

	items := aggregateAttention(events, entityReferences{projects: map[string]EntityRef{}, apps: map[string]EntityRef{}, targets: map[string]EntityRef{}}, now.Add(-time.Hour))
	if len(items) != 1 {
		t.Fatalf("attention items = %d, want 1", len(items))
	}
	if items[0].Occurrences != 2 {
		t.Fatalf("occurrences = %d, want 2 consecutive failures", items[0].Occurrences)
	}
}

func TestAggregateAttentionOmitsRecoveredResource(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	events := []model.PlatformEvent{
		{ID: "evt_success", Type: "release.succeeded", Category: "release", Severity: "info", Status: "succeeded", ProjectID: "prj_1", DeploymentTargetID: "dplt_1", OccurredAt: now},
		{ID: "evt_failure", Type: "release.failed", Category: "release", Severity: "error", Status: "failed", ProjectID: "prj_1", DeploymentTargetID: "dplt_1", OccurredAt: now.Add(-time.Minute)},
	}

	items := aggregateAttention(events, entityReferences{projects: map[string]EntityRef{}, apps: map[string]EntityRef{}, targets: map[string]EntityRef{}}, now.Add(-time.Hour))
	if len(items) != 0 {
		t.Fatalf("attention items = %#v, want none after recovery", items)
	}
}

func TestFoldRecentActivitiesClosesTerminalWorkflowAndKeepsInProgress(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	refs := entityReferences{projects: map[string]EntityRef{}, apps: map[string]EntityRef{}, targets: map[string]EntityRef{}}
	events := []model.PlatformEvent{
		{ID: "evt_build_running", Type: "build.started", Category: "build", Status: "in_progress", CorrelationID: "bldr_running", OccurredAt: now},
		{ID: "evt_rel_succeeded", Type: "release.succeeded", Category: "release", Status: "succeeded", CorrelationID: "rel_1", OccurredAt: now.Add(-10 * time.Second)},
		{ID: "evt_rel_started", Type: "release.started", Category: "release", Status: "in_progress", CorrelationID: "rel_1", OccurredAt: now.Add(-40 * time.Second)},
		{ID: "evt_build_failed", Type: "build.failed", Category: "build", Status: "failed", CorrelationID: "bldr_1", OccurredAt: now.Add(-time.Minute)},
		{ID: "evt_build_started", Type: "build.started", Category: "build", Status: "in_progress", CorrelationID: "bldr_1", OccurredAt: now.Add(-2 * time.Minute)},
		{ID: "evt_build_queued", Type: "build.queued", Category: "build", Status: "in_progress", CorrelationID: "bldr_1", OccurredAt: now.Add(-3 * time.Minute)},
	}

	got := activityIDs(foldRecentActivities(events, refs, activityLimit))
	want := []string{"evt_build_running", "evt_rel_succeeded", "evt_build_failed"}
	if !slices.Equal(got, want) {
		t.Fatalf("folded activity IDs = %v, want %v", got, want)
	}
}

func TestFoldRecentActivitiesSeparatesCategoriesSharingCorrelationID(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	refs := entityReferences{projects: map[string]EntityRef{}, apps: map[string]EntityRef{}, targets: map[string]EntityRef{}}
	events := []model.PlatformEvent{
		{ID: "evt_rel_succeeded", Type: "release.succeeded", Category: "release", Status: "succeeded", CorrelationID: "rel_shared", OccurredAt: now},
		{ID: "evt_hook_succeeded", Type: "hook.succeeded", Category: "hook", Status: "succeeded", CorrelationID: "rel_shared", OccurredAt: now.Add(-5 * time.Second)},
		{ID: "evt_hook_started", Type: "hook.started", Category: "hook", Status: "in_progress", CorrelationID: "rel_shared", OccurredAt: now.Add(-15 * time.Second)},
		{ID: "evt_rel_started", Type: "release.started", Category: "release", Status: "in_progress", CorrelationID: "rel_shared", OccurredAt: now.Add(-30 * time.Second)},
	}

	got := activityIDs(foldRecentActivities(events, refs, activityLimit))
	want := []string{"evt_rel_succeeded", "evt_hook_succeeded"}
	if !slices.Equal(got, want) {
		t.Fatalf("folded activity IDs = %v, want %v", got, want)
	}
}

func TestFoldRecentActivitiesFallsBackToResourceIdentityOrEventIDAndHonorsLimit(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	refs := entityReferences{projects: map[string]EntityRef{}, apps: map[string]EntityRef{}, targets: map[string]EntityRef{}}
	events := []model.PlatformEvent{
		{ID: "evt_res_done", Type: "release.succeeded", Category: "release", Status: "succeeded", ResourceType: "release", ResourceID: "rel_legacy", OccurredAt: now},
		{ID: "evt_res_start", Type: "release.started", Category: "release", Status: "in_progress", ResourceType: "release", ResourceID: "rel_legacy", OccurredAt: now.Add(-time.Minute)},
		{ID: "evt_plain_2", Type: "build.failed", Category: "build", Status: "failed", OccurredAt: now.Add(-2 * time.Minute)},
		{ID: "evt_plain_1", Type: "build.failed", Category: "build", Status: "failed", OccurredAt: now.Add(-3 * time.Minute)},
	}

	gotAll := activityIDs(foldRecentActivities(events, refs, activityLimit))
	wantAll := []string{"evt_res_done", "evt_plain_2", "evt_plain_1"}
	if !slices.Equal(gotAll, wantAll) {
		t.Fatalf("folded activity IDs = %v, want %v", gotAll, wantAll)
	}

	gotLimited := activityIDs(foldRecentActivities(events, refs, 2))
	wantLimited := []string{"evt_res_done", "evt_plain_2"}
	if !slices.Equal(gotLimited, wantLimited) {
		t.Fatalf("limited activity IDs = %v, want %v", gotLimited, wantLimited)
	}
}

func activityIDs(activities []Activity) []string {
	ids := make([]string, 0, len(activities))
	for _, activity := range activities {
		ids = append(ids, activity.ID)
	}
	return ids
}
