package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	ProjectVolumeIDLabel         = "luna.devops/project-volume-id"
	projectVolumeListLimit int64 = 100

	defaultStorageClassAnnotation = "storageclass.kubernetes.io/is-default-class"
	betaStorageClassAnnotation    = "storageclass.beta.kubernetes.io/is-default-class"
)

var (
	ErrInvalidProjectVolumeSpec       = errors.New("project volume specification is invalid")
	ErrProjectVolumeClaimNotFound     = errors.New("project volume claim was not found")
	ErrProjectVolumeOwnershipConflict = errors.New("project volume ownership conflicts with the existing claim")
	ErrProjectVolumeSpecConflict      = errors.New("project volume claim specification conflicts with the existing claim")
	ErrProjectVolumeClaimInUse        = errors.New("project volume claim is referenced by an active pod")
	ErrVolumeCapacityShrinkForbidden  = errors.New("project volume capacity cannot be reduced")
	ErrVolumeExpansionUnsupported     = errors.New("project volume storage class does not allow expansion")
	ErrVolumeSnapshotUnsupported      = errors.New("CSI volume snapshots are not supported")
	ErrVolumeSnapshotNotFound         = errors.New("volume snapshot was not found")
)

var volumeSnapshotClassGVR = schema.GroupVersionResource{
	Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotclasses",
}

type ProjectVolumeProvider interface {
	ListVolumeStorageClasses(ctx context.Context) ([]VolumeStorageClass, error)
	CreateProjectVolumeClaim(ctx context.Context, spec ProjectVolumeClaimSpec) (ProjectVolumeClaimObservation, error)
	ObserveProjectVolumeClaim(ctx context.Context, namespace, claimName string) (ProjectVolumeClaimObservation, error)
	ObserveProjectVolumeClaims(ctx context.Context, namespace, projectID string, claimNames []string) (map[string]ProjectVolumeClaimObservation, error)
	ExpandProjectVolumeClaim(ctx context.Context, namespace, claimName, projectID, volumeID, capacity string) (ProjectVolumeClaimObservation, error)
	DeleteProjectVolumeClaim(ctx context.Context, namespace, claimName, projectID, volumeID string) error
	InspectExistingProjectVolumeClaim(ctx context.Context, spec ExistingProjectVolumeClaimSpec) (ExistingProjectVolumeClaimInspection, error)
	AdoptExistingProjectVolumeClaim(ctx context.Context, spec ExistingProjectVolumeClaimSpec) (ProjectVolumeClaimObservation, error)
	DetectSnapshotSupport(ctx context.Context, storageClassName string) (VolumeSnapshotCapability, error)
	CreateVolumeSnapshot(ctx context.Context, spec ProjectVolumeSnapshotSpec) (VolumeSnapshotObservation, error)
	ObserveVolumeSnapshot(ctx context.Context, namespace, name string) (VolumeSnapshotObservation, error)
	DeleteVolumeSnapshot(ctx context.Context, namespace, name, projectID, volumeID string) error
}

type ProjectVolumeClaimSpec struct {
	ProjectID          string
	VolumeID           string
	Namespace          string
	ClaimName          string
	Capacity           string
	StorageClassName   string
	AccessMode         string
	VolumeMode         string
	SourceSnapshotName string
	SourceSnapshotAPI  string
	SourceSnapshotKind string
}

type ExistingProjectVolumeClaimSpec struct {
	ProjectID                string
	VolumeID                 string
	Namespace                string
	ClaimName                string
	ExpectedCapacity         string
	ExpectedStorageClassName string
	ExpectedAccessMode       string
	ExpectedVolumeMode       string
}

type ProjectVolumeClaimObservation struct {
	ClaimName         string
	Exists            bool
	Phase             string
	RequestedCapacity string
	Capacity          string
	StorageClassName  string
	AccessModes       []string
	VolumeMode        string
	BoundVolumeName   string
	CreatedAt         time.Time
	ObservedAt        time.Time
}

