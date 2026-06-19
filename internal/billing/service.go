package billing

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	MeterBuildJob        = "build.job"
	ReasonBuildUsage     = "build.usage"
	ReasonRuntimeUsage   = "runtime.usage"
	ReasonManualRecharge = "billing.recharge"
	ReasonManualAdjust   = "billing.adjustment"
	ResourceTypeBuildRun = "build_run"
	ResourceTypeRuntime  = "runtime_target"
	ResourceTypeWallet   = "project_wallet"
	defaultCPURequest    = "500m"
	defaultMemoryRequest = "512Mi"
)

var ErrAlreadySettled = errors.New("billing usage already settled")

type Service struct {
	DB *gorm.DB
}

type BuildUsageInput struct {
	Run         model.BuildRun
	Job         model.BuildJob
	Environment model.Environment
	FinishedAt  time.Time
}

type RuntimeUsageInput struct {
	ProjectID          string
	ApplicationID      string
	DeploymentTargetID string
	Environment        model.Environment
	PeriodStart        time.Time
	PeriodEnd          time.Time
	ActorID            string
}

type WalletTransactionInput struct {
	ProjectID     string
	AmountCredits decimal.Decimal
	Type          string
	Reason        string
	Description   string
	ActorID       string
}

type ProjectBillingSummary struct {
	BalanceCredits decimal.Decimal `json:"balanceCredits"`
	TodaySpend     decimal.Decimal `json:"todaySpend"`
	MonthSpend     decimal.Decimal `json:"monthSpend"`
}

type RateRuleUpdate struct {
	Meter          string
	CreditsPerUnit decimal.Decimal
	Enabled        bool
}

func (s Service) EnsureDefaultRateRules() error {
	for _, rule := range defaultRateRules() {
		if err := s.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "meter"}}, DoNothing: true}).Create(&rule).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s Service) ListRateRules() ([]model.BillingRateRule, error) {
	if err := s.EnsureDefaultRateRules(); err != nil {
		return nil, err
	}
	var rules []model.BillingRateRule
	err := s.DB.Order("meter asc").Find(&rules).Error
	return rules, err
}

