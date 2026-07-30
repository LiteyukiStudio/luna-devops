package dependency

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
)

type TopologyFilter struct {
	Stage         string
	ApplicationID string
	Origins       map[string]bool
}

type Topology struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	Nodes       []TopologyNode    `json:"nodes"`
	Edges       []TopologyLink    `json:"edges"`
	Warnings    []TopologyWarning `json:"warnings"`
}

type TopologyNode struct {
	ID                string                     `json:"id"`
	Kind              string                     `json:"kind"`
	Name              string                     `json:"name"`
	Identifier        string                     `json:"identifier"`
	Status            string                     `json:"status"`
	DeploymentTargets []TopologyDeploymentTarget `json:"deploymentTargets"`
}

type TopologyDeploymentTarget struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Stage     string `json:"stage"`
	ClusterID string `json:"clusterId"`
	Enabled   bool   `json:"enabled"`
}

type TopologyLink struct {
	ID                       string `json:"id"`
	Source                   string `json:"source"`
	Target                   string `json:"target"`
	SourceDeploymentTargetID string `json:"sourceDeploymentTargetId,omitempty"`
	TargetDeploymentTargetID string `json:"targetDeploymentTargetId,omitempty"`
	Origin                   string `json:"origin"`
	RelationType             string `json:"relationType"`
	Status                   string `json:"status"`
	Protocol                 string `json:"protocol,omitempty"`
	Port                     int    `json:"port,omitempty"`
	Description              string `json:"description,omitempty"`
	InjectionMode            string `json:"injectionMode,omitempty"`
	URLEnvVar                string `json:"urlEnvVar,omitempty"`
	HostEnvVar               string `json:"hostEnvVar,omitempty"`
	PortEnvVar               string `json:"portEnvVar,omitempty"`
}

type TopologyWarning struct {
	Code string `json:"code"`
}

func (service *Service) ProjectTopology(ctx context.Context, projectID string, filter TopologyFilter) (Topology, error) {
	applications, err := service.repository.Applications(ctx, projectID)
	if err != nil {
		return Topology{}, err
	}
	targets, err := service.repository.DeploymentTargets(ctx, projectID)
	if err != nil {
		return Topology{}, err
	}
	bindings, err := service.repository.ServiceBindings(ctx, projectID)
	if err != nil {
		return Topology{}, err
	}
	manualEdges, err := service.repository.TopologyEdges(ctx, projectID)
	if err != nil {
		return Topology{}, err
	}

	stage := strings.TrimSpace(filter.Stage)
	origins := filter.Origins
	if len(origins) == 0 {
		origins = map[string]bool{"service_binding": true, "manual": true}
	}
	targetByID := make(map[string]model.DeploymentTarget, len(targets))
	targetsByApplication := make(map[string][]TopologyDeploymentTarget)
	for _, target := range targets {
		targetByID[target.ID] = target
		if stage != "" && target.Stage != stage {
			continue
		}
		targetsByApplication[target.ApplicationID] = append(targetsByApplication[target.ApplicationID], TopologyDeploymentTarget{
			ID: target.ID, Name: target.Name, Stage: target.Stage, ClusterID: target.ClusterID, Enabled: target.Enabled,
		})
	}

	links := make([]TopologyLink, 0, len(bindings)+len(manualEdges))
	for _, binding := range bindings {
		if !origins["service_binding"] || !topologyApplicationMatches(filter.ApplicationID, binding.SourceApplicationID, binding.TargetApplicationID) {
			continue
		}
		if !topologyTargetStageMatches(stage, targetByID, binding.SourceDeploymentTargetID, binding.TargetDeploymentTargetID) {
			continue
		}
		status := observation.StatusDeclared
		if !binding.Enabled {
			status = "disabled"
		}
		links = append(links, TopologyLink{
			ID: binding.ID, Source: binding.SourceApplicationID, Target: binding.TargetApplicationID,
			SourceDeploymentTargetID: binding.SourceDeploymentTargetID, TargetDeploymentTargetID: binding.TargetDeploymentTargetID,
			Origin: "service_binding", RelationType: "calls", Status: status, Protocol: binding.Protocol, Port: binding.TargetPort,
			InjectionMode: binding.InjectionMode, URLEnvVar: binding.URLEnvVar, HostEnvVar: binding.HostEnvVar, PortEnvVar: binding.PortEnvVar,
		})
	}
	for _, edge := range manualEdges {
		if !origins["manual"] || !topologyApplicationMatches(filter.ApplicationID, edge.SourceApplicationID, edge.TargetApplicationID) {
			continue
		}
		if !topologyOptionalTargetStageMatches(stage, targetByID, edge.SourceDeploymentTargetID, edge.TargetDeploymentTargetID) {
			continue
		}
		links = append(links, TopologyLink{
			ID: edge.ID, Source: edge.SourceApplicationID, Target: edge.TargetApplicationID,
			SourceDeploymentTargetID: edge.SourceDeploymentTargetID, TargetDeploymentTargetID: edge.TargetDeploymentTargetID,
			Origin: "manual", RelationType: edge.RelationType, Status: observation.StatusDeclared, Protocol: edge.Protocol, Port: edge.Port, Description: edge.Description,
		})
	}

	visibleApplications := map[string]bool{}
	if filter.ApplicationID != "" {
		visibleApplications[filter.ApplicationID] = true
		for _, link := range links {
			visibleApplications[link.Source] = true
			visibleApplications[link.Target] = true
		}
	} else if stage != "" {
		for applicationID := range targetsByApplication {
			visibleApplications[applicationID] = true
		}
		for _, link := range links {
			visibleApplications[link.Source] = true
			visibleApplications[link.Target] = true
		}
	} else {
		for _, application := range applications {
			visibleApplications[application.ID] = true
		}
	}

	nodes := make([]TopologyNode, 0, len(visibleApplications))
	for _, application := range applications {
		if !visibleApplications[application.ID] {
			continue
		}
		applicationTargets := targetsByApplication[application.ID]
		if applicationTargets == nil {
			applicationTargets = []TopologyDeploymentTarget{}
		}
		nodes = append(nodes, TopologyNode{
			ID: application.ID, Kind: "application", Name: application.Name, Identifier: application.Identifier, Status: observation.StatusDeclared, DeploymentTargets: applicationTargets,
		})
	}

	warnings := make([]TopologyWarning, 0, 2)
	if topologyHasCycle(links) {
		warnings = append(warnings, TopologyWarning{Code: CodeDependencyCycle})
	}
	truncated := false
	if len(nodes) > maxTopologyNodes {
		nodes = nodes[:maxTopologyNodes]
		truncated = true
	}
	allowedNodes := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		allowedNodes[node.ID] = true
	}
	filteredLinks := links[:0]
	for _, link := range links {
		if allowedNodes[link.Source] && allowedNodes[link.Target] {
			filteredLinks = append(filteredLinks, link)
		}
	}
	links = filteredLinks
	if len(links) > maxTopologyEdges {
		links = links[:maxTopologyEdges]
		truncated = true
	}
	if truncated {
		warnings = append(warnings, TopologyWarning{Code: CodeTopologyTruncated})
	}
	if nodes == nil {
		nodes = []TopologyNode{}
	}
	if links == nil {
		links = []TopologyLink{}
	}
	return Topology{GeneratedAt: time.Now().UTC(), Nodes: nodes, Edges: links, Warnings: warnings}, nil
}

