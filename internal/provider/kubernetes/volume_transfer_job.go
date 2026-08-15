package kubernetes

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volumetransfer"
	"go.opentelemetry.io/otel/propagation"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	VolumeTransferIDLabel = "luna.devops/volume-transfer-id"

	volumeTransferScope                 = "volume-transfer"
	volumeTransferContainerName         = "transfer"
	volumeTransferVolumeName            = "project-volume"
	volumeTransferTokenVolumeName       = "callback-token"
	volumeTransferScratchVolumeName     = "scratch"
	volumeTransferTokenKey              = "callback-token"
	volumeTransferTokenMountPath        = "/var/run/secrets/luna-volume-transfer"
	volumeTransferTokenFilePath         = volumeTransferTokenMountPath + "/" + volumeTransferTokenKey
	volumeTransferFilesystemPath        = "/volume"
	volumeTransferBlockDevicePath       = "/dev/luna-volume"
	volumeTransferBinaryPath            = "/usr/local/bin/luna-volume-transfer"
	volumeTransferTerminalTTLSeconds    = int32(600)
	volumeTransferDefaultDeadline       = 2 * time.Hour
	volumeTransferCleanupTimeout        = 30 * time.Second
	volumeTransferCleanupPollInterval   = 200 * time.Millisecond
	volumeTransferDefaultMaxArchiveFile = 1_000_000
	volumeTransferScratchOverheadBytes  = 64 * 1024 * 1024
)

var (
	ErrInvalidVolumeTransferJobSpec = errors.New("volume transfer job specification is invalid")
	ErrVolumeTransferJobConflict    = errors.New("volume transfer job resources conflict with an existing transfer")
)

type VolumeTransferJobProvider interface {
	CreateVolumeTransferJob(context.Context, VolumeTransferJobSpec) (VolumeTransferJobReference, error)
	ObserveVolumeTransferJob(context.Context, string, string) (VolumeTransferJobObservation, error)
	CancelVolumeTransferJob(context.Context, string, string) error
	CleanupVolumeTransferJob(context.Context, string, string) error
}

type VolumeTransferJobSpec struct {
	TransferID      string
	ProjectID       string
	ProjectVolumeID string
	Namespace       string
	ClaimName       string
	Direction       string
	Format          string
	VolumeMode      string
	ConsistencyMode string
	CallbackBaseURL string
	CallbackToken   []byte
	CallbackCIDRs   []string
	Image           string
	CapacityBytes   int64
	ExpectedBytes   int64
	ExpectedSHA256  string
	ChunkSize       int64
	ExportedAt      time.Time
	ActiveDeadline  time.Duration
	MaxArchiveFiles int
	ImagePullPolicy corev1.PullPolicy
}

type VolumeTransferJobReference struct {
	Name              string
	Namespace         string
	SecretName        string
	NetworkPolicyName string
}

type VolumeTransferJobObservation struct {
	State      string
	Reason     string
	StartedAt  *time.Time
	FinishedAt *time.Time
}

type volumeTransferResources struct {
	reference VolumeTransferJobReference
	secret    *corev1.Secret
	policy    *networkingv1.NetworkPolicy
	job       *batchv1.Job
}

