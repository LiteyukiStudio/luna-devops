package project

import (
	"context"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrIdentifierInvalid          = errors.New("project identifier is invalid")
	ErrIdentifierExists           = errors.New("project identifier already exists")
	ErrIdentifierDeleteInProgress = errors.New("project with this identifier is being deleted")
	ErrIdentifierDeleteFailed     = errors.New("project with this identifier failed to delete")
	ErrInputInvalid               = errors.New("project input is invalid")
)

type CreateInput struct {
	Identifier          string
	Name                string
	Description         string
	NamespaceStrategy   string
	MaxConcurrentBuilds int
	WebConsoleEnabled   *bool
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, userID string, input CreateInput) (model.Project, error) {
	input.Identifier = strings.TrimSpace(input.Identifier)
	if err := resourceidentifier.Validate(input.Identifier, resourceidentifier.ProjectMinLength, resourceidentifier.ProjectMaxLength); err != nil {
		return model.Project{}, errors.Join(ErrIdentifierInvalid, err)
	}
	if strings.TrimSpace(input.Name) == "" {
		return model.Project{}, ErrInputInvalid
	}

	project := model.Project{
		ID:                  id.New("prj"),
		Identifier:          input.Identifier,
		KubernetesNamespace: resourceidentifier.ProjectNamespace(input.Identifier),
		Name:                input.Name,
		Description:         input.Description,
		NamespaceStrategy:   input.NamespaceStrategy,
		MaxConcurrentBuilds: input.MaxConcurrentBuilds,
		WebConsoleEnabled:   true,
		BillingOwnerUserID:  userID,
	}
	if project.NamespaceStrategy == "" {
		project.NamespaceStrategy = "project"
	}
	if project.MaxConcurrentBuilds <= 0 {
		project.MaxConcurrentBuilds = 2
	}
	if input.WebConsoleEnabled != nil {
		project.WebConsoleEnabled = *input.WebConsoleEnabled
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Project
		if err := tx.Select("id", "delete_status").Where("identifier = ?", project.Identifier).First(&existing).Error; err == nil {
			return projectIdentifierConflict(existing.DeleteStatus)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "identifier"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
			DoNothing:   true,
		}).Create(&project)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrIdentifierExists
		}
		return tx.Create(&model.ProjectMember{
			ID: id.New("mem"), ProjectID: project.ID, UserID: userID, Role: authz.ProjectRoleOwner,
		}).Error
	})
	return project, err
}

func projectIdentifierConflict(deleteStatus string) error {
	switch strings.TrimSpace(deleteStatus) {
	case "deleting":
		return ErrIdentifierDeleteInProgress
	case "delete_failed":
		return ErrIdentifierDeleteFailed
	default:
		return ErrIdentifierExists
	}
}
