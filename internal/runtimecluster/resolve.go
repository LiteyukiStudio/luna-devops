package runtimecluster

import (
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const ScopeGlobal = "global"

// ResolveForTarget is the only resolver for the runtime cluster a deployment
// target (or another workload owner storing a cluster reference) runs on.
//
// A non-empty clusterID must reference an active cluster. An empty clusterID
// means the platform default: the active global cluster marked as default, or
// the earliest created active global cluster when none is marked. Project and
// user scoped clusters are never chosen implicitly; they must be selected
// explicitly. It returns gorm.ErrRecordNotFound when no cluster applies.
//
// Each call issues exactly one query on a fresh statement, so callers may pass
// any *gorm.DB without leaking conditions between lookups.
func ResolveForTarget(db *gorm.DB, clusterID string) (model.RuntimeCluster, error) {
	var cluster model.RuntimeCluster
	query := ActiveScope(db.Session(&gorm.Session{NewDB: true}).Model(&model.RuntimeCluster{}))
	if clusterID = strings.TrimSpace(clusterID); clusterID != "" {
		err := query.Where("id = ?", clusterID).Take(&cluster).Error
		return cluster, err
	}
	err := query.
		Where("scope = ?", ScopeGlobal).
		Order("is_default desc").
		Order("created_at asc").
		Order("id asc").
		Take(&cluster).Error
	return cluster, err
}
