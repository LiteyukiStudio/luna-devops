package api

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/api/volumeapi"
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
	projectVolumeClusterTimeout      = volumeapi.ProjectVolumeClusterTimeout
	volumeObservationUnavailableCode = volumeapi.VolumeObservationUnavailableCode
	volumeTransferHTTPIdleTimeout    = volumeapi.VolumeTransferHTTPIdleTimeout
)

var errProjectVolumeClusterUnavailable = volumeapi.ErrProjectVolumeClusterUnavailable

type volumeHost struct {
	handlers *Handlers
}

func (host volumeHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}
func (host volumeHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProject(ctx, action)
}
func (host volumeHost) ProjectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool) {
	return host.handlers.projectMemberActionAllowed(ctx, projectID, userID, action)
}
func (host volumeHost) ProjectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return host.handlers.projectRoleActionAllowed(ctx, user, projectID, action)
}
func (host volumeHost) ProjectAuthorizer(ctx context.Context) authz.ProjectAuthorizer {
	return host.handlers.projectAuthorizer(ctx)
}
func (host volumeHost) RuntimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return host.handlers.runtimeClusterForProjectUse(ctx, user, projectID, clusterID)
}
func (host volumeHost) EnsureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	return host.handlers.ensureBillingAllowsDeployChange(ctx, projectID)
}
func (host volumeHost) EnsureBillingAllowsManagedVolumeChange(ctx *gin.Context, projectID string) bool {
	return host.handlers.ensureBillingAllowsManagedVolumeChange(ctx, projectID)
}
func (host volumeHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}
func (host volumeHost) CurrentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool) {
	return currentAccessTokenFromContext(ctx)
}
func (host volumeHost) AccessTokenAllows(scopeText, required string) bool {
	return accessTokenAllows(scopeText, required)
}
func (host volumeHost) RequestUsesBearerToken(ctx *gin.Context) bool {
	return requestUsesBearerToken(ctx)
}
func (host volumeHost) RequireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return host.handlers.requireContinuousAuthorizationBinding(ctx, user)
}
func (host volumeHost) CurrentInteractiveAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return host.handlers.currentInteractiveAuthorizationBinding(ctx, user)
}
func (host volumeHost) MonitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return host.handlers.monitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}
func (host volumeHost) ProjectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return host.handlers.projectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}
func (host volumeHost) ResourceCanMutateDuringDelete(status string) bool {
	return resourceCanMutateDuringDelete(status)
}

func (h *Handlers) volumeAPI() *volumeapi.Handler {
	return volumeapi.New(volumeHost{handlers: h}, volumeapi.Dependencies{
		Volumes:          h.volumes,
		Clusters:         h.volumeClusters,
		Content:          h.volumeContent,
		TransferMaxBytes: h.volumeTransferMaxBytes,
		TransferEnabled:  h.volumeTransferEnabled,
	})
}

func (h *Handlers) ListProjectVolumes(ctx *gin.Context)  { h.volumeAPI().ListProjectVolumes(ctx) }
func (h *Handlers) CreateProjectVolume(ctx *gin.Context) { h.volumeAPI().CreateProjectVolume(ctx) }
func (h *Handlers) GetProjectVolume(ctx *gin.Context)    { h.volumeAPI().GetProjectVolume(ctx) }
func (h *Handlers) UpdateProjectVolume(ctx *gin.Context) { h.volumeAPI().UpdateProjectVolume(ctx) }
func (h *Handlers) DeleteProjectVolume(ctx *gin.Context) { h.volumeAPI().DeleteProjectVolume(ctx) }
func (h *Handlers) RetryProjectVolumeOperation(ctx *gin.Context) {
	h.volumeAPI().RetryProjectVolumeOperation(ctx)
}
func (h *Handlers) PreviewProjectVolumeDeletion(ctx *gin.Context) {
	h.volumeAPI().PreviewProjectVolumeDeletion(ctx)
}
func (h *Handlers) ListProjectVolumeStorageClasses(ctx *gin.Context) {
	h.volumeAPI().ListProjectVolumeStorageClasses(ctx)
}
func (h *Handlers) CreateVolumeImport(ctx *gin.Context) { h.volumeAPI().CreateVolumeImport(ctx) }
func (h *Handlers) UploadVolumeImportContent(ctx *gin.Context) {
	h.volumeAPI().UploadVolumeImportContent(ctx)
}
func (h *Handlers) CreateVolumeExport(ctx *gin.Context)  { h.volumeAPI().CreateVolumeExport(ctx) }
func (h *Handlers) ListVolumeTransfers(ctx *gin.Context) { h.volumeAPI().ListVolumeTransfers(ctx) }
func (h *Handlers) GetVolumeTransfer(ctx *gin.Context)   { h.volumeAPI().GetVolumeTransfer(ctx) }
func (h *Handlers) RetryVolumeTransfer(ctx *gin.Context) { h.volumeAPI().RetryVolumeTransfer(ctx) }
func (h *Handlers) CancelVolumeTransfer(ctx *gin.Context) {
	h.volumeAPI().CancelVolumeTransfer(ctx)
}
func (h *Handlers) AuthorizeVolumeTransferDownload(ctx *gin.Context) {
	h.volumeAPI().AuthorizeVolumeTransferDownload(ctx)
}
func (h *Handlers) DownloadVolumeTransferContent(ctx *gin.Context) {
	h.volumeAPI().DownloadVolumeTransferContent(ctx)
}
func (h *Handlers) DownloadVolumeTransferManifest(ctx *gin.Context) {
	h.volumeAPI().DownloadVolumeTransferManifest(ctx)
}

