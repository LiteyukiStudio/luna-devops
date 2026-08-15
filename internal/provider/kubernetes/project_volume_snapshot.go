package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	defaultVolumeSnapshotClassAnnotation = "snapshot.storage.kubernetes.io/is-default-class"
	volumeSnapshotFailureCode            = "volume.snapshot_failed"
)

var volumeSnapshotGVR = schema.GroupVersionResource{
	Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshots",
}

type ProjectVolumeSnapshotSpec struct {
	ProjectID         string
	VolumeID          string
	Namespace         string
	Name              string
	SourceClaimName   string
	SnapshotClassName string
	ManagedClaim      bool
}

type VolumeSnapshotObservation struct {
	Name                 string
	Exists               bool
	ReadyToUse           bool
	SourceClaimName      string
	SnapshotClassName    string
	BoundSnapshotContent string
	RestoreSize          string
	ErrorCode            string
	ObservedAt           time.Time
}

type volumeSnapshotClass struct {
	Name      string
	Driver    string
	IsDefault bool
}

func (c *Client) DetectSnapshotSupport(ctx context.Context, storageClassName string) (capability VolumeSnapshotCapability, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_snapshot.capability", "VolumeSnapshotClass")
	defer func() { end(err) }()
	storageClassName = strings.TrimSpace(storageClassName)
	if storageClassName == "" || len(validation.IsDNS1123Subdomain(storageClassName)) > 0 {
		return VolumeSnapshotCapability{}, fmt.Errorf("%w: storage class", ErrInvalidProjectVolumeSpec)
	}
	class, err := c.client.StorageV1().StorageClasses().Get(ctx, storageClassName, metav1.GetOptions{})
	if err != nil {
		return VolumeSnapshotCapability{}, err
	}
	classes, err := c.listVolumeSnapshotClasses(ctx)
	if errors.Is(err, ErrVolumeSnapshotUnsupported) {
		return VolumeSnapshotCapability{Supported: false}, nil
	}
	if err != nil {
		return VolumeSnapshotCapability{}, err
	}
	return snapshotCapabilityForProvisioner(classes, class.Provisioner), nil
}

func (c *Client) CreateVolumeSnapshot(ctx context.Context, spec ProjectVolumeSnapshotSpec) (observation VolumeSnapshotObservation, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_snapshot.create", "VolumeSnapshot")
	defer func() { end(err) }()
	if c.dynamic == nil {
		return VolumeSnapshotObservation{}, ErrVolumeSnapshotUnsupported
	}
	if err := validateProjectVolumeSnapshotSpec(spec); err != nil {
		return VolumeSnapshotObservation{}, err
	}
	claim, err := c.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(ctx, spec.SourceClaimName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return VolumeSnapshotObservation{}, ErrProjectVolumeClaimNotFound
	}
	if err != nil {
		return VolumeSnapshotObservation{}, err
	}
	if spec.ManagedClaim {
		if err := ensureProjectVolumeOwnership(claim.Labels, spec.ProjectID, spec.VolumeID); err != nil {
			return VolumeSnapshotObservation{}, err
		}
	} else if projectID := strings.TrimSpace(claim.Labels[ProjectIDLabel]); projectID != "" && projectID != spec.ProjectID {
		return VolumeSnapshotObservation{}, ErrProjectVolumeOwnershipConflict
	}

	className, err := c.resolveVolumeSnapshotClass(ctx, valueOrEmpty(claim.Spec.StorageClassName), spec.SnapshotClassName)
	if err != nil {
		return VolumeSnapshotObservation{}, err
	}
	snapshot := buildVolumeSnapshot(spec, className)
	resource := c.dynamic.Resource(volumeSnapshotGVR).Namespace(spec.Namespace)
	existing, err := resource.Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		created, createErr := resource.Create(ctx, snapshot, metav1.CreateOptions{})
		if createErr != nil {
			if snapshotAPIUnavailable(createErr) {
				return VolumeSnapshotObservation{}, ErrVolumeSnapshotUnsupported
			}
			return VolumeSnapshotObservation{}, createErr
		}
		return observeVolumeSnapshot(created), nil
	}
	if snapshotAPIUnavailable(err) {
		return VolumeSnapshotObservation{}, ErrVolumeSnapshotUnsupported
	}
	if err != nil {
		return VolumeSnapshotObservation{}, err
	}
	if err := ensureVolumeSnapshotMatches(existing, spec, className); err != nil {
		return VolumeSnapshotObservation{}, err
	}
	return observeVolumeSnapshot(existing), nil
}

