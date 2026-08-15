package kubernetes

import (
	"context"
	"errors"
	"testing"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestListVolumeStorageClassesIncludesSnapshotCapability(t *testing.T) {
	allowExpansion := true
	defaultBinding := storagev1.VolumeBindingWaitForFirstConsumer
	classes := kubefake.NewSimpleClientset(
		&storagev1.StorageClass{
			ObjectMeta:  metav1.ObjectMeta{Name: "slow", Annotations: map[string]string{}},
			Provisioner: "other.csi.example.com",
		},
		&storagev1.StorageClass{
			ObjectMeta:           metav1.ObjectMeta{Name: "standard", Annotations: map[string]string{defaultStorageClassAnnotation: "true"}},
			Provisioner:          "csi.example.com",
			AllowVolumeExpansion: &allowExpansion,
			VolumeBindingMode:    &defaultBinding,
		},
	)
	snapshotClass := volumeSnapshotClassObject("csi-snapshots", "csi.example.com", true)
	dynamicClient := newVolumeSnapshotDynamicClient(snapshotClass)
	client := NewClientForInterfaces(classes, dynamicClient)

	items, err := client.ListVolumeStorageClasses(context.Background())
	if err != nil {
		t.Fatalf("ListVolumeStorageClasses() error = %v", err)
	}
	if len(items) != 2 || items[0].Name != "slow" || items[1].Name != "standard" {
		t.Fatalf("storage classes = %#v", items)
	}
	standard := items[1]
	if !standard.IsDefault || !standard.AllowVolumeExpansion || !standard.SnapshotSupported || standard.DefaultSnapshotClass != "csi-snapshots" || standard.VolumeBindingMode != string(defaultBinding) {
		t.Fatalf("standard storage class = %#v", standard)
	}

	capability, err := client.DetectSnapshotSupport(context.Background(), "standard")
	if err != nil {
		t.Fatalf("DetectSnapshotSupport() error = %v", err)
	}
	if !capability.Supported || capability.DefaultSnapshotClassName != "csi-snapshots" || len(capability.SnapshotClassNames) != 1 {
		t.Fatalf("snapshot capability = %#v", capability)
	}
}

func TestSnapshotCapabilityGracefullyDegradesWithoutCRDs(t *testing.T) {
	class := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "standard"}, Provisioner: "csi.example.com"}
	client := NewClientForInterface(kubefake.NewSimpleClientset(class))

	items, err := client.ListVolumeStorageClasses(context.Background())
	if err != nil {
		t.Fatalf("ListVolumeStorageClasses() error = %v", err)
	}
	if len(items) != 1 || items[0].SnapshotSupported {
		t.Fatalf("storage classes without snapshot CRDs = %#v", items)
	}
	capability, err := client.DetectSnapshotSupport(context.Background(), "standard")
	if err != nil || capability.Supported {
		t.Fatalf("DetectSnapshotSupport() = %#v, %v", capability, err)
	}
	if _, err := client.CreateVolumeSnapshot(context.Background(), ProjectVolumeSnapshotSpec{}); !errors.Is(err, ErrVolumeSnapshotUnsupported) {
		t.Fatalf("CreateVolumeSnapshot() error = %v", err)
	}
}

