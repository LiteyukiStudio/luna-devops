package volumemigration

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"gorm.io/gorm"
)

type Service struct {
	repository Repository
	inspector  Inspector
}

type projectRunState struct {
	report               *Report
	volumeCandidates     map[string]model.ProjectVolume
	volumeCandidateReady map[string]bool
	retainedVolumes      map[string]model.ProjectVolume
	claimObservations    map[string]claimObservationResult
	workloadObservations map[string]workloadObservationResult
	mountCandidates      map[string]struct{}
}

type claimObservationResult struct {
	observation ClaimObservation
	err         error
}

type workloadObservationResult struct {
	attachments map[string]WorkloadAttachment
	err         error
}

func NewService(repository Repository, inspector Inspector) *Service {
	return &Service{repository: repository, inspector: inspector}
}

func (service *Service) Run(ctx context.Context, options Options) (report Report, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_migration", "backfill")
	defer func() { end(err) }()
	options, err = normalizeOptions(options)
	if err != nil {
		return Report{}, err
	}
	if service == nil || service.repository == nil || service.inspector == nil {
		return Report{}, ErrInvalidOptions
	}
	report = Report{
		SchemaVersion: 1,
		Mode:          options.Mode(),
		PageSize:      options.PageSize,
		ProjectFilter: strings.TrimSpace(options.ProjectID),
		Repairs:       make([]RepairItem, 0),
	}
	for projectPage := 1; ; projectPage++ {
		if err = ctx.Err(); err != nil {
			return Report{}, err
		}
		projects, listErr := service.repository.ListProjects(ctx, projectPage, options.PageSize, options.ProjectID)
		if listErr != nil {
			return Report{}, listErr
		}
		for _, project := range projects {
			report.Reconciliation.Projects++
			state := projectRunState{
				report:               &report,
				volumeCandidates:     make(map[string]model.ProjectVolume),
				volumeCandidateReady: make(map[string]bool),
				retainedVolumes:      make(map[string]model.ProjectVolume),
				claimObservations:    make(map[string]claimObservationResult),
				workloadObservations: make(map[string]workloadObservationResult),
				mountCandidates:      make(map[string]struct{}),
			}
			if err = service.processProject(ctx, project, options, &state); err != nil {
				return Report{}, err
			}
		}
		if len(projects) < options.PageSize {
			break
		}
	}
	finalizeReport(&report, options.Apply)
	return report, nil
}

func normalizeOptions(options Options) (Options, error) {
	if options.PageSize == 0 {
		options.PageSize = DefaultPageSize
	}
	if options.PageSize < 1 || options.PageSize > MaxPageSize {
		return Options{}, ErrInvalidOptions
	}
	options.ProjectID = strings.TrimSpace(options.ProjectID)
	return options, nil
}

func (service *Service) processProject(ctx context.Context, project model.Project, options Options, state *projectRunState) error {
	for page := 1; ; page++ {
		retained, err := service.repository.ListRetainedVolumes(ctx, project.ID, page, options.PageSize)
		if err != nil {
			return err
		}
		for _, item := range retained {
			state.report.Reconciliation.SourceRetainedVolumes++
			if err := service.processRetainedVolume(ctx, item, options, state); err != nil {
				return err
			}
		}
		if len(retained) < options.PageSize {
			break
		}
	}
	for page := 1; ; page++ {
		targets, err := service.repository.ListDeploymentTargets(ctx, project.ID, page, options.PageSize)
		if err != nil {
			return err
		}
		for _, target := range targets {
			state.report.Reconciliation.DeploymentTargets++
			if err := service.processDeploymentTarget(ctx, project, target, options, state); err != nil {
				return err
			}
		}
		if len(targets) < options.PageSize {
			break
		}
	}
	return nil
}