func (c *Client) CreateVolumeTransferJob(ctx context.Context, spec VolumeTransferJobSpec) (reference VolumeTransferJobReference, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer_job.create")
	defer func() { end(err) }()
	if c == nil || c.client == nil {
		return VolumeTransferJobReference{}, fmt.Errorf("%w: Kubernetes client is unavailable", ErrInvalidVolumeTransferJobSpec)
	}

	resources, err := c.buildVolumeTransferResources(ctx, spec)
	if err != nil {
		return VolumeTransferJobReference{}, err
	}
	reference = resources.reference

	jobs := c.client.BatchV1().Jobs(reference.Namespace)
	existingJob, getErr := jobs.Get(ctx, reference.Name, metav1.GetOptions{})
	if getErr == nil {
		if err := c.verifyExistingVolumeTransferJob(ctx, resources, existingJob); err != nil {
			return VolumeTransferJobReference{}, err
		}
		return reference, nil
	}
	if !apierrors.IsNotFound(getErr) {
		return VolumeTransferJobReference{}, getErr
	}

	secretCreated, err := c.ensureVolumeTransferSecret(ctx, resources.secret)
	if err != nil {
		return VolumeTransferJobReference{}, err
	}
	policyCreated, err := c.ensureVolumeTransferNetworkPolicy(ctx, resources.policy)
	if err != nil {
		if secretCreated {
			_ = c.deleteVolumeTransferSecret(ctx, reference)
		}
		return VolumeTransferJobReference{}, err
	}

	created, err := jobs.Create(ctx, resources.job, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			// A concurrent retry may create the same Job after the initial GET.
			// Re-read and verify the complete execution identity before treating
			// it as idempotent. Do not roll back shared prerequisites here: the
			// winning Job may already be mounting them.
			existing, getErr := jobs.Get(ctx, reference.Name, metav1.GetOptions{})
			if getErr != nil {
				return VolumeTransferJobReference{}, getErr
			}
			if verifyErr := c.verifyExistingVolumeTransferJob(ctx, resources, existing); verifyErr != nil {
				return VolumeTransferJobReference{}, verifyErr
			}
			return reference, nil
		}
		if policyCreated {
			_ = c.deleteVolumeTransferNetworkPolicy(ctx, reference)
		}
		if secretCreated {
			_ = c.deleteVolumeTransferSecret(ctx, reference)
		}
		return VolumeTransferJobReference{}, err
	}

	// Owner references are defense in depth for TTL cleanup. The Worker still
	// performs explicit bounded cleanup so an owner-reference update failure
	// cannot leave a running Job unobserved.
	_ = c.attachVolumeTransferResourceOwners(ctx, resources, created)
	return reference, nil
}

func (c *Client) ObserveVolumeTransferJob(ctx context.Context, namespace, transferID string) (observation VolumeTransferJobObservation, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer_job.observe")
	defer func() { end(err) }()
	if c == nil || c.client == nil {
		return VolumeTransferJobObservation{}, fmt.Errorf("%w: Kubernetes client is unavailable", ErrInvalidVolumeTransferJobSpec)
	}
	reference, err := volumeTransferResourceReference(namespace, transferID)
	if err != nil {
		return VolumeTransferJobObservation{}, err
	}
	job, err := c.client.BatchV1().Jobs(namespace).Get(ctx, reference.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return VolumeTransferJobObservation{State: "not_found", Reason: "job_not_found"}, nil
	}
	if err != nil {
		return VolumeTransferJobObservation{}, err
	}
	observation = volumeTransferJobObservation(job)
	return observation, nil
}

func (c *Client) CancelVolumeTransferJob(ctx context.Context, namespace, transferID string) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer_job.cancel")
	defer func() { end(err) }()
	if c == nil || c.client == nil {
		return fmt.Errorf("%w: Kubernetes client is unavailable", ErrInvalidVolumeTransferJobSpec)
	}
	reference, err := volumeTransferResourceReference(namespace, transferID)
	if err != nil {
		return err
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, volumeTransferCleanupTimeout)
	defer cancel()
	return c.removeVolumeTransferExecutionResources(cleanupCtx, reference, transferID)
}

func (c *Client) CleanupVolumeTransferJob(ctx context.Context, namespace, transferID string) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer_job.cleanup")
	defer func() { end(err) }()
	if c == nil || c.client == nil {
		return fmt.Errorf("%w: Kubernetes client is unavailable", ErrInvalidVolumeTransferJobSpec)
	}
	reference, err := volumeTransferResourceReference(namespace, transferID)
	if err != nil {
		return err
	}
	// Keep cleanup bounded even when a caller supplies a process-lifetime
	// context. A cancelled caller remains cancelled; detaching a request is a
	// Worker lifecycle decision, not a Provider fallback.
	cleanupCtx, cancel := context.WithTimeout(ctx, volumeTransferCleanupTimeout)
	defer cancel()
	return c.removeVolumeTransferExecutionResources(cleanupCtx, reference, transferID)
}