func topologyApplicationMatches(filterApplicationID, sourceApplicationID, targetApplicationID string) bool {
	return filterApplicationID == "" || filterApplicationID == sourceApplicationID || filterApplicationID == targetApplicationID
}

func topologyTargetStageMatches(stage string, targets map[string]model.DeploymentTarget, sourceID, targetID string) bool {
	if stage == "" {
		return true
	}
	source, sourceOK := targets[sourceID]
	target, targetOK := targets[targetID]
	return sourceOK && targetOK && source.Stage == stage && target.Stage == stage
}

func topologyOptionalTargetStageMatches(stage string, targets map[string]model.DeploymentTarget, sourceID, targetID string) bool {
	if stage == "" {
		return true
	}
	for _, targetID := range []string{sourceID, targetID} {
		if targetID == "" {
			continue
		}
		target, ok := targets[targetID]
		if !ok || target.Stage != stage {
			return false
		}
	}
	return true
}

func topologyHasCycle(links []TopologyLink) bool {
	adjacency := map[string][]string{}
	for _, link := range links {
		adjacency[link.Source] = append(adjacency[link.Source], link.Target)
	}
	for source := range adjacency {
		sort.Strings(adjacency[source])
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == 1 {
			return true
		}
		if state[node] == 2 {
			return false
		}
		state[node] = 1
		for _, target := range adjacency[node] {
			if visit(target) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for node := range adjacency {
		if visit(node) {
			return true
		}
	}
	return false
}

type BindingCheck struct {
	BindingID       string             `json:"bindingId"`
	CheckedAt       time.Time          `json:"checkedAt"`
	Status          string             `json:"status"`
	ObservationCode string             `json:"observationCode,omitempty"`
	Checks          []BindingCheckItem `json:"checks"`
}

type BindingCheckItem struct {
	Code     string `json:"code"`
	Status   string `json:"status"`
	Resource string `json:"resource,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func (service *Service) CheckServiceBinding(ctx context.Context, projectID, bindingID string) (BindingCheck, error) {
	binding, err := service.repository.ServiceBinding(ctx, projectID, bindingID)
	if err != nil {
		return BindingCheck{}, repositoryError(err)
	}
	input := ServiceBindingInput{
		SourceApplicationID: binding.SourceApplicationID, SourceDeploymentTargetID: binding.SourceDeploymentTargetID,
		TargetApplicationID: binding.TargetApplicationID, TargetDeploymentTargetID: binding.TargetDeploymentTargetID,
		TargetPortName: binding.TargetPortName, TargetPort: binding.TargetPort, Protocol: binding.Protocol, Path: binding.Path,
		InjectionMode: binding.InjectionMode, URLEnvVar: binding.URLEnvVar, HostEnvVar: binding.HostEnvVar, PortEnvVar: binding.PortEnvVar,
		Enabled: &binding.Enabled,
	}
	candidate := binding
	err = service.applyServiceBindingInput(ctx, &candidate, input)
	checks := []BindingCheckItem{
		{Code: "cluster_match", Status: "passed"},
		{Code: "binding_config_valid", Status: "passed"},
	}
	status := observation.StatusDeclared
	if !binding.Enabled {
		status = "disabled"
	}
	if err != nil {
		status = "invalid"
		checks = []BindingCheckItem{{Code: ErrorCode(err), Status: "failed"}}
	}
	return BindingCheck{BindingID: binding.ID, CheckedAt: time.Now().UTC(), Status: status, Checks: checks}, nil
}
