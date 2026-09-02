package volumeapi

import (
	"context"
	"io"

	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/LiteyukiStudio/devops/internal/volumetransferapi"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ProjectVolumeClusterTimeout      = projectVolumeClusterTimeout
	VolumeObservationUnavailableCode = volumeObservationUnavailableCode
	VolumeTransferHTTPIdleTimeout    = volumeTransferHTTPIdleTimeout
)

var ErrProjectVolumeClusterUnavailable = errProjectVolumeClusterUnavailable

type ProjectVolumeStorageClass = projectVolumeStorageClass
type ProjectVolumeClusterService = projectVolumeClusterService
type ProjectVolumeClusterAdapter = projectVolumeClusterAdapter
type VolumeTaskEnqueuer = volumeTaskEnqueuer
type VolumeOperationDispatcher = volumeOperationDispatcher
type ProjectVolumeSourceInput = projectVolumeSourceInput
type ProjectVolumeCreateInput = projectVolumeCreateInput
type ProjectVolumeUpdateInput = projectVolumeUpdateInput
type ProjectVolumeObservationResponse = projectVolumeObservationResponse
type ProjectVolumeResponse = projectVolumeResponse
type ProjectVolumeBindingResponse = projectVolumeBindingResponse
type ProjectVolumeDetailResponse = projectVolumeDetailResponse
type ProjectVolumeDeletionPreviewResponse = projectVolumeDeletionPreviewResponse
type VolumeTransferContentService = volumeTransferContentService
type VolumeTransferContentAdapter = volumeTransferContentAdapter
type VolumeTransferRuntimeAdapter = volumeTransferRuntimeAdapter
type APIVolumeTransferExportStream = apiVolumeTransferExportStream
type VolumeImportCreateInput = volumeImportCreateInput
type VolumeExportCreateInput = volumeExportCreateInput
type VolumeTransferResponse = volumeTransferResponse
type VolumeDownloadAuthorizationResponse = volumeDownloadAuthorizationResponse
type VolumeDownloadBinding = projectapi.ContinuousAuthorizationBinding
type VolumeDownload = volumeDownload
type VolumeImportAuthorizationReference = volumeImportAuthorizationReference
type VolumeTransferDownloadAuthorizationReference = volumeTransferDownloadAuthorizationReference

func NewProjectVolumeClusterAdapter(db *gorm.DB, secrets secret.Store) *ProjectVolumeClusterAdapter {
	return newProjectVolumeClusterAdapter(db, secrets)
}

func NewVolumeOperationDispatcher(tasks VolumeTaskEnqueuer) VolumeOperationDispatcher {
	return volumeOperationDispatcher{tasks: tasks}
}

func ProjectVolumeObservationFromProvider(item model.ProjectVolume, observed kubeprovider.ProjectVolumeClaimObservation) ProjectVolumeObservationResponse {
	return projectVolumeObservationFromProvider(item, observed)
}

func UnavailableProjectVolumeObservation(code string) ProjectVolumeObservationResponse {
	return unavailableProjectVolumeObservation(code)
}

func QuantityBytes(value string) (int64, bool) { return quantityBytes(value) }

func ProjectVolumeRetryAuthorization(item model.ProjectVolume) (authz.Action, bool) {
	return projectVolumeRetryAuthorization(item)
}

func ProjectVolumeCreateDomainInput(ctx *gin.Context, project model.Project, user model.User, input ProjectVolumeCreateInput, idempotencyKey string) (volume.CreateProjectVolumeInput, bool) {
	return projectVolumeCreateDomainInput(ctx, project, user, input, idempotencyKey)
}

func ProjectVolumeResponseFor(item model.ProjectVolume) ProjectVolumeResponse {
	return projectVolumeResponseFor(item)
}

func ProjectVolumeResponseForObservation(item model.ProjectVolume, observation ProjectVolumeObservationResponse) ProjectVolumeResponse {
	return projectVolumeResponseForObservation(item, observation)
}

func ProjectVolumeBindingResponseFor(item model.DeploymentVolumeMount) ProjectVolumeBindingResponse {
	return projectVolumeBindingResponseFor(item)
}

func ProjectVolumeDetailResponseFor(detail volume.ProjectVolumeDetail, observation ProjectVolumeObservationResponse, privileged bool, userID string) ProjectVolumeDetailResponse {
	return projectVolumeDetailResponseFor(detail, observation, privileged, userID)
}

func ProjectVolumeDeletionPreviewResponseFor(preview volume.ProjectVolumeDeletionPreview, observation ProjectVolumeObservationResponse, userID string) ProjectVolumeDeletionPreviewResponse {
	return projectVolumeDeletionPreviewResponseFor(preview, observation, userID)
}