func (c *Client) removeVolumeTransferExecutionResources(ctx context.Context, reference VolumeTransferJobReference, transferID string) error {
	foregroundPropagation := metav1.DeletePropagationForeground
	gracePeriod := int64(0)
	deleteOptions := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
		PropagationPolicy:  &foregroundPropagation,
	}
	jobErr := c.client.BatchV1().Jobs(reference.Namespace).Delete(ctx, reference.Name, deleteOptions)
	if jobErr != nil && !apierrors.IsNotFound(jobErr) {
		return jobErr
	}
	selector := VolumeTransferIDLabel + "=" + transferID
	if podErr := c.client.CoreV1().Pods(reference.Namespace).DeleteCollection(ctx, deleteOptions, metav1.ListOptions{LabelSelector: selector}); podErr != nil && !apierrors.IsNotFound(podErr) {
		return podErr
	}

	ticker := time.NewTicker(volumeTransferCleanupPollInterval)
	defer ticker.Stop()
	for {
		jobGone := false
		if _, err := c.client.BatchV1().Jobs(reference.Namespace).Get(ctx, reference.Name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			jobGone = true
		} else if err != nil {
			return err
		}
		pods, err := c.client.CoreV1().Pods(reference.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return err
		}
		if jobGone && len(pods.Items) == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return errors.Join(c.deleteVolumeTransferSecret(ctx, reference), c.deleteVolumeTransferNetworkPolicy(ctx, reference))
}

func (c *Client) buildVolumeTransferResources(ctx context.Context, spec VolumeTransferJobSpec) (volumeTransferResources, error) {
	validated, endpoint, callbackCIDRs, err := validateVolumeTransferJobSpec(ctx, spec)
	if err != nil {
		return volumeTransferResources{}, err
	}
	spec = validated
	reference, err := volumeTransferResourceReference(spec.Namespace, spec.TransferID)
	if err != nil {
		return volumeTransferResources{}, err
	}
	labels := volumeTransferLabels(spec, reference.Name)
	traceEnvironment := volumeTransferTraceEnvironment(ctx)

	secretMode := int32(0o440)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: reference.SecretName, Namespace: spec.Namespace, Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{volumeTransferTokenKey: append([]byte(nil), spec.CallbackToken...)},
	}

	policy := volumeTransferNetworkPolicy(spec, reference.NetworkPolicyName, labels, endpoint, callbackCIDRs)
	job := volumeTransferJob(spec, reference.Name, reference.SecretName, labels, traceEnvironment, secretMode)
	return volumeTransferResources{reference: reference, secret: secret, policy: policy, job: job}, nil
}

