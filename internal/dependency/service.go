package dependency

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
)

const (
	maxTopologyNodes = 500
	maxTopologyEdges = 1000
)

var envNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

type ServiceBindingInput struct {
	SourceApplicationID      string `json:"sourceApplicationId"`
	SourceDeploymentTargetID string `json:"sourceDeploymentTargetId"`
	TargetApplicationID      string `json:"targetApplicationId"`
	TargetDeploymentTargetID string `json:"targetDeploymentTargetId"`
	TargetPortName           string `json:"targetPortName"`
	TargetPort               int    `json:"targetPort"`
	Protocol                 string `json:"protocol"`
	Path                     string `json:"path"`
	InjectionMode            string `json:"injectionMode"`
	URLEnvVar                string `json:"urlEnvVar"`
	HostEnvVar               string `json:"hostEnvVar"`
	PortEnvVar               string `json:"portEnvVar"`
	Enabled                  *bool  `json:"enabled"`
}

type TopologyEdgeInput struct {
	SourceApplicationID      string `json:"sourceApplicationId"`
	SourceDeploymentTargetID string `json:"sourceDeploymentTargetId"`
	TargetApplicationID      string `json:"targetApplicationId"`
	TargetDeploymentTargetID string `json:"targetDeploymentTargetId"`
	RelationType             string `json:"relationType"`
	Protocol                 string `json:"protocol"`
	Port                     int    `json:"port"`
	Description              string `json:"description"`
}

func (service *Service) ListServiceBindings(ctx context.Context, projectID string, options ListOptions) ([]model.ServiceBinding, int64, error) {
	return service.repository.ListServiceBindings(ctx, projectID, normalizeListOptions(options))
}

func (service *Service) ServiceBinding(ctx context.Context, projectID, bindingID string) (model.ServiceBinding, error) {
	binding, err := service.repository.ServiceBinding(ctx, projectID, bindingID)
	if err != nil {
		return model.ServiceBinding{}, repositoryError(err)
	}
	return binding, nil
}

func (service *Service) CreateServiceBinding(ctx context.Context, projectID, actorID string, input ServiceBindingInput) (model.ServiceBinding, error) {
	binding := model.ServiceBinding{ID: id.New("sbind"), ProjectID: strings.TrimSpace(projectID), CreatedBy: strings.TrimSpace(actorID)}
	if err := service.applyServiceBindingInput(ctx, &binding, input); err != nil {
		return model.ServiceBinding{}, err
	}
	if err := service.repository.CreateServiceBinding(ctx, &binding); err != nil {
		return model.ServiceBinding{}, normalizePersistenceError(err)
	}
	return binding, nil
}

func (service *Service) UpdateServiceBinding(ctx context.Context, projectID, bindingID string, input ServiceBindingInput) (model.ServiceBinding, error) {
	binding, err := service.repository.ServiceBinding(ctx, projectID, bindingID)
	if err != nil {
		return model.ServiceBinding{}, repositoryError(err)
	}
	if err := service.applyServiceBindingInput(ctx, &binding, input); err != nil {
		return model.ServiceBinding{}, err
	}
	if err := service.repository.UpdateServiceBinding(ctx, &binding); err != nil {
		return model.ServiceBinding{}, normalizePersistenceError(err)
	}
	return binding, nil
}

func (service *Service) DeleteServiceBinding(ctx context.Context, projectID, bindingID string) (model.ServiceBinding, error) {
	binding, err := service.repository.ServiceBinding(ctx, projectID, bindingID)
	if err != nil {
		return model.ServiceBinding{}, repositoryError(err)
	}
	if err := service.repository.DeleteServiceBinding(ctx, &binding); err != nil {
		return model.ServiceBinding{}, err
	}
	return binding, nil
}

