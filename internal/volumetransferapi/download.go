package volumetransferapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/provider/volumestore"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

const (
	downloadTicketKeyPrefix  = "volume_transfer:download_ticket:"
	downloadSessionKeyPrefix = "volume_transfer:download_session:"
)

type downloadCredentialPayload struct {
	ProjectID  string          `json:"projectId"`
	TransferID string          `json:"transferId"`
	Binding    DownloadBinding `json:"binding"`
	ExpiresAt  time.Time       `json:"expiresAt"`
}

func (service *Service) AuthorizeDownload(ctx context.Context, actor Actor, transfer model.VolumeTransfer, binding DownloadBinding) (authorization DownloadAuthorization, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "download.authorize")
	defer func() { end(err) }()
	if err = service.validate(); err != nil {
		return DownloadAuthorization{}, err
	}
	current, err := service.getPublicTransfer(ctx, transfer.ProjectID, transfer.ID, actor)
	if err != nil {
		return DownloadAuthorization{}, err
	}
	if err = service.validateDownloadable(current); err != nil {
		return DownloadAuthorization{}, err
	}
	binding = normalizeDownloadBinding(binding)
	if err = validateDownloadBinding(binding, service.now().UTC()); err != nil {
		return DownloadAuthorization{}, err
	}

	expiresAt := service.now().UTC().Add(service.ticketTTL)
	if binding.Deadline.Before(expiresAt) {
		expiresAt = binding.Deadline
	}
	if current.ExpiresAt.Before(expiresAt) {
		expiresAt = current.ExpiresAt
	}
	if !expiresAt.After(service.now().UTC()) {
		return DownloadAuthorization{}, domainError(volume.CodeTransferExpired, "volume transfer download authorization has expired", nil)
	}
	rawTicket, err := randomOpaqueToken("vdt_", 32)
	if err != nil {
		return DownloadAuthorization{}, domainError(volume.CodeTransferStoreUnavailable, "volume transfer download authorization is unavailable", err)
	}
	payload, err := json.Marshal(downloadCredentialPayload{
		ProjectID: current.ProjectID, TransferID: current.ID, Binding: binding, ExpiresAt: expiresAt,
	})
	if err != nil {
		return DownloadAuthorization{}, domainError(volume.CodeTransferStoreUnavailable, "volume transfer download authorization is unavailable", err)
	}
	if err = service.tickets.Put(ctx, downloadTicketKey(rawTicket), payload, expiresAt.Sub(service.now().UTC())); err != nil {
		return DownloadAuthorization{}, domainError(volume.CodeTransferStoreUnavailable, "volume transfer download authorization is unavailable", err)
	}
	return DownloadAuthorization{Ticket: rawTicket, ExpiresAt: expiresAt}, nil
}

func (service *Service) HeadDownload(ctx context.Context, actor Actor, transfer model.VolumeTransfer, credential DownloadCredential, binding DownloadBinding) (info ContentInfo, session DownloadSession, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "download.head")
	defer func() { end(err) }()
	current, exchangeTicket, err := service.authorizeDownloadCredential(ctx, actor, transfer, credential, binding)
	if err != nil {
		return ContentInfo{}, DownloadSession{}, err
	}
	object, err := service.store.Head(ctx, current.ObjectKey)
	if err != nil {
		return ContentInfo{}, DownloadSession{}, storeError("read volume transfer content metadata", err)
	}
	if object.Size != current.ExpectedBytes || object.Size < 1 {
		return ContentInfo{}, DownloadSession{}, domainError(volume.CodeTransferChecksumMismatch, "volume transfer content metadata does not match", nil)
	}
	if exchangeTicket {
		session, err = service.issueDownloadSession(ctx, current, binding)
		if err != nil {
			return ContentInfo{}, DownloadSession{}, err
		}
	}
	return ContentInfo{Size: object.Size, ETag: object.ETag}, session, nil
}

func (service *Service) OpenDownload(ctx context.Context, actor Actor, transfer model.VolumeTransfer, credential DownloadCredential, rangeHeader string, binding DownloadBinding) (download Download, session DownloadSession, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "download.open")
	defer func() { end(err) }()
	current, exchangeTicket, err := service.authorizeDownloadCredential(ctx, actor, transfer, credential, binding)
	if err != nil {
		return Download{}, DownloadSession{}, err
	}
	download, err = service.openStoredContent(ctx, current, rangeHeader)
	if err != nil {
		return Download{}, DownloadSession{}, err
	}
	if exchangeTicket {
		session, err = service.issueDownloadSession(ctx, current, binding)
		if err != nil {
			_ = download.Body.Close()
			return Download{}, DownloadSession{}, err
		}
	}
	return download, session, nil
}

