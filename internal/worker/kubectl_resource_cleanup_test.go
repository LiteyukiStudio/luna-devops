package worker

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestKubectlCleanupClustersForProjectOnlyReturnsActiveBoundClusters(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "kubectl_cleanup_clusters_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{}, &model.KubeAccessBinding{})
		},
	})
	clusters := []model.RuntimeCluster{
		{ID: "clu_active", Name: "Active", Type: "kubernetes", DeleteStatus: "active"},
		{ID: "clu_deleting", Name: "Deleting", Type: "kubernetes", DeleteStatus: "deleting"},
		{ID: "clu_other", Name: "Other", Type: "kubernetes", DeleteStatus: "active"},
	}
	if err := db.Create(&clusters).Error; err != nil {
		t.Fatalf("create clusters: %v", err)
	}
	bindings := []model.KubeAccessBinding{
		{ID: "kbd_active", AccessTokenID: "tok_one", ProjectID: "prj_one", RuntimeClusterID: "clu_active"},
		{ID: "kbd_deleting", AccessTokenID: "tok_one", ProjectID: "prj_one", RuntimeClusterID: "clu_deleting"},
		{ID: "kbd_other", AccessTokenID: "tok_two", ProjectID: "prj_two", RuntimeClusterID: "clu_other"},
	}
	if err := db.Create(&bindings).Error; err != nil {
		t.Fatalf("create bindings: %v", err)
	}

	items, err := (&Runner{db: db}).kubectlCleanupClustersForProject(t.Context(), "prj_one")
	if err != nil {
		t.Fatalf("kubectlCleanupClustersForProject() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "clu_active" {
		t.Fatalf("cleanup clusters = %#v", items)
	}
}

func TestFinishProjectDeleteRemovesSoftDeleteOrphanBindings(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "kubectl_project_binding_cleanup_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.KubeAccessBinding{})
		},
	})
	project := model.Project{ID: "prj_delete", Identifier: "delete", Name: "Delete", DeleteStatus: "deleting"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := db.Create(&model.KubeAccessBinding{ID: "kbd_delete", AccessTokenID: "tok_one", ProjectID: project.ID, RuntimeClusterID: "clu_one"}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}

	if err := (&Runner{db: db}).finishProjectDelete(project); err != nil {
		t.Fatalf("finishProjectDelete() error = %v", err)
	}
	var count int64
	if err := db.Model(&model.KubeAccessBinding{}).Where("project_id = ?", project.ID).Count(&count).Error; err != nil {
		t.Fatalf("count bindings: %v", err)
	}
	if count != 0 {
		t.Fatalf("binding count = %d, want 0", count)
	}
}
