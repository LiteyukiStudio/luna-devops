package worker

import (
	"context"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type kubectlGatewayTaskEnqueuer interface {
	EnqueueKubectlGateway(context.Context, tasks.KubectlGatewayPayload) (*asynq.TaskInfo, error)
}

func kubectlGatewayPeriodicTaskSpec() (periodicTaskSpec, error) {
	task := tasks.NewKubectlGatewaySweepTask()
	policy := tasks.KubectlGatewaySweepEnqueuePolicy()
	return periodicTaskSpec{
		Cron:      "@every 5m",
		Task:      task,
		Queue:     policy.Queue,
		Timeout:   policy.Timeout,
		MaxRetry:  policy.MaxRetry,
		Retention: policy.Retention,
		Unique:    policy.Unique,
	}, nil
}

func (r *Runner) handleKubectlGatewaySweep(ctx context.Context, _ *asynq.Task) error {
	return r.enqueueScheduledKubectlGateways(ctx)
}

func (r *Runner) enqueueScheduledKubectlGateways(ctx context.Context) error {
	if r == nil || r.db == nil || r.taskClient == nil {
		return nil
	}
	return enqueueScheduledKubectlGatewaysWith(ctx, r.db, r.taskClient)
}

func enqueueScheduledKubectlGatewaysWith(ctx context.Context, db *gorm.DB, enqueuer kubectlGatewayTaskEnqueuer) error {
	if db == nil || enqueuer == nil {
		return nil
	}
	var clusters []model.RuntimeCluster
	if err := runtimecluster.ActiveScope(db.WithContext(ctx)).Where("type in ?", []string{"kubernetes", "k3s"}).Order("created_at asc").Find(&clusters).Error; err != nil {
		return err
	}
	for _, cluster := range clusters {
		if _, err := enqueuer.EnqueueKubectlGateway(ctx, tasks.KubectlGatewayPayload{ClusterID: strings.TrimSpace(cluster.ID)}); err != nil {
			return err
		}
	}
	return nil
}