type projectVolumeStorageClass = volumeapi.ProjectVolumeStorageClass
type projectVolumeClusterService = volumeapi.ProjectVolumeClusterService
type projectVolumeClusterAdapter = volumeapi.ProjectVolumeClusterAdapter
type volumeTaskEnqueuer = volumeapi.VolumeTaskEnqueuer
type projectVolumeSourceInput = volumeapi.ProjectVolumeSourceInput
type projectVolumeCreateInput = volumeapi.ProjectVolumeCreateInput
type projectVolumeUpdateInput = volumeapi.ProjectVolumeUpdateInput
type projectVolumeObservationResponse = volumeapi.ProjectVolumeObservationResponse
type projectVolumeResponse = volumeapi.ProjectVolumeResponse
type projectVolumeBindingResponse = volumeapi.ProjectVolumeBindingResponse
type projectVolumeDetailResponse = volumeapi.ProjectVolumeDetailResponse
type projectVolumeDeletionPreviewResponse = volumeapi.ProjectVolumeDeletionPreviewResponse
type volumeTransferContentService = volumeapi.VolumeTransferContentService
type volumeTransferContentAdapter = volumeapi.VolumeTransferContentAdapter
type volumeTransferRuntimeAdapter = volumeapi.VolumeTransferRuntimeAdapter
type apiVolumeTransferExportStream = volumeapi.APIVolumeTransferExportStream
type volumeImportCreateInput = volumeapi.VolumeImportCreateInput
type volumeExportCreateInput = volumeapi.VolumeExportCreateInput
type volumeTransferResponse = volumeapi.VolumeTransferResponse
type volumeDownloadAuthorizationResponse = volumeapi.VolumeDownloadAuthorizationResponse
type volumeDownloadBinding = volumeapi.VolumeDownloadBinding
type volumeDownload = volumeapi.VolumeDownload
type volumeImportAuthorizationReference = volumeapi.VolumeImportAuthorizationReference
type volumeTransferDownloadAuthorizationReference = volumeapi.VolumeTransferDownloadAuthorizationReference

type volumeOperationDispatcher struct {
	tasks volumeTaskEnqueuer
}

func (dispatcher volumeOperationDispatcher) DispatchVolumeOperation(ctx context.Context, operation volume.VolumeOperation) error {
	return volumeapi.NewVolumeOperationDispatcher(dispatcher.tasks).DispatchVolumeOperation(ctx, operation)
}

func newProjectVolumeClusterAdapter(db *gorm.DB, secrets secret.Store) *projectVolumeClusterAdapter {
	return volumeapi.NewProjectVolumeClusterAdapter(db, secrets)
}

