package kubernetes

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

func (c *Client) StreamDataArchive(ctx context.Context, spec DataExportSpec, output io.Writer) error {
	if c.restConfig == nil {
		return fmt.Errorf("kubernetes rest config is required")
	}
	exportVolumes := spec.Volumes
	if len(exportVolumes) == 0 && strings.TrimSpace(spec.PVCName) != "" {
		exportVolumes = []DataExportVolume{{Name: "data", PVCName: spec.PVCName}}
	}
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" || len(exportVolumes) == 0 {
		return fmt.Errorf("data export name, namespace and pvc name are required")
	}
	for _, volume := range exportVolumes {
		if strings.TrimSpace(volume.Name) == "" || strings.TrimSpace(volume.PVCName) == "" {
			return fmt.Errorf("data export volume name and pvc name are required")
		}
	}
	podName, err := dataExportPodName(firstNonEmpty(spec.Name, "data-export"))
	if err != nil {
		return err
	}
	singleVolume := len(exportVolumes) == 1
	mountPath := firstNonEmpty(spec.MountPath, "/data")
	volumeMounts := make([]corev1.VolumeMount, 0, len(exportVolumes))
	volumes := make([]corev1.Volume, 0, len(exportVolumes))
	for _, volume := range exportVolumes {
		name := persistentDataVolumeName(ApplicationDataVolume{Name: volume.Name})
		targetMountPath := mountPath
		if !singleVolume {
			targetMountPath = "/mnt/" + name
		}
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: name, MountPath: targetMountPath, ReadOnly: true})
		volumes = append(volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: volume.PVCName,
				ReadOnly:  true,
			}},
		})
	}
	tarRoot := mountPath
	if !singleVolume {
		tarRoot = "/mnt"
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: spec.Namespace, Labels: baseManagedLabels(podName)},
		Spec: corev1.PodSpec{
			AutomountServiceAccountToken: boolPtr(false),
			RestartPolicy:                corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:         "export",
				Image:        "busybox:1.36",
				Command:      []string{"sh", "-c", "sleep 300"},
				VolumeMounts: volumeMounts,
			}},
			Volumes: volumes,
		},
	}
	if _, err := c.client.CoreV1().Pods(spec.Namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return err
	}
	defer func() {
		// Cleanup must outlive a canceled export request so the temporary pod is
		// still removed after a client disconnects.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.client.CoreV1().Pods(spec.Namespace).Delete(cleanupCtx, podName, metav1.DeleteOptions{})
	}()
	if err := c.waitForPodRunning(ctx, spec.Namespace, podName); err != nil {
		return err
	}
	req := c.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(spec.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "export",
			Command:   []string{"tar", "czf", "-", "-C", tarRoot, "."},
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: output, Stderr: &stderr}); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}
	return nil
}

func dataExportPodName(base string) (string, error) {
	var randomBytes [8]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", fmt.Errorf("generate data export pod name: %w", err)
	}
	suffix := hex.EncodeToString(randomBytes[:])
	normalized := dnsLabel(firstNonEmpty(base, "data-export"))
	maxBaseLength := 63 - len(suffix) - 1
	if len(normalized) > maxBaseLength {
		normalized = strings.Trim(normalized[:maxBaseLength], "-")
	}
	if normalized == "" {
		normalized = "data-export"
	}
	return normalized + "-" + suffix, nil
}

func (c *Client) waitForPodRunning(ctx context.Context, namespace string, name string) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		pod, err := c.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			return false, fmt.Errorf("export pod finished before streaming: %s", pod.Status.Phase)
		}
		return pod.Status.Phase == corev1.PodRunning, nil
	})
}
