package deploymentapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	runtimeapi "github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func targetVerificationRule() []aitool.OperationVerification {
	return []aitool.OperationVerification{{
		OperationID: "delivery_get_target",
		Family:      "deployment_target",
	}}
}

func targetDeleteVerificationRule() []aitool.OperationVerification {
	return []aitool.OperationVerification{{
		OperationID:     "delivery_get_target",
		Family:          "deployment_target",
		Expect:          "absent",
		StatusField:     "deleteStatus",
		PendingStatuses: []string{"deleting"},
		FailureStatuses: []string{"delete_failed"},
	}}
}

// --- Shared nested input/output structs ---

type aiServicePortInput struct {
	Name        string `json:"name,omitempty" desc:"端口名称（小写字母数字与短横线，如 http）"`
	Port        int    `json:"port" required:"true" min:"1" max:"65535" desc:"容器与 Service 监听端口号（1..65535）"`
	AppProtocol string `json:"appProtocol,omitempty" desc:"可选应用层协议标识（如 http、grpc）"`
}

type aiServicePortUpsertInput struct {
	Name        string `json:"name" required:"true" desc:"服务端口名称（如 http、metrics）"`
	Port        int    `json:"port" required:"true" min:"1" max:"65535" desc:"容器与 Service 监听端口号（1..65535）"`
	AppProtocol string `json:"appProtocol,omitempty" desc:"可选应用层协议标识"`
}

type aiSecretRequirementsView struct {
	Build       []string `json:"build"`
	RuntimeEnv  []string `json:"runtimeEnv"`
	RuntimeFile []string `json:"runtimeFile"`
}

type deliveryCreateTargetInput struct {
	Project             string               `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application         string               `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Stage               string               `json:"stage" required:"true" enum:"dev|test|staging|prod" desc:"目标部署环境阶段（同应用每环境最多 1 个活跃目标）"`
	Name                string               `json:"name,omitempty" desc:"环境显示名称（省略默认同 stage）"`
	CloneFromStage      string               `json:"cloneFromStage,omitempty" enum:"dev|test|staging|prod" desc:"可选从同应用已有环境安全克隆非敏感配置"`
	VolumeOverrides     map[string]string    `json:"volumeOverrides,omitempty" desc:"配合 cloneFromStage 使用：按源挂载 logicalName 映射目标环境独占的新 projectVolume（ID 或 displayName）"`
	OmitProjectVolumes  *bool                `json:"omitProjectVolumes,omitempty" desc:"配合 cloneFromStage 使用：显式跳过源环境的持久化卷挂载"`
	Cluster             string               `json:"cluster,omitempty" desc:"运行集群 ID 或名称（省略自动解析并持久化项目默认集群）"`
	WorkloadType        string               `json:"workloadType,omitempty" enum:"Deployment|StatefulSet" default:"Deployment" desc:"工作负载控制器类型"`
	SourceType          string               `json:"sourceType,omitempty" enum:"image|repository" desc:"制品来源类型（未传 cloneFromStage 时必填）"`
	ImageRef            string               `json:"imageRef,omitempty" desc:"容器镜像地址（sourceType=image 时必填）"`
	RepositoryBinding   string               `json:"repositoryBinding,omitempty" desc:"绑定代码仓库 ID 或 owner/repo（sourceType=repository 时必填）"`
	TargetRegistry      string               `json:"targetRegistry,omitempty" desc:"目标制品库 ID 或名称（sourceType=repository 时必填）"`
	DockerfilePath      string               `json:"dockerfilePath,omitempty" default:"Dockerfile" desc:"仓库内相对 Dockerfile 路径"`
	BuildContext        string               `json:"buildContext,omitempty" default:"." desc:"仓库内相对构建上下文路径"`
	BuildTemplate       string               `json:"buildTemplate,omitempty" desc:"可选构建模板 ID（传入时自动启用 template 构建模式）"`
	BuildTemplateValues map[string]string    `json:"buildTemplateValues,omitempty" desc:"配合 buildTemplate 传入的模板参数键值对"`
	Replicas            *int                 `json:"replicas,omitempty" min:"0" default:"1" desc:"初始期望副本数（>= 0）"`
	CPURequest          string               `json:"cpuRequest,omitempty" default:"1" desc:"容器 CPU 请求量（如 250m、1）"`
	MemoryRequest       string               `json:"memoryRequest,omitempty" default:"1Gi" desc:"容器内存请求量（如 512Mi、1Gi）"`
	ServicePorts        []aiServicePortInput `json:"servicePorts,omitempty" desc:"服务端口列表（省略默认 [{name: http, port: 8080}]）"`
	InternalOnly        *bool                `json:"internalOnly,omitempty" default:"false" desc:"是否仅限集群内网访问"`
}

type deliveryCreateTargetOutput struct {
	ID                 string                        `json:"id"`
	DeploymentTargetID string                        `json:"deploymentTargetId"`
	ProjectID          string                        `json:"projectId"`
	ApplicationID      string                        `json:"applicationId"`
	Name               string                        `json:"name"`
	Stage              string                        `json:"stage"`
	ClusterID          string                        `json:"clusterId"`
	EffectiveClusterID string                        `json:"effectiveClusterId"`
	WorkloadType       string                        `json:"workloadType"`
	SourceType         string                        `json:"sourceType"`
	ImageRef           string                        `json:"imageRef,omitempty"`
	Replicas           int                           `json:"replicas"`
	CPURequest         string                        `json:"cpuRequest"`
	MemoryRequest      string                        `json:"memoryRequest"`
	ServicePorts       []model.DeploymentServicePort `json:"servicePorts"`
	InternalOnly       bool                          `json:"internalOnly"`
	AutoDeploy         bool                          `json:"autoDeploy"`
	RequiresRelease    bool                          `json:"requiresRelease"`
	SecretRequirements aiSecretRequirementsView      `json:"secretRequirements"`
	Warnings           []string                      `json:"warnings,omitempty"`
}

// --- Tool 5: delivery_update_target_runtime ---

type aiAutoScalingPatchInput struct {
	Enabled       *bool `json:"enabled,omitempty" desc:"是否开启 HPA 自动扩缩容"`
	MinReplicas   *int  `json:"minReplicas,omitempty" min:"1" desc:"最小副本数（>= 1）"`
	MaxReplicas   *int  `json:"maxReplicas,omitempty" min:"1" desc:"最大副本数（>= minReplicas）"`
	CPUPercent    *int  `json:"cpuPercent,omitempty" min:"0" max:"1000" desc:"CPU 目标利用率百分比（0 表示禁用该维度）"`
	MemoryPercent *int  `json:"memoryPercent,omitempty" min:"0" max:"1000" desc:"内存目标利用率百分比（0 表示禁用该维度）"`
}

type aiHTTPHeader struct {
	Name  string `json:"name" required:"true" desc:"HTTP 请求头名称"`
	Value string `json:"value" required:"true" desc:"HTTP 请求头值"`
}

type aiHTTPGetAction struct {
	Path        string         `json:"path,omitempty" default:"/" desc:"HTTP 检查路径，如 /healthz"`
	Port        int            `json:"port" required:"true" min:"1" max:"65535" desc:"容器探测端口号"`
	Scheme      string         `json:"scheme,omitempty" enum:"HTTP|HTTPS" default:"HTTP" desc:"HTTP 或 HTTPS"`
	Host        string         `json:"host,omitempty" desc:"可选 Host 头"`
	HTTPHeaders []aiHTTPHeader `json:"httpHeaders,omitempty" desc:"自定义 HTTP 请求头列表"`
}

type aiTCPSocketAction struct {
	Port int    `json:"port" required:"true" min:"1" max:"65535" desc:"TCP 探测端口号"`
	Host string `json:"host,omitempty" desc:"可选探测主机地址"`
}

type aiExecAction struct {
	Command []string `json:"command" required:"true" min:"1" desc:"探测命令与参数数组"`
}

type aiGRPCAction struct {
	Port    int    `json:"port" required:"true" min:"1" max:"65535" desc:"gRPC 探测端口号"`
	Service string `json:"service,omitempty" desc:"可选 gRPC 服务名"`
}

type aiProbeSpec struct {
	HTTPGet             *aiHTTPGetAction   `json:"httpGet,omitempty" desc:"HTTP GET 健康检查（与 tcpSocket/exec/grpc 四选一）"`
	TCPSocket           *aiTCPSocketAction `json:"tcpSocket,omitempty" desc:"TCP 端口连通性检查"`
	Exec                *aiExecAction      `json:"exec,omitempty" desc:"容器内命令检查"`
	GRPC                *aiGRPCAction      `json:"grpc,omitempty" desc:"gRPC 健康检查"`
	InitialDelaySeconds *int32             `json:"initialDelaySeconds,omitempty" min:"0" desc:"首次探测延迟秒数"`
	PeriodSeconds       *int32             `json:"periodSeconds,omitempty" min:"1" desc:"探测周期秒数"`
	TimeoutSeconds      *int32             `json:"timeoutSeconds,omitempty" min:"1" desc:"单次探测超时秒数"`
	SuccessThreshold    *int32             `json:"successThreshold,omitempty" min:"1" desc:"成功连续次数阈值"`
	FailureThreshold    *int32             `json:"failureThreshold,omitempty" min:"1" desc:"失败连续次数阈值"`
}

type aiProbesPatchInput struct {
	Readiness *aiProbeSpec `json:"readiness,omitempty" desc:"新增或更新就绪探针"`
	Liveness  *aiProbeSpec `json:"liveness,omitempty" desc:"新增或更新存活探针"`
	Startup   *aiProbeSpec `json:"startup,omitempty" desc:"新增或更新启动探针"`
}

type aiSecurityAndSchedulingPatchInput struct {
	RunAsUser              *int64            `json:"runAsUser,omitempty" min:"0" desc:"运行用户 UID（>= 0）"`
	RunAsGroup             *int64            `json:"runAsGroup,omitempty" min:"0" desc:"运行用户组 GID（>= 0）"`
	FSGroup                *int64            `json:"fsGroup,omitempty" min:"0" desc:"卷文件系统组 GID（>= 0）"`
	ReadOnlyRootFilesystem *bool             `json:"readOnlyRootFilesystem,omitempty" desc:"是否设置根文件系统只读"`
	ClearRunAsUser         *bool             `json:"clearRunAsUser,omitempty" desc:"显式清除 runAsUser"`
	ClearRunAsGroup        *bool             `json:"clearRunAsGroup,omitempty" desc:"显式清除 runAsGroup"`
	ClearFSGroup           *bool             `json:"clearFsGroup,omitempty" desc:"显式清除 fsGroup"`
	ClearSecurityContext   *bool             `json:"clearSecurityContext,omitempty" desc:"清空自定义用户/组安全上下文并重置 readOnlyRootFilesystem=false"`
	SetNodeSelector        map[string]string `json:"setNodeSelector,omitempty" desc:"新增或更新节点选择标签"`
	RemoveNodeSelectorKeys []string          `json:"removeNodeSelectorKeys,omitempty" desc:"移除指定节点选择标签键"`
	ClearNodeSelector      *bool             `json:"clearNodeSelector,omitempty" desc:"清空全部节点选择标签"`
}

type aiTargetGovernancePatchInput struct {
	Enabled          *bool  `json:"enabled,omitempty" desc:"是否允许新建发布与自动发版（false 不停止已有 Pod，停机请用 delivery_scale_target）"`
	RequireApproval  *bool  `json:"requireApproval,omitempty" desc:"自动构建完成后是否要求人工确认才发版"`
	WebConsolePolicy string `json:"webConsolePolicy,omitempty" enum:"inherit|disabled" desc:"Web 控制台/Exec 策略：inherit 继承项目上限，disabled 强制禁用"`
}

type deliveryUpdateTargetRuntimeInput struct {
	Project               string                             `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application           string                             `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target                string                             `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
	Name                  string                             `json:"name,omitempty" desc:"修改环境显示名称"`
	Cluster               string                             `json:"cluster,omitempty" desc:"切换运行集群 ID 或名称（仅首次发布前 COUNT(releases)==0 允许）"`
	WorkloadType          string                             `json:"workloadType,omitempty" enum:"Deployment|StatefulSet" desc:"切换工作负载控制器类型"`
	ImageRef              string                             `json:"imageRef,omitempty" desc:"更新期望基准镜像地址"`
	ImagePullPolicy       string                             `json:"imagePullPolicy,omitempty" enum:"IfNotPresent|Always|Never" desc:"镜像拉取策略"`
	ResetImagePullPolicy  *bool                              `json:"resetImagePullPolicy,omitempty" desc:"清空自定义镜像拉取策略"`
	CPURequest            string                             `json:"cpuRequest,omitempty" desc:"容器 CPU 请求量（正数 Quantity）"`
	MemoryRequest         string                             `json:"memoryRequest,omitempty" desc:"容器内存请求量（正数 Quantity）"`
	AutoScaling           *aiAutoScalingPatchInput           `json:"autoScaling,omitempty" desc:"增量修改 HPA 自动扩缩容策略"`
	Command               []string                           `json:"command,omitempty" desc:"容器 ENTRYPOINT 命令数组"`
	Args                  []string                           `json:"args,omitempty" desc:"容器 CMD 参数数组"`
	ResetCommand          *bool                              `json:"resetCommand,omitempty" desc:"清空自定义 command 恢复镜像默认"`
	ResetArgs             *bool                              `json:"resetArgs,omitempty" desc:"清空自定义 args 恢复镜像默认"`
	UpsertPorts           []aiServicePortUpsertInput         `json:"upsertPorts,omitempty" desc:"按端口 name 新增或更新服务端口"`
	RemovePortNames       []string                           `json:"removePortNames,omitempty" desc:"按端口 name 删除服务端口（不可删空最后一个端口）"`
	Probes                *aiProbesPatchInput                `json:"probes,omitempty" desc:"新增或更新健康探针"`
	ClearProbes           []string                           `json:"clearProbes,omitempty" enum:"readiness|liveness|startup" desc:"显式清除指定健康探针"`
	SecurityAndScheduling *aiSecurityAndSchedulingPatchInput `json:"securityAndScheduling,omitempty" desc:"增量修改安全上下文与节点调度选择器"`
	InternalOnly          *bool                              `json:"internalOnly,omitempty" desc:"切换是否仅集群内网可访问"`
	Governance            *aiTargetGovernancePatchInput      `json:"governance,omitempty" desc:"增量修改启停、审批门禁与 Web 控制台策略"`
}

type deliveryUpdateTargetRuntimeOutput struct {
	ID                 string                        `json:"id"`
	DeploymentTargetID string                        `json:"deploymentTargetId"`
	ProjectID          string                        `json:"projectId"`
	ApplicationID      string                        `json:"applicationId"`
	Name               string                        `json:"name"`
	Stage              string                        `json:"stage"`
	ClusterID          string                        `json:"clusterId"`
	WorkloadType       string                        `json:"workloadType"`
	ImageRef           string                        `json:"imageRef,omitempty"`
	CPURequest         string                        `json:"cpuRequest"`
	MemoryRequest      string                        `json:"memoryRequest"`
	ServicePorts       []model.DeploymentServicePort `json:"servicePorts"`
	InternalOnly       bool                          `json:"internalOnly"`
	Enabled            bool                          `json:"enabled"`
	RequireApproval    bool                          `json:"requireApproval"`
	RequiresRelease    bool                          `json:"requiresRelease"`
	PendingFields      []string                      `json:"pendingFields,omitempty"`
}

// --- Tool 6: delivery_configure_target_config ---

type aiEnvKeyValueInput struct {
	Key   string `json:"key" required:"true" desc:"变量名称（匹配 ^[A-Za-z_][A-Za-z0-9_]*$）"`
	Value string `json:"value" required:"true" desc:"变量明文值"`
}

type aiRuntimeSecretActionInput struct {
	Key       string `json:"key" required:"true" desc:"加密环境变量名称（匹配 ^[A-Za-z_][A-Za-z0-9_]*$）"`
	Operation string `json:"operation,omitempty" enum:"set|generate|clear" default:"set" desc:"密钥操作：set（默认，必填非空 value）、generate（服务端随机生成）、clear（清除）"`
	Value     string `json:"value,omitempty" sensitive:"true" desc:"密钥明文值（operation=set 时必填非空）"`
	Encoding  string `json:"encoding,omitempty" enum:"base64|hex|alphanumeric|numeric" default:"base64" desc:"随机生成编码格式（仅 operation=generate）"`
	Length    *int   `json:"length,omitempty" min:"8" max:"256" default:"32" desc:"随机生成长度（8..256，默认 32，仅 operation=generate）"`
}

type aiConfigFileInput struct {
	Path    string `json:"path" required:"true" desc:"容器内非根绝对挂载路径（如 /etc/app/config.yaml）"`
	Content string `json:"content" required:"true" desc:"明文文件内容"`
}

type aiSecretFileActionInput struct {
	Path      string `json:"path" required:"true" desc:"容器内非根绝对挂载路径（如 /etc/app/tls.key）"`
	Operation string `json:"operation,omitempty" enum:"set|clear" default:"set" desc:"加密文件操作：set（默认，必填非空 content）或 clear（清除）"`
	Content   string `json:"content,omitempty" sensitive:"true" desc:"加密文件内容（operation=set 时必填非空）"`
}

type aiConfigSetBindingInput struct {
	ConfigSet       string `json:"configSet" required:"true" desc:"项目运行配置集 ID（prcs_xxx）或唯一名称"`
	Mode            string `json:"mode,omitempty" enum:"live|snapshot" default:"live" desc:"引用模式：live（实时跟随）或 snapshot（固化快照）"`
	RefreshSnapshot *bool  `json:"refreshSnapshot,omitempty" desc:"snapshot 模式下是否立即从配置集重新抓取最新快照"`
}

type deliveryConfigureTargetConfigInput struct {
	Project               string                       `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application           string                       `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target                string                       `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
	SetPublicEnv          []aiEnvKeyValueInput         `json:"setPublicEnv,omitempty" desc:"新增或更新普通环境变量"`
	RemovePublicEnvKeys   []string                     `json:"removePublicEnvKeys,omitempty" desc:"删除指定普通环境变量"`
	SecretActions         []aiRuntimeSecretActionInput `json:"secretActions,omitempty" desc:"单键写入/随机生成/清除加密环境变量"`
	UpsertConfigFiles     []aiConfigFileInput          `json:"upsertConfigFiles,omitempty" desc:"按非根绝对路径新增或更新明文挂载文件"`
	RemoveConfigFilePaths []string                     `json:"removeConfigFilePaths,omitempty" desc:"按绝对路径删除明文挂载文件"`
	SecretFileActions     []aiSecretFileActionInput    `json:"secretFileActions,omitempty" desc:"按非根绝对路径写入或清除加密挂载文件"`
	UpsertConfigSets      []aiConfigSetBindingInput    `json:"upsertConfigSets,omitempty" desc:"绑定或更新项目共享运行配置集"`
	DetachConfigSets      []string                     `json:"detachConfigSets,omitempty" desc:"解绑指定项目运行配置集（ID 或名称）"`
}

