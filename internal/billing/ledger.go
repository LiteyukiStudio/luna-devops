package billing

import (
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ReasonExternalRecharge = "billing.external_recharge"
	ReasonExternalAdjust   = "billing.external_adjustment"
	ReasonManualRecharge   = "billing.recharge"
	ReasonManualAdjust     = "billing.adjustment"
	ResourceTypeWallet     = "user_wallet"
)

type WalletTransactionInput struct {
	UserID         string
	ProjectID      string
	AmountCredits  decimal.Decimal
	Type           string
	Reason         string
	Description    string
	IdempotencyKey string
	ActorID        string
}

func (s Service) ApplyWalletTransaction(input WalletTransactionInput) (model.BillingLedgerEntry, error) {
	entry := model.BillingLedgerEntry{}
	if input.UserID == "" {
		return entry, errors.New("user id is required")
	}
	if input.AmountCredits.IsZero() {
		return entry, errors.New("amount credits cannot be zero")
	}
	entryType := input.Type
	if entryType == "" {
		entryType = "credit"
	}
	if entryType != "credit" && entryType != "adjustment" {
		return entry, errors.New("unsupported billing transaction type")
	}
	amount := input.AmountCredits
	if entryType == "credit" && amount.IsNegative() {
		return entry, errors.New("recharge amount must be positive")
	}
	reason := input.Reason
	if reason == "" {
		if entryType == "adjustment" {
			reason = ReasonManualAdjust
		} else {
			reason = ReasonManualRecharge
		}
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureWallet(tx, input.UserID); err != nil {
			return err
		}
		var wallet model.UserWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "user_id = ?", input.UserID).Error; err != nil {
			return err
		}
		if idempotencyKey != "" {
			var existing model.BillingLedgerEntry
			err := tx.First(&existing, "user_id = ? and idempotency_key = ?", input.UserID, idempotencyKey).Error
			if err == nil {
				if existing.Type != entryType || !existing.AmountCredits.Equal(amount) {
					return errors.New("idempotency key has been used by a different billing transaction")
				}
				entry = existing
				return nil
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		balanceAfter := wallet.BalanceCredits.Add(amount)
		entry = model.BillingLedgerEntry{
			ID:                  id.New("bled"),
			UserID:              input.UserID,
			ProjectID:           strings.TrimSpace(input.ProjectID),
			Type:                entryType,
			AmountCredits:       amount,
			BalanceAfterCredits: balanceAfter,
			Reason:              reason,
			ResourceType:        ResourceTypeWallet,
			ResourceID:          input.UserID,
			IdempotencyKey:      idempotencyKey,
			Description:         input.Description,
			CreatedBy:           input.ActorID,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		return tx.Model(&model.UserWallet{}).Where("id = ?", wallet.ID).Update("balance_credits", balanceAfter).Error
	})
	return entry, err
}

func (s Service) debitUsage(usage model.BillingUsageRecord, reason string, description string, actorID string) error {
	return s.debitUsages([]model.BillingUsageRecord{usage}, reason, description, actorID)
}

func (s Service) debitUsages(usages []model.BillingUsageRecord, reason string, description string, actorID string) error {
	if len(usages) == 0 {
		return nil
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		projectID := usages[0].ProjectID
		billedUserID, err := billingOwnerUserID(tx, projectID)
		if err != nil {
			return err
		}
		if err := ensureWallet(tx, billedUserID); err != nil {
			return err
		}
		var wallet model.UserWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "user_id = ?", billedUserID).Error; err != nil {
			return err
		}
		balanceAfter := wallet.BalanceCredits
		created := 0
		for _, usage := range usages {
			if usage.ProjectID != projectID {
				return errors.New("billing usage batch must belong to one project")
			}
			var existing model.BillingUsageRecord
			err := tx.First(&existing, "resource_type = ? and resource_id = ? and meter = ?", usage.ResourceType, usage.ResourceID, usage.Meter).Error
			if err == nil {
				continue
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			usage.BilledUserID = billedUserID
			balanceAfter = balanceAfter.Sub(usage.AmountCredits)
			if err := tx.Create(&usage).Error; err != nil {
				return err
			}
			entry := model.BillingLedgerEntry{
				ID:                  id.New("bled"),
				UserID:              billedUserID,
				ProjectID:           usage.ProjectID,
				Type:                "debit",
				AmountCredits:       usage.AmountCredits.Neg(),
				BalanceAfterCredits: balanceAfter,
				Reason:              reason,
				Meter:               usage.Meter,
				UsageRecordID:       usage.ID,
				ResourceType:        usage.ResourceType,
				ResourceID:          usage.ResourceID,
				Description:         description,
				CreatedBy:           actorID,
			}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			created++
		}
		if created == 0 {
			return ErrAlreadySettled
		}
		return tx.Model(&model.UserWallet{}).Where("id = ?", wallet.ID).Update("balance_credits", balanceAfter).Error
	})
}

func billingOwnerUserID(tx *gorm.DB, projectID string) (string, error) {
	var project model.Project
	if err := tx.Select("billing_owner_user_id").First(&project, "id = ?", projectID).Error; err != nil {
		return "", err
	}
	ownerID := strings.TrimSpace(project.BillingOwnerUserID)
	if ownerID != "" {
		return ownerID, nil
	}
	var member model.ProjectMember
	if err := tx.Select("user_id").First(&member, "project_id = ? and role = ?", projectID, "owner").Error; err != nil {
		return "", err
	}
	return strings.TrimSpace(member.UserID), nil
}

func ensureWallet(tx *gorm.DB, userID string) error {
	wallet := model.UserWallet{ID: id.New("wlt"), UserID: userID, BalanceCredits: decimal.Zero}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoNothing: true}).Create(&wallet).Error
}

func (s Service) EnsureWallet(userID string) (model.UserWallet, error) {
	if err := s.DB.Transaction(func(tx *gorm.DB) error { return ensureWallet(tx, userID) }); err != nil {
		return model.UserWallet{}, err
	}
	var wallet model.UserWallet
	err := s.DB.First(&wallet, "user_id = ?", userID).Error
	return wallet, err
}
