package kubernetes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/transferjob"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	VolumeTransferIDLabel = "luna.devops/volume-transfer-id"

	volumeTransferScope                       = "volume-transfer"
	volumeTransferContainerName               = "transfer"
	volumeTransferVolumeName                  = "project-volume"
	volumeTransferFilesystemPath              = "/volume"
	volumeTransferBlockDevicePath             = "/dev/luna-volume"
	volumeTransferBinaryPath                  = "/usr/local/bin/luna-volume-transfer"
	volumeTransferCleanupTimeout              = 30 * time.Second
	volumeTransferCleanupPollInterval         = 200 * time.Millisecond
	volumeTransferDefaultMaxArchiveFile       = 1_000_000
	volumeTransferControlRecordPrefix         = "LUNA_VOLUME_TRANSFER_RESULT "
	volumeTransferControlMaxBytes             = 64 * 1024
	volumeTransferMaximumBytes          int64 = 5 * 1024 * 1024 * 1024 * 1024
)

var (
	ErrInvalidVolumeTransferSpec = errors.New("volume transfer specification is invalid")
	ErrVolumeTransferConflict    = errors.New("volume transfer resources conflict with an existing transfer")
	ErrVolumeTransferNotReady    = errors.New("volume transfer pod is not ready")

	volumeTransferStreamMetricsOnce sync.Once
	volumeTransferStreamTotal       metric.Int64Counter
	volumeTransferStreamBytes       metric.Int64Counter
)

// VolumeTransferProvider prepares an isolated Pod and opens bounded direct
// stdin/stdout streams through Kubernetes exec. It never stores archive bytes.
type VolumeTransferProvider interface {
	PrepareVolumeTransfer(context.Context, VolumeTransferSpec) (VolumeTransferReference, error)
	ObserveVolumeTransfer(context.Context, string, string) (VolumeTransferObservation, error)
	OpenVolumeTransferImport(context.Context, VolumeTransferStreamTarget, io.Reader) (VolumeTransferStreamResult, error)
	OpenVolumeTransferExport(context.Context, VolumeTransferStreamTarget) (*VolumeTransferExportStream, error)
	CancelVolumeTransfer(context.Context, string, string) error
	CleanupVolumeTransfer(context.Context, string, string) error
}

type VolumeTransferSpec struct {
	TransferID      string
	ProjectID       string
	ProjectVolumeID string
	Namespace       string
	ClaimName       string
	Direction       string
	Format          string
	VolumeMode      string
	ConsistencyMode string
	Image           string
	CapacityBytes   int64
	MaxArchiveBytes int64
	ExpectedBytes   int64
	ExpectedSHA256  string
	ExportedAt      time.Time
	MaxArchiveFiles int
	ImagePullPolicy corev1.PullPolicy
}

type VolumeTransferReference struct {
	PodName           string
	Namespace         string
	ContainerName     string
	NetworkPolicyName string
}

type VolumeTransferStreamTarget struct {
	Namespace       string
	TransferID      string
	ProjectID       string
	ProjectVolumeID string
}

type VolumeTransferObservation struct {
	State      string
	Reason     string
	PodName    string
	StartedAt  *time.Time
	FinishedAt *time.Time
}

type VolumeTransferStreamResult struct {
	TransferredBytes int64
	ProcessedFiles   int64
	SHA256           string
	LogicalBytes     int64
	DataSHA256       string
}

type VolumeTransferStreamError struct{ Code string }

func (streamErr *VolumeTransferStreamError) Error() string {
	if streamErr == nil || !validVolumeTransferHelperErrorCode(streamErr.Code) {
		return transferjob.CodeJobFailed
	}
	return streamErr.Code
}

func (streamErr *VolumeTransferStreamError) TransferErrorCode() string { return streamErr.Error() }

// VolumeTransferExportStream must be closed when the client disconnects. Wait
// returns the helper's authoritative summary only after stdout reaches EOF.
type VolumeTransferExportStream struct {
	Reader io.ReadCloser
	cancel context.CancelFunc
	done   <-chan volumeTransferStreamOutcome
	once   sync.Once
	result VolumeTransferStreamResult
	err    error
}