func validateVolumeTransferJobSpec(ctx context.Context, spec VolumeTransferJobSpec) (VolumeTransferJobSpec, *url.URL, []string, error) {
	if err := ctx.Err(); err != nil {
		return VolumeTransferJobSpec{}, nil, nil, err
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
	spec.CallbackBaseURL = strings.TrimRight(strings.TrimSpace(spec.CallbackBaseURL), "/")
	spec.Image = strings.TrimSpace(spec.Image)
	spec.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(spec.ExpectedSHA256))

	for name, value := range map[string]string{
		"transfer id": spec.TransferID, "project id": spec.ProjectID, "project volume id": spec.ProjectVolumeID,
		"namespace": spec.Namespace, "claim name": spec.ClaimName, "image": spec.Image,
	} {
		if value == "" {
			return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: %s is required", ErrInvalidVolumeTransferJobSpec, name)
		}
	}
	for name, value := range map[string]string{
		"transfer id": spec.TransferID, "project id": spec.ProjectID, "project volume id": spec.ProjectVolumeID,
	} {
		if len(validation.IsValidLabelValue(value)) > 0 {
			return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: %s is invalid", ErrInvalidVolumeTransferJobSpec, name)
		}
	}
	if errs := validation.IsDNS1123Label(spec.Namespace); len(errs) > 0 {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: namespace is invalid", ErrInvalidVolumeTransferJobSpec)
	}
	if errs := validation.IsDNS1123Subdomain(spec.ClaimName); len(errs) > 0 {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: claim name is invalid", ErrInvalidVolumeTransferJobSpec)
	}
	if !validVolumeTransferCallbackToken(spec.CallbackToken) {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: callback token is invalid", ErrInvalidVolumeTransferJobSpec)
	}
	if spec.CapacityBytes < 1 || spec.CapacityBytes > volumetransfer.MaximumTransferSize ||
		spec.ExpectedBytes < 0 || spec.ExpectedBytes > volumetransfer.MaximumTransferSize {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: transfer byte limits are invalid", ErrInvalidVolumeTransferJobSpec)
	}
	if spec.ChunkSize == 0 {
		spec.ChunkSize = volumetransfer.RequiredChunkSize(spec.ExpectedBytes)
	}
	if spec.ChunkSize < volumetransfer.RequiredChunkSize(spec.ExpectedBytes) ||
		spec.ChunkSize > volumetransfer.MaximumChunkSize || spec.ChunkSize%(1024*1024) != 0 {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: chunk size is invalid", ErrInvalidVolumeTransferJobSpec)
	}
	if spec.ExpectedSHA256 != "" && !validSHA256Hex(spec.ExpectedSHA256) {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: expected checksum is invalid", ErrInvalidVolumeTransferJobSpec)
	}
	if spec.Direction != "import" && spec.Direction != "export" {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: direction is invalid", ErrInvalidVolumeTransferJobSpec)
	}
	if spec.Direction == "import" && (spec.ExpectedBytes < 1 || spec.ExpectedSHA256 == "") {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: import length and checksum are required", ErrInvalidVolumeTransferJobSpec)
	}
	switch spec.VolumeMode {
	case "Filesystem":
		if spec.Format != "tar_gz" {
			return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: filesystem transfers require tar_gz", ErrInvalidVolumeTransferJobSpec)
		}
	case "Block":
		if spec.Format != "raw_zst" {
			return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: block transfers require raw_zst", ErrInvalidVolumeTransferJobSpec)
		}
	default:
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: volume mode is invalid", ErrInvalidVolumeTransferJobSpec)
	}
	if spec.Direction == "export" {
		switch spec.ConsistencyMode {
		case "snapshot", "live", "unmounted":
		default:
			return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: consistency mode is invalid", ErrInvalidVolumeTransferJobSpec)
		}
		if spec.ExportedAt.IsZero() {
			return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: a stable export timestamp is required", ErrInvalidVolumeTransferJobSpec)
		}
	}

	endpoint, err := url.Parse(spec.CallbackBaseURL)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: a credential-free HTTPS callback URL is required", ErrInvalidVolumeTransferJobSpec)
	}
	if rawPort := endpoint.Port(); rawPort != "" {
		port, portErr := strconv.ParseInt(rawPort, 10, 32)
		if portErr != nil || port < 1 || port > 65535 {
			return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: callback port is invalid", ErrInvalidVolumeTransferJobSpec)
		}
	}
	if spec.ActiveDeadline <= 0 {
		spec.ActiveDeadline = volumeTransferDefaultDeadline
	}
	if spec.ActiveDeadline < time.Minute || spec.ActiveDeadline > 24*time.Hour {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: active deadline is outside the allowed range", ErrInvalidVolumeTransferJobSpec)
	}
	if spec.MaxArchiveFiles <= 0 {
		spec.MaxArchiveFiles = volumeTransferDefaultMaxArchiveFile
	}
	if spec.MaxArchiveFiles > volumeTransferDefaultMaxArchiveFile {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: archive file limit is too large", ErrInvalidVolumeTransferJobSpec)
	}
	if spec.ImagePullPolicy == "" {
		spec.ImagePullPolicy = corev1.PullIfNotPresent
	}
	if spec.ImagePullPolicy != corev1.PullAlways && spec.ImagePullPolicy != corev1.PullIfNotPresent && spec.ImagePullPolicy != corev1.PullNever {
		return VolumeTransferJobSpec{}, nil, nil, fmt.Errorf("%w: image pull policy is invalid", ErrInvalidVolumeTransferJobSpec)
	}

	callbackCIDRs, err := normalizeCallbackCIDRs(ctx, endpoint, spec.CallbackCIDRs)
	if err != nil {
		return VolumeTransferJobSpec{}, nil, nil, err
	}
	return spec, endpoint, callbackCIDRs, nil
}