func (service *Service) authorizeDownloadCredential(ctx context.Context, actor Actor, transfer model.VolumeTransfer, credential DownloadCredential, binding DownloadBinding) (model.VolumeTransfer, bool, error) {
	credential.Ticket = strings.TrimSpace(credential.Ticket)
	credential.Session = strings.TrimSpace(credential.Session)
	if credential.Ticket != "" {
		current, err := service.consumeDownloadTicket(ctx, actor, transfer, credential.Ticket, binding)
		return current, err == nil, err
	}
	if credential.Session == "" {
		return model.VolumeTransfer{}, false, downloadUnauthorized("volume transfer download credential is required")
	}
	current, err := service.validateDownloadSession(ctx, actor, transfer, credential.Session, binding)
	return current, false, err
}

func (service *Service) consumeDownloadTicket(ctx context.Context, actor Actor, transfer model.VolumeTransfer, rawTicket string, binding DownloadBinding) (model.VolumeTransfer, error) {
	if err := service.validate(); err != nil {
		return model.VolumeTransfer{}, err
	}
	rawTicket = strings.TrimSpace(rawTicket)
	if !strings.HasPrefix(rawTicket, "vdt_") || len(rawTicket) < 32 || len(rawTicket) > 256 {
		return model.VolumeTransfer{}, downloadUnauthorized("volume transfer download ticket is invalid")
	}
	payloadBytes, found, err := service.tickets.Consume(ctx, downloadTicketKey(rawTicket))
	if err != nil {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferStoreUnavailable, "volume transfer download authorization is unavailable", err)
	}
	if !found {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferExpired, "volume transfer download authorization is invalid or expired", nil)
	}
	var payload downloadCredentialPayload
	if json.Unmarshal(payloadBytes, &payload) != nil || !payload.ExpiresAt.After(service.now().UTC()) {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferExpired, "volume transfer download authorization is invalid or expired", nil)
	}
	binding = normalizeDownloadBinding(binding)
	if !downloadBindingMatches(payload.Binding, binding, service.now().UTC()) ||
		!constantTimeTextEqual(payload.ProjectID, transfer.ProjectID) ||
		!constantTimeTextEqual(payload.TransferID, transfer.ID) {
		return model.VolumeTransfer{}, downloadUnauthorized("volume transfer download ticket binding is invalid")
	}
	current, err := service.getPublicTransfer(ctx, payload.ProjectID, payload.TransferID, actor)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if err := service.validateDownloadable(current); err != nil {
		return model.VolumeTransfer{}, err
	}
	return current, nil
}

func (service *Service) issueDownloadSession(ctx context.Context, transfer model.VolumeTransfer, binding DownloadBinding) (DownloadSession, error) {
	now := service.now().UTC()
	expiresAt := now.Add(service.sessionTTL)
	if binding.Deadline.Before(expiresAt) {
		expiresAt = binding.Deadline
	}
	if transfer.ExpiresAt.Before(expiresAt) {
		expiresAt = transfer.ExpiresAt
	}
	if !expiresAt.After(now) {
		return DownloadSession{}, domainError(volume.CodeTransferExpired, "volume transfer download session has expired", nil)
	}
	rawSession, err := randomOpaqueToken("vds_", 32)
	if err != nil {
		return DownloadSession{}, domainError(volume.CodeTransferStoreUnavailable, "volume transfer download session is unavailable", err)
	}
	payload, err := json.Marshal(downloadCredentialPayload{
		ProjectID: transfer.ProjectID, TransferID: transfer.ID,
		Binding: normalizeDownloadBinding(binding), ExpiresAt: expiresAt,
	})
	if err != nil {
		return DownloadSession{}, domainError(volume.CodeTransferStoreUnavailable, "volume transfer download session is unavailable", err)
	}
	if err = service.tickets.Put(ctx, downloadSessionKey(rawSession), payload, expiresAt.Sub(now)); err != nil {
		return DownloadSession{}, domainError(volume.CodeTransferStoreUnavailable, "volume transfer download session is unavailable", err)
	}
	return DownloadSession{Token: rawSession, ExpiresAt: expiresAt}, nil
}

