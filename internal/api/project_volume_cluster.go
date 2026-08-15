package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/resource"
)

const projectVolumeClusterTimeout = 8 * time.Second

var errProjectVolumeClusterUnavailable = errors.New("project volume runtime cluster is unavailable")

type projectVolumeStorageClass struct {
	Name                 string `json:"name"`
	Provisioner          string `json:"provisioner"`
	IsDefault            bool   `json:"isDefault"`
	AllowVolumeExpansion bool   `json:"allowVolumeExpansion"`
	VolumeBindingMode    string `json:"volumeBindingMode"`
	ReclaimPolicy        string `json:"reclaimPolicy"`
	SnapshotSupported    bool   `json:"snapshotSupported"`
}

type projectVolumeClusterService interface {
	volume.ExistingClaimInspector
	ListStorageClasses(context.Context, string) ([]projectVolumeStorageClass, error)
	ObserveProjectVolumes(context.Context, []model.ProjectVolume) map[string]projectVolumeObservationResponse
}

type projectVolumeClusterAdapter struct {
	db      *gorm.DB
	secrets secret.Store
}

func newProjectVolumeClusterAdapter(db *gorm.DB, secrets secret.Store) *projectVolumeClusterAdapter {
	return &projectVolumeClusterAdapter{db: db, secrets: secrets}
}

func (adapter *projectVolumeClusterAdapter) InspectExistingClaim(ctx context.Context, input volume.ExistingClaimInspectionInput) (volume.ExistingClaimInspection, error) {
	client, err := adapter.clientForCluster(ctx, input.ClusterID)
	if err != nil {
		return volume.ExistingClaimInspection{}, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, projectVolumeClusterTimeout)
	defer cancel()
	inspection, err := client.InspectExistingProjectVolumeClaim(probeCtx, kubeprovider.ExistingProjectVolumeClaimSpec{
		ProjectID: input.ProjectID,
		VolumeID:  input.VolumeID,
		Namespace: input.Namespace,
		ClaimName: input.ClaimName,
	})
	if err != nil {
		switch {
		case errors.Is(err, kubeprovider.ErrProjectVolumeClaimNotFound):
			return volume.ExistingClaimInspection{}, volume.ErrExistingClaimNotFound
		case errors.Is(err, kubeprovider.ErrProjectVolumeOwnershipConflict):
			return volume.ExistingClaimInspection{}, volume.ErrExistingClaimOwnershipConflict
		default:
			return volume.ExistingClaimInspection{}, errProjectVolumeClusterUnavailable
		}
	}
	capacity := inspection.Observation.Capacity
	if capacity == "" {
		capacity = inspection.Observation.RequestedCapacity
	}
	capacityBytes, valid := quantityBytes(capacity)
	if !valid {
		return volume.ExistingClaimInspection{}, errProjectVolumeClusterUnavailable
	}
	accessMode := ""
	if len(inspection.Observation.AccessModes) > 0 {
		accessMode = inspection.Observation.AccessModes[0]
	}
	return volume.ExistingClaimInspection{
		CapacityRequest:     capacity,
		CapacityBytes:       capacityBytes,
		StorageClassName:    inspection.Observation.StorageClassName,
		AccessMode:          accessMode,
		VolumeMode:          inspection.Observation.VolumeMode,
		ManagedBy:           inspection.ManagedBy,
		OwnerProjectID:      inspection.ProjectID,
		OwnerVolumeID:       inspection.ProjectVolumeID,
		ActivePodReferences: inspection.ActivePodReferences,
	}, nil
}

func (adapter *projectVolumeClusterAdapter) ListStorageClasses(ctx context.Context, clusterID string) ([]projectVolumeStorageClass, error) {
	client, err := adapter.clientForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, projectVolumeClusterTimeout)
	defer cancel()
	items, err := client.ListVolumeStorageClasses(probeCtx)
	if err != nil {
		return nil, errProjectVolumeClusterUnavailable
	}
	result := make([]projectVolumeStorageClass, 0, len(items))
	for _, item := range items {
		result = append(result, projectVolumeStorageClass{
			Name: item.Name, Provisioner: item.Provisioner, IsDefault: item.IsDefault,
			AllowVolumeExpansion: item.AllowVolumeExpansion, VolumeBindingMode: item.VolumeBindingMode,
			ReclaimPolicy: item.ReclaimPolicy, SnapshotSupported: item.SnapshotSupported,
		})
	}
	return result, nil
}

