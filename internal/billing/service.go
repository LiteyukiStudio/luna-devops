package billing

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	MeterBuildJob          = "build.job"
	ReasonBuildUsage       = "build.usage"
	ReasonRuntimeUsage     = "runtime.usage"
	ReasonStorageUsage     = "storage.usage"
	ReasonGatewayUsage     = "gateway.usage"
	ReasonExternalRecharge = "billing.external_recharge"
	ReasonExternalAdjust   = "billing.external_adjustment"
	ReasonManualRecharge   = "billing.recharge"
	ReasonManualAdjust     = "billing.adjustment"
	ResourceTypeBuildRun   = "build_run"
	ResourceTypeRuntime    = "runtime_target"
	ResourceTypeStorage    = "storage_volume"
	ResourceTypeGateway    = "gateway_route"
	ResourceTypeWallet     = "user_wallet"
	defaultCPURequest      = "500m"
	defaultMemoryRequest   = "512Mi"
	defaultDataCapacity    = "1Gi"
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

type StorageUsageInput struct {
	Target      model.DeploymentTarget
	PeriodStart time.Time
	PeriodEnd   time.Time
	ActorID     string
}

type GatewayTrafficUsageInput struct {
	Route         model.GatewayRoute
	ResponseBytes int64
	RequestCount  int64
	PeriodStart   time.Time
	PeriodEnd     time.Time
	ActorID       string
}

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

type ProjectBillingSummary struct {
	BalanceCredits    decimal.Decimal        `json:"balanceCredits"`
	TodaySpend        decimal.Decimal        `json:"todaySpend"`
	MonthSpend        decimal.Decimal        `json:"monthSpend"`
	PendingSpend      decimal.Decimal        `json:"pendingSpend"`
	AvailableCredits  decimal.Decimal        `json:"availableCredits"`
	LowBalanceLimit   decimal.Decimal        `json:"lowBalanceLimit"`
	BalanceStatus     string                 `json:"balanceStatus"`
	MonthlyCategories []BillingSpendCategory `json:"monthlyCategories"`
}

