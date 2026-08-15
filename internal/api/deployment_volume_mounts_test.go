package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

func TestAppTemplateDataVolumesBindExplicitSelectedProjectVolume(t *testing.T) {
	template := appstore.Template{ID: "postgresql", DataVolumes: []appstore.DataVolume{{
		LogicalName: "data", SourceType: "projectVolume", MountPath: "/var/lib/postgresql/data",
	}}}
	selected := model.ProjectVolume{ID: "pvol_database"}
	inputs := appTemplateDeploymentDataVolumes(template, &selected)
	ctx, recorder := volumeTestContext(http.MethodPost, "/api/v1/projects/prj_1/app-templates/postgresql/install")
	normalized, ok := normalizeDataVolumes(ctx, inputs)
	if !ok || recorder.Code != http.StatusOK {
		t.Fatalf("typed template volume rejected: ok=%t status=%d body=%s", ok, recorder.Code, recorder.Body.String())
	}
	if len(normalized) != 1 || normalized[0].SourceType != "projectVolume" || normalized[0].ProjectVolumeID != selected.ID || normalized[0].MountPath != "/var/lib/postgresql/data" {
		t.Fatalf("template mount = %#v", normalized)
	}

	ctx, recorder = volumeTestContext(http.MethodPost, "/api/v1/projects/prj_1/app-templates/postgresql/install")
	if _, ok := normalizeDataVolumes(ctx, appTemplateDeploymentDataVolumes(template, nil)); ok || recorder.Code != http.StatusBadRequest {
		t.Fatalf("template mount without an explicit projectVolumeId was accepted: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNormalizeDeploymentDataVolumesAcceptsOnlyTypedSources(t *testing.T) {
	ctx, recorder := volumeTestContext(http.MethodPost, "/api/v1/projects/project/applications/app/deployment-targets")
	items, ok := normalizeDataVolumes(ctx, []deploymentTargetDataVolumeInput{
		{LogicalName: "data", SourceType: "projectVolume", ProjectVolumeID: "pvol_data", MountPath: "/data"},
		{LogicalName: "cache", SourceType: "emptyDir", MountPath: "/cache", EmptyDir: &deploymentTargetEmptyDirInput{Medium: "Memory", SizeLimit: "512Mi"}},
	})
	if !ok || recorder.Code != http.StatusOK || len(items) != 2 {
		t.Fatalf("typed volumes = %#v, ok=%v status=%d body=%s", items, ok, recorder.Code, recorder.Body.String())
	}

	for _, sourceType := range []string{"managed", "existingClaim", "retainedClaim", ""} {
		ctx, recorder = volumeTestContext(http.MethodPost, "/api/v1/projects/project/applications/app/deployment-targets")
		if _, accepted := normalizeDataVolumes(ctx, []deploymentTargetDataVolumeInput{{LogicalName: "data", SourceType: sourceType, MountPath: "/data"}}); accepted {
			t.Fatalf("legacy source type %q was accepted", sourceType)
		}
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), volume.CodeInvalidInput) {
			t.Fatalf("legacy source %q response = %d %s", sourceType, recorder.Code, recorder.Body.String())
		}
	}
}

func TestNormalizeDeploymentDataVolumesCapsDesiredSet(t *testing.T) {
	inputs := make([]deploymentTargetDataVolumeInput, maxDeploymentDataVolumes+1)
	ctx, recorder := volumeTestContext(http.MethodPost, "/api/v1/projects/project/applications/app/deployment-targets")
	if _, ok := normalizeDataVolumes(ctx, inputs); ok {
		t.Fatal("oversized deployment volume set was accepted")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), volume.CodeInvalidInput) {
		t.Fatalf("oversized response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDeploymentVolumeAuditRecordsContainOnlyStableIdentifiers(t *testing.T) {
	volumeID := "pvol_data"
	changes := deploymentVolumeMountChanges{
		Bound:   []model.DeploymentVolumeMount{{ID: "dvmt_bound", ProjectVolumeID: &volumeID, MountPath: deploymentStringPointer("/secret/data")}},
		Unbound: []model.DeploymentVolumeMount{{ID: "dvmt_unbound", DevicePath: deploymentStringPointer("/dev/private")}},
	}
	records := deploymentVolumeAuditRecords(model.DeploymentTarget{ID: "dplt_target"}, changes)
	if len(records) != 2 || records[0].Action != "deployment_volume.bind" || records[1].Action != "deployment_volume.unbind" {
		t.Fatalf("audit records = %#v", records)
	}
	if records[0].Resource != "dvmt_bound" || records[0].Message != "dplt_target:pvol_data" || records[1].Resource != "dvmt_unbound" || records[1].Message != "dplt_target" {
		t.Fatalf("audit identifiers = %#v", records)
	}
	joined := records[0].Message + records[1].Message
	if strings.Contains(joined, "/secret/data") || strings.Contains(joined, "/dev/private") {
		t.Fatalf("audit leaked paths: %#v", records)
	}
}

func TestDeploymentVolumeFailureAuditRecordsUseStableCodesAndIdentifiers(t *testing.T) {
	t.Parallel()
	changes := deploymentVolumeMountChanges{Attempted: []deploymentVolumeAuditRecord{
		{Action: "deployment_volume.bind", Resource: "pvol_data", Message: "dplt_target:pvol_data"},
		{Action: "deployment_volume.unbind", Resource: "dvmt_old", Message: "dplt_target"},
	}}
	records := deploymentVolumeFailureAuditRecords(changes, &volume.DomainError{
		Code:    volume.CodeBindingConflict,
		Message: "provider response containing /private/path and secret material",
	})
	if len(records) != 2 || records[0].Action != "deployment_volume.bind" || records[0].Resource != "pvol_data" || records[1].Action != "deployment_volume.unbind" || records[1].Resource != "dvmt_old" {
		t.Fatalf("failure records = %#v", records)
	}
	for _, record := range records {
		if record.Message != volume.CodeBindingConflict || strings.Contains(record.Message, "/private/path") || strings.Contains(record.Message, "secret") {
			t.Fatalf("failure audit leaked dependency detail: %#v", records)
		}
	}
}

func TestDeploymentVolumeFailureAuditRecordsFailClosedForUnknownErrors(t *testing.T) {
	t.Parallel()
	records := deploymentVolumeFailureAuditRecords(deploymentVolumeMountChanges{Attempted: []deploymentVolumeAuditRecord{{
		Action: "deployment_volume.bind", Resource: "dplt_target",
	}}}, context.Canceled)
	if len(records) != 1 || records[0].Message != "internal_error" {
		t.Fatalf("failure records = %#v", records)
	}
}

func deploymentStringPointer(value string) *string { return &value }