func (stream *VolumeTransferExportStream) Wait() (VolumeTransferStreamResult, error) {
	if stream == nil || stream.done == nil {
		return VolumeTransferStreamResult{}, ErrVolumeTransferNotReady
	}
	stream.once.Do(func() {
		outcome := <-stream.done
		stream.result, stream.err = outcome.result, outcome.err
	})
	return stream.result, stream.err
}

func (stream *VolumeTransferExportStream) Close() error {
	if stream == nil {
		return nil
	}
	if stream.cancel != nil {
		stream.cancel()
	}
	if stream.Reader != nil {
		return stream.Reader.Close()
	}
	return nil
}

type volumeTransferStreamOutcome struct {
	result VolumeTransferStreamResult
	err    error
}

func (c *Client) PrepareVolumeTransfer(ctx context.Context, spec VolumeTransferSpec) (reference VolumeTransferReference, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer_pod.prepare")
	defer func() { end(err) }()
	if c == nil || c.client == nil {
		return VolumeTransferReference{}, fmt.Errorf("%w: Kubernetes client is unavailable", ErrInvalidVolumeTransferSpec)
	}
	validated, err := validateVolumeTransferSpec(ctx, spec)
	if err != nil {
		return VolumeTransferReference{}, err
	}
	spec = validated
	reference, err = volumeTransferReference(spec.Namespace, spec.TransferID)
	if err != nil {
		return VolumeTransferReference{}, err
	}
	labels := volumeTransferLabels(spec, reference.PodName)
	pod := volumeTransferPod(spec, reference.PodName, labels)
	policy := volumeTransferDenyEgressPolicy(spec.Namespace, reference.NetworkPolicyName, labels)

	pods := c.client.CoreV1().Pods(reference.Namespace)
	existing, getErr := pods.Get(ctx, reference.PodName, metav1.GetOptions{})
	if getErr == nil {
		if !sameVolumeTransferPod(existing, pod) {
			return VolumeTransferReference{}, ErrVolumeTransferConflict
		}
		return reference, nil
	}
	if !apierrors.IsNotFound(getErr) {
		return VolumeTransferReference{}, getErr
	}
	policyCreated, err := c.ensureVolumeTransferPolicy(ctx, policy)
	if err != nil {
		return VolumeTransferReference{}, err
	}
	created, err := pods.Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			existing, getErr = pods.Get(ctx, reference.PodName, metav1.GetOptions{})
			if getErr == nil && sameVolumeTransferPod(existing, pod) {
				return reference, nil
			}
			if getErr != nil {
				return VolumeTransferReference{}, getErr
			}
			return VolumeTransferReference{}, ErrVolumeTransferConflict
		}
		if policyCreated {
			_ = c.deleteVolumeTransferPolicy(ctx, reference)
		}
		return VolumeTransferReference{}, err
	}
	_ = c.attachVolumeTransferPolicyOwner(ctx, reference, created)
	return reference, nil
}

func (c *Client) ObserveVolumeTransfer(ctx context.Context, namespace, transferID string) (observation VolumeTransferObservation, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer_pod.observe")
	defer func() { end(err) }()
	if c == nil || c.client == nil {
		return VolumeTransferObservation{}, fmt.Errorf("%w: Kubernetes client is unavailable", ErrInvalidVolumeTransferSpec)
	}
	reference, err := volumeTransferReference(namespace, transferID)
	if err != nil {
		return VolumeTransferObservation{}, err
	}
	pod, err := c.client.CoreV1().Pods(reference.Namespace).Get(ctx, reference.PodName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return VolumeTransferObservation{State: "not_found", Reason: "pod_not_found", PodName: reference.PodName}, nil
	}
	if err != nil {
		return VolumeTransferObservation{}, err
	}
	return volumeTransferPodObservation(pod), nil
}