type BillingSpendCategory struct {
	Category      string          `json:"category"`
	AmountCredits decimal.Decimal `json:"amountCredits"`
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
		{ID: id.New("brte"), Meter: "gateway.egress_gib", Unit: "gib", CreditsPerUnit: decimal.NewFromInt(1), Enabled: true, Description: "Gateway response egress traffic", CreatedAt: now, UpdatedAt: now},
		{ID: id.New("brte"), Meter: "gateway.requests_1000", Unit: "1000_requests", CreditsPerUnit: decimal.Zero, Enabled: false, Description: "Gateway request count", CreatedAt: now, UpdatedAt: now},
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
	cpuCores := cpuCoresFromQuantity(input.Run.BuildCPURequest)
	memoryGiB := memoryGiBFromQuantity(input.Run.BuildMemoryRequest)
	cpuAmount, memoryAmount, amount, err := s.buildAmount(cpuCores, memoryGiB, durationMinutes)
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{
		"buildJobId":         input.Job.ID,
		"durationMinutes":    durationMinutes.String(),
		"cpuCores":           cpuCores.String(),
		"memoryGiB":          memoryGiB.String(),
		"cpuCredits":         cpuAmount.String(),
		"memoryCredits":      memoryAmount.String(),
		"buildStatus":        input.Run.Status,
		"environmentId":      input.Environment.ID,
		"buildEnvironmentId": input.Environment.ID,
		"buildCPU":           input.Run.BuildCPURequest,
		"buildMemory":        input.Run.BuildMemoryRequest,
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

func (s Service) SettleStorageTargetWindow(input StorageUsageInput) error {
	if input.Target.ProjectID == "" || input.Target.ID == "" || !input.Target.DataRetentionEnabled || !input.PeriodEnd.After(input.PeriodStart) {
		return nil
	}
	capacityGiB := deploymentTargetStorageGiB(input.Target)
	if capacityGiB.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	durationDays := decimal.NewFromInt(int64(input.PeriodEnd.Sub(input.PeriodStart) / time.Second)).Div(decimal.NewFromInt(86400))
	if durationDays.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	quantity := capacityGiB.Mul(durationDays)
	rate, err := s.rate("storage.gib_day")
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{
		"deploymentTargetId": input.Target.ID,
		"dataRetention":      "true",
		"capacityGiB":        capacityGiB.String(),
		"durationDays":       durationDays.String(),
	})
	now := time.Now()
	usage := model.BillingUsageRecord{
		ID:            id.New("busg"),
		ProjectID:     input.Target.ProjectID,
		ApplicationID: input.Target.ApplicationID,
		Meter:         "storage.gib_day",
		Quantity:      quantity,
		Unit:          "gib_day",
		AmountCredits: quantity.Mul(rate),
		ResourceType:  ResourceTypeStorage,
		ResourceID:    storageUsageResourceID(input.Target.ID, input.PeriodStart),
		PeriodStart:   input.PeriodStart,
		PeriodEnd:     input.PeriodEnd,
		Status:        "settled",
		Metadata:      string(metadata),
		SettledAt:     &now,
	}
	return s.debitUsage(usage, ReasonStorageUsage, "Persistent storage usage", input.ActorID)
}

func (s Service) SettleGatewayTrafficWindow(input GatewayTrafficUsageInput) error {
	if input.Route.ID == "" || input.Route.ProjectID == "" || input.ResponseBytes <= 0 || !input.PeriodEnd.After(input.PeriodStart) {
		return nil
	}
	responseGiB := decimal.NewFromInt(input.ResponseBytes).Div(decimal.NewFromInt(1024 * 1024 * 1024))
	if responseGiB.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	rate, err := s.rate("gateway.egress_gib")
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{
		"gatewayRouteId": input.Route.ID,
		"host":           input.Route.Host,
		"path":           input.Route.Path,
		"responseBytes":  decimal.NewFromInt(input.ResponseBytes).String(),
		"responseGiB":    responseGiB.String(),
		"requestCount":   decimal.NewFromInt(input.RequestCount).String(),
	})
	now := time.Now()
	usage := model.BillingUsageRecord{
		ID:            id.New("busg"),
		ProjectID:     input.Route.ProjectID,
		ApplicationID: input.Route.ApplicationID,
		Meter:         "gateway.egress_gib",
		Quantity:      responseGiB,
		Unit:          "gib",
		AmountCredits: responseGiB.Mul(rate),
		ResourceType:  ResourceTypeGateway,
		ResourceID:    gatewayTrafficUsageResourceID(input.Route.ID, input.PeriodStart),
		PeriodStart:   input.PeriodStart,
		PeriodEnd:     input.PeriodEnd,
		Status:        "settled",
		Metadata:      string(metadata),
		SettledAt:     &now,
	}
	return s.debitUsage(usage, ReasonGatewayUsage, "Gateway response traffic usage", input.ActorID)
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

func runtimeUsageResourceID(deploymentTargetID string, periodStart time.Time) string {
	return deploymentTargetID + ":" + periodStart.UTC().Format("2006010215")
}

func storageUsageResourceID(deploymentTargetID string, periodStart time.Time) string {
	return deploymentTargetID + ":" + periodStart.UTC().Format("2006010215")
}

func gatewayTrafficUsageResourceID(routeID string, periodStart time.Time) string {
	return routeID + ":" + periodStart.UTC().Format("200601021504")
}

func deploymentTargetStorageGiB(target model.DeploymentTarget) decimal.Decimal {
	total := decimal.Zero
	for _, volume := range deploymentTargetBillingVolumes(target) {
		total = total.Add(storageGiBFromQuantity(volume.Capacity))
	}
	if total.GreaterThan(decimal.Zero) {
		return total
	}
	return storageGiBFromQuantity(target.DataCapacity)
}

type deploymentTargetBillingVolume struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	Capacity  string `json:"capacity"`
}

