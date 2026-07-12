package model

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

type RuntimeCluster struct {
	ID                            string         `gorm:"primaryKey" json:"id"`
	Name                          string         `gorm:"not null" json:"name"`
	Type                          string         `gorm:"not null;default:kubernetes" json:"type"`
	Endpoint                      string         `json:"endpoint"`
	Scope                         string         `gorm:"index;not null;default:global" json:"scope"`
	OwnerRef                      string         `gorm:"index" json:"ownerRef"`
	ProjectIDs                    []string       `gorm:"-" json:"projectIds"`
	KubeconfigRef                 string         `json:"-"`
	KubeconfigSet                 bool           `gorm:"-" json:"kubeconfigSet"`
	Kubeconfig                    string         `gorm:"-" json:"kubeconfig,omitempty"`
	IsDefault                     bool           `gorm:"not null;default:false" json:"isDefault"`
	MaxConcurrentBuilds           int            `gorm:"not null;default:4" json:"maxConcurrentBuilds"`
	GatewayProvider               string         `gorm:"not null;default:gateway-api" json:"gatewayProvider"`
	GatewayRootDomain             string         `gorm:"not null;default:apps.local" json:"gatewayRootDomain"`
	GatewayDomainSuffixesRaw      string         `gorm:"column:gateway_domain_suffixes;type:text;not null;default:''" json:"-"`
	GatewayDomainSuffixes         []string       `gorm:"-" json:"gatewayDomainSuffixes"`
	GatewayPublicScheme           string         `gorm:"not null;default:http" json:"gatewayPublicScheme"`
	GatewayPublicPort             int            `gorm:"not null;default:80" json:"gatewayPublicPort"`
	GatewayControllerType         string         `gorm:"not null;default:traefik" json:"gatewayControllerType"`
	GatewayClassName              string         `gorm:"not null;default:traefik" json:"gatewayClassName"`
	GatewayName                   string         `gorm:"not null;default:luna-gateway" json:"gatewayName"`
	GatewayNamespace              string         `gorm:"not null;default:kube-system" json:"gatewayNamespace"`
	GatewayHTTPListenerName       string         `gorm:"not null;default:web" json:"gatewayHttpListenerName"`
	GatewayHTTPListenerPort       int            `gorm:"not null;default:8080" json:"gatewayHttpListenerPort"`
	GatewayHTTPSListenerName      string         `gorm:"not null;default:websecure" json:"gatewayHttpsListenerName"`
	GatewayHTTPSListenerPort      int            `gorm:"not null;default:8443" json:"gatewayHttpsListenerPort"`
	GatewayTLSSecretName          string         `gorm:"not null;default:''" json:"gatewayTlsSecretName"`
	GatewayTLSSecretNamespace     string         `gorm:"not null;default:''" json:"gatewayTlsSecretNamespace"`
	GatewayCertIssuerKind         string         `gorm:"not null;default:ClusterIssuer" json:"gatewayCertIssuerKind"`
	GatewayCertIssuerName         string         `gorm:"not null;default:''" json:"gatewayCertIssuerName"`
	GatewayCertificateNamespace   string         `gorm:"not null;default:''" json:"gatewayCertificateNamespace"`
	GatewayWildcardCertEnabled    bool           `gorm:"not null;default:false" json:"gatewayWildcardCertEnabled"`
	GatewayWildcardCertDomain     string         `gorm:"not null;default:''" json:"gatewayWildcardCertDomain"`
	GatewayWildcardCertSecretName string         `gorm:"not null;default:''" json:"gatewayWildcardCertSecretName"`
	GatewayExternalTLSMode        string         `gorm:"not null;default:none" json:"gatewayExternalTLSMode"`
	GatewayForwardedHeadersMode   string         `gorm:"not null;default:preserve" json:"gatewayForwardedHeadersMode"`
	GatewayTrustedProxyCIDRs      string         `gorm:"column:gateway_trusted_proxy_cidrs;type:text;not null;default:''" json:"gatewayTrustedProxyCIDRs"`
	GatewayDefaultRequestHeaders  string         `gorm:"type:text;not null;default:''" json:"gatewayDefaultRequestHeaders"`
	GatewayDefaultResponseHeaders string         `gorm:"type:text;not null;default:''" json:"gatewayDefaultResponseHeaders"`
	Status                        string         `gorm:"not null;default:unknown" json:"status"`
	LastCheckedAt                 *time.Time     `json:"lastCheckedAt"`
	CreatedBy                     string         `gorm:"index" json:"createdBy"`
	CreatedAt                     time.Time      `json:"createdAt"`
	UpdatedAt                     time.Time      `json:"updatedAt"`
	DeletedAt                     gorm.DeletedAt `gorm:"index" json:"-"`
}

