package retention

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

var (
	ErrNilDB                = errors.New("retention database is nil")
	ErrNilDatabase          = ErrNilDB
	ErrInvalidRange         = errors.New("retention range must satisfy start < end")
	ErrNoDatasets           = errors.New("at least one retention dataset is required")
	ErrUnknownDataset       = errors.New("unknown retention dataset")
	ErrInvalidRetentionDays = errors.New("retention days must be between 0 and 3650")
)

// Result reports the rows eligible at preview time and rows actually deleted.
type Result struct {
	Dataset string `json:"dataset"`
	Matched int64  `json:"matched"`
	Deleted int64  `json:"deleted"`
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Preview(ctx context.Context, datasets []string, start, end, now time.Time) ([]Result, error) {
	selected, err := validateRequest(s, datasets, start, end)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(selected))
	for _, plan := range selected {
		matched, err := s.countPlan(ctx, plan, start, end, now)
		if err != nil {
			return nil, fmt.Errorf("preview retention dataset %q: %w", plan.dataset.Key, err)
		}
		results = append(results, Result{Dataset: plan.dataset.Key, Matched: matched})
	}
	return results, nil
}

func (s *Service) Cleanup(ctx context.Context, datasets []string, start, end, now time.Time) ([]Result, error) {
	selected, err := validateRequest(s, datasets, start, end)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(selected))
	for _, plan := range selected {
		matched, err := s.countPlan(ctx, plan, start, end, now)
		if err != nil {
			return nil, fmt.Errorf("count retention dataset %q: %w", plan.dataset.Key, err)
		}
		deleted, err := s.cleanupPlan(ctx, plan, start, end, now)
		if err != nil {
			return nil, fmt.Errorf("cleanup retention dataset %q: %w", plan.dataset.Key, err)
		}
		results = append(results, Result{Dataset: plan.dataset.Key, Matched: matched, Deleted: deleted})
	}
	return results, nil
}

// RunAutomatic applies configured retention periods. A value of zero skips a dataset.
func (s *Service) RunAutomatic(ctx context.Context, now time.Time) ([]Result, error) {
	if s == nil || s.db == nil {
		return nil, ErrNilDB
	}

	daysByDataset, err := s.loadRetentionDays(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(catalog))
	for _, dataset := range catalog {
		days := daysByDataset[dataset.Key]
		if days == 0 {
			continue
		}
		end := now.AddDate(0, 0, -days)
		result, err := s.Cleanup(ctx, []string{dataset.Key}, time.Time{}, end, now)
		if err != nil {
			return nil, err
		}
		results = append(results, result...)
	}
	return results, nil
}

func validateRequest(s *Service, datasets []string, start, end time.Time) ([]datasetPlan, error) {
	if s == nil || s.db == nil {
		return nil, ErrNilDB
	}
	if !start.Before(end) {
		return nil, ErrInvalidRange
	}
	return selectPlans(datasets)
}

func selectPlans(datasets []string) ([]datasetPlan, error) {
	if len(datasets) == 0 {
		return nil, ErrNoDatasets
	}

	selected := make([]datasetPlan, 0, len(datasets))
	seen := make(map[string]struct{}, len(datasets))
	for _, key := range datasets {
		plan, ok := plans[key]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownDataset, key)
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		selected = append(selected, plan)
	}
	return selected, nil
}

func (s *Service) countPlan(ctx context.Context, plan datasetPlan, start, end, now time.Time) (int64, error) {
	var matched int64
	for _, query := range plan.queries {
		var row struct {
			Count int64
		}
		args := query.rangeArgs(start, end, now)
		if err := s.db.WithContext(ctx).Raw(query.countSQL, args...).Scan(&row).Error; err != nil {
			return 0, err
		}
		matched += row.Count
	}
	return matched, nil
}

func (s *Service) cleanupPlan(ctx context.Context, plan datasetPlan, start, end, now time.Time) (int64, error) {
	var deleted int64
	for _, query := range plan.queries {
		args := query.rangeArgs(start, end, now)
		for {
			if err := ctx.Err(); err != nil {
				return deleted, err
			}
			result := s.db.WithContext(ctx).Exec(query.deleteSQL, args...)
			if result.Error != nil {
				return deleted, result.Error
			}
			deleted += result.RowsAffected
			if result.RowsAffected < cleanupBatchSize {
				break
			}
		}
	}
	return deleted, nil
}

func (s *Service) loadRetentionDays(ctx context.Context) (map[string]int, error) {
	type configRow struct {
		Key   string
		Value string
	}
	keys := make([]string, 0, len(catalog))
	datasetByConfigKey := make(map[string]Dataset, len(catalog))
	daysByDataset := make(map[string]int, len(catalog))
	for _, dataset := range catalog {
		keys = append(keys, dataset.ConfigKey)
		datasetByConfigKey[dataset.ConfigKey] = dataset
		daysByDataset[dataset.Key] = dataset.DefaultDays
	}

	var rows []configRow
	if err := s.db.WithContext(ctx).Table("app_configs").
		Select("key", "value").
		Where("key IN ?", keys).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load retention configs: %w", err)
	}
	for _, row := range rows {
		dataset, ok := datasetByConfigKey[row.Key]
		if !ok {
			continue
		}
		days, err := parseRetentionDays(row.Value)
		if err != nil {
			return nil, fmt.Errorf("config %q: %w", row.Key, err)
		}
		daysByDataset[dataset.Key] = days
	}
	return daysByDataset, nil
}

func parseRetentionDays(value string) (int, error) {
	days, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || days < MinRetentionDays || days > MaxRetentionDays {
		return 0, ErrInvalidRetentionDays
	}
	return days, nil
}
