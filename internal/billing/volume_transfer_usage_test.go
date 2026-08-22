package billing

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
)

func TestDefaultRateRulesIncludeDisabledVolumeTransferMeter(t *testing.T) {
	t.Parallel()
	rule, exists := defaultRateRuleByMeter()[MeterStorageTransferGiB]
	if !exists {
		t.Fatal("default rate rules are missing storage.transfer_gib")
	}
	if rule.Unit != "gib" || rule.Enabled || !rule.CreditsPerUnit.Equal(decimal.Zero) {
		t.Fatalf("volume transfer default rate = %#v", rule)
	}
}

func TestVolumeTransferUsageSkipsNonTerminalOrEmptyTransfers(t *testing.T) {
	t.Parallel()
	service := Service{}
	for _, transfer := range []model.VolumeTransfer{
		{ID: "vtx_streaming", ProjectID: "prj_demo", State: model.VolumeTransferStateStreaming, TransferredBytes: 1024},
		{ID: "vtx_empty", ProjectID: "prj_demo", State: model.VolumeTransferStateSucceeded},
		{ID: "", ProjectID: "prj_demo", State: model.VolumeTransferStateSucceeded, TransferredBytes: 1024},
	} {
		if err := service.SettleVolumeTransferUsage(context.Background(), VolumeTransferUsageInput{Transfer: transfer}); err != nil {
			t.Fatalf("skip transfer %#v: %v", transfer, err)
		}
	}
}

func TestSettleVolumeTransferUsageIsCrossReplicaIdempotent(t *testing.T) {
	db := openBillingTestDB(t)
	if err := db.AutoMigrate(
		&model.Project{}, &model.ProjectMember{}, &model.UserWallet{}, &model.BillingRateRule{},
		&model.BillingUsageRecord{}, &model.BillingLedgerEntry{},
	); err != nil {
		t.Fatalf("migrate transfer billing tables: %v", err)
	}
	project := model.Project{
		ID: "prj_transfer_billing", Identifier: "transfer-billing", Name: "Transfer Billing", NamespaceStrategy: "project",
		BillingOwnerUserID: "usr_transfer_owner", DeleteStatus: "active",
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("seed transfer billing project: %v", err)
	}
	if err := db.Create(&model.UserWallet{
		ID: "wlt_transfer_owner", UserID: project.BillingOwnerUserID, BalanceCredits: decimal.NewFromInt(100),
	}).Error; err != nil {
		t.Fatalf("seed transfer billing wallet: %v", err)
	}
	if err := db.Create(&model.BillingRateRule{
		ID: "brte_transfer", Meter: MeterStorageTransferGiB, Unit: "gib",
		CreditsPerUnit: decimal.NewFromInt(2), Enabled: true, Description: "Volume transfer bytes",
	}).Error; err != nil {
		t.Fatalf("seed transfer billing rate: %v", err)
	}

	createdAt := time.Now().UTC().Add(-time.Minute)
	finishedAt := createdAt.Add(30 * time.Second)
	transfer := model.VolumeTransfer{
		ID: "vtx_billing_once", ProjectID: project.ID, ProjectVolumeID: "pvol_billing",
		Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
		State: model.VolumeTransferStateSucceeded, TransferredBytes: 2 * 1024 * 1024 * 1024,
		ActorID: "usr_transfer_actor", SourceFilename: "private.tar.gz",
		CreatedAt: createdAt, FinishedAt: &finishedAt,
	}

	start := make(chan struct{})
	errorsByReplica := make(chan error, 2)
	var wait sync.WaitGroup
	for replica := 0; replica < 2; replica++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errorsByReplica <- (Service{DB: db}).SettleVolumeTransferUsage(context.Background(), VolumeTransferUsageInput{
				Transfer: transfer, SettledAt: finishedAt, ActorID: "system",
			})
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByReplica)
	succeeded := 0
	alreadySettled := 0
	for err := range errorsByReplica {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrAlreadySettled):
			alreadySettled++
		default:
			t.Fatalf("unexpected transfer settlement error: %v", err)
		}
	}
	if succeeded != 1 || alreadySettled != 1 {
		t.Fatalf("settlement outcomes: succeeded=%d alreadySettled=%d", succeeded, alreadySettled)
	}

	var usage model.BillingUsageRecord
	if err := db.First(&usage, "meter = ? AND resource_type = ? AND resource_id = ?",
		MeterStorageTransferGiB, ResourceTypeTransfer, transfer.ID).Error; err != nil {
		t.Fatalf("read transfer usage: %v", err)
	}
	if !usage.Quantity.Equal(decimal.NewFromInt(2)) || !usage.AmountCredits.Equal(decimal.NewFromInt(4)) || usage.ProjectID != project.ID {
		t.Fatalf("transfer usage = %#v", usage)
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(usage.Metadata), &metadata); err != nil {
		t.Fatalf("parse transfer usage metadata: %v", err)
	}
	if metadata["volumeTransferId"] != transfer.ID || metadata["projectVolumeId"] != transfer.ProjectVolumeID {
		t.Fatalf("transfer usage metadata = %#v", metadata)
	}
	for _, sensitiveOrHighCardinalityKey := range []string{"objectKey", "sourceFilename", "filename"} {
		if _, exists := metadata[sensitiveOrHighCardinalityKey]; exists {
			t.Fatalf("transfer usage metadata contains %q: %#v", sensitiveOrHighCardinalityKey, metadata)
		}
	}

	var ledgerCount int64
	if err := db.Model(&model.BillingLedgerEntry{}).
		Where("reason = ? AND meter = ? AND resource_type = ? AND resource_id = ?", ReasonTransferUsage, MeterStorageTransferGiB, ResourceTypeTransfer, transfer.ID).
		Count(&ledgerCount).Error; err != nil {
		t.Fatalf("count transfer ledger entries: %v", err)
	}
	if ledgerCount != 1 {
		t.Fatalf("transfer ledger entries = %d, want 1", ledgerCount)
	}
	var wallet model.UserWallet
	if err := db.First(&wallet, "user_id = ?", project.BillingOwnerUserID).Error; err != nil {
		t.Fatalf("read transfer billing wallet: %v", err)
	}
	if !wallet.BalanceCredits.Equal(decimal.NewFromInt(96)) {
		t.Fatalf("wallet balance = %s, want 96", wallet.BalanceCredits)
	}
}