type Environment struct {
	ID            string         `gorm:"primaryKey" json:"id"`
	ProjectID     string         `gorm:"index;not null" json:"projectId"`
	Name          string         `gorm:"not null" json:"name"`
	Slug          string         `gorm:"index;not null" json:"slug"`
	ClusterID     string         `gorm:"index" json:"clusterId"`
	Namespace     string         `json:"namespace"`
	Replicas      int            `gorm:"not null;default:1" json:"replicas"`
	CPURequest    string         `json:"cpuRequest"`
	MemoryRequest string         `json:"memoryRequest"`
	EnvVars       string         `json:"envVars"`
	ConfigRefs    string         `json:"configRefs"`
	SecretRefs    string         `json:"secretRefs"`
	CreatedBy     string         `gorm:"index" json:"createdBy"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

type Release struct {
	ID                 string         `gorm:"primaryKey" json:"id"`
	ProjectID          string         `gorm:"index;not null" json:"projectId"`
	ApplicationID      string         `gorm:"index;not null" json:"applicationId"`
	EnvironmentID      string         `gorm:"index;not null" json:"environmentId"`
	DeploymentTargetID string         `gorm:"index;not null;default:''" json:"deploymentTargetId"`
	BuildRunID         string         `gorm:"index" json:"buildRunId"`
	ImageRef           string         `gorm:"not null" json:"imageRef"`
	ForceImagePull     bool           `gorm:"not null;default:false" json:"forceImagePull"`
	Type               string         `gorm:"not null;default:deploy" json:"type"`
	Status             string         `gorm:"index;not null;default:pending" json:"status"`
	Revision           int            `gorm:"not null;default:1" json:"revision"`
	RollbackFromID     string         `gorm:"index" json:"rollbackFromId"`
	Message            string         `json:"message"`
	StartedAt          *time.Time     `json:"startedAt"`
	FinishedAt         *time.Time     `json:"finishedAt"`
	CreatedBy          string         `gorm:"index" json:"createdBy"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

type DeploymentTarget struct {
	ID                           string                        `gorm:"primaryKey" json:"id"`
	ProjectID                    string                        `gorm:"index;not null" json:"projectId"`
	ApplicationID                string                        `gorm:"index;not null" json:"applicationId"`
	EnvironmentID                string                        `gorm:"index;not null;default:''" json:"environmentId"`
	Name                         string                        `gorm:"not null" json:"name"`
	Stage                        string                        `gorm:"not null;default:prod" json:"stage"`
	ClusterID                    string                        `gorm:"index;not null;default:''" json:"clusterId"`
	Namespace                    string                        `gorm:"not null;default:''" json:"namespace"`
	WorkloadType                 string                        `gorm:"not null;default:Deployment" json:"workloadType"`
	Replicas                     int                           `gorm:"not null;default:1" json:"replicas"`
	CPURequest                   string                        `gorm:"not null;default:'1'" json:"cpuRequest"`
	MemoryRequest                string                        `gorm:"not null;default:'1Gi'" json:"memoryRequest"`
	CPULimit                     string                        `gorm:"not null;default:''" json:"cpuLimit"`
	MemoryLimit                  string                        `gorm:"not null;default:''" json:"memoryLimit"`
	ImagePullPolicy              string                        `gorm:"not null;default:''" json:"imagePullPolicy"`
	ContainerCommand             string                        `gorm:"type:text;not null;default:''" json:"containerCommand"`
	ContainerArgs                string                        `gorm:"type:text;not null;default:''" json:"containerArgs"`
	Lifecycle                    string                        `gorm:"type:text;not null;default:''" json:"lifecycle"`
	InitContainers               string                        `gorm:"type:text;not null;default:''" json:"initContainers"`
	SidecarContainers            string                        `gorm:"type:text;not null;default:''" json:"sidecarContainers"`
	ReadinessProbe               string                        `gorm:"type:text;not null;default:''" json:"readinessProbe"`
	LivenessProbe                string                        `gorm:"type:text;not null;default:''" json:"livenessProbe"`
	StartupProbe                 string                        `gorm:"type:text;not null;default:''" json:"startupProbe"`
	RunAsUser                    string                        `gorm:"not null;default:''" json:"runAsUser"`
	RunAsGroup                   string                        `gorm:"not null;default:''" json:"runAsGroup"`
	FSGroup                      string                        `gorm:"not null;default:''" json:"fsGroup"`
	FSGroupChangePolicy          string                        `gorm:"not null;default:''" json:"fsGroupChangePolicy"`
	ReadOnlyRootFilesystem       bool                          `gorm:"not null;default:false" json:"readOnlyRootFilesystem"`
	AllowPrivilegeEscalation     string                        `gorm:"not null;default:''" json:"allowPrivilegeEscalation"`
	CapabilityAdd                string                        `gorm:"type:text;not null;default:''" json:"capabilityAdd"`
	CapabilityDrop               string                        `gorm:"type:text;not null;default:''" json:"capabilityDrop"`
	NodeSelector                 string                        `gorm:"type:text;not null;default:''" json:"nodeSelector"`
	Tolerations                  string                        `gorm:"type:text;not null;default:''" json:"tolerations"`
	Affinity                     string                        `gorm:"type:text;not null;default:''" json:"affinity"`
	TopologySpreadConstraints    string                        `gorm:"type:text;not null;default:''" json:"topologySpreadConstraints"`
	PriorityClassName            string                        `gorm:"not null;default:''" json:"priorityClassName"`
	ServiceAccountName           string                        `gorm:"not null;default:''" json:"serviceAccountName,omitempty"`
	AutomountServiceAccountToken string                        `gorm:"not null;default:''" json:"automountServiceAccountToken,omitempty"`
	ServiceType                  string                        `gorm:"not null;default:''" json:"serviceType"`
	ServiceAnnotations           string                        `gorm:"type:text;not null;default:''" json:"serviceAnnotations"`
	ServiceExternalTrafficPolicy string                        `gorm:"not null;default:''" json:"serviceExternalTrafficPolicy"`
	ServiceSessionAffinity       string                        `gorm:"not null;default:''" json:"serviceSessionAffinity"`
	AutoScalingEnabled           bool                          `gorm:"not null;default:false" json:"autoScalingEnabled"`
	AutoScalingMinReplicas       int                           `gorm:"not null;default:1" json:"autoScalingMinReplicas"`
	AutoScalingMaxReplicas       int                           `gorm:"not null;default:1" json:"autoScalingMaxReplicas"`
	AutoScalingCPUPercent        int                           `gorm:"not null;default:0" json:"autoScalingCpuPercent"`
	AutoScalingMemoryPercent     int                           `gorm:"not null;default:0" json:"autoScalingMemoryPercent"`
	AutoScalingBehavior          string                        `gorm:"type:text;not null;default:''" json:"autoScalingBehavior"`
	ServicePort                  int                           `gorm:"not null;default:8080" json:"servicePort"`
	ServicePorts                 string                        `gorm:"type:text;not null;default:''" json:"servicePorts"`
	DeleteStatus                 string                        `gorm:"index;not null;default:active" json:"deleteStatus"`
	DeleteMessage                string                        `gorm:"type:text;not null;default:''" json:"deleteMessage"`
	DeleteStartedAt              *time.Time                    `json:"deleteStartedAt"`
	DeleteFinishedAt             *time.Time                    `json:"deleteFinishedAt"`
	SourceType                   string                        `gorm:"not null;default:repository" json:"sourceType"`
	RepositoryBindingID          string                        `gorm:"index" json:"repositoryBindingId"`
	DockerfilePath               string                        `gorm:"not null;default:Dockerfile" json:"dockerfilePath"`
	BuildContext                 string                        `gorm:"not null;default:." json:"buildContext"`
	BuildDirectory               string                        `json:"buildDirectory"`
	BuildArgs                    string                        `gorm:"type:text;not null;default:''" json:"buildArgs"`
	BuildEnvironmentID           string                        `gorm:"index;not null;default:''" json:"buildEnvironmentId"`
	BuildCPURequest              string                        `gorm:"not null;default:'1'" json:"buildCpuRequest"`
	BuildMemoryRequest           string                        `gorm:"not null;default:'1Gi'" json:"buildMemoryRequest"`
	BuildTimeoutSeconds          int                           `gorm:"not null;default:1800" json:"buildTimeoutSeconds"`
	TargetRegistryID             string                        `gorm:"index" json:"targetRegistryId"`
	TargetRepository             string                        `json:"targetRepository"`
	TargetTag                    string                        `json:"targetTag"`
	ImageRef                     string                        `json:"imageRef"`
	BuildLabels                  string                        `json:"buildLabels"`
	BuildVariableSetIDs          string                        `gorm:"type:text" json:"buildVariableSetIds"`
	BuildHooksEnabled            bool                          `gorm:"not null;default:true" json:"buildHooksEnabled"`
	BuildHookBindings            []DeploymentTargetHookBinding `gorm:"-" json:"buildHookBindings"`
	AutoDeploy                   bool                          `gorm:"not null;default:false" json:"autoDeploy"`
	BranchPattern                string                        `json:"branchPattern"`
	TagPattern                   string                        `json:"tagPattern"`
	ConcurrencyPolicy            string                        `gorm:"not null;default:queue" json:"concurrencyPolicy"`
	RuntimeConfigSetIDs          string                        `gorm:"type:text;not null;default:''" json:"runtimeConfigSetIds"`
	RuntimeConfigRefs            string                        `gorm:"type:text;not null;default:''" json:"runtimeConfigRefs"`
	EnvVars                      string                        `gorm:"type:text;not null;default:''" json:"envVars"`
	ConfigRefs                   string                        `gorm:"type:text;not null;default:''" json:"configRefs"`
	SecretRefs                   string                        `gorm:"type:text;not null;default:''" json:"-"`
	ConfigFiles                  string                        `gorm:"type:text;not null;default:''" json:"configFiles"`
	SecretFiles                  string                        `gorm:"type:text;not null;default:''" json:"-"`
	DataRetentionEnabled         bool                          `gorm:"not null;default:false" json:"dataRetentionEnabled"`
	DataCapacity                 string                        `gorm:"not null;default:''" json:"dataCapacity"`
	DataMountPath                string                        `gorm:"not null;default:'/data'" json:"dataMountPath"`
	DataVolumes                  string                        `gorm:"type:text;not null;default:''" json:"dataVolumes"`
	DataStorageClassName         string                        `gorm:"not null;default:''" json:"dataStorageClassName"`
	DataAccessMode               string                        `gorm:"not null;default:''" json:"dataAccessMode"`
	DataVolumeMode               string                        `gorm:"not null;default:''" json:"dataVolumeMode"`
	RequireApproval              bool                          `gorm:"not null;default:false" json:"requireApproval"`
	WebConsoleEnabled            *bool                         `json:"webConsoleEnabled"`
	Enabled                      bool                          `gorm:"not null;default:true" json:"enabled"`
	CreatedBy                    string                        `gorm:"index" json:"createdBy"`
	CreatedAt                    time.Time                     `json:"createdAt"`
	UpdatedAt                    time.Time                     `json:"updatedAt"`
	DeletedAt                    gorm.DeletedAt                `gorm:"index" json:"-"`
}

const (
	RuntimeConfigRefModeLive     = "live"
	RuntimeConfigRefModeSnapshot = "snapshot"
)

type DeploymentRuntimeConfigRef struct {
	SetID    string                           `json:"setId"`
	Mode     string                           `json:"mode"`
	Snapshot *DeploymentRuntimeConfigSnapshot `json:"snapshot,omitempty"`
}

type DeploymentRuntimeConfigSnapshot struct {
	Name        string    `json:"name"`
	EnvVars     string    `json:"envVars"`
	ConfigFiles string    `json:"configFiles"`
	SecretRefs  string    `json:"secretRefs"`
	SecretFiles string    `json:"secretFiles"`
	Enabled     bool      `json:"enabled"`
	CapturedAt  time.Time `json:"capturedAt"`
}

func RuntimeConfigRefMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RuntimeConfigRefModeSnapshot:
		return RuntimeConfigRefModeSnapshot
	default:
		return RuntimeConfigRefModeLive
	}
}