func (c *Client) OpenVolumeTransferImport(ctx context.Context, target VolumeTransferStreamTarget, source io.Reader) (result VolumeTransferStreamResult, err error) {
	ctx, end := telemetry.StartOperationWithKind(ctx, "volume", "transfer_stream.import", trace.SpanKindClient,
		attribute.String("volume.transfer.direction", "import"))
	defer func() {
		recordVolumeTransferStreamTelemetry(ctx, "import", result, err)
		end(err)
	}()
	if source == nil {
		return VolumeTransferStreamResult{}, fmt.Errorf("%w: import source is required", ErrInvalidVolumeTransferSpec)
	}
	control := &limitedBuffer{limit: volumeTransferControlMaxBytes}
	err = c.execVolumeTransfer(ctx, target, "import", source, io.Discard, control)
	result, controlErr := parseVolumeTransferControl(control.Bytes())
	if err != nil {
		return VolumeTransferStreamResult{}, errors.Join(err, controlErr)
	}
	return result, controlErr
}

func (c *Client) OpenVolumeTransferExport(ctx context.Context, target VolumeTransferStreamTarget) (*VolumeTransferExportStream, error) {
	operationCtx, end := telemetry.StartOperationWithKind(ctx, "volume", "transfer_stream.export", trace.SpanKindClient,
		attribute.String("volume.transfer.direction", "export"))
	if c == nil || c.restConfig == nil {
		err := fmt.Errorf("%w: Kubernetes REST config is unavailable", ErrInvalidVolumeTransferSpec)
		recordVolumeTransferStreamTelemetry(operationCtx, "export", VolumeTransferStreamResult{}, err)
		end(err)
		return nil, err
	}
	if _, err := c.readyVolumeTransferPod(operationCtx, target, "export"); err != nil {
		recordVolumeTransferStreamTelemetry(operationCtx, "export", VolumeTransferStreamResult{}, err)
		end(err)
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(operationCtx)
	reader, writer := io.Pipe()
	done := make(chan volumeTransferStreamOutcome, 1)
	stream := &VolumeTransferExportStream{Reader: reader, cancel: cancel, done: done}
	go func() {
		control := &limitedBuffer{limit: volumeTransferControlMaxBytes}
		err := c.execVolumeTransfer(streamCtx, target, "export", nil, writer, control)
		result, controlErr := parseVolumeTransferControl(control.Bytes())
		err = errors.Join(err, controlErr)
		_ = writer.CloseWithError(err)
		done <- volumeTransferStreamOutcome{result: result, err: err}
		close(done)
		recordVolumeTransferStreamTelemetry(operationCtx, "export", result, err)
		end(err)
	}()
	return stream, nil
}

// The helper Pod intentionally has deny-all egress, so the API-side exec span
// is the authoritative observable boundary. The helper control summary is
// recorded here without high-cardinality transfer IDs or checksums.
func recordVolumeTransferStreamTelemetry(ctx context.Context, direction string, result VolumeTransferStreamResult, err error) {
	volumeTransferStreamMetricsOnce.Do(func() {
		meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/provider/kubernetes")
		volumeTransferStreamTotal, _ = meter.Int64Counter("luna_devops_volume_transfers_total",
			metric.WithDescription("Completed direct project volume transfer streams."))
		volumeTransferStreamBytes, _ = meter.Int64Counter("luna_devops_volume_transfer_bytes_total",
			metric.WithDescription("Successfully transferred project volume archive bytes."), metric.WithUnit("By"))
	})
	outcome := telemetry.ErrorOutcome(err)
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String("volume.transfer.outcome", outcome),
		attribute.Int64("volume.transfer.transferred_bytes", result.TransferredBytes),
		attribute.Int64("volume.transfer.logical_bytes", result.LogicalBytes),
		attribute.Int64("volume.transfer.processed_files", result.ProcessedFiles),
	)
	options := metric.WithAttributes(
		attribute.String("direction", direction),
		attribute.String("outcome", outcome),
	)
	if volumeTransferStreamTotal != nil {
		volumeTransferStreamTotal.Add(ctx, 1, options)
	}
	if err == nil && result.TransferredBytes > 0 && volumeTransferStreamBytes != nil {
		volumeTransferStreamBytes.Add(ctx, result.TransferredBytes, metric.WithAttributes(attribute.String("direction", direction)))
	}
}

