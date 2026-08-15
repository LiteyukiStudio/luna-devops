package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/LiteyukiStudio/devops/internal/volumetransferapi"
	"github.com/gin-gonic/gin"
)

type volumeInternalContentInfo struct {
	Offset    int64
	Size      int64
	ChunkSize int64
	ETag      string
}

type volumeTransferProgressInput struct {
	ExpectedState    string `json:"expectedState" binding:"required"`
	TransferredBytes int64  `json:"transferredBytes"`
	ProcessedFiles   int64  `json:"processedFiles"`
	Stage            string `json:"stage" binding:"required"`
}

type volumeTransferCompleteInput struct {
	ExpectedState    string `json:"expectedState" binding:"required"`
	TransferredBytes int64  `json:"transferredBytes"`
	SHA256           string `json:"sha256" binding:"required"`
	LogicalBytes     int64  `json:"logicalBytes"`
	DataSHA256       string `json:"dataSHA256"`
}

type volumeTransferFailInput struct {
	ExpectedState string `json:"expectedState" binding:"required"`
	ErrorCode     string `json:"errorCode" binding:"required"`
	Diagnostic    string `json:"diagnostic"`
}

func (h *Handlers) HeadInternalVolumeTransferContent(ctx *gin.Context) {
	token, ok := internalVolumeTransferToken(ctx)
	if !ok || !h.ensureVolumeContentService(ctx) {
		return
	}
	info, err := h.volumeContent.InternalHead(ctx.Request.Context(), ctx.Param("transferId"), token)
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	ctx.Header("Tus-Resumable", "1.0.0")
	ctx.Header("Upload-Offset", formatInt64(info.Offset))
	ctx.Header("Upload-Length", formatInt64(info.Size))
	ctx.Header("Upload-Chunk-Size", formatInt64(info.ChunkSize))
	if info.ETag != "" {
		ctx.Header("ETag", info.ETag)
	}
	ctx.Status(http.StatusOK)
}

func (h *Handlers) UploadInternalVolumeTransferContent(ctx *gin.Context) {
	token, ok := internalVolumeTransferToken(ctx)
	if !ok || !h.ensureVolumeContentService(ctx) {
		return
	}
	offset, valid := parseNonNegativeInt64Header(ctx, "Upload-Offset")
	if !valid {
		return
	}
	checksum := strings.TrimSpace(ctx.GetHeader("Upload-Checksum"))
	if ctx.GetHeader("Tus-Resumable") != "1.0.0" || !strings.HasPrefix(checksum, "sha256 ") || ctx.ContentType() != "application/offset+octet-stream" {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "invalid internal TUS upload headers")
		return
	}
	if ctx.Request.ContentLength < 1 || ctx.Request.ContentLength > volumetransferapi.MaximumChunkSize {
		writeErrorCode(ctx, http.StatusRequestEntityTooLarge, volume.CodeInvalidInput, "volume upload chunk exceeds the allowed size")
		return
	}
	nextOffset, chunkSize, err := h.volumeContent.InternalWritePart(
		ctx.Request.Context(), ctx.Param("transferId"), token, offset,
		strings.TrimPrefix(checksum, "sha256 "), ctx.Request.Body, ctx.Request.ContentLength,
	)
	if err != nil {
		if code := volume.ErrorCode(err); code == volume.CodeTransferOffsetMismatch || code == volume.CodeTransferPartInProgress {
			if info, headErr := h.volumeContent.InternalHead(ctx.Request.Context(), ctx.Param("transferId"), token); headErr == nil {
				ctx.Header("Upload-Offset", formatInt64(info.Offset))
				ctx.Header("Upload-Chunk-Size", formatInt64(info.ChunkSize))
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

func (h *Handlers) DownloadInternalVolumeTransferContent(ctx *gin.Context) {
	token, ok := internalVolumeTransferToken(ctx)
	if !ok || !h.ensureVolumeContentService(ctx) {
		return
	}
	download, err := h.volumeContent.InternalOpen(ctx.Request.Context(), ctx.Param("transferId"), token, ctx.GetHeader("Range"))
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	defer download.Body.Close()
	ctx.Header("Accept-Ranges", "bytes")
	if download.ETag != "" {
		ctx.Header("ETag", download.ETag)
	}
	if download.ContentRange != "" {
		ctx.Header("Content-Range", download.ContentRange)
	}
	ctx.DataFromReader(download.Status, download.Size, download.ContentType, download.Body, nil)
}

func (h *Handlers) ReportInternalVolumeTransferProgress(ctx *gin.Context) {
	token, ok := internalVolumeTransferToken(ctx)
	if !ok || !h.ensureVolumeContentService(ctx) {
		return
	}
	var input volumeTransferProgressInput
	if !bindJSON(ctx, &input) || !validInternalTransferState(ctx, input.ExpectedState) {
		return
	}
	if input.TransferredBytes < 0 || input.ProcessedFiles < 0 {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeTransferProgressInvalid, "volume transfer progress cannot be negative")
		return
	}
	if err := h.volumeContent.InternalProgress(ctx.Request.Context(), ctx.Param("transferId"), token, input); err != nil {
		writeVolumeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) CompleteInternalVolumeTransfer(ctx *gin.Context) {
	token, ok := internalVolumeTransferToken(ctx)
	if !ok || !h.ensureVolumeContentService(ctx) {
		return
	}
	var input volumeTransferCompleteInput
	if !bindJSON(ctx, &input) || !validInternalTransferState(ctx, input.ExpectedState) {
		return
	}
	if input.TransferredBytes < 0 || input.LogicalBytes < 0 {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeTransferProgressInvalid, "volume transfer progress cannot be negative")
		return
	}
	transfer, err := h.volumeContent.InternalComplete(ctx.Request.Context(), ctx.Param("transferId"), token, input)
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, volumeTransferResponseFor(transfer, false, h.volumeTransferMaxBytes))
}

func (h *Handlers) FailInternalVolumeTransfer(ctx *gin.Context) {
	token, ok := internalVolumeTransferToken(ctx)
	if !ok || !h.ensureVolumeContentService(ctx) {
		return
	}
	var input volumeTransferFailInput
	if !bindJSON(ctx, &input) || !validInternalTransferState(ctx, input.ExpectedState) {
		return
	}
	if len(input.Diagnostic) > 4096 {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "volume transfer diagnostic is too long")
		return
	}
	transfer, err := h.volumeContent.InternalFail(ctx.Request.Context(), ctx.Param("transferId"), token, input)
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, volumeTransferResponseFor(transfer, false, h.volumeTransferMaxBytes))
}

func (h *Handlers) ensureVolumeContentService(ctx *gin.Context) bool {
	if h != nil && h.volumeContent != nil {
		return true
	}
	writeTransferStoreUnavailable(ctx)
	return false
}

func internalVolumeTransferToken(ctx *gin.Context) (string, bool) {
	authorization := strings.TrimSpace(ctx.GetHeader("Authorization"))
	if !strings.HasPrefix(authorization, "Bearer ") {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.unauthorized", "volume transfer job token is required")
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	if token == "" {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.unauthorized", "volume transfer job token is required")
		return "", false
	}
	return token, true
}

func validInternalTransferState(ctx *gin.Context, state string) bool {
	if state == model.VolumeTransferStateRunning {
		return true
	}
	writeErrorCode(ctx, http.StatusConflict, volume.CodeTransferStateConflict, "volume transfer state changed")
	return false
}

func formatInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}

func parseInt64(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}
