package worker

import (
	"context"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

func TestDeploymentTargetDataVolumesUsesAuthoritativeMountsAndClaim(t *testing.T) {
	volumeID := "volume"
	mountPath := "/data"
	service := &volumeWorkerServiceStub{
		listMountsFn: func(ctx context.Context, projectID, targetID string) ([]model.DeploymentVolumeMount, error) {
			if ctx == nil || projectID != "project" || targetID != "target" {
				t.Fatalf("unexpected mount lookup: project=%q target=%q", projectID, targetID)
			}
			return []model.DeploymentVolumeMount{{
				ID: "mount", ProjectID: projectID, DeploymentTargetID: targetID,
				SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: &volumeID,
				LogicalName: "data", MountPath: &mountPath, ActivationState: model.DeploymentVolumeActivationReserved,
			}}, nil
		},
		getFn: func(ctx context.Context, projectID, volumeID string) (model.ProjectVolume, error) {
			if ctx == nil || projectID != "project" || volumeID != "volume" {
				t.Fatalf("unexpected lookup: project=%q volume=%q", projectID, volumeID)
			}
			return model.ProjectVolume{
				ID: "volume", ProjectID: "project", ClusterID: "cluster", Namespace: "project-ns",
				ClaimName: "authoritative-claim", LifecycleState: model.ProjectVolumeLifecycleProvisioning,
				PendingOperation: volume.OperationProvision, CapacityRequest: "10Gi",
			}, nil
		},
	}
	runner := &Runner{volumeService: service}
	resolved, err := runner.deploymentTargetDataVolumes(context.Background(), model.DeploymentTarget{
		ID: "target", ProjectID: "project", ClusterID: "cluster",
	}, "project-ns")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].ClaimName != "authoritative-claim" {
		t.Fatalf("resolved volumes = %+v", resolved)
	}
}

func TestDeploymentTargetDataVolumesIgnoresLegacyTargetColumns(t *testing.T) {
	service := &volumeWorkerServiceStub{listMountsFn: func(context.Context, string, string) ([]model.DeploymentVolumeMount, error) {
		return nil, nil
	}}
	runner := &Runner{volumeService: service}
	resolved, err := runner.deploymentTargetDataVolumes(context.Background(), model.DeploymentTarget{
		ID: "target", ProjectID: "project", DataRetentionEnabled: true, DataVolumes: `[{"sourceType":"managed","mountPath":"/legacy"}]`,
	}, "project-ns")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Fatalf("legacy deployment target columns must be ignored, got %+v", resolved)
	}
}

func TestDeploymentVolumeAttachmentMatchesProjectVolume(t *testing.T) {
	volumeID := "volume"
	service := &volumeWorkerServiceStub{getFn: func(context.Context, string, string) (model.ProjectVolume, error) {
		return model.ProjectVolume{ID: volumeID, ClaimName: "claim"}, nil
	}}
	mountPath := "/data"
	matches, err := deploymentVolumeAttachmentMatches(context.Background(), service, "project", model.DeploymentVolumeMount{
		SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: &volumeID, MountPath: &mountPath,
	}, kubeprovider.ApplicationVolumeAttachment{ClaimName: "claim", MountPath: "/data"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !matches {
		t.Fatal("authoritative attachment should match the reserved relation")
	}

	matches, err = deploymentVolumeAttachmentMatches(context.Background(), service, "project", model.DeploymentVolumeMount{
		SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: &volumeID, MountPath: &mountPath,
	}, kubeprovider.ApplicationVolumeAttachment{ClaimName: "different", MountPath: "/data"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("a different authoritative claim must not activate the relation")
	}
}