func (service *Service) applyServiceBindingInput(ctx context.Context, binding *model.ServiceBinding, input ServiceBindingInput) error {
	binding.SourceApplicationID = strings.TrimSpace(input.SourceApplicationID)
	binding.SourceDeploymentTargetID = strings.TrimSpace(input.SourceDeploymentTargetID)
	binding.TargetApplicationID = strings.TrimSpace(input.TargetApplicationID)
	binding.TargetDeploymentTargetID = strings.TrimSpace(input.TargetDeploymentTargetID)
	binding.TargetPortName = strings.TrimSpace(input.TargetPortName)
	binding.TargetPort = input.TargetPort
	binding.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	binding.Path = strings.TrimSpace(input.Path)
	binding.InjectionMode = strings.ToLower(strings.TrimSpace(input.InjectionMode))
	binding.URLEnvVar = strings.ToUpper(strings.TrimSpace(input.URLEnvVar))
	binding.HostEnvVar = strings.ToUpper(strings.TrimSpace(input.HostEnvVar))
	binding.PortEnvVar = strings.ToUpper(strings.TrimSpace(input.PortEnvVar))
	binding.Enabled = input.Enabled == nil || *input.Enabled

	resources, err := service.validateResourcePair(ctx, binding.ProjectID, binding.SourceApplicationID, binding.SourceDeploymentTargetID, binding.TargetApplicationID, binding.TargetDeploymentTargetID, false)
	if err != nil {
		return err
	}
	if resources.sourceTarget.ID == resources.targetTarget.ID {
		return domainError(CodeSourceTargetSame, "source and target deployment targets must differ")
	}
	if resources.sourceTarget.ClusterID != resources.targetTarget.ClusterID {
		return domainError(CodeCrossCluster, "service bindings require source and target deployment targets in the same cluster")
	}
	port, ok := resolveTargetPort(resources.targetTarget, binding.TargetPortName, binding.TargetPort)
	if !ok {
		return domainError(CodePortNotFound, "target service port does not exist")
	}
	binding.TargetPortName = port.Name
	binding.TargetPort = port.Port

	if err := validateServiceBindingShape(*binding); err != nil {
		return err
	}
	envKeys := bindingEnvironmentKeys(*binding)
	if err := validateEnvironmentKeys(envKeys); err != nil {
		return err
	}
	if conflictsWithDeploymentEnv(resources.sourceTarget.EnvVars, envKeys) {
		return domainError(CodeEnvConflict, "service binding environment variable conflicts with deployment configuration")
	}
	conflict, err := service.repository.ConflictingServiceBinding(ctx, binding.SourceDeploymentTargetID, binding.ID, envKeys)
	if err != nil {
		return err
	}
	if conflict {
		return domainError(CodeEnvConflict, "service binding environment variable is already used")
	}
	return nil
}

func (service *Service) ListTopologyEdges(ctx context.Context, projectID string, options ListOptions) ([]model.ProjectTopologyEdge, int64, error) {
	return service.repository.ListTopologyEdges(ctx, projectID, normalizeListOptions(options))
}

func (service *Service) CreateTopologyEdge(ctx context.Context, projectID, actorID string, input TopologyEdgeInput) (model.ProjectTopologyEdge, error) {
	edge := model.ProjectTopologyEdge{ID: id.New("ptedge"), ProjectID: strings.TrimSpace(projectID), CreatedBy: strings.TrimSpace(actorID)}
	if err := service.applyTopologyEdgeInput(ctx, &edge, input); err != nil {
		return model.ProjectTopologyEdge{}, err
	}
	if err := service.repository.CreateTopologyEdge(ctx, &edge); err != nil {
		return model.ProjectTopologyEdge{}, normalizePersistenceError(err)
	}
	return edge, nil
}

func (service *Service) UpdateTopologyEdge(ctx context.Context, projectID, edgeID string, input TopologyEdgeInput) (model.ProjectTopologyEdge, error) {
	edge, err := service.repository.TopologyEdge(ctx, projectID, edgeID)
	if err != nil {
		return model.ProjectTopologyEdge{}, repositoryError(err)
	}
	if err := service.applyTopologyEdgeInput(ctx, &edge, input); err != nil {
		return model.ProjectTopologyEdge{}, err
	}
	if err := service.repository.UpdateTopologyEdge(ctx, &edge); err != nil {
		return model.ProjectTopologyEdge{}, normalizePersistenceError(err)
	}
	return edge, nil
}

func (service *Service) DeleteTopologyEdge(ctx context.Context, projectID, edgeID string) (model.ProjectTopologyEdge, error) {
	edge, err := service.repository.TopologyEdge(ctx, projectID, edgeID)
	if err != nil {
		return model.ProjectTopologyEdge{}, repositoryError(err)
	}
	if err := service.repository.DeleteTopologyEdge(ctx, &edge); err != nil {
		return model.ProjectTopologyEdge{}, err
	}
	return edge, nil
}

