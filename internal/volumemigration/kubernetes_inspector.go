package volumemigration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
)

const defaultInspectionTimeout = 8 * time.Second

type SecretResolver interface {
	ResolveContext(context.Context, string) string
}

type KubernetesInspector struct {
	db      *gorm.DB
	secrets SecretResolver
	timeout time.Duration
	mu      sync.Mutex
	clients map[string]*kubeprovider.Client
}

func NewKubernetesInspector(db *gorm.DB, secrets SecretResolver, timeout time.Duration) *KubernetesInspector {
	if timeout <= 0 {
		timeout = defaultInspectionTimeout
	}
	return &KubernetesInspector{db: db, secrets: secrets, timeout: timeout, clients: make(map[string]*kubeprovider.Client)}
}

func (inspector *KubernetesInspector) InspectClaim(ctx context.Context, input ClaimInspectionInput) (ClaimObservation, error) {
	client, err := inspector.clientForCluster(ctx, input.ClusterID)
	if err != nil {
		return ClaimObservation{}, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, inspector.timeout)
	defer cancel()
	inspection, err := client.InspectExistingProjectVolumeClaim(probeCtx, kubeprovider.ExistingProjectVolumeClaimSpec{
		ProjectID: input.ProjectID, VolumeID: input.ProjectVolumeID, Namespace: input.Namespace, ClaimName: input.ClaimName,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			if ctx.Err() != nil {
				return ClaimObservation{}, ctx.Err()
			}
			return ClaimObservation{}, ErrClaimObservation
		case errors.Is(err, kubeprovider.ErrProjectVolumeClaimNotFound):
			return ClaimObservation{}, ErrClaimNotFound
		case errors.Is(err, kubeprovider.ErrProjectVolumeOwnershipConflict):
			return ClaimObservation{}, ErrClaimOwnership
		default:
			return ClaimObservation{}, ErrClaimObservation
		}
	}
	capacity := inspection.Observation.RequestedCapacity
	if capacity == "" {
		capacity = inspection.Observation.Capacity
	}
	capacityBytes, ok := capacityInBytes(capacity)
	if !ok {
		return ClaimObservation{}, ErrClaimObservation
	}
	return ClaimObservation{
		Exists: true, CapacityRequest: capacity, CapacityBytes: capacityBytes,
		StorageClassName: inspection.Observation.StorageClassName,
		AccessModes:      append([]string(nil), inspection.Observation.AccessModes...),
		VolumeMode:       inspection.Observation.VolumeMode, ManagedBy: inspection.ManagedBy,
		OwnerProjectID: inspection.ProjectID, OwnerProjectVolumeID: inspection.ProjectVolumeID,
		ActivePodReferences: inspection.ActivePodReferences,
	}, nil
}

func (inspector *KubernetesInspector) InspectWorkload(ctx context.Context, input WorkloadInspectionInput) (map[string]WorkloadAttachment, error) {
	client, err := inspector.clientForCluster(ctx, input.ClusterID)
	if err != nil {
		return nil, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, inspector.timeout)
	defer cancel()
	attachments, err := client.ObserveApplicationVolumeAttachments(probeCtx, input.Namespace, input.Name, input.WorkloadType)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrWorkloadObservation
		case apierrors.IsNotFound(err):
			return nil, ErrWorkloadNotFound
		default:
			return nil, ErrWorkloadObservation
		}
	}
	result := make(map[string]WorkloadAttachment, len(attachments))
	for name, item := range attachments {
		result[name] = WorkloadAttachment{
			ClaimName: item.ClaimName, MountPath: item.MountPath, DevicePath: item.DevicePath,
			ReadOnly: item.ReadOnly, EmptyDir: item.EmptyDir,
		}
	}
	return result, nil
}

func (inspector *KubernetesInspector) clientForCluster(ctx context.Context, clusterID string) (*kubeprovider.Client, error) {
	if inspector == nil || inspector.db == nil || inspector.secrets == nil {
		return nil, ErrRuntimeCluster
	}
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, ErrRuntimeCluster
	}
	inspector.mu.Lock()
	client := inspector.clients[clusterID]
	inspector.mu.Unlock()
	if client != nil {
		return client, nil
	}
	var cluster model.RuntimeCluster
	err := inspector.db.WithContext(ctx).Where("type IN ?", []string{"kubernetes", "k3s"}).First(&cluster, "id = ?", clusterID).Error
	if err != nil || strings.TrimSpace(cluster.KubeconfigRef) == "" {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrRuntimeCluster
	}
	// The decrypted value is passed directly to client-go and is never exposed
	// through the report, logs, error strings, or telemetry attributes.
	kubeconfig := inspector.secrets.ResolveContext(ctx, cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrRuntimeCluster
	}
	client, err = kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, ErrRuntimeCluster
	}
	inspector.mu.Lock()
	if existing := inspector.clients[clusterID]; existing != nil {
		client = existing
	} else {
		inspector.clients[clusterID] = client
	}
	inspector.mu.Unlock()
	return client, nil
}

func capacityInBytes(value string) (int64, bool) {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(value))
	if err != nil || quantity.Sign() <= 0 {
		return 0, false
	}
	bytes, ok := quantity.AsInt64()
	return bytes, ok && bytes > 0
}
