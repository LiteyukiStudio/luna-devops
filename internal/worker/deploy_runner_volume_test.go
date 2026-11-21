package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHandleDeployRunDistinguishesMissingVolumeClaimFromOwnershipConflict(t *testing.T) {
	const (
		projectID     = "prj_deploy_vol"
		applicationID = "app_deploy_vol"
		targetID      = "dplt_deploy_vol"
		releaseID     = "rel_deploy_vol"
		clusterID     = "rcl_deploy_vol"
		namespace     = "luna-deploy-vol"
		volumeID      = "pvol_deploy_vol"
		claimName     = "pvc-deploy-vol"
	)
	mountPath := "/var/lib/data"

	for _, tc := range []struct {
		name             string
		ownershipMode    string
		pendingOperation string
		objects          []runtime.Object
		wantFailureCode  string
		wantErrTarget    error
	}{
		{
			name:             "missing_pvc_reports_volume_not_ready",
			ownershipMode:    model.ProjectVolumeOwnershipManaged,
			pendingOperation: volume.OperationProvision,
			objects:          nil,
			wantFailureCode:  model.ReleaseFailureVolumeNotReady,
			wantErrTarget:    kubeprovider.ErrProjectVolumeClaimNotFound,
		},
		{
			name:             "unattachable_import_volume_reports_volume_not_ready",
			ownershipMode:    model.ProjectVolumeOwnershipManaged,
			pendingOperation: volume.OperationImport,
			objects:          nil,
			wantFailureCode:  model.ReleaseFailureVolumeNotReady,
			wantErrTarget:    errDeploymentVolumeNotAttachable,
		},
		{
			name:             "conflicting_pvc_labels_reports_preflight_failed",
			ownershipMode:    model.ProjectVolumeOwnershipManaged,
			pendingOperation: volume.OperationProvision,
			objects: []runtime.Object{
				&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
					Name:      claimName,
					Namespace: namespace,
					Labels: map[string]string{
						kubeprovider.ManagedByLabel:       kubeprovider.ManagedByValue,
						kubeprovider.ProjectIDLabel:       projectID,
						kubeprovider.ProjectVolumeIDLabel: "pvol_other",
					},
				}},
			},
			wantFailureCode: model.ReleaseFailurePreflightFailed,
			wantErrTarget:   kubeprovider.ErrProjectVolumeOwnershipConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			startedAt := time.Now().UTC()
			db := openStubWorkerGormDB(t)
			if err := db.Callback().Query().After("gorm:query").Register("test:deploy_volume_seed", func(tx *gorm.DB) {
				switch dest := tx.Statement.Dest.(type) {
				case *model.Release:
					tx.Error = nil
					*dest = model.Release{
						ID:                 releaseID,
						ProjectID:          projectID,
						ApplicationID:      applicationID,
						DeploymentTargetID: targetID,
						ImageRef:           "registry.example.com/acme/api:v1",
						Status:             "running",
						StartedAt:          &startedAt,
					}
				case *model.Project:
					tx.Error = nil
					*dest = model.Project{
						ID:                  projectID,
						Identifier:          "deploy-vol",
						KubernetesNamespace: namespace,
						DeleteStatus:        "active",
					}
				case *model.Application:
					tx.Error = nil
					*dest = model.Application{
						ID:           applicationID,
						ProjectID:    projectID,
						Identifier:   "api",
						DeleteStatus: "active",
					}
				case *model.DeploymentTarget:
					tx.Error = nil
					*dest = model.DeploymentTarget{
						ID:             targetID,
						ProjectID:      projectID,
						ApplicationID:  applicationID,
						ClusterID:      clusterID,
						Name:           "prod",
						KubernetesName: "api-prod",
						Replicas:       1,
						CPURequest:     "100m",
						MemoryRequest:  "128Mi",
						DeleteStatus:   "active",
						ServicePorts:   model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 8080}}, 8080),
					}
				case *model.RuntimeCluster:
					tx.Error = nil
					*dest = model.RuntimeCluster{
						ID:               clusterID,
						Scope:            "global",
						GatewayNamespace: "luna-gateway",
					}
				}
			}); err != nil {
				t.Fatalf("register deploy query callback: %v", err)
			}

			var releaseFailureCode string
			var releaseStatus string
			if err := db.Callback().Update().Before("gorm:update").Register("test:deploy_volume_capture", func(tx *gorm.DB) {
				if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "releases" {
					return
				}
				updates, ok := tx.Statement.Dest.(map[string]any)
				if !ok {
					return
				}
				if status, hasStatus := updates["status"].(string); hasStatus {
					releaseStatus = status
				}
				if code, hasCode := updates["failure_code"].(string); hasCode {
					releaseFailureCode = code
				}
				tx.RowsAffected = 1
			}); err != nil {
				t.Fatalf("register deploy update callback: %v", err)
			}

			volID := volumeID
			volumeSvc := &volumeWorkerServiceStub{
				listMountsFn: func(context.Context, string, string) ([]model.DeploymentVolumeMount, error) {
					return []model.DeploymentVolumeMount{{
						ID:                 "dvm_1",
						ProjectID:          projectID,
						DeploymentTargetID: targetID,
						SourceType:         model.DeploymentVolumeSourceProjectVolume,
						ProjectVolumeID:    &volID,
						LogicalName:        "data",
						MountPath:          &mountPath,
						ActivationState:    model.DeploymentVolumeActivationReserved,
					}}, nil
				},
				getFn: func(context.Context, string, string) (model.ProjectVolume, error) {
					return model.ProjectVolume{
						ID:               volumeID,
						ProjectID:        projectID,
						ClusterID:        clusterID,
						Namespace:        namespace,
						ClaimName:        claimName,
						OwnershipMode:    tc.ownershipMode,
						LifecycleState:   model.ProjectVolumeLifecycleProvisioning,
						PendingOperation: tc.pendingOperation,
						CapacityRequest:  "5Gi",
					}, nil
				},
			}

			kubeClient := kubeprovider.NewClientForInterface(fake.NewSimpleClientset(tc.objects...))
			runner := &Runner{
				db:                       db,
				volumeService:            volumeSvc,
				kubernetesManagerFactory: func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) { return kubeClient, nil },
			}

			rawPayload, err := json.Marshal(tasks.DeployRunPayload{ReleaseID: releaseID, ProjectID: projectID})
			if err != nil {
				t.Fatal(err)
			}
			runErr := runner.handleDeployRun(context.Background(), asynq.NewTask(tasks.TypeDeployRun, rawPayload))
			if runErr == nil {
				t.Fatal("expected deploy preflight to fail")
			}
			if tc.wantErrTarget != nil && !errors.Is(runErr, tc.wantErrTarget) {
				t.Fatalf("handleDeployRun() error = %v, want errors.Is %v", runErr, tc.wantErrTarget)
			}
			if releaseStatus != "failed" || releaseFailureCode != tc.wantFailureCode {
				t.Fatalf("persisted release terminal state = %q/%q, want failed/%q", releaseStatus, releaseFailureCode, tc.wantFailureCode)
			}
		})
	}
}
