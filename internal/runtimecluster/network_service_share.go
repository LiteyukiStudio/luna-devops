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

// shareNeedsApprovalSQL 判断共享在某个集群上是否仍需批准：提供方当前在该集群持有出站例外，
// 且没有针对该集群的批准。按当前状态判断，部署配置换集群或之后才获得例外都会重新要求批准。
const shareNeedsApprovalSQL = `exists (select 1 from network_egress_exception_grants g join network_egress_exceptions e on e.id = g.exception_id
	where e.runtime_cluster_id = ? and g.project_id = s.provider_project_id) and s.approved_runtime_cluster_id <> ?`

// SharesNeedingApproval 在 ServiceSharesOnCluster 结果上只保留仍需该集群维护者批准的共享。
func SharesNeedingApproval(query *gorm.DB, clusterID string) *gorm.DB {
	return query.Where(shareNeedsApprovalSQL, clusterID, clusterID)
}

// activeServiceShares 在 ServiceSharesOnCluster 基础上只保留已生效的共享：使用方已确认，且不需要或已获该集群批准。
func activeServiceShares(ctx context.Context, db *gorm.DB, cluster model.RuntimeCluster) (*gorm.DB, error) {
	query, err := ServiceSharesOnCluster(ctx, db, cluster)
	if err != nil {
		return nil, err
	}
	return query.Where("s.accepted_at is not null").Where("not ("+shareNeedsApprovalSQL+")", cluster.ID, cluster.ID), nil
}

// ShareApprovalState 返回共享提供方部署配置当前所在集群，以及在该集群上是否仍需批准。
// 部署配置没有可用集群时返回空集群且不需批准（共享不会在任何集群渲染）。
func ShareApprovalState(db *gorm.DB, share model.NetworkServiceShare, targetClusterID string) (string, bool, error) {
	clusterID, err := TargetRuntimeClusterID(db, targetClusterID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if share.ApprovedRuntimeClusterID == clusterID {
		return clusterID, false, nil
	}
	holds, err := ProjectHoldsEgressExceptions(db, clusterID, share.ProviderProjectID)
	return clusterID, holds, err
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

// ProjectCanDeployToCluster 判断项目空间能否在该集群部署：全局集群对所有有效项目空间可用，项目级集群看绑定，
// 个人集群要求集群所有者是项目成员。出站例外授权与跨项目共享都以此为前提。
func ProjectCanDeployToCluster(db *gorm.DB, cluster model.RuntimeCluster, projectID string) (bool, error) {
	var project model.Project
	if err := db.Select("id", "delete_status").Where("id = ?", projectID).Take(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if !model.DeleteStatusActive(project.DeleteStatus) {
		return false, nil
	}
	var count int64
	switch cluster.Scope {
	case ScopeGlobal:
		return true, nil
	case "project":
		err := db.Model(&model.ScopedResourceProjectBinding{}).
			Where("resource_type = ? and resource_id = ? and project_id = ?", "runtime_cluster", cluster.ID, projectID).Count(&count).Error
		return count > 0, err
	case "user":
		err := db.Model(&model.ProjectMember{}).Where("project_id = ? and user_id = ?", projectID, cluster.OwnerRef).Count(&count).Error
		return count > 0, err
	}
	return false, nil
}

// SharePeerProjectIDs 返回满足条件的共享涉及的全部项目空间（提供方与使用方），供共享变化后重新下发双方策略。
func SharePeerProjectIDs(db *gorm.DB, condition string, args ...any) ([]string, error) {
	var rows []struct {
		ProviderProjectID string `gorm:"column:provider_project_id"`
		ConsumerProjectID string `gorm:"column:consumer_project_id"`
	}
	if err := db.Model(&model.NetworkServiceShare{}).Select("provider_project_id", "consumer_project_id").Where(condition, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	result := []string{}
	for _, row := range rows {
		for _, projectID := range []string{row.ProviderProjectID, row.ConsumerProjectID} {
			if !seen[projectID] {
				seen[projectID] = true
				result = append(result, projectID)
			}
		}
	}
	return result, nil
}
