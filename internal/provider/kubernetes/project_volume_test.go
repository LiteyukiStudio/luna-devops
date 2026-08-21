package kubernetes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestCreateProjectVolumeClaimIsIdempotentAndOwned(t *testing.T) {
	clientset := kubefake.NewSimpleClientset()
	client := NewClientForInterface(clientset)
	spec := ProjectVolumeClaimSpec{
		ProjectID:          "proj_demo",
		VolumeID:           "pvol_demo",
		Namespace:          "luna-demo",
		ClaimName:          "luna-pvol-demo",
		Capacity:           "20Gi",
		StorageClassName:   "standard",
		AccessMode:         string(corev1.ReadWriteOnce),
		VolumeMode:         string(corev1.PersistentVolumeFilesystem),
		SourceSnapshotName: "snapshot-demo",
	}

	observation, err := client.CreateProjectVolumeClaim(context.Background(), spec)
	if err != nil {
		t.Fatalf("CreateProjectVolumeClaim() error = %v", err)
	}
	if !observation.Exists || observation.RequestedCapacity != "20Gi" {
		t.Fatalf("observation = %#v", observation)
	}
	claim, err := clientset.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(context.Background(), spec.ClaimName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get claim: %v", err)
	}
	if claim.Labels[ProjectIDLabel] != spec.ProjectID || claim.Labels[ProjectVolumeIDLabel] != spec.VolumeID || claim.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("claim labels = %#v", claim.Labels)
	}
	if claim.Spec.DataSource == nil || claim.Spec.DataSource.Name != spec.SourceSnapshotName || claim.Spec.DataSource.APIGroup == nil || *claim.Spec.DataSource.APIGroup != "snapshot.storage.k8s.io" {
		t.Fatalf("claim data source = %#v", claim.Spec.DataSource)
	}

	if _, err := client.CreateProjectVolumeClaim(context.Background(), spec); err != nil {
		t.Fatalf("idempotent CreateProjectVolumeClaim() error = %v", err)
	}
	conflicting := spec
	conflicting.StorageClassName = "premium"
	if _, err := client.CreateProjectVolumeClaim(context.Background(), conflicting); !errors.Is(err, ErrProjectVolumeSpecConflict) {
		t.Fatalf("conflicting create error = %v", err)
	}
}

func TestObserveProjectVolumeClaimsReturnsCurrentPageMap(t *testing.T) {
	claim := managedProjectVolumeClaim("luna-demo", "claim-a", "proj_demo", "pvol_a", "10Gi", "standard")
	claim.CreationTimestamp = metav1.NewTime(time.Date(2026, 8, 15, 6, 20, 0, 0, time.UTC))
	other := managedProjectVolumeClaim("luna-demo", "claim-other", "proj_other", "pvol_other", "10Gi", "standard")
	client := NewClientForInterface(kubefake.NewSimpleClientset(claim, other))

	observations, err := client.ObserveProjectVolumeClaims(context.Background(), "luna-demo", "proj_demo", []string{"claim-a", "claim-missing"})
	if err != nil {
		t.Fatalf("ObserveProjectVolumeClaims() error = %v", err)
	}
	if !observations["claim-a"].Exists || observations["claim-missing"].Exists ||
		!observations["claim-a"].CreatedAt.Equal(claim.CreationTimestamp.Time) {
		t.Fatalf("observations = %#v", observations)
	}
	if _, ok := observations["claim-other"]; ok {
		t.Fatalf("unrequested claim leaked into observations: %#v", observations)
	}
}

