package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/LiteyukiStudio/devops/internal/volumetransferapi"
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

type volumeDownloadBinding = runtimeTerminalAuthorizationBinding

type volumeDownload struct {
	Body        io.ReadCloser
	ContentType string
}

func (h *Handlers) CreateVolumeImport(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionVolumeImport)
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
	user, project, ok := h.authorizeProject(ctx, authz.ActionVolumeImport)
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
	binding, ok := h.requireContinuousAuthorizationBinding(ctx, user)
	if !ok {
		return
	}
	transferID := strings.TrimSpace(ctx.Param("transferId"))
	streamCtx, cancelStream := context.WithCancelCause(ctx.Request.Context())
	defer cancelStream(nil)
	restoreRequestContext := replaceRequestContext(ctx, streamCtx)
	defer restoreRequestContext()
	deadlineController := http.NewResponseController(ctx.Writer)
	defer func() { _ = deadlineController.SetReadDeadline(time.Time{}) }()
	rawBody := ctx.Request.Body
	interruptDone := make(chan struct{})
	stopBodyInterrupt := context.AfterFunc(streamCtx, func() {
		defer close(interruptDone)
		_ = deadlineController.SetReadDeadline(time.Now())
		_ = rawBody.Close()
	})
	defer func() {
		if !stopBodyInterrupt() {
			<-interruptDone
		}
	}()
	reference := volumeImportAuthorizationReference{ProjectID: project.ID, TransferID: transferID}
	authorizationRevoked, authorizationActive := h.monitorContinuousAuthorization(
		streamCtx,
		binding,
		func(checkCtx context.Context, currentUser model.User) bool {
			return h.volumeImportAuthorizationAllowed(checkCtx, currentUser, reference)
		},
		func() { cancelStream(volumetransferapi.ErrStreamAuthorizationRevoked) },
	)
	if !authorizationActive {
		h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, "volume_transfer.import_stream", transferID, false, "authorization_revoked")
		writeContinuousAuthorizationRevoked(ctx)
		return
	}
	source := &volumeTransferDeadlineReader{
		reader: rawBody, controller: deadlineController, timeout: volumeTransferHTTPIdleTimeout,
	}
	transfer, err := h.volumeContent.StreamImport(streamCtx, project.ID, transferID, user, source, ctx.Request.ContentLength)
	if err != nil {
		select {
		case <-authorizationRevoked:
			h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, "volume_transfer.import_stream", transferID, false, "authorization_revoked")
			writeContinuousAuthorizationRevoked(ctx)
			return
		default:
		}
		if errors.Is(err, volumetransferapi.ErrStreamAuthorizationRevoked) || errors.Is(context.Cause(streamCtx), volumetransferapi.ErrStreamAuthorizationRevoked) {
			h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, "volume_transfer.import_stream", transferID, false, "authorization_revoked")
			writeContinuousAuthorizationRevoked(ctx)
			return
		}
		h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, "volume_transfer.import_stream", transferID, false, volumeAuditErrorCode(err))
		writeVolumeError(ctx, err)
		return
	}
	select {
	case <-authorizationRevoked:
		h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, "volume_transfer.import_stream", transferID, false, "authorization_revoked")
		writeContinuousAuthorizationRevoked(ctx)
		return
	default:
	}
	h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, "volume_transfer.import_stream", transfer.ID, true, transfer.Format)
	ctx.JSON(http.StatusOK, volumeTransferResponseFor(transfer, true))
}

type volumeImportAuthorizationReference struct {
	ProjectID  string
	TransferID string
}

func (h *Handlers) volumeImportAuthorizationAllowed(ctx context.Context, user model.User, reference volumeImportAuthorizationReference) bool {
	if !h.projectContinuousAuthorizationAllowed(ctx, user, reference.ProjectID, authz.ActionVolumeImport) {
		return false
	}
	db := h.dbWithContext(ctx)
	if db == nil {
		return false
	}
	var transfer model.VolumeTransfer
	if err := db.First(&transfer, "id = ? and project_id = ?", reference.TransferID, reference.ProjectID).Error; err != nil ||
		transfer.Direction != model.VolumeTransferDirectionImport ||
		(transfer.State != model.VolumeTransferStateReady && transfer.State != model.VolumeTransferStateStreaming && transfer.State != model.VolumeTransferStateSucceeded) {
		return false
	}
	if transfer.State == model.VolumeTransferStateReady && !transfer.ExpiresAt.After(time.Now()) {
		return false
	}
	if transfer.ActorID == user.ID {
		return true
	}
	return h.projectContinuousAuthorizationAllowed(ctx, user, reference.ProjectID, authz.ActionVolumeExport)
}