type ExistingProjectVolumeClaimInspection struct {
	Observation         ProjectVolumeClaimObservation
	ManagedBy           string
	ProjectID           string
	ProjectVolumeID     string
	ActivePodReferences int
}

type VolumeStorageClass struct {
	Name                 string
	Provisioner          string
	IsDefault            bool
	AllowVolumeExpansion bool
	VolumeBindingMode    string
	ReclaimPolicy        string
	SnapshotSupported    bool
	DefaultSnapshotClass string
}

type VolumeSnapshotCapability struct {
	Supported                bool
	SnapshotClassNames       []string
	DefaultSnapshotClassName string
}

func (c *Client) ListVolumeStorageClasses(ctx context.Context) (items []VolumeStorageClass, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "storage_class.list", "StorageClass")
	defer func() { end(err) }()
	classes, err := c.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	snapshotClasses, snapshotErr := c.listVolumeSnapshotClasses(ctx)
	if snapshotErr != nil && !errors.Is(snapshotErr, ErrVolumeSnapshotUnsupported) {
		return nil, snapshotErr
	}

	items = make([]VolumeStorageClass, 0, len(classes.Items))
	for i := range classes.Items {
		class := &classes.Items[i]
		capability := snapshotCapabilityForProvisioner(snapshotClasses, class.Provisioner)
		item := VolumeStorageClass{
			Name:                 class.Name,
			Provisioner:          class.Provisioner,
			IsDefault:            isDefaultStorageClass(class),
			AllowVolumeExpansion: class.AllowVolumeExpansion != nil && *class.AllowVolumeExpansion,
			SnapshotSupported:    capability.Supported,
			DefaultSnapshotClass: capability.DefaultSnapshotClassName,
		}
		if class.VolumeBindingMode != nil {
			item.VolumeBindingMode = string(*class.VolumeBindingMode)
		}
		if class.ReclaimPolicy != nil {
			item.ReclaimPolicy = string(*class.ReclaimPolicy)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (c *Client) CreateProjectVolumeClaim(ctx context.Context, spec ProjectVolumeClaimSpec) (observation ProjectVolumeClaimObservation, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_claim.create", "PersistentVolumeClaim")
	defer func() { end(err) }()
	pvc, err := buildProjectVolumeClaim(spec)
	if err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	claims := c.client.CoreV1().PersistentVolumeClaims(spec.Namespace)
	existing, err := claims.Get(ctx, spec.ClaimName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		created, createErr := claims.Create(ctx, pvc, metav1.CreateOptions{})
		if createErr != nil {
			return ProjectVolumeClaimObservation{}, createErr
		}
		return observeProjectVolumeClaim(created), nil
	}
	if err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	if err := ensureProjectVolumeClaimMatches(existing, spec); err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	return observeProjectVolumeClaim(existing), nil
}

func (c *Client) ObserveProjectVolumeClaim(ctx context.Context, namespace, claimName string) (observation ProjectVolumeClaimObservation, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_claim.get", "PersistentVolumeClaim")
	defer func() { end(err) }()
	if err := validateNamespacedResource(namespace, claimName); err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	claim, err := c.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, claimName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return ProjectVolumeClaimObservation{}, ErrProjectVolumeClaimNotFound
	}
	if err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	return observeProjectVolumeClaim(claim), nil
}

func (c *Client) ObserveProjectVolumeClaims(ctx context.Context, namespace, projectID string, claimNames []string) (observations map[string]ProjectVolumeClaimObservation, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_claim.list", "PersistentVolumeClaim")
	defer func() { end(err) }()
	if strings.TrimSpace(namespace) != namespace || len(validation.IsDNS1123Label(namespace)) > 0 {
		return nil, fmt.Errorf("%w: namespace", ErrInvalidProjectVolumeSpec)
	}
	if strings.TrimSpace(projectID) != projectID || len(validation.IsValidLabelValue(projectID)) > 0 || projectID == "" {
		return nil, fmt.Errorf("%w: project ID", ErrInvalidProjectVolumeSpec)
	}
	wanted := make(map[string]struct{}, len(claimNames))
	for _, claimName := range claimNames {
		claimName = strings.TrimSpace(claimName)
		if errs := validation.IsDNS1123Subdomain(claimName); len(errs) > 0 {
			return nil, fmt.Errorf("%w: claim name", ErrInvalidProjectVolumeSpec)
		}
		wanted[claimName] = struct{}{}
	}
	if len(wanted) == 0 {
		return map[string]ProjectVolumeClaimObservation{}, nil
	}
	selector := labels.Set{ManagedByLabel: ManagedByValue, ProjectIDLabel: projectID}.AsSelector().String()
	observations = make(map[string]ProjectVolumeClaimObservation, len(wanted))
	for claimName := range wanted {
		observations[claimName] = ProjectVolumeClaimObservation{ClaimName: claimName, Exists: false, ObservedAt: time.Now().UTC()}
	}
	remaining := make(map[string]struct{}, len(wanted))
	for claimName := range wanted {
		remaining[claimName] = struct{}{}
	}
	options := metav1.ListOptions{LabelSelector: selector, Limit: projectVolumeListLimit}
	for {
		claims, listErr := c.client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, options)
		if listErr != nil {
			return nil, listErr
		}
		for i := range claims.Items {
			if _, ok := remaining[claims.Items[i].Name]; !ok {
				continue
			}
			observations[claims.Items[i].Name] = observeProjectVolumeClaim(&claims.Items[i])
			delete(remaining, claims.Items[i].Name)
		}
		if len(remaining) == 0 || claims.Continue == "" {
			break
		}
		options.Continue = claims.Continue
	}
	return observations, nil
}