func TestAdoptExistingProjectVolumeClaimIsRejectedWhenInUse(t *testing.T) {
	storageClass := "standard"
	volumeMode := corev1.PersistentVolumeFilesystem
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy-data", Namespace: "luna-demo"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClass,
			VolumeMode:       &volumeMode,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy-app", Namespace: "luna-demo"},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
			Name:         "data",
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim.Name}},
		}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	clientset := kubefake.NewSimpleClientset(claim, pod)
	client := NewClientForInterface(clientset)
	spec := ExistingProjectVolumeClaimSpec{
		ProjectID: "proj_demo", VolumeID: "pvol_demo", Namespace: "luna-demo", ClaimName: "legacy-data",
		ExpectedCapacity: "10Gi", ExpectedStorageClassName: storageClass,
		ExpectedAccessMode: string(corev1.ReadWriteOnce), ExpectedVolumeMode: string(volumeMode),
	}

	inspection, err := client.InspectExistingProjectVolumeClaim(context.Background(), spec)
	if err != nil {
		t.Fatalf("InspectExistingProjectVolumeClaim() error = %v", err)
	}
	if inspection.ActivePodReferences != 1 {
		t.Fatalf("active references = %d", inspection.ActivePodReferences)
	}
	if _, err := client.AdoptExistingProjectVolumeClaim(context.Background(), spec); !errors.Is(err, ErrProjectVolumeClaimInUse) {
		t.Fatalf("adopt in-use error = %v", err)
	}

	pod.Status.Phase = corev1.PodSucceeded
	if _, err := clientset.CoreV1().Pods(pod.Namespace).Update(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update pod: %v", err)
	}
	if _, err := client.AdoptExistingProjectVolumeClaim(context.Background(), spec); err != nil {
		t.Fatalf("AdoptExistingProjectVolumeClaim() error = %v", err)
	}
	adopted, err := clientset.CoreV1().PersistentVolumeClaims(spec.Namespace).Get(context.Background(), spec.ClaimName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get adopted claim: %v", err)
	}
	if adopted.Labels[ProjectIDLabel] != spec.ProjectID || adopted.Labels[ProjectVolumeIDLabel] != spec.VolumeID || adopted.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("adopted labels = %#v", adopted.Labels)
	}
}

func TestAdoptExistingProjectVolumeClaimRejectsSpecChangedAfterAPICheck(t *testing.T) {
	claim := managedProjectVolumeClaim("luna-demo", "legacy-data", "", "", "20Gi", "premium")
	claim.Labels = nil
	clientset := kubefake.NewSimpleClientset(claim)
	client := NewClientForInterface(clientset)
	_, err := client.AdoptExistingProjectVolumeClaim(context.Background(), ExistingProjectVolumeClaimSpec{
		ProjectID: "proj_demo", VolumeID: "pvol_demo", Namespace: "luna-demo", ClaimName: "legacy-data",
		ExpectedCapacity: "10Gi", ExpectedStorageClassName: "standard",
		ExpectedAccessMode: string(corev1.ReadWriteOnce), ExpectedVolumeMode: string(corev1.PersistentVolumeFilesystem),
	})
	if !errors.Is(err, ErrProjectVolumeSpecConflict) {
		t.Fatalf("changed claim specification error = %v", err)
	}
	current, getErr := clientset.CoreV1().PersistentVolumeClaims("luna-demo").Get(context.Background(), "legacy-data", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("get claim: %v", getErr)
	}
	if current.Labels[ManagedByLabel] != "" || current.Labels[ProjectVolumeIDLabel] != "" {
		t.Fatalf("conflicting claim was adopted: %#v", current.Labels)
	}
}

func TestInspectExistingProjectVolumeClaimRejectsConflictingOwnership(t *testing.T) {
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "legacy-data", Namespace: "luna-demo", Labels: map[string]string{ProjectIDLabel: "proj_other"},
	}}
	client := NewClientForInterface(kubefake.NewSimpleClientset(claim))
	_, err := client.InspectExistingProjectVolumeClaim(context.Background(), ExistingProjectVolumeClaimSpec{
		ProjectID: "proj_demo", VolumeID: "pvol_demo", Namespace: "luna-demo", ClaimName: "legacy-data",
	})
	if !errors.Is(err, ErrProjectVolumeOwnershipConflict) {
		t.Fatalf("ownership conflict error = %v", err)
	}
}

