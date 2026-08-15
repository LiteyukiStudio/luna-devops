package api

import (
	"context"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/LiteyukiStudio/devops/internal/volumetransferapi"
	"github.com/gin-gonic/gin"
)

// volumeTransferContentService owns object-store streaming, upload offsets,
// one-time download tickets, and transfer-bound callback authentication. The
// HTTP layer deliberately cannot reach the backing object store directly.
type volumeTransferContentService interface {
	CreateImport(context.Context, model.User, model.Project, volumeImportCreateInput, string) (model.ProjectVolume, model.VolumeTransfer, error)
	HeadImport(context.Context, string, string, model.User) (int64, int64, int64, error)
	WriteImportPart(context.Context, string, string, model.User, int64, string, io.Reader, int64) (int64, int64, error)
	CompleteImport(context.Context, string, string, model.User, int64, string) (model.VolumeTransfer, error)
	CreateExport(context.Context, model.User, model.Project, string, volumeExportCreateInput, string) (model.VolumeTransfer, error)
	RetryTransfer(context.Context, model.User, model.Project, model.VolumeTransfer, string) (model.VolumeTransfer, error)
	AuthorizeDownload(context.Context, model.User, model.Project, model.VolumeTransfer, volumeDownloadBinding) (volumeDownloadAuthorizationResponse, error)
	HeadDownload(context.Context, model.User, model.Project, model.VolumeTransfer, volumeDownloadCredential, volumeDownloadBinding) (volumeDownloadInfo, volumeDownloadSession, error)
	OpenDownload(context.Context, model.User, model.Project, model.VolumeTransfer, volumeDownloadCredential, string, volumeDownloadBinding) (volumeDownload, volumeDownloadSession, error)
	HeadManifest(context.Context, model.User, model.Project, model.VolumeTransfer, volumeDownloadCredential, volumeDownloadBinding) (volumeDownloadInfo, volumeDownloadSession, error)
	OpenManifest(context.Context, model.User, model.Project, model.VolumeTransfer, volumeDownloadCredential, volumeDownloadBinding) (volumeDownload, volumeDownloadSession, error)
	InternalHead(context.Context, string, string) (volumeInternalContentInfo, error)
	InternalWritePart(context.Context, string, string, int64, string, io.Reader, int64) (int64, int64, error)
	InternalOpen(context.Context, string, string, string) (volumeDownload, error)
	InternalProgress(context.Context, string, string, volumeTransferProgressInput) error
	InternalComplete(context.Context, string, string, volumeTransferCompleteInput) (model.VolumeTransfer, error)
	InternalFail(context.Context, string, string, volumeTransferFailInput) (model.VolumeTransfer, error)
}

type volumeImportCreateInput struct {
	DisplayName      string `json:"displayName" binding:"required"`
	ClusterID        string `json:"clusterId" binding:"required"`
	Capacity         string `json:"capacity" binding:"required"`
	StorageClassName string `json:"storageClassName" binding:"required"`
	AccessMode       string `json:"accessMode" binding:"required"`
	VolumeMode       string `json:"volumeMode" binding:"required"`
	Format           string `json:"format" binding:"required"`
	Filename         string `json:"filename" binding:"required"`
	ContentLength    int64  `json:"contentLength" binding:"required"`
	SHA256           string `json:"sha256"`
}

type volumeExportCreateInput struct {
	Format      string `json:"format" binding:"required"`
	Consistency string `json:"consistency" binding:"required"`
}

type volumeImportCompleteInput struct {
	ContentLength int64  `json:"contentLength" binding:"required"`
	SHA256        string `json:"sha256" binding:"required"`
}