func (s Service) UpdateRateRules(updates []RateRuleUpdate) ([]model.BillingRateRule, error) {
	defaults := defaultRateRuleByMeter()
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		service := Service{DB: tx}
		if err := service.EnsureDefaultRateRules(); err != nil {
			return err
		}
		for _, update := range updates {
			defaultRule, ok := defaults[update.Meter]
			if !ok {
				return gorm.ErrRecordNotFound
			}
			if update.CreditsPerUnit.IsNegative() {
				return errors.New("credits per unit cannot be negative")
			}
			if err := tx.Model(&model.BillingRateRule{}).
				Where("meter = ?", update.Meter).
				Updates(map[string]any{
					"unit":             defaultRule.Unit,
					"description":      defaultRule.Description,
					"credits_per_unit": update.CreditsPerUnit,
					"enabled":          update.Enabled,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ListRateRules()
}

func defaultRateRules() []model.BillingRateRule {
	now := time.Now()
	return []model.BillingRateRule{
		{ID: id.New("brte"), Meter: "build.cpu_vcpu_minute", Unit: "vcpu_minute", CreditsPerUnit: decimal.NewFromInt(10), Enabled: true, Description: "Build CPU usage", CreatedAt: now, UpdatedAt: now},
		{ID: id.New("brte"), Meter: "build.memory_gib_minute", Unit: "gib_minute", CreditsPerUnit: decimal.NewFromInt(2), Enabled: true, Description: "Build memory usage", CreatedAt: now, UpdatedAt: now},
		{ID: id.New("brte"), Meter: "runtime.cpu_vcpu_hour", Unit: "vcpu_hour", CreditsPerUnit: decimal.NewFromInt(30), Enabled: true, Description: "Runtime CPU usage", CreatedAt: now, UpdatedAt: now},
		{ID: id.New("brte"), Meter: "runtime.memory_gib_hour", Unit: "gib_hour", CreditsPerUnit: decimal.NewFromInt(6), Enabled: true, Description: "Runtime memory usage", CreatedAt: now, UpdatedAt: now},
		{ID: id.New("brte"), Meter: "storage.gib_day", Unit: "gib_day", CreditsPerUnit: decimal.NewFromInt(1), Enabled: true, Description: "Persistent storage usage", CreatedAt: now, UpdatedAt: now},
		{ID: id.New("brte"), Meter: "gateway.requests_1000", Unit: "1000_requests", CreditsPerUnit: decimal.NewFromInt(1), Enabled: true, Description: "Gateway request usage", CreatedAt: now, UpdatedAt: now},
	}
}

func defaultRateRuleByMeter() map[string]model.BillingRateRule {
	result := map[string]model.BillingRateRule{}
	for _, rule := range defaultRateRules() {
		result[rule.Meter] = rule
	}
	return result
}

func (s Service) SettleBuildRun(input BuildUsageInput) error {
	if input.Run.ID == "" || input.Run.ProjectID == "" || input.Job.ID == "" || input.Run.StartedAt == nil {
		return nil
	}
	periodStart := *input.Run.StartedAt
	periodEnd := input.FinishedAt
	if input.Run.FinishedAt != nil {
		periodEnd = *input.Run.FinishedAt
	}
	if !periodEnd.After(periodStart) {
		periodEnd = periodStart.Add(time.Minute)
	}
	durationSeconds := int64(periodEnd.Sub(periodStart) / time.Second)
	if durationSeconds < 1 {
		durationSeconds = 1
	}
	durationMinutes := decimal.NewFromInt(durationSeconds).Div(decimal.NewFromInt(60))
	if durationMinutes.LessThan(decimal.NewFromInt(1)) {
		durationMinutes = decimal.NewFromInt(1)
	}
	cpuCores := cpuCoresFromQuantity(input.Environment.CPURequest)
	memoryGiB := memoryGiBFromQuantity(input.Environment.MemoryRequest)
	cpuAmount, memoryAmount, amount, err := s.buildAmount(cpuCores, memoryGiB, durationMinutes)
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{
		"buildJobId":      input.Job.ID,
		"durationMinutes": durationMinutes.String(),
		"cpuCores":        cpuCores.String(),
		"memoryGiB":       memoryGiB.String(),
		"cpuCredits":      cpuAmount.String(),
		"memoryCredits":   memoryAmount.String(),
		"buildStatus":     input.Run.Status,
		"environmentId":   input.Environment.ID,
	})
	now := time.Now()
	usage := model.BillingUsageRecord{
		ID:            id.New("busg"),
		ProjectID:     input.Run.ProjectID,
		ApplicationID: input.Run.ApplicationID,
		Meter:         MeterBuildJob,
		Quantity:      durationMinutes,
		Unit:          "minute",
		AmountCredits: amount,
		ResourceType:  ResourceTypeBuildRun,
		ResourceID:    input.Run.ID,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		Status:        "settled",
		Metadata:      string(metadata),
		SettledAt:     &now,
	}
	return s.debitUsage(usage, ReasonBuildUsage, "Build job usage", input.Run.CreatedBy)
}

func (s Service) SettleRuntimeTargetWindow(input RuntimeUsageInput) error {
	if input.ProjectID == "" || input.DeploymentTargetID == "" || !input.PeriodEnd.After(input.PeriodStart) {
		return nil
	}
	replicas := input.Environment.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	durationHours := decimal.NewFromInt(int64(input.PeriodEnd.Sub(input.PeriodStart) / time.Second)).Div(decimal.NewFromInt(3600))
	if durationHours.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	replicaHours := decimal.NewFromInt(int64(replicas)).Mul(durationHours)
	cpuQuantity := cpuCoresFromQuantity(input.Environment.CPURequest).Mul(replicaHours)
	memoryQuantity := memoryGiBFromQuantity(input.Environment.MemoryRequest).Mul(replicaHours)
	cpuRate, err := s.rate("runtime.cpu_vcpu_hour")
	if err != nil {
		return err
	}
	memoryRate, err := s.rate("runtime.memory_gib_hour")
	if err != nil {
		return err
	}
	resourceID := runtimeUsageResourceID(input.DeploymentTargetID, input.PeriodStart)
	metadata, _ := json.Marshal(map[string]string{
		"deploymentTargetId": input.DeploymentTargetID,
		"environmentId":      input.Environment.ID,
		"replicas":           decimal.NewFromInt(int64(replicas)).String(),
		"durationHours":      durationHours.String(),
		"cpuCores":           cpuCoresFromQuantity(input.Environment.CPURequest).String(),
		"memoryGiB":          memoryGiBFromQuantity(input.Environment.MemoryRequest).String(),
	})
	now := time.Now()
	records := []model.BillingUsageRecord{
		{
			ID:            id.New("busg"),
			ProjectID:     input.ProjectID,
			ApplicationID: input.ApplicationID,
			Meter:         "runtime.cpu_vcpu_hour",
			Quantity:      cpuQuantity,
			Unit:          "vcpu_hour",
			AmountCredits: cpuQuantity.Mul(cpuRate),
			ResourceType:  ResourceTypeRuntime,
			ResourceID:    resourceID,
			PeriodStart:   input.PeriodStart,
			PeriodEnd:     input.PeriodEnd,
			Status:        "settled",
			Metadata:      string(metadata),
			SettledAt:     &now,
		},
		{
			ID:            id.New("busg"),
			ProjectID:     input.ProjectID,
			ApplicationID: input.ApplicationID,
			Meter:         "runtime.memory_gib_hour",
			Quantity:      memoryQuantity,
			Unit:          "gib_hour",
			AmountCredits: memoryQuantity.Mul(memoryRate),
			ResourceType:  ResourceTypeRuntime,
			ResourceID:    resourceID,
			PeriodStart:   input.PeriodStart,
			PeriodEnd:     input.PeriodEnd,
			Status:        "settled",
			Metadata:      string(metadata),
			SettledAt:     &now,
		},
	}
	return s.debitUsages(records, ReasonRuntimeUsage, "Runtime resource usage", input.ActorID)
}

func (s Service) ApplyWalletTransaction(input WalletTransactionInput) (model.BillingLedgerEntry, error) {
	entry := model.BillingLedgerEntry{}
	if input.ProjectID == "" {
		return entry, errors.New("project id is required")
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
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureWallet(tx, input.ProjectID); err != nil {
			return err
		}
		var wallet model.ProjectWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "project_id = ?", input.ProjectID).Error; err != nil {
			return err
		}
		balanceAfter := wallet.BalanceCredits.Add(amount)
		entry = model.BillingLedgerEntry{
			ID:                  id.New("bled"),
			ProjectID:           input.ProjectID,
			Type:                entryType,
			AmountCredits:       amount,
			BalanceAfterCredits: balanceAfter,
			Reason:              reason,
			ResourceType:        ResourceTypeWallet,
			ResourceID:          input.ProjectID,
			Description:         input.Description,
			CreatedBy:           input.ActorID,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		return tx.Model(&model.ProjectWallet{}).Where("id = ?", wallet.ID).Update("balance_credits", balanceAfter).Error
	})
	return entry, err
}

func runtimeUsageResourceID(deploymentTargetID string, periodStart time.Time) string {
	return deploymentTargetID + ":" + periodStart.UTC().Format("2006010215")
}

func (s Service) buildAmount(cpuCores decimal.Decimal, memoryGiB decimal.Decimal, durationMinutes decimal.Decimal) (decimal.Decimal, decimal.Decimal, decimal.Decimal, error) {
	cpuRate, err := s.rate("build.cpu_vcpu_minute")
	if err != nil {
		return decimal.Zero, decimal.Zero, decimal.Zero, err
	}
	memoryRate, err := s.rate("build.memory_gib_minute")
	if err != nil {
		return decimal.Zero, decimal.Zero, decimal.Zero, err
	}
	cpuAmount := cpuCores.Mul(durationMinutes).Mul(cpuRate)
	memoryAmount := memoryGiB.Mul(durationMinutes).Mul(memoryRate)
	return cpuAmount, memoryAmount, cpuAmount.Add(memoryAmount), nil
}

func (s Service) rate(meter string) (decimal.Decimal, error) {
	var rule model.BillingRateRule
	if err := s.DB.First(&rule, "meter = ?", meter).Error; err != nil {
		for _, defaultRule := range defaultRateRules() {
			if defaultRule.Meter == meter {
				return defaultRule.CreditsPerUnit, nil
			}
		}
		return decimal.Zero, err
	}
	if !rule.Enabled {
		return decimal.Zero, nil
	}
	return rule.CreditsPerUnit, nil
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
		if err := ensureWallet(tx, projectID); err != nil {
			return err
		}
		var wallet model.ProjectWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "project_id = ?", projectID).Error; err != nil {
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
			balanceAfter = balanceAfter.Sub(usage.AmountCredits)
			if err := tx.Create(&usage).Error; err != nil {
				return err
			}
			entry := model.BillingLedgerEntry{
				ID:                  id.New("bled"),
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
		return tx.Model(&model.ProjectWallet{}).Where("id = ?", wallet.ID).Update("balance_credits", balanceAfter).Error
	})
}