type deliveryConfigureTargetConfigOutput struct {
	ID                 string                     `json:"id"`
	DeploymentTargetID string                     `json:"deploymentTargetId"`
	ProjectID          string                     `json:"projectId"`
	ApplicationID      string                     `json:"applicationId"`
	Stage              string                     `json:"stage"`
	PublicEnv          map[string]string          `json:"publicEnv"`
	SecretKeys         []string                   `json:"secretKeys"`
	ConfigFiles        []runtimeConfigFileInput   `json:"configFiles"`
	SecretFilePaths    []string                   `json:"secretFilePaths"`
	ConfigSets         []aiTargetConfigSetRefView `json:"configSets"`
	RequiresRelease    bool                       `json:"requiresRelease"`
}

// --- Tool 7: delivery_configure_target_volumes ---

type aiTargetVolumeMountUpsertInput struct {
	LogicalName       string `json:"logicalName,omitempty" desc:"挂载逻辑名称（小写 DNS label；省略时优先继承同路径存量挂载名，否则按路径 basename 生成）"`
	SourceType        string `json:"sourceType" required:"true" enum:"projectVolume|emptyDir" desc:"卷来源类型：projectVolume 或 emptyDir"`
	ProjectVolume     string `json:"projectVolume,omitempty" desc:"项目数据卷 ID（pvol_xxx）或 displayName（sourceType=projectVolume 时必填）"`
	MountPath         string `json:"mountPath,omitempty" desc:"Filesystem 卷或 emptyDir 的容器内非根绝对挂载路径"`
	DevicePath        string `json:"devicePath,omitempty" desc:"Block 块设备卷的容器内非根绝对设备路径"`
	ReadOnly          *bool  `json:"readOnly,omitempty" desc:"是否只读挂载（emptyDir 禁止设为 true）"`
	EmptyDirMedium    string `json:"emptyDirMedium,omitempty" desc:"emptyDir 介质：省略或空串表示默认磁盘，Memory 表示内存盘"`
	EmptyDirSizeLimit string `json:"emptyDirSizeLimit,omitempty" desc:"可选 emptyDir 容量上限（如 512Mi）"`
}

