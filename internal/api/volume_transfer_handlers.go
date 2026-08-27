package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
)

// volumeTransferContentService owns direct runtime streaming and one-time
// download authorization. The HTTP layer never buffers a complete archive.
type volumeTransferContentService interface {
	CreateImport(context.Context, model.User, model.Project, volumeImportCreateInput, string) (model.ProjectVolume, model.VolumeTransfer, error)
	StreamImport(context.Context, string, string, model.User, io.Reader, int64) (model.VolumeTransfer, error)
	CreateExport(context.Context, model.User, model.Project, string, volumeExportCreateInput, string) (model.VolumeTransfer, error)
	RetryTransfer(context.Context, model.User, model.Project, model.VolumeTransfer, string) (model.VolumeTransfer, error)
	AuthorizeDownload(context.Context, model.User, model.Project, model.VolumeTransfer, volumeDownloadBinding) (volumeDownloadAuthorizationResponse, error)
	OpenDownload(context.Context, model.User, model.Project, model.VolumeTransfer, string, volumeDownloadBinding) (volumeDownload, error)
	OpenManifest(context.Context, model.User, model.Project, model.VolumeTransfer, string, volumeDownloadBinding) (volumeDownload, error)
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
}

type volumeExportCreateInput struct {
	Format      string `json:"format" binding:"required"`
	Consistency string `json:"consistency" binding:"required"`
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
}

type volumeDownloadAuthorizationResponse struct {
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type volumeDownloadBinding struct {
	UserID    string
	SubjectID string
	Deadline  time.Time
}

type volumeDownload struct {
	Body        io.ReadCloser
	ContentType string
}

func (h *Handlers) CreateVolumeImport(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeImport)...)
	if !ok {
		return
	}
	if !h.ensureBillingAllowsManagedVolumeChange(ctx, project.ID) {
		return
	}
	if !h.ensureVolumeTransferConfigured(ctx) {
		return
	}
	if h.volumeContent == nil {
		writeTransferUnavailable(ctx)
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

func (h *Handlers) UploadVolumeImportContent(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeImport)...)
	if !ok {
		return
	}
	if h.volumeContent == nil {
		writeTransferUnavailable(ctx)
		return
	}
	if ctx.ContentType() != "application/octet-stream" || ctx.Request.ContentLength < 1 {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "Content-Length and application/octet-stream are required")
		return
	}
	deadlineController := http.NewResponseController(ctx.Writer)
	defer func() { _ = deadlineController.SetReadDeadline(time.Time{}) }()
	source := &volumeTransferDeadlineReader{
		reader: ctx.Request.Body, controller: deadlineController, timeout: volumeTransferHTTPIdleTimeout,
	}
	transfer, err := h.volumeContent.StreamImport(ctx.Request.Context(), project.ID, ctx.Param("transferId"), user, source, ctx.Request.ContentLength)
	if err != nil {
		h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, "volume_transfer.import_stream", ctx.Param("transferId"), false, volumeAuditErrorCode(err))
		writeVolumeError(ctx, err)
		return
	}
	h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, "volume_transfer.import_stream", transfer.ID, true, transfer.Format)
	ctx.JSON(http.StatusOK, volumeTransferResponseFor(transfer, true))
}