func (c *Client) CancelVolumeTransfer(ctx context.Context, namespace, transferID string) error {
	return c.cleanupVolumeTransfer(ctx, namespace, transferID, "transfer_pod.cancel")
}

func (c *Client) CleanupVolumeTransfer(ctx context.Context, namespace, transferID string) error {
	return c.cleanupVolumeTransfer(ctx, namespace, transferID, "transfer_pod.cleanup")
}

func (c *Client) cleanupVolumeTransfer(ctx context.Context, namespace, transferID, operation string) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", operation)
	defer func() { end(err) }()
	if c == nil || c.client == nil {
		return fmt.Errorf("%w: Kubernetes client is unavailable", ErrInvalidVolumeTransferSpec)
	}
	reference, err := volumeTransferReference(namespace, transferID)
	if err != nil {
		return err
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, volumeTransferCleanupTimeout)
	defer cancel()
	gracePeriod := int64(0)
	deleteOptions := metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod}
	podErr := c.client.CoreV1().Pods(reference.Namespace).Delete(cleanupCtx, reference.PodName, deleteOptions)
	if podErr != nil && !apierrors.IsNotFound(podErr) {
		return podErr
	}
	ticker := time.NewTicker(volumeTransferCleanupPollInterval)
	defer ticker.Stop()
	for {
		_, getErr := c.client.CoreV1().Pods(reference.Namespace).Get(cleanupCtx, reference.PodName, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			break
		}
		if getErr != nil {
			return getErr
		}
		select {
		case <-cleanupCtx.Done():
			return cleanupCtx.Err()
		case <-ticker.C:
		}
	}
	return c.deleteVolumeTransferPolicy(cleanupCtx, reference)
}

func (c *Client) execVolumeTransfer(ctx context.Context, target VolumeTransferStreamTarget, direction string, stdin io.Reader, stdout, stderr io.Writer) error {
	if c == nil || c.restConfig == nil {
		return fmt.Errorf("%w: Kubernetes REST config is unavailable", ErrInvalidVolumeTransferSpec)
	}
	pod, err := c.readyVolumeTransferPod(ctx, target, direction)
	if err != nil {
		return err
	}
	req := c.client.CoreV1().RESTClient().Post().Resource("pods").Name(pod.Name).Namespace(pod.Namespace).
		SubResource("exec").VersionedParams(&corev1.PodExecOptions{
		Container: volumeTransferContainerName,
		Command:   volumeTransferExecCommand(ctx, direction),
		Stdin:     stdin != nil,
		Stdout:    stdout != nil,
		Stderr:    stderr != nil,
		TTY:       false,
	}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: stdin, Stdout: stdout, Stderr: stderr, Tty: false})
}

