package volumemigration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourcename"
)

const migrationActorID = "system:volume-center-migration"

type legacyDataVolume struct {
	Name              string `json:"name"`
	MountPath         string `json:"mountPath"`
	DevicePath        string `json:"devicePath,omitempty"`
	Capacity          string `json:"capacity"`
	SourceType        string `json:"sourceType"`
	ProjectVolumeID   string `json:"projectVolumeId,omitempty"`
	ReadOnly          bool   `json:"readOnly,omitempty"`
	ExistingClaimName string `json:"existingClaimName"`
	RetainedVolumeID  string `json:"retainedVolumeId"`
	EmptyDirMedium    string `json:"emptyDirMedium"`
	EmptyDirSizeLimit string `json:"emptyDirSizeLimit"`
}

type plannedTargetVolume struct {
	Index             int
	LogicalName       string
	MountPath         string
	DevicePath        string
	Capacity          string
	SourceType        string
	ProjectVolumeID   string
	ReadOnly          bool
	ExistingClaimName string
	RetainedVolumeID  string
	EmptyDirMedium    string
	EmptyDirSizeLimit string
}

func stableResourceID(prefix string, parts ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("luna-devops/volume-center-backfill/v1"))
	for _, part := range append([]string{prefix}, parts...) {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strings.TrimSpace(part)))
	}
	return prefix + "_" + hex.EncodeToString(hash.Sum(nil))[:24]
}

func stableProjectVolumeID(sourceKind string, parts ...string) string {
	return stableResourceID("pvol", append([]string{sourceKind}, parts...)...)
}

func stableMountID(targetID, logicalName string, _ int) string {
	return stableResourceID("dvmt", targetID, logicalName)
}

func stableRepairID(sourceKind, sourceID, code string) string {
	return stableResourceID("vrpi", sourceKind, sourceID, code)
}

func integerString(value int) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	buffer := [20]byte{}
	position := len(buffer)
	for value > 0 {
		position--
		buffer[position] = digits[value%10]
		value /= 10
	}
	return string(buffer[position:])
}

func parseTargetVolumes(target model.DeploymentTarget) ([]plannedTargetVolume, error) {
	raw := strings.TrimSpace(target.DataVolumes)
	if raw == "" || raw == "[]" {
		if !target.DataRetentionEnabled {
			return nil, nil
		}
		return []plannedTargetVolume{normalizeLegacyTargetVolume(target, legacyDataVolume{
			Name:      "data",
			MountPath: firstNonEmpty(target.DataMountPath, "/data"),
			Capacity:  firstNonEmpty(target.DataCapacity, "1Gi"),
		}, 0)}, nil
	}
	var decoded []legacyDataVolume
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil || len(decoded) == 0 {
		return nil, ErrInvalidOptions
	}
	result := make([]plannedTargetVolume, 0, len(decoded))
	for index, item := range decoded {
		result = append(result, normalizeLegacyTargetVolume(target, item, index))
	}
	return result, nil
}

func normalizeLegacyTargetVolume(target model.DeploymentTarget, item legacyDataVolume, index int) plannedTargetVolume {
	sourceType := normalizeLegacySourceType(item.SourceType)
	mountPath := cleanAbsolutePath(item.MountPath)
	devicePath := cleanAbsolutePath(item.DevicePath)
	if mountPath == "" && devicePath == "" {
		mountPath = cleanAbsolutePath(firstNonEmpty(target.DataMountPath, "/data"))
	}
	logicalPath := mountPath
	if logicalPath == "" {
		logicalPath = devicePath
	}
	logicalName := strings.TrimSpace(item.Name)
	if logicalName == "" {
		logicalName = path.Base(logicalPath)
		if logicalName == "." || logicalName == "/" || logicalName == "" {
			logicalName = "data-" + integerString(index+1)
		}
	}
	return plannedTargetVolume{
		Index:             index,
		LogicalName:       resourcename.DNSLabel(logicalName),
		MountPath:         mountPath,
		DevicePath:        devicePath,
		Capacity:          firstNonEmpty(item.Capacity, target.DataCapacity, "1Gi"),
		SourceType:        sourceType,
		ProjectVolumeID:   strings.TrimSpace(item.ProjectVolumeID),
		ReadOnly:          item.ReadOnly,
		ExistingClaimName: strings.TrimSpace(item.ExistingClaimName),
		RetainedVolumeID:  strings.TrimSpace(item.RetainedVolumeID),
		EmptyDirMedium:    strings.TrimSpace(item.EmptyDirMedium),
		EmptyDirSizeLimit: strings.TrimSpace(item.EmptyDirSizeLimit),
	}
}

func normalizeLegacySourceType(value string) string {
	switch strings.TrimSpace(value) {
	case "projectVolume":
		return "projectVolume"
	case "existingClaim":
		return "existingClaim"
	case "retainedClaim":
		return "retainedClaim"
	case "emptyDir":
		return "emptyDir"
	default:
		return "managed"
	}
}

func cleanAbsolutePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") {
		return ""
	}
	cleaned := path.Clean(value)
	if cleaned == "/" || cleaned == "." {
		return ""
	}
	return cleaned
}

func legacyManagedClaimName(target model.DeploymentTarget, logicalName string) string {
	resourceName := resourcename.PersistedOrLegacy(target.KubernetesName, "dplt", target.ID)
	if logicalName == "data" {
		return resourceName + "-data"
	}
	return resourcename.DNSLabel(resourceName + "-" + logicalName + "-data")
}