func (service *Service) processRetainedVolume(ctx context.Context, retained model.RetainedVolume, options Options, state *projectRunState) error {
	switch retained.Status {
	case model.RetainedVolumeStatusRetained, model.RetainedVolumeStatusReserved, model.RetainedVolumeStatusClaimed:
	default:
		state.addRepair("retained_volume", retained.ID, retained.ProjectID, retained.ClusterID, retained.Namespace, retained.ClaimName, RepairLegacyRetainedStateUnsupported)
		return nil
	}
	volumeID := stableProjectVolumeID(model.ProjectVolumeSourceRetained, retained.ID)
	observation, err := service.inspectClaim(ctx, state, ClaimInspectionInput{
		ProjectID: retained.ProjectID, ProjectVolumeID: volumeID, ClusterID: retained.ClusterID,
		Namespace: retained.Namespace, ClaimName: retained.ClaimName,
	})
	if err != nil {
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			return ctx.Err()
		}
		state.addRepair("retained_volume", retained.ID, retained.ProjectID, retained.ClusterID, retained.Namespace, retained.ClaimName, repairCodeForClaimError(err))
		return nil
	}
	if code := validateClaimObservation(observation, retained.ProjectID, volumeID, true); code != "" {
		state.addRepair("retained_volume", retained.ID, retained.ProjectID, retained.ClusterID, retained.Namespace, retained.ClaimName, code)
		return nil
	}
	volume := retainedProjectVolume(retained, observation)
	ready, err := service.syncVolume(ctx, volume, options.Apply, state)
	if ready {
		state.retainedVolumes[retained.ID] = volume
	}
	return err
}

