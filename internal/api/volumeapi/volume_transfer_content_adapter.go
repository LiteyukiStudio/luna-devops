package volumeapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/LiteyukiStudio/devops/internal/volumetransferapi"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type volumeTransferContentAdapter struct {
	handlers *Handlers
	service  *volumetransferapi.Service
}

func NewVolumeTransferContentAdapter(host Host, volumes *volume.Service, clusterAdapter *ProjectVolumeClusterAdapter, redisClient *redis.Client, maxBytes int64) (*VolumeTransferContentAdapter, error) {
	if host == nil || volumes == nil || clusterAdapter == nil || redisClient == nil {
		return nil, errors.New("volume transfer dependencies are unavailable")
	}
	handlers := New(host, Dependencies{Volumes: volumes, Clusters: clusterAdapter, TransferMaxBytes: maxBytes, TransferEnabled: true})
	runtime := &volumeTransferRuntimeAdapter{clusters: clusterAdapter}
	return &volumeTransferContentAdapter{
		handlers: handlers,
		service: volumetransferapi.NewService(volumes, runtime,
			volumetransferapi.NewRedisTicketStore(redisClient),
			volumetransferapi.Options{MaxBytes: maxBytes}),
	}, nil
}

type volumeTransferRuntimeAdapter struct {
	clusters *projectVolumeClusterAdapter
}

func (adapter *volumeTransferRuntimeAdapter) OpenVolumeTransferImport(ctx context.Context, projectVolume model.ProjectVolume, transfer model.VolumeTransfer, source io.Reader) (volumetransferapi.StreamResult, error) {
	client, err := adapter.clusters.clientForCluster(ctx, projectVolume.ClusterID)
	if err != nil {
		return volumetransferapi.StreamResult{}, err
	}
	result, err := client.OpenVolumeTransferImport(ctx, kubeprovider.VolumeTransferStreamTarget{
		Namespace: projectVolume.Namespace, TransferID: transfer.ID,
		ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
	}, source)
	return apiVolumeTransferStreamResult(result), err
}

func (adapter *volumeTransferRuntimeAdapter) OpenVolumeTransferExport(ctx context.Context, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) (volumetransferapi.ExportStream, error) {
	client, err := adapter.clusters.clientForCluster(ctx, projectVolume.ClusterID)
	if err != nil {
		return nil, err
	}
	stream, err := client.OpenVolumeTransferExport(ctx, kubeprovider.VolumeTransferStreamTarget{
		Namespace: projectVolume.Namespace, TransferID: transfer.ID,
		ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
	})
	if err != nil {
		return nil, err
	}
	return &apiVolumeTransferExportStream{stream: stream}, nil
}

type apiVolumeTransferExportStream struct {
	stream *kubeprovider.VolumeTransferExportStream
}

func (stream *apiVolumeTransferExportStream) Read(buffer []byte) (int, error) {
	return stream.stream.Reader.Read(buffer)
}

func (stream *apiVolumeTransferExportStream) Close() error {
	return stream.stream.Close()
}

func (stream *apiVolumeTransferExportStream) Wait() (volumetransferapi.StreamResult, error) {
	result, err := stream.stream.Wait()
	return apiVolumeTransferStreamResult(result), err
}

func apiVolumeTransferStreamResult(result kubeprovider.VolumeTransferStreamResult) volumetransferapi.StreamResult {
	return volumetransferapi.StreamResult{
		TransferredBytes: result.TransferredBytes, ProcessedFiles: result.ProcessedFiles,
		SHA256: result.SHA256, LogicalBytes: result.LogicalBytes, DataSHA256: result.DataSHA256,
	}
}

func (adapter *volumeTransferContentAdapter) CreateImport(ctx context.Context, user model.User, project model.Project, input volumeImportCreateInput, idempotencyKey string) (model.ProjectVolume, model.VolumeTransfer, error) {
	capacity, capacityBytes, valid := parseVolumeCapacity(input.Capacity)
	if !valid {
		return model.ProjectVolume{}, model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeInvalidInput, Message: "volume import capacity is invalid"}
	}
	result, err := adapter.service.CreateImport(ctx, volumetransferapi.ImportRequest{
		ProjectID: project.ID, Namespace: runtimeProjectNamespace(project), DisplayName: input.DisplayName,
		ClusterID: input.ClusterID, CapacityRequest: capacity, CapacityBytes: capacityBytes,
		StorageClassName: input.StorageClassName, AccessMode: input.AccessMode, VolumeMode: input.VolumeMode,
		Format: input.Format, Filename: input.Filename, ContentLength: input.ContentLength,
		ActorID: user.ID, IdempotencyKey: idempotencyKey,
	})
	return result.Volume, result.Transfer, err
}