func (c *Client) readyVolumeTransferPod(ctx context.Context, target VolumeTransferStreamTarget, direction string) (*corev1.Pod, error) {
	target.Namespace = strings.TrimSpace(target.Namespace)
	target.TransferID = strings.TrimSpace(target.TransferID)
	target.ProjectID = strings.TrimSpace(target.ProjectID)
	target.ProjectVolumeID = strings.TrimSpace(target.ProjectVolumeID)
	reference, err := volumeTransferReference(target.Namespace, target.TransferID)
	if err != nil {
		return nil, err
	}
	if target.ProjectID == "" || target.ProjectVolumeID == "" || len(validation.IsValidLabelValue(target.ProjectID)) > 0 || len(validation.IsValidLabelValue(target.ProjectVolumeID)) > 0 {
		return nil, ErrInvalidVolumeTransferSpec
	}
	pod, err := c.client.CoreV1().Pods(reference.Namespace).Get(ctx, reference.PodName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if pod.Labels[ManagedByLabel] != ManagedByValue || pod.Labels[ScopeLabel] != volumeTransferScope ||
		pod.Labels[ApplicationNameKey] != reference.PodName || pod.Labels[ProjectIDLabel] != target.ProjectID ||
		pod.Labels[ProjectVolumeIDLabel] != target.ProjectVolumeID || pod.Labels[VolumeTransferIDLabel] != target.TransferID ||
		pod.Labels["luna.devops/volume-transfer-direction"] != direction ||
		volumeTransferPodObservation(pod).State != "ready" {
		return nil, ErrVolumeTransferNotReady
	}
	return pod, nil
}

func validateVolumeTransferSpec(ctx context.Context, spec VolumeTransferSpec) (VolumeTransferSpec, error) {
	if err := ctx.Err(); err != nil {
		return VolumeTransferSpec{}, err
	}
	spec.TransferID = strings.TrimSpace(spec.TransferID)
	spec.ProjectID = strings.TrimSpace(spec.ProjectID)
	spec.ProjectVolumeID = strings.TrimSpace(spec.ProjectVolumeID)
	spec.Namespace = strings.TrimSpace(spec.Namespace)
	spec.ClaimName = strings.TrimSpace(spec.ClaimName)
	spec.Direction = strings.ToLower(strings.TrimSpace(spec.Direction))
	spec.Format = strings.ToLower(strings.TrimSpace(spec.Format))
	spec.VolumeMode = strings.TrimSpace(spec.VolumeMode)
	spec.ConsistencyMode = strings.ToLower(strings.TrimSpace(spec.ConsistencyMode))
	spec.Image = strings.TrimSpace(spec.Image)
	spec.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(spec.ExpectedSHA256))
	for name, value := range map[string]string{"transfer id": spec.TransferID, "project id": spec.ProjectID,
		"project volume id": spec.ProjectVolumeID, "namespace": spec.Namespace, "claim name": spec.ClaimName, "image": spec.Image} {
		if value == "" {
			return VolumeTransferSpec{}, fmt.Errorf("%w: %s is required", ErrInvalidVolumeTransferSpec, name)
		}
	}
	for _, value := range []string{spec.TransferID, spec.ProjectID, spec.ProjectVolumeID} {
		if len(validation.IsValidLabelValue(value)) > 0 {
			return VolumeTransferSpec{}, fmt.Errorf("%w: identity is invalid", ErrInvalidVolumeTransferSpec)
		}
	}
	if len(validation.IsDNS1123Label(spec.Namespace)) > 0 || len(validation.IsDNS1123Subdomain(spec.ClaimName)) > 0 {
		return VolumeTransferSpec{}, fmt.Errorf("%w: namespace or claim is invalid", ErrInvalidVolumeTransferSpec)
	}
	if spec.Direction != "import" && spec.Direction != "export" {
		return VolumeTransferSpec{}, fmt.Errorf("%w: direction is invalid", ErrInvalidVolumeTransferSpec)
	}
	if spec.CapacityBytes < 1 || spec.CapacityBytes > volumeTransferMaximumBytes || spec.MaxArchiveBytes < 1 || spec.MaxArchiveBytes > volumeTransferMaximumBytes || spec.ExpectedBytes < 0 || spec.ExpectedBytes > volumeTransferMaximumBytes {
		return VolumeTransferSpec{}, fmt.Errorf("%w: byte limits are invalid", ErrInvalidVolumeTransferSpec)
	}
	if spec.Direction == "import" && spec.ExpectedBytes < 1 {
		return VolumeTransferSpec{}, fmt.Errorf("%w: import length is required", ErrInvalidVolumeTransferSpec)
	}
	if spec.ExpectedSHA256 != "" && !validSHA256Hex(spec.ExpectedSHA256) {
		return VolumeTransferSpec{}, fmt.Errorf("%w: expected checksum is invalid", ErrInvalidVolumeTransferSpec)
	}
	if spec.VolumeMode == "Filesystem" && spec.Format != "tar_gz" || spec.VolumeMode == "Block" && spec.Format != "raw_zst" || spec.VolumeMode != "Filesystem" && spec.VolumeMode != "Block" {
		return VolumeTransferSpec{}, fmt.Errorf("%w: volume mode and format mismatch", ErrInvalidVolumeTransferSpec)
	}
	if spec.Direction == "export" {
		if spec.ConsistencyMode != "snapshot" && spec.ConsistencyMode != "live" && spec.ConsistencyMode != "unmounted" || spec.ExportedAt.IsZero() {
			return VolumeTransferSpec{}, fmt.Errorf("%w: export consistency is invalid", ErrInvalidVolumeTransferSpec)
		}
	}
	if spec.MaxArchiveFiles <= 0 {
		spec.MaxArchiveFiles = volumeTransferDefaultMaxArchiveFile
	}
	if spec.MaxArchiveFiles > volumeTransferDefaultMaxArchiveFile {
		return VolumeTransferSpec{}, fmt.Errorf("%w: archive file limit is invalid", ErrInvalidVolumeTransferSpec)
	}
	if spec.ImagePullPolicy == "" {
		spec.ImagePullPolicy = corev1.PullIfNotPresent
	}
	return spec, nil
}

