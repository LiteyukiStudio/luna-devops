package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"
)

type ApplicationResourcesSpec struct {
	Name                         string
	Namespace                    string
	WorkloadType                 string
	ProjectID                    string
	ApplicationID                string
	DeploymentTargetID           string
	ReleaseID                    string
	BuildRunID                   string
	ImageDigest                  string
	ServiceBindingsDigest        string
	Image                        string
	Replicas                     int32
	ServicePort                  int32
	ServicePorts                 []ApplicationServicePort
	CPURequest                   string
	MemoryRequest                string
	CPULimit                     string
	MemoryLimit                  string
	ImagePullPolicy              string
	ContainerCommand             string
	ContainerArgs                string
	Lifecycle                    string
	InitContainers               string
	SidecarContainers            string
	ReadinessProbe               string
	LivenessProbe                string
	StartupProbe                 string
	RunAsUser                    string
	RunAsGroup                   string
	FSGroup                      string
	FSGroupChangePolicy          string
	ReadOnlyRootFilesystem       bool
	AllowPrivilegeEscalation     string
	CapabilityAdd                string
	CapabilityDrop               string
	NodeSelector                 string
	Tolerations                  string
	Affinity                     string
	TopologySpreadConstraints    string
	PriorityClassName            string
	ServiceAccountName           string
	AutomountServiceAccountToken string
	TrustedServiceAccounts       []string
	ServiceType                  string
	ServiceAnnotations           string
	ServiceExternalTrafficPolicy string
	ServiceSessionAffinity       string
	AutoScalingEnabled           bool
	AutoScalingMinReplicas       int32
	AutoScalingMaxReplicas       int32
	AutoScalingCPUPercent        int32
	AutoScalingMemoryPercent     int32
	AutoScalingBehavior          string
	RolloutTimeoutSeconds        int32
	ConfigData                   map[string]string
	SecretData                   map[string]string
	ConfigFiles                  []ApplicationConfigFile
	SecretFiles                  []ApplicationConfigFile
	DataVolumes                  []ApplicationDataVolume
	ForceImagePull               bool
}

type ApplicationServicePort struct {
	Name        string
	Port        int32
	AppProtocol string
}

type ApplicationConfigFile struct {
	Path    string
	Key     string
	Content string
}

type ApplicationDataVolume struct {
	Name              string
	MountPath         string
	DevicePath        string
	SourceType        string
	ProjectVolumeID   string
	ReadOnly          bool
	ClaimName         string
	EmptyDirMedium    string
	EmptyDirSizeLimit string
}

type HookJobSpec struct {
	Name               string
	Namespace          string
	ProjectID          string
	ApplicationID      string
	BuildRunID         string
	DeploymentTargetID string
	Stage              string
	ReleaseID          string
	HookRunID          string
	Phase              string
	Image              string
	GitBranch          string
	GitTag             string
	GitRefName         string
	GitRefType         string
	GitRef             string
	GitSHA             string
	GitShortSHA        string
	Shell              string
	Script             string
	TimeoutSeconds     int32
	ConfigMapName      string
	SecretName         string
}

type HookJobResult struct {
	Succeeded bool
	ExitCode  int32
	Message   string
	Logs      string
}

const (
	hookJobSuccessTTLSeconds int32 = 300
	hookJobFailureTTLSeconds int32 = 86400
)

func (c *Client) ApplyApplicationResources(ctx context.Context, spec ApplicationResourcesSpec) error {
	if err := c.PreflightApplicationResources(ctx, spec); err != nil {
		return err
	}
	objectLabels := appObjectLabels(spec)
	selectorLabels := appSelectorLabels(spec)
	if err := c.applyApplicationRuntimeConfig(ctx, spec, objectLabels); err != nil {
		return err
	}
	for _, volume := range spec.DataVolumes {
		if err := validateApplicationDataVolume(volume); err != nil {
			return err
		}
	}
	effectiveSelectorLabels, err := c.applyApplicationWorkload(ctx, spec, objectLabels, selectorLabels)
	if err != nil {
		return err
	}
	if err := c.applyApplicationAutoScaling(ctx, spec, objectLabels); err != nil {
		return err
	}
	return c.applyService(ctx, spec, objectLabels, effectiveSelectorLabels)
}

// PreflightApplicationResources validates every same-name Kubernetes object
// before the deploy workflow performs its first mutation. This keeps a new
// database lifecycle from partially updating resources retained by an older
// lifecycle that used the same human-readable identifier.
func (c *Client) PreflightApplicationResources(ctx context.Context, spec ApplicationResourcesSpec) error {
	if err := validateApplicationResourcesSpec(spec); err != nil {
		return err
	}
	if err := c.ensureApplicationRuntimeConfigOwnership(ctx, spec); err != nil {
		return err
	}
	if err := c.ensureApplicationStorageOwnership(ctx, spec); err != nil {
		return err
	}
	return c.ensureApplicationWorkloadOwnership(ctx, spec)
}