type deliveryConfigureTargetVolumesInput struct {
	Project      string                           `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application  string                           `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target       string                           `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
	UpsertMounts []aiTargetVolumeMountUpsertInput `json:"upsertMounts,omitempty" desc:"增量新增或恢复的数据卷/临时目录挂载项"`
	RemoveMounts []string                         `json:"removeMounts,omitempty" desc:"按 logicalName、mountPath 或 devicePath 解绑的数据卷挂载项"`
}

type deliveryConfigureTargetVolumesOutput struct {
	ID                 string                               `json:"id"`
	DeploymentTargetID string                               `json:"deploymentTargetId"`
	ProjectID          string                               `json:"projectId"`
	ApplicationID      string                               `json:"applicationId"`
	Stage              string                               `json:"stage"`
	ClusterID          string                               `json:"clusterId"`
	Volumes            []deploymentTargetDataVolumeResponse `json:"volumes"`
	RequiresRelease    bool                                 `json:"requiresRelease"`
}

// --- Tool 8: delivery_delete_target ---

type deliveryDeleteTargetInput struct {
	Project     string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application string `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target      string `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
}

type deliveryDeleteTargetOutput struct {
	ID                 string `json:"id"`
	DeploymentTargetID string `json:"deploymentTargetId"`
	ApplicationID      string `json:"applicationId"`
	ProjectID          string `json:"projectId"`
	Stage              string `json:"stage"`
	DeleteStatus       string `json:"deleteStatus"`
}

func (h *Handler) registerDeliveryTargetWriteAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[deliveryCreateTargetInput, deliveryCreateTargetOutput]{
		Name:           "delivery_create_target",
		Domain:         aitool.DomainDelivery,
		Title:          "创建部署环境目标",
		Summary:        "创建部署环境目标，支持镜像源、代码仓库源或跨环境安全克隆（自动保护持久化卷与密钥隔离）。",
		AliasesZH:      []string{"新建部署环境", "创建部署目标", "克隆环境"},
		AliasesEN:      []string{"create deployment target", "clone stage"},
		Purpose:        aitool.Text("在应用下创建 dev/test/staging/prod 部署目标，支持镜像源、仓库源或从已有阶段安全克隆。", "Create a deployment target stage from an image, repository, or existing stage clone."),
		ApprovalTier:   aitool.ApprovalTierL2,
		RequiredScopes: []string{string(authz.ActionDeploymentUpdate)},
		Effect:         aitool.EffectWrite,
		Verification:   targetVerificationRule(),
		Preview:        h.previewDeliveryCreateTarget,
		Execute:        h.executeDeliveryCreateTarget,
	})

	aitool.Register(reg, aitool.Spec[deliveryUpdateTargetRuntimeInput, deliveryUpdateTargetRuntimeOutput]{
		Name:           "delivery_update_target_runtime",
		Domain:         aitool.DomainDelivery,
		Title:          "增量修改部署目标运行时规格与治理策略",
		Summary:        "纯增量修改部署目标算力、镜像、HPA、命令参数、端口、探针、安全调度与治理策略，完整保留未传字段。",
		AliasesZH:      []string{"修改部署规格", "更新探针端口", "更新HPA配置"},
		AliasesEN:      []string{"update target runtime", "patch deployment spec"},
		Purpose:        aitool.Text("纯增量更新部署目标的容器规格、镜像、自动扩缩容、命令、端口、探针、安全调度或治理开关。", "Incrementally patch runtime spec, image, HPA, command/args, ports, probes, security/scheduling, and governance of a deployment target."),
		ApprovalTier:   aitool.ApprovalTierL2,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentUpdate)},
		Effect:         aitool.EffectWrite,
		Verification:   targetVerificationRule(),
		Preview:        h.previewDeliveryUpdateTargetRuntime,
		Execute:        h.executeDeliveryUpdateTargetRuntime,
	})

	aitool.Register(reg, aitool.Spec[deliveryConfigureTargetConfigInput, deliveryConfigureTargetConfigOutput]{
		Name:           "delivery_configure_target_config",
		Domain:         aitool.DomainDelivery,
		Title:          "增量配置部署目标环境变量、密钥、挂载文件与配置集",
		Summary:        "纯增量修改部署目标普通环境变量、加密密钥、明文/加密挂载文件与共享运行配置集绑定，绝不全量覆盖。",
		AliasesZH:      []string{"配置环境变量", "修改运行密钥", "挂载配置文件"},
		AliasesEN:      []string{"configure target env", "update runtime secrets", "mount config files"},
		Purpose:        aitool.Text("纯增量增删改部署目标的环境变量、加密密钥、明文文件、加密文件及运行配置集绑定（需发版生效）。", "Incrementally patch public env vars, encrypted runtime secrets, config files, secret files, and config set bindings of a deployment target."),
		ApprovalTier:   aitool.ApprovalTierL2,
		RequiredScopes: []string{string(authz.ActionDeploymentUpdate), string(authz.ActionSecretUpdate)},
		Effect:         aitool.EffectWrite,
		Verification:   targetVerificationRule(),
		Preview:        h.previewDeliveryConfigureTargetConfig,
		Execute:        h.executeDeliveryConfigureTargetConfig,
	})

	aitool.Register(reg, aitool.Spec[deliveryConfigureTargetVolumesInput, deliveryConfigureTargetVolumesOutput]{
		Name:           "delivery_configure_target_volumes",
		Domain:         aitool.DomainDelivery,
		Title:          "增量配置部署目标持久化数据卷与临时目录挂载",
		Summary:        "增量绑定或解绑部署目标的 ProjectVolume 持久化数据卷与 emptyDir 临时目录。",
		AliasesZH:      []string{"挂载数据卷", "解绑PVC", "配置容器存储"},
		AliasesEN:      []string{"configure target volumes", "mount pvc", "unbind volume"},
		Purpose:        aitool.Text("增量绑定或安全解绑部署目标上的持久化数据卷（PVC）与临时目录（emptyDir）。", "Incrementally bind or unbind persistent project volumes and emptyDir mounts on a deployment target."),
		ApprovalTier:   aitool.ApprovalTierL2,
		RequiredScopes: []string{string(authz.ActionDeploymentUpdate), string(authz.ActionVolumeWrite)},
		Effect:         aitool.EffectWrite,
		Verification:   targetVerificationRule(),
		Preview:        h.previewDeliveryConfigureTargetVolumes,
		Execute:        h.executeDeliveryConfigureTargetVolumes,
	})

	aitool.Register(reg, aitool.Spec[deliveryDeleteTargetInput, deliveryDeleteTargetOutput]{
		Name:           "delivery_delete_target",
		Domain:         aitool.DomainDelivery,
		Title:          "异步删除部署环境目标",
		Summary:        "异步删除单个部署环境目标，前置拦截入站服务依赖并级联清理网关路由、安全解绑数据卷。",
		AliasesZH:      []string{"删除部署环境", "移除部署目标"},
		AliasesEN:      []string{"delete deployment target", "remove stage"},
		Purpose:        aitool.Text("异步删除指定部署环境及其 K8s 工作负载与关联网关路由（保留已解绑的持久化数据卷数据）。", "Asynchronously delete a deployment target and cascade-clean its gateway routes while preserving unbound persistent volumes."),
		ApprovalTier:   aitool.ApprovalTierL2,
		RequiredScopes: []string{string(authz.ActionDeploymentDelete)},
		Effect:         aitool.EffectAsync,
		Verification:   targetDeleteVerificationRule(),
		Preview:        h.previewDeliveryDeleteTarget,
		Execute:        h.executeDeliveryDeleteTarget,
	})
}

// ============================================================================
// Tool 4 Implementation: delivery_create_target
// ============================================================================

type preparedCreateTargetPlan struct {
	user               model.User
	project            model.Project
	app                model.Application
	target             model.DeploymentTarget
	dataVolumes        []deploymentTargetDataVolumeInput
	hookInputs         []deploymentTargetHookBindingInput
	buildEnvironment   *model.BuildEnvironmentConfig
	secretRequirements aiSecretRequirementsView
	warnings           []string
	previewChanges     []aitool.PreviewChange
}

func (h *Handler) prepareCreateTargetPlan(tc *aitool.ToolContext, in deliveryCreateTargetInput) (preparedCreateTargetPlan, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentUpdate)
	if err != nil {
		return preparedCreateTargetPlan{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return preparedCreateTargetPlan{}, err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return preparedCreateTargetPlan{}, err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return preparedCreateTargetPlan{}, err
	}
	if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
		return preparedCreateTargetPlan{}, err
	}

	stage, validStage := normalizePublicStage(in.Stage)
	if !validStage {
		return preparedCreateTargetPlan{}, aitool.BadRequest("deployment.stage_invalid", "stage must be dev, test, staging, or prod")
	}
	if err := resourceidentifier.Validate(stage, stageIdentifierMinLength, stageIdentifierMaxLength); err != nil {
		return preparedCreateTargetPlan{}, aitool.BadRequest("deployment.stage_invalid", err.Error())
	}
	if err := checkDeploymentStageAvailable(db, app.ID, stage, ""); err != nil {
		return preparedCreateTargetPlan{}, err
	}

	targetID := id.New("dplt")
	kubernetesName := resourceidentifier.DeploymentTargetName(app.Identifier, stage)

	secretReqs := aiSecretRequirementsView{
		Build:       []string{},
		RuntimeEnv:  []string{},
		RuntimeFile: []string{},
	}
	var (
		warnings         []string
		rawInput         deploymentTargetInput
		hookInputs       []deploymentTargetHookBindingInput
		buildEnvironment *model.BuildEnvironmentConfig
	)

	cloneStage := strings.TrimSpace(in.CloneFromStage)
	if cloneStage != "" {
		normClone, ok := normalizePublicStage(cloneStage)
		if !ok {
			return preparedCreateTargetPlan{}, aitool.BadRequest("argument_invalid", "cloneFromStage must be dev, test, staging, or prod")
		}
		if normClone == stage {
			return preparedCreateTargetPlan{}, aitool.BadRequest("argument_invalid", "cloneFromStage must differ from destination stage")
		}
		var srcTarget model.DeploymentTarget
		if err := db.First(&srcTarget, "project_id = ? AND application_id = ? AND stage = ? AND deleted_at IS NULL", project.ID, app.ID, normClone).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return preparedCreateTargetPlan{}, aitool.NotFound("deployment_target.source_not_found", fmt.Sprintf("source stage %q was not found in this application", normClone))
			}
			return preparedCreateTargetPlan{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}

		srcWithHooks, hErr := h.deploymentTargetWithHookBindings(srcTarget, tc.Context())
		if hErr != nil {
			return preparedCreateTargetPlan{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", hErr.Error())
		}
		mountsMap, mErr := h.deploymentTargetVolumeMountsByTarget(tc.Context(), []model.DeploymentTarget{srcWithHooks})
		if mErr != nil {
			return preparedCreateTargetPlan{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", mErr.Error())
		}
		srcMounts := mountsMap[srcWithHooks.ID]

		// Resolve destination cluster (explicit > source cluster > default cluster)
		clusterRef := strings.TrimSpace(in.Cluster)
		if clusterRef == "" {
			clusterRef = strings.TrimSpace(srcWithHooks.ClusterID)
		}
		destCluster, cErr := h.resolveRuntimeClusterForProject(tc, user, project.ID, clusterRef)
		if cErr != nil {
			return preparedCreateTargetPlan{}, cErr
		}

		// Handle source persistent volumes safely
		var srcPVMounts []model.DeploymentVolumeMount
		var clonedVolumes []deploymentTargetDataVolumeInput
		for _, m := range srcMounts {
			if m.ActivationState == model.DeploymentVolumeActivationReleasePending {
				continue
			}
			if m.SourceType == model.DeploymentVolumeSourceProjectVolume {
				srcPVMounts = append(srcPVMounts, m)
			} else if m.SourceType == model.DeploymentVolumeSourceEmptyDir {
				clonedVolumes = append(clonedVolumes, deploymentTargetDataVolumeInput{
					LogicalName: m.LogicalName,
					SourceType:  "emptyDir",
					MountPath:   optionalStringValue(m.MountPath),
					EmptyDir:    &deploymentTargetEmptyDirInput{Medium: m.EmptyDirMedium, SizeLimit: m.EmptyDirSizeLimit},
				})
			}
		}

		omitPV := boolVal(in.OmitProjectVolumes, false)
		if omitPV && len(in.VolumeOverrides) > 0 {
			return preparedCreateTargetPlan{}, aitool.BadRequest("argument_invalid", "omitProjectVolumes=true cannot be combined with volumeOverrides")
		}
		if len(srcPVMounts) > 0 && !omitPV {
			requiredNames := make([]string, 0, len(srcPVMounts))
			srcByName := make(map[string]model.DeploymentVolumeMount, len(srcPVMounts))
			for _, m := range srcPVMounts {
				requiredNames = append(requiredNames, m.LogicalName)
				srcByName[m.LogicalName] = m
			}
			var missing []string
			for _, reqName := range requiredNames {
				if strings.TrimSpace(in.VolumeOverrides[reqName]) == "" {
					missing = append(missing, reqName)
				}
			}
			if len(missing) > 0 {
				tErr := aitool.NewErrorWithHint(
					http.StatusBadRequest,
					"deployment_target.volume_mapping_required",
					fmt.Sprintf("source stage %q mounts persistent projectVolume(s) %v; provide distinct destination volumes via volumeOverrides or set omitProjectVolumes=true", normClone, missing),
					"Create or pick target-stage volumes via `runtime_list_volumes` / `runtime_create_volume`, then pass `volumeOverrides` keyed by logicalName.",
				)
				tErr.Details = map[string]any{"requiredLogicalNames": requiredNames, "missingLogicalNames": missing}
				return preparedCreateTargetPlan{}, tErr
			}
			for k := range in.VolumeOverrides {
				if _, ok := srcByName[k]; !ok {
					return preparedCreateTargetPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("volumeOverrides contains unknown logicalName %q", k))
				}
			}
			seenVolIDs := map[string]bool{}
			for _, m := range srcPVMounts {
				vol, vErr := resolveProjectVolume(db, project.ID, in.VolumeOverrides[m.LogicalName])
				if vErr != nil {
					return preparedCreateTargetPlan{}, vErr
				}
				if seenVolIDs[vol.ID] {
					return preparedCreateTargetPlan{}, aitool.Conflict("volume.binding_conflict", fmt.Sprintf("projectVolume %s cannot be bound to multiple mounts", vol.ID))
				}
				seenVolIDs[vol.ID] = true
				if !volume.CanAttachProjectVolume(vol) {
					return preparedCreateTargetPlan{}, aitool.Conflict("volume.state_conflict", fmt.Sprintf("projectVolume %s (%s) is not attachable", vol.ID, vol.LifecycleState))
				}
				if vol.ClusterID != destCluster.ID {
					return preparedCreateTargetPlan{}, aitool.Conflict("volume.incompatible_cluster", fmt.Sprintf("projectVolume %s belongs to cluster %s, but destination target uses cluster %s", vol.ID, vol.ClusterID, destCluster.ID))
				}
				clonedVolumes = append(clonedVolumes, deploymentTargetDataVolumeInput{
					LogicalName:     m.LogicalName,
					SourceType:      "projectVolume",
					ProjectVolumeID: vol.ID,
					MountPath:       optionalStringValue(m.MountPath),
					DevicePath:      optionalStringValue(m.DevicePath),
					ReadOnly:        m.ReadOnly,
				})
			}
		} else if len(in.VolumeOverrides) > 0 {
			return preparedCreateTargetPlan{}, aitool.BadRequest("argument_invalid", "source stage has no persistent projectVolume mounts for volumeOverrides")
		}

		// Gate deployment-scoped BuildEnvironmentConfig behind ActionSecretViewValue
		srcBuildEnv, becErr := h.findBuildEnvironmentConfig(db, model.BuildEnvironmentScopeDeployment, srcWithHooks.ID)
		if becErr != nil && !errors.Is(becErr, gorm.ErrRecordNotFound) {
			return preparedCreateTargetPlan{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", becErr.Error())
		}
		srcBuildVars := buildenv.Decode(srcBuildEnv.Variables)
		srcBuildSecrets := buildenv.Decode(srcBuildEnv.SecretRefs)
		if len(srcBuildVars) > 0 || len(srcBuildSecrets) > 0 {
			canViewBuildSecrets, _ := h.projectRoleActionAllowed(tc.Context(), user, project.ID, authz.ActionSecretViewValue)
			if canViewBuildSecrets {
				if len(srcBuildVars) > 0 {
					buildEnvironment = &model.BuildEnvironmentConfig{
						ID:         id.New("bec"),
						Scope:      model.BuildEnvironmentScopeDeployment,
						ScopeRef:   targetID,
						Variables:  buildenv.Encode(srcBuildVars),
						SecretRefs: "{}",
						UpdatedBy:  user.ID,
					}
				}
				for k, ref := range srcBuildSecrets {
					if strings.TrimSpace(k) != "" && strings.TrimSpace(ref) != "" {
						secretReqs.Build = append(secretReqs.Build, strings.TrimSpace(k))
					}
				}
				sort.Strings(secretReqs.Build)
			} else {
				warnings = append(warnings, "Skipped deployment-scoped build environment variables because caller lacks secret:view_value permission; a project owner/admin may configure them if needed.")
			}
		}

		secretReqs.RuntimeEnv = runtimeSecretKeys(srcWithHooks.SecretRefs)
		if secretReqs.RuntimeEnv == nil {
			secretReqs.RuntimeEnv = []string{}
		}
		secretReqs.RuntimeFile = sortedSecretFilePaths(srcWithHooks.SecretFiles)
		if secretReqs.RuntimeFile == nil {
			secretReqs.RuntimeFile = []string{}
		}

		baseCfg, cfgErr := deploymentBundleConfiguration(srcWithHooks, nil, model.BuildEnvironmentConfig{})
		if cfgErr != nil {
			return preparedCreateTargetPlan{}, aitool.NewError(http.StatusInternalServerError, "deployment_target.clone_failed", cfgErr.Error())
		}
		if shouldResetGeneratedTargetRepository(tc.Context(), h, project, app, srcWithHooks) {
			baseCfg.TargetRepository = ""
			baseCfg.TargetTag = ""
			baseCfg.TargetImageRef = ""
		}

		rawInput = baseCfg
		rawInput.Stage = stage
		rawInput.Name = fallback(strings.TrimSpace(in.Name), stage)
		rawInput.ClusterID = destCluster.ID
		rawInput.Enabled = true
		rawInput.AutoDeploy = false // Prevent premature auto-deploy before secrets are configured
		rawInput.RepositoryBindingID = srcWithHooks.RepositoryBindingID
		rawInput.BuildEnvironmentID = srcWithHooks.BuildEnvironmentID
		rawInput.TargetRegistryID = srcWithHooks.TargetRegistryID
		rawInput.BuildVariableSetIDs = buildVariableSetIDs(srcWithHooks.BuildVariableSetIDs)
		rawInput.DataVolumes = clonedVolumes

		// Preserve RuntimeConfigRefs while forcing fresh snapshot capture from live ProjectRuntimeConfigSet
		for _, r := range model.DecodeDeploymentRuntimeConfigRefs(srcWithHooks.RuntimeConfigRefs) {
			rawInput.RuntimeConfigRefs = append(rawInput.RuntimeConfigRefs, deploymentRuntimeConfigRefInput{
				SetID: r.SetID,
				Mode:  model.RuntimeConfigRefMode(r.Mode),
			})
		}
		for _, hb := range srcWithHooks.BuildHookBindings {
			hookInputs = append(hookInputs, deploymentTargetHookBindingInput{
				HookConfigID: hb.HookConfigID,
				Phase:        hb.Phase,
				RunOrder:     hb.RunOrder,
			})
		}

		// Apply optional caller overrides on top of cloned configuration
		if err := h.applyCreateOverrides(tc, user, project, app, &rawInput, in, false); err != nil {
			return preparedCreateTargetPlan{}, err
		}

		if len(secretReqs.Build) > 0 || len(secretReqs.RuntimeEnv) > 0 || len(secretReqs.RuntimeFile) > 0 {
			warnings = append(warnings, "Cloned target has autoDeploy=false and requires configuring missing secrets via `delivery_configure_target_config` / `build_configure_scoped_env` before deploying.")
		}
	} else {
		if len(in.VolumeOverrides) > 0 || in.OmitProjectVolumes != nil {
			return preparedCreateTargetPlan{}, aitool.BadRequest("argument_invalid", "volumeOverrides and omitProjectVolumes are only valid with cloneFromStage")
		}
		if strings.TrimSpace(in.SourceType) == "" {
			return preparedCreateTargetPlan{}, aitool.BadRequest("deployment_target.source_type_required", "sourceType (`image` or `repository`) is required when cloneFromStage is not set")
		}
		destCluster, cErr := h.resolveRuntimeClusterForProject(tc, user, project.ID, in.Cluster)
		if cErr != nil {
			return preparedCreateTargetPlan{}, cErr
		}
		defaultHooks := true
		rawInput = deploymentTargetInput{
			Name:              fallback(strings.TrimSpace(in.Name), stage),
			Stage:             stage,
			ClusterID:         destCluster.ID,
			WorkloadType:      fallback(strings.TrimSpace(in.WorkloadType), "Deployment"),
			CPURequest:        fallback(strings.TrimSpace(in.CPURequest), model.DefaultDeploymentCPURequest),
			MemoryRequest:     fallback(strings.TrimSpace(in.MemoryRequest), model.DefaultDeploymentMemoryRequest),
			BuildHooksEnabled: &defaultHooks,
			Enabled:           true,
		}
		if err := h.applyCreateOverrides(tc, user, project, app, &rawInput, in, true); err != nil {
			return preparedCreateTargetPlan{}, err
		}
	}

	scratchCtx, rec := newScratchGin(tc)
	// Pass existingRuntimeConfigRefs = "" so any snapshot-mode configSet captures a fresh snapshot from DB.
	targetModel, normVolumes, _, ok := h.deploymentTargetFromInput(scratchCtx, user, app, rawInput, targetID, kubernetesName, nil, "")
	if !ok {
		return preparedCreateTargetPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "deployment_target.invalid", "deployment target configuration is invalid")
	}

	previewChanges := []aitool.PreviewChange{
		{Field: "stage", After: targetModel.Stage, Action: "create"},
		{Field: "clusterId", After: targetModel.ClusterID, Action: "create"},
		{Field: "workloadType", After: targetModel.WorkloadType, Action: "create"},
		{Field: "sourceType", After: targetModel.SourceType, Action: "create"},
		{Field: "replicas", After: targetModel.Replicas, Action: "create"},
		{Field: "resources", After: targetModel.CPURequest + " / " + targetModel.MemoryRequest, Action: "create"},
	}
	if targetModel.ImageRef != "" {
		previewChanges = append(previewChanges, aitool.PreviewChange{Field: "imageRef", After: targetModel.ImageRef, Action: "create"})
	}
	if cloneStage != "" {
		previewChanges = append(previewChanges, aitool.PreviewChange{Field: "cloneFromStage", After: cloneStage, Action: "clone"})
	}

	return preparedCreateTargetPlan{
		user:               user,
		project:            project,
		app:                app,
		target:             targetModel,
		dataVolumes:        normVolumes,
		hookInputs:         hookInputs,
		buildEnvironment:   buildEnvironment,
		secretRequirements: secretReqs,
		warnings:           warnings,
		previewChanges:     previewChanges,
	}, nil
}

func (h *Handler) applyCreateOverrides(
	tc *aitool.ToolContext,
	user model.User,
	project model.Project,
	app model.Application,
	dst *deploymentTargetInput,
	in deliveryCreateTargetInput,
	freshCreate bool,
) error {
	db := h.aiDB(tc)
	if strings.TrimSpace(in.WorkloadType) != "" {
		dst.WorkloadType = strings.TrimSpace(in.WorkloadType)
	}
	if strings.TrimSpace(in.SourceType) != "" {
		st := strings.ToLower(strings.TrimSpace(in.SourceType))
		if st != "image" && st != "repository" {
			return aitool.BadRequest("argument_invalid", "sourceType must be image or repository")
		}
		dst.SourceType = st
	}
	if in.Replicas != nil {
		dst.Replicas = in.Replicas
	} else if freshCreate {
		defRep := 1
		dst.Replicas = &defRep
	}
	if strings.TrimSpace(in.CPURequest) != "" {
		dst.CPURequest = strings.TrimSpace(in.CPURequest)
	}
	if strings.TrimSpace(in.MemoryRequest) != "" {
		dst.MemoryRequest = strings.TrimSpace(in.MemoryRequest)
	}
	if in.InternalOnly != nil {
		dst.InternalOnly = *in.InternalOnly
	}
	if len(in.ServicePorts) > 0 {
		ports := make([]model.DeploymentServicePort, 0, len(in.ServicePorts))
		for _, p := range in.ServicePorts {
			ports = append(ports, model.DeploymentServicePort{Name: p.Name, Port: p.Port, AppProtocol: p.AppProtocol})
		}
		dst.ServicePorts = ports
	}

	sourceType := normalizeDeploymentSourceType(dst.SourceType)
	if sourceType == "image" {
		if strings.TrimSpace(in.ImageRef) != "" {
			dst.ImageRef = strings.TrimSpace(in.ImageRef)
		}
		if strings.TrimSpace(dst.ImageRef) == "" {
			return aitool.BadRequest("deployment_target.image_ref_required", "imageRef is required when sourceType=image")
		}
	} else {
		if strings.TrimSpace(in.RepositoryBinding) != "" {
			binding, err := resolveRepositoryBindingForApp(db, project.ID, app.ID, in.RepositoryBinding)
			if err != nil {
				return err
			}
			dst.RepositoryBindingID = binding.ID
		} else if freshCreate && strings.TrimSpace(dst.RepositoryBindingID) == "" {
			return aitool.BadRequest("deployment_target.repository_binding_required", "repositoryBinding is required when sourceType=repository")
		}

		if strings.TrimSpace(in.TargetRegistry) != "" {
			reg, err := h.resolveArtifactRegistryForProject(tc, user, project.ID, in.TargetRegistry)
			if err != nil {
				return err
			}
			dst.TargetRegistryID = reg.ID
		} else if freshCreate && strings.TrimSpace(dst.TargetRegistryID) == "" {
			return aitool.BadRequest("deployment_target.target_registry_required", "targetRegistry is required when sourceType=repository")
		}

		if strings.TrimSpace(in.DockerfilePath) != "" {
			cleaned, err := validateRelativeRepoPath(in.DockerfilePath, "dockerfilePath", false)
			if err != nil {
				return err
			}
			dst.DockerfilePath = cleaned
		} else if strings.TrimSpace(dst.DockerfilePath) == "" {
			dst.DockerfilePath = "Dockerfile"
		}

		if strings.TrimSpace(in.BuildContext) != "" {
			cleaned, err := validateRelativeRepoPath(in.BuildContext, "buildContext", true)
			if err != nil {
				return err
			}
			dst.BuildContext = cleaned
		} else if strings.TrimSpace(dst.BuildContext) == "" {
			dst.BuildContext = "."
		}

		if strings.TrimSpace(in.BuildTemplate) != "" {
			dst.BuildDefinitionMode = buildtemplate.DefinitionModeTemplate
			dst.BuildTemplateID = strings.TrimSpace(in.BuildTemplate)
			dst.BuildTemplateVersion = ""
			if len(in.BuildTemplateValues) > 0 {
				encoded, _ := json.Marshal(in.BuildTemplateValues)
				dst.BuildTemplateValues = string(encoded)
			} else {
				dst.BuildTemplateValues = "{}"
			}
		} else if len(in.BuildTemplateValues) > 0 {
			if strings.TrimSpace(dst.BuildTemplateID) == "" {
				return aitool.BadRequest("argument_invalid", "buildTemplateValues requires buildTemplate")
			}
			encoded, _ := json.Marshal(in.BuildTemplateValues)
			dst.BuildTemplateValues = string(encoded)
		}
	}
	return nil
}

func (h *Handler) previewDeliveryCreateTarget(tc *aitool.ToolContext, in deliveryCreateTargetInput) (*aitool.PreviewResult, error) {
	plan, err := h.prepareCreateTargetPlan(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("在应用 %s 下创建 %s 环境部署目标", plan.app.Identifier, plan.target.Stage),
			TargetResource: fmt.Sprintf("%s/%s", plan.app.Identifier, plan.target.Stage),
			ProjectID:      plan.project.ID,
			Changes:        plan.previewChanges,
			Warnings:       plan.warnings,
		},
	}, nil
}

func (h *Handler) executeDeliveryCreateTarget(tc *aitool.ToolContext, in deliveryCreateTargetInput) (deliveryCreateTargetOutput, error) {
	plan, err := h.prepareCreateTargetPlan(tc, in)
	if err != nil {
		return deliveryCreateTargetOutput{}, err
	}
	ctx := tc.Context()
	target := plan.target
	changes, persistErr := h.persistDeploymentTargetWithAfterPersist(
		target,
		plan.dataVolumes,
		nil,
		plan.buildEnvironment,
		nil,
		true,
		func(tx *gorm.DB, _ model.DeploymentTarget) error {
			if len(plan.hookInputs) > 0 {
				if _, hErr := h.replaceDeploymentTargetHookBindingsPreserveOrder(tx, target, plan.hookInputs); hErr != nil {
					return hErr
				}
			}
			return nil
		},
		ctx,
	)
	if errors.Is(persistErr, errDeploymentStageExists) {
		return deliveryCreateTargetOutput{}, aitool.Conflict("deployment.stage_exists", "deployment stage already exists in this application")
	}
	if persistErr != nil {
		h.auditDeploymentVolumeMountFailure(ctx, plan.user.ID, changes, persistErr)
		if vCode := volume.ErrorCode(persistErr); vCode != "" {
			return deliveryCreateTargetOutput{}, mapVolumeDomainError(persistErr)
		}
		return deliveryCreateTargetOutput{}, aitool.BadRequest("deployment_target.create_failed", persistErr.Error())
	}

	h.auditDeploymentVolumeMountChanges(ctx, plan.user.ID, target, changes)
	h.auditWithContext(plan.user.ID, "deployment_target.create", target.ID, true, target.Stage, ctx)
	observed := h.observeDeploymentTarget(ctx, plan.project, target)

	return deliveryCreateTargetOutput{
		ID:                 target.ID,
		DeploymentTargetID: target.ID,
		ProjectID:          plan.project.ID,
		ApplicationID:      plan.app.ID,
		Name:               target.Name,
		Stage:              target.Stage,
		ClusterID:          target.ClusterID,
		EffectiveClusterID: observed.EffectiveClusterID,
		WorkloadType:       normalizeWorkloadType(target.WorkloadType),
		SourceType:         normalizeDeploymentSourceType(target.SourceType),
		ImageRef:           target.ImageRef,
		Replicas:           target.Replicas,
		CPURequest:         target.CPURequest,
		MemoryRequest:      target.MemoryRequest,
		ServicePorts:       model.DeploymentTargetServicePorts(target),
		InternalOnly:       target.InternalOnly,
		AutoDeploy:         target.AutoDeploy,
		RequiresRelease:    true,
		SecretRequirements: plan.secretRequirements,
		Warnings:           plan.warnings,
	}, nil
}

// ============================================================================
// Tool 5 Implementation: delivery_update_target_runtime
// ============================================================================

type preparedRuntimePatchPlan struct {
	user            model.User
	project         model.Project
	app             model.Application
	existing        model.DeploymentTarget
	next            model.DeploymentTarget
	clusterChanged  bool
	internalChanged bool
	requiresRelease bool
	pendingFields   []string
	changes         []aitool.PreviewChange
}

func (h *Handler) prepareRuntimePatchPlan(tc *aitool.ToolContext, in deliveryUpdateTargetRuntimeInput) (preparedRuntimePatchPlan, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentUpdate)
	if err != nil {
		return preparedRuntimePatchPlan{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return preparedRuntimePatchPlan{}, err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return preparedRuntimePatchPlan{}, err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return preparedRuntimePatchPlan{}, err
	}
	existing, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return preparedRuntimePatchPlan{}, err
	}
	if err := h.ensureAIDeploymentTargetCanMutate(tc, existing); err != nil {
		return preparedRuntimePatchPlan{}, err
	}
	if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
		return preparedRuntimePatchPlan{}, err
	}

	// Start from a full copy of existing so 100% of unexposed & omitted DB columns are preserved!
	next := existing
	var (
		changes       []aitool.PreviewChange
		pendingFields []string
		reqRelease    bool
	)
	recordChange := func(field string, before, after any, needsRelease bool) {
		changes = append(changes, aitool.PreviewChange{Field: field, Before: before, After: after, Action: "update"})
		if needsRelease {
			reqRelease = true
			if !slices.Contains(pendingFields, field) {
				pendingFields = append(pendingFields, field)
			}
		}
	}

	if name := strings.TrimSpace(in.Name); name != "" && name != existing.Name {
		next.Name = name
		recordChange("name", existing.Name, next.Name, false)
	}

	clusterChanged := false
	if strings.TrimSpace(in.Cluster) != "" {
		destCluster, cErr := h.resolveRuntimeClusterForProject(tc, user, project.ID, in.Cluster)
		if cErr != nil {
			return preparedRuntimePatchPlan{}, cErr
		}
		if strings.TrimSpace(destCluster.ID) != strings.TrimSpace(existing.ClusterID) {
			if err := validateTargetClusterSwitchTx(db, project.ID, app.ID, existing.ID, destCluster.ID); err != nil {
				return preparedRuntimePatchPlan{}, err
			}
			clusterChanged = true
			next.ClusterID = destCluster.ID
			recordChange("clusterId", existing.ClusterID, next.ClusterID, true)
		}
	}

	if strings.TrimSpace(in.WorkloadType) != "" {
		wt := normalizeWorkloadType(in.WorkloadType)
		if strings.TrimSpace(in.WorkloadType) != "Deployment" && strings.TrimSpace(in.WorkloadType) != "StatefulSet" {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "workloadType must be Deployment or StatefulSet")
		}
		if wt != normalizeWorkloadType(existing.WorkloadType) {
			next.WorkloadType = wt
			recordChange("workloadType", normalizeWorkloadType(existing.WorkloadType), wt, true)
		}
	}

	if strings.TrimSpace(in.ImageRef) != "" {
		img := strings.TrimSpace(in.ImageRef)
		if img != strings.TrimSpace(existing.ImageRef) {
			next.ImageRef = img
			recordChange("imageRef", existing.ImageRef, img, true)
		}
	}

	if boolVal(in.ResetImagePullPolicy, false) {
		if strings.TrimSpace(in.ImagePullPolicy) != "" {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "imagePullPolicy and resetImagePullPolicy=true are mutually exclusive")
		}
		if existing.ImagePullPolicy != "" {
			next.ImagePullPolicy = ""
			recordChange("imagePullPolicy", existing.ImagePullPolicy, "", true)
		}
	} else if strings.TrimSpace(in.ImagePullPolicy) != "" {
		ipp := normalizeImagePullPolicyValue(in.ImagePullPolicy)
		if ipp == "" {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "imagePullPolicy must be IfNotPresent, Always, or Never")
		}
		if ipp != normalizeImagePullPolicyValue(existing.ImagePullPolicy) {
			next.ImagePullPolicy = ipp
			recordChange("imagePullPolicy", normalizeImagePullPolicyValue(existing.ImagePullPolicy), ipp, true)
		}
	}

	if strings.TrimSpace(in.CPURequest) != "" {
		cpuQ, qErr := normalizeBuildResourceQuantityValue(in.CPURequest, "", "cpuRequest")
		if qErr != nil {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("deployment.resource_quantity_invalid", "cpuRequest must be a positive Kubernetes quantity")
		}
		if cpuQ != existing.CPURequest {
			next.CPURequest = cpuQ
			recordChange("cpuRequest", existing.CPURequest, cpuQ, true)
		}
	}

	if strings.TrimSpace(in.MemoryRequest) != "" {
		memQ, qErr := normalizeBuildResourceQuantityValue(in.MemoryRequest, "", "memoryRequest")
		if qErr != nil {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("deployment.resource_quantity_invalid", "memoryRequest must be a positive Kubernetes quantity")
		}
		if memQ != existing.MemoryRequest {
			next.MemoryRequest = memQ
			recordChange("memoryRequest", existing.MemoryRequest, memQ, true)
		}
	}

	// AutoScaling pure patch: when enabled=false, preserve existing thresholds & AutoScalingBehavior!
	if in.AutoScaling != nil {
		as := in.AutoScaling
		nextEnabled := existing.AutoScalingEnabled
		if as.Enabled != nil {
			nextEnabled = *as.Enabled
		}
		nextMin := fallbackInt(existing.AutoScalingMinReplicas, 1)
		if as.MinReplicas != nil {
			nextMin = *as.MinReplicas
		}
		nextMax := fallbackInt(existing.AutoScalingMaxReplicas, max(nextMin, fallbackInt(existing.Replicas, 1)))
		if as.MaxReplicas != nil {
			nextMax = *as.MaxReplicas
		}
		nextCPU := existing.AutoScalingCPUPercent
		if as.CPUPercent != nil {
			nextCPU = *as.CPUPercent
		}
		nextMem := existing.AutoScalingMemoryPercent
		if as.MemoryPercent != nil {
			nextMem = *as.MemoryPercent
		}
		if nextMin < 1 || nextMax < 1 || nextMax < nextMin {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "autoScaling maxReplicas must be >= minReplicas (>= 1)")
		}
		if nextCPU < 0 || nextCPU > 1000 || nextMem < 0 || nextMem > 1000 {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "autoScaling cpuPercent and memoryPercent must be between 0 and 1000")
		}
		if nextEnabled && nextCPU == 0 && nextMem == 0 {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "enabling autoScaling requires at least one of cpuPercent or memoryPercent >= 1")
		}
		if nextEnabled != existing.AutoScalingEnabled ||
			nextMin != existing.AutoScalingMinReplicas ||
			nextMax != existing.AutoScalingMaxReplicas ||
			nextCPU != existing.AutoScalingCPUPercent ||
			nextMem != existing.AutoScalingMemoryPercent {
			beforeSummary := fmt.Sprintf("enabled=%t min=%d max=%d cpu=%d%% mem=%d%%", existing.AutoScalingEnabled, existing.AutoScalingMinReplicas, existing.AutoScalingMaxReplicas, existing.AutoScalingCPUPercent, existing.AutoScalingMemoryPercent)
			next.AutoScalingEnabled = nextEnabled
			next.AutoScalingMinReplicas = nextMin
			next.AutoScalingMaxReplicas = nextMax
			next.AutoScalingCPUPercent = nextCPU
			next.AutoScalingMemoryPercent = nextMem
			// Existing AutoScalingBehavior is preserved intact on `next`!
			afterSummary := fmt.Sprintf("enabled=%t min=%d max=%d cpu=%d%% mem=%d%%", nextEnabled, nextMin, nextMax, nextCPU, nextMem)
			recordChange("autoScaling", beforeSummary, afterSummary, true)
		}
	}

	// Command & Args
	if boolVal(in.ResetCommand, false) {
		if len(in.Command) > 0 {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "command and resetCommand=true are mutually exclusive")
		}
		if strings.TrimSpace(existing.ContainerCommand) != "" {
			next.ContainerCommand = ""
			recordChange("command", decodeStringListField(existing.ContainerCommand), []string{}, true)
		}
	} else if len(in.Command) > 0 {
		encodedCmd, err := encodeNonEmptyStringSliceJSON(in.Command, "command")
		if err != nil {
			return preparedRuntimePatchPlan{}, err
		}
		if encodedCmd != existing.ContainerCommand {
			next.ContainerCommand = encodedCmd
			recordChange("command", decodeStringListField(existing.ContainerCommand), decodeStringListField(encodedCmd), true)
		}
	}

	if boolVal(in.ResetArgs, false) {
		if len(in.Args) > 0 {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "args and resetArgs=true are mutually exclusive")
		}
		if strings.TrimSpace(existing.ContainerArgs) != "" {
			next.ContainerArgs = ""
			recordChange("args", decodeStringListField(existing.ContainerArgs), []string{}, true)
		}
	} else if len(in.Args) > 0 {
		encodedArgs, err := encodeNonEmptyStringSliceJSON(in.Args, "args")
		if err != nil {
			return preparedRuntimePatchPlan{}, err
		}
		if encodedArgs != existing.ContainerArgs {
			next.ContainerArgs = encodedArgs
			recordChange("args", decodeStringListField(existing.ContainerArgs), decodeStringListField(encodedArgs), true)
		}
	}

	// Ports upsert & remove + inbound ServiceBinding / GatewayRoute conflict checks
	if len(in.UpsertPorts) > 0 || len(in.RemovePortNames) > 0 {
		currentPorts := model.DeploymentTargetServicePorts(existing)
		removeSet := map[string]bool{}
		for _, rawName := range in.RemovePortNames {
			n := strings.ToLower(strings.TrimSpace(rawName))
			if n != "" {
				removeSet[n] = true
			}
		}
		merged := make([]model.DeploymentServicePort, 0, len(currentPorts)+len(in.UpsertPorts))
		upsertedByName := map[string]model.DeploymentServicePort{}
		upsertOrder := make([]string, 0, len(in.UpsertPorts))
		for idx, up := range in.UpsertPorts {
			if strings.TrimSpace(up.Name) == "" {
				return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "upsertPorts[].name is required")
			}
			normName := normalizeDeploymentServicePortName(up.Name, up.Port, idx)
			if removeSet[normName] {
				return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("port %q cannot be both upserted and removed", normName))
			}
			if _, dup := upsertedByName[normName]; dup {
				return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("duplicate port name %q in upsertPorts", normName))
			}
			upsertedByName[normName] = model.DeploymentServicePort{Name: normName, Port: up.Port, AppProtocol: normalizeAppProtocol(up.AppProtocol)}
			upsertOrder = append(upsertOrder, normName)
		}

		consumedUpsert := map[string]bool{}
		for _, cp := range currentPorts {
			if removeSet[cp.Name] {
				continue
			}
			if replacement, exists := upsertedByName[cp.Name]; exists {
				merged = append(merged, replacement)
				consumedUpsert[cp.Name] = true
			} else {
				merged = append(merged, cp)
			}
		}
		for _, normName := range upsertOrder {
			if !consumedUpsert[normName] {
				merged = append(merged, upsertedByName[normName])
			}
		}
		if len(merged) == 0 {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("deployment_target.ports_empty", "cannot remove all service ports; at least one port must remain")
		}
		scratchCtx, rec := newScratchGin(tc)
		validatedPorts, ok := normalizeDeploymentServicePorts(scratchCtx, merged, 0)
		if !ok {
			return preparedRuntimePatchPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "deployment_target.ports_invalid", "service ports are invalid")
		}
		if err := validateTargetPortsDependenciesTx(db, project.ID, existing.ID, validatedPorts); err != nil {
			return preparedRuntimePatchPlan{}, err
		}
		encodedPorts := model.EncodeDeploymentServicePorts(validatedPorts, validatedPorts[0].Port)
		if encodedPorts != existing.ServicePorts {
			next.ServicePorts = encodedPorts
			recordChange("servicePorts", currentPorts, validatedPorts, true)
		}
	}

	// Probes patch & clear
	clearProbeSet := map[string]bool{}
	for _, pName := range in.ClearProbes {
		pName = strings.TrimSpace(pName)
		switch pName {
		case "readiness", "liveness", "startup":
			clearProbeSet[pName] = true
		default:
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "clearProbes items must be readiness, liveness, or startup")
		}
	}
	if clearProbeSet["readiness"] {
		if in.Probes != nil && in.Probes.Readiness != nil {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "readiness probe cannot be both set and cleared")
		}
		if existing.ReadinessProbe != "" {
			next.ReadinessProbe = ""
			recordChange("probes.readiness", "configured", "cleared", true)
		}
	} else if in.Probes != nil && in.Probes.Readiness != nil {
		rawProbe, pErr := marshalAndValidateProbeSpec(tc, *in.Probes.Readiness, "probes.readiness")
		if pErr != nil {
			return preparedRuntimePatchPlan{}, pErr
		}
		if rawProbe != existing.ReadinessProbe {
			next.ReadinessProbe = rawProbe
			recordChange("probes.readiness", existing.ReadinessProbe != "", rawProbe, true)
		}
	}
	if clearProbeSet["liveness"] {
		if in.Probes != nil && in.Probes.Liveness != nil {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "liveness probe cannot be both set and cleared")
		}
		if existing.LivenessProbe != "" {
			next.LivenessProbe = ""
			recordChange("probes.liveness", "configured", "cleared", true)
		}
	} else if in.Probes != nil && in.Probes.Liveness != nil {
		rawProbe, pErr := marshalAndValidateProbeSpec(tc, *in.Probes.Liveness, "probes.liveness")
		if pErr != nil {
			return preparedRuntimePatchPlan{}, pErr
		}
		if rawProbe != existing.LivenessProbe {
			next.LivenessProbe = rawProbe
			recordChange("probes.liveness", existing.LivenessProbe != "", rawProbe, true)
		}
	}
	if clearProbeSet["startup"] {
		if in.Probes != nil && in.Probes.Startup != nil {
			return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "startup probe cannot be both set and cleared")
		}
		if existing.StartupProbe != "" {
			next.StartupProbe = ""
			recordChange("probes.startup", "configured", "cleared", true)
		}
	} else if in.Probes != nil && in.Probes.Startup != nil {
		rawProbe, pErr := marshalAndValidateProbeSpec(tc, *in.Probes.Startup, "probes.startup")
		if pErr != nil {
			return preparedRuntimePatchPlan{}, pErr
		}
		if rawProbe != existing.StartupProbe {
			next.StartupProbe = rawProbe
			recordChange("probes.startup", existing.StartupProbe != "", rawProbe, true)
		}
	}

	// Security & Scheduling patch
	if in.SecurityAndScheduling != nil {
		sec := in.SecurityAndScheduling
		if boolVal(sec.ClearSecurityContext, false) {
			if existing.RunAsUser != "" || existing.RunAsGroup != "" || existing.FSGroup != "" || existing.ReadOnlyRootFilesystem {
				next.RunAsUser = ""
				next.RunAsGroup = ""
				next.FSGroup = ""
				next.ReadOnlyRootFilesystem = false
				recordChange("securityContext", "custom", "cleared", true)
			}
		} else {
			if boolVal(sec.ClearRunAsUser, false) {
				if sec.RunAsUser != nil {
					return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "runAsUser and clearRunAsUser=true are mutually exclusive")
				}
				if existing.RunAsUser != "" {
					next.RunAsUser = ""
					recordChange("securityAndScheduling.runAsUser", existing.RunAsUser, "", true)
				}
			} else if sec.RunAsUser != nil {
				if *sec.RunAsUser < 0 {
					return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "runAsUser must be >= 0")
				}
				val := strconv.FormatInt(*sec.RunAsUser, 10)
				if val != strings.TrimSpace(existing.RunAsUser) {
					next.RunAsUser = val
					recordChange("securityAndScheduling.runAsUser", existing.RunAsUser, val, true)
				}
			}
			if boolVal(sec.ClearRunAsGroup, false) {
				if sec.RunAsGroup != nil {
					return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "runAsGroup and clearRunAsGroup=true are mutually exclusive")
				}
				if existing.RunAsGroup != "" {
					next.RunAsGroup = ""
					recordChange("securityAndScheduling.runAsGroup", existing.RunAsGroup, "", true)
				}
			} else if sec.RunAsGroup != nil {
				if *sec.RunAsGroup < 0 {
					return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "runAsGroup must be >= 0")
				}
				val := strconv.FormatInt(*sec.RunAsGroup, 10)
				if val != strings.TrimSpace(existing.RunAsGroup) {
					next.RunAsGroup = val
					recordChange("securityAndScheduling.runAsGroup", existing.RunAsGroup, val, true)
				}
			}
			if boolVal(sec.ClearFSGroup, false) {
				if sec.FSGroup != nil {
					return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "fsGroup and clearFsGroup=true are mutually exclusive")
				}
				if existing.FSGroup != "" {
					next.FSGroup = ""
					recordChange("securityAndScheduling.fsGroup", existing.FSGroup, "", true)
				}
			} else if sec.FSGroup != nil {
				if *sec.FSGroup < 0 {
					return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "fsGroup must be >= 0")
				}
				val := strconv.FormatInt(*sec.FSGroup, 10)
				if val != strings.TrimSpace(existing.FSGroup) {
					next.FSGroup = val
					recordChange("securityAndScheduling.fsGroup", existing.FSGroup, val, true)
				}
			}
			if sec.ReadOnlyRootFilesystem != nil && *sec.ReadOnlyRootFilesystem != existing.ReadOnlyRootFilesystem {
				next.ReadOnlyRootFilesystem = *sec.ReadOnlyRootFilesystem
				recordChange("securityAndScheduling.readOnlyRootFilesystem", existing.ReadOnlyRootFilesystem, next.ReadOnlyRootFilesystem, true)
			}
		}

		if boolVal(sec.ClearNodeSelector, false) {
			if len(sec.SetNodeSelector) > 0 || len(sec.RemoveNodeSelectorKeys) > 0 {
				return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "clearNodeSelector=true cannot be combined with setNodeSelector or removeNodeSelectorKeys")
			}
			if strings.TrimSpace(existing.NodeSelector) != "" {
				next.NodeSelector = ""
				recordChange("securityAndScheduling.nodeSelector", decodeStringMapField(existing.NodeSelector), map[string]string{}, true)
			}
		} else if len(sec.SetNodeSelector) > 0 || len(sec.RemoveNodeSelectorKeys) > 0 {
			nsMap := decodeStringMapField(existing.NodeSelector)
			for _, rk := range sec.RemoveNodeSelectorKeys {
				delete(nsMap, strings.TrimSpace(rk))
			}
			for k, v := range sec.SetNodeSelector {
				k = strings.TrimSpace(k)
				if k == "" {
					return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "nodeSelector key cannot be empty")
				}
				nsMap[k] = strings.TrimSpace(v)
			}
			encodedNS := ""
			if len(nsMap) > 0 {
				rawNS, _ := json.Marshal(nsMap)
				encodedNS = string(rawNS)
			}
			if encodedNS != strings.TrimSpace(existing.NodeSelector) {
				next.NodeSelector = encodedNS
				recordChange("securityAndScheduling.nodeSelector", decodeStringMapField(existing.NodeSelector), nsMap, true)
			}
		}
	}

	// InternalOnly
	internalChanged := false
	if in.InternalOnly != nil && *in.InternalOnly != existing.InternalOnly {
		if *in.InternalOnly {
			var activeRoutes int64
			if err := db.Model(&model.GatewayRoute{}).Where("deployment_target_id = ? AND delete_status = ?", existing.ID, "active").Count(&activeRoutes).Error; err != nil {
				return preparedRuntimePatchPlan{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			if activeRoutes > 0 {
				return preparedRuntimePatchPlan{}, aitool.Conflict("deployment_target.internal_has_routes", "remove active gateway routes of this deployment target before making it internalOnly")
			}
		}
		internalChanged = true
		next.InternalOnly = *in.InternalOnly
		recordChange("internalOnly", existing.InternalOnly, next.InternalOnly, false)
	}

	// Governance
	if in.Governance != nil {
		gov := in.Governance
		if gov.Enabled != nil && *gov.Enabled != existing.Enabled {
			next.Enabled = *gov.Enabled
			recordChange("governance.enabled", existing.Enabled, next.Enabled, false)
		}
		if gov.RequireApproval != nil && *gov.RequireApproval != existing.RequireApproval {
			next.RequireApproval = *gov.RequireApproval
			recordChange("governance.requireApproval", existing.RequireApproval, next.RequireApproval, false)
		}
		if policy := strings.TrimSpace(gov.WebConsolePolicy); policy != "" {
			var desiredOverride *bool
			switch policy {
			case "inherit":
				desiredOverride = nil
			case "disabled":
				f := false
				desiredOverride = &f
			default:
				return preparedRuntimePatchPlan{}, aitool.BadRequest("argument_invalid", "governance.webConsolePolicy must be inherit or disabled")
			}
			curPolicy := "inherit"
			if existing.WebConsoleEnabled != nil && !*existing.WebConsoleEnabled {
				curPolicy = "disabled"
			}
			if policy != curPolicy {
				next.WebConsoleEnabled = desiredOverride
				recordChange("governance.webConsolePolicy", curPolicy, policy, false)
			}
		}
	}

	// Preserve platform-managed service account identity when applicable
	scratchCtx, rec := newScratchGin(tc)
	preserveIdentity, ok := h.shouldPreserveSystemComponentServiceAccount(scratchCtx, project, existing, next.ClusterID)
	if !ok {
		return preparedRuntimePatchPlan{}, toolErrorFromRecorder(rec, http.StatusInternalServerError, "database.query_failed", "system component check failed")
	}
	if preserveIdentity {
		next.ServiceAccountName = existing.ServiceAccountName
		next.AutomountServiceAccountToken = existing.AutomountServiceAccountToken
	}

	return preparedRuntimePatchPlan{
		user:            user,
		project:         project,
		app:             app,
		existing:        existing,
		next:            next,
		clusterChanged:  clusterChanged,
		internalChanged: internalChanged,
		requiresRelease: reqRelease,
		pendingFields:   pendingFields,
		changes:         changes,
	}, nil
}

func validateTargetClusterSwitchTx(db *gorm.DB, projectID, appID, targetID, nextClusterID string) error {
	var releaseCount int64
	if err := db.Model(&model.Release{}).
		Where("project_id = ? AND application_id = ? AND deployment_target_id = ?", projectID, appID, targetID).
		Count(&releaseCount).Error; err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if releaseCount > 0 {
		return aitool.NewErrorWithHint(
			http.StatusConflict,
			"deployment_target.cluster_immutable_after_deploy",
			"cannot switch runtime cluster after a release has been created for this deployment target",
			"Use `delivery_create_target` with `cloneFromStage` to create a target in the new cluster instead.",
		)
	}

	var mounts []model.DeploymentVolumeMount
	if err := db.Where("project_id = ? AND deployment_target_id = ? AND source_type = ? AND activation_state <> ?",
		projectID, targetID, model.DeploymentVolumeSourceProjectVolume, model.DeploymentVolumeActivationReleasePending).
		Find(&mounts).Error; err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	for _, m := range mounts {
		if m.ProjectVolumeID == nil || strings.TrimSpace(*m.ProjectVolumeID) == "" {
			continue
		}
		var vol model.ProjectVolume
		if err := db.First(&vol, "id = ? AND project_id = ?", *m.ProjectVolumeID, projectID).Error; err == nil {
			if strings.TrimSpace(vol.ClusterID) != strings.TrimSpace(nextClusterID) {
				return aitool.Conflict("volume.incompatible_cluster", fmt.Sprintf("bound projectVolume %s belongs to cluster %s and cannot move to %s", vol.ID, vol.ClusterID, nextClusterID))
			}
		}
	}

	var activeRoutes int64
	if err := db.Model(&model.GatewayRoute{}).Where("deployment_target_id = ? AND delete_status = ?", targetID, "active").Count(&activeRoutes).Error; err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if activeRoutes > 0 {
		return aitool.Conflict("deployment_target.gateway_route_in_use", "remove active gateway routes before switching runtime cluster")
	}

	var activeBindings int64
	if err := db.Model(&model.ServiceBinding{}).
		Where("project_id = ? AND enabled = ? AND (source_deployment_target_id = ? OR target_deployment_target_id = ?)", projectID, true, targetID, targetID).
		Count(&activeBindings).Error; err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if activeBindings > 0 {
		return aitool.Conflict("service_binding_in_use", "remove active inbound/outbound service bindings before switching runtime cluster")
	}
	return nil
}

func validateTargetPortsDependenciesTx(db *gorm.DB, projectID, targetID string, nextPorts []model.DeploymentServicePort) error {
	byName := make(map[string]int, len(nextPorts))
	byPort := make(map[int]bool, len(nextPorts))
	for _, p := range nextPorts {
		byName[p.Name] = p.Port
		byPort[p.Port] = true
	}

	var inboundBindings []model.ServiceBinding
	if err := db.Where("project_id = ? AND target_deployment_target_id = ? AND enabled = ?", projectID, targetID, true).Find(&inboundBindings).Error; err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var brokenBindings []string
	for _, b := range inboundBindings {
		ok := false
		for _, candidate := range nextPorts {
			if strings.TrimSpace(b.TargetPortName) != "" && candidate.Name != strings.TrimSpace(b.TargetPortName) {
				continue
			}
			if b.TargetPort > 0 && candidate.Port != b.TargetPort {
				continue
			}
			ok = true
			break
		}
		if !ok {
			brokenBindings = append(brokenBindings, b.ID)
		}
	}
	if len(brokenBindings) > 0 {
		tErr := aitool.Conflict("service_binding_in_use", fmt.Sprintf("changing/removing port breaks active inbound service binding(s): %s", strings.Join(brokenBindings, ", ")))
		tErr.Details = map[string]any{"affectedServiceBindingIds": brokenBindings}
		return tErr
	}

	var routes []model.GatewayRoute
	if err := db.Where("deployment_target_id = ? AND delete_status = ?", targetID, "active").Find(&routes).Error; err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var brokenRoutes []string
	for _, r := range routes {
		if !byPort[r.ServicePort] {
			brokenRoutes = append(brokenRoutes, fmt.Sprintf("%s (%s%s -> :%d)", r.ID, r.Host, r.Path, r.ServicePort))
		}
	}
	if len(brokenRoutes) > 0 {
		tErr := aitool.Conflict("deployment_target.gateway_route_in_use", fmt.Sprintf("changing/removing port breaks active gateway route(s): %s", strings.Join(brokenRoutes, ", ")))
		tErr.Details = map[string]any{"affectedGatewayRoutes": brokenRoutes}
		return tErr
	}
	return nil
}

func encodeNonEmptyStringSliceJSON(items []string, fieldName string) (string, error) {
	cleaned := make([]string, 0, len(items))
	for _, s := range items {
		if strings.TrimSpace(s) == "" {
			return "", aitool.BadRequest("argument_invalid", fieldName+" elements cannot be empty strings")
		}
		cleaned = append(cleaned, s)
	}
	raw, err := json.Marshal(cleaned)
	if err != nil {
		return "", aitool.BadRequest("argument_invalid", err.Error())
	}
	return string(raw), nil
}

func marshalAndValidateProbeSpec(tc *aitool.ToolContext, spec aiProbeSpec, label string) (string, error) {
	handlerCount := 0
	var p corev1.Probe
	if spec.HTTPGet != nil {
		handlerCount++
		scheme := corev1.URISchemeHTTP
		if strings.EqualFold(strings.TrimSpace(spec.HTTPGet.Scheme), "HTTPS") {
			scheme = corev1.URISchemeHTTPS
		}
		headers := make([]corev1.HTTPHeader, 0, len(spec.HTTPGet.HTTPHeaders))
		for _, h := range spec.HTTPGet.HTTPHeaders {
			headers = append(headers, corev1.HTTPHeader{Name: strings.TrimSpace(h.Name), Value: h.Value})
		}
		p.HTTPGet = &corev1.HTTPGetAction{
			Path:        fallback(strings.TrimSpace(spec.HTTPGet.Path), "/"),
			Port:        intstr.FromInt32(int32(spec.HTTPGet.Port)),
			Host:        strings.TrimSpace(spec.HTTPGet.Host),
			Scheme:      scheme,
			HTTPHeaders: headers,
		}
	}
	if spec.TCPSocket != nil {
		handlerCount++
		p.TCPSocket = &corev1.TCPSocketAction{
			Port: intstr.FromInt32(int32(spec.TCPSocket.Port)),
			Host: strings.TrimSpace(spec.TCPSocket.Host),
		}
	}
	if spec.Exec != nil {
		handlerCount++
		if len(spec.Exec.Command) == 0 {
			return "", aitool.BadRequest("argument_invalid", label+".exec.command cannot be empty")
		}
		p.Exec = &corev1.ExecAction{Command: append([]string(nil), spec.Exec.Command...)}
	}
	if spec.GRPC != nil {
		handlerCount++
		var svc *string
		if s := strings.TrimSpace(spec.GRPC.Service); s != "" {
			svc = &s
		}
		p.GRPC = &corev1.GRPCAction{Port: int32(spec.GRPC.Port), Service: svc}
	}
	if handlerCount != 1 {
		return "", aitool.BadRequest("argument_invalid", label+" must specify exactly one of httpGet, tcpSocket, exec, or grpc")
	}
	if spec.InitialDelaySeconds != nil {
		p.InitialDelaySeconds = *spec.InitialDelaySeconds
	}
	if spec.PeriodSeconds != nil {
		p.PeriodSeconds = *spec.PeriodSeconds
	}
	if spec.TimeoutSeconds != nil {
		p.TimeoutSeconds = *spec.TimeoutSeconds
	}
	if spec.SuccessThreshold != nil {
		p.SuccessThreshold = *spec.SuccessThreshold
	}
	if spec.FailureThreshold != nil {
		p.FailureThreshold = *spec.FailureThreshold
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return "", aitool.BadRequest("argument_invalid", err.Error())
	}
	scratchCtx, rec := newScratchGin(tc)
	normalized, ok := normalizeProbeJSON(scratchCtx, string(encoded), label)
	if !ok {
		return "", toolErrorFromRecorder(rec, http.StatusBadRequest, "argument_invalid", label+" is invalid")
	}
	return normalized, nil
}

func (h *Handler) previewDeliveryUpdateTargetRuntime(tc *aitool.ToolContext, in deliveryUpdateTargetRuntimeInput) (*aitool.PreviewResult, error) {
	plan, err := h.prepareRuntimePatchPlan(tc, in)
	if err != nil {
		return nil, err
	}
	var warnings []string
	if plan.requiresRelease {
		warnings = append(warnings, "Spec changes require calling `delivery_deploy_target` to take effect on live Kubernetes Pods.")
	}
	if in.Governance != nil && in.Governance.Enabled != nil && !*in.Governance.Enabled {
		warnings = append(warnings, "Setting governance.enabled=false blocks new releases/auto-deploys without stopping running Pods; call `delivery_scale_target(replicas: 0)` to stop Pods.")
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("更新部署目标 %s/%s 运行时配置", plan.app.Identifier, plan.existing.Stage),
			TargetResource: fmt.Sprintf("%s/%s", plan.app.Identifier, plan.existing.Stage),
			TargetID:       plan.existing.ID,
			ProjectID:      plan.project.ID,
			Changes:        plan.changes,
			Warnings:       warnings,
		},
	}, nil
}

func (h *Handler) executeDeliveryUpdateTargetRuntime(tc *aitool.ToolContext, in deliveryUpdateTargetRuntimeInput) (deliveryUpdateTargetRuntimeOutput, error) {
	plan, err := h.prepareRuntimePatchPlan(tc, in)
	if err != nil {
		return deliveryUpdateTargetRuntimeOutput{}, err
	}
	ctx := tc.Context()
	db := h.aiDB(tc)
	next := plan.next

	txErr := db.Transaction(func(tx *gorm.DB) error {
		var locked model.DeploymentTarget
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&locked, "id = ? AND project_id = ?", next.ID, plan.project.ID).Error; err != nil {
			return err
		}
		if !h.host.ResourceCanMutateDuringDelete(locked.DeleteStatus) {
			return aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is in progress")
		}
		if plan.clusterChanged {
			if err := validateTargetClusterSwitchTx(tx, plan.project.ID, plan.app.ID, next.ID, next.ClusterID); err != nil {
				return err
			}
		}
		if err := validateTargetPortsDependenciesTx(tx, plan.project.ID, next.ID, model.DeploymentTargetServicePorts(next)); err != nil {
			return err
		}
		if next.InternalOnly {
			var activeRoutes int64
			if err := tx.Model(&model.GatewayRoute{}).Where("deployment_target_id = ? AND delete_status = ?", next.ID, "active").Count(&activeRoutes).Error; err != nil {
				return err
			}
			if activeRoutes > 0 {
				return aitool.Conflict("deployment_target.internal_has_routes", "remove active gateway routes before making target internalOnly")
			}
		}
		// Keep concurrently owned config/secret columns from the locked row!
		next.EnvVars = locked.EnvVars
		next.SecretRefs = locked.SecretRefs
		next.ConfigFiles = locked.ConfigFiles
		next.SecretFiles = locked.SecretFiles
		next.RuntimeConfigRefs = locked.RuntimeConfigRefs
		next.Replicas = locked.Replicas
		next.Stage = locked.Stage
		next.CreatedBy = locked.CreatedBy
		next.CreatedAt = locked.CreatedAt
		return tx.Save(&next).Error
	})
	if txErr != nil {
		if te, ok := aitool.AsToolError(txErr); ok {
			return deliveryUpdateTargetRuntimeOutput{}, te
		}
		return deliveryUpdateTargetRuntimeOutput{}, aitool.NewError(http.StatusBadRequest, "deployment_target.update_failed", txErr.Error())
	}

	h.auditWithContext(plan.user.ID, "deployment_target.update", next.ID, true, next.Stage, ctx)
	if plan.internalChanged {
		h.auditWithContext(plan.user.ID, "deployment_target.internal_only", next.ID, true, fmt.Sprintf("internalOnly=%t", next.InternalOnly), ctx)
		h.host.EnqueueNetworkPolicyProjectReconcile(ctx, plan.project.ID)
	}
	if plan.clusterChanged {
		peers, peerErr := runtimecluster.SharePeerProjectIDs(db, "deployment_target_id = ?", next.ID)
		if peerErr != nil {
			slog.WarnContext(ctx, "network share peers lookup failed", "event.name", "network_share.peer_lookup_failed", "error", peerErr)
		}
		for _, pID := range peers {
			h.host.EnqueueNetworkPolicyProjectReconcile(ctx, pID)
		}
	}

	return deliveryUpdateTargetRuntimeOutput{
		ID:                 next.ID,
		DeploymentTargetID: next.ID,
		ProjectID:          plan.project.ID,
		ApplicationID:      plan.app.ID,
		Name:               next.Name,
		Stage:              next.Stage,
		ClusterID:          next.ClusterID,
		WorkloadType:       normalizeWorkloadType(next.WorkloadType),
		ImageRef:           next.ImageRef,
		CPURequest:         next.CPURequest,
		MemoryRequest:      next.MemoryRequest,
		ServicePorts:       model.DeploymentTargetServicePorts(next),
		InternalOnly:       next.InternalOnly,
		Enabled:            next.Enabled,
		RequireApproval:    next.RequireApproval,
		RequiresRelease:    plan.requiresRelease,
		PendingFields:      plan.pendingFields,
	}, nil
}

// ============================================================================
// Tool 6 Implementation: delivery_configure_target_config
// ============================================================================

type preparedTargetConfigPlan struct {
	user                 model.User
	project              model.Project
	app                  model.Application
	existing             model.DeploymentTarget
	nextPublicEnv        map[string]string
	encodedEnvVars       string
	secretMutation       runtimeapi.RuntimeSecretMutationInput
	nextConfigFiles      []runtimeConfigFileInput
	encodedConfigFiles   string
	secretFilesSetMap    map[string]string // path -> plaintext content to encrypt in tx
	secretFilesClearList []string          // normalized paths to remove & delete refs in tx
	nextConfigRefs       []model.DeploymentRuntimeConfigRef
	encodedConfigRefs    string
	changes              []aitool.PreviewChange
	sensitiveKeys        []string
}

func (h *Handler) prepareTargetConfigPlan(tc *aitool.ToolContext, in deliveryConfigureTargetConfigInput) (preparedTargetConfigPlan, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentUpdate)
	if err != nil {
		return preparedTargetConfigPlan{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return preparedTargetConfigPlan{}, err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return preparedTargetConfigPlan{}, err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return preparedTargetConfigPlan{}, err
	}
	existing, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return preparedTargetConfigPlan{}, err
	}
	if err := h.ensureAIDeploymentTargetCanMutate(tc, existing); err != nil {
		return preparedTargetConfigPlan{}, err
	}
	if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
		return preparedTargetConfigPlan{}, err
	}

	var (
		changes       []aitool.PreviewChange
		sensitiveKeys []string
	)

	// 1. Public Env patch
	nextPublicEnv := runtimeConfigMap(existing.EnvVars)
	if nextPublicEnv == nil {
		nextPublicEnv = map[string]string{}
	}
	removeEnvSet := map[string]bool{}
	for _, rawKey := range in.RemovePublicEnvKeys {
		k := strings.TrimSpace(rawKey)
		if k == "" {
			continue
		}
		removeEnvSet[k] = true
		if oldVal, existed := nextPublicEnv[k]; existed {
			delete(nextPublicEnv, k)
			changes = append(changes, aitool.PreviewChange{Field: "publicEnv." + k, Before: oldVal, Action: "remove"})
		}
	}
	seenSetEnv := map[string]bool{}
	for _, item := range in.SetPublicEnv {
		k := strings.TrimSpace(item.Key)
		if !isBuildEnvKey(k) {
			return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.runtime_environment_key_invalid", fmt.Sprintf("invalid environment variable key %q (must match ^[A-Za-z_][A-Za-z0-9_]*$)", k))
		}
		if removeEnvSet[k] {
			return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("public env key %q cannot be both set and removed", k))
		}
		if seenSetEnv[k] {
			return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("duplicate key %q in setPublicEnv", k))
		}
		if utf8.RuneCountInString(item.Value) > runtimeapi.MaxRuntimeEnvironmentValueLength {
			return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.runtime_environment_value_too_long", fmt.Sprintf("public env %q value exceeds maximum length", k))
		}
		seenSetEnv[k] = true
		oldVal, hadOld := nextPublicEnv[k]
		nextPublicEnv[k] = item.Value
		action := "set"
		var before any
		if hadOld {
			before = oldVal
			action = "update"
		}
		changes = append(changes, aitool.PreviewChange{Field: "publicEnv." + k, Before: before, After: item.Value, Action: action})
	}
	if len(nextPublicEnv) > runtimeapi.MaxRuntimeEnvironmentVariables {
		return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.runtime_environment_limit_exceeded", "too many runtime environment variables")
	}
	encodedEnvVars, err := runtimeconfig.EncodeKeyValue(nextPublicEnv)
	if err != nil {
		return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.runtime_config_invalid", err.Error())
	}

	// 2. Secret Actions patch
	secretMut := runtimeapi.RuntimeSecretMutationInput{
		Values:   map[string]string{},
		Generate: map[string]runtimeapi.RuntimeSecretGeneration{},
	}
	existingSecretRefs := decodeSecretRefs(existing.SecretRefs)
	nextSecretKeySet := make(map[string]struct{}, len(existingSecretRefs)+len(in.SecretActions))
	for k, ref := range existingSecretRefs {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(ref) != "" {
			nextSecretKeySet[strings.TrimSpace(k)] = struct{}{}
		}
	}

	inboundSecretUsages, err := inboundServiceBindingSecretUsages(db, project.ID, existing.ID)
	if err != nil {
		return preparedTargetConfigPlan{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	seenSecretAction := map[string]bool{}
	for _, sa := range in.SecretActions {
		k := strings.TrimSpace(sa.Key)
		if !isBuildEnvKey(k) {
			return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.secret_key_invalid", fmt.Sprintf("invalid secret key %q", k))
		}
		if seenSecretAction[k] {
			return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.secret_update_conflict", fmt.Sprintf("duplicate secret key %q in secretActions", k))
		}
		seenSecretAction[k] = true
		op := strings.ToLower(strings.TrimSpace(sa.Operation))
		if op == "" {
			op = "set"
		}
		switch op {
		case "set":
			if strings.TrimSpace(sa.Value) == "" {
				return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("secretActions[%s]: value must be non-empty when operation=set", k))
			}
			if utf8.RuneCountInString(sa.Value) > runtimeapi.MaxRuntimeEnvironmentValueLength {
				return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.secret_value_too_long", fmt.Sprintf("secret %q value is too long", k))
			}
			secretMut.Values[k] = sa.Value
			nextSecretKeySet[k] = struct{}{}
			sensitiveKeys = append(sensitiveKeys, k)
			changes = append(changes, aitool.PreviewChange{Field: "secret." + k, Action: "set"})
		case "generate":
			if strings.TrimSpace(sa.Value) != "" {
				return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.secret_update_conflict", fmt.Sprintf("secretActions[%s]: cannot specify value when operation=generate", k))
			}
			gen := runtimeapi.RuntimeSecretGeneration{
				Length:   intVal(sa.Length, 32),
				Encoding: fallback(strings.TrimSpace(sa.Encoding), "base64"),
			}
			if vErr := secret.ValidateGeneration(gen.Length, gen.Encoding); vErr != nil {
				return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.secret_generation_invalid", vErr.Error())
			}
			secretMut.Generate[k] = gen
			nextSecretKeySet[k] = struct{}{}
			sensitiveKeys = append(sensitiveKeys, k)
			changes = append(changes, aitool.PreviewChange{Field: "secret." + k, After: fmt.Sprintf("generated(%s,%d)", gen.Encoding, gen.Length), Action: "generate"})
		case "clear":
			if strings.TrimSpace(sa.Value) != "" || sa.Length != nil || strings.TrimSpace(sa.Encoding) != "" {
				return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.secret_update_conflict", fmt.Sprintf("secretActions[%s]: clear cannot include value, length, or encoding", k))
			}
			if bindingIDs := inboundSecretUsages[k]; len(bindingIDs) > 0 {
				tErr := aitool.Conflict("service_binding.secret_in_use", fmt.Sprintf("secret %q is referenced by active inbound service binding(s): %s", k, strings.Join(bindingIDs, ", ")))
				tErr.Details = map[string]any{"secretKey": k, "dependentServiceBindingIds": bindingIDs}
				return preparedTargetConfigPlan{}, tErr
			}
			secretMut.Clear = append(secretMut.Clear, k)
			delete(nextSecretKeySet, k)
			sensitiveKeys = append(sensitiveKeys, k)
			changes = append(changes, aitool.PreviewChange{Field: "secret." + k, Action: "clear"})
		default:
			return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.secret_operation_invalid", fmt.Sprintf("unsupported secret operation %q", sa.Operation))
		}
	}

	// Bidirectional conflict check between public env and secret keys
	if conflicts := runtimeapi.RuntimeEnvironmentKeyConflicts(encodedEnvVars, nextSecretKeySet); len(conflicts) > 0 {
		return preparedTargetConfigPlan{}, aitool.NewErrorWithHint(
			http.StatusConflict,
			"deployment.runtime_environment_key_conflict",
			fmt.Sprintf("environment variable key(s) %v conflict between publicEnv and encrypted secrets", conflicts),
			"To convert a secret to plaintext, also pass `secretActions: [{key, operation: \"clear\"}]`; to convert plaintext to secret, also pass `removePublicEnvKeys`.",
		)
	}

	// 3. ConfigFiles patch
	configFileMap := map[string]string{}
	for _, f := range decodeConfigFilesList(existing.ConfigFiles) {
		configFileMap[f.Path] = f.Content
	}
	removedCfgPaths := map[string]bool{}
	for _, rawPath := range in.RemoveConfigFilePaths {
		scratchCtx, rec := newScratchGin(tc)
		cleaned, ok := normalizeRuntimeConfigFilePathInput(scratchCtx, rawPath)
		if !ok {
			return preparedTargetConfigPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "deployment.runtime_config_path_invalid", "invalid config file path")
		}
		removedCfgPaths[cleaned] = true
		if _, existed := configFileMap[cleaned]; existed {
			delete(configFileMap, cleaned)
			changes = append(changes, aitool.PreviewChange{Field: "configFile:" + cleaned, Action: "remove"})
		}
	}
	seenUpsertCfg := map[string]bool{}
	for _, item := range in.UpsertConfigFiles {
		scratchCtx, rec := newScratchGin(tc)
		cleaned, ok := normalizeRuntimeConfigFilePathInput(scratchCtx, item.Path)
		if !ok {
			return preparedTargetConfigPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "deployment.runtime_config_path_invalid", "invalid config file path")
		}
		if removedCfgPaths[cleaned] {
			return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("configFile %q cannot be both upserted and removed", cleaned))
		}
		if seenUpsertCfg[cleaned] {
			return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.runtime_config_files_invalid", fmt.Sprintf("duplicate path %q in upsertConfigFiles", cleaned))
		}
		seenUpsertCfg[cleaned] = true
		_, hadOld := configFileMap[cleaned]
		configFileMap[cleaned] = item.Content
		action := "create"
		if hadOld {
			action = "update"
		}
		changes = append(changes, aitool.PreviewChange{Field: "configFile:" + cleaned, After: fmt.Sprintf("%d bytes", len(item.Content)), Action: action})
	}
	nextConfigFiles := make([]runtimeConfigFileInput, 0, len(configFileMap))
	nextConfigFilePaths := make([]string, 0, len(configFileMap))
	for p, c := range configFileMap {
		nextConfigFiles = append(nextConfigFiles, runtimeConfigFileInput{Path: p, Content: c})
		nextConfigFilePaths = append(nextConfigFilePaths, p)
	}
	encodedConfigFiles, err := encodeConfigFilesList(nextConfigFiles)
	if err != nil {
		return preparedTargetConfigPlan{}, aitool.BadRequest("deployment.runtime_config_files_invalid", err.Error())
	}

	// 4. SecretFiles patch
	existingSecretFiles := decodeSecretRefs(existing.SecretFiles)
	nextSecretFilePathSet := map[string]bool{}
	for p, ref := range existingSecretFiles {
		if strings.TrimSpace(p) != "" && strings.TrimSpace(ref) != "" {
			nextSecretFilePathSet[path.Clean(strings.TrimSpace(p))] = true
		}
	}
	secretFilesSetMap := map[string]string{}
	var secretFilesClearList []string
	seenSecretFile := map[string]bool{}
	for _, sf := range in.SecretFileActions {
		scratchCtx, rec := newScratchGin(tc)
		cleaned, ok := normalizeRuntimeConfigFilePathInput(scratchCtx, sf.Path)
		if !ok {
			return preparedTargetConfigPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "deployment.runtime_config_path_invalid", "invalid secret file path")
		}
		if seenSecretFile[cleaned] {
			return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("duplicate path %q in secretFileActions", cleaned))
		}
		seenSecretFile[cleaned] = true
		op := strings.ToLower(strings.TrimSpace(sf.Operation))
		if op == "" {
			op = "set"
		}
		switch op {
		case "set":
			if strings.TrimSpace(sf.Content) == "" {
				return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("secretFileActions[%s]: content must be non-empty when operation=set", cleaned))
			}
			secretFilesSetMap[cleaned] = sf.Content
			nextSecretFilePathSet[cleaned] = true
			sensitiveKeys = append(sensitiveKeys, cleaned)
			changes = append(changes, aitool.PreviewChange{Field: "secretFile:" + cleaned, Action: "set"})
		case "clear":
			if strings.TrimSpace(sf.Content) != "" {
				return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("secretFileActions[%s]: clear cannot include content", cleaned))
			}
			secretFilesClearList = append(secretFilesClearList, cleaned)
			delete(nextSecretFilePathSet, cleaned)
			sensitiveKeys = append(sensitiveKeys, cleaned)
			changes = append(changes, aitool.PreviewChange{Field: "secretFile:" + cleaned, Action: "clear"})
		default:
			return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("unsupported secretFileActions operation %q", sf.Operation))
		}
	}
	nextSecretFilePaths := make([]string, 0, len(nextSecretFilePathSet))
	for p := range nextSecretFilePathSet {
		nextSecretFilePaths = append(nextSecretFilePaths, p)
	}
	sort.Strings(nextSecretFilePaths)

	// 5. ConfigSets patch
	currentConfigRefs := model.DecodeDeploymentRuntimeConfigRefs(existing.RuntimeConfigRefs)
	detachSetIDs := map[string]bool{}
	for _, rawDetach := range in.DetachConfigSets {
		cs, csErr := resolveProjectConfigSet(db, project.ID, rawDetach)
		if csErr != nil {
			// Also allow detaching by exact raw SetID if the config set was deleted
			foundRaw := false
			for _, cr := range currentConfigRefs {
				if cr.SetID == strings.TrimSpace(rawDetach) {
					detachSetIDs[cr.SetID] = true
					foundRaw = true
					break
				}
			}
			if !foundRaw {
				return preparedTargetConfigPlan{}, csErr
			}
			continue
		}
		detachSetIDs[cs.ID] = true
	}

	now := time.Now()
	upsertedRefsByID := map[string]model.DeploymentRuntimeConfigRef{}
	var upsertOrder []string
	existingRefMap := map[string]model.DeploymentRuntimeConfigRef{}
	for _, cr := range currentConfigRefs {
		existingRefMap[cr.SetID] = cr
	}

	for _, ucs := range in.UpsertConfigSets {
		cs, csErr := resolveProjectConfigSet(db, project.ID, ucs.ConfigSet)
		if csErr != nil {
			return preparedTargetConfigPlan{}, csErr
		}
		if detachSetIDs[cs.ID] {
			return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("configSet %q cannot be both upserted and detached", cs.Name))
		}
		if _, dup := upsertedRefsByID[cs.ID]; dup {
			return preparedTargetConfigPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("duplicate configSet %q in upsertConfigSets", cs.Name))
		}
		prev, hadPrev := existingRefMap[cs.ID]
		modeStr := strings.TrimSpace(ucs.Mode)
		if modeStr == "" {
			if hadPrev {
				modeStr = prev.Mode
			} else {
				modeStr = model.RuntimeConfigRefModeLive
			}
		}
		mode := model.RuntimeConfigRefMode(modeStr)
		ref := model.DeploymentRuntimeConfigRef{SetID: cs.ID, Mode: mode}
		if mode == model.RuntimeConfigRefModeSnapshot {
			if hadPrev && prev.Snapshot != nil && !boolVal(ucs.RefreshSnapshot, false) {
				snap := *prev.Snapshot
				ref.Snapshot = &snap
			} else {
				snap := model.ProjectRuntimeConfigSetSnapshot(cs, now)
				ref.Snapshot = &snap
			}
		}
		upsertedRefsByID[cs.ID] = ref
		upsertOrder = append(upsertOrder, cs.ID)
		changes = append(changes, aitool.PreviewChange{Field: "configSet:" + cs.Name, After: mode, Action: "bind"})
	}

	var nextConfigRefs []model.DeploymentRuntimeConfigRef
	consumedRef := map[string]bool{}
	for _, cr := range currentConfigRefs {
		if detachSetIDs[cr.SetID] {
			changes = append(changes, aitool.PreviewChange{Field: "configSet:" + cr.SetID, Before: cr.Mode, Action: "detach"})
			continue
		}
		if up, ok := upsertedRefsByID[cr.SetID]; ok {
			nextConfigRefs = append(nextConfigRefs, up)
			consumedRef[cr.SetID] = true
		} else {
			nextConfigRefs = append(nextConfigRefs, cr)
		}
	}
	for _, idVal := range upsertOrder {
		if !consumedRef[idVal] {
			nextConfigRefs = append(nextConfigRefs, upsertedRefsByID[idVal])
		}
	}
	nextConfigRefs = model.NormalizeDeploymentRuntimeConfigRefs(nextConfigRefs)
	encodedConfigRefs := model.EncodeDeploymentRuntimeConfigRefs(nextConfigRefs)

	// 6. Validate effective files, ConfigSets, dataVolume mountPaths, and outbound ServiceBindings!
	mountsMap, mErr := h.deploymentTargetVolumeMountsByTarget(tc.Context(), []model.DeploymentTarget{existing})
	if mErr != nil {
		return preparedTargetConfigPlan{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", mErr.Error())
	}
	var volMountPaths []string
	for _, m := range mountsMap[existing.ID] {
		if m.ActivationState == model.DeploymentVolumeActivationReleasePending {
			continue
		}
		if mp := optionalStringValue(m.MountPath); mp != "" {
			volMountPaths = append(volMountPaths, mp)
		}
	}
	if err := validateTargetEffectiveFilesAndEnv(
		db,
		project.ID,
		existing.ID,
		nextPublicEnv,
		nextSecretKeySet,
		nextConfigFilePaths,
		nextSecretFilePaths,
		nextConfigRefs,
		volMountPaths,
	); err != nil {
		return preparedTargetConfigPlan{}, err
	}

	sort.Strings(sensitiveKeys)
	sensitiveKeys = slices.Compact(sensitiveKeys)

	return preparedTargetConfigPlan{
		user:                 user,
		project:              project,
		app:                  app,
		existing:             existing,
		nextPublicEnv:        nextPublicEnv,
		encodedEnvVars:       encodedEnvVars,
		secretMutation:       secretMut,
		nextConfigFiles:      nextConfigFiles,
		encodedConfigFiles:   encodedConfigFiles,
		secretFilesSetMap:    secretFilesSetMap,
		secretFilesClearList: secretFilesClearList,
		nextConfigRefs:       nextConfigRefs,
		encodedConfigRefs:    encodedConfigRefs,
		changes:              changes,
		sensitiveKeys:        sensitiveKeys,
	}, nil
}