func (service *Service) processDeploymentTarget(ctx context.Context, project model.Project, target model.DeploymentTarget, options Options, state *projectRunState) error {
	items, err := parseTargetVolumes(target)
	if err != nil {
		state.addRepair("deployment_target", target.ID, project.ID, target.ClusterID, projectNamespace(project), "", RepairLegacyDataVolumesInvalid)
		return nil
	}
	if len(items) == 0 {
		return nil
	}
	clusterID, err := service.repository.ResolveRuntimeClusterID(ctx, target.ClusterID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		state.addRepair("deployment_target", target.ID, project.ID, target.ClusterID, projectNamespace(project), "", RepairRuntimeClusterUnavailable)
		return nil
	}
	namespace := projectNamespace(project)
	applicationName := ""
	if application, getErr := service.repository.GetApplication(ctx, project.ID, target.ApplicationID); getErr == nil {
		applicationName = application.Name
	} else if !errors.Is(getErr, gorm.ErrRecordNotFound) {
		return getErr
	}
	attachments, workloadErr := service.inspectWorkload(ctx, state, WorkloadInspectionInput{
		ClusterID: clusterID, Namespace: namespace, Name: workloadName(target), WorkloadType: target.WorkloadType,
	})
	if workloadErr != nil && !errors.Is(workloadErr, ErrWorkloadNotFound) {
		if ctx.Err() != nil && (errors.Is(workloadErr, context.Canceled) || errors.Is(workloadErr, context.DeadlineExceeded)) {
			return ctx.Err()
		}
		state.addRepair("deployment_target", target.ID, project.ID, clusterID, namespace, "", RepairWorkloadObservationUnavailable)
		attachments = nil
	}
	for _, item := range items {
		if err := service.processTargetVolume(ctx, project, target, item, clusterID, namespace, applicationName, attachments, workloadErr, options, state); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) processTargetVolume(ctx context.Context, project model.Project, target model.DeploymentTarget, item plannedTargetVolume, clusterID, namespace, applicationName string, attachments map[string]WorkloadAttachment, workloadErr error, options Options, state *projectRunState) error {
	if item.SourceType == "emptyDir" {
		state.report.Reconciliation.SourceEmptyDirMounts++
	} else {
		state.report.Reconciliation.SourcePersistentMounts++
	}
	if item.MountPath == "" && item.DevicePath == "" {
		state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, "", RepairLegacyMountInvalid)
		return nil
	}
	if item.MountPath != "" && item.DevicePath != "" {
		state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, "", RepairLegacyMountInvalid)
		return nil
	}
	if item.SourceType == "emptyDir" {
		activation := observedActivation(item, "", attachments, workloadErr)
		mount := targetDeploymentMount(target, item, nil, activation.state)
		if activation.mismatch {
			state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, "", RepairWorkloadMountMismatch)
		}
		return service.syncMount(ctx, mount, options.Apply, state)
	}
	var projectVolume model.ProjectVolume
	switch item.SourceType {
	case "retainedClaim":
		projectVolume, _ = state.retainedVolumes[item.RetainedVolumeID]
		if projectVolume.ID == "" {
			state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, item.ExistingClaimName, RepairRetainedVolumeMissing)
			return nil
		}
		if projectVolume.ClusterID != clusterID || projectVolume.Namespace != namespace ||
			(item.ExistingClaimName != "" && item.ExistingClaimName != projectVolume.ClaimName) {
			state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, projectVolume.ClaimName, RepairProjectVolumeConflict)
			return nil
		}
	case "projectVolume":
		if item.ProjectVolumeID == "" {
			state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, "", RepairProjectVolumeMissing)
			return nil
		}
		var err error
		projectVolume, err = service.repository.GetProjectVolume(ctx, project.ID, item.ProjectVolumeID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, "", RepairProjectVolumeMissing)
			return nil
		}
		if err != nil {
			return err
		}
		if projectVolume.ClusterID != clusterID || projectVolume.Namespace != namespace {
			state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, projectVolume.ClaimName, RepairProjectVolumeConflict)
			return nil
		}
	case "existingClaim", "managed":
		claimName := item.ExistingClaimName
		sourceKind := model.ProjectVolumeSourceExistingClaim
		ownershipMode := model.ProjectVolumeOwnershipReferenced
		if item.SourceType == "managed" {
			claimName = legacyManagedClaimName(target, item.LogicalName)
			sourceKind = model.ProjectVolumeSourceManaged
			ownershipMode = model.ProjectVolumeOwnershipManaged
		}
		if claimName == "" {
			state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, "", RepairLegacyMountInvalid)
			return nil
		}
		volumeID := stableProjectVolumeID(sourceKind, target.ID, item.LogicalName)
		if sourceKind == model.ProjectVolumeSourceExistingClaim {
			volumeID = stableProjectVolumeID(sourceKind, clusterID, namespace, claimName)
		}
		observation, err := service.inspectClaim(ctx, state, ClaimInspectionInput{
			ProjectID: project.ID, ProjectVolumeID: volumeID, ClusterID: clusterID, Namespace: namespace, ClaimName: claimName,
		})
		if err != nil {
			if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				return ctx.Err()
			}
			state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, claimName, repairCodeForClaimError(err))
			return nil
		}
		if code := validateClaimObservation(observation, project.ID, volumeID, ownershipMode == model.ProjectVolumeOwnershipManaged); code != "" {
			state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, claimName, code)
			return nil
		}
		projectVolume = targetProjectVolume(project, target, item, clusterID, namespace, claimName, sourceKind, ownershipMode, applicationName, observation)
		ready, err := service.syncVolume(ctx, projectVolume, options.Apply, state)
		if err != nil {
			return err
		}
		if !ready {
			return nil
		}
	default:
		state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, "", RepairLegacyDataVolumesInvalid)
		return nil
	}
	if projectVolume.VolumeMode == model.ProjectVolumeModeBlock && item.DevicePath == "" {
		state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, projectVolume.ClaimName, RepairLegacyBlockDevicePathMissing)
		return nil
	}
	if projectVolume.VolumeMode == model.ProjectVolumeModeFilesystem && item.MountPath == "" {
		state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, projectVolume.ClaimName, RepairLegacyMountInvalid)
		return nil
	}
	activation := observedActivation(item, projectVolume.ClaimName, attachments, workloadErr)
	if activation.mismatch {
		state.addRepair("deployment_volume", target.ID+":"+item.LogicalName, project.ID, clusterID, namespace, projectVolume.ClaimName, RepairWorkloadMountMismatch)
	}
	mount := targetDeploymentMount(target, item, &projectVolume, activation.state)
	return service.syncMount(ctx, mount, options.Apply, state)
}

type activationResult struct {
	state    string
	mismatch bool
}