func (c *Client) ExpandProjectVolumeClaim(ctx context.Context, namespace, claimName, projectID, volumeID, capacity string) (observation ProjectVolumeClaimObservation, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_claim.expand", "PersistentVolumeClaim")
	defer func() { end(err) }()
	if err := validateExistingClaimSpec(ExistingProjectVolumeClaimSpec{ProjectID: projectID, VolumeID: volumeID, Namespace: namespace, ClaimName: claimName}); err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	requested, err := positiveQuantity(capacity)
	if err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	claims := c.client.CoreV1().PersistentVolumeClaims(namespace)
	claim, err := claims.Get(ctx, claimName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return ProjectVolumeClaimObservation{}, ErrProjectVolumeClaimNotFound
	}
	if err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	if err := ensureProjectVolumeOwnership(claim.Labels, projectID, volumeID); err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	current := claim.Spec.Resources.Requests[corev1.ResourceStorage]
	switch requested.Cmp(current) {
	case -1:
		return ProjectVolumeClaimObservation{}, ErrVolumeCapacityShrinkForbidden
	case 0:
		return observeProjectVolumeClaim(claim), nil
	}
	if claim.Spec.StorageClassName == nil || strings.TrimSpace(*claim.Spec.StorageClassName) == "" {
		return ProjectVolumeClaimObservation{}, ErrVolumeExpansionUnsupported
	}
	class, err := c.client.StorageV1().StorageClasses().Get(ctx, *claim.Spec.StorageClassName, metav1.GetOptions{})
	if err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	if class.AllowVolumeExpansion == nil || !*class.AllowVolumeExpansion {
		return ProjectVolumeClaimObservation{}, ErrVolumeExpansionUnsupported
	}
	updated := claim.DeepCopy()
	if updated.Spec.Resources.Requests == nil {
		updated.Spec.Resources.Requests = corev1.ResourceList{}
	}
	updated.Spec.Resources.Requests[corev1.ResourceStorage] = requested
	updated, err = claims.Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	return observeProjectVolumeClaim(updated), nil
}