func volumeTransferJob(spec VolumeTransferJobSpec, name, secretName string, labels map[string]string, traceEnvironment []corev1.EnvVar, secretMode int32) *batchv1.Job {
	readOnly := spec.Direction == "export"
	deadline := int64(spec.ActiveDeadline / time.Second)
	backoffLimit := int32(0)
	terminalTTL := volumeTransferTerminalTTLSeconds
	terminationGrace := int64(30)
	readOnlyRootFilesystem := true
	allowPrivilegeEscalation := false
	runAsNonRoot := spec.VolumeMode != "Block"
	nonRootUser := int64(65532)
	rootUser := int64(0)
	fsGroup := int64(65532)
	fsGroupPolicy := corev1.FSGroupChangeOnRootMismatch
	maxArchiveFiles := strconv.Itoa(spec.MaxArchiveFiles)
	scratchBytes := spec.ChunkSize + volumeTransferScratchOverheadBytes
	ephemeralLimitBytes := scratchBytes + volumeTransferScratchOverheadBytes
	scratchQuantity := *resource.NewQuantity(scratchBytes, resource.BinarySI)
	ephemeralLimitQuantity := *resource.NewQuantity(ephemeralLimitBytes, resource.BinarySI)

	environment := []corev1.EnvVar{
		{Name: "LUNA_VOLUME_TRANSFER_ID", Value: spec.TransferID},
		{Name: "LUNA_VOLUME_TRANSFER_DIRECTION", Value: spec.Direction},
		{Name: "LUNA_VOLUME_TRANSFER_FORMAT", Value: spec.Format},
		{Name: "LUNA_VOLUME_TRANSFER_VOLUME_MODE", Value: spec.VolumeMode},
		{Name: "LUNA_VOLUME_TRANSFER_CONSISTENCY_MODE", Value: spec.ConsistencyMode},
		{Name: "LUNA_VOLUME_TRANSFER_CALLBACK_BASE_URL", Value: spec.CallbackBaseURL},
		{Name: "LUNA_VOLUME_TRANSFER_TOKEN_FILE", Value: volumeTransferTokenFilePath},
		{Name: "LUNA_VOLUME_TRANSFER_CAPACITY_BYTES", Value: strconv.FormatInt(spec.CapacityBytes, 10)},
		{Name: "LUNA_VOLUME_TRANSFER_EXPECTED_BYTES", Value: strconv.FormatInt(spec.ExpectedBytes, 10)},
		{Name: "LUNA_VOLUME_TRANSFER_EXPECTED_SHA256", Value: spec.ExpectedSHA256},
		{Name: "LUNA_VOLUME_TRANSFER_MAX_FILES", Value: maxArchiveFiles},
		{Name: "LUNA_VOLUME_TRANSFER_CHUNK_SIZE", Value: strconv.FormatInt(spec.ChunkSize, 10)},
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
	environment = append(environment, traceEnvironment...)

	container := corev1.Container{
		Name:            volumeTransferContainerName,
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Command:         []string{volumeTransferBinaryPath},
		Env:             environment,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("100m"),
				corev1.ResourceMemory:           resource.MustParse("128Mi"),
				corev1.ResourceEphemeralStorage: scratchQuantity,
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("1"),
				corev1.ResourceMemory:           resource.MustParse("1Gi"),
				corev1.ResourceEphemeralStorage: ephemeralLimitQuantity,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
			RunAsNonRoot:             &runAsNonRoot,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: volumeTransferTokenVolumeName, MountPath: volumeTransferTokenMountPath, ReadOnly: true},
			{Name: volumeTransferScratchVolumeName, MountPath: "/tmp"},
		},
	}
	if spec.VolumeMode == "Filesystem" {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name: volumeTransferVolumeName, MountPath: volumeTransferFilesystemPath, ReadOnly: readOnly,
		})
		container.SecurityContext.RunAsUser = &nonRootUser
	} else {
		// Kubernetes exposes raw block volumeDevices as device nodes. The
		// transfer container needs root device access, but remains unprivileged,
		// drops every capability and cannot write its root filesystem.
		container.SecurityContext.RunAsUser = &rootUser
		container.VolumeDevices = []corev1.VolumeDevice{{Name: volumeTransferVolumeName, DevicePath: volumeTransferBlockDevicePath}}
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: spec.Namespace, Labels: cloneStringMap(labels)},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: &terminalTTL,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: cloneStringMap(labels)},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken:  boolPtr(false),
					RestartPolicy:                 corev1.RestartPolicyNever,
					TerminationGracePeriodSeconds: &terminationGrace,
					SecurityContext: &corev1.PodSecurityContext{
						SeccompProfile:      &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
						FSGroup:             &fsGroup,
						FSGroupChangePolicy: &fsGroupPolicy,
					},
					Containers: []corev1.Container{container},
					Volumes: []corev1.Volume{
						{Name: volumeTransferVolumeName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: spec.ClaimName, ReadOnly: readOnly,
						}}},
						{Name: volumeTransferTokenVolumeName, VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
							SecretName: secretName, DefaultMode: &secretMode,
						}}},
						{Name: volumeTransferScratchVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: &scratchQuantity,
						}}},
					},
				},
			},
		},
	}
}

