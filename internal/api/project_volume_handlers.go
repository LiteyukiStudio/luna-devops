package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"k8s.io/apimachinery/pkg/api/resource"
)

const volumeObservationUnavailableCode = "volume.cluster_unavailable"

type volumeOperationDispatcher struct {
	tasks volumeTaskEnqueuer
}

type volumeTaskEnqueuer interface {
	EnqueueVolumeProvision(context.Context, tasks.VolumeProvisionPayload) (*asynq.TaskInfo, error)
	EnqueueVolumeImport(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error)
	EnqueueVolumeExport(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error)
	EnqueueVolumeTransferCleanup(context.Context, tasks.VolumeTransferCleanupPayload) (*asynq.TaskInfo, error)
	EnqueueVolumeDelete(context.Context, tasks.VolumeDeletePayload) (*asynq.TaskInfo, error)
}

func (dispatcher volumeOperationDispatcher) DispatchVolumeOperation(ctx context.Context, operation volume.VolumeOperation) error {
	if dispatcher.tasks == nil {
		return errors.New("volume task queue is unavailable")
	}
	switch operation.Kind {
	case volume.OperationProvision, volume.OperationExpand:
		_, err := dispatcher.tasks.EnqueueVolumeProvision(ctx, tasks.VolumeProvisionPayload{
			VolumeID: operation.VolumeID, ProjectID: operation.ProjectID, ActorID: operation.ActorID, Operation: operation.Kind,
		})
		return err
	case volume.OperationImport:
		_, err := dispatcher.tasks.EnqueueVolumeImport(ctx, tasks.VolumeTransferPayload{
			TransferID: operation.TransferID, VolumeID: operation.VolumeID, ProjectID: operation.ProjectID, ActorID: operation.ActorID,
		})
		return err
	case volume.OperationExport:
		_, err := dispatcher.tasks.EnqueueVolumeExport(ctx, tasks.VolumeTransferPayload{
			TransferID: operation.TransferID, VolumeID: operation.VolumeID, ProjectID: operation.ProjectID, ActorID: operation.ActorID,
		})
		return err
	case volume.OperationCleanup:
		_, err := dispatcher.tasks.EnqueueVolumeTransferCleanup(ctx, tasks.VolumeTransferCleanupPayload{
			TransferID: operation.TransferID, ActorID: operation.ActorID,
		})
		return err
	case volume.OperationDelete:
		_, err := dispatcher.tasks.EnqueueVolumeDelete(ctx, tasks.VolumeDeletePayload{
			VolumeID: operation.VolumeID, ProjectID: operation.ProjectID, ActorID: operation.ActorID,
		})
		return err
	default:
		return errors.New("unsupported volume operation")
	}
}

type projectVolumeSourceInput struct {
	Type          string `json:"type" binding:"required"`
	ClaimName     string `json:"claimName"`
	OwnershipMode string `json:"ownershipMode"`
	SnapshotName  string `json:"snapshotName"`
}

type projectVolumeCreateInput struct {
	DisplayName      string                   `json:"displayName" binding:"required"`
	ClusterID        string                   `json:"clusterId" binding:"required"`
	Capacity         string                   `json:"capacity"`
	StorageClassName string                   `json:"storageClassName"`
	AccessMode       string                   `json:"accessMode"`
	VolumeMode       string                   `json:"volumeMode"`
	Source           projectVolumeSourceInput `json:"source" binding:"required"`
}

type projectVolumeUpdateInput struct {
	DisplayName *string `json:"displayName"`
	Capacity    *string `json:"capacity"`
}

type projectVolumeObservationResponse struct {
	Status          string    `json:"status"`
	Exists          bool      `json:"exists"`
	Phase           string    `json:"phase"`
	Capacity        string    `json:"capacity"`
	StorageClass    string    `json:"storageClassName"`
	AccessModes     []string  `json:"accessModes"`
	VolumeMode      string    `json:"volumeMode"`
	BoundVolumeName string    `json:"boundVolumeName"`
	ObservedAt      time.Time `json:"observedAt"`
	ObservationCode string    `json:"observationCode"`
}