func volumeTransferPod(spec VolumeTransferSpec, name string, labels map[string]string) *corev1.Pod {
	readOnly := spec.Direction == "export"
	terminationGrace := int64(30)
	readOnlyRootFilesystem := true
	allowPrivilegeEscalation := false
	runAsNonRoot := spec.VolumeMode != "Block"
	nonRootUser := int64(65532)
	rootUser := int64(0)
	fsGroup := int64(65532)
	fsGroupPolicy := corev1.FSGroupChangeOnRootMismatch
	environment := []corev1.EnvVar{
		{Name: "LUNA_VOLUME_TRANSFER_ID", Value: spec.TransferID},
		{Name: "LUNA_VOLUME_TRANSFER_DIRECTION", Value: spec.Direction},
		{Name: "LUNA_VOLUME_TRANSFER_FORMAT", Value: spec.Format},
		{Name: "LUNA_VOLUME_TRANSFER_VOLUME_MODE", Value: spec.VolumeMode},
		{Name: "LUNA_VOLUME_TRANSFER_CONSISTENCY_MODE", Value: spec.ConsistencyMode},
		{Name: "LUNA_VOLUME_TRANSFER_CAPACITY_BYTES", Value: strconv.FormatInt(spec.CapacityBytes, 10)},
		{Name: "LUNA_VOLUME_TRANSFER_MAX_BYTES", Value: strconv.FormatInt(spec.MaxArchiveBytes, 10)},
		{Name: "LUNA_VOLUME_TRANSFER_EXPECTED_BYTES", Value: strconv.FormatInt(spec.ExpectedBytes, 10)},
		{Name: "LUNA_VOLUME_TRANSFER_EXPECTED_SHA256", Value: spec.ExpectedSHA256},
		{Name: "LUNA_VOLUME_TRANSFER_MAX_FILES", Value: strconv.Itoa(spec.MaxArchiveFiles)},
	}
	if spec.ExportedAt.IsZero() {
		environment = append(environment, corev1.EnvVar{Name: "LUNA_VOLUME_TRANSFER_EXPORTED_AT", Value: ""})
	} else {
		environment = append(environment, corev1.EnvVar{Name: "LUNA_VOLUME_TRANSFER_EXPORTED_AT", Value: spec.ExportedAt.UTC().Format(time.RFC3339Nano)})
	}
	dataPath := volumeTransferFilesystemPath
	if spec.VolumeMode == "Block" {
		dataPath = volumeTransferBlockDevicePath
	}
	environment = append(environment, corev1.EnvVar{Name: "LUNA_VOLUME_TRANSFER_DATA_PATH", Value: dataPath})
	container := corev1.Container{
		Name: volumeTransferContainerName, Image: spec.Image, ImagePullPolicy: spec.ImagePullPolicy,
		Command: []string{volumeTransferBinaryPath, "serve"}, Env: environment,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi")},
		},
		SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			ReadOnlyRootFilesystem: &readOnlyRootFilesystem, RunAsNonRoot: &runAsNonRoot,
			Capabilities:   &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
	}
	if spec.VolumeMode == "Filesystem" {
		container.VolumeMounts = []corev1.VolumeMount{{Name: volumeTransferVolumeName, MountPath: volumeTransferFilesystemPath, ReadOnly: readOnly}}
		container.SecurityContext.RunAsUser = &nonRootUser
	} else {
		container.SecurityContext.RunAsUser = &rootUser
		container.VolumeDevices = []corev1.VolumeDevice{{Name: volumeTransferVolumeName, DevicePath: volumeTransferBlockDevicePath}}
	}
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: spec.Namespace, Labels: cloneStringMap(labels)}, Spec: corev1.PodSpec{
		AutomountServiceAccountToken: boolPtr(false), RestartPolicy: corev1.RestartPolicyNever,
		TerminationGracePeriodSeconds: &terminationGrace,
		SecurityContext: &corev1.PodSecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			FSGroup: &fsGroup, FSGroupChangePolicy: &fsGroupPolicy},
		Containers: []corev1.Container{container},
		Volumes: []corev1.Volume{{Name: volumeTransferVolumeName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: spec.ClaimName, ReadOnly: readOnly}}}},
	}}
}

