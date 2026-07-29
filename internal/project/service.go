package project

import (
	"context"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"gorm.io/gorm"
)

var (
	ErrIdentifierInvalid = errors.New("project identifier is invalid")
	ErrIdentifierExists  = errors.New("project identifier already exists")
	ErrInputInvalid      = errors.New("project input is invalid")
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
		ID:                  resourceidentifier.ProjectID(input.Identifier),
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
		var count int64
		if err := tx.Model(&model.Project{}).Where("identifier = ? and deleted_at is null", project.Identifier).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrIdentifierExists
		}
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		return tx.Create(&model.ProjectMember{
			ID: id.New("mem"), ProjectID: project.ID, UserID: userID, Role: "owner",
		}).Error
	})
	return project, err
}