func (service *Service) applyTopologyEdgeInput(ctx context.Context, edge *model.ProjectTopologyEdge, input TopologyEdgeInput) error {
	edge.SourceApplicationID = strings.TrimSpace(input.SourceApplicationID)
	edge.SourceDeploymentTargetID = strings.TrimSpace(input.SourceDeploymentTargetID)
	edge.TargetApplicationID = strings.TrimSpace(input.TargetApplicationID)
	edge.TargetDeploymentTargetID = strings.TrimSpace(input.TargetDeploymentTargetID)
	edge.RelationType = strings.ToLower(strings.TrimSpace(input.RelationType))
	edge.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	edge.Port = input.Port
	edge.Description = strings.TrimSpace(input.Description)
	if edge.SourceApplicationID == edge.TargetApplicationID {
		return domainError(CodeSourceTargetSame, "source and target applications must differ")
	}
	if _, err := service.validateResourcePair(ctx, edge.ProjectID, edge.SourceApplicationID, edge.SourceDeploymentTargetID, edge.TargetApplicationID, edge.TargetDeploymentTargetID, true); err != nil {
		return err
	}
	if err := validateTopologyEdgeShape(*edge); err != nil {
		return err
	}
	duplicate, err := service.repository.DuplicateTopologyEdge(ctx, *edge)
	if err != nil {
		return err
	}
	if duplicate {
		return domainError(CodeTopologyDuplicate, "topology edge already exists")
	}
	return nil
}

type resourcePair struct {
	sourceTarget model.DeploymentTarget
	targetTarget model.DeploymentTarget
}

func (service *Service) validateResourcePair(ctx context.Context, projectID, sourceApplicationID, sourceTargetID, targetApplicationID, targetTargetID string, optionalTargets bool) (resourcePair, error) {
	if projectID == "" || sourceApplicationID == "" || targetApplicationID == "" {
		return resourcePair{}, domainError(CodeInvalidInput, "project and application identifiers are required")
	}
	sourceApplication, err := service.repository.Application(ctx, sourceApplicationID)
	if err != nil {
		return resourcePair{}, repositoryError(err)
	}
	targetApplication, err := service.repository.Application(ctx, targetApplicationID)
	if err != nil {
		return resourcePair{}, repositoryError(err)
	}
	if sourceApplication.ProjectID != projectID || targetApplication.ProjectID != projectID {
		return resourcePair{}, domainError(CodeCrossProject, "dependency resources must belong to the current project")
	}
	if !optionalTargets && (sourceTargetID == "" || targetTargetID == "") {
		return resourcePair{}, domainError(CodeInvalidInput, "both deployment target identifiers are required when either is specified")
	}
	var sourceTarget model.DeploymentTarget
	if sourceTargetID != "" {
		sourceTarget, err = service.repository.DeploymentTarget(ctx, sourceTargetID)
		if err != nil {
			return resourcePair{}, repositoryError(err)
		}
		if sourceTarget.ProjectID != projectID || sourceTarget.ApplicationID != sourceApplicationID {
			return resourcePair{}, domainError(CodeCrossProject, "source deployment target does not belong to the selected application")
		}
	}
	var targetTarget model.DeploymentTarget
	if targetTargetID != "" {
		targetTarget, err = service.repository.DeploymentTarget(ctx, targetTargetID)
		if err != nil {
			return resourcePair{}, repositoryError(err)
		}
		if targetTarget.ProjectID != projectID || targetTarget.ApplicationID != targetApplicationID {
			return resourcePair{}, domainError(CodeCrossProject, "target deployment target does not belong to the selected application")
		}
	}
	return resourcePair{sourceTarget: sourceTarget, targetTarget: targetTarget}, nil
}

