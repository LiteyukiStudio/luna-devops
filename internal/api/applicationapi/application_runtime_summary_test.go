package applicationapi

import (
	"reflect"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
)

func TestSummarizeApplicationDeploymentTargetsMatrix(t *testing.T) {
	finished := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	released := func(ids ...string) map[string]*applicationCurrentRelease {
		result := map[string]*applicationCurrentRelease{}
		for _, id := range ids {
			result[id] = &applicationCurrentRelease{ID: "rel-" + id, DeploymentTargetID: id, DeployedAt: finished}
		}
		return result
	}
	target := func(id, status string, desired, ready int32) model.DeploymentTarget {
		return model.DeploymentTarget{ID: id, Enabled: true, Status: status, DesiredReplicas: desired, ReadyReplicas: ready}
	}
	disabled := func(id, status string, desired, ready int32) model.DeploymentTarget {
		item := target(id, status, desired, ready)
		item.Enabled = false
		return item
	}
	tests := []struct {
		name     string
		targets  []model.DeploymentTarget
		releases map[string]*applicationCurrentRelease
		want     applicationDeploymentSummary
	}{
		{name: "no targets", want: applicationDeploymentSummary{Status: applicationRuntimeNotDeployed}},
		{
			name:    "enabled targets without a succeeded release are not deployed",
			targets: []model.DeploymentTarget{target("a", observation.StatusNotFound, 0, 0), target("b", observation.StatusNotConfigured, 0, 0)},
			want:    applicationDeploymentSummary{TargetCount: 2, Status: applicationRuntimeNotDeployed},
		},
		{
			name:     "ready replicas are aggregated over deployed targets",
			targets:  []model.DeploymentTarget{target("a", observation.StatusReady, 2, 2), target("b", observation.StatusReady, 1, 1)},
			releases: released("a", "b"),
			want:     applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 3, ReadyReplicas: 3, Status: observation.StatusReady},
		},
		{
			name:     "disabled target never affects health or replicas",
			targets:  []model.DeploymentTarget{target("a", observation.StatusReady, 1, 1), disabled("b", observation.StatusDisabled, 2, 0)},
			releases: released("a", "b"),
			want:     applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 1, ReadyReplicas: 1, Status: observation.StatusReady},
		},
		{
			name:     "never deployed target does not drag a ready application",
			targets:  []model.DeploymentTarget{target("a", observation.StatusReady, 1, 1), target("b", observation.StatusNotConfigured, 0, 0)},
			releases: released("a"),
			want:     applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 1, ReadyReplicas: 1, Status: observation.StatusReady},
		},
		{
			name:     "only disabled deployed targets mean not deployed",
			targets:  []model.DeploymentTarget{disabled("a", observation.StatusDisabled, 1, 1)},
			releases: released("a"),
			want:     applicationDeploymentSummary{TargetCount: 1, Status: applicationRuntimeNotDeployed},
		},
		{
			name:     "missing workload of a deployed target needs attention",
			targets:  []model.DeploymentTarget{target("a", observation.StatusReady, 1, 1), target("b", observation.StatusNotFound, 0, 0)},
			releases: released("a", "b"),
			want:     applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 1, ReadyReplicas: 1, Status: observation.StatusNotFound},
		},
		{
			name:     "deployed target whose cluster disappeared is not configured",
			targets:  []model.DeploymentTarget{target("a", observation.StatusNotConfigured, 0, 0)},
			releases: released("a"),
			want:     applicationDeploymentSummary{TargetCount: 1, Status: observation.StatusNotConfigured},
		},
		{
			name:     "partial replicas are progressing",
			targets:  []model.DeploymentTarget{target("a", observation.StatusProgressing, 3, 1), target("b", observation.StatusReady, 1, 1)},
			releases: released("a", "b"),
			want:     applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 4, ReadyReplicas: 2, Status: observation.StatusProgressing},
		},
		{
			name:     "unavailable wins over degraded",
			targets:  []model.DeploymentTarget{target("a", observation.StatusDegraded, 2, 1), target("b", observation.StatusUnavailable, 0, 0)},
			releases: released("a", "b"),
			want:     applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 2, ReadyReplicas: 1, Status: observation.StatusUnavailable},
		},
		{
			name:     "empty observation is unknown",
			targets:  []model.DeploymentTarget{target("a", "", 0, 0)},
			releases: released("a"),
			want:     applicationDeploymentSummary{TargetCount: 1, Status: observation.StatusUnknown},
		},
		{
			name:     "scale to zero has a distinct observed runtime status",
			targets:  []model.DeploymentTarget{target("a", observation.StatusScaledToZero, 0, 0)},
			releases: released("a"),
			want:     applicationDeploymentSummary{TargetCount: 1, Status: observation.StatusScaledToZero},
		},
		{
			name:     "ready and scaled targets remain ready",
			targets:  []model.DeploymentTarget{target("a", observation.StatusReady, 1, 1), target("b", observation.StatusScaledToZero, 0, 0)},
			releases: released("a", "b"),
			want:     applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 1, ReadyReplicas: 1, Status: observation.StatusReady},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := summarizeApplicationDeploymentTargets(test.targets, test.releases)
			got.Targets = nil
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("summarizeApplicationDeploymentTargets() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSummarizeApplicationDeploymentTargetsReportsPerTargetStatus(t *testing.T) {
	finished := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	devRelease := &applicationCurrentRelease{ID: "rel-dev", DeploymentTargetID: "dev-target", DeployedAt: finished}
	prodRelease := &applicationCurrentRelease{ID: "rel-prod", DeploymentTargetID: "prod-target", DeployedAt: finished}
	summary := summarizeApplicationDeploymentTargets([]model.DeploymentTarget{
		{ID: "dev-target", Stage: "dev", Enabled: true, Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1},
		{ID: "test-target", Stage: "test", Status: observation.StatusDisabled, DesiredReplicas: 1, ReadyReplicas: 1},
		{ID: "staging-target", Stage: "staging", Enabled: true, Status: observation.StatusNotConfigured, ObservationCode: "deployment_target.runtime_cluster_not_configured"},
		{ID: "prod-target", Stage: "prod", Enabled: true, Status: observation.StatusDegraded, DesiredReplicas: 2, ReadyReplicas: 1},
	}, map[string]*applicationCurrentRelease{"dev-target": devRelease, "prod-target": prodRelease})

	want := []applicationDeploymentTargetSummary{
		{ID: "prod-target", Stage: "prod", Enabled: true, Deployed: true, Status: observation.StatusDegraded, DesiredReplicas: 2, ReadyReplicas: 1, CurrentRelease: prodRelease},
		{ID: "dev-target", Stage: "dev", Enabled: true, Deployed: true, Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1, CurrentRelease: devRelease},
		{ID: "staging-target", Stage: "staging", Enabled: true, Status: applicationRuntimeNotDeployed, ObservationCode: "deployment_target.runtime_cluster_not_configured"},
		{ID: "test-target", Stage: "test", Status: observation.StatusDisabled, DesiredReplicas: 1, ReadyReplicas: 1},
	}
	if !reflect.DeepEqual(summary.Targets, want) {
		t.Fatalf("summarizeApplicationDeploymentTargets().Targets = %#v, want %#v", summary.Targets, want)
	}
}

