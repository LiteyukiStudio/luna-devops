package dependency

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

type fakeRepository struct {
	applications map[string]model.Application
	targets      map[string]model.DeploymentTarget
	bindings     map[string]model.ServiceBinding
	edges        map[string]model.ProjectTopologyEdge
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		applications: map[string]model.Application{}, targets: map[string]model.DeploymentTarget{},
		bindings: map[string]model.ServiceBinding{}, edges: map[string]model.ProjectTopologyEdge{},
	}
}

func (repository *fakeRepository) Application(_ context.Context, id string) (model.Application, error) {
	item, ok := repository.applications[id]
	if !ok {
		return model.Application{}, gorm.ErrRecordNotFound
	}
	return item, nil
}

func (repository *fakeRepository) DeploymentTarget(_ context.Context, id string) (model.DeploymentTarget, error) {
	item, ok := repository.targets[id]
	if !ok {
		return model.DeploymentTarget{}, gorm.ErrRecordNotFound
	}
	return item, nil
}

func (repository *fakeRepository) Applications(_ context.Context, projectID string) ([]model.Application, error) {
	items := []model.Application{}
	for _, item := range repository.applications {
		if item.ProjectID == projectID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (repository *fakeRepository) DeploymentTargets(_ context.Context, projectID string) ([]model.DeploymentTarget, error) {
	items := []model.DeploymentTarget{}
	for _, item := range repository.targets {
		if item.ProjectID == projectID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (repository *fakeRepository) ServiceBinding(_ context.Context, projectID, id string) (model.ServiceBinding, error) {
	item, ok := repository.bindings[id]
	if !ok || item.ProjectID != projectID {
		return model.ServiceBinding{}, gorm.ErrRecordNotFound
	}
	return item, nil
}

func (repository *fakeRepository) ServiceBindings(_ context.Context, projectID string) ([]model.ServiceBinding, error) {
	items := []model.ServiceBinding{}
	for _, item := range repository.bindings {
		if item.ProjectID == projectID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (repository *fakeRepository) ListServiceBindings(ctx context.Context, projectID string, _ ListOptions) ([]model.ServiceBinding, int64, error) {
	items, err := repository.ServiceBindings(ctx, projectID)
	return items, int64(len(items)), err
}

func (repository *fakeRepository) ConflictingServiceBinding(_ context.Context, sourceTargetID, excludedID string, envKeys []string) (bool, error) {
	keySet := map[string]bool{}
	for _, key := range envKeys {
		keySet[key] = true
	}
	for _, item := range repository.bindings {
		if item.ID == excludedID || item.SourceDeploymentTargetID != sourceTargetID {
			continue
		}
		if keySet[item.URLEnvVar] || keySet[item.HostEnvVar] || keySet[item.PortEnvVar] {
			return true, nil
		}
	}
	return false, nil
}

func (repository *fakeRepository) CreateServiceBinding(_ context.Context, item *model.ServiceBinding) error {
	repository.bindings[item.ID] = *item
	return nil
}

func (repository *fakeRepository) UpdateServiceBinding(_ context.Context, item *model.ServiceBinding) error {
	repository.bindings[item.ID] = *item
	return nil
}

func (repository *fakeRepository) DeleteServiceBinding(_ context.Context, item *model.ServiceBinding) error {
	delete(repository.bindings, item.ID)
	return nil
}

func (repository *fakeRepository) TopologyEdge(_ context.Context, projectID, id string) (model.ProjectTopologyEdge, error) {
	item, ok := repository.edges[id]
	if !ok || item.ProjectID != projectID {
		return model.ProjectTopologyEdge{}, gorm.ErrRecordNotFound
	}
	return item, nil
}

func (repository *fakeRepository) TopologyEdges(_ context.Context, projectID string) ([]model.ProjectTopologyEdge, error) {
	items := []model.ProjectTopologyEdge{}
	for _, item := range repository.edges {
		if item.ProjectID == projectID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (repository *fakeRepository) ListTopologyEdges(ctx context.Context, projectID string, _ ListOptions) ([]model.ProjectTopologyEdge, int64, error) {
	items, err := repository.TopologyEdges(ctx, projectID)
	return items, int64(len(items)), err
}

func (repository *fakeRepository) DuplicateTopologyEdge(_ context.Context, candidate model.ProjectTopologyEdge) (bool, error) {
	for _, item := range repository.edges {
		if item.ID == candidate.ID {
			continue
		}
		if item.ProjectID == candidate.ProjectID && item.SourceApplicationID == candidate.SourceApplicationID &&
			item.SourceDeploymentTargetID == candidate.SourceDeploymentTargetID && item.TargetApplicationID == candidate.TargetApplicationID &&
			item.TargetDeploymentTargetID == candidate.TargetDeploymentTargetID && item.RelationType == candidate.RelationType &&
			item.Protocol == candidate.Protocol && item.Port == candidate.Port {
			return true, nil
		}
	}
	return false, nil
}

func (repository *fakeRepository) CreateTopologyEdge(_ context.Context, item *model.ProjectTopologyEdge) error {
	repository.edges[item.ID] = *item
	return nil
}

func (repository *fakeRepository) UpdateTopologyEdge(_ context.Context, item *model.ProjectTopologyEdge) error {
	repository.edges[item.ID] = *item
	return nil
}

func (repository *fakeRepository) DeleteTopologyEdge(_ context.Context, item *model.ProjectTopologyEdge) error {
	delete(repository.edges, item.ID)
	return nil
}

func dependencyFixture() *fakeRepository {
	repository := newFakeRepository()
	repository.applications["app_source"] = model.Application{ID: "app_source", ProjectID: "prj_main", Name: "Source", Identifier: "source"}
	repository.applications["app_target"] = model.Application{ID: "app_target", ProjectID: "prj_main", Name: "Target", Identifier: "target"}
	repository.targets["dplt_source"] = model.DeploymentTarget{
		ID: "dplt_source", ProjectID: "prj_main", ApplicationID: "app_source", Name: "source-prod", Stage: "prod", ClusterID: "clu_main", Enabled: true,
		ServicePorts: model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 8080}}, 8080), EnvVars: `{}`,
	}
	repository.targets["dplt_target"] = model.DeploymentTarget{
		ID: "dplt_target", ProjectID: "prj_main", ApplicationID: "app_target", Name: "target-prod", Stage: "prod", ClusterID: "clu_main", Enabled: true,
		ServicePorts: model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 9000}, {Name: "metrics", Port: 9090}}, 9000),
	}
	return repository
}

func validBindingInput() ServiceBindingInput {
	return ServiceBindingInput{
		SourceApplicationID: "app_source", SourceDeploymentTargetID: "dplt_source",
		TargetApplicationID: "app_target", TargetDeploymentTargetID: "dplt_target",
		TargetPortName: "http", Protocol: "http", InjectionMode: "url", URLEnvVar: "API_URL",
	}
}

func TestCreateServiceBindingNormalizesAndPersistsSafeAddressMetadata(t *testing.T) {
	repository := dependencyFixture()
	binding, err := NewService(repository).CreateServiceBinding(context.Background(), "prj_main", "usr_owner", validBindingInput())
	if err != nil {
		t.Fatalf("create service binding: %v", err)
	}
	if binding.TargetPort != 9000 || binding.TargetPortName != "http" || !binding.Enabled {
		t.Fatalf("normalized binding = %#v", binding)
	}
	encoded, err := json.Marshal(binding)
	if err != nil {
		t.Fatalf("marshal binding: %v", err)
	}
	for _, forbidden := range []string{"password", "token", "secret"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("binding response contains secret-bearing field %q: %s", forbidden, encoded)
		}
	}
}

func TestServiceBindingValidationCodes(t *testing.T) {
	tests := []struct {
		name string
		edit func(*fakeRepository, *ServiceBindingInput)
		code string
	}{
		{name: "cross project", code: CodeCrossProject, edit: func(repository *fakeRepository, _ *ServiceBindingInput) {
			item := repository.applications["app_target"]
			item.ProjectID = "prj_other"
			repository.applications[item.ID] = item
		}},
		{name: "same target", code: CodeSourceTargetSame, edit: func(_ *fakeRepository, input *ServiceBindingInput) {
			input.TargetApplicationID = "app_source"
			input.TargetDeploymentTargetID = "dplt_source"
			input.TargetPortName = "http"
		}},
		{name: "cross cluster", code: CodeCrossCluster, edit: func(repository *fakeRepository, _ *ServiceBindingInput) {
			item := repository.targets["dplt_target"]
			item.ClusterID = "clu_other"
			repository.targets[item.ID] = item
		}},
		{name: "port missing", code: CodePortNotFound, edit: func(_ *fakeRepository, input *ServiceBindingInput) {
			input.TargetPortName = "admin"
		}},
		{name: "existing env", code: CodeEnvConflict, edit: func(repository *fakeRepository, _ *ServiceBindingInput) {
			item := repository.targets["dplt_source"]
			item.EnvVars = `{"API_URL":"manual"}`
			repository.targets[item.ID] = item
		}},
		{name: "reserved env", code: CodeReservedEnv, edit: func(_ *fakeRepository, input *ServiceBindingInput) {
			input.URLEnvVar = "LUNA_INTERNAL_URL"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := dependencyFixture()
			input := validBindingInput()
			test.edit(repository, &input)
			_, err := NewService(repository).CreateServiceBinding(context.Background(), "prj_main", "usr_owner", input)
			if got := ErrorCode(err); got != test.code {
				t.Fatalf("error code = %q, want %q (error=%v)", got, test.code, err)
			}
		})
	}
}

func TestTopologyEdgeRejectsSelfReferenceAndDuplicate(t *testing.T) {
	repository := dependencyFixture()
	service := NewService(repository)
	input := TopologyEdgeInput{SourceApplicationID: "app_source", TargetApplicationID: "app_target", RelationType: "depends_on"}
	if _, err := service.CreateTopologyEdge(context.Background(), "prj_main", "usr_owner", input); err != nil {
		t.Fatalf("create topology edge: %v", err)
	}
	if _, err := service.CreateTopologyEdge(context.Background(), "prj_main", "usr_owner", input); ErrorCode(err) != CodeTopologyDuplicate {
		t.Fatalf("duplicate error = %v", err)
	}
	input.TargetApplicationID = input.SourceApplicationID
	if _, err := service.CreateTopologyEdge(context.Background(), "prj_main", "usr_owner", input); ErrorCode(err) != CodeSourceTargetSame {
		t.Fatalf("self-reference error = %v", err)
	}
}

func TestTopologyEdgeAllowsOneOptionalDeploymentTarget(t *testing.T) {
	repository := dependencyFixture()
	edge, err := NewService(repository).CreateTopologyEdge(context.Background(), "prj_main", "usr_owner", TopologyEdgeInput{
		SourceApplicationID: "app_source", SourceDeploymentTargetID: "dplt_source",
		TargetApplicationID: "app_target", RelationType: "calls", Protocol: "http", Port: 9000,
	})
	if err != nil {
		t.Fatalf("create application-level target edge: %v", err)
	}
	if edge.SourceDeploymentTargetID != "dplt_source" || edge.TargetDeploymentTargetID != "" {
		t.Fatalf("edge targets = %#v", edge)
	}
}

func TestCheckServiceBindingReturnsTimestampAndDisabledStatus(t *testing.T) {
	repository := dependencyFixture()
	binding, err := NewService(repository).CreateServiceBinding(context.Background(), "prj_main", "usr_owner", validBindingInput())
	if err != nil {
		t.Fatalf("create service binding: %v", err)
	}
	binding.Enabled = false
	repository.bindings[binding.ID] = binding
	result, err := NewService(repository).CheckServiceBinding(context.Background(), "prj_main", binding.ID)
	if err != nil {
		t.Fatalf("check service binding: %v", err)
	}
	if result.CheckedAt.IsZero() || result.Status != "disabled" {
		t.Fatalf("check result = %#v", result)
	}
}

func TestProjectTopologyAggregatesBindingsAndManualEdgesAndWarnsOnCycle(t *testing.T) {
	repository := dependencyFixture()
	now := time.Now().UTC()
	repository.bindings["sbind_forward"] = model.ServiceBinding{
		ID: "sbind_forward", ProjectID: "prj_main", SourceApplicationID: "app_source", SourceDeploymentTargetID: "dplt_source",
		TargetApplicationID: "app_target", TargetDeploymentTargetID: "dplt_target", TargetPortName: "http", TargetPort: 9000,
		Protocol: "http", InjectionMode: "url", URLEnvVar: "API_URL", Enabled: true, UpdatedAt: now,
	}
	repository.edges["ptedge_reverse"] = model.ProjectTopologyEdge{
		ID: "ptedge_reverse", ProjectID: "prj_main", SourceApplicationID: "app_target", TargetApplicationID: "app_source", RelationType: "depends_on",
	}
	topology, err := NewService(repository).ProjectTopology(context.Background(), "prj_main", TopologyFilter{})
	if err != nil {
		t.Fatalf("project topology: %v", err)
	}
	if len(topology.Nodes) != 2 || len(topology.Edges) != 2 {
		t.Fatalf("topology nodes=%d edges=%d", len(topology.Nodes), len(topology.Edges))
	}
	if !reflect.DeepEqual(topology.Warnings, []TopologyWarning{{Code: CodeDependencyCycle}}) {
		t.Fatalf("warnings = %#v", topology.Warnings)
	}
}

func TestProjectTopologyKeepsDeclaredStateIndependentFromReleaseHistory(t *testing.T) {
	repository := dependencyFixture()
	now := time.Now().UTC()
	repository.bindings["sbind_pending"] = model.ServiceBinding{
		ID: "sbind_pending", ProjectID: "prj_main", SourceApplicationID: "app_source", SourceDeploymentTargetID: "dplt_source",
		TargetApplicationID: "app_target", TargetDeploymentTargetID: "dplt_target", TargetPortName: "http", TargetPort: 9000,
		Protocol: "http", InjectionMode: "url", URLEnvVar: "API_URL", Enabled: true, UpdatedAt: now,
	}
	topology, err := NewService(repository).ProjectTopology(context.Background(), "prj_main", TopologyFilter{})
	if err != nil {
		t.Fatalf("project topology: %v", err)
	}
	if len(topology.Edges) != 1 || topology.Edges[0].Status != "declared" {
		t.Fatalf("topology edges = %#v", topology.Edges)
	}
	topology, err = NewService(repository).ProjectTopology(context.Background(), "prj_main", TopologyFilter{})
	if err != nil {
		t.Fatalf("project topology repeated read: %v", err)
	}
	if topology.Edges[0].Status != "declared" {
		t.Fatalf("topology edge status = %q", topology.Edges[0].Status)
	}
}

func TestSortFieldsAreWhitelisted(t *testing.T) {
	if bindingSortColumns()["created_at desc; drop table users"] != "" {
		t.Fatal("unsafe binding sort field was accepted")
	}
	if edgeSortColumns()["relationType"] != "relation_type" {
		t.Fatal("expected relationType sort mapping")
	}
	if got := normalizedSortOrder("sideways"); got != "desc" {
		t.Fatalf("unexpected sort order %q", got)
	}
}