func (h *Handlers) CreateVolumeExport(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeExport)...)
	if !ok {
		return
	}
	if !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
		return
	}
	if !h.ensureVolumeTransferConfigured(ctx) {
		return
	}
	if h.volumeContent == nil {
		writeTransferUnavailable(ctx)
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
	if transfer.Direction == model.VolumeTransferDirectionImport {
		if !h.ensureBillingAllowsManagedVolumeChange(ctx, project.ID) {
			return
		}
	} else if !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
		return
	}
	if !h.ensureVolumeTransferConfigured(ctx) {
		return
	}
	if h.volumeContent == nil {
		writeTransferUnavailable(ctx)
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
	if h.volumeContent == nil {
		writeTransferUnavailable(ctx)
		return
	}
	binding, ok := h.volumeTransferDownloadBinding(ctx, user)
	if !ok {
		return
	}
	authorization, err := h.volumeContent.AuthorizeDownload(ctx.Request.Context(), user, project, transfer, binding)
	if err != nil {
		h.auditWithContext(user.ID, "volume_transfer.export_authorize", transfer.ID, false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "volume_transfer.export_authorize", transfer.ID, true, transfer.Format, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, authorization)
}

func volumeTransferArchiveFilename(transfer model.VolumeTransfer) string {
	suffix := ".tar.gz"
	if transfer.Format == model.VolumeTransferFormatRawZST {
		suffix = ".raw.zst"
	}
	return transfer.ID + suffix
}

func (h *Handlers) DownloadVolumeTransferContent(ctx *gin.Context) {
	h.serveDirectVolumeTransferDownload(ctx, false)
}

func (h *Handlers) DownloadVolumeTransferManifest(ctx *gin.Context) {
	h.serveDirectVolumeTransferDownload(ctx, true)
}

func (h *Handlers) serveDirectVolumeTransferDownload(ctx *gin.Context, manifest bool) {
	user, project, transfer, ok := h.volumeTransferForRead(ctx)
	if !ok || transfer.Direction != model.VolumeTransferDirectionExport || !h.authorizeTransferDirection(ctx, user, project, transfer) {
		return
	}
	if h.volumeContent == nil {
		writeTransferUnavailable(ctx)
		return
	}
	binding, ok := h.volumeTransferDownloadBinding(ctx, user)
	if !ok {
		return
	}
	ticket := strings.TrimSpace(ctx.Query("ticket"))
	var download volumeDownload
	var err error
	filename := volumeTransferArchiveFilename(transfer)
	if manifest {
		download, err = h.volumeContent.OpenManifest(ctx.Request.Context(), user, project, transfer, ticket, binding)
		filename += ".manifest.json"
	} else {
		download, err = h.volumeContent.OpenDownload(ctx.Request.Context(), user, project, transfer, ticket, binding)
	}
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	ctx.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	ctx.Header("Content-Type", download.ContentType)
	ctx.Header("Cache-Control", "no-store")
	ctx.Status(http.StatusOK)
	deadlineController := http.NewResponseController(ctx.Writer)
	defer func() { _ = deadlineController.SetWriteDeadline(time.Time{}) }()
	destination := &volumeTransferDeadlineWriter{
		writer: ctx.Writer, controller: deadlineController, timeout: volumeTransferHTTPIdleTimeout,
	}
	_, copyErr := io.Copy(destination, download.Body)
	closeErr := download.Body.Close()
	streamErr := errors.Join(copyErr, closeErr)
	action := "volume_transfer.export_stream"
	if manifest {
		action = "volume_transfer.export_manifest"
	}
	if streamErr != nil {
		h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, action, transfer.ID, false, volumeAuditErrorCode(streamErr))
		return
	}
	h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, action, transfer.ID, true, transfer.Format)
}

func (h *Handlers) auditVolumeTransferStreamOutcome(ctx context.Context, userID, action, transferID string, success bool, message string) {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	h.auditWithContext(userID, action, transferID, success, message, auditCtx)
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

func (h *Handlers) ensureVolumeTransferConfigured(ctx *gin.Context) bool {
	if h != nil && h.volumeTransferEnabled {
		return true
	}
	writeVolumeError(ctx, &volume.DomainError{Code: volume.CodeTransferUnavailable, Message: "direct volume transfer is not configured"})
	return false
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

func volumeTransferResponseFor(item model.VolumeTransfer, includeFilename bool, _ ...int64) volumeTransferResponse {
	filename := ""
	if includeFilename {
		filename = item.SourceFilename
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
	}
}

func writeTransferUnavailable(ctx *gin.Context) {
	writeErrorCode(ctx, http.StatusServiceUnavailable, volume.CodeClusterUnavailable, "direct volume transfer is unavailable")
}
