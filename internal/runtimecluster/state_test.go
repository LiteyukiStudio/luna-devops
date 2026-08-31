package runtimecluster

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

func TestIsActive(t *testing.T) {
	if !IsActive(model.RuntimeCluster{}) || !IsActive(model.RuntimeCluster{DeleteStatus: DeleteStatusActive}) {
		t.Fatal("active and legacy empty clusters must be active")
	}
	for _, status := range []string{DeleteStatusDeleting, DeleteStatusDeleteFailed, DeleteStatusDeleted} {
		if IsActive(model.RuntimeCluster{DeleteStatus: status}) {
			t.Fatalf("cluster status %q was active", status)
		}
	}
	deleted := model.RuntimeCluster{DeleteStatus: DeleteStatusActive, DeletedAt: gorm.DeletedAt{Valid: true}}
	if IsActive(deleted) {
		t.Fatal("soft-deleted cluster was active")
	}
}