func TestPrimaryGatewayRoutesPrefersDefaultThenStage(t *testing.T) {
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	targets := map[string]model.DeploymentTarget{
		"dev":  {ID: "dev", ApplicationID: "app-a", Stage: "dev"},
		"prod": {ID: "prod", ApplicationID: "app-a", Stage: "prod"},
		"b":    {ID: "b", ApplicationID: "app-b", Stage: "prod"},
	}
	routes := []model.GatewayRoute{
		{ID: "dev-route", ApplicationID: "app-a", DeploymentTargetID: "dev", Host: "dev.example.com", Enabled: true, CreatedAt: base},
		{ID: "prod-route", ApplicationID: "app-a", DeploymentTargetID: "prod", Host: "prod.example.com", Enabled: true, CreatedAt: base.Add(time.Hour)},
		{ID: "b-old", ApplicationID: "app-b", DeploymentTargetID: "b", Host: "old.example.com", Enabled: true, CreatedAt: base},
		{ID: "b-default", ApplicationID: "app-b", DeploymentTargetID: "b", Host: "default.example.com", Enabled: true, IsDefault: true, CreatedAt: base.Add(time.Hour)},
		{ID: "disabled", ApplicationID: "app-c", DeploymentTargetID: "b", Host: "disabled.example.com", Enabled: false},
		{ID: "orphan", ApplicationID: "app-d", DeploymentTargetID: "deleted-target", Host: "orphan.example.com", Enabled: true},
	}

	got := primaryGatewayRoutes(routes, targets)
	if len(got) != 2 {
		t.Fatalf("primary routes = %#v, want app-a and app-b only", got)
	}
	if got["app-a"].ID != "prod-route" {
		t.Fatalf("app-a primary route = %q, want prod-route", got["app-a"].ID)
	}
	if got["app-b"].ID != "b-default" {
		t.Fatalf("app-b primary route = %q, want b-default", got["app-b"].ID)
	}
}

