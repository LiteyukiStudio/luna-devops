package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/provider/volumestore"
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
	store, err := volumestore.NewS3Store(volumestore.S3Config{
		Endpoint: cfg.VolumeTransferS3Endpoint, Region: cfg.VolumeTransferS3Region,
		Bucket: cfg.VolumeTransferS3Bucket, AccessKeyID: cfg.VolumeTransferS3AccessKeyID,
		SecretAccessKey: cfg.VolumeTransferS3SecretKey, PathStyle: cfg.VolumeTransferS3PathStyle,
		AllowInsecureEndpoint: handlers.mode == "development",
	})
	if err != nil {
		return nil, fmt.Errorf("initialize volume transfer store: %w", err)
	}
	tickets := volumetransferapi.NewRedisTicketStore(handlers.rateLimiter.redis)
	return &volumeTransferContentAdapter{
		handlers: handlers,
		service: volumetransferapi.NewService(handlers.volumes, store, tickets, volumetransferapi.Options{
			ObjectTTL:         cfg.VolumeTransferObjectTTL,
			MaxBytes:          cfg.VolumeTransferMaxBytes,
			TempDir:           cfg.VolumeTransferSpoolDir,
			SpoolMaxBytes:     cfg.VolumeTransferSpoolMaxBytes,
			SpoolMinFreeBytes: cfg.VolumeTransferSpoolMinFreeBytes,
			SpoolOrphanAge:    cfg.VolumeTransferSpoolOrphanAge,
		}),
	}, nil
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
		Format: input.Format, Filename: input.Filename, ContentLength: input.ContentLength, SHA256: input.SHA256,
		ActorID: user.ID, IdempotencyKey: idempotencyKey,
	})
	return result.Volume, result.Transfer, err
}

func (adapter *volumeTransferContentAdapter) HeadImport(ctx context.Context, projectID, transferID string, user model.User) (int64, int64, int64, error) {
	return adapter.service.HeadImport(ctx, projectID, transferID, adapter.actor(ctx, user, projectID))
}

func (adapter *volumeTransferContentAdapter) WriteImportPart(ctx context.Context, projectID, transferID string, user model.User, offset int64, checksum string, body io.Reader, size int64) (int64, int64, error) {
	return adapter.service.WriteImportPart(ctx, projectID, transferID, adapter.actor(ctx, user, projectID), offset, checksum, body, size)
}

