package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/LiteyukiStudio/devops/internal/volumetransferapi"
	"github.com/gin-gonic/gin"
)

type volumeTransferContentAdapter struct {
	handlers *Handlers
	service  *volumetransferapi.Service
}

func newVolumeTransferContentAdapter(handlers *Handlers, cfg config.Config) (*volumeTransferContentAdapter, error) {
	if handlers == nil || handlers.volumes == nil || handlers.rateLimiter == nil || handlers.rateLimiter.redis == nil {
		return nil, errors.New("volume transfer dependencies are unavailable")
	}
	clusterAdapter, ok := handlers.volumeClusters.(*projectVolumeClusterAdapter)
	if !ok || clusterAdapter == nil {
		return nil, errors.New("volume transfer runtime streaming is unavailable")
	}
	runtime := &volumeTransferRuntimeAdapter{clusters: clusterAdapter}
	return &volumeTransferContentAdapter{
		handlers: handlers,
		service: volumetransferapi.NewService(handlers.volumes, runtime,
			volumetransferapi.NewRedisTicketStore(handlers.rateLimiter.redis),
			volumetransferapi.Options{MaxBytes: cfg.VolumeTransferMaxBytes}),
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
	return adapter.service.StreamImport(ctx, projectID, transferID, adapter.actor(ctx, user, projectID), body, length)
}

func (adapter *volumeTransferContentAdapter) CreateExport(ctx context.Context, user model.User, project model.Project, volumeID string, input volumeExportCreateInput, idempotencyKey string) (model.VolumeTransfer, error) {
	return adapter.service.CreateExport(ctx, volumetransferapi.ExportRequest{
		ProjectID: project.ID, VolumeID: volumeID, Format: input.Format, Consistency: input.Consistency,
		ActorID: user.ID, IdempotencyKey: idempotencyKey,
	})
}

func (adapter *volumeTransferContentAdapter) RetryTransfer(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, idempotencyKey string) (model.VolumeTransfer, error) {
	return adapter.service.RetryTransfer(ctx, adapter.actor(ctx, user, project.ID), transfer, idempotencyKey)
}

func (adapter *volumeTransferContentAdapter) AuthorizeDownload(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, binding volumeDownloadBinding) (volumeDownloadAuthorizationResponse, error) {
	result, err := adapter.service.AuthorizeDownload(ctx, adapter.actor(ctx, user, project.ID), transfer, coreDownloadBinding(binding))
	return volumeDownloadAuthorizationResponse{Ticket: result.Ticket, ExpiresAt: result.ExpiresAt}, err
}

func (adapter *volumeTransferContentAdapter) OpenDownload(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, ticket string, binding volumeDownloadBinding) (volumeDownload, error) {
	result, err := adapter.service.OpenDownload(ctx, adapter.actor(ctx, user, project.ID), transfer, ticket, coreDownloadBinding(binding))
	return volumeDownload{Body: result.Body, ContentType: result.ContentType}, err
}

func (adapter *volumeTransferContentAdapter) OpenManifest(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, ticket string, binding volumeDownloadBinding) (volumeDownload, error) {
	result, err := adapter.service.OpenManifest(ctx, adapter.actor(ctx, user, project.ID), transfer, ticket, coreDownloadBinding(binding))
	return volumeDownload{Body: result.Body, ContentType: result.ContentType}, err
}

func (adapter *volumeTransferContentAdapter) actor(ctx context.Context, user model.User, projectID string) volumetransferapi.Actor {
	actor := volumetransferapi.Actor{UserID: user.ID, CanManage: authz.IsPlatformAdmin(user.Role)}
	if actor.CanManage || adapter == nil || adapter.handlers == nil || adapter.handlers.db == nil {
		return actor
	}
	var member model.ProjectMember
	err := adapter.handlers.db.WithContext(ctx).Select("role").First(&member, "project_id = ? and user_id = ?", projectID, user.ID).Error
	actor.CanManage = err == nil && (member.Role == authz.ProjectRoleOwner || member.Role == authz.ProjectRoleAdmin)
	return actor
}

func coreDownloadBinding(binding volumeDownloadBinding) volumetransferapi.DownloadBinding {
	return volumetransferapi.DownloadBinding{UserID: binding.UserID, SubjectID: binding.SubjectID, Deadline: binding.Deadline}
}

func (h *Handlers) volumeTransferDownloadBinding(ctx *gin.Context, user model.User) (volumeDownloadBinding, bool) {
	subject, ok := h.currentInteractiveSubject(ctx, user)
	if !ok {
		writeErrorCode(ctx, http.StatusForbidden, "volume.download_session_required", "a browser session or Luna CLI OAuth grant is required for volume downloads")
		return volumeDownloadBinding{}, false
	}
	binding := volumeDownloadBinding{UserID: user.ID, SubjectID: subject}
	if strings.HasPrefix(subject, "oauth:") {
		token, tokenOK := currentAccessTokenFromContext(ctx)
		if !tokenOK || token.UserID != user.ID || token.OAuthGrantID == "" {
			writeErrorCode(ctx, http.StatusForbidden, "volume.download_session_required", "Luna CLI OAuth grant is invalid")
			return volumeDownloadBinding{}, false
		}
		binding.Deadline = time.Now().Add(time.Hour)
		if token.ExpiresAt != nil && token.ExpiresAt.Before(binding.Deadline) {
			binding.Deadline = *token.ExpiresAt
		}
	} else {
		session, sessionOK := h.currentSessionFromCookie(ctx)
		if !sessionOK || session.UserID != user.ID || session.ID != subject {
			writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
			return volumeDownloadBinding{}, false
		}
		binding.Deadline = session.ExpiresAt
	}
	return binding, true
}

var _ volumeTransferContentService = (*volumeTransferContentAdapter)(nil)