func (service *Service) validateDownloadSession(ctx context.Context, actor Actor, transfer model.VolumeTransfer, rawSession string, binding DownloadBinding) (model.VolumeTransfer, error) {
	if err := service.validate(); err != nil {
		return model.VolumeTransfer{}, err
	}
	rawSession = strings.TrimSpace(rawSession)
	if !strings.HasPrefix(rawSession, "vds_") || len(rawSession) < 32 || len(rawSession) > 256 {
		return model.VolumeTransfer{}, downloadUnauthorized("volume transfer download session is invalid")
	}
	payloadBytes, found, err := service.tickets.Get(ctx, downloadSessionKey(rawSession))
	if err != nil {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferStoreUnavailable, "volume transfer download session is unavailable", err)
	}
	if !found {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferExpired, "volume transfer download session has expired", nil)
	}
	var payload downloadCredentialPayload
	if json.Unmarshal(payloadBytes, &payload) != nil || !payload.ExpiresAt.After(service.now().UTC()) {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferExpired, "volume transfer download session has expired", nil)
	}
	binding = normalizeDownloadBinding(binding)
	if !downloadBindingMatches(payload.Binding, binding, service.now().UTC()) ||
		!constantTimeTextEqual(payload.ProjectID, transfer.ProjectID) ||
		!constantTimeTextEqual(payload.TransferID, transfer.ID) {
		return model.VolumeTransfer{}, downloadUnauthorized("volume transfer download session binding is invalid")
	}
	current, err := service.getPublicTransfer(ctx, payload.ProjectID, payload.TransferID, actor)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if err := service.validateDownloadable(current); err != nil {
		return model.VolumeTransfer{}, err
	}
	return current, nil
}

func downloadUnauthorized(message string) error {
	return domainError(volume.CodeTransferDownloadUnauthorized, message, nil)
}

func (service *Service) validateDownloadable(transfer model.VolumeTransfer) error {
	if transfer.Direction != model.VolumeTransferDirectionExport || transfer.State != model.VolumeTransferStateSucceeded {
		return domainError(volume.CodeTransferStateConflict, "volume transfer content is not available", nil)
	}
	if transfer.ObjectDeletedAt != nil || !transfer.ExpiresAt.After(service.now().UTC()) {
		return domainError(volume.CodeTransferExpired, "volume transfer content has expired", nil)
	}
	if transfer.ObjectKey == "" || transfer.ExpectedBytes < 1 || !validSHA256(transfer.SHA256) {
		return domainError(volume.CodeTransferChecksumInvalid, "volume transfer content is incomplete", nil)
	}
	return nil
}

func (service *Service) openStoredContent(ctx context.Context, transfer model.VolumeTransfer, rangeHeader string) (Download, error) {
	object, err := service.store.Head(ctx, transfer.ObjectKey)
	if err != nil {
		return Download{}, storeError("read volume transfer content metadata", err)
	}
	if object.Size < 1 || object.Size != transfer.ExpectedBytes {
		return Download{}, domainError(volume.CodeTransferChecksumMismatch, "volume transfer content metadata does not match", nil)
	}
	offset, length, status, contentRange, err := parseByteRange(rangeHeader, object.Size)
	if err != nil {
		return Download{}, err
	}
	body, err := service.store.ReadRange(ctx, transfer.ObjectKey, offset, length)
	if err != nil {
		return Download{}, storeError("read volume transfer content", err)
	}
	return Download{
		Body: body, Status: status, ContentType: transferContentType(transfer.Format),
		Size: length, ETag: object.ETag, ContentRange: contentRange,
	}, nil
}

func (service *Service) verifyStoredObjectSize(ctx context.Context, transfer model.VolumeTransfer, expectedBytes int64) (volumestore.ObjectInfo, error) {
	if err := service.validate(); err != nil {
		return volumestore.ObjectInfo{}, err
	}
	if transfer.ObjectKey == "" || expectedBytes < 1 || expectedBytes > service.maxBytes {
		return volumestore.ObjectInfo{}, domainError(volume.CodeTransferChecksumInvalid, "volume transfer object verification metadata is invalid", nil)
	}
	info, err := service.store.Head(ctx, transfer.ObjectKey)
	if err != nil {
		return volumestore.ObjectInfo{}, storeError("read volume transfer content metadata", err)
	}
	if info.Size != expectedBytes {
		return volumestore.ObjectInfo{}, domainError(volume.CodeTransferChecksumMismatch, "volume transfer content length does not match", nil)
	}
	return info, nil
}