func (c *Client) DeleteProjectVolumeClaim(ctx context.Context, namespace, claimName, projectID, volumeID string) (err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_claim.delete", "PersistentVolumeClaim")
	defer func() { end(err) }()
	if err := validateExistingClaimSpec(ExistingProjectVolumeClaimSpec{ProjectID: projectID, VolumeID: volumeID, Namespace: namespace, ClaimName: claimName}); err != nil {
		return err
	}
	claims := c.client.CoreV1().PersistentVolumeClaims(namespace)
	claim, err := claims.Get(ctx, claimName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureProjectVolumeOwnership(claim.Labels, projectID, volumeID); err != nil {
		return err
	}
	activeReferences, err := c.countActivePodsReferencingClaim(ctx, namespace, claimName)
	if err != nil {
		return err
	}
	if activeReferences > 0 {
		return ErrProjectVolumeClaimInUse
	}
	policy := metav1.DeletePropagationForeground
	return claims.Delete(ctx, claimName, metav1.DeleteOptions{PropagationPolicy: &policy})
}

func (c *Client) InspectExistingProjectVolumeClaim(ctx context.Context, spec ExistingProjectVolumeClaimSpec) (inspection ExistingProjectVolumeClaimInspection, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_claim.inspect", "PersistentVolumeClaim")
	defer func() { end(err) }()
	claim, activeReferences, err := c.inspectExistingProjectVolumeClaim(ctx, spec)
	if err != nil {
		return ExistingProjectVolumeClaimInspection{}, err
	}
	return existingClaimInspection(claim, activeReferences), nil
}

func (c *Client) AdoptExistingProjectVolumeClaim(ctx context.Context, spec ExistingProjectVolumeClaimSpec) (observation ProjectVolumeClaimObservation, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_claim.adopt", "PersistentVolumeClaim")
	defer func() { end(err) }()
	claim, activeReferences, err := c.inspectExistingProjectVolumeClaim(ctx, spec)
	if err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	if ensureProjectVolumeOwnership(claim.Labels, spec.ProjectID, spec.VolumeID) == nil {
		return observeProjectVolumeClaim(claim), nil
	}
	if activeReferences > 0 {
		return ProjectVolumeClaimObservation{}, ErrProjectVolumeClaimInUse
	}
	if managedBy := strings.TrimSpace(claim.Labels[ManagedByLabel]); managedBy != "" && managedBy != ManagedByValue {
		return ProjectVolumeClaimObservation{}, ErrProjectVolumeOwnershipConflict
	}
	updated := claim.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	updated.Labels[ManagedByLabel] = ManagedByValue
	updated.Labels[ProjectIDLabel] = spec.ProjectID
	updated.Labels[ProjectVolumeIDLabel] = spec.VolumeID
	updated, err = c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return ProjectVolumeClaimObservation{}, err
	}
	return observeProjectVolumeClaim(updated), nil
}

