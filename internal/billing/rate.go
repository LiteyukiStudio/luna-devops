package billing

import (
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MeterBuildJob = "build.job"

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