func observedActivation(item plannedTargetVolume, claimName string, attachments map[string]WorkloadAttachment, workloadErr error) activationResult {
	result := activationResult{state: model.DeploymentVolumeActivationReserved}
	if workloadErr != nil || attachments == nil {
		return result
	}
	attachment, exists := attachments[item.LogicalName]
	if !exists {
		return result
	}
	matches := attachment.MountPath == item.MountPath && attachment.DevicePath == item.DevicePath && attachment.ReadOnly == item.ReadOnly
	if item.SourceType == "emptyDir" {
		matches = matches && attachment.EmptyDir && attachment.ClaimName == ""
	} else {
		matches = matches && !attachment.EmptyDir && attachment.ClaimName == claimName
	}
	if matches {
		result.state = model.DeploymentVolumeActivationActive
	} else {
		result.mismatch = true
	}
	return result
}

func (service *Service) inspectClaim(ctx context.Context, state *projectRunState, input ClaimInspectionInput) (ClaimObservation, error) {
	key := strings.Join([]string{input.ClusterID, input.Namespace, input.ClaimName, input.ProjectVolumeID}, "\x00")
	if cached, exists := state.claimObservations[key]; exists {
		return cached.observation, cached.err
	}
	observation, err := service.inspector.InspectClaim(ctx, input)
	state.claimObservations[key] = claimObservationResult{observation: observation, err: err}
	return observation, err
}

func (service *Service) inspectWorkload(ctx context.Context, state *projectRunState, input WorkloadInspectionInput) (map[string]WorkloadAttachment, error) {
	key := strings.Join([]string{input.ClusterID, input.Namespace, input.Name, input.WorkloadType}, "\x00")
	if cached, exists := state.workloadObservations[key]; exists {
		return cached.attachments, cached.err
	}
	attachments, err := service.inspector.InspectWorkload(ctx, input)
	state.workloadObservations[key] = workloadObservationResult{attachments: attachments, err: err}
	return attachments, err
}

func validateClaimObservation(observation ClaimObservation, projectID, volumeID string, managed bool) string {
	if !observation.Exists || observation.CapacityBytes <= 0 || strings.TrimSpace(observation.CapacityRequest) == "" || len(observation.AccessModes) != 1 || !validAccessMode(observation.AccessModes[0]) || !validVolumeMode(observation.VolumeMode) {
		return RepairClaimSpecUnsupported
	}
	if owner := strings.TrimSpace(observation.OwnerProjectID); owner != "" && owner != projectID {
		return RepairClaimLabelsMismatch
	}
	if owner := strings.TrimSpace(observation.OwnerProjectVolumeID); owner != "" && owner != volumeID {
		return RepairClaimLabelsMismatch
	}
	if managed && (strings.TrimSpace(observation.ManagedBy) != kubernetes.ManagedByValue || strings.TrimSpace(observation.OwnerProjectID) != projectID) {
		return RepairClaimLabelsMismatch
	}
	return ""
}

func validAccessMode(value string) bool {
	switch value {
	case model.ProjectVolumeAccessReadWriteOnce, model.ProjectVolumeAccessReadWriteOncePod, model.ProjectVolumeAccessReadOnlyMany, model.ProjectVolumeAccessReadWriteMany:
		return true
	default:
		return false
	}
}

func validVolumeMode(value string) bool {
	return value == model.ProjectVolumeModeFilesystem || value == model.ProjectVolumeModeBlock
}

func repairCodeForClaimError(err error) string {
	switch {
	case errors.Is(err, ErrClaimNotFound):
		return RepairClaimNotFound
	case errors.Is(err, ErrClaimOwnership):
		return RepairClaimOwnershipConflict
	case errors.Is(err, ErrRuntimeCluster):
		return RepairRuntimeClusterUnavailable
	default:
		return RepairClaimObservationUnavailable
	}
}

