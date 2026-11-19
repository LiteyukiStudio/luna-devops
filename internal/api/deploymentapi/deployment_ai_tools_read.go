package deploymentapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
)

type deliveryGetTargetInput struct {
	Project                string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application            string `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识（identifier）或唯一显示名称"`
	Target                 string `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、环境阶段（dev|test|staging|prod）或同应用唯一名称"`
	IncludeImageCandidates *bool  `json:"includeImageCandidates,omitempty" default:"false" desc:"是否按需拉取最近成功构建及关联镜像仓候选 Tag（上限 30 条）"`
}

type aiTargetAutoScalingView struct {
	Enabled           bool `json:"enabled"`
	MinReplicas       int  `json:"minReplicas"`
	MaxReplicas       int  `json:"maxReplicas"`
	CPUPercent        int  `json:"cpuPercent"`
	MemoryPercent     int  `json:"memoryPercent"`
	HasCustomBehavior bool `json:"hasCustomBehavior"`
}

type aiTargetProbesView struct {
	Readiness *corev1.Probe `json:"readiness,omitempty"`
	Liveness  *corev1.Probe `json:"liveness,omitempty"`
	Startup   *corev1.Probe `json:"startup,omitempty"`
}

type aiTargetSecuritySchedulingView struct {
	RunAsUser              string            `json:"runAsUser,omitempty"`
	RunAsGroup             string            `json:"runAsGroup,omitempty"`
	FSGroup                string            `json:"fsGroup,omitempty"`
	FSGroupChangePolicy    string            `json:"fsGroupChangePolicy,omitempty"`
	ReadOnlyRootFilesystem bool              `json:"readOnlyRootFilesystem"`
	NodeSelector           map[string]string `json:"nodeSelector"`
}

type aiTargetGovernanceView struct {
	Enabled                    bool   `json:"enabled"`
	WebConsolePolicy           string `json:"webConsolePolicy"`
	EffectiveWebConsoleEnabled bool   `json:"effectiveWebConsoleEnabled"`
	RequireApproval            bool   `json:"requireApproval"`
}

type aiTargetAdvancedK8sSummaryView struct {
	HasLifecycle                 bool   `json:"hasLifecycle"`
	InitContainerCount           int    `json:"initContainerCount"`
	SidecarContainerCount        int    `json:"sidecarContainerCount"`
	HasTolerations               bool   `json:"hasTolerations"`
	HasAffinity                  bool   `json:"hasAffinity"`
	HasTopologySpreadConstraints bool   `json:"hasTopologySpreadConstraints"`
	PriorityClassName            string `json:"priorityClassName,omitempty"`
	ServiceSessionAffinity       string `json:"serviceSessionAffinity,omitempty"`
	HasServiceAnnotations        bool   `json:"hasServiceAnnotations"`
}

type aiTargetConfigSetRefView struct {
	SetID              string     `json:"setId"`
	Name               string     `json:"name,omitempty"`
	Mode               string     `json:"mode"`
	SnapshotCapturedAt *time.Time `json:"snapshotCapturedAt,omitempty"`
}

type aiTargetBuildPipelineView struct {
	SourceType                 string                              `json:"sourceType"`
	RepositoryBindingID        string                              `json:"repositoryBindingId,omitempty"`
	BuildDefinitionMode        string                              `json:"buildDefinitionMode"`
	BuildTemplateID            string                              `json:"buildTemplateId,omitempty"`
	BuildTemplateVersion       string                              `json:"buildTemplateVersion,omitempty"`
	BuildTemplateValues        map[string]string                   `json:"buildTemplateValues"`
	DockerfilePath             string                              `json:"dockerfilePath,omitempty"`
	BuildContext               string                              `json:"buildContext,omitempty"`
	BuildDirectory             string                              `json:"buildDirectory,omitempty"`
	BuildArgs                  map[string]string                   `json:"buildArgs"`
	BuildCPURequest            string                              `json:"buildCpuRequest"`
	BuildMemoryRequest         string                              `json:"buildMemoryRequest"`
	BuildTimeoutSeconds        int                                 `json:"buildTimeoutSeconds"`
	TargetRegistryID           string                              `json:"targetRegistryId,omitempty"`
	TargetRepository           string                              `json:"targetRepository,omitempty"`
	TargetTag                  string                              `json:"targetTag,omitempty"`
	BuildEnvironmentID         string                              `json:"buildEnvironmentId,omitempty"`
	BuildVariableSetIDs        []string                            `json:"buildVariableSetIds"`
	AutoDeploy                 bool                                `json:"autoDeploy"`
	RequireApproval            bool                                `json:"requireApproval"`
	BuildConcurrencyPolicy     string                              `json:"buildConcurrencyPolicy"`
	BranchPattern              string                              `json:"branchPattern,omitempty"`
	TagPattern                 string                              `json:"tagPattern,omitempty"`
	BuildHooksEnabled          bool                                `json:"buildHooksEnabled"`
	BuildHookBindings          []model.DeploymentTargetHookBinding `json:"buildHookBindings"`
	CanInspectBuildEnvironment bool                                `json:"canInspectBuildEnvironment"`
	BuildVariables             map[string]string                   `json:"buildVariables,omitempty"`
	BuildSecretKeys            []string                            `json:"buildSecretKeys,omitempty"`
}