func (h *Handlers) CreateVolumeExport(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionVolumeExport)
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
	user, project, ok := h.authorizeProject(ctx, authz.ActionVolumeRead)
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
	privileged := authz.IsPlatformAdmin(user.Role)
	if !privileged {
		var available bool
		privileged, available = h.projectMemberActionAllowed(ctx, project.ID, user.ID, authz.ActionVolumeExport)
		if !available {
			return
		}
	}
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
	user, project, ok := h.authorizeProject(ctx, authz.ActionVolumeRead)
	if !ok {
		return
	}
	transfer, err := h.volumes.GetVolumeTransfer(ctx.Request.Context(), project.ID, ctx.Param("transferId"))
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	privileged := authz.IsPlatformAdmin(user.Role)
	if !privileged {
		var available bool
		privileged, available = h.projectMemberActionAllowed(ctx, project.ID, user.ID, authz.ActionVolumeExport)
		if !available {
			return
		}
	}
	ctx.JSON(http.StatusOK, volumeTransferResponseFor(transfer, privileged || transfer.ActorID == user.ID, h.volumeTransferMaxBytes))
}

func (h *Handlers) RetryVolumeTransfer(ctx *gin.Context) {
	user, project, transfer, ok := h.volumeTransferForAction(ctx, authz.ActionVolumeRead)
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
	user, project, transfer, ok := h.volumeTransferForAction(ctx, authz.ActionVolumeRead)
	if !ok {
		return
	}
	if transfer.ActorID != user.ID {
		if !authz.IsPlatformAdmin(user.Role) {
			allowed, available := h.projectMemberActionAllowed(ctx, project.ID, user.ID, authz.ActionVolumeExport)
			if !available {
				return
			}
			if !allowed {
				writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "only the creator or a project Owner/Admin can cancel this transfer")
				return
			}
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
	user, project, transfer, ok := h.volumeTransferForAction(ctx, authz.ActionVolumeExport)
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
	user, project, transfer, ok := h.volumeTransferForAction(ctx, authz.ActionVolumeExport)
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
	streamCtx, cancelStream := context.WithDeadline(ctx.Request.Context(), binding.Deadline)
	defer cancelStream()
	ticket := strings.TrimSpace(ctx.Query("ticket"))
	var download volumeDownload
	var err error
	filename := volumeTransferArchiveFilename(transfer)
	action := "volume_transfer.export_stream"
	if manifest {
		download, err = h.volumeContent.OpenManifest(streamCtx, user, project, transfer, ticket, binding)
		filename += ".manifest.json"
		action = "volume_transfer.export_manifest"
	} else {
		download, err = h.volumeContent.OpenDownload(streamCtx, user, project, transfer, ticket, binding)
	}
	if err != nil {
		if reason := volumeDownloadStreamFailureReason(streamCtx, nil, err); reason == "authorization_deadline_reached" || reason == "request_cancelled" {
			h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, action, transfer.ID, false, reason)
			writeErrorCode(ctx, http.StatusUnauthorized, volume.CodeTransferDownloadUnauthorized, "download authorization expired or was cancelled")
			return
		}
		writeVolumeError(ctx, err)
		return
	}
	reference := volumeTransferDownloadAuthorizationReference{
		ProjectID: project.ID, TransferID: transfer.ID, Manifest: manifest,
	}
	authorizationAllowed := func(checkCtx context.Context, currentUser model.User) bool {
		return h.volumeTransferDownloadAuthorizationAllowed(checkCtx, currentUser, reference)
	}
	authorizationRevoked, authorizationActive := h.monitorContinuousAuthorization(streamCtx, binding, authorizationAllowed, cancelStream)
	if !authorizationActive {
		if download.Body != nil {
			_ = download.Body.Close()
		}
		h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, action, transfer.ID, false, "authorization_revoked")
		writeErrorCode(ctx, http.StatusUnauthorized, volume.CodeTransferDownloadUnauthorized, "download authorization expired or was revoked")
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
	streamErr := copyVolumeDownloadBody(streamCtx, destination, download.Body, func() {
		_ = deadlineController.SetWriteDeadline(time.Now())
	})
	reason := volumeDownloadStreamFailureReason(streamCtx, authorizationRevoked, streamErr)
	cancelStream()
	if reason != "" {
		h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, action, transfer.ID, false, reason)
		return
	}
	h.auditVolumeTransferStreamOutcome(ctx.Request.Context(), user.ID, action, transfer.ID, true, transfer.Format)
}