func (adapter *volumeTransferContentAdapter) StreamImport(ctx context.Context, projectID, transferID string, user model.User, body io.Reader, length int64) (model.VolumeTransfer, error) {
	actor, err := adapter.actor(ctx, user, projectID)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	return adapter.service.StreamImport(ctx, projectID, transferID, actor, body, length)
}

func (adapter *volumeTransferContentAdapter) CreateExport(ctx context.Context, user model.User, project model.Project, volumeID string, input volumeExportCreateInput, idempotencyKey string) (model.VolumeTransfer, error) {
	return adapter.service.CreateExport(ctx, volumetransferapi.ExportRequest{
		ProjectID: project.ID, VolumeID: volumeID, Format: input.Format, Consistency: input.Consistency,
		ActorID: user.ID, IdempotencyKey: idempotencyKey,
	})
}

func (adapter *volumeTransferContentAdapter) RetryTransfer(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, idempotencyKey string) (model.VolumeTransfer, error) {
	actor, err := adapter.actor(ctx, user, project.ID)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	return adapter.service.RetryTransfer(ctx, actor, transfer, idempotencyKey)
}

func (adapter *volumeTransferContentAdapter) AuthorizeDownload(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, binding volumeDownloadBinding) (volumeDownloadAuthorizationResponse, error) {
	actor, err := adapter.actor(ctx, user, project.ID)
	if err != nil {
		return volumeDownloadAuthorizationResponse{}, err
	}
	result, err := adapter.service.AuthorizeDownload(ctx, actor, transfer, coreDownloadBinding(binding))
	return volumeDownloadAuthorizationResponse{Ticket: result.Ticket, ExpiresAt: result.ExpiresAt}, err
}

func (adapter *volumeTransferContentAdapter) OpenDownload(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, ticket string, binding volumeDownloadBinding) (volumeDownload, error) {
	actor, err := adapter.actor(ctx, user, project.ID)
	if err != nil {
		return volumeDownload{}, err
	}
	result, err := adapter.service.OpenDownload(ctx, actor, transfer, ticket, coreDownloadBinding(binding))
	return volumeDownload{Body: result.Body, ContentType: result.ContentType}, err
}

func (adapter *volumeTransferContentAdapter) OpenManifest(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, ticket string, binding volumeDownloadBinding) (volumeDownload, error) {
	actor, err := adapter.actor(ctx, user, project.ID)
	if err != nil {
		return volumeDownload{}, err
	}
	result, err := adapter.service.OpenManifest(ctx, actor, transfer, ticket, coreDownloadBinding(binding))
	return volumeDownload{Body: result.Body, ContentType: result.ContentType}, err
}

func (adapter *volumeTransferContentAdapter) actor(ctx context.Context, user model.User, projectID string) (volumetransferapi.Actor, error) {
	if adapter == nil || adapter.handlers == nil {
		return volumetransferapi.Actor{}, authz.ErrProjectAuthorizationUnavailable
	}
	canManage, err := adapter.handlers.projectRoleActionAllowed(ctx, user, projectID, authz.ActionVolumeExport)
	if err != nil {
		return volumetransferapi.Actor{}, err
	}
	return volumetransferapi.Actor{UserID: user.ID, CanManage: canManage}, nil
}

func coreDownloadBinding(binding volumeDownloadBinding) volumetransferapi.DownloadBinding {
	return volumetransferapi.DownloadBinding{UserID: binding.UserID, SubjectID: binding.SubjectID, Deadline: binding.Deadline}
}

func (h *Handlers) volumeTransferDownloadBinding(ctx *gin.Context, user model.User) (volumeDownloadBinding, bool) {
	binding, ok := h.currentInteractiveAuthorizationBinding(ctx, user)
	if !ok {
		if h.requestUsesBearerToken(ctx) {
			writeErrorCode(ctx, http.StatusForbidden, "volume.download_session_required", "a browser session or Luna CLI OAuth grant is required for volume downloads")
		} else {
			writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		}
		return volumeDownloadBinding{}, false
	}
	return binding, true
}

var _ volumeTransferContentService = (*volumeTransferContentAdapter)(nil)