func parseByteRange(header string, size int64) (offset, length int64, status int, contentRange string, err error) {
	header = strings.TrimSpace(header)
	if size < 1 {
		return 0, 0, 0, "", domainError(volume.CodeTransferProgressInvalid, "volume transfer content range is invalid", nil)
	}
	if header == "" {
		return 0, size, http.StatusOK, "", nil
	}
	if !strings.HasPrefix(header, "bytes=") || strings.Contains(header, ",") {
		return 0, 0, 0, "", domainError(volume.CodeTransferProgressInvalid, "volume transfer content range is invalid", nil)
	}
	value := strings.TrimSpace(strings.TrimPrefix(header, "bytes="))
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, 0, "", domainError(volume.CodeTransferProgressInvalid, "volume transfer content range is invalid", nil)
	}
	if parts[0] == "" {
		suffix, parseErr := strconv.ParseInt(parts[1], 10, 64)
		if parseErr != nil || suffix < 1 {
			return 0, 0, 0, "", domainError(volume.CodeTransferProgressInvalid, "volume transfer content range is invalid", nil)
		}
		if suffix > size {
			suffix = size
		}
		offset, length = size-suffix, suffix
	} else {
		start, parseErr := strconv.ParseInt(parts[0], 10, 64)
		if parseErr != nil || start < 0 || start >= size {
			return 0, 0, 0, "", domainError(volume.CodeTransferProgressInvalid, "volume transfer content range is invalid", nil)
		}
		end := size - 1
		if parts[1] != "" {
			end, parseErr = strconv.ParseInt(parts[1], 10, 64)
			if parseErr != nil || end < start {
				return 0, 0, 0, "", domainError(volume.CodeTransferProgressInvalid, "volume transfer content range is invalid", nil)
			}
			if end >= size {
				end = size - 1
			}
		}
		offset, length = start, end-start+1
	}
	return offset, length, http.StatusPartialContent, fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, size), nil
}

func transferContentType(format string) string {
	if format == model.VolumeTransferFormatRawZST {
		return "application/zstd"
	}
	return "application/gzip"
}

func normalizeDownloadBinding(binding DownloadBinding) DownloadBinding {
	binding.UserID = strings.TrimSpace(binding.UserID)
	binding.SubjectID = strings.TrimSpace(binding.SubjectID)
	binding.AssertionID = strings.TrimSpace(binding.AssertionID)
	binding.Deadline = binding.Deadline.UTC()
	return binding
}

func validateDownloadBinding(binding DownloadBinding, now time.Time) error {
	if binding.UserID == "" || binding.SubjectID == "" || !binding.Deadline.After(now) ||
		(binding.AssertionRequired && binding.AssertionID == "") || (!binding.AssertionRequired && binding.AssertionID != "") {
		return domainError(volume.CodeTransferExpired, "volume transfer download identity is invalid or expired", nil)
	}
	return nil
}

func downloadBindingMatches(expected, current DownloadBinding, now time.Time) bool {
	expected = normalizeDownloadBinding(expected)
	current = normalizeDownloadBinding(current)
	if validateDownloadBinding(expected, now) != nil || validateDownloadBinding(current, now) != nil {
		return false
	}
	return expected.AssertionRequired == current.AssertionRequired &&
		constantTimeTextEqual(expected.UserID, current.UserID) &&
		constantTimeTextEqual(expected.SubjectID, current.SubjectID) &&
		constantTimeTextEqual(expected.AssertionID, current.AssertionID)
}

func randomOpaqueToken(prefix string, bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buffer), nil
}

func downloadTicketKey(rawTicket string) string {
	sum := sha256.Sum256([]byte(rawTicket))
	return downloadTicketKeyPrefix + hex.EncodeToString(sum[:])
}

func downloadSessionKey(rawSession string) string {
	sum := sha256.Sum256([]byte(rawSession))
	return downloadSessionKeyPrefix + hex.EncodeToString(sum[:])
}

func constantTimeTextEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