func (c *Client) inspectExistingProjectVolumeClaim(ctx context.Context, spec ExistingProjectVolumeClaimSpec) (*corev1.PersistentVolumeClaim, int, error) {
	if err := validateExistingClaimSpec(spec); err != nil {
		return nil, 0, err
	}
	claim, err := c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(ctx, spec.ClaimName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, 0, ErrProjectVolumeClaimNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	if projectID := strings.TrimSpace(claim.Labels[ProjectIDLabel]); projectID != "" && projectID != spec.ProjectID {
		return nil, 0, ErrProjectVolumeOwnershipConflict
	}
	if volumeID := strings.TrimSpace(claim.Labels[ProjectVolumeIDLabel]); volumeID != "" && volumeID != spec.VolumeID {
		return nil, 0, ErrProjectVolumeOwnershipConflict
	}
	if err := ensureExistingProjectVolumeClaimSpec(claim, spec); err != nil {
		return nil, 0, err
	}
	activeReferences, err := c.countActivePodsReferencingClaim(ctx, spec.Namespace, spec.ClaimName)
	if err != nil {
		return nil, 0, err
	}
	return claim, activeReferences, nil
}

func ensureExistingProjectVolumeClaimSpec(claim *corev1.PersistentVolumeClaim, spec ExistingProjectVolumeClaimSpec) error {
	if spec.ExpectedCapacity == "" && spec.ExpectedStorageClassName == "" && spec.ExpectedAccessMode == "" && spec.ExpectedVolumeMode == "" {
		return nil
	}
	if claim == nil || spec.ExpectedCapacity == "" || spec.ExpectedStorageClassName == "" || spec.ExpectedAccessMode == "" || spec.ExpectedVolumeMode == "" {
		return ErrProjectVolumeSpecConflict
	}
	observation := observeProjectVolumeClaim(claim)
	actualCapacity := observation.Capacity
	if actualCapacity == "" {
		actualCapacity = observation.RequestedCapacity
	}
	actual, actualErr := resource.ParseQuantity(actualCapacity)
	expected, expectedErr := resource.ParseQuantity(spec.ExpectedCapacity)
	if actualErr != nil || expectedErr != nil || actual.Cmp(expected) != 0 ||
		observation.StorageClassName != spec.ExpectedStorageClassName ||
		observation.VolumeMode != spec.ExpectedVolumeMode ||
		!containsProjectVolumeAccessMode(observation.AccessModes, spec.ExpectedAccessMode) {
		return ErrProjectVolumeSpecConflict
	}
	return nil
}

func containsProjectVolumeAccessMode(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (c *Client) countActivePodsReferencingClaim(ctx context.Context, namespace, claimName string) (int, error) {
	count := 0
	options := metav1.ListOptions{Limit: projectVolumeListLimit}
	for {
		pods, err := c.client.CoreV1().Pods(namespace).List(ctx, options)
		if err != nil {
			return 0, err
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				continue
			}
			for _, volume := range pod.Spec.Volumes {
				if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claimName {
					count++
					break
				}
			}
		}
		if pods.Continue == "" {
			return count, nil
		}
		options.Continue = pods.Continue
	}
}

func buildProjectVolumeClaim(spec ProjectVolumeClaimSpec) (*corev1.PersistentVolumeClaim, error) {
	if err := validateProjectVolumeClaimSpec(spec); err != nil {
		return nil, err
	}
	capacity, _ := positiveQuantity(spec.Capacity)
	storageClassName := strings.TrimSpace(spec.StorageClassName)
	volumeMode := corev1.PersistentVolumeMode(strings.TrimSpace(spec.VolumeMode))
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.ClaimName,
			Namespace: spec.Namespace,
			Labels: map[string]string{
				ManagedByLabel:       ManagedByValue,
				ProjectIDLabel:       spec.ProjectID,
				ProjectVolumeIDLabel: spec.VolumeID,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.PersistentVolumeAccessMode(spec.AccessMode)},
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: capacity}},
			StorageClassName: &storageClassName,
			VolumeMode:       &volumeMode,
		},
	}
	if snapshotName := strings.TrimSpace(spec.SourceSnapshotName); snapshotName != "" {
		apiGroup := firstNonEmpty(spec.SourceSnapshotAPI, "snapshot.storage.k8s.io")
		kind := firstNonEmpty(spec.SourceSnapshotKind, "VolumeSnapshot")
		pvc.Spec.DataSource = &corev1.TypedLocalObjectReference{APIGroup: &apiGroup, Kind: kind, Name: snapshotName}
	}
	return pvc, nil
}

