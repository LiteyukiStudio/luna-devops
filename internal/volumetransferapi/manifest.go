package volumetransferapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

const blockManifestContentType = "application/json; charset=utf-8"

func (service *Service) HeadManifest(ctx context.Context, actor Actor, transfer model.VolumeTransfer, credential DownloadCredential, binding DownloadBinding) (info ContentInfo, session DownloadSession, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "manifest.head")
	defer func() { end(err) }()
	content, current, exchangeTicket, err := service.authorizeBlockManifest(ctx, actor, transfer, credential, binding)
	if err != nil {
		return ContentInfo{}, DownloadSession{}, err
	}
	if exchangeTicket {
		session, err = service.issueDownloadSession(ctx, current, binding)
		if err != nil {
			return ContentInfo{}, DownloadSession{}, err
		}
	}
	return ContentInfo{Size: int64(len(content)), ETag: blockManifestETag(content)}, session, nil
}

func (service *Service) OpenManifest(ctx context.Context, actor Actor, transfer model.VolumeTransfer, credential DownloadCredential, binding DownloadBinding) (download Download, session DownloadSession, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "manifest.open")
	defer func() { end(err) }()
	content, current, exchangeTicket, err := service.authorizeBlockManifest(ctx, actor, transfer, credential, binding)
	if err != nil {
		return Download{}, DownloadSession{}, err
	}
	if exchangeTicket {
		session, err = service.issueDownloadSession(ctx, current, binding)
		if err != nil {
			return Download{}, DownloadSession{}, err
		}
	}
	return Download{
		Body: io.NopCloser(bytes.NewReader(content)), Status: http.StatusOK,
		ContentType: blockManifestContentType, Size: int64(len(content)), ETag: blockManifestETag(content),
	}, session, nil
}

func (service *Service) authorizeBlockManifest(ctx context.Context, actor Actor, transfer model.VolumeTransfer, credential DownloadCredential, binding DownloadBinding) ([]byte, model.VolumeTransfer, bool, error) {
	current, exchangeTicket, err := service.authorizeDownloadCredential(ctx, actor, transfer, credential, binding)
	if err != nil {
		return nil, model.VolumeTransfer{}, false, err
	}
	manifest, err := blockManifestFor(current)
	if err != nil {
		return nil, model.VolumeTransfer{}, false, err
	}
	content, err := json.Marshal(manifest)
	if err != nil {
		return nil, model.VolumeTransfer{}, false, domainError(volume.CodeTransferChecksumInvalid, "volume transfer manifest is unavailable", err)
	}
	return content, current, exchangeTicket, nil
}

func blockManifestFor(transfer model.VolumeTransfer) (BlockManifest, error) {
	if transfer.Direction != model.VolumeTransferDirectionExport || transfer.State != model.VolumeTransferStateSucceeded ||
		transfer.Format != model.VolumeTransferFormatRawZST {
		return BlockManifest{}, domainError(volume.CodeTransferStateConflict, "volume transfer does not provide a block manifest", nil)
	}
	if transfer.FinishedAt == nil || transfer.FinishedAt.IsZero() || transfer.LogicalBytes < 1 || !validSHA256(strings.ToLower(transfer.DataSHA256)) {
		return BlockManifest{}, domainError(volume.CodeTransferChecksumInvalid, "volume transfer block manifest is incomplete", nil)
	}
	switch transfer.ConsistencyMode {
	case model.VolumeTransferConsistencySnapshot, model.VolumeTransferConsistencyLive, model.VolumeTransferConsistencyUnmounted:
	default:
		return BlockManifest{}, domainError(volume.CodeTransferChecksumInvalid, "volume transfer consistency metadata is invalid", nil)
	}
	return BlockManifest{
		SchemaVersion: 1, VolumeMode: model.ProjectVolumeModeBlock, Format: model.VolumeTransferFormatRawZST,
		ExportedAt: transfer.FinishedAt.UTC(), LogicalBytes: transfer.LogicalBytes, FileCount: 0,
		DataSHA256: strings.ToLower(transfer.DataSHA256), ConsistencyMode: transfer.ConsistencyMode,
	}, nil
}

func blockManifestETag(content []byte) string {
	digest := sha256.Sum256(content)
	return `"` + hex.EncodeToString(digest[:]) + `"`
}