type deliveryGetTargetOutput struct {
	ID                        string                               `json:"id"`
	DeploymentTargetID        string                               `json:"deploymentTargetId"`
	ProjectID                 string                               `json:"projectId"`
	ApplicationID             string                               `json:"applicationId"`
	Name                      string                               `json:"name"`
	Stage                     string                               `json:"stage"`
	KubernetesName            string                               `json:"kubernetesName"`
	ClusterID                 string                               `json:"clusterId"`
	EffectiveClusterID        string                               `json:"effectiveClusterId"`
	WorkloadType              string                               `json:"workloadType"`
	Replicas                  int                                  `json:"replicas"`
	CPURequest                string                               `json:"cpuRequest"`
	MemoryRequest             string                               `json:"memoryRequest"`
	ResourceLimitNote         string                               `json:"resourceLimitNote"`
	ImageRef                  string                               `json:"imageRef"`
	ImagePullPolicy           string                               `json:"imagePullPolicy,omitempty"`
	AutoScaling               aiTargetAutoScalingView              `json:"autoScaling"`
	ServicePorts              []model.DeploymentServicePort        `json:"servicePorts"`
	Command                   []string                             `json:"command"`
	Args                      []string                             `json:"args"`
	Probes                    aiTargetProbesView                   `json:"probes"`
	SecurityAndScheduling     aiTargetSecuritySchedulingView       `json:"securityAndScheduling"`
	InternalOnly              bool                                 `json:"internalOnly"`
	Governance                aiTargetGovernanceView               `json:"governance"`
	AdvancedKubernetesSummary aiTargetAdvancedK8sSummaryView       `json:"advancedKubernetesSummary"`
	PublicEnv                 map[string]string                    `json:"publicEnv"`
	SecretKeys                []string                             `json:"secretKeys"`
	ConfigFiles               []runtimeConfigFileInput             `json:"configFiles"`
	SecretFilePaths           []string                             `json:"secretFilePaths"`
	ConfigSets                []aiTargetConfigSetRefView           `json:"configSets"`
	Volumes                   []deploymentTargetDataVolumeResponse `json:"volumes"`
	BuildPipeline             aiTargetBuildPipelineView            `json:"buildPipeline"`
	Status                    string                               `json:"status"`
	ObservationCode           string                               `json:"observationCode,omitempty"`
	LastCheckedAt             *time.Time                           `json:"lastCheckedAt,omitempty"`
	DesiredReplicas           int32                                `json:"desiredReplicas"`
	UpdatedReplicas           int32                                `json:"updatedReplicas"`
	ReadyReplicas             int32                                `json:"readyReplicas"`
	AvailableReplicas         int32                                `json:"availableReplicas"`
	DeleteStatus              string                               `json:"deleteStatus"`
	DeleteCode                string                               `json:"deleteCode,omitempty"`
	DeleteMessage             string                               `json:"deleteMessage,omitempty"`
	CreatedBy                 string                               `json:"createdBy"`
	CreatedAt                 time.Time                            `json:"createdAt"`
	ImageCandidates           *releaseImageCandidatesOutput        `json:"imageCandidates,omitempty"`
}