func (adapter *projectVolumeClusterAdapter) ObserveProjectVolumes(ctx context.Context, items []model.ProjectVolume) map[string]projectVolumeObservationResponse {
	ctx, end := telemetry.StartOperation(ctx, "volume", "observe")
	var operationErr error
	result := make(map[string]projectVolumeObservationResponse, len(items))
	defer func() {
		recordProjectVolumeObservationMetrics(ctx, result)
		end(operationErr)
	}()
	type group struct {
		clusterID string
		namespace string
		projectID string
		items     []model.ProjectVolume
	}
	groups := map[string]*group{}
	for _, item := range items {
		result[item.ID] = unavailableProjectVolumeObservation(volumeObservationUnavailableCode)
		key := item.ClusterID + "\x00" + item.Namespace + "\x00" + item.ProjectID
		if groups[key] == nil {
			groups[key] = &group{clusterID: item.ClusterID, namespace: item.Namespace, projectID: item.ProjectID}
		}
		groups[key].items = append(groups[key].items, item)
	}
	for _, grouped := range groups {
		if ctx.Err() != nil {
			operationErr = ctx.Err()
			break
		}
		client, err := adapter.clientForCluster(ctx, grouped.clusterID)
		if err != nil {
			operationErr = errProjectVolumeClusterUnavailable
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, projectVolumeClusterTimeout)
		claimNames := make([]string, 0, len(grouped.items))
		for _, item := range grouped.items {
			if item.OwnershipMode == model.ProjectVolumeOwnershipManaged {
				claimNames = append(claimNames, item.ClaimName)
			}
		}
		observations := map[string]kubeprovider.ProjectVolumeClaimObservation{}
		if len(claimNames) > 0 {
			observations, err = client.ObserveProjectVolumeClaims(probeCtx, grouped.namespace, grouped.projectID, claimNames)
			if err != nil {
				operationErr = errProjectVolumeClusterUnavailable
				observations = map[string]kubeprovider.ProjectVolumeClaimObservation{}
			}
		}
		for _, item := range grouped.items {
			if item.OwnershipMode == model.ProjectVolumeOwnershipReferenced {
				observation, observeErr := client.ObserveProjectVolumeClaim(probeCtx, item.Namespace, item.ClaimName)
				if observeErr == nil {
					result[item.ID] = projectVolumeObservationFromProvider(item, observation)
				} else if errors.Is(observeErr, kubeprovider.ErrProjectVolumeClaimNotFound) {
					result[item.ID] = unavailableProjectVolumeObservation("volume.claim_not_found")
				} else {
					operationErr = errProjectVolumeClusterUnavailable
				}
				continue
			}
			observation, found := observations[item.ClaimName]
			if !found || !observation.Exists {
				result[item.ID] = unavailableProjectVolumeObservation("volume.claim_not_found")
				continue
			}
			result[item.ID] = projectVolumeObservationFromProvider(item, observation)
		}
		cancel()
	}
	return result
}

func (adapter *projectVolumeClusterAdapter) clientForCluster(ctx context.Context, clusterID string) (*kubeprovider.Client, error) {
	if adapter == nil || adapter.db == nil || strings.TrimSpace(clusterID) == "" {
		return nil, errProjectVolumeClusterUnavailable
	}
	var cluster model.RuntimeCluster
	err := adapter.db.WithContext(ctx).
		Where("type in ?", []string{"kubernetes", "k3s"}).
		First(&cluster, "id = ?", strings.TrimSpace(clusterID)).Error
	if err != nil || strings.TrimSpace(cluster.KubeconfigRef) == "" {
		return nil, errProjectVolumeClusterUnavailable
	}
	kubeconfig := strings.TrimSpace(adapter.secrets.ResolveContext(ctx, cluster.KubeconfigRef))
	if kubeconfig == "" {
		return nil, errProjectVolumeClusterUnavailable
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, errProjectVolumeClusterUnavailable
	}
	return client, nil
}

func projectVolumeObservationFromProvider(item model.ProjectVolume, observed kubeprovider.ProjectVolumeClaimObservation) projectVolumeObservationResponse {
	status := item.Availability
	if status == "" {
		status = model.ProjectVolumeAvailabilityAvailable
	}
	capacity := observed.Capacity
	if capacity == "" {
		capacity = observed.RequestedCapacity
	}
	return projectVolumeObservationResponse{
		Status: status, Exists: observed.Exists, Phase: observed.Phase, Capacity: capacity,
		StorageClass: observed.StorageClassName, AccessModes: observed.AccessModes,
		VolumeMode: observed.VolumeMode, BoundVolumeName: observed.BoundVolumeName,
		ObservedAt: observed.ObservedAt,
	}
}

func unavailableProjectVolumeObservation(code string) projectVolumeObservationResponse {
	return projectVolumeObservationResponse{
		Status: model.ProjectVolumeAvailabilityUnavailable, AccessModes: []string{},
		ObservedAt: time.Now().UTC(), ObservationCode: code,
	}
}

func quantityBytes(value string) (int64, bool) {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(value))
	if err != nil || quantity.Sign() <= 0 {
		return 0, false
	}
	bytes, ok := quantity.AsInt64()
	return bytes, ok && bytes > 0
}