func validateApplicationResourcesSpec(spec ApplicationResourcesSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" {
		return fmt.Errorf("application resource name and namespace are required")
	}
	if strings.TrimSpace(spec.Image) == "" {
		return fmt.Errorf("release image is required")
	}
	if strings.TrimSpace(spec.ProjectID) == "" || strings.TrimSpace(spec.ApplicationID) == "" || strings.TrimSpace(spec.DeploymentTargetID) == "" {
		return fmt.Errorf("project, application, and deployment target ids are required")
	}
	for _, port := range applicationServicePorts(spec) {
		if port.Port <= 0 || port.Port > 65535 {
			return fmt.Errorf("service port must be between 1 and 65535")
		}
	}
	if _, err := resourceRequirements(spec); err != nil {
		return err
	}
	if _, err := applicationPodSecurityContext(spec); err != nil {
		return err
	}
	if _, err := applicationContainerSecurityContext(spec); err != nil {
		return err
	}
	capabilityAdd, err := applicationStringList(spec.CapabilityAdd, "capability add")
	if err != nil {
		return err
	}
	if len(capabilityAdd) > 0 {
		return fmt.Errorf("application workloads cannot add Linux capabilities")
	}
	serviceAccountName := strings.TrimSpace(spec.ServiceAccountName)
	trustedServiceAccount := applicationServiceAccountTrusted(spec, serviceAccountName)
	if serviceAccountName != "" && !trustedServiceAccount {
		return fmt.Errorf("application service account is not approved by a trusted platform plan")
	}
	if automount, configured, err := optionalBool(spec.AutomountServiceAccountToken); err != nil {
		return fmt.Errorf("invalid automountServiceAccountToken: %w", err)
	} else if configured && automount && !trustedServiceAccount {
		return fmt.Errorf("application workloads cannot mount a service account token")
	}
	if serviceType := strings.TrimSpace(spec.ServiceType); serviceType != "" && serviceType != "ClusterIP" {
		return fmt.Errorf("application services only support ClusterIP")
	}
	if strings.TrimSpace(spec.ServiceExternalTrafficPolicy) != "" {
		return fmt.Errorf("application services cannot configure external traffic policy")
	}
	if _, err := applicationNodeSelector(spec); err != nil {
		return err
	}
	if _, err := applicationTolerations(spec); err != nil {
		return err
	}
	if _, err := applicationAffinity(spec); err != nil {
		return err
	}
	if _, err := applicationTopologySpreadConstraints(spec); err != nil {
		return err
	}
	if _, err := applicationProbe(spec.ReadinessProbe, "readiness probe"); err != nil {
		return err
	}
	if _, err := applicationProbe(spec.LivenessProbe, "liveness probe"); err != nil {
		return err
	}
	if _, err := applicationProbe(spec.StartupProbe, "startup probe"); err != nil {
		return err
	}
	if _, err := applicationLifecycle(spec); err != nil {
		return err
	}
	if _, err := applicationAuxContainers(spec.InitContainers, "init containers", spec, nil); err != nil {
		return err
	}
	if _, err := applicationAuxContainers(spec.SidecarContainers, "sidecar containers", spec, nil); err != nil {
		return err
	}
	if _, err := applicationStringList(spec.ContainerCommand, "container command"); err != nil {
		return err
	}
	if _, err := applicationStringList(spec.ContainerArgs, "container args"); err != nil {
		return err
	}
	if _, err := applicationStringList(spec.CapabilityDrop, "capability drop"); err != nil {
		return err
	}
	if _, err := applicationServiceAnnotations(spec); err != nil {
		return err
	}
	if err := validateApplicationAutoScaling(spec); err != nil {
		return err
	}
	if _, err := applicationAutoScalingBehavior(spec); err != nil {
		return err
	}
	for _, volume := range spec.DataVolumes {
		if err := validateApplicationDataVolume(volume); err != nil {
			return err
		}
	}
	return nil
}

func applicationServiceAccountTrusted(spec ApplicationResourcesSpec, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, candidate := range spec.TrustedServiceAccounts {
		if strings.TrimSpace(candidate) == name {
			return true
		}
	}
	return false
}

func intstrFromInt32(value int32) intstr.IntOrString {
	return intstr.FromInt(int(value))
}

func int64Ptr(value int64) *int64 {
	return &value
}

func int32Ptr(value int32) *int32 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalInt64(value string) (int64, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, false, fmt.Errorf("must be a non-negative integer")
	}
	return parsed, true, nil
}

func optionalBool(value string) (bool, bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return false, false, nil
	case "true":
		return true, true, nil
	case "false":
		return false, true, nil
	default:
		return false, false, fmt.Errorf("must be true or false")
	}
}

func applicationStringList(raw string, label string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", label, err)
		}
		return compactStringList(values), nil
	}
	values := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	return compactStringList(values), nil
}

func mustApplicationStringList(raw string) []string {
	values, err := applicationStringList(raw, "string list")
	if err != nil {
		panic(err)
	}
	return values
}

func compactStringList(values []string) []string {
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			output = append(output, value)
		}
	}
	return output
}

func stringMapFromJSONOrLines(raw string, label string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "{") {
		values := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", label, err)
		}
		return compactStringMap(values), nil
	}
	values := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid %s line %q", label, line)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid %s empty key", label)
		}
		values[key] = strings.TrimSpace(value)
	}
	return values, nil
}

func compactStringMap(values map[string]string) map[string]string {
	output := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key != "" {
			output[key] = strings.TrimSpace(value)
		}
	}
	if len(output) == 0 {
		return nil
	}
	return output
}
