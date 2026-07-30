package dependency

import (
	"context"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

type ListOptions struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

func (options ListOptions) offset() int {
	if options.Page <= 1 {
		return 0
	}
	return (options.Page - 1) * options.PageSize
}

type Repository interface {
	Application(context.Context, string) (model.Application, error)
	DeploymentTarget(context.Context, string) (model.DeploymentTarget, error)
	Applications(context.Context, string) ([]model.Application, error)
	DeploymentTargets(context.Context, string) ([]model.DeploymentTarget, error)
	ServiceBinding(context.Context, string, string) (model.ServiceBinding, error)
	ServiceBindings(context.Context, string) ([]model.ServiceBinding, error)
	ListServiceBindings(context.Context, string, ListOptions) ([]model.ServiceBinding, int64, error)
	ConflictingServiceBinding(context.Context, string, string, []string) (bool, error)
	CreateServiceBinding(context.Context, *model.ServiceBinding) error
	UpdateServiceBinding(context.Context, *model.ServiceBinding) error
	DeleteServiceBinding(context.Context, *model.ServiceBinding) error
	TopologyEdge(context.Context, string, string) (model.ProjectTopologyEdge, error)
	TopologyEdges(context.Context, string) ([]model.ProjectTopologyEdge, error)
	ListTopologyEdges(context.Context, string, ListOptions) ([]model.ProjectTopologyEdge, int64, error)
	DuplicateTopologyEdge(context.Context, model.ProjectTopologyEdge) (bool, error)
	CreateTopologyEdge(context.Context, *model.ProjectTopologyEdge) error
	UpdateTopologyEdge(context.Context, *model.ProjectTopologyEdge) error
	DeleteTopologyEdge(context.Context, *model.ProjectTopologyEdge) error
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (repository *GormRepository) Application(ctx context.Context, id string) (model.Application, error) {
	var application model.Application
	err := repository.db.WithContext(ctx).First(&application, "id = ?", id).Error
	return application, err
}

func (repository *GormRepository) DeploymentTarget(ctx context.Context, id string) (model.DeploymentTarget, error) {
	var target model.DeploymentTarget
	err := repository.db.WithContext(ctx).First(&target, "id = ?", id).Error
	return target, err
}

func (repository *GormRepository) Applications(ctx context.Context, projectID string) ([]model.Application, error) {
	var applications []model.Application
	err := repository.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at asc, id asc").Find(&applications).Error
	return applications, err
}

func (repository *GormRepository) DeploymentTargets(ctx context.Context, projectID string) ([]model.DeploymentTarget, error) {
	var targets []model.DeploymentTarget
	err := repository.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at asc, id asc").Find(&targets).Error
	return targets, err
}

func (repository *GormRepository) ServiceBinding(ctx context.Context, projectID, id string) (model.ServiceBinding, error) {
	var binding model.ServiceBinding
	err := repository.db.WithContext(ctx).First(&binding, "id = ? and project_id = ?", id, projectID).Error
	return binding, err
}

func (repository *GormRepository) ServiceBindings(ctx context.Context, projectID string) ([]model.ServiceBinding, error) {
	var bindings []model.ServiceBinding
	err := repository.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at asc, id asc").Find(&bindings).Error
	return bindings, err
}

func (repository *GormRepository) ListServiceBindings(ctx context.Context, projectID string, options ListOptions) ([]model.ServiceBinding, int64, error) {
	query := repository.db.WithContext(ctx).Model(&model.ServiceBinding{}).Where("project_id = ?", projectID)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var bindings []model.ServiceBinding
	column := bindingSortColumns()[options.SortBy]
	if column == "" {
		column = "created_at"
	}
	err := query.Order(column + " " + normalizedSortOrder(options.SortOrder) + ", id asc").Limit(options.PageSize).Offset(options.offset()).Find(&bindings).Error
	return bindings, total, err
}

func (repository *GormRepository) ConflictingServiceBinding(ctx context.Context, sourceTargetID, excludedID string, envKeys []string) (bool, error) {
	if len(envKeys) == 0 {
		return false, nil
	}
	query := repository.db.WithContext(ctx).Model(&model.ServiceBinding{}).
		Where("source_deployment_target_id = ?", sourceTargetID).
		Where("url_env_var in ? or host_env_var in ? or port_env_var in ?", envKeys, envKeys, envKeys)
	if excludedID != "" {
		query = query.Where("id <> ?", excludedID)
	}
	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

func (repository *GormRepository) CreateServiceBinding(ctx context.Context, binding *model.ServiceBinding) error {
	return repository.db.WithContext(ctx).Create(binding).Error
}

func (repository *GormRepository) UpdateServiceBinding(ctx context.Context, binding *model.ServiceBinding) error {
	return repository.db.WithContext(ctx).Save(binding).Error
}

func (repository *GormRepository) DeleteServiceBinding(ctx context.Context, binding *model.ServiceBinding) error {
	return repository.db.WithContext(ctx).Delete(binding).Error
}

func (repository *GormRepository) TopologyEdge(ctx context.Context, projectID, id string) (model.ProjectTopologyEdge, error) {
	var edge model.ProjectTopologyEdge
	err := repository.db.WithContext(ctx).First(&edge, "id = ? and project_id = ?", id, projectID).Error
	return edge, err
}

func (repository *GormRepository) TopologyEdges(ctx context.Context, projectID string) ([]model.ProjectTopologyEdge, error) {
	var edges []model.ProjectTopologyEdge
	err := repository.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at asc, id asc").Find(&edges).Error
	return edges, err
}

func (repository *GormRepository) ListTopologyEdges(ctx context.Context, projectID string, options ListOptions) ([]model.ProjectTopologyEdge, int64, error) {
	query := repository.db.WithContext(ctx).Model(&model.ProjectTopologyEdge{}).Where("project_id = ?", projectID)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var edges []model.ProjectTopologyEdge
	column := edgeSortColumns()[options.SortBy]
	if column == "" {
		column = "created_at"
	}
	err := query.Order(column + " " + normalizedSortOrder(options.SortOrder) + ", id asc").Limit(options.PageSize).Offset(options.offset()).Find(&edges).Error
	return edges, total, err
}

func (repository *GormRepository) DuplicateTopologyEdge(ctx context.Context, edge model.ProjectTopologyEdge) (bool, error) {
	query := repository.db.WithContext(ctx).Model(&model.ProjectTopologyEdge{}).Where(
		"project_id = ? and source_application_id = ? and source_deployment_target_id = ? and target_application_id = ? and target_deployment_target_id = ? and relation_type = ? and protocol = ? and port = ?",
		edge.ProjectID, edge.SourceApplicationID, edge.SourceDeploymentTargetID, edge.TargetApplicationID, edge.TargetDeploymentTargetID, edge.RelationType, edge.Protocol, edge.Port,
	)
	if edge.ID != "" {
		query = query.Where("id <> ?", edge.ID)
	}
	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

func (repository *GormRepository) CreateTopologyEdge(ctx context.Context, edge *model.ProjectTopologyEdge) error {
	return repository.db.WithContext(ctx).Create(edge).Error
}

func (repository *GormRepository) UpdateTopologyEdge(ctx context.Context, edge *model.ProjectTopologyEdge) error {
	return repository.db.WithContext(ctx).Save(edge).Error
}

func (repository *GormRepository) DeleteTopologyEdge(ctx context.Context, edge *model.ProjectTopologyEdge) error {
	return repository.db.WithContext(ctx).Delete(edge).Error
}

func bindingSortColumns() map[string]string {
	return map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"protocol":  "protocol",
		"enabled":   "enabled",
	}
}

func edgeSortColumns() map[string]string {
	return map[string]string{
		"createdAt":    "created_at",
		"updatedAt":    "updated_at",
		"relationType": "relation_type",
		"protocol":     "protocol",
	}
}

func normalizedSortOrder(order string) string {
	if strings.EqualFold(strings.TrimSpace(order), "asc") {
		return "asc"
	}
	return "desc"
}

func repositoryError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainError(CodeNotFound, "dependency resource not found")
	}
	return err
}