func validateProjectVolumeClaimSpec(spec ProjectVolumeClaimSpec) error {
	if err := validateNamespacedResource(spec.Namespace, spec.ClaimName); err != nil {
		return err
	}
	if strings.TrimSpace(spec.ProjectID) == "" || strings.TrimSpace(spec.ProjectID) != spec.ProjectID || len(validation.IsValidLabelValue(spec.ProjectID)) > 0 {
		return fmt.Errorf("%w: project ID", ErrInvalidProjectVolumeSpec)
	}
	if strings.TrimSpace(spec.VolumeID) == "" || strings.TrimSpace(spec.VolumeID) != spec.VolumeID || len(validation.IsValidLabelValue(spec.VolumeID)) > 0 {
		return fmt.Errorf("%w: volume ID", ErrInvalidProjectVolumeSpec)
	}
	if _, err := positiveQuantity(spec.Capacity); err != nil {
		return err
	}
	if strings.TrimSpace(spec.StorageClassName) == "" || strings.TrimSpace(spec.StorageClassName) != spec.StorageClassName || len(validation.IsDNS1123Subdomain(spec.StorageClassName)) > 0 {
		return fmt.Errorf("%w: storage class", ErrInvalidProjectVolumeSpec)
	}
	switch corev1.PersistentVolumeAccessMode(spec.AccessMode) {
	case corev1.ReadWriteOnce, corev1.ReadWriteOncePod, corev1.ReadOnlyMany, corev1.ReadWriteMany:
	default:
		return fmt.Errorf("%w: access mode", ErrInvalidProjectVolumeSpec)
	}
	switch corev1.PersistentVolumeMode(spec.VolumeMode) {
	case corev1.PersistentVolumeFilesystem, corev1.PersistentVolumeBlock:
	default:
		return fmt.Errorf("%w: volume mode", ErrInvalidProjectVolumeSpec)
	}
	if spec.SourceSnapshotName != "" && len(validation.IsDNS1123Subdomain(spec.SourceSnapshotName)) > 0 {
		return fmt.Errorf("%w: snapshot name", ErrInvalidProjectVolumeSpec)
	}
	if spec.SourceSnapshotAPI != "" && spec.SourceSnapshotAPI != "snapshot.storage.k8s.io" {
		return fmt.Errorf("%w: snapshot API group", ErrInvalidProjectVolumeSpec)
	}
	if spec.SourceSnapshotKind != "" && spec.SourceSnapshotKind != "VolumeSnapshot" {
		return fmt.Errorf("%w: snapshot kind", ErrInvalidProjectVolumeSpec)
	}
	return nil
}

func validateExistingClaimSpec(spec ExistingProjectVolumeClaimSpec) error {
	if err := validateNamespacedResource(spec.Namespace, spec.ClaimName); err != nil {
		return err
	}
	if strings.TrimSpace(spec.ProjectID) == "" || strings.TrimSpace(spec.ProjectID) != spec.ProjectID || len(validation.IsValidLabelValue(spec.ProjectID)) > 0 {
		return fmt.Errorf("%w: project ID", ErrInvalidProjectVolumeSpec)
	}
	if strings.TrimSpace(spec.VolumeID) == "" || strings.TrimSpace(spec.VolumeID) != spec.VolumeID || len(validation.IsValidLabelValue(spec.VolumeID)) > 0 {
		return fmt.Errorf("%w: volume ID", ErrInvalidProjectVolumeSpec)
	}
	return nil
}

func validateNamespacedResource(namespace, name string) error {
	if strings.TrimSpace(namespace) != namespace || strings.TrimSpace(name) != name || len(validation.IsDNS1123Label(namespace)) > 0 || len(validation.IsDNS1123Subdomain(name)) > 0 {
		return fmt.Errorf("%w: namespace or resource name", ErrInvalidProjectVolumeSpec)
	}
	return nil
}

func positiveQuantity(value string) (resource.Quantity, error) {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(value))
	if err != nil || quantity.Sign() <= 0 {
		return resource.Quantity{}, fmt.Errorf("%w: capacity", ErrInvalidProjectVolumeSpec)
	}
	return quantity, nil
}

func ensureProjectVolumeClaimMatches(claim *corev1.PersistentVolumeClaim, spec ProjectVolumeClaimSpec) error {
	if err := ensureProjectVolumeOwnership(claim.Labels, spec.ProjectID, spec.VolumeID); err != nil {
		return err
	}
	requested, _ := positiveQuantity(spec.Capacity)
	existing := claim.Spec.Resources.Requests[corev1.ResourceStorage]
	if existing.Cmp(requested) < 0 || valueOrEmpty(claim.Spec.StorageClassName) != spec.StorageClassName || projectVolumeMode(claim) != spec.VolumeMode {
		return ErrProjectVolumeSpecConflict
	}
	if len(claim.Spec.AccessModes) != 1 || string(claim.Spec.AccessModes[0]) != spec.AccessMode {
		return ErrProjectVolumeSpecConflict
	}
	if !projectVolumeDataSourceMatches(claim.Spec.DataSource, spec) {
		return ErrProjectVolumeSpecConflict
	}
	return nil
}