type projectVolumeResponse struct {
	ID                       string                            `json:"id"`
	ProjectID                string                            `json:"projectId"`
	DisplayName              string                            `json:"displayName"`
	ClusterID                string                            `json:"clusterId"`
	Namespace                string                            `json:"namespace"`
	ClaimName                string                            `json:"claimName"`
	OwnershipMode            string                            `json:"ownershipMode"`
	SourceKind               string                            `json:"sourceKind"`
	SourceSnapshotName       string                            `json:"sourceSnapshotName,omitempty"`
	LifecycleState           string                            `json:"lifecycleState"`
	PendingOperation         string                            `json:"pendingOperation,omitempty"`
	Availability             string                            `json:"availability"`
	Capacity                 string                            `json:"capacity"`
	CapacityBytes            int64                             `json:"capacityBytes"`
	StorageClassName         string                            `json:"storageClassName"`
	AccessMode               string                            `json:"accessMode"`
	VolumeMode               string                            `json:"volumeMode"`
	SourceApplicationID      *string                           `json:"sourceApplicationId,omitempty"`
	SourceApplicationName    string                            `json:"sourceApplicationName,omitempty"`
	SourceDeploymentTargetID *string                           `json:"sourceDeploymentTargetId,omitempty"`
	BindingSummary           model.ProjectVolumeBindingSummary `json:"bindingSummary"`
	Revision                 int64                             `json:"revision"`
	LastErrorCode            string                            `json:"lastErrorCode,omitempty"`
	Observation              projectVolumeObservationResponse  `json:"observation"`
	CreatedAt                time.Time                         `json:"createdAt"`
	UpdatedAt                time.Time                         `json:"updatedAt"`
}