func volumeTransferNetworkPolicy(spec VolumeTransferJobSpec, name string, labels map[string]string, endpoint *url.URL, callbackCIDRs []string) *networkingv1.NetworkPolicy {
	port := int32(443)
	if rawPort := endpoint.Port(); rawPort != "" {
		parsed, _ := strconv.ParseInt(rawPort, 10, 32)
		port = int32(parsed)
	}
	protocol := corev1.ProtocolTCP
	callbackPort := networkingv1.NetworkPolicyPort{Protocol: &protocol, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: port}}
	egress := make([]networkingv1.NetworkPolicyEgressRule, 0, len(callbackCIDRs)+1)
	for _, cidr := range callbackCIDRs {
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{
			To:    []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: cidr}}},
			Ports: []networkingv1.NetworkPolicyPort{callbackPort},
		})
	}
	if net.ParseIP(endpoint.Hostname()) == nil {
		dnsProtocolTCP := corev1.ProtocolTCP
		dnsProtocolUDP := corev1.ProtocolUDP
		dnsPort := intstr.FromInt32(53)
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "kube-system"}},
				PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "kube-dns"}},
			}},
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &dnsProtocolUDP, Port: &dnsPort},
				{Protocol: &dnsProtocolTCP, Port: &dnsPort},
			},
		})
	}
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: spec.Namespace, Labels: cloneStringMap(labels)},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: volumeTransferSelectorLabels(spec.TransferID)},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      egress,
		},
	}
}

