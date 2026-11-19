package applicationapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/dependency"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"gorm.io/gorm"
)

var errPreviewDryRunRollback = errors.New("ai.preview.dry_run_rollback")

func mapDependencyErr(err error) *aitool.ToolError {
	code := dependency.ErrorCode(err)
	status := http.StatusInternalServerError
	switch code {
	case dependency.CodeNotFound:
		status = http.StatusNotFound
	case dependency.CodeEnvConflict, dependency.CodeTopologyDuplicate, dependency.CodeTargetDeleting:
		status = http.StatusConflict
	case dependency.CodeInvalidInput, dependency.CodeCrossProject, dependency.CodeCrossCluster,
		dependency.CodeSourceTargetSame, dependency.CodePortNotFound, dependency.CodeReservedEnv:
		status = http.StatusBadRequest
	}
	if code == "" {
		code = "dependency_operation_failed"
	}
	return aitool.NewError(status, code, err.Error())
}

// --- DTOs & Input Structs for Network Topology & Service Binding Tools ---

type aiServiceBindingDTO struct {
	ID                       string                   `json:"id"`
	ProjectID                string                   `json:"projectId"`
	SourceApplicationID      string                   `json:"sourceApplicationId"`
	SourceDeploymentTargetID string                   `json:"sourceDeploymentTargetId"`
	TargetApplicationID      string                   `json:"targetApplicationId"`
	TargetDeploymentTargetID string                   `json:"targetDeploymentTargetId"`
	TargetPortName           string                   `json:"targetPortName"`
	TargetPort               int                      `json:"targetPort"`
	Protocol                 string                   `json:"protocol"`
	Path                     string                   `json:"path"`
	InjectionMode            string                   `json:"injectionMode"`
	URLEnvVar                string                   `json:"urlEnvVar"`
	HostEnvVar               string                   `json:"hostEnvVar"`
	PortEnvVar               string                   `json:"portEnvVar"`
	Enabled                  bool                     `json:"enabled"`
	CredentialMap            []model.SecretMapping    `json:"credentialMap"`
	CreatedBy                string                   `json:"createdBy"`
	CreatedAt                time.Time                `json:"createdAt"`
	UpdatedAt                time.Time                `json:"updatedAt"`
	Check                    *dependency.BindingCheck `json:"check,omitempty"`
}

func toAIServiceBindingDTO(binding model.ServiceBinding, check *dependency.BindingCheck) aiServiceBindingDTO {
	creds := model.DecodeSecretMap(binding.SecretMap)
	if creds == nil {
		creds = []model.SecretMapping{}
	}
	return aiServiceBindingDTO{
		ID:                       binding.ID,
		ProjectID:                binding.ProjectID,
		SourceApplicationID:      binding.SourceApplicationID,
		SourceDeploymentTargetID: binding.SourceDeploymentTargetID,
		TargetApplicationID:      binding.TargetApplicationID,
		TargetDeploymentTargetID: binding.TargetDeploymentTargetID,
		TargetPortName:           binding.TargetPortName,
		TargetPort:               binding.TargetPort,
		Protocol:                 binding.Protocol,
		Path:                     binding.Path,
		InjectionMode:            binding.InjectionMode,
		URLEnvVar:                binding.URLEnvVar,
		HostEnvVar:               binding.HostEnvVar,
		PortEnvVar:               binding.PortEnvVar,
		Enabled:                  binding.Enabled,
		CredentialMap:            creds,
		CreatedBy:                binding.CreatedBy,
		CreatedAt:                binding.CreatedAt,
		UpdatedAt:                binding.UpdatedAt,
		Check:                    check,
	}
}

type networkGetTopologyInput struct {
	Project                    string   `json:"project" desc:"项目 ID 或 identifier"`
	Application                *string  `json:"application,omitempty" desc:"可选应用 ID、identifier 或名称；传入时聚焦该应用子图"`
	Stage                      *string  `json:"stage,omitempty" enum:"dev|test|staging|prod" desc:"按环境阶段过滤依赖拓扑"`
	Origins                    []string `json:"origins,omitempty" enum:"service_binding|manual" desc:"按连线来源过滤"`
	IncludeKubernetesResources *bool    `json:"includeKubernetesResources,omitempty" default:"false" desc:"仅传入 application 时有效，是否额外聚合 Deployment/Pod/Service/HTTPRoute/PVC 实时资源拓扑"`
}

type networkListServiceBindingsInput struct {
	Project        string  `json:"project" desc:"项目 ID 或 identifier"`
	BindingID      *string `json:"bindingId,omitempty" desc:"纯数据库精确过滤单条依赖绑定 ID（不触发 K8s 实时探测）"`
	CheckBindingID *string `json:"checkBindingId,omitempty" desc:"传入具体 bindingId 时实时探测目标 K8s Service 与就绪 Endpoint 状态"`
	SortBy         *string `json:"sortBy,omitempty" enum:"createdAt|updatedAt|protocol|enabled" default:"createdAt" desc:"排序字段"`
	SortOrder      *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page           *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize       *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type networkSaveServiceBindingInput struct {
	Project                 string                `json:"project" desc:"项目 ID 或 identifier"`
	BindingID               *string               `json:"bindingId,omitempty" desc:"省略为新建，传入 bindingId 为基于存量记录增量更新"`
	SourceApplication       *string               `json:"sourceApplication,omitempty" desc:"调用方应用 ID、identifier 或名称（新建必填）"`
	SourceTarget            *string               `json:"sourceTarget,omitempty" desc:"调用方部署环境 ID、stage 或名称（新建必填）"`
	TargetApplication       *string               `json:"targetApplication,omitempty" desc:"被调方应用 ID、identifier 或名称（新建必填）"`
	TargetTarget            *string               `json:"targetTarget,omitempty" desc:"被调方部署环境 ID、stage 或名称（新建必填）"`
	TargetPortName          *string               `json:"targetPortName,omitempty" desc:"被调服务端口名称"`
	TargetPort              *int                  `json:"targetPort,omitempty" min:"1" max:"65535" desc:"被调服务端口号"`
	Protocol                *string               `json:"protocol,omitempty" enum:"http|https|tcp" desc:"通信协议（新建必填，Redis/Postgres 等中间件传 tcp）"`
	InjectionMode           *string               `json:"injectionMode,omitempty" enum:"url|host_port" desc:"环境变量注入模式（新建必填，url 仅支持 http/https）"`
	URLEnvVar               *string               `json:"urlEnvVar,omitempty" desc:"injectionMode=url 时注入的完整连接 URL 大写环境变量名"`
	Path                    *string               `json:"path,omitempty" desc:"injectionMode=url 时可选拼接的以 / 开头路径"`
	ClearPath               *bool                 `json:"clearPath,omitempty" desc:"清空拼接路径"`
	HostEnvVar              *string               `json:"hostEnvVar,omitempty" desc:"injectionMode=host_port 时注入的主机名大写环境变量名"`
	PortEnvVar              *string               `json:"portEnvVar,omitempty" desc:"injectionMode=host_port 时注入的端口号大写环境变量名"`
	UpsertCredentialMap     []model.SecretMapping `json:"upsertCredentialMap,omitempty" desc:"按 sourceEnvVar 增量绑定跨服务 K8s Secret 键名引用"`
	RemoveCredentialEnvVars []string              `json:"removeCredentialEnvVars,omitempty" desc:"按 sourceEnvVar 移除跨服务 Secret 键名引用"`
	Enabled                 *bool                 `json:"enabled,omitempty" desc:"是否启用该服务依赖绑定（新建默认 true）"`
}

type serviceBindingLockedPayload struct {
	ProjectID string                         `json:"projectId"`
	BindingID string                         `json:"bindingId,omitempty"`
	Input     dependency.ServiceBindingInput `json:"input"`
}

type networkDeleteServiceBindingInput struct {
	Project   string `json:"project" desc:"项目 ID 或 identifier"`
	BindingID string `json:"bindingId" desc:"待删除服务依赖绑定 ID"`
}

type networkSaveTopologyEdgeInput struct {
	Project           string  `json:"project" desc:"项目 ID 或 identifier"`
	EdgeID            *string `json:"edgeId,omitempty" desc:"省略为新建，传入 edgeId 为基于存量记录增量更新"`
	SourceApplication *string `json:"sourceApplication,omitempty" desc:"源应用 ID、identifier 或名称（新建必填）"`
	SourceTarget      *string `json:"sourceTarget,omitempty" desc:"可选源部署环境 ID、stage 或名称"`
	ClearSourceTarget *bool   `json:"clearSourceTarget,omitempty" desc:"清空源部署环境绑定"`
	TargetApplication *string `json:"targetApplication,omitempty" desc:"目标应用 ID、identifier 或名称（新建必填）"`
	TargetTarget      *string `json:"targetTarget,omitempty" desc:"可选目标部署环境 ID、stage 或名称"`
	ClearTargetTarget *bool   `json:"clearTargetTarget,omitempty" desc:"清空目标部署环境绑定"`
	RelationType      *string `json:"relationType,omitempty" enum:"depends_on|calls|reads_writes|publishes_to|consumes_from" desc:"架构依赖关系类型（新建必填）"`
	Protocol          *string `json:"protocol,omitempty" enum:"http|https|tcp" desc:"可选通信协议"`
	ClearProtocol     *bool   `json:"clearProtocol,omitempty" desc:"清空通信协议"`
	Port              *int    `json:"port,omitempty" min:"1" max:"65535" desc:"可选端口号（1..65535）"`
	ClearPort         *bool   `json:"clearPort,omitempty" desc:"清空端口号"`
	Description       *string `json:"description,omitempty" desc:"可选架构连线备注说明"`
	ClearDescription  *bool   `json:"clearDescription,omitempty" desc:"清空架构连线备注说明"`
}

type networkDeleteTopologyEdgeInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	EdgeID  string `json:"edgeId" desc:"待删除拓扑自定义连线 ID"`
}