func ensureProjectVolumeOwnership(resourceLabels map[string]string, projectID, volumeID string) error {
	if resourceLabels[ManagedByLabel] != ManagedByValue || resourceLabels[ProjectIDLabel] != projectID || resourceLabels[ProjectVolumeIDLabel] != volumeID {
		return ErrProjectVolumeOwnershipConflict
	}
	return nil
}

func projectVolumeDataSourceMatches(source *corev1.TypedLocalObjectReference, spec ProjectVolumeClaimSpec) bool {
	if strings.TrimSpace(spec.SourceSnapshotName) == "" {
		return source == nil
	}
	if source == nil || source.Name != spec.SourceSnapshotName || source.Kind != firstNonEmpty(spec.SourceSnapshotKind, "VolumeSnapshot") {
		return false
	}
	wantAPI := firstNonEmpty(spec.SourceSnapshotAPI, "snapshot.storage.k8s.io")
	return source.APIGroup != nil && *source.APIGroup == wantAPI
}

func observeProjectVolumeClaim(claim *corev1.PersistentVolumeClaim) ProjectVolumeClaimObservation {
	observation := ProjectVolumeClaimObservation{
		ClaimName:        claim.Name,
		Exists:           true,
		Phase:            string(claim.Status.Phase),
		StorageClassName: valueOrEmpty(claim.Spec.StorageClassName),
		VolumeMode:       projectVolumeMode(claim),
		BoundVolumeName:  claim.Spec.VolumeName,
		CreatedAt:        claim.CreationTimestamp.Time.UTC(),
		ObservedAt:       time.Now().UTC(),
	}
	if requested, ok := claim.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		observation.RequestedCapacity = requested.String()
	}
	if capacity, ok := claim.Status.Capacity[corev1.ResourceStorage]; ok {
		observation.Capacity = capacity.String()
	}
	if observation.Capacity == "" {
		observation.Capacity = observation.RequestedCapacity
	}
	for _, mode := range claim.Spec.AccessModes {
		observation.AccessModes = append(observation.AccessModes, string(mode))
	}
	return observation
}

func projectVolumeMode(claim *corev1.PersistentVolumeClaim) string {
	if claim.Spec.VolumeMode == nil || *claim.Spec.VolumeMode == "" {
		return string(corev1.PersistentVolumeFilesystem)
	}
	return string(*claim.Spec.VolumeMode)
}

func existingClaimInspection(claim *corev1.PersistentVolumeClaim, activeReferences int) ExistingProjectVolumeClaimInspection {
	return ExistingProjectVolumeClaimInspection{
		Observation:         observeProjectVolumeClaim(claim),
		ManagedBy:           strings.TrimSpace(claim.Labels[ManagedByLabel]),
		ProjectID:           strings.TrimSpace(claim.Labels[ProjectIDLabel]),
		ProjectVolumeID:     strings.TrimSpace(claim.Labels[ProjectVolumeIDLabel]),
		ActivePodReferences: activeReferences,
	}
}

func isDefaultStorageClass(class *storagev1.StorageClass) bool {
	return strings.EqualFold(class.Annotations[defaultStorageClassAnnotation], "true") ||
		strings.EqualFold(class.Annotations[betaStorageClassAnnotation], "true")
}

func startKubernetesVolumeOperation(ctx context.Context, operation, resourceKind string) (context.Context, telemetry.OperationEnd) {
	return telemetry.StartOperationWithKind(ctx, "kubernetes", operation, trace.SpanKindClient,
		attribute.String("k8s.resource.kind", resourceKind),
	)
}

func snapshotAPIUnavailable(err error) bool {
	return apierrors.IsNotFound(err) || meta.IsNoMatchError(err)
}