type deliveryListReleasesInput struct {
	Project     string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application string `json:"application,omitempty" desc:"可选应用 ID、唯一标识或唯一名称"`
	Target      string `json:"target,omitempty" desc:"可选部署目标（传入 application 时支持 targetId|stage|name；省略 application 时支持 targetId 或公开 stage）"`
	Status      string `json:"status,omitempty" enum:"pending|running|succeeded|failed|canceled" desc:"可选按发布状态过滤"`
	SortBy      string `json:"sortBy,omitempty" enum:"createdAt|revision|status" default:"createdAt" desc:"排序字段"`
	SortOrder   string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page        *int   `json:"page,omitempty" min:"1" default:"1" desc:"页码（从 1 开始）"`
	PageSize    *int   `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（默认 20，最大 50）"`
}

type deliveryGetReleaseInput struct {
	Project      string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	ReleaseID    string `json:"releaseId" required:"true" desc:"发布记录 ID（rel_xxx）"`
	IncludeLogs  *bool  `json:"includeLogs,omitempty" default:"true" desc:"是否读取权威发布日志（含 pre/post Hook 与 K8s Rollout 进展）"`
	LogTailBytes *int   `json:"logTailBytes,omitempty" min:"256" max:"32768" default:"8192" desc:"日志尾部最大字节数（默认 8192，上限 32768，按 UTF-8 字符边界截断）"`
}

type deliveryGetReleaseOutput struct {
	ID                 string     `json:"id"`
	ReleaseID          string     `json:"releaseId"`
	ProjectID          string     `json:"projectId"`
	ApplicationID      string     `json:"applicationId"`
	DeploymentTargetID string     `json:"deploymentTargetId"`
	BuildRunID         string     `json:"buildRunId,omitempty"`
	ImageRef           string     `json:"imageRef"`
	ForceImagePull     bool       `json:"forceImagePull"`
	Type               string     `json:"type"`
	Status             string     `json:"status"`
	FailureCode        string     `json:"failureCode,omitempty"`
	Revision           int        `json:"revision"`
	RollbackFromID     string     `json:"rollbackFromId,omitempty"`
	Message            string     `json:"message,omitempty"`
	StartedAt          *time.Time `json:"startedAt,omitempty"`
	FinishedAt         *time.Time `json:"finishedAt,omitempty"`
	CreatedBy          string     `json:"createdBy"`
	CreatedAt          time.Time  `json:"createdAt"`
	Logs               string     `json:"logs,omitempty"`
	LogTruncated       bool       `json:"logTruncated,omitempty"`
	LogTruncatedBytes  int        `json:"logTruncatedBytes,omitempty"`
}

func (h *Handler) registerDeliveryReadAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[deliveryGetTargetInput, deliveryGetTargetOutput]{
		Name:           "delivery_get_target",
		Domain:         aitool.DomainDelivery,
		Title:          "查看部署目标完整配置与实时状态",
		Summary:        "查看单个部署目标的完整期望配置（运行时规格、环境变量/加密键名、挂载卷、构建流水线）及 K8s 实时副本状态。",
		AliasesZH:      []string{"查看部署目标", "部署配置详情", "环境配置详情"},
		AliasesEN:      []string{"get deployment target", "inspect target config"},
		Purpose:        aitool.Text("读取单个部署目标的完整运行时期望规格、公开配置、已配置密钥键名、数据卷挂载、构建流水线及集群实时副本状态。", "Inspect the full desired runtime spec, public config, secret key names, volume mounts, build pipeline, and live K8s replica observation of a deployment target."),
		ApprovalTier:   aitool.ApprovalTierL0,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentRead)},
		Effect:         aitool.EffectRead,
		Execute:        h.executeDeliveryGetTarget,
	})

	aitool.Register(reg, aitool.Spec[deliveryListReleasesInput, map[string]any]{
		Name:           "delivery_list_releases",
		Domain:         aitool.DomainDelivery,
		Title:          "分页查询发布历史记录",
		Summary:        "分页查询项目、应用或特定部署目标的发布历史记录，支持按状态过滤与排序。",
		AliasesZH:      []string{"发布列表", "发布历史", "部署记录"},
		AliasesEN:      []string{"list releases", "deployment history"},
		Purpose:        aitool.Text("分页查询项目下应用或特定部署环境的发布记录。", "List paginated release history for a project, application, or deployment target."),
		ApprovalTier:   aitool.ApprovalTierL0,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentRead)},
		Effect:         aitool.EffectRead,
		Execute:        h.executeDeliveryListReleases,
	})

	aitool.Register(reg, aitool.Spec[deliveryGetReleaseInput, deliveryGetReleaseOutput]{
		Name:           "delivery_get_release",
		Domain:         aitool.DomainDelivery,
		Title:          "查看单次发布详情与发布日志",
		Summary:        "查看单次发布记录详情及按 UTF-8 字符边界尾部截断的权威发布日志。",
		AliasesZH:      []string{"发布详情", "发布日志", "查看发布结果"},
		AliasesEN:      []string{"get release", "release logs"},
		Purpose:        aitool.Text("读取单次发布的状态、错误码、镜像版本及尾部截断的发布执行日志。", "Read release status, failure code, image reference, and UTF-8 tail-truncated release logs."),
		ApprovalTier:   aitool.ApprovalTierL0,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentRead)},
		Effect:         aitool.EffectRead,
		Execute:        h.executeDeliveryGetRelease,
	})
}