func TestPrimaryGatewayRoutesWithoutRoutes(t *testing.T) {
	if got := primaryGatewayRoutes(nil, map[string]model.DeploymentTarget{"t": {ID: "t"}}); len(got) != 0 {
		t.Fatalf("primary routes = %#v, want empty", got)
	}
}

func TestLatestSucceededReleasesByTargetUsesFinishedAt(t *testing.T) {
	older := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	got := latestSucceededReleasesByTarget([]model.Release{
		// Created later but finished earlier: finished_at decides the current version.
		{ID: "rel-created-late", ApplicationID: "app-a", DeploymentTargetID: "t1", ImageRef: "repo/app:v1", Status: "succeeded", FinishedAt: &older, CreatedAt: newer},
		{ID: "rel-finished-late", ApplicationID: "app-a", DeploymentTargetID: "t1", ImageRef: "repo/app:v2", Status: "succeeded", FinishedAt: &newer, CreatedAt: older},
		{ID: "rel-failed", ApplicationID: "app-b", DeploymentTargetID: "t2", ImageRef: "repo/b:v1", Status: "failed", FinishedAt: &newer},
		{ID: "rel-unfinished", ApplicationID: "app-c", DeploymentTargetID: "t3", ImageRef: "repo/c:v1", Status: "succeeded"},
	})

	want := map[string]*applicationCurrentRelease{
		"t1": {ID: "rel-finished-late", DeploymentTargetID: "t1", ImageRef: "repo/app:v2", DeployedAt: newer},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("latestSucceededReleasesByTarget() = %#v, want %#v", got, want)
	}
	if empty := latestSucceededReleasesByTarget(nil); len(empty) != 0 {
		t.Fatalf("latestSucceededReleasesByTarget(nil) = %#v, want empty", empty)
	}
}

func TestApplicationCurrentReleaseOfOnlyUsesExistingTargets(t *testing.T) {
	older := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	releases := map[string]*applicationCurrentRelease{
		"dev":     {ID: "rel-dev", DeploymentTargetID: "dev", DeployedAt: older},
		"prod":    {ID: "rel-prod", DeploymentTargetID: "prod", DeployedAt: newer},
		"deleted": {ID: "rel-deleted", DeploymentTargetID: "deleted", DeployedAt: newer.Add(time.Hour)},
	}
	got := applicationCurrentReleaseOf([]model.DeploymentTarget{{ID: "dev"}, {ID: "prod"}}, releases)
	if got == nil || got.ID != "rel-prod" {
		t.Fatalf("applicationCurrentReleaseOf() = %#v, want rel-prod", got)
	}
	if got := applicationCurrentReleaseOf([]model.DeploymentTarget{{ID: "none"}}, releases); got != nil {
		t.Fatalf("applicationCurrentReleaseOf() without releases = %#v, want nil", got)
	}
}