func newVolumeTransferContentAdapter(handlers *Handlers, cfg Config) (*volumeTransferContentAdapter, error) {
	if handlers == nil || handlers.volumes == nil || handlers.rateLimiter == nil || handlers.rateLimiter.redis == nil {
		return nil, errors.New("volume transfer dependencies are unavailable")
	}
	clusterAdapter, ok := handlers.volumeClusters.(*projectVolumeClusterAdapter)
	if !ok || clusterAdapter == nil {
		return nil, errors.New("volume transfer runtime streaming is unavailable")
	}
	return volumeapi.NewVolumeTransferContentAdapter(
		volumeHost{handlers: handlers}, handlers.volumes, clusterAdapter,
		handlers.rateLimiter.redis, cfg.VolumeTransferMaxBytes,
	)
}

func projectVolumeObservationFromProvider(item model.ProjectVolume, observed kubeprovider.ProjectVolumeClaimObservation) projectVolumeObservationResponse {
	return volumeapi.ProjectVolumeObservationFromProvider(item, observed)
}
func unavailableProjectVolumeObservation(code string) projectVolumeObservationResponse {
	return volumeapi.UnavailableProjectVolumeObservation(code)
}
func quantityBytes(value string) (int64, bool) { return volumeapi.QuantityBytes(value) }
func projectVolumeRetryAuthorization(item model.ProjectVolume) (authz.Action, bool) {
	return volumeapi.ProjectVolumeRetryAuthorization(item)
}
func projectVolumeCreateDomainInput(ctx *gin.Context, project model.Project, user model.User, input projectVolumeCreateInput, idempotencyKey string) (volume.CreateProjectVolumeInput, bool) {
	return volumeapi.ProjectVolumeCreateDomainInput(ctx, project, user, input, idempotencyKey)
}
func projectVolumeResponseFor(item model.ProjectVolume) projectVolumeResponse {
	return volumeapi.ProjectVolumeResponseFor(item)
}
func projectVolumeResponseForObservation(item model.ProjectVolume, observation projectVolumeObservationResponse) projectVolumeResponse {
	return volumeapi.ProjectVolumeResponseForObservation(item, observation)
}
func (h *Handlers) observeProjectVolumeResponses(ctx context.Context, items []model.ProjectVolume) map[string]projectVolumeObservationResponse {
	return h.volumeAPI().ObserveProjectVolumeResponses(ctx, items)
}
func projectVolumeBindingResponseFor(item model.DeploymentVolumeMount) projectVolumeBindingResponse {
	return volumeapi.ProjectVolumeBindingResponseFor(item)
}
func projectVolumeDetailResponseFor(detail volume.ProjectVolumeDetail, observation projectVolumeObservationResponse, privileged bool, userID string) projectVolumeDetailResponse {
	return volumeapi.ProjectVolumeDetailResponseFor(detail, observation, privileged, userID)
}
func projectVolumeDeletionPreviewResponseFor(preview volume.ProjectVolumeDeletionPreview, observation projectVolumeObservationResponse, userID string) projectVolumeDeletionPreviewResponse {
	return volumeapi.ProjectVolumeDeletionPreviewResponseFor(preview, observation, userID)
}
func volumePagination(ctx *gin.Context, allowedSort map[string]bool, fallbackSort string) (paginationParams, bool) {
	return volumeapi.VolumePagination(ctx, allowedSort, fallbackSort)
}
func volumeIdempotencyKey(ctx *gin.Context) (string, bool) {
	return volumeapi.VolumeIdempotencyKey(ctx)
}
func volumeRevisionHeader(ctx *gin.Context) (int64, bool) {
	return volumeapi.VolumeRevisionHeader(ctx)
}
func parseVolumeCapacity(raw string) (string, int64, bool) {
	return volumeapi.ParseVolumeCapacity(raw)
}
func projectVolumeCreateAuditAction(input volume.CreateProjectVolumeInput) string {
	return volumeapi.ProjectVolumeCreateAuditAction(input)
}
func volumeAuditErrorCode(err error) string { return volumeapi.VolumeAuditErrorCode(err) }
func writeVolumeError(ctx *gin.Context, err error) {
	volumeapi.WriteVolumeError(ctx, err)
}
func recordProjectVolumeObservationMetrics(ctx context.Context, observations map[string]projectVolumeObservationResponse) {
	volumeapi.RecordProjectVolumeObservationMetrics(ctx, observations)
}
func projectVolumeObservationMetricCode(value string) string {
	return volumeapi.ProjectVolumeObservationMetricCode(value)
}

