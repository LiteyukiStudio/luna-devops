package worker

import (
	"context"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestParseKubectlRuntimeSyntheticTargetIDRoundTrip(t *testing.T) {
	value := "kubectl:rcl_demo:prj_demo:app_demo:uid_demo"
	clusterID, projectID, applicationID, resourceUID, ok := kubeprovider.ParseKubectlRuntimeSyntheticTargetID(value)
	if !ok || clusterID != "rcl_demo" || projectID != "prj_demo" || applicationID != "app_demo" || resourceUID != "uid_demo" {
		t.Fatalf("parse result = %q %q %q %q %v", clusterID, projectID, applicationID, resourceUID, ok)
	}
}

func TestValidateKubectlRuntimeWorkloadRejectsCrossBoundaryObservation(t *testing.T) {
	cluster := model.RuntimeCluster{ID: "rcl_demo"}
	project := model.Project{ID: "prj_demo", KubernetesNamespace: "project-demo"}
	valid := kubeprovider.KubectlRuntimeWorkload{
		RuntimeClusterID: "rcl_demo", ProjectID: "prj_demo", Namespace: "project-demo", ResourceUID: "uid_demo",
		ManagementSource: kubeprovider.KubectlGatewayManagementSourceValue,
	}
	if err := validateKubectlRuntimeWorkload(cluster, project, valid); err != nil {
		t.Fatalf("valid workload rejected: %v", err)
	}
	invalid := []kubeprovider.KubectlRuntimeWorkload{
		func() kubeprovider.KubectlRuntimeWorkload {
			value := valid
			value.RuntimeClusterID = "rcl_foreign"
			return value
		}(),
		func() kubeprovider.KubectlRuntimeWorkload {
			value := valid
			value.ProjectID = "prj_foreign"
			return value
		}(),
		func() kubeprovider.KubectlRuntimeWorkload { value := valid; value.Namespace = "foreign"; return value }(),
		func() kubeprovider.KubectlRuntimeWorkload { value := valid; value.ResourceUID = ""; return value }(),
		func() kubeprovider.KubectlRuntimeWorkload {
			value := valid
			value.ManagementSource = kubeprovider.PlatformManagementSourceValue
			return value
		}(),
	}
	for index, workload := range invalid {
		if err := validateKubectlRuntimeWorkload(cluster, project, workload); err == nil {
			t.Fatalf("invalid workload %d was accepted: %#v", index, workload)
		}
	}
}

func TestRecordKubectlRuntimeObservationPreservesTraceContext(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 port=1 user=context_test dbname=context_test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	var observed context.Context
	if err := db.Callback().Create().Before("gorm:create").Register("test:capture_kubectl_runtime_observation_context", func(tx *gorm.DB) {
		observed = tx.Statement.Context
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	traceID := trace.TraceID{7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7}
	parent := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  trace.SpanID{1, 1, 1, 1, 1, 1, 1, 1},
		Remote:  true,
	}))
	now := time.Now().UTC()
	err = (&Runner{db: db}).recordKubectlRuntimeObservation(parent,
		model.RuntimeCluster{ID: "rcl_demo", CPURequestPercent: 10, MemoryRequestPercent: 25, CPULimitPercent: 100, MemoryLimitPercent: 100},
		kubeprovider.KubectlRuntimeWorkload{
			RuntimeClusterID:       "rcl_demo",
			ProjectID:              "prj_demo",
			ApplicationID:          "app_demo",
			ResourceUID:            "uid_demo",
			DesiredReplicas:        1,
			EffectiveCPURequest:    "250m",
			EffectiveMemoryRequest: "536870912",
			Status:                 "ready",
			CreatedAt:              now.Add(-time.Hour),
			ObservedAt:             now,
		},
		kubeprovider.RuntimeMetricsSnapshot{Available: true, PodCount: 1, ContainerCount: 1, CPUUsageMilli: 25, MemoryUsageBytes: 64 * 1024 * 1024},
		now,
	)
	if err != nil {
		t.Fatalf("recordKubectlRuntimeObservation() error = %v", err)
	}
	if observed == nil {
		t.Fatal("runtime observation did not execute a database create")
	}
	if got := trace.SpanContextFromContext(observed).TraceID(); got != traceID {
		t.Fatalf("database trace ID = %s, want %s", got, traceID)
	}
}
