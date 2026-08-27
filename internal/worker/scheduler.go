package worker

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
)

type periodicTaskSpec struct {
	Cron      string
	Task      *asynq.Task
	Queue     string
	Timeout   time.Duration
	MaxRetry  int
	Retention time.Duration
	Unique    time.Duration
}

func periodicTaskSpecs() ([]periodicTaskSpec, error) {
	gitRefreshTask, err := tasks.NewGitAccountRefreshTask(tasks.GitAccountRefreshPayload{ActorID: "system"})
	if err != nil {
		return nil, err
	}
	volumeReconcileTask, err := tasks.NewVolumeReconcileTask(tasks.VolumeReconcilePayload{})
	if err != nil {
		return nil, err
	}
	volumeTransferCleanupTask, err := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{})
	if err != nil {
		return nil, err
	}
	return []periodicTaskSpec{
		{Cron: "@every 5m", Task: gitRefreshTask, Queue: tasks.QueueLight, Timeout: 10 * time.Minute},
		periodicVolumeTaskSpec("@every 5m", volumeReconcileTask),
		periodicVolumeTaskSpec("@every 15m", volumeTransferCleanupTask),
		{Cron: "@every 1m", Task: asynq.NewTask(tasks.TypeSyncStatus, []byte("{}")), Queue: tasks.QueueLight, Timeout: 5 * time.Minute},
		{Cron: "@every 1m", Task: asynq.NewTask(tasks.TypeBillingAI, []byte("{}")), Queue: tasks.QueueLight, Timeout: time.Minute},
		{Cron: "@every 1m", Task: asynq.NewTask(tasks.TypeBillingRuntime, []byte("{}")), Queue: tasks.QueueLight, Timeout: 5 * time.Minute},
		{Cron: "@every 24h", Task: asynq.NewTask(tasks.TypeRetentionRun, []byte("{}")), Queue: tasks.QueueLight, Timeout: 30 * time.Minute},
	}, nil
}

func periodicVolumeTaskSpec(cron string, task *asynq.Task) periodicTaskSpec {
	policy := tasks.PolicyForType(task.Type())
	return periodicTaskSpec{
		Cron: cron, Task: task, Queue: policy.Queue, Timeout: policy.Timeout,
		MaxRetry: policy.MaxRetry, Retention: policy.Retention, Unique: policy.Unique,
	}
}

func startScheduler(redisAddr string) (*asynq.Scheduler, error) {
	return startSchedulerWithRedis(redisconfig.Options{Addr: redisAddr})
}

func startSchedulerWithRedis(options redisconfig.Options) (*asynq.Scheduler, error) {
	scheduler := asynq.NewScheduler(options.Asynq(), &asynq.SchedulerOpts{
		Logger:   asynqTelemetryLogger{},
		LogLevel: asynq.WarnLevel,
	})
	specs, err := periodicTaskSpecs()
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		options := periodicTaskOptions(spec)
		if _, err := scheduler.Register(spec.Cron, spec.Task, options...); err != nil {
			return nil, err
		}
	}
	go func() {
		if err := scheduler.Run(); err != nil {
			telemetry.LogError(context.Background(), "Worker scheduler stopped",
				"worker.scheduler.stopped", "worker.scheduler.run", "worker.scheduler.failed", err)
		}
	}()
	return scheduler, nil
}

func periodicTaskOptions(spec periodicTaskSpec) []asynq.Option {
	options := []asynq.Option{
		asynq.Queue(spec.Queue),
		asynq.Timeout(spec.Timeout),
		asynq.MaxRetry(spec.MaxRetry),
	}
	if spec.Retention > 0 {
		options = append(options, asynq.Retention(spec.Retention))
	}
	if spec.Unique > 0 {
		options = append(options, asynq.Unique(spec.Unique))
	}
	return options
}