type volumeTransferDownloadAuthorizationReference struct {
	ProjectID  string
	TransferID string
	Manifest   bool
}

func (h *Handlers) volumeTransferDownloadAuthorizationAllowed(ctx context.Context, user model.User, reference volumeTransferDownloadAuthorizationReference) bool {
	db := h.dbWithContext(ctx)
	if db == nil {
		return false
	}
	var project model.Project
	if err := db.First(&project, "id = ?", reference.ProjectID).Error; err != nil || !resourceCanMutateDuringDelete(project.DeleteStatus) {
		return false
	}
	var transfer model.VolumeTransfer
	if err := db.First(&transfer, "id = ? and project_id = ?", reference.TransferID, reference.ProjectID).Error; err != nil {
		return false
	}
	if transfer.Direction != model.VolumeTransferDirectionExport {
		return false
	}
	if reference.Manifest {
		if transfer.State != model.VolumeTransferStateSucceeded || transfer.Format != model.VolumeTransferFormatRawZST {
			return false
		}
	} else if transfer.State != model.VolumeTransferStateReady && transfer.State != model.VolumeTransferStateStreaming && transfer.State != model.VolumeTransferStateSucceeded {
		return false
	}

	subject := authz.ProjectSubject{UserID: user.ID, PlatformRole: user.Role}
	_, err := h.projectAuthorizer(ctx).AuthorizeProject(ctx, subject, reference.ProjectID, authz.ActionVolumeExport)
	return err == nil
}

type closeOnceReadCloser struct {
	body io.ReadCloser
	once sync.Once
	err  error
}

func (reader *closeOnceReadCloser) Read(buffer []byte) (int, error) {
	return reader.body.Read(buffer)
}

func (reader *closeOnceReadCloser) Close() error {
	reader.once.Do(func() { reader.err = reader.body.Close() })
	return reader.err
}

func copyVolumeDownloadBody(ctx context.Context, destination io.Writer, body io.ReadCloser, interrupt func()) error {
	if body == nil {
		return io.ErrUnexpectedEOF
	}
	closer := &closeOnceReadCloser{body: body}
	interruptDone := make(chan struct{})
	stopInterrupt := context.AfterFunc(ctx, func() {
		defer close(interruptDone)
		if interrupt != nil {
			interrupt()
		}
		_ = closer.Close()
	})
	_, copyErr := io.Copy(destination, closer)
	closeErr := closer.Close()
	if !stopInterrupt() {
		<-interruptDone
	}
	return errors.Join(copyErr, closeErr)
}

func volumeDownloadStreamFailureReason(ctx context.Context, authorizationRevoked <-chan struct{}, streamErr error) string {
	if authorizationRevoked != nil {
		select {
		case <-authorizationRevoked:
			return "authorization_revoked"
		default:
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "authorization_deadline_reached"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "request_cancelled"
	}
	if streamErr != nil {
		if code := volume.ErrorCode(streamErr); code != "" {
			return code
		}
		return "stream_interrupted"
	}
	return ""
}

func (h *Handlers) auditVolumeTransferStreamOutcome(ctx context.Context, userID, action, transferID string, success bool, message string) {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	h.auditWithContext(userID, action, transferID, success, message, auditCtx)
}

func (h *Handlers) volumeTransferForAction(ctx *gin.Context, action authz.Action) (model.User, model.Project, model.VolumeTransfer, bool) {
	user, project, ok := h.authorizeProject(ctx, action)
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
	if !authz.IsPlatformAdmin(user.Role) {
		allowed, available := h.projectMemberActionAllowed(ctx, project.ID, user.ID, action)
		if !available {
			return false
		}
		if !allowed {
			writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "project role does not allow this transfer operation")
			return false
		}
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