type deleteIDLockedPayload struct {
	ProjectID string `json:"projectId"`
	ID        string `json:"id"`
}

func (h *Handler) buildApplicationKubernetesTopology(
	tc *aitool.ToolContext,
	user model.User,
	project model.Project,
	applicationID string,
	stageFilter string,
) (applicationTopologyResponse, error) {
	query := tc.DB.Where("project_id = ? AND application_id = ? AND delete_status <> ?", project.ID, applicationID, "deleted")
	if stageFilter != "" {
		query = query.Where("stage = ?", stageFilter)
	}
	var targets []model.DeploymentTarget
	if err := query.Order("created_at asc").Find(&targets).Error; err != nil {
		return applicationTopologyResponse{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", "failed to load deployment targets")
	}

	nodes := map[string]kubeprovider.ApplicationTopologyNode{}
	edges := map[string]kubeprovider.ApplicationTopologyEdge{}
	response := applicationTopologyResponse{
		GeneratedAt: time.Now().UTC(),
		Targets:     make([]applicationTopologyTargetResponse, 0, len(targets)),
		Nodes:       []kubeprovider.ApplicationTopologyNode{},
		Edges:       []kubeprovider.ApplicationTopologyEdge{},
		Warnings:    []applicationTopologyWarningResponse{},
	}

	for _, target := range targets {
		c, rec := newAIToolGin(tc)
		cluster, allowed := h.topologyClusterForTarget(c, user, project.ID, target)
		if !allowed {
			return applicationTopologyResponse{}, aiToolErrorFromRecorder(rec, "runtime_cluster.access_denied", "runtime cluster authorization failed")
		}
		namespace := h.host.DeploymentTargetNamespace(project, target)
		response.Targets = append(response.Targets, applicationTopologyTargetResponse{
			ID:          target.ID,
			Name:        target.Name,
			Stage:       target.Stage,
			ClusterID:   cluster.ID,
			ClusterName: cluster.Name,
			Namespace:   namespace,
		})
		warning := applicationTopologyWarningResponse{
			DeploymentTargetID:   target.ID,
			DeploymentTargetName: target.Name,
			ClusterID:            cluster.ID,
			ClusterName:          cluster.Name,
		}
		if strings.TrimSpace(cluster.KubeconfigRef) == "" {
			warning.Code = "cluster_kubeconfig_missing"
			response.Warnings = append(response.Warnings, warning)
			continue
		}
		kubeconfig := h.secrets.ResolveContext(tc.Context(), cluster.KubeconfigRef)
		if strings.TrimSpace(kubeconfig) == "" {
			warning.Code = "cluster_kubeconfig_unavailable"
			response.Warnings = append(response.Warnings, warning)
			continue
		}
		client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
		if err != nil {
			warning.Code = "cluster_client_failed"
			response.Warnings = append(response.Warnings, warning)
			continue
		}
		readCtx, cancel := context.WithTimeout(tc.Context(), 12*time.Second)
		snapshot, err := client.BuildApplicationTopology(readCtx, kubeprovider.ApplicationTopologyOptions{
			ClusterID:          cluster.ID,
			ClusterName:        cluster.Name,
			Namespace:          namespace,
			ProjectID:          project.ID,
			ApplicationID:      applicationID,
			DeploymentTargetID: target.ID,
		})
		cancel()
		if err != nil {
			warning.Code = "topology_read_failed"
			response.Warnings = append(response.Warnings, warning)
			continue
		}
		for _, n := range snapshot.Nodes {
			nodes[n.ID] = n
		}
		for _, e := range snapshot.Edges {
			edges[e.ID] = e
		}
	}

	for _, n := range nodes {
		response.Nodes = append(response.Nodes, n)
	}
	for _, e := range edges {
		response.Edges = append(response.Edges, e)
	}
	sort.Slice(response.Nodes, func(i, j int) bool { return response.Nodes[i].ID < response.Nodes[j].ID })
	sort.Slice(response.Edges, func(i, j int) bool { return response.Edges[i].ID < response.Edges[j].ID })
	return response, nil
}

func (h *Handler) checkAIServiceBindingLive(tc *aitool.ToolContext, project model.Project, bindingID string) (dependency.BindingCheck, error) {
	svc := h.dependencyService(tc.Context())
	binding, err := svc.ServiceBinding(tc.Context(), project.ID, bindingID)
	if err != nil {
		return dependency.BindingCheck{}, mapDependencyErr(err)
	}
	result, err := svc.CheckServiceBinding(tc.Context(), project.ID, binding.ID)
	if err != nil {
		return dependency.BindingCheck{}, mapDependencyErr(err)
	}
	if result.Status == observation.StatusDeclared {
		var targetTarget model.DeploymentTarget
		if err := tc.DB.First(&targetTarget, "id = ? AND project_id = ?", binding.TargetDeploymentTargetID, project.ID).Error; err != nil {
			return dependency.BindingCheck{}, aitool.NotFound(dependency.CodeNotFound, "target deployment target not found")
		}
		client, _, unavailableCode := h.host.KubernetesClientForDeploymentTargetObservation(project, targetTarget, tc.Context())
		if client == nil {
			result.Status = observation.StatusUnavailable
			result.ObservationCode = "service_binding." + unavailableCode
			result.Checks = append(result.Checks, dependency.BindingCheckItem{
				Code: "kubernetes_check", Status: observation.StatusUnavailable,
			})
			return result, nil
		}
		readCtx, cancel := context.WithTimeout(tc.Context(), 12*time.Second)
		diagnostic, diagnosticErr := client.CheckServiceDependency(readCtx, kubeprovider.ServiceDependencyCheckOptions{
			SourceNamespace: h.host.RuntimeProjectNamespace(project),
			TargetNamespace: h.host.RuntimeProjectNamespace(project),
			ServiceName:     h.host.DeploymentTargetResourceName(targetTarget),
			PortName:        binding.TargetPortName,
			PortNumber:      int32(binding.TargetPort),
		})
		cancel()
		if diagnosticErr != nil {
			result.Status = observation.StatusUnavailable
			result.ObservationCode = "service_binding.kubernetes_unavailable"
			result.Checks = append(result.Checks, dependency.BindingCheckItem{
				Code: "kubernetes_check", Status: observation.StatusUnavailable,
			})
		} else {
			result.Status = observation.StatusReady
			result.ObservationCode = "service_binding.ready"
			for _, check := range diagnostic.Checks {
				result.Checks = append(result.Checks, dependency.BindingCheckItem{
					Code:     check.Code,
					Status:   string(check.Status),
					Resource: fmt.Sprintf("%s/%s", diagnostic.TargetNamespace, diagnostic.ServiceName),
				})
				switch {
				case check.Code == kubeprovider.ServiceDependencyCheckServicePortResolved && check.Status == kubeprovider.ServiceDependencyCheckFailed:
					result.Status = "invalid"
					result.ObservationCode = "service_binding.port_unresolved"
				case (check.Code == kubeprovider.ServiceDependencyCheckServiceExists || check.Code == kubeprovider.ServiceDependencyCheckEndpointReady) &&
					check.Status == kubeprovider.ServiceDependencyCheckFailed && result.Status != "invalid":
					result.Status = observation.StatusUnavailable
					if check.Code == kubeprovider.ServiceDependencyCheckServiceExists {
						result.ObservationCode = "service_binding.service_not_found"
					} else {
						result.ObservationCode = "service_binding.endpoint_unavailable"
					}
				}
			}
		}
	} else if result.Status == "disabled" {
		result.ObservationCode = "service_binding.disabled"
	} else if result.Status == "invalid" {
		result.ObservationCode = "service_binding.invalid"
	}
	return result, nil
}

func mergeAICredentialMap(existing []model.SecretMapping, upserts []model.SecretMapping, removeEnvVars []string) ([]model.SecretMapping, error) {
	removeSet := make(map[string]bool, len(removeEnvVars))
	for _, k := range removeEnvVars {
		norm := strings.ToUpper(strings.TrimSpace(k))
		if norm != "" {
			removeSet[norm] = true
		}
	}
	out := make([]model.SecretMapping, 0, len(existing)+len(upserts))
	idxByEnv := map[string]int{}
	for _, item := range existing {
		norm := strings.ToUpper(strings.TrimSpace(item.SourceEnvVar))
		if removeSet[norm] {
			continue
		}
		idxByEnv[norm] = len(out)
		out = append(out, model.SecretMapping{
			SourceEnvVar:    norm,
			TargetSecretKey: strings.TrimSpace(item.TargetSecretKey),
		})
	}
	for _, up := range upserts {
		norm := strings.ToUpper(strings.TrimSpace(up.SourceEnvVar))
		targetKey := strings.TrimSpace(up.TargetSecretKey)
		if norm == "" || targetKey == "" {
			return nil, aitool.BadRequest("argument_invalid", "upsertCredentialMap entries require non-empty sourceEnvVar and targetSecretKey")
		}
		if removeSet[norm] {
			return nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("credential env var %q cannot be in both upsertCredentialMap and removeCredentialEnvVars", norm))
		}
		entry := model.SecretMapping{SourceEnvVar: norm, TargetSecretKey: targetKey}
		if idx, ok := idxByEnv[norm]; ok {
			out[idx] = entry
		} else {
			idxByEnv[norm] = len(out)
			out = append(out, entry)
		}
	}
	return out, nil
}