func (h *Handler) previewDeliveryConfigureTargetConfig(tc *aitool.ToolContext, in deliveryConfigureTargetConfigInput) (*aitool.PreviewResult, error) {
	plan, err := h.prepareTargetConfigPlan(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("增量更新部署目标 %s/%s 的运行配置与密钥", plan.app.Identifier, plan.existing.Stage),
			TargetResource: fmt.Sprintf("%s/%s", plan.app.Identifier, plan.existing.Stage),
			TargetID:       plan.existing.ID,
			ProjectID:      plan.project.ID,
			Changes:        plan.changes,
			SensitiveKeys:  plan.sensitiveKeys,
			Warnings:       []string{"Configuration and secret changes take effect on Kubernetes Pods after calling `delivery_deploy_target`."},
		},
	}, nil
}

func (h *Handler) executeDeliveryConfigureTargetConfig(tc *aitool.ToolContext, in deliveryConfigureTargetConfigInput) (deliveryConfigureTargetConfigOutput, error) {
	plan, err := h.prepareTargetConfigPlan(tc, in)
	if err != nil {
		return deliveryConfigureTargetConfigOutput{}, err
	}
	ctx := tc.Context()
	db := h.aiDB(tc)
	var updated model.DeploymentTarget

	txErr := db.Transaction(func(tx *gorm.DB) error {
		var locked model.DeploymentTarget
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&locked, "id = ? AND project_id = ?", plan.existing.ID, plan.project.ID).Error; err != nil {
			return err
		}
		if !h.host.ResourceCanMutateDuringDelete(locked.DeleteStatus) {
			return aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is in progress")
		}

		// Handle secret files atomically with StoreContextWithDB + DeleteRefContextWithDB
		prevSecretFiles := decodeSecretRefs(locked.SecretFiles)
		nextSecretFiles := make(map[string]string, len(prevSecretFiles)+len(plan.secretFilesSetMap))
		for p, ref := range prevSecretFiles {
			if strings.TrimSpace(p) != "" && strings.TrimSpace(ref) != "" {
				nextSecretFiles[path.Clean(strings.TrimSpace(p))] = strings.TrimSpace(ref)
			}
		}
		for _, clearPath := range plan.secretFilesClearList {
			delete(nextSecretFiles, clearPath)
		}
		for filePath, content := range plan.secretFilesSetMap {
			ref, sErr := h.secrets.StoreContextWithDB(ctx, tx, content, plan.user.ID, "runtime_config:"+locked.ID+":file:"+filePath)
			if sErr != nil {
				return sErr
			}
			nextSecretFiles[filePath] = ref
		}

		// Delete superseded or cleared secret file refs
		retainedRefs := make(map[string]struct{}, len(nextSecretFiles))
		for _, ref := range nextSecretFiles {
			retainedRefs[ref] = struct{}{}
		}
		for p, oldRef := range prevSecretFiles {
			oldRef = strings.TrimSpace(oldRef)
			if oldRef == "" {
				continue
			}
			if _, keep := retainedRefs[oldRef]; !keep {
				if dErr := h.secrets.DeleteRefContextWithDB(ctx, tx, oldRef, "runtime_config:"+locked.ID+":file:"+path.Clean(strings.TrimSpace(p))); dErr != nil {
					return dErr
				}
			}
		}

		encodedSecretFiles := encodeStringMap(nextSecretFiles)

		// Re-validate under lock
		var mounts []model.DeploymentVolumeMount
		if err := tx.Where("project_id = ? AND deployment_target_id = ? AND activation_state <> ?",
			plan.project.ID, locked.ID, model.DeploymentVolumeActivationReleasePending).Find(&mounts).Error; err != nil {
			return err
		}
		var volPaths []string
		for _, m := range mounts {
			if mp := optionalStringValue(m.MountPath); mp != "" {
				volPaths = append(volPaths, mp)
			}
		}

		locked.EnvVars = plan.encodedEnvVars
		locked.ConfigFiles = plan.encodedConfigFiles
		locked.SecretFiles = encodedSecretFiles
		locked.RuntimeConfigRefs = plan.encodedConfigRefs
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}

		// Apply runtime env secret mutations & enforce public/secret key non-intersection
		if err := h.applyRuntimeSecretsInTx(ctx, tx, plan.user, &locked, plan.secretMutation); err != nil {
			return err
		}

		nextSecretKeySet := map[string]struct{}{}
		for _, k := range runtimeSecretKeys(locked.SecretRefs) {
			nextSecretKeySet[k] = struct{}{}
		}
		cfgPaths := make([]string, 0, len(plan.nextConfigFiles))
		for _, f := range plan.nextConfigFiles {
			cfgPaths = append(cfgPaths, f.Path)
		}
		if err := validateTargetEffectiveFilesAndEnv(
			tx,
			plan.project.ID,
			locked.ID,
			plan.nextPublicEnv,
			nextSecretKeySet,
			cfgPaths,
			sortedSecretFilePaths(locked.SecretFiles),
			plan.nextConfigRefs,
			volPaths,
		); err != nil {
			return err
		}

		updated = locked
		return nil
	})
	if txErr != nil {
		if te, ok := aitool.AsToolError(txErr); ok {
			return deliveryConfigureTargetConfigOutput{}, te
		}
		var secretErr *deploymentTargetSecretError
		if errors.As(txErr, &secretErr) && runtimeapi.IsRuntimeEnvironmentKeyConflict(secretErr.err) {
			return deliveryConfigureTargetConfigOutput{}, aitool.Conflict("deployment.runtime_environment_key_conflict", "public environment variable conflicts with an encrypted secret key")
		}
		return deliveryConfigureTargetConfigOutput{}, aitool.NewError(http.StatusBadRequest, "deployment.runtime_config_update_failed", txErr.Error())
	}

	if len(plan.secretFilesSetMap) > 0 || len(plan.secretFilesClearList) > 0 {
		h.auditWithContext(plan.user.ID, "deployment_target.secret_files_update", updated.ID, true, updated.Stage, ctx)
	}
	h.auditWithContext(plan.user.ID, "deployment_target.config_update", updated.ID, true, updated.Stage, ctx)

	outConfigSets := make([]aiTargetConfigSetRefView, 0, len(plan.nextConfigRefs))
	for _, r := range plan.nextConfigRefs {
		item := aiTargetConfigSetRefView{SetID: r.SetID, Mode: model.RuntimeConfigRefMode(r.Mode)}
		if r.Snapshot != nil {
			item.Name = r.Snapshot.Name
			captured := r.Snapshot.CapturedAt
			item.SnapshotCapturedAt = &captured
		}
		outConfigSets = append(outConfigSets, item)
	}

	secretKeys := runtimeSecretKeys(updated.SecretRefs)
	if secretKeys == nil {
		secretKeys = []string{}
	}
	secretFilePaths := sortedSecretFilePaths(updated.SecretFiles)
	if secretFilePaths == nil {
		secretFilePaths = []string{}
	}

	return deliveryConfigureTargetConfigOutput{
		ID:                 updated.ID,
		DeploymentTargetID: updated.ID,
		ProjectID:          plan.project.ID,
		ApplicationID:      plan.app.ID,
		Stage:              updated.Stage,
		PublicEnv:          plan.nextPublicEnv,
		SecretKeys:         secretKeys,
		ConfigFiles:        plan.nextConfigFiles,
		SecretFilePaths:    secretFilePaths,
		ConfigSets:         outConfigSets,
		RequiresRelease:    true,
	}, nil
}

