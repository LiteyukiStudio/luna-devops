package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/LiteyukiStudio/devops/internal/volumetransferapi"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
)

func TestVolumePaginationUsesSafeDefaultsAndMaximum(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantPage int
		wantSize int
	}{
		{name: "omitted", wantPage: 1, wantSize: 20},
		{name: "invalid", query: "?page=0&pageSize=nope", wantPage: 1, wantSize: 20},
		{name: "capped", query: "?page=2&pageSize=1000", wantPage: 2, wantSize: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := volumeTestContext(http.MethodGet, "/api/v1/projects/prj_1/volumes"+test.query)
			pagination, ok := volumePagination(ctx, map[string]bool{"createdAt": true}, "createdAt")
			if !ok || recorder.Code != http.StatusOK {
				t.Fatalf("volumePagination() ok = %v, status = %d", ok, recorder.Code)
			}
			if pagination.Page != test.wantPage || pagination.PageSize != test.wantSize || pagination.SortBy != "createdAt" || pagination.SortOrder != "desc" {
				t.Fatalf("pagination = %#v", pagination)
			}
		})
	}
}

func TestVolumePaginationRejectsUnknownSortParameters(t *testing.T) {
	for _, query := range []string{"?sortBy=deletedAt", "?sortOrder=sideways"} {
		ctx, recorder := volumeTestContext(http.MethodGet, "/api/v1/projects/prj_1/volumes"+query)
		if _, ok := volumePagination(ctx, map[string]bool{"createdAt": true}, "createdAt"); ok {
			t.Fatalf("volumePagination(%q) unexpectedly succeeded", query)
		}
		if recorder.Code != http.StatusBadRequest || (!strings.Contains(recorder.Body.String(), volume.CodePaginationSortByInvalid) && !strings.Contains(recorder.Body.String(), volume.CodePaginationOrderInvalid)) {
			t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestProjectVolumeExistingClaimIgnoresClientControlledSpecification(t *testing.T) {
	ctx, recorder := volumeTestContext(http.MethodPost, "/api/v1/projects/prj_1/volumes")
	input, ok := projectVolumeCreateDomainInput(ctx, model.Project{ID: "prj_1", KubernetesNamespace: "luna-prj-1"}, model.User{ID: "usr_1"}, projectVolumeCreateInput{
		DisplayName: "database", ClusterID: "cluster_1", Capacity: "999Ti", StorageClassName: "attacker-class",
		AccessMode: model.ProjectVolumeAccessReadWriteMany, VolumeMode: model.ProjectVolumeModeBlock,
		Source: projectVolumeSourceInput{Type: "existingClaim", ClaimName: "database", OwnershipMode: model.ProjectVolumeOwnershipReferenced},
	}, "idem-existing-claim")
	if !ok || recorder.Code != http.StatusOK {
		t.Fatalf("projectVolumeCreateDomainInput() ok = %v, status = %d", ok, recorder.Code)
	}
	if input.CapacityRequest != "" || input.CapacityBytes != 0 || input.StorageClassName != "" || input.AccessMode != "" || input.VolumeMode != "" {
		t.Fatalf("existing claim retained client-controlled specification: %#v", input)
	}
}

func TestProjectVolumeSnapshotSourcePreservesReference(t *testing.T) {
	ctx, recorder := volumeTestContext(http.MethodPost, "/api/v1/projects/prj_1/volumes")
	input, ok := projectVolumeCreateDomainInput(ctx, model.Project{ID: "prj_1", KubernetesNamespace: "luna-prj-1"}, model.User{ID: "usr_1"}, projectVolumeCreateInput{
		DisplayName: "restored", ClusterID: "cluster_1", Capacity: "20Gi", StorageClassName: "standard",
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		Source: projectVolumeSourceInput{Type: "volumeSnapshot", SnapshotName: "snapshot-1"},
	}, "idem-snapshot-restore")
	if !ok || recorder.Code != http.StatusOK {
		t.Fatalf("projectVolumeCreateDomainInput() ok = %v, status = %d body = %s", ok, recorder.Code, recorder.Body.String())
	}
	if input.SourceKind != model.ProjectVolumeSourceSnapshotRestore || input.SourceSnapshotName != "snapshot-1" {
		t.Fatalf("snapshot source = %#v", input)
	}
}

func TestVolumeOperationDispatcherPreservesContextAndOperation(t *testing.T) {
	stub := &volumeTaskEnqueuerStub{}
	dispatcher := volumeOperationDispatcher{tasks: stub}
	ctx := context.WithValue(context.Background(), volumeDispatcherContextKey{}, "trace-parent")
	if err := dispatcher.DispatchVolumeOperation(ctx, volume.VolumeOperation{
		Kind: volume.OperationExpand, ProjectID: "prj_1", VolumeID: "pvol_1", ActorID: "usr_1",
	}); err != nil {
		t.Fatalf("DispatchVolumeOperation() error = %v", err)
	}
	if stub.provision.Operation != volume.OperationExpand || stub.provision.ProjectID != "prj_1" || stub.provision.VolumeID != "pvol_1" || stub.contextValue != "trace-parent" {
		t.Fatalf("dispatched payload = %#v, context = %q", stub.provision, stub.contextValue)
	}
}

func TestProjectVolumeRetryAuthorizationPreservesOriginalRiskBoundary(t *testing.T) {
	tests := []struct {
		name       string
		item       model.ProjectVolume
		wantAction authz.Action
		wantOK     bool
	}{
		{name: "provision", item: model.ProjectVolume{PendingOperation: volume.OperationProvision}, wantAction: authz.ActionVolumeWrite, wantOK: true},
		{name: "adopt", item: model.ProjectVolume{PendingOperation: volume.OperationProvision, SourceKind: model.ProjectVolumeSourceExistingClaim, OwnershipMode: model.ProjectVolumeOwnershipManaged}, wantAction: authz.ActionVolumeWrite, wantOK: true},
		{name: "expand", item: model.ProjectVolume{PendingOperation: volume.OperationExpand}, wantAction: authz.ActionVolumeWrite, wantOK: true},
		{name: "delete", item: model.ProjectVolume{PendingOperation: volume.OperationDelete}, wantAction: authz.ActionVolumeDelete, wantOK: true},
		{name: "import uses transfer retry", item: model.ProjectVolume{PendingOperation: volume.OperationImport}, wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, ok := projectVolumeRetryAuthorization(test.item)
			if action != test.wantAction || ok != test.wantOK {
				t.Fatalf("authorization = (%q, %t), want (%q, %t)", action, ok, test.wantAction, test.wantOK)
			}
		})
	}
}

func TestVolumeResponsesDoNotExposeInternalDiagnosticsOrObjectKeys(t *testing.T) {
	volumePayload, err := json.Marshal(projectVolumeResponseFor(model.ProjectVolume{
		ID: "pvol_1", LastErrorCode: "volume.cluster_unavailable", LastErrorMessage: "token=secret internal-provider.local",
	}))
	if err != nil {
		t.Fatal(err)
	}
	transferPayload, err := json.Marshal(volumeTransferResponseFor(model.VolumeTransfer{
		ID: "vtx_1", ObjectKey: "private/project/archive.tar.gz", LastErrorMessage: "s3 secret failure",
	}, true))
	if err != nil {
		t.Fatal(err)
	}
	combined := string(volumePayload) + string(transferPayload)
	for _, secret := range []string{"token=secret", "internal-provider.local", "private/project/archive.tar.gz", "s3 secret failure"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("safe API response contains %q: %s", secret, combined)
		}
	}
}

func TestProjectVolumeResponseKeepsFirstConsumerProvisioningAvailable(t *testing.T) {
	response := projectVolumeResponseFor(model.ProjectVolume{
		ID: "pvol_first_consumer", LifecycleState: model.ProjectVolumeLifecycleProvisioning,
		PendingOperation: volume.OperationProvision, Availability: model.ProjectVolumeAvailabilityAvailable,
	})
	if response.Availability != model.ProjectVolumeAvailabilityAvailable {
		t.Fatalf("availability=%q, want %q", response.Availability, model.ProjectVolumeAvailabilityAvailable)
	}
}

func TestVolumeTransferResponsePublishesRequiredFiveTiBChunkSize(t *testing.T) {
	const fiveTiB = int64(5 * 1024 * 1024 * 1024 * 1024)
	response := volumeTransferResponseFor(model.VolumeTransfer{
		ID:            "vtx_large",
		Direction:     model.VolumeTransferDirectionImport,
		ExpectedBytes: fiveTiB,
	}, true)
	if response.ChunkSize != 525*1024*1024 {
		t.Fatalf("chunkSize = %d, want %d", response.ChunkSize, int64(525*1024*1024))
	}
	parts := (fiveTiB + response.ChunkSize - 1) / response.ChunkSize
	if parts > volumetransferapi.MaxMultipartParts {
		t.Fatalf("5 TiB needs %d parts, max = %d", parts, volumetransferapi.MaxMultipartParts)
	}
}

func TestWriteVolumeErrorUsesStableStatusAndHidesUnknownProductionDetail(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	tests := []struct {
		code   string
		status int
	}{
		{code: volume.CodeClusterUnavailable, status: http.StatusServiceUnavailable},
		{code: volume.CodeClaimNotFound, status: http.StatusNotFound},
		{code: volume.CodeTransferOffsetMismatch, status: http.StatusConflict},
		{code: volume.CodeTransferChecksumMismatch, status: http.StatusUnprocessableEntity},
		{code: volume.CodeTransferExpired, status: http.StatusGone},
		{code: volume.CodeTransferCallbackUnauthorized, status: http.StatusUnauthorized},
		{code: volume.CodeTransferDownloadUnauthorized, status: http.StatusUnauthorized},
		{code: volume.CodeTransferSpoolBusy, status: http.StatusTooManyRequests},
		{code: volume.CodeTransferSpoolUnavailable, status: http.StatusServiceUnavailable},
		{code: volume.CodeTransferSpoolInsufficient, status: http.StatusInsufficientStorage},
		{code: volume.CodeTransferPartInProgress, status: http.StatusConflict},
	}
	for _, test := range tests {
		ctx, recorder := volumeTestContext(http.MethodGet, "/api/v1/projects/prj_1/volumes")
		writeVolumeError(ctx, &volume.DomainError{Code: test.code, Message: "public volume error"})
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.code) {
			t.Fatalf("%s response = %d %s", test.code, recorder.Code, recorder.Body.String())
		}
		if test.code == volume.CodeTransferPartInProgress && recorder.Header().Get("Retry-After") != "1" {
			t.Fatalf("%s Retry-After = %q", test.code, recorder.Header().Get("Retry-After"))
		}
	}

	ctx, recorder := volumeTestContext(http.MethodGet, "/api/v1/projects/prj_1/volumes")
	writeVolumeError(ctx, context.Canceled)
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), `"code":"internal_error"`) || strings.Contains(recorder.Body.String(), context.Canceled.Error()) {
		t.Fatalf("unknown production error response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestVolumeAuditErrorCodeNeverPersistsRawDependencyErrors(t *testing.T) {
	t.Parallel()
	if got := volumeAuditErrorCode(&volume.DomainError{Code: volume.CodeClusterUnavailable, Message: "provider detail"}); got != volume.CodeClusterUnavailable {
		t.Fatalf("domain audit code = %q", got)
	}
	if got := volumeAuditErrorCode(context.Canceled); got != "internal_error" {
		t.Fatalf("unknown audit code = %q", got)
	}
}

func volumeTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	return ctx, recorder
}

type volumeDispatcherContextKey struct{}

type volumeTaskEnqueuerStub struct {
	provision    tasks.VolumeProvisionPayload
	contextValue string
}

func (stub *volumeTaskEnqueuerStub) EnqueueVolumeProvision(ctx context.Context, payload tasks.VolumeProvisionPayload) (*asynq.TaskInfo, error) {
	stub.provision = payload
	stub.contextValue, _ = ctx.Value(volumeDispatcherContextKey{}).(string)
	return &asynq.TaskInfo{}, nil
}

func (*volumeTaskEnqueuerStub) EnqueueVolumeImport(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{}, nil
}

func (*volumeTaskEnqueuerStub) EnqueueVolumeExport(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{}, nil
}

func (*volumeTaskEnqueuerStub) EnqueueVolumeDelete(context.Context, tasks.VolumeDeletePayload) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{}, nil
}