func volumeTransferDenyEgressPolicy(namespace, name string, labels map[string]string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: cloneStringMap(labels)},
		Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{VolumeTransferIDLabel: labels[VolumeTransferIDLabel]}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}, Egress: []networkingv1.NetworkPolicyEgressRule{}}}
}

func volumeTransferPodObservation(pod *corev1.Pod) VolumeTransferObservation {
	observation := VolumeTransferObservation{State: "pending", Reason: "waiting", PodName: pod.Name}
	if pod.Status.StartTime != nil {
		value := pod.Status.StartTime.Time.UTC()
		observation.StartedAt = &value
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != volumeTransferContainerName {
			continue
		}
		if status.State.Terminated != nil {
			value := status.State.Terminated.FinishedAt.Time.UTC()
			observation.FinishedAt = &value
			observation.State = "failed"
			observation.Reason = "container_terminated"
			return observation
		}
	}
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		observation.State, observation.Reason = "failed", "pod_terminated"
		return observation
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			observation.State, observation.Reason = "ready", "ready"
			return observation
		}
	}
	return observation
}

func volumeTransferReference(namespace, transferID string) (VolumeTransferReference, error) {
	namespace, transferID = strings.TrimSpace(namespace), strings.TrimSpace(transferID)
	if len(validation.IsDNS1123Label(namespace)) > 0 || len(validation.IsValidLabelValue(transferID)) > 0 || transferID == "" {
		return VolumeTransferReference{}, ErrInvalidVolumeTransferSpec
	}
	digest := sha256.Sum256([]byte(transferID))
	suffix := hex.EncodeToString(digest[:8])
	name := "luna-vtx-" + suffix
	return VolumeTransferReference{PodName: name, Namespace: namespace, ContainerName: volumeTransferContainerName,
		NetworkPolicyName: "luna-vtx-net-" + suffix}, nil
}

func volumeTransferLabels(spec VolumeTransferSpec, applicationName string) map[string]string {
	return map[string]string{ManagedByLabel: ManagedByValue, ScopeLabel: volumeTransferScope, ApplicationNameKey: applicationName,
		ProjectIDLabel: spec.ProjectID, ProjectVolumeIDLabel: spec.ProjectVolumeID, VolumeTransferIDLabel: spec.TransferID,
		"luna.devops/volume-transfer-direction": spec.Direction}
}