// ============================================================================
// Tool 7 Implementation: delivery_configure_target_volumes
// ============================================================================

type preparedVolumePatchPlan struct {
	user               model.User
	project            model.Project
	app                model.Application
	target             model.DeploymentTarget
	effectiveClusterID string
	upserts            []deploymentTargetDataVolumeInput
	removeMountIDs     []string
	changes            []aitool.PreviewChange
}

func (h *Handler) prepareVolumePatchPlan(tc *aitool.ToolContext, in deliveryConfigureTargetVolumesInput) (preparedVolumePatchPlan, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentUpdate)
	if err != nil {
		return preparedVolumePatchPlan{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return preparedVolumePatchPlan{}, err
	}
	allowedVol, vAuthErr := h.projectRoleActionAllowed(tc.Context(), user, project.ID, authz.ActionVolumeWrite)
	if vAuthErr != nil || !allowedVol {
		return preparedVolumePatchPlan{}, aitool.Forbidden("project.access_denied", "volume:write permission is required")
	}

	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return preparedVolumePatchPlan{}, err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return preparedVolumePatchPlan{}, err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return preparedVolumePatchPlan{}, err
	}
	if err := h.ensureAIDeploymentTargetCanMutate(tc, target); err != nil {
		return preparedVolumePatchPlan{}, err
	}
	if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
		return preparedVolumePatchPlan{}, err
	}
	if len(in.UpsertMounts) == 0 && len(in.RemoveMounts) == 0 {
		return preparedVolumePatchPlan{}, aitool.BadRequest("argument_invalid", "specify at least one item in upsertMounts or removeMounts")
	}

	// Resolve effective cluster ID (backfilling historical empty target.ClusterID)
	effectiveClusterID := strings.TrimSpace(target.ClusterID)
	if effectiveClusterID == "" {
		resolvedCluster, cErr := runtimecluster.ResolveForTarget(db, "")
		if cErr != nil {
			return preparedVolumePatchPlan{}, aitool.BadRequest("runtime_cluster.not_configured", "no active runtime cluster is configured for this deployment target")
		}
		effectiveClusterID = resolvedCluster.ID
	}

	volSvc := volume.NewGormService(db)
	existingMounts, err := volSvc.ListDeploymentTargetMounts(tc.Context(), project.ID, target.ID)
	if err != nil {
		return preparedVolumePatchPlan{}, mapVolumeDomainError(err)
	}

	var changes []aitool.PreviewChange
	removeIDSet := map[string]bool{}
	var removeMountIDs []string
	for _, rawRem := range in.RemoveMounts {
		rem := strings.TrimSpace(rawRem)
		if rem == "" {
			continue
		}
		cleanedPath := ""
		if strings.HasPrefix(rem, "/") {
			cleanedPath = path.Clean(rem)
		}
		var matched *model.DeploymentVolumeMount
		for i := range existingMounts {
			m := &existingMounts[i]
			if m.ActivationState == model.DeploymentVolumeActivationReleasePending {
				continue
			}
			if m.LogicalName == rem ||
				(cleanedPath != "" && (optionalStringValue(m.MountPath) == cleanedPath || optionalStringValue(m.DevicePath) == cleanedPath)) {
				matched = m
				break
			}
		}
		if matched == nil {
			return preparedVolumePatchPlan{}, aitool.NotFound("volume.mount_not_found", fmt.Sprintf("volume mount %q was not found on this deployment target", rem))
		}
		if !removeIDSet[matched.ID] {
			removeIDSet[matched.ID] = true
			removeMountIDs = append(removeMountIDs, matched.ID)
			changes = append(changes, aitool.PreviewChange{
				Field:  "volumeMount:" + matched.LogicalName,
				Before: firstNonEmpty(optionalStringValue(matched.MountPath), optionalStringValue(matched.DevicePath)),
				Action: "unbind",
			})
		}
	}

	// Build normalized upsert inputs, inheriting existing logicalName by path if omitted!
	rawUpserts := make([]deploymentTargetDataVolumeInput, 0, len(in.UpsertMounts))
	for idx, item := range in.UpsertMounts {
		st := strings.TrimSpace(item.SourceType)
		if st != "projectVolume" && st != "emptyDir" {
			return preparedVolumePatchPlan{}, aitool.BadRequest("volume.invalid_input", "sourceType must be projectVolume or emptyDir")
		}
		rawLogical := strings.TrimSpace(item.LogicalName)
		if rawLogical != "" {
			if len(rawLogical) > 63 || !aiDNSLabelPattern.MatchString(rawLogical) {
				return preparedVolumePatchPlan{}, aitool.BadRequest("volume.invalid_input", fmt.Sprintf("logicalName %q must be a lowercase DNS label (^[a-z0-9]([-a-z0-9]*[a-z0-9])?$)", rawLogical))
			}
		}

		scratchCtx, rec := newScratchGin(tc)
		mountPath := strings.TrimSpace(item.MountPath)
		devicePath := strings.TrimSpace(item.DevicePath)
		var pvID string

		if st == "projectVolume" {
			vol, vErr := resolveProjectVolume(db, project.ID, item.ProjectVolume)
			if vErr != nil {
				return preparedVolumePatchPlan{}, vErr
			}
			if !volume.CanAttachProjectVolume(vol) {
				return preparedVolumePatchPlan{}, aitool.Conflict("volume.state_conflict", fmt.Sprintf("projectVolume %s (%s) is not attachable", vol.ID, vol.LifecycleState))
			}
			if strings.TrimSpace(vol.ClusterID) != effectiveClusterID {
				return preparedVolumePatchPlan{}, aitool.Conflict("volume.incompatible_cluster", fmt.Sprintf("projectVolume %s belongs to cluster %s, but target uses cluster %s", vol.ID, vol.ClusterID, effectiveClusterID))
			}
			if strings.EqualFold(strings.TrimSpace(vol.VolumeMode), string(corev1.PersistentVolumeBlock)) {
				if devicePath == "" || mountPath != "" {
					return preparedVolumePatchPlan{}, aitool.BadRequest("volume.invalid_input", fmt.Sprintf("Block volume %s requires devicePath and forbids mountPath", vol.DisplayName))
				}
				var ok bool
				devicePath, ok = normalizeDataDevicePath(scratchCtx, devicePath)
				if !ok {
					return preparedVolumePatchPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "volume.invalid_input", "invalid devicePath")
				}
			} else {
				if mountPath == "" || devicePath != "" {
					return preparedVolumePatchPlan{}, aitool.BadRequest("volume.invalid_input", fmt.Sprintf("Filesystem volume %s requires mountPath and forbids devicePath", vol.DisplayName))
				}
				var ok bool
				mountPath, ok = normalizeDataMountPath(scratchCtx, mountPath)
				if !ok {
					return preparedVolumePatchPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "volume.invalid_input", "invalid mountPath")
				}
			}
			if strings.TrimSpace(item.EmptyDirMedium) != "" || strings.TrimSpace(item.EmptyDirSizeLimit) != "" {
				return preparedVolumePatchPlan{}, aitool.BadRequest("volume.invalid_input", "projectVolume mount cannot set emptyDirMedium or emptyDirSizeLimit")
			}
			pvID = vol.ID
		} else {
			if strings.TrimSpace(item.ProjectVolume) != "" || devicePath != "" || boolVal(item.ReadOnly, false) {
				return preparedVolumePatchPlan{}, aitool.BadRequest("volume.invalid_input", "emptyDir mount requires mountPath and forbids projectVolume, devicePath, and readOnly=true")
			}
			var ok bool
			mountPath, ok = normalizeDataMountPath(scratchCtx, mountPath)
			if !ok {
				return preparedVolumePatchPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "volume.invalid_input", "invalid mountPath")
			}
			med := strings.TrimSpace(item.EmptyDirMedium)
			if med != "" && med != string(corev1.StorageMediumMemory) {
				return preparedVolumePatchPlan{}, aitool.BadRequest("volume.invalid_input", "emptyDirMedium must be empty or Memory")
			}
			if lim := strings.TrimSpace(item.EmptyDirSizeLimit); lim != "" {
				q, qErr := resource.ParseQuantity(lim)
				if qErr != nil || q.Sign() <= 0 {
					return preparedVolumePatchPlan{}, aitool.BadRequest("volume.invalid_input", "emptyDirSizeLimit must be a positive Kubernetes quantity")
				}
			}
		}

		// Inherit existing logicalName by normalized mountPath/devicePath when omitted!
		if rawLogical == "" {
			for _, em := range existingMounts {
				if removeIDSet[em.ID] {
					continue
				}
				if (mountPath != "" && optionalStringValue(em.MountPath) == mountPath) ||
					(devicePath != "" && optionalStringValue(em.DevicePath) == devicePath) {
					rawLogical = em.LogicalName
					break
				}
			}
			if rawLogical == "" {
				rawLogical = normalizeDataVolumeName("", firstNonEmpty(mountPath, devicePath), len(existingMounts)+idx)
			}
		}

		var emptyDir *deploymentTargetEmptyDirInput
		if st == "emptyDir" {
			emptyDir = &deploymentTargetEmptyDirInput{
				Medium:    normalizeEmptyDirMedium(item.EmptyDirMedium),
				SizeLimit: strings.TrimSpace(item.EmptyDirSizeLimit),
			}
		}
		dv := deploymentTargetDataVolumeInput{
			LogicalName:     rawLogical,
			SourceType:      st,
			ProjectVolumeID: pvID,
			MountPath:       mountPath,
			DevicePath:      devicePath,
			ReadOnly:        boolVal(item.ReadOnly, false),
			EmptyDir:        emptyDir,
		}
		rawUpserts = append(rawUpserts, dv)
		changes = append(changes, aitool.PreviewChange{
			Field:  "volumeMount:" + rawLogical,
			After:  fmt.Sprintf("%s -> %s", firstNonEmpty(pvID, "emptyDir"), firstNonEmpty(mountPath, devicePath)),
			Action: "bind",
		})
	}

	// Build combined desired mounts (surviving existing + upserts) to validate pairwise conflicts & active volume replacement
	var resultingMountPaths []string
	seenLogical := map[string]bool{}
	seenDevice := map[string]bool{}

	for _, em := range existingMounts {
		if removeIDSet[em.ID] || em.ActivationState == model.DeploymentVolumeActivationReleasePending {
			continue
		}
		overriddenByUpsert := false
		for _, up := range rawUpserts {
			if up.LogicalName == em.LogicalName ||
				(up.MountPath != "" && optionalStringValue(em.MountPath) == up.MountPath) ||
				(up.DevicePath != "" && optionalStringValue(em.DevicePath) == up.DevicePath) {
				// Check if identical; if not identical, reject replacing an active/reserved mount without unbinding first!
				wanted := volumeReserveInputFromDTO(target, up)
				if !deploymentVolumeMountMatches(em, wanted) {
					return preparedVolumePatchPlan{}, aitool.NewErrorWithHint(
						http.StatusConflict,
						volume.CodeBindingConflict,
						fmt.Sprintf("existing volume mount %q (%s) cannot be replaced in-place while %s", em.LogicalName, firstNonEmpty(optionalStringValue(em.MountPath), optionalStringValue(em.DevicePath)), em.ActivationState),
						"First remove the existing mount via `removeMounts` (and call `delivery_deploy_target` if active to detach the PVC), then bind the new volume.",
					)
				}
				overriddenByUpsert = true
				break
			}
		}
		if overriddenByUpsert {
			continue
		}
		seenLogical[em.LogicalName] = true
		if mp := optionalStringValue(em.MountPath); mp != "" {
			resultingMountPaths = append(resultingMountPaths, mp)
		}
		if dp := optionalStringValue(em.DevicePath); dp != "" {
			seenDevice[dp] = true
		}
	}

	for _, up := range rawUpserts {
		if seenLogical[up.LogicalName] {
			return preparedVolumePatchPlan{}, aitool.Conflict(volume.CodeBindingConflict, fmt.Sprintf("duplicate volume logicalName %q", up.LogicalName))
		}
		seenLogical[up.LogicalName] = true
		if up.MountPath != "" {
			for _, cur := range resultingMountPaths {
				if pathsOverlapOrNest(up.MountPath, cur) {
					return preparedVolumePatchPlan{}, aitool.Conflict(volume.CodeBindingConflict, fmt.Sprintf("volume mountPath %q conflicts with %q", up.MountPath, cur))
				}
			}
			resultingMountPaths = append(resultingMountPaths, up.MountPath)
		} else if up.DevicePath != "" {
			if seenDevice[up.DevicePath] {
				return preparedVolumePatchPlan{}, aitool.Conflict(volume.CodeBindingConflict, fmt.Sprintf("duplicate volume devicePath %q", up.DevicePath))
			}
			seenDevice[up.DevicePath] = true
		}
	}

	if len(seenLogical) > maxDeploymentDataVolumes {
		return preparedVolumePatchPlan{}, aitool.BadRequest(volume.CodeInvalidInput, "deployment dataVolumes exceeds the maximum item count (32)")
	}

	// Symmetric path conflict check against target & enabled ConfigSet configFiles + secretFiles!
	pubEnv := runtimeConfigMap(target.EnvVars)
	secKeySet := map[string]struct{}{}
	for _, k := range runtimeSecretKeys(target.SecretRefs) {
		secKeySet[k] = struct{}{}
	}
	var cfgPaths []string
	for _, f := range decodeConfigFilesList(target.ConfigFiles) {
		cfgPaths = append(cfgPaths, f.Path)
	}
	if err := validateTargetEffectiveFilesAndEnv(
		db,
		project.ID,
		target.ID,
		pubEnv,
		secKeySet,
		cfgPaths,
		sortedSecretFilePaths(target.SecretFiles),
		model.DecodeDeploymentRuntimeConfigRefs(target.RuntimeConfigRefs),
		resultingMountPaths,
	); err != nil {
		return preparedVolumePatchPlan{}, err
	}

	return preparedVolumePatchPlan{
		user:               user,
		project:            project,
		app:                app,
		target:             target,
		effectiveClusterID: effectiveClusterID,
		upserts:            rawUpserts,
		removeMountIDs:     removeMountIDs,
		changes:            changes,
	}, nil
}

