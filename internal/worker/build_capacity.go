package worker

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

const (
	defaultClusterBuildConcurrency = 4
	defaultProjectBuildConcurrency = 2
	buildCapacityRetryDelay        = 10 * time.Second
)

var errBuildCapacityUnavailable = errors.New("build capacity unavailable")

func (r *Runner) ensureBuildCapacity(project model.Project, cluster model.RuntimeCluster, environment model.Environment) error {
	projectLimit := normalizeBuildConcurrency(project.MaxConcurrentBuilds, defaultProjectBuildConcurrency)
	clusterLimit := normalizeBuildConcurrency(cluster.MaxConcurrentBuilds, defaultClusterBuildConcurrency)

	projectRunning, err := r.runningProjectBuilds(project.ID)
	if err != nil {
		return err
	}
	if projectRunning >= int64(projectLimit) {
		return fmt.Errorf("%w: project %s running builds %d/%d", errBuildCapacityUnavailable, project.ID, projectRunning, projectLimit)
	}

	clusterRunning, err := r.runningClusterBuilds(cluster.ID, environment)
	if err != nil {
		return err
	}
	if clusterRunning >= int64(clusterLimit) {
		return fmt.Errorf("%w: cluster %s running builds %d/%d", errBuildCapacityUnavailable, cluster.ID, clusterRunning, clusterLimit)
	}
	return nil
}

func (r *Runner) runningProjectBuilds(projectID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.BuildJob{}).
		Where("project_id = ? and status = ?", projectID, "running").
		Count(&count).Error
	return count, err
}

func (r *Runner) runningClusterBuilds(clusterID string, environment model.Environment) (int64, error) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return 0, nil
	}
	query := r.db.Table("build_jobs").
		Joins("join build_runs on build_runs.id = build_jobs.build_run_id and build_runs.project_id = build_jobs.project_id").
		Joins("join deployment_targets on deployment_targets.id = build_runs.deployment_target_id and deployment_targets.project_id = build_runs.project_id and deployment_targets.application_id = build_runs.application_id").
		Where("build_jobs.status = ?", "running")
	if strings.TrimSpace(environment.ClusterID) == "" {
		query = query.Where("deployment_targets.cluster_id = '' or deployment_targets.cluster_id = ?", clusterID)
	} else {
		query = query.Where("deployment_targets.cluster_id = ?", clusterID)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

func normalizeBuildConcurrency(value int, defaultValue int) int {
	if value > 0 {
		return value
	}
	return defaultValue
}

func buildCapacityMessage(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
