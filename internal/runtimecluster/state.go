package runtimecluster

import (
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const (
	DeleteStatusActive       = "active"
	DeleteStatusDeleting     = "deleting"
	DeleteStatusDeleteFailed = "delete_failed"
	DeleteStatusDeleted      = "deleted"
)

func NormalizeDeleteStatus(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func IsActive(cluster model.RuntimeCluster) bool {
	return NormalizeDeleteStatus(cluster.DeleteStatus) == DeleteStatusActive && !cluster.DeletedAt.Valid
}

// ActiveScope is the only reusable availability predicate for runtime
// clusters outside management and cleanup flows.
func ActiveScope(db *gorm.DB) *gorm.DB {
	return db.Where("delete_status = ?", DeleteStatusActive)
}