func volumeReserveInputFromDTO(target model.DeploymentTarget, input deploymentTargetDataVolumeInput) volume.ReserveMountInput {
	if input.SourceType == "projectVolume" {
		return volume.ReserveMountInput{
			ProjectID:          target.ProjectID,
			ApplicationID:      target.ApplicationID,
			DeploymentTargetID: target.ID,
			SourceType:         model.DeploymentVolumeSourceProjectVolume,
			ProjectVolumeID:    strings.TrimSpace(input.ProjectVolumeID),
			LogicalName:        input.LogicalName,
			MountPath:          input.MountPath,
			DevicePath:         input.DevicePath,
			ReadOnly:           input.ReadOnly,
		}
	}
	ed := deploymentTargetEmptyDirInput{}
	if input.EmptyDir != nil {
		ed = *input.EmptyDir
	}
	return volume.ReserveMountInput{
		ProjectID:          target.ProjectID,
		ApplicationID:      target.ApplicationID,
		DeploymentTargetID: target.ID,
		SourceType:         model.DeploymentVolumeSourceEmptyDir,
		LogicalName:        input.LogicalName,
		MountPath:          input.MountPath,
		EmptyDirMedium:     ed.Medium,
		EmptyDirSizeLimit:  ed.SizeLimit,
	}
}