func (h *Handler) executeDeliveryGetTarget(tc *aitool.ToolContext, in deliveryGetTargetInput) (deliveryGetTargetOutput, error) {
	tc.SetNoStore()
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRead)
	if err != nil {
		return deliveryGetTargetOutput{}, err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return deliveryGetTargetOutput{}, err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return deliveryGetTargetOutput{}, err
	}
	return h.buildDeliveryTargetDetailOutput(tc, user, project, app, target, boolVal(in.IncludeImageCandidates, false))
}

func (h *Handler) buildDeliveryTargetDetailOutput(
	tc *aitool.ToolContext,
	user model.User,
	project model.Project,
	app model.Application,
	target model.DeploymentTarget,
	includeCandidates bool,
) (deliveryGetTargetOutput, error) {
	ctx := tc.Context()
	db := h.aiDB(tc)

	targetWithHooks, err := h.deploymentTargetWithHookBindings(target, ctx)
	if err != nil {
		return deliveryGetTargetOutput{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	mountsByTarget, err := h.deploymentTargetVolumeMountsByTarget(ctx, []model.DeploymentTarget{targetWithHooks})
	if err != nil {
		return deliveryGetTargetOutput{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	observed := h.observeDeploymentTarget(ctx, project, targetWithHooks)
	if observed.BuildHookBindings == nil {
		observed.BuildHookBindings = []model.DeploymentTargetHookBinding{}
	}

	publicEnv := runtimeConfigMap(observed.EnvVars)
	if publicEnv == nil {
		publicEnv = map[string]string{}
	}
	secretKeys := runtimeSecretKeys(observed.SecretRefs)
	if secretKeys == nil {
		secretKeys = []string{}
	}
	configFiles := decodeConfigFilesList(observed.ConfigFiles)
	secretFilePaths := sortedSecretFilePaths(observed.SecretFiles)
	if secretFilePaths == nil {
		secretFilePaths = []string{}
	}

	rawRefs := model.DecodeDeploymentRuntimeConfigRefs(observed.RuntimeConfigRefs)
	setNames := map[string]string{}
	if len(rawRefs) > 0 {
		ids := make([]string, 0, len(rawRefs))
		for _, r := range rawRefs {
			ids = append(ids, r.SetID)
		}
		var sets []model.ProjectRuntimeConfigSet
		if qErr := db.Select("id", "name").Where("project_id = ? AND id IN ?", project.ID, ids).Find(&sets).Error; qErr == nil {
			for _, s := range sets {
				setNames[s.ID] = s.Name
			}
		}
	}
	configSets := make([]aiTargetConfigSetRefView, 0, len(rawRefs))
	for _, r := range rawRefs {
		item := aiTargetConfigSetRefView{
			SetID: r.SetID,
			Name:  setNames[r.SetID],
			Mode:  model.RuntimeConfigRefMode(r.Mode),
		}
		if r.Snapshot != nil {
			if item.Name == "" {
				item.Name = r.Snapshot.Name
			}
			captured := r.Snapshot.CapturedAt
			item.SnapshotCapturedAt = &captured
		}
		configSets = append(configSets, item)
	}

	webConsolePolicy := "inherit"
	if observed.WebConsoleEnabled != nil && !*observed.WebConsoleEnabled {
		webConsolePolicy = "disabled"
	}

	templateValues := map[string]string{}
	if strings.TrimSpace(observed.BuildTemplateValues) != "" {
		_ = json.Unmarshal([]byte(observed.BuildTemplateValues), &templateValues)
	}
	buildArgsMap := model.BuildArgs(observed.BuildArgs)
	if buildArgsMap == nil {
		buildArgsMap = map[string]string{}
	}
	varSetIDs := buildVariableSetIDs(observed.BuildVariableSetIDs)
	if varSetIDs == nil {
		varSetIDs = []string{}
	}

	// Gate deployment-scoped BuildEnvironmentConfig strictly behind ActionSecretViewValue.
	canInspectBuildEnv, _ := h.projectRoleActionAllowed(ctx, user, project.ID, authz.ActionSecretViewValue)
	var (
		buildVars       map[string]string
		buildSecretKeys []string
	)
	if canInspectBuildEnv {
		buildVars = map[string]string{}
		buildSecretKeys = []string{}
		if bec, becErr := h.findBuildEnvironmentConfig(db, model.BuildEnvironmentScopeDeployment, observed.ID); becErr == nil {
			if decoded := buildenv.Decode(bec.Variables); len(decoded) > 0 {
				buildVars = decoded
			}
			for k, ref := range buildenv.Decode(bec.SecretRefs) {
				if strings.TrimSpace(k) != "" && strings.TrimSpace(ref) != "" {
					buildSecretKeys = append(buildSecretKeys, strings.TrimSpace(k))
				}
			}
			sort.Strings(buildSecretKeys)
		}
	}

	var candidates *releaseImageCandidatesOutput
	if includeCandidates {
		cand := releaseImageCandidatesOutput{Items: []releaseImageCandidateOutput{}}
		seen := map[string]bool{}
		if normalizeDeploymentSourceType(observed.SourceType) == "repository" {
			scratchCtx, _ := newScratchGin(tc)
			h.appendRegistryReleaseImageCandidates(scratchCtx, user, observed, &cand, seen)
		}
		h.appendBuildRunReleaseImageCandidates(observed, &cand, seen, ctx)
		if normalizeDeploymentSourceType(observed.SourceType) == "image" && strings.TrimSpace(observed.ImageRef) != "" {
			appendReleaseImageCandidate(&cand.Items, seen, releaseImageCandidateOutput{
				Key:      "image:" + strings.TrimSpace(observed.ImageRef),
				Source:   "target",
				Label:    strings.TrimSpace(observed.ImageRef),
				ImageRef: strings.TrimSpace(observed.ImageRef),
			})
		}
		cand.FallbackUsed = !cand.RegistryAvailable && len(cand.Items) > 0
		candidates = &cand
	}

	return deliveryGetTargetOutput{
		ID:                 observed.ID,
		DeploymentTargetID: observed.ID,
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		Name:               observed.Name,
		Stage:              fallback(strings.TrimSpace(observed.Stage), model.DefaultDeploymentStage),
		KubernetesName:     strings.TrimSpace(observed.KubernetesName),
		ClusterID:          observed.ClusterID,
		EffectiveClusterID: observed.EffectiveClusterID,
		WorkloadType:       normalizeWorkloadType(observed.WorkloadType),
		Replicas:           observed.Replicas,
		CPURequest:         fallback(strings.TrimSpace(observed.CPURequest), model.DefaultDeploymentCPURequest),
		MemoryRequest:      fallback(strings.TrimSpace(observed.MemoryRequest), model.DefaultDeploymentMemoryRequest),
		ResourceLimitNote:  "Container CPU/memory limits are automatically derived by the bound runtime cluster's overcommit policy.",
		ImageRef:           observed.ImageRef,
		ImagePullPolicy:    normalizeImagePullPolicyValue(observed.ImagePullPolicy),
		AutoScaling: aiTargetAutoScalingView{
			Enabled:           observed.AutoScalingEnabled,
			MinReplicas:       fallbackInt(observed.AutoScalingMinReplicas, 1),
			MaxReplicas:       fallbackInt(observed.AutoScalingMaxReplicas, fallbackInt(observed.Replicas, 1)),
			CPUPercent:        observed.AutoScalingCPUPercent,
			MemoryPercent:     observed.AutoScalingMemoryPercent,
			HasCustomBehavior: strings.TrimSpace(observed.AutoScalingBehavior) != "",
		},
		ServicePorts: model.DeploymentTargetServicePorts(observed),
		Command:      decodeStringListField(observed.ContainerCommand),
		Args:         decodeStringListField(observed.ContainerArgs),
		Probes: aiTargetProbesView{
			Readiness: decodeOptionalProbe(observed.ReadinessProbe),
			Liveness:  decodeOptionalProbe(observed.LivenessProbe),
			Startup:   decodeOptionalProbe(observed.StartupProbe),
		},
		SecurityAndScheduling: aiTargetSecuritySchedulingView{
			RunAsUser:              strings.TrimSpace(observed.RunAsUser),
			RunAsGroup:             strings.TrimSpace(observed.RunAsGroup),
			FSGroup:                strings.TrimSpace(observed.FSGroup),
			FSGroupChangePolicy:    normalizeFSGroupChangePolicy(observed.FSGroupChangePolicy),
			ReadOnlyRootFilesystem: observed.ReadOnlyRootFilesystem,
			NodeSelector:           decodeStringMapField(observed.NodeSelector),
		},
		InternalOnly: observed.InternalOnly,
		Governance: aiTargetGovernanceView{
			Enabled:                    observed.Enabled,
			WebConsolePolicy:           webConsolePolicy,
			EffectiveWebConsoleEnabled: runtimeWebConsoleEnabled(project, observed),
			RequireApproval:            observed.RequireApproval,
		},
		AdvancedKubernetesSummary: aiTargetAdvancedK8sSummaryView{
			HasLifecycle:                 strings.TrimSpace(observed.Lifecycle) != "",
			InitContainerCount:           countContainersInJSON(observed.InitContainers),
			SidecarContainerCount:        countContainersInJSON(observed.SidecarContainers),
			HasTolerations:               strings.TrimSpace(observed.Tolerations) != "" && strings.TrimSpace(observed.Tolerations) != "[]",
			HasAffinity:                  strings.TrimSpace(observed.Affinity) != "" && strings.TrimSpace(observed.Affinity) != "{}",
			HasTopologySpreadConstraints: strings.TrimSpace(observed.TopologySpreadConstraints) != "" && strings.TrimSpace(observed.TopologySpreadConstraints) != "[]",
			PriorityClassName:            strings.TrimSpace(observed.PriorityClassName),
			ServiceSessionAffinity:       normalizeServiceSessionAffinity(observed.ServiceSessionAffinity),
			HasServiceAnnotations:        len(decodeStringMapField(observed.ServiceAnnotations)) > 0,
		},
		PublicEnv:       publicEnv,
		SecretKeys:      secretKeys,
		ConfigFiles:     configFiles,
		SecretFilePaths: secretFilePaths,
		ConfigSets:      configSets,
		Volumes:         deploymentTargetDataVolumeResponses(mountsByTarget[observed.ID]),
		BuildPipeline: aiTargetBuildPipelineView{
			SourceType:                 normalizeDeploymentSourceType(observed.SourceType),
			RepositoryBindingID:        observed.RepositoryBindingID,
			BuildDefinitionMode:        fallback(strings.TrimSpace(observed.BuildDefinitionMode), buildtemplate.DefinitionModeRepository),
			BuildTemplateID:            observed.BuildTemplateID,
			BuildTemplateVersion:       observed.BuildTemplateVersion,
			BuildTemplateValues:        templateValues,
			DockerfilePath:             observed.DockerfilePath,
			BuildContext:               observed.BuildContext,
			BuildDirectory:             observed.BuildDirectory,
			BuildArgs:                  buildArgsMap,
			BuildCPURequest:            fallback(strings.TrimSpace(observed.BuildCPURequest), defaultBuildCPURequest),
			BuildMemoryRequest:         fallback(strings.TrimSpace(observed.BuildMemoryRequest), defaultBuildMemoryRequest),
			BuildTimeoutSeconds:        normalizeBuildTimeoutSecondsValue(observed.BuildTimeoutSeconds),
			TargetRegistryID:           observed.TargetRegistryID,
			TargetRepository:           observed.TargetRepository,
			TargetTag:                  observed.TargetTag,
			BuildEnvironmentID:         strings.TrimSpace(observed.BuildEnvironmentID),
			BuildVariableSetIDs:        varSetIDs,
			AutoDeploy:                 observed.AutoDeploy,
			RequireApproval:            observed.RequireApproval,
			BuildConcurrencyPolicy:     fallback(strings.TrimSpace(observed.BuildConcurrencyPolicy), model.BuildConcurrencyQueue),
			BranchPattern:              observed.BranchPattern,
			TagPattern:                 observed.TagPattern,
			BuildHooksEnabled:          observed.BuildHooksEnabled,
			BuildHookBindings:          observed.BuildHookBindings,
			CanInspectBuildEnvironment: canInspectBuildEnv,
			BuildVariables:             buildVars,
			BuildSecretKeys:            buildSecretKeys,
		},
		Status:            observed.Status,
		ObservationCode:   observed.ObservationCode,
		LastCheckedAt:     observed.LastCheckedAt,
		DesiredReplicas:   observed.DesiredReplicas,
		UpdatedReplicas:   observed.UpdatedReplicas,
		ReadyReplicas:     observed.ReadyReplicas,
		AvailableReplicas: observed.AvailableReplicas,
		DeleteStatus:      observed.DeleteStatus,
		DeleteCode:        observed.DeleteCode,
		DeleteMessage:     observed.DeleteMessage,
		CreatedBy:         observed.CreatedBy,
		CreatedAt:         observed.CreatedAt,
		ImageCandidates:   candidates,
	}, nil
}

func decodeOptionalProbe(raw string) *corev1.Probe {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var p corev1.Probe
	if err := json.Unmarshal([]byte(trimmed), &p); err != nil {
		return nil
	}
	return &p
}

func countContainersInJSON(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[]" {
		return 0
	}
	var list []corev1.Container
	if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
		return 0
	}
	return len(list)
}

func (h *Handler) executeDeliveryListReleases(tc *aitool.ToolContext, in deliveryListReleasesInput) (map[string]any, error) {
	tc.SetNoStore()
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRead)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	sortBy := strings.TrimSpace(in.SortBy)
	if sortBy == "" {
		sortBy = "createdAt"
	}
	sortColumn, ok := map[string]string{
		"createdAt": "releases.created_at",
		"revision":  "releases.revision",
		"status":    "releases.status",
	}[sortBy]
	if !ok {
		return nil, aitool.BadRequest("argument_invalid", "sortBy must be createdAt, revision, or status")
	}
	sortOrder := strings.ToLower(strings.TrimSpace(in.SortOrder))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, aitool.BadRequest("argument_invalid", "sortOrder must be asc or desc")
	}

	query := db.Model(&model.Release{}).Where("releases.project_id = ?", project.ID)

	var (
		resolvedAppID    string
		resolvedTargetID string
	)
	if strings.TrimSpace(in.Application) != "" {
		app, appErr := aitool.ResolveApplication(db, project.ID, in.Application)
		if appErr != nil {
			return nil, appErr
		}
		resolvedAppID = app.ID
		query = query.Where("releases.application_id = ?", app.ID)
		if strings.TrimSpace(in.Target) != "" {
			target, tErr := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
			if tErr != nil {
				return nil, tErr
			}
			resolvedTargetID = target.ID
			query = query.Where("releases.deployment_target_id = ?", target.ID)
		}
	} else if targetRef := strings.TrimSpace(in.Target); targetRef != "" {
		var byID model.DeploymentTarget
		if idErr := db.Where("project_id = ? AND id = ? AND deleted_at IS NULL", project.ID, targetRef).First(&byID).Error; idErr == nil {
			resolvedTargetID = byID.ID
			query = query.Where("releases.deployment_target_id = ?", byID.ID)
		} else if !errors.Is(idErr, gorm.ErrRecordNotFound) {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", idErr.Error())
		} else if stage, validStage := normalizePublicStage(targetRef); validStage {
			query = query.Joins("JOIN deployment_targets ON deployment_targets.id = releases.deployment_target_id AND deployment_targets.deleted_at IS NULL").
				Where("deployment_targets.stage = ?", stage).
				Select("releases.*")
		} else {
			var byName []model.DeploymentTarget
			if nameErr := db.Where("project_id = ? AND name = ? AND deleted_at IS NULL", project.ID, targetRef).
				Order("created_at asc").Find(&byName).Error; nameErr != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", nameErr.Error())
			}
			if len(byName) == 1 {
				resolvedTargetID = byName[0].ID
				query = query.Where("releases.deployment_target_id = ?", byName[0].ID)
			} else if len(byName) > 1 {
				cands := make([]aitool.AmbiguousCandidate, len(byName))
				for i, item := range byName {
					cands[i] = aitool.AmbiguousCandidate{ID: item.ID, Name: item.Name, Stage: item.Stage}
				}
				return nil, aitool.AmbiguousResource("deployment_target", targetRef, cands)
			} else {
				return nil, aitool.NotFound("deployment_target.not_found", "deployment target was not found")
			}
		}
	}

	if status := strings.TrimSpace(in.Status); status != "" {
		switch status {
		case "pending", "running", "succeeded", "failed", "canceled":
			query = query.Where("releases.status = ?", status)
		default:
			return nil, aitool.BadRequest("argument_invalid", "status must be pending, running, succeeded, failed, or canceled")
		}
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var releases []model.Release
	orderClause := sortColumn + " " + sortOrder + ", releases.id " + sortOrder
	if err := query.Order(orderClause).Limit(pageSize).Offset((page - 1) * pageSize).Find(&releases).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	env := aitool.PageEnvelope(releases, page, pageSize, sortBy, sortOrder, total)
	env["projectId"] = project.ID
	if resolvedAppID != "" {
		env["applicationId"] = resolvedAppID
	}
	if resolvedTargetID != "" {
		env["deploymentTargetId"] = resolvedTargetID
	}
	return env, nil
}