func EncodeDeploymentRuntimeConfigRefs(refs []DeploymentRuntimeConfigRef) string {
	normalized := NormalizeDeploymentRuntimeConfigRefs(refs)
	if len(normalized) == 0 {
		return ""
	}
	content, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(content)
}

func DecodeDeploymentRuntimeConfigRefs(raw string) []DeploymentRuntimeConfigRef {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var refs []DeploymentRuntimeConfigRef
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return nil
	}
	return NormalizeDeploymentRuntimeConfigRefs(refs)
}

func NormalizeDeploymentRuntimeConfigRefs(refs []DeploymentRuntimeConfigRef) []DeploymentRuntimeConfigRef {
	seen := map[string]bool{}
	normalized := make([]DeploymentRuntimeConfigRef, 0, len(refs))
	for _, ref := range refs {
		setID := strings.TrimSpace(ref.SetID)
		if setID == "" || seen[setID] {
			continue
		}
		seen[setID] = true
		mode := RuntimeConfigRefMode(ref.Mode)
		next := DeploymentRuntimeConfigRef{SetID: setID, Mode: mode}
		if mode == RuntimeConfigRefModeSnapshot && ref.Snapshot != nil {
			snapshot := *ref.Snapshot
			next.Snapshot = &snapshot
		}
		normalized = append(normalized, next)
	}
	return normalized
}