type projectVolumeBindingResponse struct {
	ID                 string    `json:"id"`
	ApplicationID      string    `json:"applicationId"`
	DeploymentTargetID string    `json:"deploymentTargetId"`
	LogicalName        string    `json:"logicalName"`
	SourceType         string    `json:"sourceType"`
	MountPath          *string   `json:"mountPath,omitempty"`
	DevicePath         *string   `json:"devicePath,omitempty"`
	ReadOnly           bool      `json:"readOnly"`
	ActivationState    string    `json:"activationState"`
	LastErrorCode      string    `json:"lastErrorCode,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type projectVolumeDetailResponse struct {
	projectVolumeResponse
	Bindings           []projectVolumeBindingResponse `json:"bindings"`
	BindingPage        int                            `json:"bindingPage"`
	BindingPageSize    int                            `json:"bindingPageSize"`
	BindingTotal       int64                          `json:"bindingTotal"`
	BindingTotalPages  int                            `json:"bindingTotalPages"`
	RecentTransfers    []volumeTransferResponse       `json:"recentTransfers"`
	TransferPage       int                            `json:"transferPage"`
	TransferPageSize   int                            `json:"transferPageSize"`
	TransferTotal      int64                          `json:"transferTotal"`
	TransferTotalPages int                            `json:"transferTotalPages"`
}

type projectVolumeDeletionPreviewResponse struct {
	VolumeID                  string                           `json:"volumeId"`
	OwnershipMode             string                           `json:"ownershipMode"`
	DataAction                string                           `json:"dataAction"`
	HasActiveBindings         bool                             `json:"hasActiveBindings"`
	HasRunningTransfers       bool                             `json:"hasRunningTransfers"`
	Bindings                  []projectVolumeBindingResponse   `json:"bindings"`
	RunningTransfers          []volumeTransferResponse         `json:"runningTransfers"`
	UnderlyingClaimWillDelete bool                             `json:"underlyingClaimWillBeDeleted"`
	Observation               projectVolumeObservationResponse `json:"observation"`
}

func (h *Handlers) ListProjectVolumes(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	if _, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeRead)...); ok {
		pagination, valid := volumePagination(ctx, map[string]bool{
			"createdAt": true, "updatedAt": true, "displayName": true, "capacity": true,
		}, "createdAt")
		if !valid {
			return
		}
		result, err := h.volumes.ListProjectVolumes(ctx.Request.Context(), project.ID, volume.ProjectVolumeListOptions{
			Page: pagination.Page, PageSize: pagination.PageSize, SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
			Search: strings.TrimSpace(ctx.Query("search")), Availability: strings.TrimSpace(ctx.Query("availability")),
			LifecycleState: strings.TrimSpace(ctx.Query("lifecycleState")), ClusterID: strings.TrimSpace(ctx.Query("clusterId")),
			SourceKind: strings.TrimSpace(ctx.Query("sourceKind")), OwnershipMode: strings.TrimSpace(ctx.Query("ownershipMode")),
			VolumeMode: strings.TrimSpace(ctx.Query("volumeMode")),
		})
		if err != nil {
			writeVolumeError(ctx, err)
			return
		}
		items := make([]projectVolumeResponse, 0, len(result.Items))
		observations := h.observeProjectVolumeResponses(ctx.Request.Context(), result.Items)
		for _, item := range result.Items {
			items = append(items, projectVolumeResponseForObservation(item, observations[item.ID]))
		}
		ctx.JSON(http.StatusOK, gin.H{
			"items": items, "page": result.Page, "pageSize": result.PageSize,
			"sortBy": result.SortBy, "sortOrder": result.SortOrder,
			"total": result.Total, "totalPages": result.TotalPages,
		})
	}
}

func (h *Handlers) CreateProjectVolume(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeWrite)...)
	if !ok {
		return
	}
	var input projectVolumeCreateInput
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
	domainInput, ok := projectVolumeCreateDomainInput(ctx, project, user, input, idempotencyKey)
	if !ok {
		return
	}
	if domainInput.OwnershipMode == model.ProjectVolumeOwnershipManaged && !h.ensureBillingAllowsManagedVolumeChange(ctx, project.ID) {
		return
	}
	result, err := h.volumes.CreateProjectVolume(ctx.Request.Context(), domainInput)
	if err != nil {
		h.auditWithContext(user.ID, projectVolumeCreateAuditAction(domainInput), project.ID, false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, projectVolumeCreateAuditAction(domainInput), result.Volume.ID, true, domainInput.SourceKind, ctx.Request.Context())
	ctx.JSON(http.StatusAccepted, projectVolumeResponseFor(result.Volume))
}

func (h *Handlers) GetProjectVolume(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	if user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeRead)...); ok {
		bindingPage := parsePositiveInt(ctx.Query("bindingPage"), 1)
		bindingPageSize := parsePositiveInt(ctx.Query("bindingPageSize"), volume.DefaultPageSize)
		transferPage := parsePositiveInt(ctx.Query("transferPage"), 1)
		transferPageSize := parsePositiveInt(ctx.Query("transferPageSize"), volume.DefaultPageSize)
		detail, err := h.volumes.GetProjectVolumeDetailPage(ctx.Request.Context(), project.ID, ctx.Param("volumeId"), bindingPage, bindingPageSize, transferPage, transferPageSize)
		if err != nil {
			writeVolumeError(ctx, err)
			return
		}
		privileged := authz.IsPlatformAdmin(user.Role) || h.currentProjectRoleAllows(ctx, project.ID, user.ID, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
		observations := h.observeProjectVolumeResponses(ctx.Request.Context(), []model.ProjectVolume{detail.Volume})
		ctx.JSON(http.StatusOK, projectVolumeDetailResponseFor(detail, observations[detail.Volume.ID], privileged, user.ID))
	}
}

func (h *Handlers) UpdateProjectVolume(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeWrite)...)
	if !ok {
		return
	}
	revision, ok := volumeRevisionHeader(ctx)
	if !ok {
		return
	}
	var input projectVolumeUpdateInput
	if !bindJSON(ctx, &input) {
		return
	}
	if input.Capacity != nil && !h.ensureBillingAllowsManagedVolumeChange(ctx, project.ID) {
		return
	}
	domainInput := volume.UpdateProjectVolumeInput{ActorID: user.ID, DisplayName: input.DisplayName}
	if input.Capacity != nil {
		capacity, bytes, valid := parseVolumeCapacity(*input.Capacity)
		if !valid {
			writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "volume capacity is invalid")
			return
		}
		domainInput.CapacityRequest = &capacity
		domainInput.CapacityBytes = &bytes
	}
	item, err := h.volumes.UpdateProjectVolume(ctx.Request.Context(), project.ID, ctx.Param("volumeId"), revision, domainInput)
	if err != nil {
		h.auditWithContext(user.ID, "project_volume.update", ctx.Param("volumeId"), false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "project_volume.update", item.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, projectVolumeResponseFor(item))
}

func (h *Handlers) DeleteProjectVolume(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeDelete)...)
	if !ok {
		return
	}
	revision, ok := volumeRevisionHeader(ctx)
	if !ok {
		return
	}
	item, detached, err := h.volumes.RequestDeleteProjectVolume(ctx.Request.Context(), volume.DeleteProjectVolumeInput{
		ProjectID: project.ID, VolumeID: ctx.Param("volumeId"), ActorID: user.ID,
		ExpectedRevision: revision, DataAction: ctx.Query("dataAction"),
	})
	if err != nil {
		h.auditWithContext(user.ID, "project_volume.delete", ctx.Param("volumeId"), false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	action := "project_volume.delete"
	if detached {
		action = "project_volume.detach"
	}
	h.auditWithContext(user.ID, action, item.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusAccepted, projectVolumeResponseFor(item))
}

func (h *Handlers) RetryProjectVolumeOperation(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeRead)...)
	if !ok {
		return
	}
	revision, ok := volumeRevisionHeader(ctx)
	if !ok {
		return
	}
	current, err := h.volumes.GetProjectVolume(ctx.Request.Context(), project.ID, ctx.Param("volumeId"))
	if err != nil {
		h.auditWithContext(user.ID, "project_volume.retry", ctx.Param("volumeId"), false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	retryAction, authorized := projectVolumeRetryAuthorization(current)
	if !authorized {
		h.auditWithContext(user.ID, "project_volume.retry", current.ID, false, volume.CodeStateConflict, ctx.Request.Context())
		writeErrorCode(ctx, http.StatusConflict, volume.CodeStateConflict, "project volume operation cannot be retried")
		return
	}
	if !authz.IsPlatformAdmin(user.Role) && !h.currentProjectRoleAllows(ctx, project.ID, user.ID, volumeActionRoles(retryAction)...) {
		writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "project role does not allow retrying this volume operation")
		return
	}
	if token, bearer := currentAccessTokenFromContext(ctx); bearer && !accessTokenAllows(token.Scope, string(retryAction)) {
		writeErrorCode(ctx, http.StatusForbidden, "auth.token.scope_insufficient", "the original volume operation scope is required")
		return
	}
	if current.OwnershipMode == model.ProjectVolumeOwnershipManaged &&
		(current.PendingOperation == volume.OperationProvision || current.PendingOperation == volume.OperationExpand) &&
		!h.ensureBillingAllowsManagedVolumeChange(ctx, project.ID) {
		return
	}
	item, err := h.volumes.RetryProjectVolumeOperation(ctx.Request.Context(), project.ID, ctx.Param("volumeId"), user.ID, revision)
	if err != nil {
		h.auditWithContext(user.ID, "project_volume.retry", current.ID, false, volumeAuditErrorCode(err), ctx.Request.Context())
		writeVolumeError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "project_volume.retry", item.ID, true, item.PendingOperation, ctx.Request.Context())
	ctx.JSON(http.StatusAccepted, projectVolumeResponseFor(item))
}

func projectVolumeRetryAuthorization(item model.ProjectVolume) (authz.Action, bool) {
	switch item.PendingOperation {
	case volume.OperationDelete:
		return authz.ActionVolumeDelete, true
	case volume.OperationProvision:
		return authz.ActionVolumeWrite, true
	case volume.OperationExpand:
		return authz.ActionVolumeWrite, true
	default:
		return "", false
	}
}

func (h *Handlers) PreviewProjectVolumeDeletion(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeDelete)...)
	if !ok {
		return
	}
	preview, err := h.volumes.PreviewProjectVolumeDeletion(ctx.Request.Context(), project.ID, ctx.Param("volumeId"))
	if err != nil {
		writeVolumeError(ctx, err)
		return
	}
	observations := h.observeProjectVolumeResponses(ctx.Request.Context(), []model.ProjectVolume{preview.Volume})
	ctx.JSON(http.StatusOK, projectVolumeDeletionPreviewResponseFor(preview, observations[preview.Volume.ID], user.ID))
}

func (h *Handlers) ListProjectVolumeStorageClasses(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, volumeActionRoles(authz.ActionVolumeRead)...)
	if !ok {
		return
	}
	clusterID := strings.TrimSpace(ctx.Query("clusterId"))
	if clusterID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "clusterId is required")
		return
	}
	if _, usable := h.runtimeClusterForProjectUse(ctx, user, project.ID, clusterID); !usable {
		return
	}
	pagination, valid := volumePagination(ctx, map[string]bool{"name": true, "provisioner": true}, "name")
	if !valid {
		return
	}
	if h.volumeClusters == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, volumeObservationUnavailableCode, "project volume storage classes are unavailable")
		return
	}
	items, err := h.volumeClusters.ListStorageClasses(ctx.Request.Context(), clusterID)
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, volumeObservationUnavailableCode, "project volume storage classes are unavailable")
		return
	}
	sort.SliceStable(items, func(left, right int) bool {
		leftValue, rightValue := items[left].Name, items[right].Name
		if pagination.SortBy == "provisioner" {
			leftValue, rightValue = items[left].Provisioner, items[right].Provisioner
			if leftValue == rightValue {
				leftValue, rightValue = items[left].Name, items[right].Name
			}
		}
		if pagination.SortOrder == "desc" {
			return leftValue > rightValue
		}
		return leftValue < rightValue
	})
	total := int64(len(items))
	ctx.JSON(http.StatusOK, paginatedResponse(paginateSlice(items, pagination), total, pagination))
}

func projectVolumeCreateDomainInput(ctx *gin.Context, project model.Project, user model.User, input projectVolumeCreateInput, idempotencyKey string) (volume.CreateProjectVolumeInput, bool) {
	sourceType := strings.TrimSpace(input.Source.Type)
	ownershipMode := model.ProjectVolumeOwnershipManaged
	sourceKind := model.ProjectVolumeSourceBlank
	claimName := ""
	switch sourceType {
	case "blank":
	case "existingClaim":
		sourceKind = model.ProjectVolumeSourceExistingClaim
		ownershipMode = strings.TrimSpace(input.Source.OwnershipMode)
		claimName = strings.TrimSpace(input.Source.ClaimName)
		if claimName == "" || (ownershipMode != model.ProjectVolumeOwnershipManaged && ownershipMode != model.ProjectVolumeOwnershipReferenced) {
			writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "existing claim source is invalid")
			return volume.CreateProjectVolumeInput{}, false
		}
	case "volumeSnapshot":
		sourceKind = model.ProjectVolumeSourceSnapshotRestore
		if strings.TrimSpace(input.Source.SnapshotName) == "" {
			writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "volume snapshot source is invalid")
			return volume.CreateProjectVolumeInput{}, false
		}
	default:
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "project volume source type is invalid")
		return volume.CreateProjectVolumeInput{}, false
	}
	capacity, capacityBytes, valid := parseVolumeCapacity(input.Capacity)
	storageClassName := strings.TrimSpace(input.StorageClassName)
	accessMode := strings.TrimSpace(input.AccessMode)
	volumeMode := strings.TrimSpace(input.VolumeMode)
	if sourceType == "existingClaim" {
		capacity, capacityBytes, valid = "", 0, true
		storageClassName, accessMode, volumeMode = "", "", ""
	}
	if !valid {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "volume capacity is invalid")
		return volume.CreateProjectVolumeInput{}, false
	}
	return volume.CreateProjectVolumeInput{
		ProjectID: project.ID, DisplayName: input.DisplayName, ClusterID: input.ClusterID,
		Namespace: runtimeProjectNamespace(project), ClaimName: claimName, OwnershipMode: ownershipMode,
		SourceKind: sourceKind, CapacityRequest: capacity, CapacityBytes: capacityBytes,
		SourceSnapshotName: strings.TrimSpace(input.Source.SnapshotName),
		StorageClassName:   storageClassName, AccessMode: accessMode, VolumeMode: volumeMode,
		ActorID: user.ID, IdempotencyKey: idempotencyKey,
	}, true
}

func projectVolumeResponseFor(item model.ProjectVolume) projectVolumeResponse {
	return projectVolumeResponseForObservation(item, unavailableProjectVolumeObservation(volumeObservationUnavailableCode))
}

func projectVolumeResponseForObservation(item model.ProjectVolume, observation projectVolumeObservationResponse) projectVolumeResponse {
	availability := item.Availability
	if !volume.CanAttachProjectVolume(item) {
		availability = model.ProjectVolumeAvailabilityUnavailable
	}
	if availability == "" {
		availability = model.ProjectVolumeAvailabilityUnavailable
	}
	if observation.ObservationCode == "" {
		observation.Status = availability
	}
	return projectVolumeResponse{
		ID: item.ID, ProjectID: item.ProjectID, DisplayName: item.DisplayName,
		ClusterID: item.ClusterID, Namespace: item.Namespace, ClaimName: item.ClaimName,
		OwnershipMode: item.OwnershipMode, SourceKind: item.SourceKind,
		SourceSnapshotName: item.SourceSnapshotName, LifecycleState: item.LifecycleState,
		PendingOperation: item.PendingOperation, Availability: availability,
		Capacity: item.CapacityRequest, CapacityBytes: item.CapacityBytes,
		StorageClassName: item.StorageClassName, AccessMode: item.AccessMode, VolumeMode: item.VolumeMode,
		SourceApplicationID: item.SourceApplicationID, SourceApplicationName: item.SourceApplicationName,
		SourceDeploymentTargetID: item.SourceDeploymentTargetID, BindingSummary: item.BindingSummary,
		Revision: item.Revision, LastErrorCode: item.LastErrorCode,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		Observation: observation,
	}
}

func (h *Handlers) observeProjectVolumeResponses(ctx context.Context, items []model.ProjectVolume) map[string]projectVolumeObservationResponse {
	if h == nil || h.volumeClusters == nil {
		observations := make(map[string]projectVolumeObservationResponse, len(items))
		for _, item := range items {
			observations[item.ID] = unavailableProjectVolumeObservation(volumeObservationUnavailableCode)
		}
		return observations
	}
	return h.volumeClusters.ObserveProjectVolumes(ctx, items)
}

func projectVolumeBindingResponseFor(item model.DeploymentVolumeMount) projectVolumeBindingResponse {
	return projectVolumeBindingResponse{
		ID: item.ID, ApplicationID: item.ApplicationID, DeploymentTargetID: item.DeploymentTargetID,
		LogicalName: item.LogicalName, SourceType: item.SourceType, MountPath: item.MountPath,
		DevicePath: item.DevicePath, ReadOnly: item.ReadOnly, ActivationState: item.ActivationState,
		LastErrorCode: item.LastErrorCode, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func projectVolumeDetailResponseFor(detail volume.ProjectVolumeDetail, observation projectVolumeObservationResponse, privileged bool, userID string) projectVolumeDetailResponse {
	bindings := make([]projectVolumeBindingResponse, 0, len(detail.Bindings))
	for _, binding := range detail.Bindings {
		bindings = append(bindings, projectVolumeBindingResponseFor(binding))
	}
	transfers := make([]volumeTransferResponse, 0, len(detail.Transfers))
	for _, transfer := range detail.Transfers {
		transfers = append(transfers, volumeTransferResponseFor(transfer, privileged || transfer.ActorID == userID))
	}
	return projectVolumeDetailResponse{
		projectVolumeResponse: projectVolumeResponseForObservation(detail.Volume, observation),
		Bindings:              bindings, BindingPage: detail.BindingPage, BindingPageSize: detail.BindingPageSize,
		BindingTotal: detail.BindingTotal, BindingTotalPages: detail.BindingTotalPages,
		RecentTransfers: transfers, TransferPage: detail.TransferPage, TransferPageSize: detail.TransferPageSize,
		TransferTotal: detail.TransferTotal, TransferTotalPages: detail.TransferTotalPages,
	}
}

func projectVolumeDeletionPreviewResponseFor(preview volume.ProjectVolumeDeletionPreview, observation projectVolumeObservationResponse, _ string) projectVolumeDeletionPreviewResponse {
	bindings := make([]projectVolumeBindingResponse, 0, len(preview.BlockingBindings))
	for _, binding := range preview.BlockingBindings {
		bindings = append(bindings, projectVolumeBindingResponseFor(binding))
	}
	transfers := make([]volumeTransferResponse, 0, len(preview.BlockingTransfers))
	for _, transfer := range preview.BlockingTransfers {
		transfers = append(transfers, volumeTransferResponseFor(transfer, true))
	}
	return projectVolumeDeletionPreviewResponse{
		VolumeID: preview.Volume.ID, OwnershipMode: preview.Volume.OwnershipMode,
		DataAction: preview.RequiredDataAction, HasActiveBindings: preview.BlockingBindingCount > 0,
		HasRunningTransfers: preview.BlockingTransferCount > 0, Bindings: bindings, RunningTransfers: transfers,
		UnderlyingClaimWillDelete: preview.Volume.OwnershipMode == model.ProjectVolumeOwnershipManaged,
		Observation:               observation,
	}
}

func volumeActionRoles(action authz.Action) []string {
	roles := []string{authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper, authz.ProjectRoleViewer}
	allowed := make([]string, 0, len(roles))
	for _, role := range roles {
		if authz.ProjectRoleAllows(role, action) {
			allowed = append(allowed, role)
		}
	}
	return allowed
}

func volumePagination(ctx *gin.Context, allowedSort map[string]bool, fallbackSort string) (paginationParams, bool) {
	pagination := paginationFromQuery(ctx)
	sortBy := strings.TrimSpace(ctx.Query("sortBy"))
	if sortBy == "" {
		sortBy = fallbackSort
	}
	if !allowedSort[sortBy] {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodePaginationSortByInvalid, "unsupported volume sort field")
		return paginationParams{}, false
	}
	sortOrder := strings.ToLower(strings.TrimSpace(ctx.Query("sortOrder")))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodePaginationOrderInvalid, "unsupported volume sort order")
		return paginationParams{}, false
	}
	pagination.SortBy = sortBy
	pagination.SortOrder = sortOrder
	return pagination, true
}

func volumeIdempotencyKey(ctx *gin.Context) (string, bool) {
	key := strings.TrimSpace(ctx.GetHeader("Idempotency-Key"))
	if len(key) < 8 || len(key) > 160 {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "Idempotency-Key must contain 8 to 160 characters")
		return "", false
	}
	return key, true
}

func volumeRevisionHeader(ctx *gin.Context) (int64, bool) {
	raw := strings.Trim(strings.TrimSpace(ctx.GetHeader("If-Match")), `"`)
	revision, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || revision < 1 {
		writeErrorCode(ctx, http.StatusBadRequest, volume.CodeInvalidInput, "If-Match must contain a positive numeric revision")
		return 0, false
	}
	return revision, true
}