func (service *Service) syncVolume(ctx context.Context, volume model.ProjectVolume, apply bool, state *projectRunState) (bool, error) {
	if _, exists := state.volumeCandidates[volume.ID]; exists {
		return state.volumeCandidateReady[volume.ID], nil
	}
	state.volumeCandidates[volume.ID] = volume
	state.report.Reconciliation.ExpectedProjectVolumes++
	state.report.Reconciliation.ObservedCapacityBytes += volume.CapacityBytes
	result, err := service.repository.SyncProjectVolume(ctx, volume, apply)
	if errors.Is(err, ErrProjectVolumeConflict) {
		state.addRepair("project_volume", volume.ID, volume.ProjectID, volume.ClusterID, volume.Namespace, volume.ClaimName, RepairProjectVolumeConflict)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch result.Outcome {
	case OutcomePlanned:
		state.report.Reconciliation.PlannedProjectVolumes++
	case OutcomeApplied:
		state.report.Reconciliation.AppliedProjectVolumes++
		state.report.Reconciliation.VerifiedProjectVolumes++
		state.report.Reconciliation.VerifiedCapacityBytes += result.CapacityBytes
	case OutcomeUnchanged:
		state.report.Reconciliation.UnchangedProjectVolumes++
		state.report.Reconciliation.VerifiedProjectVolumes++
		state.report.Reconciliation.VerifiedCapacityBytes += result.CapacityBytes
	default:
		return false, ErrProjectVolumeConflict
	}
	state.volumeCandidateReady[volume.ID] = true
	return true, nil
}

func (service *Service) syncMount(ctx context.Context, mount model.DeploymentVolumeMount, apply bool, state *projectRunState) error {
	if _, exists := state.mountCandidates[mount.ID]; exists {
		return nil
	}
	state.mountCandidates[mount.ID] = struct{}{}
	state.report.Reconciliation.ExpectedDeploymentMounts++
	result, err := service.repository.SyncDeploymentVolumeMount(ctx, mount, apply)
	if errors.Is(err, ErrDeploymentMountConflict) {
		state.addRepair("deployment_volume_mount", mount.ID, mount.ProjectID, "", "", "", RepairDeploymentMountConflict)
		return nil
	}
	if err != nil {
		return err
	}
	switch result.Outcome {
	case OutcomePlanned:
		state.report.Reconciliation.PlannedDeploymentMounts++
	case OutcomeApplied:
		state.report.Reconciliation.AppliedDeploymentMounts++
		state.report.Reconciliation.VerifiedDeploymentMounts++
	case OutcomeUnchanged:
		state.report.Reconciliation.UnchangedDeploymentMounts++
		state.report.Reconciliation.VerifiedDeploymentMounts++
	default:
		return ErrDeploymentMountConflict
	}
	return nil
}

func (state *projectRunState) addRepair(resourceKind, sourceID, projectID, clusterID, namespace, claimName, code string) {
	item := RepairItem{
		ID: stableRepairID(resourceKind, sourceID, code), Code: code, ResourceKind: resourceKind,
		SourceID: sourceID, ProjectID: projectID, ClusterID: clusterID, Namespace: namespace, ClaimName: claimName,
	}
	for _, existing := range state.report.Repairs {
		if existing.ID == item.ID {
			return
		}
	}
	state.report.Repairs = append(state.report.Repairs, item)
}

func finalizeReport(report *Report, apply bool) {
	sort.Slice(report.Repairs, func(i, j int) bool { return report.Repairs[i].ID < report.Repairs[j].ID })
	reconciliation := &report.Reconciliation
	reconciliation.RepairItems = int64(len(report.Repairs))
	reconciliation.PlanBalanced = reconciliation.RepairItems == 0 &&
		reconciliation.ExpectedDeploymentMounts == reconciliation.SourcePersistentMounts+reconciliation.SourceEmptyDirMounts &&
		reconciliation.ExpectedProjectVolumes <= reconciliation.SourceRetainedVolumes+reconciliation.SourcePersistentMounts
	reconciliation.DatabaseBalanced = reconciliation.RepairItems == 0 &&
		reconciliation.VerifiedProjectVolumes == reconciliation.ExpectedProjectVolumes &&
		reconciliation.VerifiedDeploymentMounts == reconciliation.ExpectedDeploymentMounts &&
		reconciliation.VerifiedCapacityBytes == reconciliation.ObservedCapacityBytes
	reconciliation.ReadyForSwitch = apply && reconciliation.PlanBalanced && reconciliation.DatabaseBalanced
}