func normalizeCallbackCIDRs(ctx context.Context, endpoint *url.URL, configured []string) ([]string, error) {
	values := make([]netip.Prefix, 0, len(configured)+2)
	for _, raw := range configured {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: callback CIDR is invalid", ErrInvalidVolumeTransferJobSpec)
		}
		prefix = prefix.Masked()
		if prefix.Bits() != prefix.Addr().BitLen() || !safeCallbackAddress(prefix.Addr()) {
			return nil, fmt.Errorf("%w: callback CIDR is too broad or unsafe", ErrInvalidVolumeTransferJobSpec)
		}
		values = append(values, prefix)
	}
	if len(values) == 0 {
		host := endpoint.Hostname()
		if address, err := netip.ParseAddr(host); err == nil {
			if !safeCallbackAddress(address) {
				return nil, fmt.Errorf("%w: callback address is unsafe", ErrInvalidVolumeTransferJobSpec)
			}
			values = append(values, netip.PrefixFrom(address, address.BitLen()))
		} else {
			addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(addresses) == 0 {
				return nil, fmt.Errorf("%w: callback host could not be resolved", ErrInvalidVolumeTransferJobSpec)
			}
			for _, address := range addresses {
				if !safeCallbackAddress(address) {
					continue
				}
				values = append(values, netip.PrefixFrom(address, address.BitLen()))
			}
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: callback host has no safe address", ErrInvalidVolumeTransferJobSpec)
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, prefix := range values {
		value := prefix.String()
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func safeCallbackAddress(address netip.Addr) bool {
	return address.IsValid() && !address.IsUnspecified() && !address.IsLoopback() && !address.IsMulticast() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast()
}

func volumeTransferTraceEnvironment(ctx context.Context) []corev1.EnvVar {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	result := make([]corev1.EnvVar, 0, 2)
	if value := strings.TrimSpace(carrier.Get("traceparent")); value != "" && len(value) <= 128 {
		result = append(result, corev1.EnvVar{Name: "LUNA_VOLUME_TRANSFER_TRACEPARENT", Value: value})
	}
	if value := strings.TrimSpace(carrier.Get("tracestate")); value != "" && len(value) <= 512 {
		result = append(result, corev1.EnvVar{Name: "LUNA_VOLUME_TRANSFER_TRACESTATE", Value: value})
	}
	return result
}

func (c *Client) verifyExistingVolumeTransferJob(ctx context.Context, resources volumeTransferResources, existing *batchv1.Job) error {
	if !volumeTransferResourceMatches(existing.Labels, resources.secret.Labels) {
		return ErrVolumeTransferJobConflict
	}
	secret, err := c.client.CoreV1().Secrets(resources.reference.Namespace).Get(ctx, resources.reference.SecretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ErrVolumeTransferJobConflict
		}
		return err
	}
	existingToken := secret.Data[volumeTransferTokenKey]
	expectedToken := resources.secret.Data[volumeTransferTokenKey]
	if len(existingToken) != len(expectedToken) || subtle.ConstantTimeCompare(existingToken, expectedToken) != 1 {
		return ErrVolumeTransferJobConflict
	}
	policy, err := c.client.NetworkingV1().NetworkPolicies(resources.reference.Namespace).Get(ctx, resources.reference.NetworkPolicyName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ErrVolumeTransferJobConflict
		}
		return err
	}
	if !reflect.DeepEqual(policy.Spec, resources.policy.Spec) || !sameVolumeTransferJobExecution(existing, resources.job) {
		return ErrVolumeTransferJobConflict
	}
	return nil
}

func sameVolumeTransferJobExecution(actual, expected *batchv1.Job) bool {
	if actual == nil || expected == nil || len(actual.Spec.Template.Spec.Containers) != 1 || len(expected.Spec.Template.Spec.Containers) != 1 {
		return false
	}
	if !sameInt64Pointer(actual.Spec.ActiveDeadlineSeconds, expected.Spec.ActiveDeadlineSeconds) ||
		!sameInt32Pointer(actual.Spec.BackoffLimit, expected.Spec.BackoffLimit) ||
		!sameInt32Pointer(actual.Spec.TTLSecondsAfterFinished, expected.Spec.TTLSecondsAfterFinished) {
		return false
	}
	actualPod := actual.Spec.Template.Spec
	expectedPod := expected.Spec.Template.Spec
	if actualPod.AutomountServiceAccountToken == nil || *actualPod.AutomountServiceAccountToken ||
		actualPod.RestartPolicy != expectedPod.RestartPolicy ||
		!reflect.DeepEqual(actualPod.Volumes, expectedPod.Volumes) {
		return false
	}
	actualContainer := actualPod.Containers[0]
	expectedContainer := expectedPod.Containers[0]
	return actualContainer.Name == expectedContainer.Name &&
		actualContainer.Image == expectedContainer.Image &&
		actualContainer.ImagePullPolicy == expectedContainer.ImagePullPolicy &&
		reflect.DeepEqual(actualContainer.Command, expectedContainer.Command) &&
		reflect.DeepEqual(filteredVolumeTransferEnvironment(actualContainer.Env), filteredVolumeTransferEnvironment(expectedContainer.Env)) &&
		reflect.DeepEqual(actualContainer.VolumeMounts, expectedContainer.VolumeMounts) &&
		reflect.DeepEqual(actualContainer.VolumeDevices, expectedContainer.VolumeDevices) &&
		reflect.DeepEqual(actualContainer.SecurityContext, expectedContainer.SecurityContext)
}

func filteredVolumeTransferEnvironment(environment []corev1.EnvVar) []corev1.EnvVar {
	result := make([]corev1.EnvVar, 0, len(environment))
	for _, item := range environment {
		if item.Name == "LUNA_VOLUME_TRANSFER_TRACEPARENT" || item.Name == "LUNA_VOLUME_TRANSFER_TRACESTATE" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func sameInt32Pointer(left, right *int32) bool {
	return left != nil && right != nil && *left == *right
}

func sameInt64Pointer(left, right *int64) bool {
	return left != nil && right != nil && *left == *right
}

func (c *Client) ensureVolumeTransferSecret(ctx context.Context, expected *corev1.Secret) (bool, error) {
	secrets := c.client.CoreV1().Secrets(expected.Namespace)
	_, err := secrets.Create(ctx, expected, metav1.CreateOptions{})
	if err == nil {
		return true, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return false, err
	}
	existing, err := secrets.Get(ctx, expected.Name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	actual := existing.Data[volumeTransferTokenKey]
	wanted := expected.Data[volumeTransferTokenKey]
	if !volumeTransferResourceMatches(existing.Labels, expected.Labels) || len(actual) != len(wanted) || subtle.ConstantTimeCompare(actual, wanted) != 1 {
		return false, ErrVolumeTransferJobConflict
	}
	return false, nil
}

func (c *Client) ensureVolumeTransferNetworkPolicy(ctx context.Context, expected *networkingv1.NetworkPolicy) (bool, error) {
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
		return false, ErrVolumeTransferJobConflict
	}
	return false, nil
}

func (c *Client) attachVolumeTransferResourceOwners(ctx context.Context, resources volumeTransferResources, job *batchv1.Job) error {
	owner := *metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job"))
	secret, err := c.client.CoreV1().Secrets(resources.reference.Namespace).Get(ctx, resources.reference.SecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	secret.OwnerReferences = []metav1.OwnerReference{owner}
	if _, err := c.client.CoreV1().Secrets(resources.reference.Namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return err
	}
	policy, err := c.client.NetworkingV1().NetworkPolicies(resources.reference.Namespace).Get(ctx, resources.reference.NetworkPolicyName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	policy.OwnerReferences = []metav1.OwnerReference{owner}
	_, err = c.client.NetworkingV1().NetworkPolicies(resources.reference.Namespace).Update(ctx, policy, metav1.UpdateOptions{})
	return err
}

func (c *Client) deleteVolumeTransferSecret(ctx context.Context, reference VolumeTransferJobReference) error {
	err := c.client.CoreV1().Secrets(reference.Namespace).Delete(ctx, reference.SecretName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) deleteVolumeTransferNetworkPolicy(ctx context.Context, reference VolumeTransferJobReference) error {
	err := c.client.NetworkingV1().NetworkPolicies(reference.Namespace).Delete(ctx, reference.NetworkPolicyName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func volumeTransferResourceReference(namespace, transferID string) (VolumeTransferJobReference, error) {
	namespace = strings.TrimSpace(namespace)
	transferID = strings.TrimSpace(transferID)
	if namespace == "" || transferID == "" || len(validation.IsDNS1123Label(namespace)) > 0 {
		return VolumeTransferJobReference{}, fmt.Errorf("%w: namespace and transfer id are required", ErrInvalidVolumeTransferJobSpec)
	}
	digest := sha256.Sum256([]byte(transferID))
	suffix := hex.EncodeToString(digest[:8])
	name := "luna-vtx-" + suffix
	return VolumeTransferJobReference{
		Name: name, Namespace: namespace, SecretName: name + "-token", NetworkPolicyName: name + "-egress",
	}, nil
}

func volumeTransferLabels(spec VolumeTransferJobSpec, applicationName string) map[string]string {
	labels := baseManagedLabels(applicationName)
	labels[ScopeLabel] = volumeTransferScope
	setLabel(labels, ProjectIDLabel, spec.ProjectID)
	setLabel(labels, ProjectVolumeIDLabel, spec.ProjectVolumeID)
	setLabel(labels, VolumeTransferIDLabel, spec.TransferID)
	return labels
}

func volumeTransferSelectorLabels(transferID string) map[string]string {
	return map[string]string{
		ManagedByLabel:        ManagedByValue,
		ScopeLabel:            volumeTransferScope,
		VolumeTransferIDLabel: transferID,
	}
}

func volumeTransferResourceMatches(actual, expected map[string]string) bool {
	return actual[ManagedByLabel] == expected[ManagedByLabel] &&
		actual[ScopeLabel] == expected[ScopeLabel] &&
		actual[VolumeTransferIDLabel] == expected[VolumeTransferIDLabel]
}

func volumeTransferJobObservation(job *batchv1.Job) VolumeTransferJobObservation {
	observation := VolumeTransferJobObservation{State: "pending", Reason: "waiting"}
	if !job.Status.StartTime.IsZero() {
		value := job.Status.StartTime.Time.UTC()
		observation.StartedAt = &value
	}
	if job.Status.CompletionTime != nil {
		value := job.Status.CompletionTime.Time.UTC()
		observation.FinishedAt = &value
	}
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete:
			observation.State = "succeeded"
			observation.Reason = "completed"
			return observation
		case batchv1.JobFailed:
			observation.State = "failed"
			observation.Reason = normalizeVolumeTransferJobReason(condition.Reason)
			return observation
		}
	}
	if job.Status.Succeeded > 0 {
		observation.State = "succeeded"
		observation.Reason = "completed"
	} else if job.Status.Failed > 0 {
		observation.State = "failed"
		observation.Reason = "job_failed"
	} else if job.Status.Active > 0 || job.Status.StartTime != nil {
		observation.State = "running"
		observation.Reason = "running"
	}
	return observation
}

func normalizeVolumeTransferJobReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "deadlineexceeded":
		return "deadline_exceeded"
	case "backofflimitexceeded":
		return "backoff_limit_exceeded"
	case "podfailurepolicy":
		return "pod_failure_policy"
	default:
		return "job_failed"
	}
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validVolumeTransferCallbackToken(token []byte) bool {
	if len(token) < 32 || len(token) > 512 {
		return false
	}
	padding := false
	for _, value := range token {
		if value == '=' {
			padding = true
			continue
		}
		if padding || !isVolumeTransferToken68Byte(value) {
			return false
		}
	}
	return true
}

func isVolumeTransferToken68Byte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' ||
		value == '-' || value == '.' || value == '_' || value == '~' || value == '+' || value == '/'
}

func resourceQuantityPtr(value string) *resource.Quantity {
	quantity := resource.MustParse(value)
	return &quantity
}
