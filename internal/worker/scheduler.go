package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
)

type periodicTaskSpec struct {
	Cron    string
	Task    *asynq.Task
	Queue   string
	Timeout time.Duration
}

func periodicTaskSpecs() ([]periodicTaskSpec, error) {
	gitRefreshTask, err := tasks.NewGitAccountRefreshTask(tasks.GitAccountRefreshPayload{ActorID: "system"})
	if err != nil {
		return nil, err
	}
	return []periodicTaskSpec{
		{Cron: "@every 5m", Task: gitRefreshTask, Queue: tasks.QueueLight, Timeout: 10 * time.Minute},
		{Cron: "@every 1m", Task: asynq.NewTask(tasks.TypeSyncStatus, []byte("{}")), Queue: tasks.QueueLight, Timeout: 5 * time.Minute},
		{Cron: "@every 1m", Task: asynq.NewTask(tasks.TypeBillingAI, []byte("{}")), Queue: tasks.QueueLight, Timeout: time.Minute},
		{Cron: "@every 10m", Task: asynq.NewTask(tasks.TypeBillingRuntime, []byte("{}")), Queue: tasks.QueueLight, Timeout: 5 * time.Minute},
		{Cron: "@every 24h", Task: asynq.NewTask(tasks.TypeRetentionRun, []byte("{}")), Queue: tasks.QueueLight, Timeout: 30 * time.Minute},
	}, nil
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
		if _, err := scheduler.Register(spec.Cron, spec.Task, asynq.Queue(spec.Queue), asynq.Timeout(spec.Timeout)); err != nil {
			return nil, err
		}
	}
	go func() {
		if err := scheduler.Run(); err != nil {
			telemetry.Logger().ErrorContext(context.Background(), "worker scheduler stopped",
				slog.String("event.name", "worker.scheduler.stopped"),
				slog.String("error.type", telemetry.ErrorType(err)),
			)
		}
	}()
	return scheduler, nil
}