func mapVolumeDomainError(err error) *aitool.ToolError {
	if te, ok := aitool.AsToolError(err); ok {
		return te
	}
	code := volume.ErrorCode(err)
	switch code {
	case volume.CodeNotFound:
		return aitool.NotFound(code, err.Error())
	case volume.CodeBindingConflict, volume.CodeStateConflict, volume.CodeIncompatibleCluster, volume.CodeInUse, volume.CodeNameConflict, volume.CodeClaimConflict, volume.CodeRevisionConflict:
		return aitool.Conflict(code, err.Error())
	case "":
		return aitool.BadRequest("volume.operation_failed", err.Error())
	default:
		return aitool.BadRequest(code, err.Error())
	}
}

func (h *Handler) previewDeliveryConfigureTargetVolumes(tc *aitool.ToolContext, in deliveryConfigureTargetVolumesInput) (*aitool.PreviewResult, error) {
	plan, err := h.prepareVolumePatchPlan(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("增量更新部署目标 %s/%s 的数据卷挂载", plan.app.Identifier, plan.target.Stage),
			TargetResource: fmt.Sprintf("%s/%s", plan.app.Identifier, plan.target.Stage),
			TargetID:       plan.target.ID,
			ProjectID:      plan.project.ID,
			Changes:        plan.changes,
			Warnings:       []string{"Volume mount changes take effect on live Pods after calling `delivery_deploy_target`."},
		},
	}, nil
}

