package kubernetes

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestObserveApplicationVolumeAttachmentsReadsWorkloadTemplate(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "project"},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "files", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "shared", ReadOnly: true}}},
				{Name: "scratch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			Containers: []corev1.Container{{
				Name: "app",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "files", MountPath: "/srv/data"},
					{Name: "scratch", MountPath: "/tmp/work"},
				},
			}},
		}}},
	}
	client := NewClientForInterface(fake.NewSimpleClientset(deployment))
	attachments, err := client.ObserveApplicationVolumeAttachments(context.Background(), "project", "app", "Deployment")
	if err != nil {
		t.Fatal(err)
	}
	if got := attachments["files"]; got.ClaimName != "shared" || got.MountPath != "/srv/data" || !got.ReadOnly || got.EmptyDir {
		t.Fatalf("files attachment = %+v", got)
	}
	if got := attachments["scratch"]; !got.EmptyDir || got.MountPath != "/tmp/work" || got.ClaimName != "" {
		t.Fatalf("scratch attachment = %+v", got)
	}
}

func TestObserveApplicationVolumeAttachmentsReadsBlockDevice(t *testing.T) {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "project"},
		Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes:    []corev1.Volume{{Name: "block", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "database-block"}}}},
			Containers: []corev1.Container{{Name: "app", VolumeDevices: []corev1.VolumeDevice{{Name: "block", DevicePath: "/dev/data"}}}},
		}}},
	}
	client := NewClientForInterface(fake.NewSimpleClientset(statefulSet))
	attachments, err := client.ObserveApplicationVolumeAttachments(context.Background(), "project", "database", "StatefulSet")
	if err != nil {
		t.Fatal(err)
	}
	if got := attachments["block"]; got.ClaimName != "database-block" || got.DevicePath != "/dev/data" || got.MountPath != "" {
		t.Fatalf("block attachment = %+v", got)
	}
}
