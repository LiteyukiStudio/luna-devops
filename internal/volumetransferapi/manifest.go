package volumetransferapi

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

func (service *Service) OpenManifest(ctx context.Context, actor Actor, transfer model.VolumeTransfer, ticket string, binding DownloadBinding) (Download, error) {
	if err := service.validate(); err != nil {
		return Download{}, err
	}
	if err := service.consumeTicket(ctx, actor, transfer, strings.TrimSpace(ticket), binding); err != nil {
		return Download{}, err
	}
	current, err := service.volumes.GetVolumeTransfer(ctx, transfer.ProjectID, transfer.ID)
	if err != nil {
		return Download{}, err
	}
	manifest, err := blockManifestFor(current)
	if err != nil {
		return Download{}, err
	}
	content, err := json.Marshal(manifest)
	if err != nil {
		return Download{}, domainError(volume.CodeTransferChecksumInvalid, "volume transfer manifest is invalid", err)
	}
	return Download{Body: ioNopCloser{Reader: bytes.NewReader(content)}, ContentType: "application/json; charset=utf-8"}, nil
}

type ioNopCloser struct{ *bytes.Reader }

func (ioNopCloser) Close() error { return nil }

func blockManifestFor(transfer model.VolumeTransfer) (BlockManifest, error) {
	if transfer.Direction != model.VolumeTransferDirectionExport || transfer.Format != model.VolumeTransferFormatRawZST || transfer.State != model.VolumeTransferStateSucceeded {
		return BlockManifest{}, domainError(volume.CodeTransferStateConflict, "volume transfer does not provide a completed block manifest", nil)
	}
	if transfer.LogicalBytes < 1 || !validSHA256(transfer.DataSHA256) || transfer.FinishedAt == nil {
		return BlockManifest{}, domainError(volume.CodeTransferChecksumInvalid, "volume transfer block manifest is incomplete", nil)
	}
	return BlockManifest{
		SchemaVersion: 1, VolumeMode: model.ProjectVolumeModeBlock, Format: transfer.Format,
		ExportedAt: *transfer.FinishedAt, LogicalBytes: transfer.LogicalBytes, FileCount: 0,
		DataSHA256: transfer.DataSHA256, ConsistencyMode: transfer.ConsistencyMode,
	}, nil
}
