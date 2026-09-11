package deploymentapi

import (
	"context"
	"errors"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type recordingApplicationWorkloadScaler struct {
	spec     kubeprovider.ApplicationResourcesSpec
	replicas int32
	err      error
	calls    int
}

func (s *recordingApplicationWorkloadScaler) ScaleApplicationWorkload(_ context.Context, spec kubeprovider.ApplicationResourcesSpec, replicas int32) error {
	s.calls++
	s.spec = spec
	s.replicas = replicas
	return s.err
}

func TestScaleDeploymentTargetWorkloadUsesPersistedRuntimeIdentity(t *testing.T) {
	scaler := &recordingApplicationWorkloadScaler{}
	target := model.DeploymentTarget{
		ID:             "dplt_demo",
		ProjectID:      "prj_demo",
		ApplicationID:  "app_demo",
		KubernetesName: "luna-api-prod",
		WorkloadType:   "StatefulSet",
	}
	found, err := scaleDeploymentTargetWorkload(t.Context(), scaler, "luna-demo", target, 0)
	if err != nil {
		t.Fatalf("scaleDeploymentTargetWorkload() error = %v", err)
	}
	if !found || scaler.calls != 1 || scaler.replicas != 0 {
		t.Fatalf("scale result found=%v calls=%d replicas=%d", found, scaler.calls, scaler.replicas)
	}
	if scaler.spec.Name != target.KubernetesName || scaler.spec.Namespace != "luna-demo" ||
		scaler.spec.ProjectID != target.ProjectID || scaler.spec.ApplicationID != target.ApplicationID ||
		scaler.spec.DeploymentTargetID != target.ID || scaler.spec.WorkloadType != "StatefulSet" {
		t.Fatalf("scale spec = %#v", scaler.spec)
	}
}

func TestScaleDeploymentTargetWorkloadTreatsMissingRuntimeAsNoop(t *testing.T) {
	scaler := &recordingApplicationWorkloadScaler{err: apierrors.NewNotFound(schema.GroupResource{Resource: "deployments"}, "luna-api-prod")}
	found, err := scaleDeploymentTargetWorkload(t.Context(), scaler, "luna-demo", model.DeploymentTarget{}, 2)
	if err != nil || found {
		t.Fatalf("missing workload result found=%v err=%v", found, err)
	}
}

func TestScaleDeploymentTargetWorkloadRejectsNegativeReplicas(t *testing.T) {
	scaler := &recordingApplicationWorkloadScaler{}
	found, err := scaleDeploymentTargetWorkload(t.Context(), scaler, "luna-demo", model.DeploymentTarget{}, -1)
	if err == nil || found || scaler.calls != 0 {
		t.Fatalf("negative replicas result found=%v calls=%d err=%v", found, scaler.calls, err)
	}
}

func TestScaleDeploymentTargetWorkloadReturnsProviderError(t *testing.T) {
	wantErr := errors.New("cluster unavailable")
	scaler := &recordingApplicationWorkloadScaler{err: wantErr}
	found, err := scaleDeploymentTargetWorkload(t.Context(), scaler, "luna-demo", model.DeploymentTarget{}, 3)
	if found || !errors.Is(err, wantErr) {
		t.Fatalf("provider error result found=%v err=%v", found, err)
	}
}