func projectNamespace(project model.Project) string {
	return resourcename.PersistedOrLegacy(project.KubernetesNamespace, "ns", project.ID)
}

func workloadName(target model.DeploymentTarget) string {
	return resourcename.PersistedOrLegacy(target.KubernetesName, "dplt", target.ID)
}

func migratedDisplayName(claimName, volumeID string) string {
	claimName = strings.TrimSpace(claimName)
	if claimName == "" {
		claimName = "volume"
	}
	suffix := strings.TrimPrefix(volumeID, "pvol_")
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return "migrated-" + claimName + "-" + suffix
}

func retainedProjectVolume(retained model.RetainedVolume, observation ClaimObservation) model.ProjectVolume {
	volumeID := stableProjectVolumeID(model.ProjectVolumeSourceRetained, retained.ID)
	createdAt := retained.CreatedAt
	if !retained.RetainedAt.IsZero() {
		createdAt = retained.RetainedAt
	}
	if createdAt.IsZero() {
		createdAt = time.Unix(0, 0).UTC()
	}
	return model.ProjectVolume{
		ID:                       volumeID,
		ProjectID:                retained.ProjectID,
		DisplayName:              migratedDisplayName(retained.ClaimName, volumeID),
		ClusterID:                retained.ClusterID,
		Namespace:                retained.Namespace,
		ClaimName:                retained.ClaimName,
		OwnershipMode:            model.ProjectVolumeOwnershipManaged,
		SourceKind:               model.ProjectVolumeSourceRetained,
		LifecycleState:           model.ProjectVolumeLifecycleReady,
		PendingOperation:         "",
		CapacityRequest:          observation.CapacityRequest,
		CapacityBytes:            observation.CapacityBytes,
		StorageClassName:         observation.StorageClassName,
		AccessMode:               observation.AccessModes[0],
		VolumeMode:               observation.VolumeMode,
		SourceApplicationID:      optionalString(retained.SourceApplicationID),
		SourceApplicationName:    retained.SourceApplicationName,
		SourceDeploymentTargetID: optionalString(retained.SourceDeploymentTargetID),
		CreatedBy:                migrationActorID,
		Revision:                 1,
		CreatedAt:                createdAt,
		UpdatedAt:                createdAt,
	}
}

func targetProjectVolume(project model.Project, target model.DeploymentTarget, item plannedTargetVolume, clusterID, namespace, claimName, sourceKind, ownershipMode, applicationName string, observation ClaimObservation) model.ProjectVolume {
	identityParts := []string{target.ID, item.LogicalName}
	if sourceKind == model.ProjectVolumeSourceExistingClaim {
		identityParts = []string{clusterID, namespace, claimName}
	}
	volumeID := stableProjectVolumeID(sourceKind, identityParts...)
	createdAt := target.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Unix(0, 0).UTC()
	}
	return model.ProjectVolume{
		ID:                       volumeID,
		ProjectID:                project.ID,
		DisplayName:              migratedDisplayName(claimName, volumeID),
		ClusterID:                clusterID,
		Namespace:                namespace,
		ClaimName:                claimName,
		OwnershipMode:            ownershipMode,
		SourceKind:               sourceKind,
		LifecycleState:           model.ProjectVolumeLifecycleReady,
		PendingOperation:         "",
		CapacityRequest:          observation.CapacityRequest,
		CapacityBytes:            observation.CapacityBytes,
		StorageClassName:         observation.StorageClassName,
		AccessMode:               observation.AccessModes[0],
		VolumeMode:               observation.VolumeMode,
		SourceApplicationID:      optionalString(target.ApplicationID),
		SourceApplicationName:    applicationName,
		SourceDeploymentTargetID: optionalString(target.ID),
		CreatedBy:                migrationActorID,
		Revision:                 1,
		CreatedAt:                createdAt,
		UpdatedAt:                createdAt,
	}
}

func targetDeploymentMount(target model.DeploymentTarget, item plannedTargetVolume, projectVolume *model.ProjectVolume, activationState string) model.DeploymentVolumeMount {
	sourceType := model.DeploymentVolumeSourceProjectVolume
	var projectVolumeID *string
	if projectVolume != nil {
		projectVolumeID = optionalString(projectVolume.ID)
	} else {
		sourceType = model.DeploymentVolumeSourceEmptyDir
	}
	createdAt := target.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Unix(0, 0).UTC()
	}
	return model.DeploymentVolumeMount{
		ID:                 stableMountID(target.ID, item.LogicalName, item.Index),
		ProjectID:          target.ProjectID,
		ApplicationID:      target.ApplicationID,
		DeploymentTargetID: target.ID,
		SourceType:         sourceType,
		ProjectVolumeID:    projectVolumeID,
		LogicalName:        item.LogicalName,
		MountPath:          optionalString(item.MountPath),
		DevicePath:         optionalString(item.DevicePath),
		ReadOnly:           item.ReadOnly,
		Exclusive:          projectVolume != nil && isExclusiveAccessMode(projectVolume.AccessMode),
		ActivationState:    activationState,
		EmptyDirMedium:     item.EmptyDirMedium,
		EmptyDirSizeLimit:  item.EmptyDirSizeLimit,
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt,
	}
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func isExclusiveAccessMode(value string) bool {
	return value == model.ProjectVolumeAccessReadWriteOnce || value == model.ProjectVolumeAccessReadWriteOncePod
}
