package worker

import (
	"context"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func TestVolumeTransferBillingReconcilerSettlesTerminalBytesOnce(t *testing.T) {
	db := openVolumeTransferBillingWorkerTestDB(t)
	if err := db.AutoMigrate(
		&model.Project{}, &model.ProjectMember{}, &model.UserWallet{}, &model.BillingRateRule{},
		&model.BillingUsageRecord{}, &model.BillingLedgerEntry{}, &model.VolumeTransfer{},
	); err != nil {
		t.Fatalf("migrate worker transfer billing tables: %v", err)
	}
	project := model.Project{
		ID: "prj_worker_transfer", Identifier: "worker-transfer", Name: "Worker transfer", NamespaceStrategy: "project",
		BillingOwnerUserID: "usr_worker_transfer", DeleteStatus: "active",
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("seed worker transfer project: %v", err)
	}
	if err := db.Create(&model.UserWallet{
		ID: "wlt_worker_transfer", UserID: project.BillingOwnerUserID, BalanceCredits: decimal.NewFromInt(100),
	}).Error; err != nil {
		t.Fatalf("seed worker transfer wallet: %v", err)
	}
	if err := db.Create(&model.BillingRateRule{
		ID: "brte_worker_transfer", Meter: billing.MeterStorageTransferGiB, Unit: "gib",
		CreditsPerUnit: decimal.NewFromInt(2), Enabled: true, Description: "Volume transfer bytes",
	}).Error; err != nil {
		t.Fatalf("seed worker transfer rate: %v", err)
	}
	now := time.Now().UTC()
	finishedAt := now.Add(-time.Second)
	transfers := []model.VolumeTransfer{
		workerBillingTransfer("vtx_worker_succeeded", project.ID, model.VolumeTransferStateSucceeded, 1024*1024*1024, now, &finishedAt),
		workerBillingTransfer("vtx_worker_failed", project.ID, model.VolumeTransferStateFailed, 512*1024*1024, now, &finishedAt),
		workerBillingTransfer("vtx_worker_running", project.ID, model.VolumeTransferStateRunning, 1024*1024*1024, now, nil),
		workerBillingTransfer("vtx_worker_empty", project.ID, model.VolumeTransferStateCancelled, 0, now, &finishedAt),
	}
	if err := db.Create(&transfers).Error; err != nil {
		t.Fatalf("seed worker volume transfers: %v", err)
	}
	runner := &Runner{db: db}
	service := billing.Service{DB: db}
	if err := runner.settleVolumeTransferUsage(context.Background(), service, now); err != nil {
		t.Fatalf("settle terminal transfer usage: %v", err)
	}
	if err := runner.settleVolumeTransferUsage(context.Background(), service, now.Add(time.Minute)); err != nil {
		t.Fatalf("repeat terminal transfer usage settlement: %v", err)
	}
	var usage []model.BillingUsageRecord
	if err := db.Where("meter = ? AND resource_type = ?", billing.MeterStorageTransferGiB, billing.ResourceTypeTransfer).
		Order("resource_id ASC").Find(&usage).Error; err != nil {
		t.Fatalf("read worker transfer usage: %v", err)
	}
	if len(usage) != 2 || usage[0].ResourceID != "vtx_worker_failed" || usage[1].ResourceID != "vtx_worker_succeeded" {
		t.Fatalf("worker transfer usage = %#v", usage)
	}
	var ledgerCount int64
	if err := db.Model(&model.BillingLedgerEntry{}).Where("reason = ?", billing.ReasonTransferUsage).Count(&ledgerCount).Error; err != nil {
		t.Fatalf("count worker transfer ledger entries: %v", err)
	}
	if ledgerCount != 2 {
		t.Fatalf("worker transfer ledger count = %d, want 2", ledgerCount)
	}
	var wallet model.UserWallet
	if err := db.First(&wallet, "user_id = ?", project.BillingOwnerUserID).Error; err != nil {
		t.Fatalf("read worker transfer wallet: %v", err)
	}
	if !wallet.BalanceCredits.Equal(decimal.NewFromInt(97)) {
		t.Fatalf("worker transfer wallet balance = %s, want 97", wallet.BalanceCredits)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.settleVolumeTransferUsage(cancelled, service, now); err != context.Canceled {
		t.Fatalf("cancelled transfer settlement error = %v", err)
	}
}

func workerBillingTransfer(id, projectID, state string, transferredBytes int64, now time.Time, finishedAt *time.Time) model.VolumeTransfer {
	return model.VolumeTransfer{
		ID: id, ProjectID: projectID, ProjectVolumeID: "pvol_worker", Direction: model.VolumeTransferDirectionExport,
		Format: model.VolumeTransferFormatTarGZ, ConsistencyMode: model.VolumeTransferConsistencySnapshot,
		State: state, ObjectKey: "worker/" + id, ExpectedBytes: transferredBytes, TransferredBytes: transferredBytes,
		ActorID: "usr_worker_transfer", ExpiresAt: now.Add(time.Hour), CreatedAt: now.Add(-time.Minute), UpdatedAt: now, FinishedAt: finishedAt,
	}
}

func openVolumeTransferBillingWorkerTestDB(t *testing.T) *gorm.DB {
	return testdb.Open(t, testdb.Options{SchemaPrefix: "worker_transfer_billing_test"})
}