func validateServiceBindingShape(binding model.ServiceBinding) error {
	if binding.Protocol != "http" && binding.Protocol != "https" && binding.Protocol != "tcp" {
		return domainError(CodeInvalidInput, "unsupported service binding protocol")
	}
	switch binding.InjectionMode {
	case "url":
		if binding.Protocol == "tcp" || binding.URLEnvVar == "" || binding.HostEnvVar != "" || binding.PortEnvVar != "" {
			return domainError(CodeInvalidInput, "URL injection requires HTTP(S) and exactly one URL environment variable")
		}
	case "host_port":
		if binding.URLEnvVar != "" || binding.HostEnvVar == "" || binding.PortEnvVar == "" || binding.HostEnvVar == binding.PortEnvVar {
			return domainError(CodeInvalidInput, "host/port injection requires two distinct environment variables")
		}
	default:
		return domainError(CodeInvalidInput, "unsupported injection mode")
	}
	if binding.Path != "" {
		parsed, err := url.Parse(binding.Path)
		if err != nil || !strings.HasPrefix(binding.Path, "/") || strings.HasPrefix(binding.Path, "//") || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return domainError(CodeInvalidInput, "service binding path must be an absolute path without host, query, or fragment")
		}
		if binding.Protocol == "tcp" {
			return domainError(CodeInvalidInput, "TCP service bindings cannot define an HTTP path")
		}
	}
	return nil
}

func validateTopologyEdgeShape(edge model.ProjectTopologyEdge) error {
	allowedRelations := map[string]bool{"depends_on": true, "calls": true, "reads_writes": true, "publishes_to": true, "consumes_from": true}
	if !allowedRelations[edge.RelationType] {
		return domainError(CodeInvalidInput, "unsupported topology relation type")
	}
	if edge.Protocol != "" && edge.Protocol != "http" && edge.Protocol != "https" && edge.Protocol != "tcp" {
		return domainError(CodeInvalidInput, "unsupported topology protocol")
	}
	if edge.Port < 0 || edge.Port > 65535 {
		return domainError(CodeInvalidInput, "topology port must be between 1 and 65535")
	}
	if len([]rune(edge.Description)) > 500 {
		return domainError(CodeInvalidInput, "topology description must not exceed 500 characters")
	}
	return nil
}

func resolveTargetPort(target model.DeploymentTarget, name string, port int) (model.DeploymentServicePort, bool) {
	name = strings.TrimSpace(name)
	for _, candidate := range model.DeploymentTargetServicePorts(target) {
		if name != "" && candidate.Name != name {
			continue
		}
		if port > 0 && candidate.Port != port {
			continue
		}
		return candidate, true
	}
	return model.DeploymentServicePort{}, false
}

func bindingEnvironmentKeys(binding model.ServiceBinding) []string {
	if binding.InjectionMode == "url" {
		return []string{binding.URLEnvVar}
	}
	return []string{binding.HostEnvVar, binding.PortEnvVar}
}

func validateEnvironmentKeys(keys []string) error {
	for _, key := range keys {
		if !envNamePattern.MatchString(key) {
			return domainError(CodeInvalidInput, "invalid environment variable name")
		}
		if strings.HasPrefix(key, "LUNA_") || strings.HasPrefix(key, "LUNA_DEVOPS_") || strings.HasPrefix(key, "KUBERNETES_") {
			return domainError(CodeReservedEnv, "environment variable is reserved by the platform")
		}
	}
	return nil
}

func conflictsWithDeploymentEnv(raw string, keys []string) bool {
	existing := map[string]struct{}{}
	var object map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &object) == nil {
		for key := range object {
			existing[strings.ToUpper(strings.TrimSpace(key))] = struct{}{}
		}
	} else {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, _, found := strings.Cut(line, "=")
			if found {
				existing[strings.ToUpper(strings.TrimSpace(key))] = struct{}{}
			}
		}
	}
	for _, key := range keys {
		if _, ok := existing[key]; ok {
			return true
		}
	}
	return false
}

func normalizeListOptions(options ListOptions) ListOptions {
	if options.Page < 1 {
		options.Page = 1
	}
	if options.PageSize < 1 {
		options.PageSize = 20
	}
	if options.PageSize > 100 {
		options.PageSize = 100
	}
	options.SortOrder = normalizedSortOrder(options.SortOrder)
	return options
}

func normalizePersistenceError(err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "idx_project_topology_edges_identity") {
		return domainError(CodeTopologyDuplicate, "topology edge already exists")
	}
	if strings.Contains(message, "idx_service_bindings_source_") || strings.Contains(message, "duplicate key") && strings.Contains(message, "env") {
		return domainError(CodeEnvConflict, "service binding environment variable is already used")
	}
	return err
}
