package repository

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestImageRepositoryClaimedByOtherProject(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "image_repository_claim_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.DeploymentTarget{}, &model.BuildRun{})
		},
	})
	registry := model.ArtifactRegistry{ID: "reg_harbor", Endpoint: "https://harbor.example.com"}
	seeds := []any{
		&model.DeploymentTarget{
			ID: "dplt_owner", ProjectID: "prj_owner", ApplicationID: "app_owner", Name: "Prod", Stage: "prod",
			KubernetesName: "owner-prod", TargetRegistryID: registry.ID, TargetRepository: "team/owner-api",
		},
		&model.BuildRun{
			ID: "brn_owner", ProjectID: "prj_owner", ApplicationID: "app_owner", Status: "succeeded",
			TargetRegistryID: registry.ID, TargetRepository: "harbor.example.com/team/owner-worker",
		},
	}
	for _, seed := range seeds {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed %T: %v", seed, err)
		}
	}

	for _, test := range []struct {
		name       string
		projectID  string
		registry   model.ArtifactRegistry
		repository string
		want       bool
	}{
		{name: "deployment target of other project", projectID: "prj_other", registry: registry, repository: "team/owner-api", want: true},
		{name: "host-qualified reference of deployment target", projectID: "prj_other", registry: registry, repository: "harbor.example.com/team/owner-api", want: true},
		{name: "build run of other project", projectID: "prj_other", registry: registry, repository: "team/owner-worker", want: true},
		{name: "owner project keeps using it", projectID: "prj_owner", registry: registry, repository: "team/owner-api", want: false},
		{name: "unused repository", projectID: "prj_other", registry: registry, repository: "team/other-api", want: false},
		{name: "same path on another registry", projectID: "prj_other", registry: model.ArtifactRegistry{ID: "reg_other", Endpoint: "https://harbor.example.com"}, repository: "team/owner-api", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			claimed, err := ImageRepositoryClaimedByOtherProject(t.Context(), db, test.projectID, test.registry, test.repository)
			if err != nil {
				t.Fatalf("claim check: %v", err)
			}
			if claimed != test.want {
				t.Fatalf("claimed = %t, want %t", claimed, test.want)
			}
		})
	}
}