func (h *Handler) executeDeliveryGetRelease(tc *aitool.ToolContext, in deliveryGetReleaseInput) (deliveryGetReleaseOutput, error) {
	tc.SetNoStore()
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRead)
	if err != nil {
		return deliveryGetReleaseOutput{}, err
	}
	db := h.aiDB(tc)
	releaseID := strings.TrimSpace(in.ReleaseID)
	if releaseID == "" {
		return deliveryGetReleaseOutput{}, aitool.BadRequest("release.required", "releaseId is required")
	}
	var release model.Release
	if err := db.First(&release, "id = ? AND project_id = ?", releaseID, project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return deliveryGetReleaseOutput{}, aitool.NotFound("release.not_found", "release was not found")
		}
		return deliveryGetReleaseOutput{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	out := deliveryGetReleaseOutput{
		ID:                 release.ID,
		ReleaseID:          release.ID,
		ProjectID:          project.ID,
		ApplicationID:      release.ApplicationID,
		DeploymentTargetID: release.DeploymentTargetID,
		BuildRunID:         release.BuildRunID,
		ImageRef:           release.ImageRef,
		ForceImagePull:     release.ForceImagePull,
		Type:               release.Type,
		Status:             release.Status,
		FailureCode:        release.FailureCode,
		Revision:           release.Revision,
		RollbackFromID:     release.RollbackFromID,
		Message:            release.Message,
		StartedAt:          release.StartedAt,
		FinishedAt:         release.FinishedAt,
		CreatedBy:          release.CreatedBy,
		CreatedAt:          release.CreatedAt,
	}

	if boolVal(in.IncludeLogs, true) {
		tailBytes := intVal(in.LogTailBytes, 8192)
		if tailBytes < 256 {
			tailBytes = 256
		}
		if tailBytes > 32768 {
			tailBytes = 32768
		}
		var relLog model.ReleaseLog
		if logErr := db.First(&relLog, "release_id = ? AND project_id = ?", release.ID, project.ID).Error; logErr == nil {
			logs, trunc, truncBytes := truncateUTF8Tail(relLog.Content, tailBytes)
			out.Logs = logs
			out.LogTruncated = trunc
			out.LogTruncatedBytes = truncBytes
		} else if !errors.Is(logErr, gorm.ErrRecordNotFound) {
			return deliveryGetReleaseOutput{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", logErr.Error())
		}
	}
	return out, nil
}