func ensureWallet(tx *gorm.DB, projectID string) error {
	wallet := model.ProjectWallet{ID: id.New("wlt"), ProjectID: projectID, BalanceCredits: decimal.Zero}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "project_id"}}, DoNothing: true}).Create(&wallet).Error
}

func (s Service) EnsureWallet(projectID string) (model.ProjectWallet, error) {
	if err := s.DB.Transaction(func(tx *gorm.DB) error { return ensureWallet(tx, projectID) }); err != nil {
		return model.ProjectWallet{}, err
	}
	var wallet model.ProjectWallet
	err := s.DB.First(&wallet, "project_id = ?", projectID).Error
	return wallet, err
}

func (s Service) Summary(projectIDs []string, now time.Time) (ProjectBillingSummary, error) {
	summary := ProjectBillingSummary{}
	if len(projectIDs) == 0 {
		return summary, nil
	}
	for _, projectID := range projectIDs {
		wallet, err := s.EnsureWallet(projectID)
		if err != nil {
			return summary, err
		}
		summary.BalanceCredits = summary.BalanceCredits.Add(wallet.BalanceCredits)
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	summary.TodaySpend = s.spendSince(projectIDs, dayStart)
	summary.MonthSpend = s.spendSince(projectIDs, monthStart)
	return summary, nil
}

func (s Service) spendSince(projectIDs []string, since time.Time) decimal.Decimal {
	var entries []model.BillingLedgerEntry
	if err := s.DB.Where("project_id in ? and type = ? and created_at >= ?", projectIDs, "debit", since).Find(&entries).Error; err != nil {
		return decimal.Zero
	}
	total := decimal.Zero
	for _, entry := range entries {
		total = total.Add(entry.AmountCredits.Abs())
	}
	return total
}

func cpuCoresFromQuantity(value string) decimal.Decimal {
	if value == "" {
		value = defaultCPURequest
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		quantity = resource.MustParse(defaultCPURequest)
	}
	return decimal.NewFromInt(quantity.MilliValue()).Div(decimal.NewFromInt(1000))
}

func memoryGiBFromQuantity(value string) decimal.Decimal {
	if value == "" {
		value = defaultMemoryRequest
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		quantity = resource.MustParse(defaultMemoryRequest)
	}
	return decimal.NewFromInt(quantity.Value()).Div(decimal.NewFromInt(1024 * 1024 * 1024))
}