func TestCreateObserveAndDeleteVolumeSnapshot(t *testing.T) {
	storageClass := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "standard"}, Provisioner: "csi.example.com"}
	claim := managedProjectVolumeClaim("luna-demo", "claim-a", "proj_demo", "pvol_a", "10Gi", "standard")
	clientset := kubefake.NewSimpleClientset(storageClass, claim)
	dynamicClient := newVolumeSnapshotDynamicClient(volumeSnapshotClassObject("csi-snapshots", "csi.example.com", true))
	client := NewClientForInterfaces(clientset, dynamicClient)
	spec := ProjectVolumeSnapshotSpec{
		ProjectID:       "proj_demo",
		VolumeID:        "pvol_a",
		Namespace:       "luna-demo",
		Name:            "luna-vsnap-demo",
		SourceClaimName: "claim-a",
		ManagedClaim:    true,
	}

	created, err := client.CreateVolumeSnapshot(context.Background(), spec)
	if err != nil {
		t.Fatalf("CreateVolumeSnapshot() error = %v", err)
	}
	if !created.Exists || created.SnapshotClassName != "csi-snapshots" || created.SourceClaimName != "claim-a" {
		t.Fatalf("created snapshot = %#v", created)
	}
	if _, err := client.CreateVolumeSnapshot(context.Background(), spec); err != nil {
		t.Fatalf("idempotent CreateVolumeSnapshot() error = %v", err)
	}

	resource := dynamicClient.Resource(volumeSnapshotGVR).Namespace(spec.Namespace)
	snapshot, err := resource.Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if err := unstructured.SetNestedField(snapshot.Object, true, "status", "readyToUse"); err != nil {
		t.Fatalf("set ready status: %v", err)
	}
	if err := unstructured.SetNestedField(snapshot.Object, "snapshot-content-demo", "status", "boundVolumeSnapshotContentName"); err != nil {
		t.Fatalf("set content status: %v", err)
	}
	if err := unstructured.SetNestedField(snapshot.Object, "10Gi", "status", "restoreSize"); err != nil {
		t.Fatalf("set restore size: %v", err)
	}
	if _, err := resource.Update(context.Background(), snapshot, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update snapshot: %v", err)
	}

	observation, err := client.ObserveVolumeSnapshot(context.Background(), spec.Namespace, spec.Name)
	if err != nil {
		t.Fatalf("ObserveVolumeSnapshot() error = %v", err)
	}
	if !observation.ReadyToUse || observation.RestoreSize != "10Gi" || observation.BoundSnapshotContent != "snapshot-content-demo" {
		t.Fatalf("snapshot observation = %#v", observation)
	}
	if err := client.DeleteVolumeSnapshot(context.Background(), spec.Namespace, spec.Name, spec.ProjectID, spec.VolumeID); err != nil {
		t.Fatalf("DeleteVolumeSnapshot() error = %v", err)
	}
	if _, err := resource.Get(context.Background(), spec.Name, metav1.GetOptions{}); err == nil {
		t.Fatal("snapshot still exists after delete")
	}
}

func TestVolumeSnapshotDoesNotExposeRawControllerError(t *testing.T) {
	snapshot := buildVolumeSnapshot(ProjectVolumeSnapshotSpec{
		ProjectID: "proj_demo", VolumeID: "pvol_demo", Namespace: "luna-demo", Name: "luna-vsnap-demo", SourceClaimName: "claim-a",
	}, "csi-snapshots")
	snapshot.Object["status"] = map[string]any{
		"readyToUse": false,
		"error": map[string]any{
			"message": "provider internal URL https://secret.invalid and filesystem path /var/lib/csi",
		},
	}
	observation := observeVolumeSnapshot(snapshot)
	if observation.ErrorCode != volumeSnapshotFailureCode {
		t.Fatalf("snapshot error observation = %#v", observation)
	}
}

func newVolumeSnapshotDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		volumeSnapshotClassGVR: "VolumeSnapshotClassList",
		volumeSnapshotGVR:      "VolumeSnapshotList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...)
}

func volumeSnapshotClassObject(name, driver string, isDefault bool) *unstructured.Unstructured {
	annotations := map[string]any{}
	if isDefault {
		annotations[defaultVolumeSnapshotClassAnnotation] = "true"
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "snapshot.storage.k8s.io/v1",
		"kind":       "VolumeSnapshotClass",
		"metadata": map[string]any{
			"name":        name,
			"annotations": annotations,
		},
		"driver":         driver,
		"deletionPolicy": "Delete",
	}}
}