func deploymentTargetBillingVolumes(target model.DeploymentTarget) []deploymentTargetBillingVolume {
	var volumes []deploymentTargetBillingVolume
	if err := json.Unmarshal([]byte(target.DataVolumes), &volumes); err != nil {
		return nil
	}
	output := make([]deploymentTargetBillingVolume, 0, len(volumes))
	for _, volume := range volumes {
		if volume.Capacity != "" {
			output = append(output, volume)
		}
	}
	return output
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

func (s Service) Summary(userIDs []string, projectIDs []string, now time.Time, lowBalanceLimit decimal.Decimal) (ProjectBillingSummary, error) {
	summary := ProjectBillingSummary{LowBalanceLimit: lowBalanceLimit, BalanceStatus: "ok"}
	if len(userIDs) == 0 {
		return summary, nil
	}
	for _, userID := range userIDs {
		wallet, err := s.EnsureWallet(userID)
		if err != nil {
			return summary, err
		}
		summary.BalanceCredits = summary.BalanceCredits.Add(wallet.BalanceCredits)
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	summary.TodaySpend = s.spendSince(userIDs, projectIDs, dayStart)
	summary.MonthSpend = s.spendSince(userIDs, projectIDs, monthStart)
	summary.PendingSpend = s.pendingSpend(userIDs, projectIDs)
	summary.AvailableCredits = summary.BalanceCredits.Sub(summary.PendingSpend)
	summary.BalanceStatus = balanceStatus(summary.AvailableCredits, lowBalanceLimit)
	summary.MonthlyCategories = s.monthlyCategories(userIDs, projectIDs, monthStart)
	return summary, nil
}

func (s Service) spendSince(userIDs []string, projectIDs []string, since time.Time) decimal.Decimal {
	var entries []model.BillingLedgerEntry
	query := s.DB.Where("user_id in ? and type = ? and created_at >= ?", userIDs, "debit", since)
	if len(projectIDs) > 0 {
		query = query.Where("project_id in ?", projectIDs)
	}
	if err := query.Find(&entries).Error; err != nil {
		return decimal.Zero
	}
	total := decimal.Zero
	for _, entry := range entries {
		total = total.Add(entry.AmountCredits.Abs())
	}
	return total
}

func (s Service) pendingSpend(userIDs []string, projectIDs []string) decimal.Decimal {
	var records []model.BillingUsageRecord
	query := s.DB.Where("billed_user_id in ? and status = ?", userIDs, "pending")
	if len(projectIDs) > 0 {
		query = query.Where("project_id in ?", projectIDs)
	}
	if err := query.Find(&records).Error; err != nil {
		return decimal.Zero
	}
	total := decimal.Zero
	for _, record := range records {
		total = total.Add(record.AmountCredits)
	}
	return total
}

func (s Service) monthlyCategories(userIDs []string, projectIDs []string, monthStart time.Time) []BillingSpendCategory {
	var entries []model.BillingLedgerEntry
	query := s.DB.Where("user_id in ? and type = ? and created_at >= ?", userIDs, "debit", monthStart)
	if len(projectIDs) > 0 {
		query = query.Where("project_id in ?", projectIDs)
	}
	if err := query.Find(&entries).Error; err != nil {
		return nil
	}
	amounts := map[string]decimal.Decimal{}
	for _, entry := range entries {
		category := billingCategory(entry.Reason, entry.Meter)
		amounts[category] = amounts[category].Add(entry.AmountCredits.Abs())
	}
	order := []string{"build", "runtime", "storage", "gateway", "adjustment", "other"}
	categories := make([]BillingSpendCategory, 0, len(order))
	for _, category := range order {
		amount := amounts[category]
		if amount.IsZero() {
			continue
		}
		categories = append(categories, BillingSpendCategory{Category: category, AmountCredits: amount})
	}
	return categories
}

func billingCategory(reason string, meter string) string {
	value := strings.TrimSpace(reason)
	if value == "" {
		value = strings.TrimSpace(meter)
	}
	switch {
	case strings.HasPrefix(value, "build."):
		return "build"
	case strings.HasPrefix(value, "runtime."):
		return "runtime"
	case strings.HasPrefix(value, "storage."):
		return "storage"
	case strings.HasPrefix(value, "gateway."):
		return "gateway"
	case strings.HasPrefix(value, "billing."):
		return "adjustment"
	default:
		return "other"
	}
}

func balanceStatus(available decimal.Decimal, lowBalanceLimit decimal.Decimal) string {
	if available.IsNegative() || available.IsZero() {
		return "insufficient"
	}
	if lowBalanceLimit.IsPositive() && available.LessThanOrEqual(lowBalanceLimit) {
		return "low"
	}
	return "ok"
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

func storageGiBFromQuantity(value string) decimal.Decimal {
	if value == "" {
		value = defaultDataCapacity
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		quantity = resource.MustParse(defaultDataCapacity)
	}
	return decimal.NewFromInt(quantity.Value()).Div(decimal.NewFromInt(1024 * 1024 * 1024))
}