func DeploymentRuntimeConfigLiveSetIDs(refs []DeploymentRuntimeConfigRef) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range NormalizeDeploymentRuntimeConfigRefs(refs) {
		if ref.Mode == RuntimeConfigRefModeLive {
			ids = append(ids, ref.SetID)
		}
	}
	return ids
}

func ProjectRuntimeConfigSetSnapshot(set ProjectRuntimeConfigSet, capturedAt time.Time) DeploymentRuntimeConfigSnapshot {
	return DeploymentRuntimeConfigSnapshot{
		Name:        set.Name,
		EnvVars:     set.EnvVars,
		ConfigFiles: set.ConfigFiles,
		SecretRefs:  set.SecretRefs,
		SecretFiles: set.SecretFiles,
		Enabled:     set.Enabled,
		CapturedAt:  capturedAt,
	}
}

type DeploymentServicePort struct {
	Name        string `json:"name"`
	Port        int    `json:"port"`
	AppProtocol string `json:"appProtocol,omitempty"`
}

func DeploymentTargetServicePorts(target DeploymentTarget) []DeploymentServicePort {
	return DeploymentServicePortsFromJSON(target.ServicePorts, target.ServicePort)
}

func DeploymentServicePortsFromJSON(raw string, fallbackPort int) []DeploymentServicePort {
	var ports []DeploymentServicePort
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &ports)
	}
	return NormalizeDeploymentServicePorts(ports, fallbackPort)
}