func (h *Handler) resolveAIServiceBindingInput(
	tc *aitool.ToolContext,
	projectID string,
	in networkSaveServiceBindingInput,
) (string, *model.ServiceBinding, dependency.ServiceBindingInput, error) {
	if in.ClearPath != nil && *in.ClearPath && in.Path != nil && strings.TrimSpace(*in.Path) != "" {
		return "", nil, dependency.ServiceBindingInput{}, aitool.BadRequest("argument_invalid", "cannot specify both path and clearPath=true")
	}

	db := tc.DB
	svc := h.dependencyService(tc.Context())
	bindingID := ""
	if in.BindingID != nil {
		bindingID = strings.TrimSpace(*in.BindingID)
	}

	if bindingID == "" {
		// Create mode
		if in.SourceApplication == nil || strings.TrimSpace(*in.SourceApplication) == "" {
			return "", nil, dependency.ServiceBindingInput{}, aitool.BadRequest("argument_invalid", "sourceApplication is required when creating a service binding")
		}
		if in.SourceTarget == nil || strings.TrimSpace(*in.SourceTarget) == "" {
			return "", nil, dependency.ServiceBindingInput{}, aitool.BadRequest("argument_invalid", "sourceTarget is required when creating a service binding")
		}
		if in.TargetApplication == nil || strings.TrimSpace(*in.TargetApplication) == "" {
			return "", nil, dependency.ServiceBindingInput{}, aitool.BadRequest("argument_invalid", "targetApplication is required when creating a service binding")
		}
		if in.TargetTarget == nil || strings.TrimSpace(*in.TargetTarget) == "" {
			return "", nil, dependency.ServiceBindingInput{}, aitool.BadRequest("argument_invalid", "targetTarget is required when creating a service binding")
		}
		if in.Protocol == nil || strings.TrimSpace(*in.Protocol) == "" {
			return "", nil, dependency.ServiceBindingInput{}, aitool.BadRequest("argument_invalid", "protocol is required when creating a service binding")
		}
		if in.InjectionMode == nil || strings.TrimSpace(*in.InjectionMode) == "" {
			return "", nil, dependency.ServiceBindingInput{}, aitool.BadRequest("argument_invalid", "injectionMode is required when creating a service binding")
		}

		srcApp, err := aitool.ResolveApplication(db, projectID, *in.SourceApplication)
		if err != nil {
			return "", nil, dependency.ServiceBindingInput{}, err
		}
		srcTarget, err := aitool.ResolveDeploymentTarget(db, projectID, srcApp.ID, *in.SourceTarget)
		if err != nil {
			return "", nil, dependency.ServiceBindingInput{}, err
		}
		tgtApp, err := aitool.ResolveApplication(db, projectID, *in.TargetApplication)
		if err != nil {
			return "", nil, dependency.ServiceBindingInput{}, err
		}
		tgtTarget, err := aitool.ResolveDeploymentTarget(db, projectID, tgtApp.ID, *in.TargetTarget)
		if err != nil {
			return "", nil, dependency.ServiceBindingInput{}, err
		}

		creds, err := mergeAICredentialMap(nil, in.UpsertCredentialMap, in.RemoveCredentialEnvVars)
		if err != nil {
			return "", nil, dependency.ServiceBindingInput{}, err
		}

		protocol := strings.ToLower(strings.TrimSpace(*in.Protocol))
		injectionMode := strings.ToLower(strings.TrimSpace(*in.InjectionMode))
		urlEnv := ""
		if in.URLEnvVar != nil {
			urlEnv = strings.TrimSpace(*in.URLEnvVar)
		}
		pathVal := ""
		if in.Path != nil && (in.ClearPath == nil || !*in.ClearPath) {
			pathVal = strings.TrimSpace(*in.Path)
		}
		hostEnv := ""
		if in.HostEnvVar != nil {
			hostEnv = strings.TrimSpace(*in.HostEnvVar)
		}
		portEnv := ""
		if in.PortEnvVar != nil {
			portEnv = strings.TrimSpace(*in.PortEnvVar)
		}
		if injectionMode == "host_port" {
			urlEnv = ""
			pathVal = ""
		} else if injectionMode == "url" {
			hostEnv = ""
			portEnv = ""
		}
		if protocol == "tcp" {
			pathVal = ""
		}

		portName := ""
		if in.TargetPortName != nil {
			portName = strings.TrimSpace(*in.TargetPortName)
		}
		portNum := 0
		if in.TargetPort != nil {
			portNum = *in.TargetPort
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}

		return "", nil, dependency.ServiceBindingInput{
			SourceApplicationID:      srcApp.ID,
			SourceDeploymentTargetID: srcTarget.ID,
			TargetApplicationID:      tgtApp.ID,
			TargetDeploymentTargetID: tgtTarget.ID,
			TargetPortName:           portName,
			TargetPort:               portNum,
			Protocol:                 protocol,
			Path:                     pathVal,
			InjectionMode:            injectionMode,
			URLEnvVar:                urlEnv,
			HostEnvVar:               hostEnv,
			PortEnvVar:               portEnv,
			SecretMap:                creds,
			Enabled:                  &enabled,
		}, nil
	}

	// Update mode (pure patch over existing DB record)
	existing, err := svc.ServiceBinding(tc.Context(), projectID, bindingID)
	if err != nil {
		return "", nil, dependency.ServiceBindingInput{}, mapDependencyErr(err)
	}

	srcAppID := existing.SourceApplicationID
	if in.SourceApplication != nil && strings.TrimSpace(*in.SourceApplication) != "" {
		srcApp, err := aitool.ResolveApplication(db, projectID, *in.SourceApplication)
		if err != nil {
			return "", nil, dependency.ServiceBindingInput{}, err
		}
		if srcApp.ID != existing.SourceApplicationID && (in.SourceTarget == nil || strings.TrimSpace(*in.SourceTarget) == "") {
			return "", nil, dependency.ServiceBindingInput{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"argument_invalid",
				"switching sourceApplication requires specifying sourceTarget under the new application",
				"Pass sourceTarget (stage, name, or ID) belonging to the new sourceApplication.",
			)
		}
		srcAppID = srcApp.ID
	}
	srcTargetID := existing.SourceDeploymentTargetID
	if in.SourceTarget != nil && strings.TrimSpace(*in.SourceTarget) != "" {
		srcTarget, err := aitool.ResolveDeploymentTarget(db, projectID, srcAppID, *in.SourceTarget)
		if err != nil {
			return "", nil, dependency.ServiceBindingInput{}, err
		}
		srcTargetID = srcTarget.ID
	}

	tgtAppID := existing.TargetApplicationID
	if in.TargetApplication != nil && strings.TrimSpace(*in.TargetApplication) != "" {
		tgtApp, err := aitool.ResolveApplication(db, projectID, *in.TargetApplication)
		if err != nil {
			return "", nil, dependency.ServiceBindingInput{}, err
		}
		if tgtApp.ID != existing.TargetApplicationID && (in.TargetTarget == nil || strings.TrimSpace(*in.TargetTarget) == "") {
			return "", nil, dependency.ServiceBindingInput{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"argument_invalid",
				"switching targetApplication requires specifying targetTarget under the new application",
				"Pass targetTarget (stage, name, or ID) belonging to the new targetApplication.",
			)
		}
		tgtAppID = tgtApp.ID
	}
	tgtTargetID := existing.TargetDeploymentTargetID
	if in.TargetTarget != nil && strings.TrimSpace(*in.TargetTarget) != "" {
		tgtTarget, err := aitool.ResolveDeploymentTarget(db, projectID, tgtAppID, *in.TargetTarget)
		if err != nil {
			return "", nil, dependency.ServiceBindingInput{}, err
		}
		tgtTargetID = tgtTarget.ID
	}

	// Port dual-write mutual reset (catalog §3B.2 #33 rule 1)
	portName := existing.TargetPortName
	portNum := existing.TargetPort
	targetSwitched := tgtAppID != existing.TargetApplicationID || tgtTargetID != existing.TargetDeploymentTargetID
	hasPortNameArg := in.TargetPortName != nil && strings.TrimSpace(*in.TargetPortName) != ""
	hasPortNumArg := in.TargetPort != nil
	switch {
	case hasPortNumArg && !hasPortNameArg:
		portNum = *in.TargetPort
		portName = ""
	case hasPortNameArg && !hasPortNumArg:
		portName = strings.TrimSpace(*in.TargetPortName)
		portNum = 0
	case hasPortNameArg && hasPortNumArg:
		portName = strings.TrimSpace(*in.TargetPortName)
		portNum = *in.TargetPort
	case targetSwitched:
		portName = ""
		portNum = 0
	}

	// Protocol & InjectionMode mutual cleanup (catalog §3B.2 #33 rule 2)
	protocol := existing.Protocol
	if in.Protocol != nil && strings.TrimSpace(*in.Protocol) != "" {
		protocol = strings.ToLower(strings.TrimSpace(*in.Protocol))
	}
	injectionMode := existing.InjectionMode
	if in.InjectionMode != nil && strings.TrimSpace(*in.InjectionMode) != "" {
		injectionMode = strings.ToLower(strings.TrimSpace(*in.InjectionMode))
	}
	urlEnv := existing.URLEnvVar
	if in.URLEnvVar != nil && strings.TrimSpace(*in.URLEnvVar) != "" {
		urlEnv = strings.TrimSpace(*in.URLEnvVar)
	}
	pathVal := existing.Path
	if in.ClearPath != nil && *in.ClearPath {
		pathVal = ""
	} else if in.Path != nil {
		pathVal = strings.TrimSpace(*in.Path)
	}
	hostEnv := existing.HostEnvVar
	if in.HostEnvVar != nil && strings.TrimSpace(*in.HostEnvVar) != "" {
		hostEnv = strings.TrimSpace(*in.HostEnvVar)
	}
	portEnv := existing.PortEnvVar
	if in.PortEnvVar != nil && strings.TrimSpace(*in.PortEnvVar) != "" {
		portEnv = strings.TrimSpace(*in.PortEnvVar)
	}

	if injectionMode == "host_port" {
		urlEnv = ""
		pathVal = ""
	} else if injectionMode == "url" {
		hostEnv = ""
		portEnv = ""
	}
	if protocol == "tcp" {
		pathVal = ""
	}

	creds, err := mergeAICredentialMap(model.DecodeSecretMap(existing.SecretMap), in.UpsertCredentialMap, in.RemoveCredentialEnvVars)
	if err != nil {
		return "", nil, dependency.ServiceBindingInput{}, err
	}
	enabled := existing.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	return existing.ID, &existing, dependency.ServiceBindingInput{
		SourceApplicationID:      srcAppID,
		SourceDeploymentTargetID: srcTargetID,
		TargetApplicationID:      tgtAppID,
		TargetDeploymentTargetID: tgtTargetID,
		TargetPortName:           portName,
		TargetPort:               portNum,
		Protocol:                 protocol,
		Path:                     pathVal,
		InjectionMode:            injectionMode,
		URLEnvVar:                urlEnv,
		HostEnvVar:               hostEnv,
		PortEnvVar:               portEnv,
		SecretMap:                creds,
		Enabled:                  &enabled,
	}, nil
}