func (h *Handler) executeDeliveryConfigureTargetVolumes(tc *aitool.ToolContext, in deliveryConfigureTargetVolumesInput) (deliveryConfigureTargetVolumesOutput, error) {
	plan, err := h.prepareVolumePatchPlan(tc, in)
	if err != nil {
		return deliveryConfigureTargetVolumesOutput{}, err
	}
	ctx := tc.Context()
	db := h.aiDB(tc)
	changes := deploymentVolumeMountChanges{}
	requiresRelease := false

	txErr := db.Transaction(func(tx *gorm.DB) error {
		volRepo := volume.NewGormRepository(tx)
		lockedTarget, lockErr := volRepo.LockDeploymentTarget(ctx, plan.project.ID, plan.target.ID)
		if lockErr != nil {
			return lockErr
		}
		if !h.host.ResourceCanMutateDuringDelete(lockedTarget.DeleteStatus) {
			return aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is in progress")
		}

		// Backfill empty ClusterID before ReserveDeploymentVolumeMount
		if strings.TrimSpace(lockedTarget.ClusterID) == "" {
			if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", lockedTarget.ID).Update("cluster_id", plan.effectiveClusterID).Error; err != nil {
				return err
			}
			lockedTarget.ClusterID = plan.effectiveClusterID
		}

		var releaseCount int64
		if err := tx.Model(&model.Release{}).
			Where("project_id = ? AND application_id = ? AND deployment_target_id = ?", plan.project.ID, plan.app.ID, lockedTarget.ID).
			Count(&releaseCount).Error; err != nil {
			return err
		}

		volSvc := volume.NewGormService(tx)
		existingMounts, listErr := volSvc.ListDeploymentTargetMounts(ctx, plan.project.ID, lockedTarget.ID)
		if listErr != nil {
			return listErr
		}
		byID := make(map[string]model.DeploymentVolumeMount, len(existingMounts))
		for _, m := range existingMounts {
			byID[m.ID] = m
		}

		// 1. Process removals with RWO exclusive lock safety!
		for _, remID := range plan.removeMountIDs {
			cur, exists := byID[remID]
			if !exists || cur.ActivationState == model.DeploymentVolumeActivationReleasePending {
				continue
			}
			prevState := cur.ActivationState
			changes.Attempted = append(changes.Attempted, deploymentVolumeAuditRecordForMount("deployment_volume.unbind", lockedTarget, cur))
			unbound, uErr := volSvc.BeginDeploymentVolumeUnbind(ctx, plan.project.ID, cur.ID)
			if uErr != nil {
				return uErr
			}
			if prevState == model.DeploymentVolumeActivationReserved && releaseCount == 0 {
				// Target has never had any Release, so no Pod could ever hold this RWO PVC — safe to release immediately!
				if cErr := volSvc.CompleteDeploymentVolumeUnbind(ctx, plan.project.ID, cur.ID); cErr != nil {
					return cErr
				}
			} else {
				requiresRelease = true
			}
			changes.Unbound = append(changes.Unbound, unbound)
		}

		// Refresh mounts after removals
		currentMounts, listErr := volSvc.ListDeploymentTargetMounts(ctx, plan.project.ID, lockedTarget.ID)
		if listErr != nil {
			return listErr
		}

		// 2. Process upserts
		for _, up := range plan.upserts {
			wanted := volumeReserveInputFromDTO(lockedTarget, up)
			matchedExisting := false
			for _, cur := range currentMounts {
				if cur.LogicalName != wanted.LogicalName {
					continue
				}
				if !deploymentVolumeMountMatches(cur, wanted) {
					changes.Attempted = append(changes.Attempted, deploymentVolumeAuditRecordForMount("deployment_volume.bind", lockedTarget, cur))
					return &volume.DomainError{Code: volume.CodeBindingConflict, Message: "an active deployment volume must be removed before reusing its logical name or path"}
				}
				if cur.ActivationState == model.DeploymentVolumeActivationReleasePending {
					changes.Attempted = append(changes.Attempted, deploymentVolumeAuditRecordForMount("deployment_volume.bind", lockedTarget, cur))
					restored, rErr := volSvc.RestoreReleasePendingDeploymentVolumeMount(ctx, plan.project.ID, cur.ID)
					if rErr != nil {
						return rErr
					}
					changes.Bound = append(changes.Bound, restored)
					requiresRelease = true
				} else if cur.ActivationState == model.DeploymentVolumeActivationReserved {
					requiresRelease = true
				}
				matchedExisting = true
				break
			}
			if !matchedExisting {
				changes.Attempted = append(changes.Attempted, deploymentVolumeAuditRecordForDesired("deployment_volume.bind", lockedTarget, wanted))
				reserved, resErr := volSvc.ReserveDeploymentVolumeMount(ctx, wanted)
				if resErr != nil {
					return resErr
				}
				changes.Bound = append(changes.Bound, reserved)
				requiresRelease = true
			}
		}
		return nil
	})
	if txErr != nil {
		h.auditDeploymentVolumeMountFailure(ctx, plan.user.ID, changes, txErr)
		return deliveryConfigureTargetVolumesOutput{}, mapVolumeDomainError(txErr)
	}

	h.auditDeploymentVolumeMountChanges(ctx, plan.user.ID, plan.target, changes)
	mountsMap, err := h.deploymentTargetVolumeMountsByTarget(ctx, []model.DeploymentTarget{plan.target})
	if err != nil {
		return deliveryConfigureTargetVolumesOutput{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	return deliveryConfigureTargetVolumesOutput{
		ID:                 plan.target.ID,
		DeploymentTargetID: plan.target.ID,
		ProjectID:          plan.project.ID,
		ApplicationID:      plan.app.ID,
		Stage:              plan.target.Stage,
		ClusterID:          plan.effectiveClusterID,
		Volumes:            deploymentTargetDataVolumeResponses(mountsMap[plan.target.ID]),
		RequiresRelease:    requiresRelease,
	}, nil
}

// ============================================================================
// Tool 8 Implementation: delivery_delete_target
// ============================================================================

func (h *Handler) previewDeliveryDeleteTarget(tc *aitool.ToolContext, in deliveryDeleteTargetInput) (*aitool.PreviewResult, error) {
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentDelete)
	if err != nil {
		return nil, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return nil, err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return nil, err
	}
	if !h.host.DeleteStatusCanStart(target.DeleteStatus) {
		return nil, aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is already in progress")
	}

	// Precheck incoming ServiceBindings so we never pop up a doomed approval card!
	if sbErr := applicationapi.EnsureNoIncomingServiceBindingsTx(db, project.ID, app.ID, target.ID); sbErr != nil {
		scratchCtx, rec := newScratchGin(tc)
		if applicationapi.WriteServiceBindingInUseError(scratchCtx, sbErr) {
			return nil, toolErrorFromRecorder(rec, http.StatusConflict, "service_binding_in_use", "deployment target is referenced by active inbound service bindings")
		}
		return nil, aitool.Conflict("service_binding_in_use", sbErr.Error())
	}

	var routes []model.GatewayRoute
	if err := db.Where("deployment_target_id = ? AND delete_status = ?", target.ID, "active").Order("created_at asc").Find(&routes).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	mountsMap, err := h.deploymentTargetVolumeMountsByTarget(tc.Context(), []model.DeploymentTarget{target})
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var impact []string
	impact = append(impact, fmt.Sprintf("Delete Kubernetes workload and runtime resources for %s/%s (%s).", app.Identifier, target.Stage, target.ID))
	if len(routes) > 0 {
		hosts := make([]string, 0, len(routes))
		for _, r := range routes {
			hosts = append(hosts, r.Host+r.Path)
		}
		impact = append(impact, fmt.Sprintf("Cascade-delete %d gateway route(s): %s", len(routes), strings.Join(hosts, ", ")))
	}
	var pvIDs []string
	for _, m := range mountsMap[target.ID] {
		if m.SourceType == model.DeploymentVolumeSourceProjectVolume && m.ProjectVolumeID != nil && *m.ProjectVolumeID != "" {
			pvIDs = append(pvIDs, *m.ProjectVolumeID)
		}
	}
	if len(pvIDs) > 0 {
		impact = append(impact, fmt.Sprintf("Unbind %d persistent ProjectVolume(s) while retaining underlying PVC data: %s", len(pvIDs), strings.Join(pvIDs, ", ")))
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("异步删除部署目标 %s/%s", app.Identifier, target.Stage),
			TargetResource: fmt.Sprintf("%s/%s", app.Identifier, target.Stage),
			TargetID:       target.ID,
			ProjectID:      project.ID,
			Changes:        []aitool.PreviewChange{{Field: "deleteStatus", Before: target.DeleteStatus, After: "deleting", Action: "delete"}},
			Impact:         impact,
		},
	}, nil
}

func (h *Handler) executeDeliveryDeleteTarget(tc *aitool.ToolContext, in deliveryDeleteTargetInput) (deliveryDeleteTargetOutput, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentDelete)
	if err != nil {
		return deliveryDeleteTargetOutput{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return deliveryDeleteTargetOutput{}, err
	}
	db := h.aiDB(tc)
	ctx := tc.Context()
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return deliveryDeleteTargetOutput{}, err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return deliveryDeleteTargetOutput{}, err
	}
	if !h.host.DeleteStatusCanStart(target.DeleteStatus) {
		return deliveryDeleteTargetOutput{}, aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is already in progress")
	}

	volumeChanges := deploymentVolumeMountChanges{}
	txErr := db.Transaction(func(tx *gorm.DB) error {
		var locked model.DeploymentTarget
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Select("id").
			First(&locked, "id = ? AND project_id = ?", target.ID, target.ProjectID).Error; err != nil {
			return err
		}
		if err := applicationapi.EnsureNoIncomingServiceBindingsTx(tx, target.ProjectID, target.ApplicationID, target.ID); err != nil {
			return err
		}
		if err := h.host.MarkResourceDeleting(tx, &model.DeploymentTarget{}, target.ID); err != nil {
			return err
		}
		if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Update("delete_code", "").Error; err != nil {
			return err
		}
		if err := h.host.MarkDeploymentTargetGatewayRoutesDeleting(tx, target); err != nil {
			return err
		}
		var syncErr error
		volumeChanges, syncErr = syncDeploymentTargetVolumeMounts(ctx, tx, target, nil)
		return syncErr
	})
	if txErr != nil {
		scratchCtx, rec := newScratchGin(tc)
		if applicationapi.WriteServiceBindingInUseError(scratchCtx, txErr) {
			return deliveryDeleteTargetOutput{}, toolErrorFromRecorder(rec, http.StatusConflict, "service_binding_in_use", "deployment target is referenced by active inbound service bindings")
		}
		if h.host.IsResourceDeleteAlreadyStarted(txErr) {
			return deliveryDeleteTargetOutput{}, aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is already in progress")
		}
		h.auditDeploymentVolumeMountFailure(ctx, user.ID, volumeChanges, txErr)
		if volume.ErrorCode(txErr) != "" {
			return deliveryDeleteTargetOutput{}, mapVolumeDomainError(txErr)
		}
		return deliveryDeleteTargetOutput{}, aitool.NewError(http.StatusInternalServerError, "deployment_target.delete_failed", txErr.Error())
	}

	h.auditDeploymentVolumeMountChanges(ctx, user.ID, target, volumeChanges)
	if !h.enqueueResourceCleanup(ctx, "deployment_target", target.ID, target.ProjectID, user.ID) {
		const message = "deployment target cleanup task enqueue failed"
		_ = h.host.MarkResourceDeleteFailed(db, &model.DeploymentTarget{}, target.ID, message)
		_ = db.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Update("delete_code", deploymentTargetDeleteEnqueueFailedCode).Error
		_ = h.host.MarkDeploymentTargetGatewayRoutesDeleteFailed(db, target, message)
		h.auditWithContext(user.ID, "deployment.delete", target.ID, false, "cleanup_enqueue_failed", ctx)
		return deliveryDeleteTargetOutput{}, aitool.NewError(http.StatusServiceUnavailable, deploymentTargetDeleteEnqueueFailedCode, message)
	}
	h.auditWithContext(user.ID, "deployment.delete", target.ID, true, "cleanup_queued", ctx)

	return deliveryDeleteTargetOutput{
		ID:                 target.ID,
		DeploymentTargetID: target.ID,
		ApplicationID:      app.ID,
		ProjectID:          project.ID,
		Stage:              target.Stage,
		DeleteStatus:       "deleting",
	}, nil
}
