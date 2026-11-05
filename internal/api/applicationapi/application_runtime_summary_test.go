package applicationapi

import (
	"reflect"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
)

func TestSummarizeApplicationDeploymentTargets(t *testing.T) {
	tests := []struct {
		name    string
		targets []model.DeploymentTarget
		want    applicationDeploymentSummary
	}{
		{name: "not deployed", want: applicationDeploymentSummary{Status: applicationRuntimeNotDeployed}},
		{
			name: "ready replicas are aggregated",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusReady, DesiredReplicas: 2, ReadyReplicas: 2},
				{Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1},
			},
			want: applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 3, ReadyReplicas: 3, Status: observation.StatusReady},
		},
		{
			name: "partial replicas are progressing",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusProgressing, DesiredReplicas: 3, ReadyReplicas: 1},
			},
			want: applicationDeploymentSummary{TargetCount: 1, DesiredReplicas: 3, ReadyReplicas: 1, Status: observation.StatusProgressing},
		},
		{
			name: "unavailable observation is not reported as undeployed",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusUnavailable},
			},
			want: applicationDeploymentSummary{TargetCount: 1, Status: observation.StatusUnavailable},
		},
		{
			name: "scale to zero has a distinct observed runtime status",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusScaledToZero, DesiredReplicas: 0, ReadyReplicas: 0},
			},
			want: applicationDeploymentSummary{TargetCount: 1, Status: observation.StatusScaledToZero},
		},
		{
			name: "ready and scaled targets remain ready when the aggregate serves replicas",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1},
				{Status: observation.StatusScaledToZero, DesiredReplicas: 0, ReadyReplicas: 0},
			},
			want: applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 1, ReadyReplicas: 1, Status: observation.StatusReady},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := summarizeApplicationDeploymentTargets(test.targets)
			got.Targets = nil
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("summarizeApplicationDeploymentTargets() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSummarizeApplicationDeploymentTargetsKeepsStageStatusSeparate(t *testing.T) {
	summary := summarizeApplicationDeploymentTargets([]model.DeploymentTarget{
		{ID: "dev-target", Stage: "dev", Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1},
		{ID: "test-target", Stage: "test", Status: "disabled"},
		{ID: "prod-target", Stage: "prod", Status: observation.StatusDegraded, DesiredReplicas: 2, ReadyReplicas: 1},
	})

	want := []applicationDeploymentTargetSummary{
		{ID: "prod-target", Stage: "prod", Status: observation.StatusDegraded, DesiredReplicas: 2, ReadyReplicas: 1},
		{ID: "dev-target", Stage: "dev", Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1},
		{ID: "test-target", Stage: "test", Status: "disabled"},
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

func TestLatestSucceededReleases(t *testing.T) {
	older := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	got := latestSucceededReleases([]model.Release{
		{ID: "rel-old", ApplicationID: "app-a", DeploymentTargetID: "t1", ImageRef: "repo/app:v1", Status: "succeeded", FinishedAt: &older},
		{ID: "rel-new", ApplicationID: "app-a", DeploymentTargetID: "t1", ImageRef: "repo/app:v2", Status: "succeeded", FinishedAt: &newer},
		{ID: "rel-failed", ApplicationID: "app-b", ImageRef: "repo/b:v1", Status: "failed", FinishedAt: &newer},
		{ID: "rel-running", ApplicationID: "app-c", ImageRef: "repo/c:v1", Status: "succeeded"},
	})

	want := map[string]*applicationCurrentRelease{
		"app-a": {ID: "rel-new", DeploymentTargetID: "t1", ImageRef: "repo/app:v2", DeployedAt: newer},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("latestSucceededReleases() = %#v, want %#v", got, want)
	}
	if empty := latestSucceededReleases(nil); len(empty) != 0 {
		t.Fatalf("latestSucceededReleases(nil) = %#v, want empty", empty)
	}
}
