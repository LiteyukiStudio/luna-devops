package worker

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/retention"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func newAutomaticRetentionRunner(db *gorm.DB) func(context.Context, time.Time) error {
	return func(ctx context.Context, now time.Time) error {
		_, err := retention.NewService(db).RunAutomatic(ctx, now)
		return err
	}
}

func (r *Runner) handleRetentionRun(ctx context.Context, task *asynq.Task) error {
	return workerStage(ctx, "retention.run", func(stageCtx context.Context) error {
		return r.runAutomaticRetention(stageCtx, time.Now())
	})
}
