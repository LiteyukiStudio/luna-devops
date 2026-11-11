package runtimecluster

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

// activeDeleteStatuses 与 model.DeleteStatusActive 一致，供 SQL 过滤使用。
var activeDeleteStatuses = []string{"", "active", "delete_failed"}

// ServiceSharesOnCluster 返回提供方部署配置位于该集群、双方项目空间与部署配置都有效的共享（别名 s、t、pp、cp）。
// 部署配置未指定集群时使用默认集群。
func ServiceSharesOnCluster(ctx context.Context, db *gorm.DB, cluster model.RuntimeCluster) (*gorm.DB, error) {
	isDefault := false
	defaultCluster, err := ResolveForTarget(db.WithContext(ctx), "")
	switch {
	case err == nil:
		isDefault = defaultCluster.ID == cluster.ID
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}
	query := db.WithContext(ctx).Table("network_service_shares as s").
		Joins("join deployment_targets t on t.id = s.deployment_target_id and t.project_id = s.provider_project_id and t.deleted_at is null and t.delete_status in ?", activeDeleteStatuses).
		Joins("join projects pp on pp.id = s.provider_project_id and pp.deleted_at is null and pp.delete_status in ?", activeDeleteStatuses).
		Joins("join projects cp on cp.id = s.consumer_project_id and cp.deleted_at is null and cp.delete_status in ?", activeDeleteStatuses)
	if isDefault {
		query = query.Where("(t.cluster_id = ? or t.cluster_id = '')", cluster.ID)
	} else {
		query = query.Where("t.cluster_id = ?", cluster.ID)
	}
	return query, nil
}

// activeServiceShares 在 ServiceSharesOnCluster 基础上只保留已生效的共享：使用方已确认，且无需批准或已批准。
func activeServiceShares(ctx context.Context, db *gorm.DB, cluster model.RuntimeCluster) (*gorm.DB, error) {
	query, err := ServiceSharesOnCluster(ctx, db, cluster)
	if err != nil {
		return nil, err
	}
	return query.Where("s.accepted_at is not null and (s.approval_required = ? or s.approved_at is not null)", false), nil
}

// TargetRuntimeClusterID 返回部署配置实际使用的集群（未指定时为默认集群）。
func TargetRuntimeClusterID(db *gorm.DB, clusterID string) (string, error) {
	cluster, err := ResolveForTarget(db, clusterID)
	if err != nil {
		return "", err
	}
	return cluster.ID, nil
}

// ProjectHoldsEgressExceptions 判断项目空间在该集群是否持有出站例外授权。
func ProjectHoldsEgressExceptions(db *gorm.DB, clusterID string, projectID string) (bool, error) {
	var count int64
	err := db.Table("network_egress_exception_grants as g").
		Joins("join network_egress_exceptions e on e.id = g.exception_id").
		Where("e.runtime_cluster_id = ? and g.project_id = ?", clusterID, projectID).Count(&count).Error
	return count > 0, err
}

// sharedServicePolicies 读取项目空间在该集群作为提供方与使用方的已生效共享。
func sharedServicePolicies(ctx context.Context, db *gorm.DB, cluster model.RuntimeCluster, projectID string) (map[string][]string, []kubeprovider.SharedService, error) {
	var provided []struct {
		TargetID  string `gorm:"column:target_id"`
		Namespace string `gorm:"column:namespace"`
	}
	query, err := activeServiceShares(ctx, db, cluster)
	if err != nil {
		return nil, nil, err
	}
	if err := query.Select("t.id as target_id, cp.kubernetes_namespace as namespace").
		Where("s.provider_project_id = ?", projectID).Order("s.id asc").Scan(&provided).Error; err != nil {
		return nil, nil, err
	}
	ingress := map[string][]string{}
	for _, row := range provided {
		ingress[row.TargetID] = append(ingress[row.TargetID], row.Namespace)
	}
	var consumed []struct {
		TargetID  string `gorm:"column:target_id"`
		Namespace string `gorm:"column:namespace"`
	}
	query, err = activeServiceShares(ctx, db, cluster)
	if err != nil {
		return nil, nil, err
	}
	if err := query.Select("t.id as target_id, pp.kubernetes_namespace as namespace").
		Where("s.consumer_project_id = ?", projectID).Order("s.id asc").Scan(&consumed).Error; err != nil {
		return nil, nil, err
	}
	egress := make([]kubeprovider.SharedService, 0, len(consumed))
	for _, row := range consumed {
		egress = append(egress, kubeprovider.SharedService{Namespace: row.Namespace, TargetID: row.TargetID})
	}
	return ingress, egress, nil
}
