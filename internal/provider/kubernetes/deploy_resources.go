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
	EnvironmentID                string
	DeploymentTargetID           string
	ReleaseID                    string
	BuildRunID                   string
	ImageDigest                  string
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
	DataRetentionEnabled         bool
	DataCapacity                 string
	DataMountPath                string
	DataVolumes                  []ApplicationDataVolume
	DataStorageClassName         string
	DataAccessMode               string
	DataVolumeMode               string
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
	Capacity          string
	SourceType        string
	ExistingClaimName string
	EmptyDirMedium    string
	EmptyDirSizeLimit string
}

type HookJobSpec struct {
	Name               string
	Namespace          string
	ProjectID          string
	ApplicationID      string
	BuildRunID         string
	EnvironmentID      string
	DeploymentTargetID string
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

type DataExportSpec struct {
	Name      string
	Namespace string
	PVCName   string
	MountPath string
	Volumes   []DataExportVolume
}

type DataExportVolume struct {
	Name    string
	PVCName string
}

func (c *Client) ApplyApplicationResources(ctx context.Context, spec ApplicationResourcesSpec) error {
	if err := validateApplicationResourcesSpec(spec); err != nil {
		return err
	}
	objectLabels := appObjectLabels(spec)
	selectorLabels := appSelectorLabels(spec)
	if err := c.applyApplicationRuntimeConfig(ctx, spec, objectLabels); err != nil {
		return err
	}
	if spec.DataRetentionEnabled {
		for _, volume := range persistentDataVolumes(spec) {
			if err := validateApplicationDataVolume(volume); err != nil {
				return err
			}
			if !dataVolumeNeedsPVC(volume) {
				continue
			}
			if err := c.applyPersistentDataVolume(ctx, spec, volume, objectLabels); err != nil {
				return err
			}
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

func validateApplicationResourcesSpec(spec ApplicationResourcesSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" {
		return fmt.Errorf("application resource name and namespace are required")
	}
	if strings.TrimSpace(spec.Image) == "" {
		return fmt.Errorf("release image is required")
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
	if _, err := applicationStringList(spec.CapabilityAdd, "capability add"); err != nil {
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
	if spec.DataRetentionEnabled {
		for _, volume := range persistentDataVolumes(spec) {
			if !dataVolumeNeedsPVC(volume) {
				continue
			}
			if _, err := persistentDataCapacity(volume); err != nil {
				return err
			}
		}
	}
	return nil
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