func volumeTransferExecCommand(ctx context.Context, direction string) []string {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	command := []string{"/usr/bin/env"}
	if value := strings.TrimSpace(carrier.Get("traceparent")); value != "" {
		command = append(command, "LUNA_VOLUME_TRANSFER_TRACEPARENT="+value)
	}
	if value := strings.TrimSpace(carrier.Get("tracestate")); value != "" {
		command = append(command, "LUNA_VOLUME_TRANSFER_TRACESTATE="+value)
	}
	return append(command, volumeTransferBinaryPath, direction)
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sameVolumeTransferPod(actual, expected *corev1.Pod) bool {
	if actual == nil || expected == nil || !volumeTransferResourceMatches(actual.Labels, expected.Labels) {
		return false
	}
	return reflect.DeepEqual(actual.Spec, expected.Spec)
}

func volumeTransferResourceMatches(actual, expected map[string]string) bool {
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func (c *Client) ensureVolumeTransferPolicy(ctx context.Context, expected *networkingv1.NetworkPolicy) (bool, error) {
	policies := c.client.NetworkingV1().NetworkPolicies(expected.Namespace)
	_, err := policies.Create(ctx, expected, metav1.CreateOptions{})
	if err == nil {
		return true, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return false, err
	}
	existing, err := policies.Get(ctx, expected.Name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if !volumeTransferResourceMatches(existing.Labels, expected.Labels) || !reflect.DeepEqual(existing.Spec, expected.Spec) {
		return false, ErrVolumeTransferConflict
	}
	return false, nil
}

func (c *Client) attachVolumeTransferPolicyOwner(ctx context.Context, reference VolumeTransferReference, pod *corev1.Pod) error {
	policy, err := c.client.NetworkingV1().NetworkPolicies(reference.Namespace).Get(ctx, reference.NetworkPolicyName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	controller, block := true, true
	policy.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: pod.Name, UID: pod.UID, Controller: &controller, BlockOwnerDeletion: &block}}
	_, err = c.client.NetworkingV1().NetworkPolicies(reference.Namespace).Update(ctx, policy, metav1.UpdateOptions{})
	return err
}

func (c *Client) deleteVolumeTransferPolicy(ctx context.Context, reference VolumeTransferReference) error {
	err := c.client.NetworkingV1().NetworkPolicies(reference.Namespace).Delete(ctx, reference.NetworkPolicyName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

type volumeTransferControlRecord struct {
	Result    transferjob.Result `json:"result"`
	ErrorCode string             `json:"errorCode,omitempty"`
}

func parseVolumeTransferControl(content []byte) (VolumeTransferStreamResult, error) {
	lines := bytes.Split(content, []byte{'\n'})
	for index := len(lines) - 1; index >= 0; index-- {
		line := bytes.TrimSpace(lines[index])
		if !bytes.HasPrefix(line, []byte(volumeTransferControlRecordPrefix)) {
			continue
		}
		var record volumeTransferControlRecord
		if err := json.Unmarshal(bytes.TrimPrefix(line, []byte(volumeTransferControlRecordPrefix)), &record); err != nil {
			return VolumeTransferStreamResult{}, fmt.Errorf("volume transfer control record is invalid")
		}
		if record.ErrorCode != "" {
			if !validVolumeTransferHelperErrorCode(record.ErrorCode) {
				record.ErrorCode = transferjob.CodeJobFailed
			}
			return VolumeTransferStreamResult{}, &VolumeTransferStreamError{Code: record.ErrorCode}
		}
		if !validSHA256Hex(record.Result.SHA256) || record.Result.TransferredBytes < 1 || record.Result.ProcessedFiles < 0 || record.Result.LogicalBytes < 0 {
			return VolumeTransferStreamResult{}, fmt.Errorf("volume transfer result is invalid")
		}
		return VolumeTransferStreamResult{TransferredBytes: record.Result.TransferredBytes, ProcessedFiles: record.Result.ProcessedFiles,
			SHA256: record.Result.SHA256, LogicalBytes: record.Result.LogicalBytes, DataSHA256: record.Result.DataSHA256}, nil
	}
	return VolumeTransferStreamResult{}, fmt.Errorf("volume transfer result is missing")
}

func validVolumeTransferHelperErrorCode(code string) bool {
	switch code {
	case transferjob.CodeArchiveUnsafe, transferjob.CodeCapacityExceeded, transferjob.CodeChecksumMismatch,
		transferjob.CodeStateConflict, transferjob.CodeFormatUnsupported, transferjob.CodeJobFailed:
		return true
	default:
		return false
	}
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
	over   bool
}

func (writer *limitedBuffer) Write(content []byte) (int, error) {
	original := len(content)
	remaining := writer.limit - writer.buffer.Len()
	if remaining <= 0 {
		writer.over = true
		return original, nil
	}
	if len(content) > remaining {
		content = content[:remaining]
		writer.over = true
	}
	_, _ = writer.buffer.Write(content)
	return original, nil
}

func (writer *limitedBuffer) Bytes() []byte {
	if writer.over {
		return nil
	}
	return writer.buffer.Bytes()
}