func apiVolumeTransferStreamResult(result kubeprovider.VolumeTransferStreamResult) volumetransferapi.StreamResult {
	return volumeapi.APIVolumeTransferStreamResult(result)
}
func coreDownloadBinding(binding volumeDownloadBinding) volumetransferapi.DownloadBinding {
	return volumeapi.CoreDownloadBinding(binding)
}
func (h *Handlers) volumeTransferDownloadBinding(ctx *gin.Context, user model.User) (volumeDownloadBinding, bool) {
	return h.volumeAPI().VolumeTransferDownloadBinding(ctx, user)
}
func (h *Handlers) volumeImportAuthorizationAllowed(ctx context.Context, user model.User, reference volumeImportAuthorizationReference) bool {
	return h.volumeAPI().VolumeImportAuthorizationAllowed(ctx, user, reference)
}
func volumeTransferArchiveFilename(transfer model.VolumeTransfer) string {
	return volumeapi.VolumeTransferArchiveFilename(transfer)
}
func (h *Handlers) serveDirectVolumeTransferDownload(ctx *gin.Context, manifest bool) {
	h.volumeAPI().ServeDirectVolumeTransferDownload(ctx, manifest)
}
func (h *Handlers) volumeTransferDownloadAuthorizationAllowed(ctx context.Context, user model.User, reference volumeTransferDownloadAuthorizationReference) bool {
	return h.volumeAPI().VolumeTransferDownloadAuthorizationAllowed(ctx, user, reference)
}
func copyVolumeDownloadBody(ctx context.Context, destination io.Writer, body io.ReadCloser, interrupt func()) error {
	return volumeapi.CopyVolumeDownloadBody(ctx, destination, body, interrupt)
}
func volumeDownloadStreamFailureReason(ctx context.Context, authorizationRevoked <-chan struct{}, streamErr error) string {
	return volumeapi.VolumeDownloadStreamFailureReason(ctx, authorizationRevoked, streamErr)
}
func (h *Handlers) auditVolumeTransferStreamOutcome(ctx context.Context, userID, action, transferID string, success bool, message string) {
	h.volumeAPI().AuditVolumeTransferStreamOutcome(ctx, userID, action, transferID, success, message)
}
func (h *Handlers) volumeTransferForAction(ctx *gin.Context, action authz.Action) (model.User, model.Project, model.VolumeTransfer, bool) {
	return h.volumeAPI().VolumeTransferForAction(ctx, action)
}
func (h *Handlers) ensureVolumeTransferConfigured(ctx *gin.Context) bool {
	return h.volumeAPI().EnsureVolumeTransferConfigured(ctx)
}
func (h *Handlers) authorizeTransferDirection(ctx *gin.Context, user model.User, project model.Project, transfer model.VolumeTransfer) bool {
	return h.volumeAPI().AuthorizeTransferDirection(ctx, user, project, transfer)
}
func volumeTransferResponseFor(item model.VolumeTransfer, includeFilename bool, maxBytes ...int64) volumeTransferResponse {
	return volumeapi.VolumeTransferResponseFor(item, includeFilename, maxBytes...)
}
func writeTransferUnavailable(ctx *gin.Context) { volumeapi.WriteTransferUnavailable(ctx) }

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

type volumeTransferDeadlineController interface {
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

type volumeTransferDeadlineReader struct {
	reader     io.Reader
	controller volumeTransferDeadlineController
	timeout    time.Duration
}

func (reader *volumeTransferDeadlineReader) Read(buffer []byte) (int, error) {
	if reader.controller != nil {
		_ = reader.controller.SetReadDeadline(time.Now().Add(reader.timeout))
	}
	return reader.reader.Read(buffer)
}
func (reader *volumeTransferDeadlineReader) Close() error {
	if closer, ok := reader.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

type volumeTransferDeadlineWriter struct {
	writer     io.Writer
	controller volumeTransferDeadlineController
	timeout    time.Duration
}

func (writer *volumeTransferDeadlineWriter) Write(buffer []byte) (int, error) {
	if writer.controller != nil {
		_ = writer.controller.SetWriteDeadline(time.Now().Add(writer.timeout))
	}
	return writer.writer.Write(buffer)
}