func (c *Client) ObserveVolumeSnapshot(ctx context.Context, namespace, name string) (observation VolumeSnapshotObservation, err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_snapshot.get", "VolumeSnapshot")
	defer func() { end(err) }()
	if c.dynamic == nil {
		return VolumeSnapshotObservation{}, ErrVolumeSnapshotUnsupported
	}
	if err := validateNamespacedResource(namespace, name); err != nil {
		return VolumeSnapshotObservation{}, err
	}
	snapshot, err := c.dynamic.Resource(volumeSnapshotGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return VolumeSnapshotObservation{}, ErrVolumeSnapshotNotFound
	}
	if snapshotAPIUnavailable(err) {
		return VolumeSnapshotObservation{}, ErrVolumeSnapshotUnsupported
	}
	if err != nil {
		return VolumeSnapshotObservation{}, err
	}
	return observeVolumeSnapshot(snapshot), nil
}

func (c *Client) DeleteVolumeSnapshot(ctx context.Context, namespace, name, projectID, volumeID string) (err error) {
	ctx, end := startKubernetesVolumeOperation(ctx, "volume_snapshot.delete", "VolumeSnapshot")
	defer func() { end(err) }()
	if c.dynamic == nil {
		return ErrVolumeSnapshotUnsupported
	}
	if err := validateNamespacedResource(namespace, name); err != nil {
		return err
	}
	resource := c.dynamic.Resource(volumeSnapshotGVR).Namespace(namespace)
	snapshot, err := resource.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if snapshotAPIUnavailable(err) {
		return ErrVolumeSnapshotUnsupported
	}
	if err != nil {
		return err
	}
	if err := ensureProjectVolumeOwnership(snapshot.GetLabels(), projectID, volumeID); err != nil {
		return err
	}
	policy := metav1.DeletePropagationForeground
	return resource.Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &policy})
}

func (c *Client) resolveVolumeSnapshotClass(ctx context.Context, storageClassName, requestedClassName string) (string, error) {
	storageClassName = strings.TrimSpace(storageClassName)
	if storageClassName == "" {
		return "", ErrVolumeSnapshotUnsupported
	}
	class, err := c.client.StorageV1().StorageClasses().Get(ctx, storageClassName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	classes, err := c.listVolumeSnapshotClasses(ctx)
	if err != nil {
		return "", err
	}
	capability := snapshotCapabilityForProvisioner(classes, class.Provisioner)
	if !capability.Supported {
		return "", ErrVolumeSnapshotUnsupported
	}
	requestedClassName = strings.TrimSpace(requestedClassName)
	if requestedClassName != "" {
		for _, name := range capability.SnapshotClassNames {
			if name == requestedClassName {
				return requestedClassName, nil
			}
		}
		return "", ErrVolumeSnapshotUnsupported
	}
	if capability.DefaultSnapshotClassName != "" {
		return capability.DefaultSnapshotClassName, nil
	}
	if len(capability.SnapshotClassNames) == 1 {
		return capability.SnapshotClassNames[0], nil
	}
	return "", fmt.Errorf("%w: a snapshot class must be selected", ErrInvalidProjectVolumeSpec)
}

func (c *Client) listVolumeSnapshotClasses(ctx context.Context) ([]volumeSnapshotClass, error) {
	if c.dynamic == nil {
		return nil, ErrVolumeSnapshotUnsupported
	}
	list, err := c.dynamic.Resource(volumeSnapshotClassGVR).List(ctx, metav1.ListOptions{})
	if snapshotAPIUnavailable(err) {
		return nil, ErrVolumeSnapshotUnsupported
	}
	if err != nil {
		return nil, err
	}
	classes := make([]volumeSnapshotClass, 0, len(list.Items))
	for i := range list.Items {
		driver, _, _ := unstructured.NestedString(list.Items[i].Object, "driver")
		if strings.TrimSpace(driver) == "" {
			continue
		}
		classes = append(classes, volumeSnapshotClass{
			Name:      list.Items[i].GetName(),
			Driver:    driver,
			IsDefault: strings.EqualFold(list.Items[i].GetAnnotations()[defaultVolumeSnapshotClassAnnotation], "true"),
		})
	}
	sort.Slice(classes, func(i, j int) bool { return classes[i].Name < classes[j].Name })
	return classes, nil
}

func snapshotCapabilityForProvisioner(classes []volumeSnapshotClass, provisioner string) VolumeSnapshotCapability {
	capability := VolumeSnapshotCapability{}
	for _, class := range classes {
		if class.Driver != provisioner {
			continue
		}
		capability.Supported = true
		capability.SnapshotClassNames = append(capability.SnapshotClassNames, class.Name)
		if class.IsDefault && capability.DefaultSnapshotClassName == "" {
			capability.DefaultSnapshotClassName = class.Name
		}
	}
	return capability
}

func buildVolumeSnapshot(spec ProjectVolumeSnapshotSpec, className string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "snapshot.storage.k8s.io/v1",
		"kind":       "VolumeSnapshot",
		"metadata": map[string]any{
			"name":      spec.Name,
			"namespace": spec.Namespace,
			"labels": map[string]any{
				ManagedByLabel:       ManagedByValue,
				ProjectIDLabel:       spec.ProjectID,
				ProjectVolumeIDLabel: spec.VolumeID,
			},
		},
		"spec": map[string]any{
			"volumeSnapshotClassName": className,
			"source": map[string]any{
				"persistentVolumeClaimName": spec.SourceClaimName,
			},
		},
	}}
}

