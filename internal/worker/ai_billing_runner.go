package worker

import (
	"context"
	"log/slog"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
)

func (r *Runner) handleBillingAI(ctx context.Context, _ *asynq.Task) error {
	settled, err := workerStageValue(ctx, "billing.settle_ai_usage", func(stageCtx context.Context) (int, error) {
		return (billing.Service{DB: r.db.WithContext(stageCtx)}).SettlePendingAIModelUsage(stageCtx, 200)
	})
	if err != nil {
		return err
	}
	telemetry.Logger().InfoContext(ctx, "AI token usage settled",
		slog.String("event.name", "billing.ai_usage.settled"),
		slog.Int("billing.usage.count", settled),
	)
	return nil
}