type volumeTransferResponse struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"projectId"`
	ProjectVolumeID  string     `json:"projectVolumeId"`
	Direction        string     `json:"direction"`
	Format           string     `json:"format"`
	ConsistencyMode  string     `json:"consistencyMode"`
	State            string     `json:"state"`
	SourceFilename   string     `json:"sourceFilename,omitempty"`
	ExpectedBytes    int64      `json:"expectedBytes"`
	TransferredBytes int64      `json:"transferredBytes"`
	ProcessedFiles   int64      `json:"processedFiles"`
	Phase            string     `json:"phase,omitempty"`
	SHA256           string     `json:"sha256"`
	LogicalBytes     int64      `json:"logicalBytes"`
	DataSHA256       string     `json:"dataSHA256"`
	ActorID          string     `json:"actorId"`
	ExpiresAt        time.Time  `json:"expiresAt"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	LastErrorCode    string     `json:"lastErrorCode"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	ChunkSize        int64      `json:"chunkSize"`
}

type volumeDownloadAuthorizationResponse struct {
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type volumeDownloadInfo struct {
	Size int64
	ETag string
}

type volumeDownloadCredential struct {
	Ticket  string
	Session string
}

type volumeDownloadSession struct {
	Token     string
	ExpiresAt time.Time
}

type volumeDownloadBinding struct {
	UserID            string
	SubjectID         string
	AssertionID       string
	AssertionRequired bool
	Deadline          time.Time
}

type volumeDownload struct {
	Body         io.ReadCloser
	Status       int
	ContentType  string
	Size         int64
	ETag         string
	ContentRange string
}

func (h *Handlers) CreateVolumeImport(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeImport)...)
	if !ok {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeVolumeImport) {
		return
	}
	if !h.ensureBillingAllowsManagedVolumeChange(ctx, project.ID) {
		return
	}
	if h.volumeContent == nil {
		writeTransferStoreUnavailable(ctx)
		return
	}
	var input volumeImportCreateInput
	if !bindJSON(ctx, &input) {
		return
	}
	if _, usable := h.runtimeClusterForProjectUse(ctx, user, project.ID, input.ClusterID); !usable {
		return
	}
	idempotencyKey, ok := volumeIdempotencyKey(ctx)
	if !ok {
		return
	}
	if _, _, valid := parseVolumeCapacity(input.Capacity); !valid || input.ContentLength < 1 {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "volume import size or capacity is invalid")
		return
	}
	projectVolume, transfer, err := h.volumeContent.CreateImport(ctx.Request.Context(), user, project, input, idempotencyKey)
	if err != nil {
		h.auditWithContext(user.ID, "volume_transfer.import_start", project.ID, false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "volume_transfer.import_start", transfer.ID, true, transfer.Format, ctx.Request.Context())
	ctx.JSON(http.StatusAccepted, gin.H{
		"volume":   projectVolumeResponseFor(projectVolume),
		"transfer": volumeTransferResponseFor(transfer, true, h.volumeTransferMaxBytes),
	})
}

func (h *Handlers) GetVolumeImportUploadOffset(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeImport)...)
	if !ok {
		return
	}
	if h.volumeContent == nil {
		writeTransferStoreUnavailable(ctx)
		return
	}
	if ctx.GetHeader("Tus-Resumable") != "1.0.0" {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "invalid TUS upload headers")
		return
	}
	offset, length, chunkSize, err := h.volumeContent.HeadImport(ctx.Request.Context(), project.ID, ctx.Param("transferId"), user)
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	ctx.Header("Tus-Resumable", "1.0.0")
	ctx.Header("Upload-Offset", formatInt64(offset))
	ctx.Header("Upload-Length", formatInt64(length))
	ctx.Header("Upload-Chunk-Size", formatInt64(chunkSize))
	ctx.Status(http.StatusOK)
}

func (h *Handlers) UploadVolumeImportContent(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeImport)...)
	if !ok {
		return
	}
	if h.volumeContent == nil {
		writeTransferStoreUnavailable(ctx)
		return
	}
	offset, valid := parseNonNegativeInt64Header(ctx, "Upload-Offset")
	if !valid {
		return
	}
	checksum := strings.TrimSpace(ctx.GetHeader("Upload-Checksum"))
	if ctx.GetHeader("Tus-Resumable") != "1.0.0" || !strings.HasPrefix(checksum, "sha256 ") || ctx.ContentType() != "application/offset+octet-stream" {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "invalid TUS upload headers")
		return
	}
	if ctx.Request.ContentLength < 1 || ctx.Request.ContentLength > volumetransferapi.MaximumChunkSize {
		writeErrorCode(ctx, http.StatusRequestEntityTooLarge, volume.CodeInvalidInput, "volume upload chunk exceeds the allowed size")
		return
	}
	nextOffset, chunkSize, err := h.volumeContent.WriteImportPart(
		ctx.Request.Context(), project.ID, ctx.Param("transferId"), user, offset,
		strings.TrimPrefix(checksum, "sha256 "), ctx.Request.Body, ctx.Request.ContentLength,
	)
	if err != nil {
		if code := volume.ErrorCode(err); code == volume.CodeTransferOffsetMismatch || code == volume.CodeTransferPartInProgress {
			if current, _, currentChunkSize, headErr := h.volumeContent.HeadImport(ctx.Request.Context(), project.ID, ctx.Param("transferId"), user); headErr == nil {
				ctx.Header("Upload-Offset", formatInt64(current))
				ctx.Header("Upload-Chunk-Size", formatInt64(currentChunkSize))
			}
		}
		writeVolumeError(ctx, err)
		return
	}
	ctx.Header("Tus-Resumable", "1.0.0")
	ctx.Header("Upload-Offset", formatInt64(nextOffset))
	ctx.Header("Upload-Chunk-Size", formatInt64(chunkSize))
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) CompleteVolumeImportUpload(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeImport)...)
	if !ok {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeVolumeImport) {
		return
	}
	if h.volumeContent == nil {
		writeTransferStoreUnavailable(ctx)
		return
	}
	var input volumeImportCompleteInput
	if !bindJSON(ctx, &input) {
		return
	}
	transfer, err := h.volumeContent.CompleteImport(ctx.Request.Context(), project.ID, ctx.Param("transferId"), user, input.ContentLength, input.SHA256)
	if err != nil {
		h.auditWithContext(user.ID, "volume_transfer.import_complete", ctx.Param("transferId"), false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "volume_transfer.import_complete", transfer.ID, true, transfer.Format, ctx.Request.Context())
	ctx.JSON(http.StatusAccepted, volumeTransferResponseFor(transfer, true, h.volumeTransferMaxBytes))
}

func (h *Handlers) CreateVolumeExport(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeExport)...)
	if !ok {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeVolumeExport) {
		return
	}
	if !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
		return
	}
	if h.volumeContent == nil {
		writeTransferStoreUnavailable(ctx)
		return
	}
	var input volumeExportCreateInput
	if !bindJSON(ctx, &input) {
		return
	}
	idempotencyKey, ok := volumeIdempotencyKey(ctx)
	if !ok {
		return
	}
	transfer, err := h.volumeContent.CreateExport(ctx.Request.Context(), user, project, ctx.Param("volumeId"), input, idempotencyKey)
	if err != nil {
		h.auditWithContext(user.ID, "volume_transfer.export_start", ctx.Param("volumeId"), false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "volume_transfer.export_start", transfer.ID, true, transfer.Format, ctx.Request.Context())
	ctx.JSON(http.StatusAccepted, volumeTransferResponseFor(transfer, true, h.volumeTransferMaxBytes))
}

func (h *Handlers) ListVolumeTransfers(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeRead)...)
	if !ok {
		return
	}
	pagination, valid := volumePagination(ctx, map[string]bool{
		"createdAt": true, "updatedAt": true, "state": true, "transferredBytes": true,
	}, "createdAt")
	if !valid {
		return
	}
	result, err := h.volumes.ListVolumeTransfers(ctx.Request.Context(), project.ID, volume.VolumeTransferListOptions{
		Page: pagination.Page, PageSize: pagination.PageSize, SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
		Direction: ctx.Query("direction"), State: ctx.Query("state"), VolumeID: ctx.Query("volumeId"), CreatedBy: ctx.Query("createdBy"),
	})
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	privileged := authz.IsPlatformAdmin(user.Role) || h.currentProjectRoleAllows(ctx, project.ID, user.ID, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	items := make([]volumeTransferResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, volumeTransferResponseFor(item, privileged || item.ActorID == user.ID, h.volumeTransferMaxBytes))
	}
	ctx.JSON(http.StatusOK, gin.H{
		"items": items, "page": result.Page, "pageSize": result.PageSize,
		"sortBy": result.SortBy, "sortOrder": result.SortOrder,
		"total": result.Total, "totalPages": result.TotalPages,
	})
}

func (h *Handlers) GetVolumeTransfer(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeRead)...)
	if !ok {
		return
	}
	transfer, err := h.volumes.GetVolumeTransfer(ctx.Request.Context(), project.ID, ctx.Param("transferId"))
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	privileged := authz.IsPlatformAdmin(user.Role) || h.currentProjectRoleAllows(ctx, project.ID, user.ID, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	ctx.JSON(http.StatusOK, volumeTransferResponseFor(transfer, privileged || transfer.ActorID == user.ID, h.volumeTransferMaxBytes))
}

func (h *Handlers) RetryVolumeTransfer(ctx *gin.Context) {
	user, project, transfer, ok := h.volumeTransferForRead(ctx)
	if !ok {
		return
	}
	if !h.authorizeTransferDirection(ctx, user, project, transfer) {
		return
	}
	if !h.requireStepUp(ctx, user, transferStepUpPurpose(transfer)) {
		return
	}
	if transfer.Direction == model.VolumeTransferDirectionImport {
		if !h.ensureBillingAllowsManagedVolumeChange(ctx, project.ID) {
			return
		}
	} else if !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
		return
	}
	if h.volumeContent == nil {
		writeTransferStoreUnavailable(ctx)
		return
	}
	idempotencyKey, ok := volumeIdempotencyKey(ctx)
	if !ok {
		return
	}
	retried, err := h.volumeContent.RetryTransfer(ctx.Request.Context(), user, project, transfer, idempotencyKey)
	if err != nil {
		h.auditWithContext(user.ID, "volume_transfer.retry", transfer.ID, false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "volume_transfer.retry", retried.ID, true, transfer.Direction, ctx.Request.Context())
	ctx.JSON(http.StatusAccepted, volumeTransferResponseFor(retried, true, h.volumeTransferMaxBytes))
}

func (h *Handlers) CancelVolumeTransfer(ctx *gin.Context) {
	user, project, transfer, ok := h.volumeTransferForRead(ctx)
	if !ok {
		return
	}
	if transfer.ActorID != user.ID {
		if !authz.IsPlatformAdmin(user.Role) && !h.currentProjectRoleAllows(ctx, project.ID, user.ID, authz.ProjectRoleOwner, authz.ProjectRoleAdmin) {
			writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "only the creator or a project Owner/Admin can cancel this transfer")
			return
		}
		if token, bearer := currentAccessTokenFromContext(ctx); bearer && !accessTokenAllows(token.Scope, string(authz.ActionVolumeDelete)) {
			writeErrorCode(ctx, http.StatusForbidden, "auth.token.scope_insufficient", "volume:delete scope is required to cancel another user's transfer")
			return
		}
	}
	cancelled, err := h.volumes.TransitionVolumeTransfer(ctx.Request.Context(), project.ID, transfer.ID, model.VolumeTransferStateCancelled, "", "")
	if err != nil {
		h.auditWithContext(user.ID, "volume_transfer.cancel", transfer.ID, false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "volume_transfer.cancel", transfer.ID, true, transfer.Direction, ctx.Request.Context())
	ctx.JSON(http.StatusOK, volumeTransferResponseFor(cancelled, true, h.volumeTransferMaxBytes))
}

func (h *Handlers) AuthorizeVolumeTransferDownload(ctx *gin.Context) {
	user, project, transfer, ok := h.volumeTransferForRead(ctx)
	if !ok {
		return
	}
	if !h.authorizeTransferDirection(ctx, user, project, transfer) || transfer.Direction != model.VolumeTransferDirectionExport {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeVolumeExport) {
		return
	}
	if h.volumeContent == nil {
		writeTransferStoreUnavailable(ctx)
		return
	}
	binding, ok := h.volumeTransferDownloadBinding(ctx, user)
	if !ok {
		return
	}
	authorization, err := h.volumeContent.AuthorizeDownload(ctx.Request.Context(), user, project, transfer, binding)
	if err != nil {
		h.auditWithContext(user.ID, "volume_transfer.export_download", transfer.ID, false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "volume_transfer.export_download", transfer.ID, true, transfer.Format, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, authorization)
}

func (h *Handlers) HeadVolumeTransferContent(ctx *gin.Context) {
	h.serveVolumeTransferContent(ctx, true)
}

func (h *Handlers) DownloadVolumeTransferContent(ctx *gin.Context) {
	h.serveVolumeTransferContent(ctx, false)
}

func (h *Handlers) HeadVolumeTransferManifest(ctx *gin.Context) {
	h.serveVolumeTransferManifest(ctx, true)
}

func (h *Handlers) DownloadVolumeTransferManifest(ctx *gin.Context) {
	h.serveVolumeTransferManifest(ctx, false)
}

func (h *Handlers) serveVolumeTransferContent(ctx *gin.Context, head bool) {
	user, project, transfer, ok := h.volumeTransferForRead(ctx)
	if !ok {
		return
	}
	if transfer.Direction != model.VolumeTransferDirectionExport || !h.authorizeTransferDirection(ctx, user, project, transfer) {
		return
	}
	if h.volumeContent == nil {
		writeTransferStoreUnavailable(ctx)
		return
	}
	binding, ok := h.volumeTransferDownloadBinding(ctx, user)
	if !ok {
		return
	}
	credential := volumeDownloadCredential{Ticket: strings.TrimSpace(ctx.Query("ticket"))}
	credential.Session, _ = ctx.Cookie(volumeDownloadSessionCookieName)
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": volumeTransferArchiveFilename(transfer)})
	if head {
		info, session, err := h.volumeContent.HeadDownload(ctx.Request.Context(), user, project, transfer, credential, binding)
		if err != nil {
			writeVolumeError(ctx, err)
			return
		}
		h.setVolumeDownloadSessionCookie(ctx, session)
		ctx.Header("Accept-Ranges", "bytes")
		ctx.Header("Content-Disposition", disposition)
		ctx.Header("Content-Length", formatInt64(info.Size))
		ctx.Header("ETag", info.ETag)
		ctx.Status(http.StatusOK)
		return
	}
	download, session, err := h.volumeContent.OpenDownload(ctx.Request.Context(), user, project, transfer, credential, ctx.GetHeader("Range"), binding)
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	defer download.Body.Close()
	h.setVolumeDownloadSessionCookie(ctx, session)
	ctx.Header("Accept-Ranges", "bytes")
	ctx.Header("Content-Disposition", disposition)
	ctx.Header("ETag", download.ETag)
	if download.ContentRange != "" {
		ctx.Header("Content-Range", download.ContentRange)
	}
	ctx.DataFromReader(download.Status, download.Size, download.ContentType, download.Body, nil)
}

func volumeTransferArchiveFilename(transfer model.VolumeTransfer) string {
	suffix := ".tar.gz"
	if transfer.Format == model.VolumeTransferFormatRawZST {
		suffix = ".raw.zst"
	}
	return transfer.ID + suffix
}

func (h *Handlers) serveVolumeTransferManifest(ctx *gin.Context, head bool) {
	user, project, transfer, ok := h.volumeTransferForRead(ctx)
	if !ok {
		return
	}
	if transfer.Direction != model.VolumeTransferDirectionExport || !h.authorizeTransferDirection(ctx, user, project, transfer) {
		return
	}
	if h.volumeContent == nil {
		writeTransferStoreUnavailable(ctx)
		return
	}
	binding, ok := h.volumeTransferDownloadBinding(ctx, user)
	if !ok {
		return
	}
	credential := volumeDownloadCredential{Ticket: strings.TrimSpace(ctx.Query("ticket"))}
	credential.Session, _ = ctx.Cookie(volumeDownloadSessionCookieName)
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": transfer.ID + ".raw.zst.manifest.json"})
	if head {
		info, session, err := h.volumeContent.HeadManifest(ctx.Request.Context(), user, project, transfer, credential, binding)
		if err != nil {
			writeVolumeError(ctx, err)
			return
		}
		h.setVolumeDownloadSessionCookie(ctx, session)
		ctx.Header("Content-Length", formatInt64(info.Size))
		ctx.Header("Content-Type", "application/json; charset=utf-8")
		ctx.Header("Content-Disposition", disposition)
		ctx.Header("ETag", info.ETag)
		ctx.Status(http.StatusOK)
		return
	}
	download, session, err := h.volumeContent.OpenManifest(ctx.Request.Context(), user, project, transfer, credential, binding)
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	defer download.Body.Close()
	h.setVolumeDownloadSessionCookie(ctx, session)
	ctx.Header("Content-Disposition", disposition)
	ctx.Header("ETag", download.ETag)
	ctx.DataFromReader(download.Status, download.Size, download.ContentType, download.Body, nil)
}

const volumeDownloadSessionCookieName = "luna_volume_download_session"

func (h *Handlers) setVolumeDownloadSessionCookie(ctx *gin.Context, session volumeDownloadSession) {
	remaining := time.Until(session.ExpiresAt)
	if strings.TrimSpace(session.Token) == "" || remaining < time.Second {
		return
	}
	maxAge := int(remaining.Seconds())
	secure := h == nil || h.mode != "development"
	http.SetCookie(ctx.Writer, &http.Cookie{
		Name: volumeDownloadSessionCookieName, Value: session.Token,
		Path: volumeDownloadSessionCookiePath(ctx.Request.URL.Path), Expires: session.ExpiresAt, MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

func volumeDownloadSessionCookiePath(requestPath string) string {
	const marker = "/volume-transfers/"
	index := strings.Index(requestPath, marker)
	if index < 0 {
		return requestPath
	}
	transferStart := index + len(marker)
	remainder := requestPath[transferStart:]
	slash := strings.IndexByte(remainder, '/')
	if slash < 1 {
		return requestPath
	}
	return requestPath[:transferStart+slash+1]
}

func (h *Handlers) volumeTransferForRead(ctx *gin.Context) (model.User, model.Project, model.VolumeTransfer, bool) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeRead)...)
	if !ok {
		return model.User{}, model.Project{}, model.VolumeTransfer{}, false
	}
	transfer, err := h.volumes.GetVolumeTransfer(ctx.Request.Context(), project.ID, ctx.Param("transferId"))
	if err != nil {
		writeVolumeError(ctx, err)
		return model.User{}, model.Project{}, model.VolumeTransfer{}, false
	}
	return user, project, transfer, true
}

func (h *Handlers) authorizeTransferDirection(ctx *gin.Context, user model.User, project model.Project, transfer model.VolumeTransfer) bool {
	action := authz.ActionVolumeImport
	if transfer.Direction == model.VolumeTransferDirectionExport {
		action = authz.ActionVolumeExport
	}
	if !authz.IsPlatformAdmin(user.Role) && !h.currentProjectRoleAllows(ctx, project.ID, user.ID, volumeActionRoles(action)...) {
		writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "project role does not allow this transfer operation")
		return false
	}
	if token, bearer := currentAccessTokenFromContext(ctx); bearer && !accessTokenAllows(token.Scope, string(action)) {
		writeErrorCode(ctx, http.StatusForbidden, "auth.token.scope_insufficient", "the original transfer operation scope is required")
		return false
	}
	return true
}

func transferStepUpPurpose(transfer model.VolumeTransfer) string {
	if transfer.Direction == model.VolumeTransferDirectionExport {
		return stepUpPurposeVolumeExport
	}
	return stepUpPurposeVolumeImport
}

func volumeTransferResponseFor(item model.VolumeTransfer, includeFilename bool, configuredMaxBytes ...int64) volumeTransferResponse {
	filename := ""
	if includeFilename {
		filename = item.SourceFilename
	}
	expectedForChunk := item.ExpectedBytes
	if item.Direction == model.VolumeTransferDirectionExport && len(configuredMaxBytes) > 0 && configuredMaxBytes[0] > 0 {
		expectedForChunk = configuredMaxBytes[0]
	}
	return volumeTransferResponse{
		ID: item.ID, ProjectID: item.ProjectID, ProjectVolumeID: item.ProjectVolumeID,
		Direction: item.Direction, Format: item.Format, ConsistencyMode: item.ConsistencyMode,
		State: item.State, SourceFilename: filename, ExpectedBytes: item.ExpectedBytes,
		TransferredBytes: item.TransferredBytes, ProcessedFiles: item.ProcessedFiles, Phase: item.Phase,
		SHA256: item.SHA256, LogicalBytes: item.LogicalBytes, DataSHA256: item.DataSHA256,
		ActorID: item.ActorID, ExpiresAt: item.ExpiresAt,
		StartedAt: item.StartedAt, FinishedAt: item.FinishedAt, LastErrorCode: item.LastErrorCode,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		ChunkSize: volumetransferapi.RequiredChunkSize(expectedForChunk),
	}
}

func writeTransferStoreUnavailable(ctx *gin.Context) {
	writeErrorCode(ctx, http.StatusServiceUnavailable, volume.CodeTransferStoreUnavailable, "volume transfer store is unavailable")
}

func parseNonNegativeInt64Header(ctx *gin.Context, name string) (int64, bool) {
	raw := strings.TrimSpace(ctx.GetHeader(name))
	value, err := parseInt64(raw)
	if err != nil || value < 0 {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, name+" must be a non-negative integer")
		return 0, false
	}
	return value, true
}