func validateProjectVolumeSnapshotSpec(spec ProjectVolumeSnapshotSpec) error {
	if err := validateNamespacedResource(spec.Namespace, spec.Name); err != nil {
		return err
	}
	if len(validation.IsDNS1123Subdomain(strings.TrimSpace(spec.SourceClaimName))) > 0 {
		return fmt.Errorf("%w: source claim name", ErrInvalidProjectVolumeSpec)
	}
	if strings.TrimSpace(spec.ProjectID) == "" || strings.TrimSpace(spec.ProjectID) != spec.ProjectID || len(validation.IsValidLabelValue(spec.ProjectID)) > 0 {
		return fmt.Errorf("%w: project ID", ErrInvalidProjectVolumeSpec)
	}
	if strings.TrimSpace(spec.VolumeID) == "" || strings.TrimSpace(spec.VolumeID) != spec.VolumeID || len(validation.IsValidLabelValue(spec.VolumeID)) > 0 {
		return fmt.Errorf("%w: volume ID", ErrInvalidProjectVolumeSpec)
	}
	if spec.SnapshotClassName != "" && len(validation.IsDNS1123Subdomain(spec.SnapshotClassName)) > 0 {
		return fmt.Errorf("%w: snapshot class", ErrInvalidProjectVolumeSpec)
	}
	return nil
}

func ensureVolumeSnapshotMatches(snapshot *unstructured.Unstructured, spec ProjectVolumeSnapshotSpec, className string) error {
	if err := ensureProjectVolumeOwnership(snapshot.GetLabels(), spec.ProjectID, spec.VolumeID); err != nil {
		return err
	}
	sourceClaimName, _, _ := unstructured.NestedString(snapshot.Object, "spec", "source", "persistentVolumeClaimName")
	existingClassName, _, _ := unstructured.NestedString(snapshot.Object, "spec", "volumeSnapshotClassName")
	if sourceClaimName != spec.SourceClaimName || existingClassName != className {
		return ErrProjectVolumeSpecConflict
	}
	return nil
}

func observeVolumeSnapshot(snapshot *unstructured.Unstructured) VolumeSnapshotObservation {
	ready, _, _ := unstructured.NestedBool(snapshot.Object, "status", "readyToUse")
	sourceClaimName, _, _ := unstructured.NestedString(snapshot.Object, "spec", "source", "persistentVolumeClaimName")
	className, _, _ := unstructured.NestedString(snapshot.Object, "spec", "volumeSnapshotClassName")
	boundContent, _, _ := unstructured.NestedString(snapshot.Object, "status", "boundVolumeSnapshotContentName")
	restoreSize, _, _ := unstructured.NestedString(snapshot.Object, "status", "restoreSize")
	_, hasError, _ := unstructured.NestedMap(snapshot.Object, "status", "error")
	observation := VolumeSnapshotObservation{
		Name:                 snapshot.GetName(),
		Exists:               true,
		ReadyToUse:           ready,
		SourceClaimName:      sourceClaimName,
		SnapshotClassName:    className,
		BoundSnapshotContent: boundContent,
		RestoreSize:          restoreSize,
		ObservedAt:           time.Now().UTC(),
	}
	if hasError {
		observation.ErrorCode = volumeSnapshotFailureCode
	}
	return observation
}