func TestExpandAndDeleteProjectVolumeClaim(t *testing.T) {
	claim := managedProjectVolumeClaim("luna-demo", "claim-a", "proj_demo", "pvol_a", "10Gi", "standard")
	allowExpansion := true
	class := &storagev1.StorageClass{
		ObjectMeta:           metav1.ObjectMeta{Name: "standard"},
		Provisioner:          "csi.example.com",
		AllowVolumeExpansion: &allowExpansion,
	}
	clientset := kubefake.NewSimpleClientset(claim, class)
	client := NewClientForInterface(clientset)

	if _, err := client.ExpandProjectVolumeClaim(context.Background(), "luna-demo", "claim-a", "proj_demo", "pvol_a", "5Gi"); !errors.Is(err, ErrVolumeCapacityShrinkForbidden) {
		t.Fatalf("shrink error = %v", err)
	}
	observation, err := client.ExpandProjectVolumeClaim(context.Background(), "luna-demo", "claim-a", "proj_demo", "pvol_a", "20Gi")
	if err != nil {
		t.Fatalf("ExpandProjectVolumeClaim() error = %v", err)
	}
	if observation.RequestedCapacity != "20Gi" {
		t.Fatalf("expanded observation = %#v", observation)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "consumer", Namespace: "luna-demo"},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
			Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "claim-a"}},
		}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if _, err := clientset.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pod: %v", err)
	}
	if err := client.DeleteProjectVolumeClaim(context.Background(), "luna-demo", "claim-a", "proj_demo", "pvol_a"); !errors.Is(err, ErrProjectVolumeClaimInUse) {
		t.Fatalf("delete in-use error = %v", err)
	}
	pod.Status.Phase = corev1.PodFailed
	if _, err := clientset.CoreV1().Pods(pod.Namespace).Update(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update pod: %v", err)
	}
	if err := client.DeleteProjectVolumeClaim(context.Background(), "luna-demo", "claim-a", "proj_demo", "pvol_a"); err != nil {
		t.Fatalf("DeleteProjectVolumeClaim() error = %v", err)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims("luna-demo").Get(context.Background(), "claim-a", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("deleted claim get error = %v", err)
	}
}

func TestProjectVolumeProviderSpanKeepsParentWithoutResourceIdentifiers(t *testing.T) {
	previous := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	})

	claim := managedProjectVolumeClaim("luna-sensitive", "claim-sensitive", "proj_sensitive", "pvol_sensitive", "10Gi", "standard")
	client := NewClientForInterface(kubefake.NewSimpleClientset(claim))
	parentCtx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	_, err := client.ObserveProjectVolumeClaim(parentCtx, claim.Namespace, claim.Name)
	parent.End()
	if err != nil {
		t.Fatalf("ObserveProjectVolumeClaim() error = %v", err)
	}

	var operation sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "kubernetes.volume_claim.get" {
			operation = span
			break
		}
	}
	if operation == nil {
		t.Fatal("Kubernetes volume operation span not recorded")
	}
	if operation.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatalf("operation parent = %s, want %s", operation.Parent().SpanID(), parent.SpanContext().SpanID())
	}
	for _, attr := range operation.Attributes() {
		value := attr.Value.Emit()
		if strings.Contains(value, "sensitive") {
			t.Fatalf("resource identifier leaked in telemetry attribute %s=%q", attr.Key, value)
		}
	}
}

func managedProjectVolumeClaim(namespace, name, projectID, volumeID, capacity, storageClass string) *corev1.PersistentVolumeClaim {
	quantity := resource.MustParse(capacity)
	mode := corev1.PersistentVolumeFilesystem
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{
			ManagedByLabel: ManagedByValue, ProjectIDLabel: projectID, ProjectVolumeIDLabel: volumeID,
		}},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClass,
			VolumeMode:       &mode,
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: quantity}},
		},
	}
}
