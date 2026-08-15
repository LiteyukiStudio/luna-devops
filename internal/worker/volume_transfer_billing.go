package worker

import (
	"context"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
)

const volumeTransferBillingBatchSize = 100

// settleVolumeTransferUsage reconciles terminal transfers against the billing
// ledger. The NOT EXISTS query is only an optimization: the billing service's
// unique resource/meter key remains the cross-replica idempotency authority.
func (r *Runner) settleVolumeTransferUsage(ctx context.Context, service billing.Service, now time.Time) error {
	if r == nil || r.db == nil {
		return nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		items := make([]model.VolumeTransfer, 0, volumeTransferBillingBatchSize)
		err := r.db.WithContext(ctx).
			Where("state IN ? AND transferred_bytes > 0", []string{
				model.VolumeTransferStateSucceeded,
				model.VolumeTransferStateFailed,
				model.VolumeTransferStateCancelled,
				model.VolumeTransferStateExpired,
			}).
			Where(`NOT EXISTS (
				SELECT 1 FROM billing_usage_records
				WHERE billing_usage_records.meter = ?
				  AND billing_usage_records.resource_type = ?
				  AND billing_usage_records.resource_id = volume_transfers.id
			)`, billing.MeterStorageTransferGiB, billing.ResourceTypeTransfer).
			Order("id ASC").
			Limit(volumeTransferBillingBatchSize).
			Find(&items).Error
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for _, transfer := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			err := service.SettleVolumeTransferUsage(ctx, billing.VolumeTransferUsageInput{
				Transfer: transfer, SettledAt: now, ActorID: "system",
			})
			if err != nil && !errors.Is(err, billing.ErrAlreadySettled) {
				return err
			}
		}
	}
}