func VolumePagination(ctx *gin.Context, allowedSort map[string]bool, fallbackSort string) (paginationParams, bool) {
	return volumePagination(ctx, allowedSort, fallbackSort)
}

func VolumeIdempotencyKey(ctx *gin.Context) (string, bool) { return volumeIdempotencyKey(ctx) }
func VolumeRevisionHeader(ctx *gin.Context) (int64, bool)  { return volumeRevisionHeader(ctx) }
func ParseVolumeCapacity(raw string) (string, int64, bool) { return parseVolumeCapacity(raw) }

func ProjectVolumeCreateAuditAction(input volume.CreateProjectVolumeInput) string {
	return projectVolumeCreateAuditAction(input)
}

func VolumeAuditErrorCode(err error) string { return volumeAuditErrorCode(err) }
func WriteVolumeError(ctx *gin.Context, err error) {
	writeVolumeError(ctx, err)
}

func RecordProjectVolumeObservationMetrics(ctx context.Context, observations map[string]ProjectVolumeObservationResponse) {
	recordProjectVolumeObservationMetrics(ctx, observations)
}

func ProjectVolumeObservationMetricCode(value string) string {
	return projectVolumeObservationMetricCode(value)
}

func APIVolumeTransferStreamResult(result kubeprovider.VolumeTransferStreamResult) volumetransferapi.StreamResult {
	return apiVolumeTransferStreamResult(result)
}

func CoreDownloadBinding(binding VolumeDownloadBinding) volumetransferapi.DownloadBinding {
	return coreDownloadBinding(binding)
}

func VolumeTransferArchiveFilename(transfer model.VolumeTransfer) string {
	return volumeTransferArchiveFilename(transfer)
}

func CopyVolumeDownloadBody(ctx context.Context, destination io.Writer, body io.ReadCloser, interrupt func()) error {
	return copyVolumeDownloadBody(ctx, destination, body, interrupt)
}

func VolumeDownloadStreamFailureReason(ctx context.Context, authorizationRevoked <-chan struct{}, streamErr error) string {
	return volumeDownloadStreamFailureReason(ctx, authorizationRevoked, streamErr)
}

func VolumeTransferResponseFor(item model.VolumeTransfer, includeFilename bool, maxBytes ...int64) VolumeTransferResponse {
	return volumeTransferResponseFor(item, includeFilename, maxBytes...)
}

func WriteTransferUnavailable(ctx *gin.Context) { writeTransferUnavailable(ctx) }

func (h *Handler) ObserveProjectVolumeResponses(ctx context.Context, items []model.ProjectVolume) map[string]ProjectVolumeObservationResponse {
	return h.observeProjectVolumeResponses(ctx, items)
}

func (h *Handler) VolumeTransferDownloadBinding(ctx *gin.Context, user model.User) (VolumeDownloadBinding, bool) {
	return h.volumeTransferDownloadBinding(ctx, user)
}

func (h *Handler) VolumeImportAuthorizationAllowed(ctx context.Context, user model.User, reference VolumeImportAuthorizationReference) bool {
	return h.volumeImportAuthorizationAllowed(ctx, user, reference)
}

func (h *Handler) ServeDirectVolumeTransferDownload(ctx *gin.Context, manifest bool) {
	h.serveDirectVolumeTransferDownload(ctx, manifest)
}

func (h *Handler) VolumeTransferDownloadAuthorizationAllowed(ctx context.Context, user model.User, reference VolumeTransferDownloadAuthorizationReference) bool {
	return h.volumeTransferDownloadAuthorizationAllowed(ctx, user, reference)
}

func (h *Handler) AuditVolumeTransferStreamOutcome(ctx context.Context, userID, action, transferID string, success bool, message string) {
	h.auditVolumeTransferStreamOutcome(ctx, userID, action, transferID, success, message)
}

func (h *Handler) VolumeTransferForAction(ctx *gin.Context, action authz.Action) (model.User, model.Project, model.VolumeTransfer, bool) {
	return h.volumeTransferForAction(ctx, action)
}

func (h *Handler) EnsureVolumeTransferConfigured(ctx *gin.Context) bool {
	return h.ensureVolumeTransferConfigured(ctx)
}

func (h *Handler) AuthorizeTransferDirection(ctx *gin.Context, user model.User, project model.Project, transfer model.VolumeTransfer) bool {
	return h.authorizeTransferDirection(ctx, user, project, transfer)
}
