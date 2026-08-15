package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestProjectVolumeProviderE2E is an opt-in destructive test for a disposable
// Kubernetes cluster. It never uses the process default kubeconfig, so running
// the normal test suite cannot accidentally target a developer cluster.
func TestProjectVolumeProviderE2E(t *testing.T) {
	kubeconfigPath := os.Getenv("VOLUME_E2E_KUBECONFIG")
	if kubeconfigPath == "" {
		t.Skip("VOLUME_E2E_KUBECONFIG is not configured")
	}
	kubeconfig, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		t.Fatalf("read disposable kubeconfig: %v", err)
	}
	client, err := NewClientFromKubeconfig(string(kubeconfig))
	if err != nil {
		t.Fatalf("create Kubernetes client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	runID := fmt.Sprintf("%x", time.Now().UnixNano())
	namespace := "luna-volume-e2e-" + runID
	projectID := "prj_volume_e2e"
	volumeID := "pvol_volume_e2e_" + runID
	claimName := "luna-volume-e2e"
	if err := client.EnsureNamespace(ctx, namespace, ProjectNamespaceLabels(projectID)); err != nil {
		t.Fatalf("ensure namespace: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		_ = client.DeleteProjectVolumeClaim(cleanupCtx, namespace, claimName, projectID, volumeID)
		_ = client.client.CoreV1().Namespaces().Delete(cleanupCtx, namespace, metav1.DeleteOptions{})
	})
	classes, err := client.ListVolumeStorageClasses(ctx)
	if err != nil || len(classes) == 0 {
		t.Fatalf("list storage classes: items=%v error=%v", classes, err)
	}
	storageClass := classes[0].Name
	for _, class := range classes {
		if class.IsDefault {
			storageClass = class.Name
			break
		}
	}
	spec := ProjectVolumeClaimSpec{
		ProjectID: projectID, VolumeID: volumeID, Namespace: namespace, ClaimName: claimName,
		Capacity: "64Mi", StorageClassName: storageClass,
		AccessMode: string(corev1.ReadWriteOnce), VolumeMode: string(corev1.PersistentVolumeFilesystem),
	}
	created, err := client.CreateProjectVolumeClaim(ctx, spec)
	if err != nil || !created.Exists || created.RequestedCapacity == "" {
		t.Fatalf("create PVC: observation=%#v error=%v", created, err)
	}
	observed, err := client.ObserveProjectVolumeClaim(ctx, namespace, claimName)
	if err != nil || !observed.Exists || observed.StorageClassName != storageClass {
		t.Fatalf("observe PVC: observation=%#v error=%v", observed, err)
	}
	inspection, err := client.InspectExistingProjectVolumeClaim(ctx, ExistingProjectVolumeClaimSpec{
		ProjectID: projectID, VolumeID: volumeID, Namespace: namespace, ClaimName: claimName,
		ExpectedCapacity: "64Mi", ExpectedStorageClassName: storageClass,
		ExpectedAccessMode: string(corev1.ReadWriteOnce), ExpectedVolumeMode: string(corev1.PersistentVolumeFilesystem),
	})
	if err != nil || inspection.ProjectID != projectID || inspection.ProjectVolumeID != volumeID {
		t.Fatalf("inspect owned PVC: inspection=%#v error=%v", inspection, err)
	}
	if _, err := client.InspectExistingProjectVolumeClaim(ctx, ExistingProjectVolumeClaimSpec{
		ProjectID: "prj_other", VolumeID: "pvol_other", Namespace: namespace, ClaimName: claimName,
	}); !errors.Is(err, ErrProjectVolumeOwnershipConflict) {
		t.Fatalf("cross-project inspection error=%v, want ownership conflict", err)
	}
	cancelledCtx, cancelRequest := context.WithCancel(ctx)
	cancelRequest()
	if _, err := client.ObserveProjectVolumeClaim(cancelledCtx, namespace, claimName); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled observation error=%v, want context canceled", err)
	}
	if err := client.DeleteProjectVolumeClaim(ctx, namespace, claimName, projectID, volumeID); err != nil {
		t.Fatalf("delete PVC: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		_, err = client.ObserveProjectVolumeClaim(ctx, namespace, claimName)
		if errors.Is(err, ErrProjectVolumeClaimNotFound) {
			return
		}
		if err != nil {
			t.Fatalf("observe PVC deletion: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("PVC was not deleted before the deadline")
}
