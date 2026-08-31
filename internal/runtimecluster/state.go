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

// NormalizeDeleteStatus treats an empty in-memory value as active so legacy
// fixtures match the database default. Persisted rows are backfilled by the
// versioned migration and never rely on this compatibility rule.
func NormalizeDeleteStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DeleteStatusActive
	}
	return value
}

func IsActive(cluster model.RuntimeCluster) bool {
	return NormalizeDeleteStatus(cluster.DeleteStatus) == DeleteStatusActive && !cluster.DeletedAt.Valid
}

// ActiveScope is the only reusable availability predicate for runtime
// clusters outside management and cleanup flows.
func ActiveScope(db *gorm.DB) *gorm.DB {
	if db == nil {
		return nil
	}
	return db.Where("delete_status = ?", DeleteStatusActive)
}