func (h *Handler) registerNetworkTopologyAndBindingAITools(reg *aitool.Registry) {
	// 9. network_get_topology
	aitool.Register(reg, aitool.Spec[networkGetTopologyInput, map[string]any]{
		Name:           "network_get_topology",
		Domain:         aitool.DomainNetwork,
		Title:          "查看服务依赖与资源拓扑",
		Summary:        "查看项目服务依赖拓扑图，或聚焦单个应用的上下游子图与 K8s 实时资源拓扑",
		AliasesZH:      []string{"项目拓扑", "应用拓扑", "依赖关系图"},
		AliasesEN:      []string{"get topology", "service topology", "dependency graph"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionApplicationRead)},
		Execute: func(tc *aitool.ToolContext, in networkGetTopologyInput) (map[string]any, error) {
			user, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionApplicationRead)
			if err != nil {
				return nil, err
			}

			stageFilter := ""
			if in.Stage != nil && strings.TrimSpace(*in.Stage) != "" {
				normStage, ok := h.host.NormalizePublicStage(*in.Stage)
				if !ok {
					return nil, aitool.BadRequest("argument_invalid", "stage must be one of dev, test, staging, or prod")
				}
				stageFilter = normStage
			}

			originsMap := map[string]bool{}
			for _, o := range in.Origins {
				trimmed := strings.ToLower(strings.TrimSpace(o))
				if trimmed != "service_binding" && trimmed != "manual" {
					return nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid origin %q; allowed values: service_binding, manual", o))
				}
				originsMap[trimmed] = true
			}

			var appID string
			if in.Application != nil && strings.TrimSpace(*in.Application) != "" {
				app, err := aitool.ResolveApplication(tc.DB, project.ID, *in.Application)
				if err != nil {
					return nil, err
				}
				appID = app.ID
			}

			includeK8s := in.IncludeKubernetesResources != nil && *in.IncludeKubernetesResources
			if includeK8s && appID == "" {
				return nil, aitool.NewErrorWithHint(
					http.StatusBadRequest,
					"argument_invalid",
					"includeKubernetesResources=true requires specifying application",
					"Specify application to inspect its live Kubernetes resource tree, or omit includeKubernetesResources for project-wide dependency topology.",
				)
			}

			topo, err := h.dependencyService(tc.Context()).ProjectTopology(tc.Context(), project.ID, dependency.TopologyFilter{
				Stage:         stageFilter,
				ApplicationID: appID,
				Origins:       originsMap,
			})
			if err != nil {
				return nil, mapDependencyErr(err)
			}

			out := map[string]any{
				"projectId":   project.ID,
				"generatedAt": topo.GeneratedAt,
				"nodes":       topo.Nodes,
				"edges":       topo.Edges,
				"warnings":    topo.Warnings,
			}
			if appID != "" {
				out["applicationId"] = appID
			}
			if includeK8s {
				tc.SetNoStore()
				k8sTopo, err := h.buildApplicationKubernetesTopology(tc, user, project, appID, stageFilter)
				if err != nil {
					return nil, err
				}
				out["kubernetesResources"] = k8sTopo
			}
			return out, nil
		},
	})

	// 10. network_list_service_bindings
	aitool.Register(reg, aitool.Spec[networkListServiceBindingsInput, map[string]any]{
		Name:           "network_list_service_bindings",
		Domain:         aitool.DomainNetwork,
		Title:          "分页列出服务间依赖绑定",
		Summary:        "分页列出项目内服务间调用依赖（ServiceBinding），支持按需实时探测 K8s Service/Endpoint 连通性",
		AliasesZH:      []string{"服务依赖列表", "查看服务绑定", "检查服务连通"},
		AliasesEN:      []string{"list service bindings", "check service binding"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionApplicationRead)},
		Execute: func(tc *aitool.ToolContext, in networkListServiceBindingsInput) (map[string]any, error) {
			_, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionApplicationRead)
			if err != nil {
				return nil, err
			}
			sortBy, sortOrder, _, err := normalizeAISort(in.SortBy, in.SortOrder, map[string]string{
				"createdAt": "created_at",
				"updatedAt": "updated_at",
				"protocol":  "protocol",
				"enabled":   "enabled",
			}, "createdAt", "desc")
			if err != nil {
				return nil, err
			}
			page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

			var liveCheck *dependency.BindingCheck
			if in.CheckBindingID != nil && strings.TrimSpace(*in.CheckBindingID) != "" {
				tc.SetNoStore()
				chk, err := h.checkAIServiceBindingLive(tc, project, strings.TrimSpace(*in.CheckBindingID))
				if err != nil {
					return nil, err
				}
				liveCheck = &chk
			}

			var bindings []model.ServiceBinding
			var total int64
			if in.BindingID != nil && strings.TrimSpace(*in.BindingID) != "" {
				var single model.ServiceBinding
				err := tc.DB.First(&single, "id = ? AND project_id = ?", strings.TrimSpace(*in.BindingID), project.ID).Error
				if err == nil {
					bindings = []model.ServiceBinding{single}
					total = 1
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
				}
			} else {
				var listErr error
				bindings, total, listErr = h.dependencyService(tc.Context()).ListServiceBindings(tc.Context(), project.ID, dependency.ListOptions{
					Page:      page,
					PageSize:  pageSize,
					SortBy:    sortBy,
					SortOrder: sortOrder,
				})
				if listErr != nil {
					return nil, mapDependencyErr(listErr)
				}
			}

			dtos := make([]aiServiceBindingDTO, 0, len(bindings))
			for _, b := range bindings {
				var itemCheck *dependency.BindingCheck
				if liveCheck != nil && liveCheck.BindingID == b.ID {
					itemCheck = liveCheck
				}
				dtos = append(dtos, toAIServiceBindingDTO(b, itemCheck))
			}

			out := aitool.PageEnvelope(dtos, page, pageSize, sortBy, sortOrder, total)
			out["projectId"] = project.ID
			if liveCheck != nil {
				out["check"] = liveCheck
			}
			return out, nil
		},
	})

	// 11. network_save_service_binding
	aitool.Register(reg, aitool.Spec[networkSaveServiceBindingInput, map[string]any]{
		Name:           "network_save_service_binding",
		Domain:         aitool.DomainNetwork,
		Title:          "创建或修改服务间依赖绑定",
		Summary:        "创建或纯增量修改项目内服务间调用依赖（立即触发 NetworkPolicy 对账，调用方需重新发版生效环境变量）",
		AliasesZH:      []string{"创建服务依赖", "更新服务绑定", "连接内部服务"},
		AliasesEN:      []string{"save service binding", "create service binding", "update service binding"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "network_list_service_bindings",
				Family:         "resource",
				IDFrom:         "result.id",
				ReadIDArgument: "bindingId",
				Collection:     true,
				Expect:         "present",
			},
		},
		Preview: func(tc *aitool.ToolContext, in networkSaveServiceBindingInput) (*aitool.PreviewResult, error) {
			user, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionProjectManage)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}

			bindingID, existing, resolvedInput, err := h.resolveAIServiceBindingInput(tc, project.ID, in)
			if err != nil {
				return nil, err
			}

			// Dry-run full domain & DB constraint validation inside a rolled-back transaction
			var validated model.ServiceBinding
			txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
				drySvc := dependency.NewService(dependency.NewGormRepository(tx))
				var opErr error
				if bindingID == "" {
					validated, opErr = drySvc.CreateServiceBinding(tc.Context(), project.ID, user.ID, resolvedInput)
				} else {
					validated, opErr = drySvc.UpdateServiceBinding(tc.Context(), project.ID, bindingID, resolvedInput)
				}
				if opErr != nil {
					return opErr
				}
				return errPreviewDryRunRollback
			})
			if txErr != nil && !errors.Is(txErr, errPreviewDryRunRollback) {
				return nil, mapDependencyErr(txErr)
			}

			actionVerb := "Create"
			targetID := ""
			changes := make([]aitool.PreviewChange, 0, 5)
			if existing != nil {
				actionVerb = "Update"
				targetID = existing.ID
				changes = append(changes,
					aitool.PreviewChange{
						Field:  "endpoint",
						Before: fmt.Sprintf("%s -> %s:%d (%s/%s)", existing.SourceDeploymentTargetID, existing.TargetDeploymentTargetID, existing.TargetPort, existing.Protocol, existing.InjectionMode),
						After:  fmt.Sprintf("%s -> %s:%d (%s/%s)", validated.SourceDeploymentTargetID, validated.TargetDeploymentTargetID, validated.TargetPort, validated.Protocol, validated.InjectionMode),
						Action: "update",
					},
					aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: validated.Enabled, Action: "update"},
				)
			} else {
				changes = append(changes, aitool.PreviewChange{
					Field:  "serviceBinding",
					After:  fmt.Sprintf("%s -> %s:%d (%s/%s)", validated.SourceDeploymentTargetID, validated.TargetDeploymentTargetID, validated.TargetPort, validated.Protocol, validated.InjectionMode),
					Action: "create",
				})
			}

			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("%s service binding (%s -> %s:%d)", actionVerb, validated.SourceDeploymentTargetID, validated.TargetDeploymentTargetID, validated.TargetPort),
					TargetResource: "service_binding",
					TargetID:       targetID,
					ProjectID:      project.ID,
					Changes:        changes,
					Impact: []string{
						"Immediately triggers project NetworkPolicy reconciliation",
						fmt.Sprintf("Caller target %s requires a new Release (delivery_deploy_target) to inject updated environment variables", validated.SourceDeploymentTargetID),
					},
				},
				LockedPayload: serviceBindingLockedPayload{
					ProjectID: project.ID,
					BindingID: bindingID,
					Input:     resolvedInput,
				},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in networkSaveServiceBindingInput) (map[string]any, error) {
			user, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionProjectManage)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}

			var bindingID string
			var resolvedInput dependency.ServiceBindingInput
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked serviceBindingLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				if locked.ProjectID != project.ID {
					return nil, aitool.Conflict("ai.preview_lock_mismatch", "preview lock project mismatch")
				}
				bindingID = locked.BindingID
				resolvedInput = locked.Input
			} else {
				bindingID, _, resolvedInput, err = h.resolveAIServiceBindingInput(tc, project.ID, in)
				if err != nil {
					return nil, err
				}
			}

			svc := h.dependencyService(tc.Context())
			var binding model.ServiceBinding
			auditAction := "service_binding.create"
			eventStatus := "created"
			if bindingID == "" {
				binding, err = svc.CreateServiceBinding(tc.Context(), project.ID, user.ID, resolvedInput)
			} else {
				auditAction = "service_binding.update"
				eventStatus = "updated"
				binding, err = svc.UpdateServiceBinding(tc.Context(), project.ID, bindingID, resolvedInput)
			}
			if err != nil {
				auditResource := project.ID
				if bindingID != "" {
					auditResource = bindingID
				}
				h.auditWithContext(user.ID, auditAction, auditResource, false, dependencyAuditMessage(err), tc.Context())
				return nil, mapDependencyErr(err)
			}

			h.auditWithContext(user.ID, auditAction, binding.ID, true, binding.SourceDeploymentTargetID, tc.Context())
			h.emitServiceBindingEvent(tc.Context(), user, project, binding, eventStatus, notification.SeverityInfo)
			h.host.EnqueueNetworkPolicyProjectReconcile(tc.Context(), project.ID)

			dto := toAIServiceBindingDTO(binding, nil)
			return map[string]any{
				"id":                        dto.ID,
				"projectId":                 project.ID,
				"sourceApplicationId":       dto.SourceApplicationID,
				"sourceDeploymentTargetId":  dto.SourceDeploymentTargetID,
				"targetApplicationId":       dto.TargetApplicationID,
				"targetDeploymentTargetId":  dto.TargetDeploymentTargetID,
				"targetPortName":            dto.TargetPortName,
				"targetPort":                dto.TargetPort,
				"protocol":                  dto.Protocol,
				"path":                      dto.Path,
				"injectionMode":             dto.InjectionMode,
				"urlEnvVar":                 dto.URLEnvVar,
				"hostEnvVar":                dto.HostEnvVar,
				"portEnvVar":                dto.PortEnvVar,
				"enabled":                   dto.Enabled,
				"credentialMap":             dto.CredentialMap,
				"item":                      dto,
				"requiresRelease":           true,
				"affectedDeploymentTargets": []map[string]string{{"applicationId": dto.SourceApplicationID, "deploymentTargetId": dto.SourceDeploymentTargetID}},
			}, nil
		},
	})

	// 12. network_delete_service_binding
	aitool.Register(reg, aitool.Spec[networkDeleteServiceBindingInput, map[string]any]{
		Name:           "network_delete_service_binding",
		Domain:         aitool.DomainNetwork,
		Title:          "删除服务间依赖绑定",
		Summary:        "删除服务间调用依赖并同步重算项目 NetworkPolicy",
		AliasesZH:      []string{"删除服务依赖", "解除服务绑定"},
		AliasesEN:      []string{"delete service binding", "remove service binding"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "network_list_service_bindings",
				Family:         "resource",
				IDFrom:         "result.id",
				ReadIDArgument: "bindingId",
				Collection:     true,
				Expect:         "absent",
			},
		},
		Preview: func(tc *aitool.ToolContext, in networkDeleteServiceBindingInput) (*aitool.PreviewResult, error) {
			_, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionProjectManage)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}
			bindingID := strings.TrimSpace(in.BindingID)
			if bindingID == "" {
				return nil, aitool.BadRequest("argument_invalid", "bindingId is required")
			}
			binding, err := h.dependencyService(tc.Context()).ServiceBinding(tc.Context(), project.ID, bindingID)
			if err != nil {
				return nil, mapDependencyErr(err)
			}

			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("Delete service binding %s (%s -> %s:%d)", binding.ID, binding.SourceDeploymentTargetID, binding.TargetDeploymentTargetID, binding.TargetPort),
					TargetResource: "service_binding",
					TargetID:       binding.ID,
					ProjectID:      project.ID,
					Changes: []aitool.PreviewChange{
						{Field: "bindingId", Before: binding.ID, Action: "delete"},
					},
					Impact: []string{
						"Immediately updates project NetworkPolicy ingress rules",
						fmt.Sprintf("Caller target %s requires a new Release to remove injected environment variables", binding.SourceDeploymentTargetID),
					},
				},
				LockedPayload: deleteIDLockedPayload{
					ProjectID: project.ID,
					ID:        binding.ID,
				},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in networkDeleteServiceBindingInput) (map[string]any, error) {
			user, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionProjectManage)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}
			bindingID := strings.TrimSpace(in.BindingID)
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked deleteIDLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				if locked.ProjectID != project.ID || locked.ID != bindingID {
					return nil, aitool.Conflict("ai.preview_lock_mismatch", "preview lock does not match target service binding")
				}
			}

			binding, err := h.dependencyService(tc.Context()).DeleteServiceBinding(tc.Context(), project.ID, bindingID)
			if err != nil {
				h.auditWithContext(user.ID, "service_binding.delete", bindingID, false, dependencyAuditMessage(err), tc.Context())
				return nil, mapDependencyErr(err)
			}
			h.auditWithContext(user.ID, "service_binding.delete", binding.ID, true, binding.SourceDeploymentTargetID, tc.Context())
			h.emitServiceBindingEvent(tc.Context(), user, project, binding, "deleted", notification.SeverityInfo)
			h.host.EnqueueNetworkPolicyProjectReconcile(tc.Context(), project.ID)

			return map[string]any{
				"id":                       binding.ID,
				"projectId":                project.ID,
				"deleted":                  true,
				"requiresRelease":          true,
				"sourceDeploymentTargetId": binding.SourceDeploymentTargetID,
			}, nil
		},
	})

	// 13. network_save_topology_edge
	aitool.Register(reg, aitool.Spec[networkSaveTopologyEdgeInput, model.ProjectTopologyEdge]{
		Name:           "network_save_topology_edge",
		Domain:         aitool.DomainNetwork,
		Title:          "创建或修改拓扑自定义连线",
		Summary:        "创建或纯增量修改项目拓扑图自定义架构说明连线（仅拓扑视图展示，不影响流量策略）",
		AliasesZH:      []string{"添加拓扑连线", "修改拓扑连线", "架构连线"},
		AliasesEN:      []string{"save topology edge", "create topology edge", "update topology edge"},
		ApprovalTier:   aitool.ApprovalTierL1,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "network_get_topology",
				Family:         "resource",
				IDFrom:         "result.id",
				Collection:     true,
				CollectionPath: "edges",
				Expect:         "present",
			},
		},
		Execute: func(tc *aitool.ToolContext, in networkSaveTopologyEdgeInput) (model.ProjectTopologyEdge, error) {
			user, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionProjectManage)
			if err != nil {
				return model.ProjectTopologyEdge{}, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return model.ProjectTopologyEdge{}, err
			}

			if in.ClearSourceTarget != nil && *in.ClearSourceTarget && in.SourceTarget != nil && strings.TrimSpace(*in.SourceTarget) != "" {
				return model.ProjectTopologyEdge{}, aitool.BadRequest("argument_invalid", "cannot specify both sourceTarget and clearSourceTarget=true")
			}
			if in.ClearTargetTarget != nil && *in.ClearTargetTarget && in.TargetTarget != nil && strings.TrimSpace(*in.TargetTarget) != "" {
				return model.ProjectTopologyEdge{}, aitool.BadRequest("argument_invalid", "cannot specify both targetTarget and clearTargetTarget=true")
			}
			if in.ClearProtocol != nil && *in.ClearProtocol && in.Protocol != nil && strings.TrimSpace(*in.Protocol) != "" {
				return model.ProjectTopologyEdge{}, aitool.BadRequest("argument_invalid", "cannot specify both protocol and clearProtocol=true")
			}
			if in.ClearPort != nil && *in.ClearPort && in.Port != nil {
				return model.ProjectTopologyEdge{}, aitool.BadRequest("argument_invalid", "cannot specify both port and clearPort=true")
			}
			if in.ClearDescription != nil && *in.ClearDescription && in.Description != nil && strings.TrimSpace(*in.Description) != "" {
				return model.ProjectTopologyEdge{}, aitool.BadRequest("argument_invalid", "cannot specify both description and clearDescription=true")
			}

			db := tc.DB
			svc := h.dependencyService(tc.Context())
			edgeID := ""
			if in.EdgeID != nil {
				edgeID = strings.TrimSpace(*in.EdgeID)
			}

			var edgeInput dependency.TopologyEdgeInput
			if edgeID == "" {
				if in.SourceApplication == nil || strings.TrimSpace(*in.SourceApplication) == "" {
					return model.ProjectTopologyEdge{}, aitool.BadRequest("argument_invalid", "sourceApplication is required when creating a topology edge")
				}
				if in.TargetApplication == nil || strings.TrimSpace(*in.TargetApplication) == "" {
					return model.ProjectTopologyEdge{}, aitool.BadRequest("argument_invalid", "targetApplication is required when creating a topology edge")
				}
				if in.RelationType == nil || strings.TrimSpace(*in.RelationType) == "" {
					return model.ProjectTopologyEdge{}, aitool.BadRequest("argument_invalid", "relationType is required when creating a topology edge")
				}
				srcApp, err := aitool.ResolveApplication(db, project.ID, *in.SourceApplication)
				if err != nil {
					return model.ProjectTopologyEdge{}, err
				}
				tgtApp, err := aitool.ResolveApplication(db, project.ID, *in.TargetApplication)
				if err != nil {
					return model.ProjectTopologyEdge{}, err
				}
				var srcTargetID, tgtTargetID string
				if in.SourceTarget != nil && strings.TrimSpace(*in.SourceTarget) != "" && (in.ClearSourceTarget == nil || !*in.ClearSourceTarget) {
					st, err := aitool.ResolveDeploymentTarget(db, project.ID, srcApp.ID, *in.SourceTarget)
					if err != nil {
						return model.ProjectTopologyEdge{}, err
					}
					srcTargetID = st.ID
				}
				if in.TargetTarget != nil && strings.TrimSpace(*in.TargetTarget) != "" && (in.ClearTargetTarget == nil || !*in.ClearTargetTarget) {
					tt, err := aitool.ResolveDeploymentTarget(db, project.ID, tgtApp.ID, *in.TargetTarget)
					if err != nil {
						return model.ProjectTopologyEdge{}, err
					}
					tgtTargetID = tt.ID
				}
				proto := ""
				if in.Protocol != nil && (in.ClearProtocol == nil || !*in.ClearProtocol) {
					proto = strings.TrimSpace(*in.Protocol)
				}
				port := 0
				if in.Port != nil && (in.ClearPort == nil || !*in.ClearPort) {
					port = *in.Port
				}
				desc := ""
				if in.Description != nil && (in.ClearDescription == nil || !*in.ClearDescription) {
					desc = strings.TrimSpace(*in.Description)
				}

				edgeInput = dependency.TopologyEdgeInput{
					SourceApplicationID:      srcApp.ID,
					SourceDeploymentTargetID: srcTargetID,
					TargetApplicationID:      tgtApp.ID,
					TargetDeploymentTargetID: tgtTargetID,
					RelationType:             strings.TrimSpace(*in.RelationType),
					Protocol:                 proto,
					Port:                     port,
					Description:              desc,
				}
				edge, createErr := svc.CreateTopologyEdge(tc.Context(), project.ID, user.ID, edgeInput)
				if createErr != nil {
					h.auditWithContext(user.ID, "project_topology_edge.create", project.ID, false, dependencyAuditMessage(createErr), tc.Context())
					return model.ProjectTopologyEdge{}, mapDependencyErr(createErr)
				}
				h.auditWithContext(user.ID, "project_topology_edge.create", edge.ID, true, edge.RelationType, tc.Context())
				return edge, nil
			}

			// Update mode
			var existing model.ProjectTopologyEdge
			if err := db.First(&existing, "id = ? AND project_id = ?", edgeID, project.ID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return model.ProjectTopologyEdge{}, aitool.NotFound(dependency.CodeNotFound, "topology edge was not found")
				}
				return model.ProjectTopologyEdge{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}

			srcAppID := existing.SourceApplicationID
			if in.SourceApplication != nil && strings.TrimSpace(*in.SourceApplication) != "" {
				srcApp, err := aitool.ResolveApplication(db, project.ID, *in.SourceApplication)
				if err != nil {
					return model.ProjectTopologyEdge{}, err
				}
				if srcApp.ID != existing.SourceApplicationID && existing.SourceDeploymentTargetID != "" &&
					(in.SourceTarget == nil || strings.TrimSpace(*in.SourceTarget) == "") &&
					(in.ClearSourceTarget == nil || !*in.ClearSourceTarget) {
					return model.ProjectTopologyEdge{}, aitool.NewErrorWithHint(
						http.StatusBadRequest,
						"argument_invalid",
						"switching sourceApplication on an edge with an existing sourceTarget requires specifying a new sourceTarget or clearSourceTarget=true",
						"Pass sourceTarget under the new application or set clearSourceTarget=true.",
					)
				}
				srcAppID = srcApp.ID
			}
			srcTargetID := existing.SourceDeploymentTargetID
			if in.ClearSourceTarget != nil && *in.ClearSourceTarget {
				srcTargetID = ""
			} else if in.SourceTarget != nil && strings.TrimSpace(*in.SourceTarget) != "" {
				st, err := aitool.ResolveDeploymentTarget(db, project.ID, srcAppID, *in.SourceTarget)
				if err != nil {
					return model.ProjectTopologyEdge{}, err
				}
				srcTargetID = st.ID
			}

			tgtAppID := existing.TargetApplicationID
			if in.TargetApplication != nil && strings.TrimSpace(*in.TargetApplication) != "" {
				tgtApp, err := aitool.ResolveApplication(db, project.ID, *in.TargetApplication)
				if err != nil {
					return model.ProjectTopologyEdge{}, err
				}
				if tgtApp.ID != existing.TargetApplicationID && existing.TargetDeploymentTargetID != "" &&
					(in.TargetTarget == nil || strings.TrimSpace(*in.TargetTarget) == "") &&
					(in.ClearTargetTarget == nil || !*in.ClearTargetTarget) {
					return model.ProjectTopologyEdge{}, aitool.NewErrorWithHint(
						http.StatusBadRequest,
						"argument_invalid",
						"switching targetApplication on an edge with an existing targetTarget requires specifying a new targetTarget or clearTargetTarget=true",
						"Pass targetTarget under the new application or set clearTargetTarget=true.",
					)
				}
				tgtAppID = tgtApp.ID
			}
			tgtTargetID := existing.TargetDeploymentTargetID
			if in.ClearTargetTarget != nil && *in.ClearTargetTarget {
				tgtTargetID = ""
			} else if in.TargetTarget != nil && strings.TrimSpace(*in.TargetTarget) != "" {
				tt, err := aitool.ResolveDeploymentTarget(db, project.ID, tgtAppID, *in.TargetTarget)
				if err != nil {
					return model.ProjectTopologyEdge{}, err
				}
				tgtTargetID = tt.ID
			}

			relType := existing.RelationType
			if in.RelationType != nil && strings.TrimSpace(*in.RelationType) != "" {
				relType = strings.TrimSpace(*in.RelationType)
			}
			proto := existing.Protocol
			if in.ClearProtocol != nil && *in.ClearProtocol {
				proto = ""
			} else if in.Protocol != nil && strings.TrimSpace(*in.Protocol) != "" {
				proto = strings.TrimSpace(*in.Protocol)
			}
			port := existing.Port
			if in.ClearPort != nil && *in.ClearPort {
				port = 0
			} else if in.Port != nil {
				port = *in.Port
			}
			desc := existing.Description
			if in.ClearDescription != nil && *in.ClearDescription {
				desc = ""
			} else if in.Description != nil {
				desc = strings.TrimSpace(*in.Description)
			}

			edgeInput = dependency.TopologyEdgeInput{
				SourceApplicationID:      srcAppID,
				SourceDeploymentTargetID: srcTargetID,
				TargetApplicationID:      tgtAppID,
				TargetDeploymentTargetID: tgtTargetID,
				RelationType:             relType,
				Protocol:                 proto,
				Port:                     port,
				Description:              desc,
			}
			edge, updateErr := svc.UpdateTopologyEdge(tc.Context(), project.ID, edgeID, edgeInput)
			if updateErr != nil {
				h.auditWithContext(user.ID, "project_topology_edge.update", edgeID, false, dependencyAuditMessage(updateErr), tc.Context())
				return model.ProjectTopologyEdge{}, mapDependencyErr(updateErr)
			}
			h.auditWithContext(user.ID, "project_topology_edge.update", edge.ID, true, edge.RelationType, tc.Context())
			return edge, nil
		},
	})

	// 14. network_delete_topology_edge
	aitool.Register(reg, aitool.Spec[networkDeleteTopologyEdgeInput, map[string]any]{
		Name:           "network_delete_topology_edge",
		Domain:         aitool.DomainNetwork,
		Title:          "删除拓扑自定义连线",
		Summary:        "删除项目拓扑图中的自定义架构说明连线",
		AliasesZH:      []string{"删除拓扑连线", "移除架构连线"},
		AliasesEN:      []string{"delete topology edge", "remove topology edge"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "network_get_topology",
				Family:         "resource",
				IDFrom:         "result.id",
				Collection:     true,
				CollectionPath: "edges",
				Expect:         "absent",
			},
		},
		Preview: func(tc *aitool.ToolContext, in networkDeleteTopologyEdgeInput) (*aitool.PreviewResult, error) {
			_, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionProjectManage)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}
			edgeID := strings.TrimSpace(in.EdgeID)
			if edgeID == "" {
				return nil, aitool.BadRequest("argument_invalid", "edgeId is required")
			}
			var edge model.ProjectTopologyEdge
			if err := tc.DB.First(&edge, "id = ? AND project_id = ?", edgeID, project.ID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, aitool.NotFound(dependency.CodeNotFound, "topology edge was not found")
				}
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}

			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("Delete topology edge %s (%s -[%s]-> %s)", edge.ID, edge.SourceApplicationID, edge.RelationType, edge.TargetApplicationID),
					TargetResource: "project_topology_edge",
					TargetID:       edge.ID,
					ProjectID:      project.ID,
					Changes: []aitool.PreviewChange{
						{Field: "edgeId", Before: edge.ID, Action: "delete"},
					},
				},
				LockedPayload: deleteIDLockedPayload{
					ProjectID: project.ID,
					ID:        edge.ID,
				},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in networkDeleteTopologyEdgeInput) (map[string]any, error) {
			user, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionProjectManage)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}
			edgeID := strings.TrimSpace(in.EdgeID)
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked deleteIDLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				if locked.ProjectID != project.ID || locked.ID != edgeID {
					return nil, aitool.Conflict("ai.preview_lock_mismatch", "preview lock does not match target topology edge")
				}
			}

			edge, err := h.dependencyService(tc.Context()).DeleteTopologyEdge(tc.Context(), project.ID, edgeID)
			if err != nil {
				h.auditWithContext(user.ID, "project_topology_edge.delete", edgeID, false, dependencyAuditMessage(err), tc.Context())
				return nil, mapDependencyErr(err)
			}
			h.auditWithContext(user.ID, "project_topology_edge.delete", edge.ID, true, edge.RelationType, tc.Context())

			return map[string]any{
				"id":        edge.ID,
				"projectId": project.ID,
				"deleted":   true,
			}, nil
		},
	})
}