func (adapter *volumeTransferContentAdapter) CompleteImport(ctx context.Context, projectID, transferID string, user model.User, length int64, checksum string) (model.VolumeTransfer, error) {
	return adapter.service.CompleteImport(ctx, projectID, transferID, adapter.actor(ctx, user, projectID), length, checksum)
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

func (adapter *volumeTransferContentAdapter) HeadDownload(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, credential volumeDownloadCredential, binding volumeDownloadBinding) (volumeDownloadInfo, volumeDownloadSession, error) {
	result, session, err := adapter.service.HeadDownload(ctx, adapter.actor(ctx, user, project.ID), transfer, coreDownloadCredential(credential), coreDownloadBinding(binding))
	return volumeDownloadInfo{Size: result.Size, ETag: result.ETag}, apiDownloadSession(session), err
}

func (adapter *volumeTransferContentAdapter) OpenDownload(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, credential volumeDownloadCredential, rangeHeader string, binding volumeDownloadBinding) (volumeDownload, volumeDownloadSession, error) {
	result, session, err := adapter.service.OpenDownload(ctx, adapter.actor(ctx, user, project.ID), transfer, coreDownloadCredential(credential), rangeHeader, coreDownloadBinding(binding))
	return apiVolumeDownload(result), apiDownloadSession(session), err
}

func (adapter *volumeTransferContentAdapter) HeadManifest(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, credential volumeDownloadCredential, binding volumeDownloadBinding) (volumeDownloadInfo, volumeDownloadSession, error) {
	result, session, err := adapter.service.HeadManifest(ctx, adapter.actor(ctx, user, project.ID), transfer, coreDownloadCredential(credential), coreDownloadBinding(binding))
	return volumeDownloadInfo{Size: result.Size, ETag: result.ETag}, apiDownloadSession(session), err
}

func (adapter *volumeTransferContentAdapter) OpenManifest(ctx context.Context, user model.User, project model.Project, transfer model.VolumeTransfer, credential volumeDownloadCredential, binding volumeDownloadBinding) (volumeDownload, volumeDownloadSession, error) {
	result, session, err := adapter.service.OpenManifest(ctx, adapter.actor(ctx, user, project.ID), transfer, coreDownloadCredential(credential), coreDownloadBinding(binding))
	return apiVolumeDownload(result), apiDownloadSession(session), err
}

func (adapter *volumeTransferContentAdapter) InternalHead(ctx context.Context, transferID, token string) (volumeInternalContentInfo, error) {
	result, err := adapter.service.InternalHead(ctx, transferID, token)
	return volumeInternalContentInfo{Offset: result.Offset, Size: result.Size, ChunkSize: result.ChunkSize, ETag: result.ETag}, err
}

func (adapter *volumeTransferContentAdapter) InternalWritePart(ctx context.Context, transferID, token string, offset int64, checksum string, body io.Reader, size int64) (int64, int64, error) {
	return adapter.service.InternalWritePart(ctx, transferID, token, offset, checksum, body, size)
}

func (adapter *volumeTransferContentAdapter) InternalOpen(ctx context.Context, transferID, token, rangeHeader string) (volumeDownload, error) {
	result, err := adapter.service.InternalOpen(ctx, transferID, token, rangeHeader)
	return apiVolumeDownload(result), err
}

func (adapter *volumeTransferContentAdapter) InternalProgress(ctx context.Context, transferID, token string, input volumeTransferProgressInput) error {
	return adapter.service.InternalProgress(ctx, transferID, token, volumetransferapi.Progress{
		ExpectedState: input.ExpectedState, TransferredBytes: input.TransferredBytes,
		ProcessedFiles: input.ProcessedFiles, Stage: input.Stage,
	})
}

func (adapter *volumeTransferContentAdapter) InternalComplete(ctx context.Context, transferID, token string, input volumeTransferCompleteInput) (model.VolumeTransfer, error) {
	return adapter.service.InternalComplete(ctx, transferID, token, volumetransferapi.Completion{
		ExpectedState: input.ExpectedState, TransferredBytes: input.TransferredBytes, SHA256: input.SHA256,
		LogicalBytes: input.LogicalBytes, DataSHA256: input.DataSHA256,
	})
}

func (adapter *volumeTransferContentAdapter) InternalFail(ctx context.Context, transferID, token string, input volumeTransferFailInput) (model.VolumeTransfer, error) {
	return adapter.service.InternalFail(ctx, transferID, token, volumetransferapi.Failure{
		ExpectedState: input.ExpectedState, ErrorCode: input.ErrorCode, Diagnostic: input.Diagnostic,
	})
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
	return volumetransferapi.DownloadBinding{
		UserID: binding.UserID, SubjectID: binding.SubjectID, Deadline: binding.Deadline,
	}
}

func coreDownloadCredential(credential volumeDownloadCredential) volumetransferapi.DownloadCredential {
	return volumetransferapi.DownloadCredential{Ticket: credential.Ticket, Session: credential.Session}
}

func apiDownloadSession(session volumetransferapi.DownloadSession) volumeDownloadSession {
	return volumeDownloadSession{Token: session.Token, ExpiresAt: session.ExpiresAt}
}

func apiVolumeDownload(download volumetransferapi.Download) volumeDownload {
	return volumeDownload{
		Body: download.Body, Status: download.Status, ContentType: download.ContentType,
		Size: download.Size, ETag: download.ETag, ContentRange: download.ContentRange,
	}
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