func EncodeDeploymentServicePorts(ports []DeploymentServicePort, fallbackPort int) string {
	normalized := NormalizeDeploymentServicePorts(ports, fallbackPort)
	data, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(data)
}

func NormalizeDeploymentServicePorts(ports []DeploymentServicePort, fallbackPort int) []DeploymentServicePort {
	if fallbackPort <= 0 {
		fallbackPort = 8080
	}
	seen := map[int]bool{}
	normalized := make([]DeploymentServicePort, 0, len(ports))
	for _, item := range ports {
		port := item.Port
		if port <= 0 || port > 65535 || seen[port] {
			continue
		}
		seen[port] = true
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = "port"
		}
		normalized = append(normalized, DeploymentServicePort{Name: name, Port: port, AppProtocol: strings.TrimSpace(item.AppProtocol)})
	}
	if len(normalized) == 0 {
		return []DeploymentServicePort{{Name: "http", Port: fallbackPort}}
	}
	return normalized
}

type ProjectRuntimeConfigSet struct {
	ID               string         `gorm:"primaryKey" json:"id"`
	ProjectID        string         `gorm:"index;not null" json:"projectId"`
	Name             string         `gorm:"not null" json:"name"`
	EnvVars          string         `gorm:"type:text;not null;default:''" json:"envVars"`
	ConfigFiles      string         `gorm:"type:text;not null;default:''" json:"configFiles"`
	SecretRefs       string         `gorm:"type:text;not null;default:''" json:"-"`
	SecretFiles      string         `gorm:"type:text;not null;default:''" json:"-"`
	Enabled          bool           `gorm:"not null;default:true" json:"enabled"`
	DeleteStatus     string         `gorm:"index;not null;default:active" json:"deleteStatus"`
	DeleteMessage    string         `gorm:"type:text;not null;default:''" json:"deleteMessage"`
	DeleteStartedAt  *time.Time     `json:"deleteStartedAt"`
	DeleteFinishedAt *time.Time     `json:"deleteFinishedAt"`
	CreatedBy        string         `gorm:"index" json:"createdBy"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

type ReleaseLog struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	ReleaseID string    `gorm:"uniqueIndex;not null" json:"releaseId"`
	ProjectID string    `gorm:"index;not null" json:"projectId"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