func parseVolumeCapacity(raw string) (string, int64, bool) {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(raw))
	if err != nil || quantity.Sign() <= 0 {
		return "", 0, false
	}
	bytes, ok := quantity.AsInt64()
	if !ok || bytes <= 0 {
		return "", 0, false
	}
	return quantity.String(), bytes, true
}

func projectVolumeCreateAuditAction(input volume.CreateProjectVolumeInput) string {
	if input.SourceKind == model.ProjectVolumeSourceExistingClaim && input.OwnershipMode == model.ProjectVolumeOwnershipManaged {
		return "project_volume.adopt"
	}
	return "project_volume.create"
}

func volumeAuditErrorCode(err error) string {
	if code := volume.ErrorCode(err); code != "" {
		return code
	}
	return "internal_error"
}

func writeVolumeError(ctx *gin.Context, err error) {
	code := volume.ErrorCode(err)
	status := http.StatusInternalServerError
	switch code {
	case volume.CodeInvalidInput, volume.CodePaginationSortByInvalid, volume.CodePaginationOrderInvalid:
		status = http.StatusBadRequest
	case volume.CodeNotFound, volume.CodeClaimNotFound, volume.CodeTransferNotFound:
		status = http.StatusNotFound
	case volume.CodeTaskEnqueueFailed, volume.CodeClusterUnavailable, volume.CodeQuotaUnavailable, volume.CodeTransferUnavailable:
		status = http.StatusServiceUnavailable
	case volume.CodeOwnershipConflict, volume.CodeIncompatibleCluster, volume.CodeBindingConflict,
		volume.CodeInUse, volume.CodeCapacityShrinkForbidden, volume.CodeRevisionConflict,
		volume.CodeStateConflict, volume.CodeIdempotencyConflict, volume.CodeNameConflict,
		volume.CodeClaimConflict, volume.CodeClaimSpecConflict, volume.CodeTransferStateConflict,
		volume.CodeTransferFormatMismatch,
		volume.CodeQuotaExceeded:
		status = http.StatusConflict
	case volume.CodeExpansionUnsupported, volume.CodeSnapshotUnsupported, volume.CodeSnapshotRequired,
		volume.CodeTransferProgressInvalid, volume.CodeTransferChecksumInvalid,
		volume.CodeTransferChecksumMismatch, volume.CodeTransferArchiveUnsafe,
		volume.CodeTransferCapacityExceeded:
		status = http.StatusUnprocessableEntity
	case volume.CodeTransferExpired:
		status = http.StatusGone
	case volume.CodeTransferDownloadUnauthorized:
		status = http.StatusUnauthorized
	case "":
		code = "internal_error"
	}
	writeErrorCode(ctx, status, code, err.Error())
}
