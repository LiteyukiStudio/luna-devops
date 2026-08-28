package notification

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	RuleScopeProjects = "projects"
	RuleScopeAll      = "all"
)

var ErrInvalidRuleFilter = errors.New("invalid notification rule filter")

type RuleFilter struct {
	Scope               string   `json:"scope"`
	ProjectIDs          []string `json:"projectIds,omitempty"`
	Severities          []string `json:"severities,omitempty"`
	ApplicationIDs      []string `json:"applicationIds,omitempty"`
	DeploymentTargetIDs []string `json:"deploymentTargetIds,omitempty"`
}

func DecodeRuleFilter(data []byte) (RuleFilter, error) {
	var filter RuleFilter
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&filter); err != nil {
		return RuleFilter{}, fmt.Errorf("%w: %v", ErrInvalidRuleFilter, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values are not allowed")
		}
		return RuleFilter{}, fmt.Errorf("%w: %v", ErrInvalidRuleFilter, err)
	}
	return ValidateRuleFilter(filter)
}

func ValidateRuleFilter(filter RuleFilter) (RuleFilter, error) {
	filter.Scope = strings.TrimSpace(filter.Scope)
	filter.ProjectIDs = normalizedRuleFilterValues(filter.ProjectIDs)
	filter.Severities = normalizedRuleFilterValues(filter.Severities)
	filter.ApplicationIDs = normalizedRuleFilterValues(filter.ApplicationIDs)
	filter.DeploymentTargetIDs = normalizedRuleFilterValues(filter.DeploymentTargetIDs)

	switch filter.Scope {
	case RuleScopeProjects:
		if len(filter.ProjectIDs) == 0 {
			return RuleFilter{}, fmt.Errorf("%w: projectIds are required for projects scope", ErrInvalidRuleFilter)
		}
	case RuleScopeAll:
		if len(filter.ProjectIDs) != 0 {
			return RuleFilter{}, fmt.Errorf("%w: projectIds must be empty for all scope", ErrInvalidRuleFilter)
		}
	default:
		return RuleFilter{}, fmt.Errorf("%w: unsupported scope", ErrInvalidRuleFilter)
	}
	for _, severity := range filter.Severities {
		switch severity {
		case SeverityInfo, SeverityWarning, SeverityError:
		default:
			return RuleFilter{}, fmt.Errorf("%w: unsupported severity", ErrInvalidRuleFilter)
		}
	}
	return filter, nil
}

func EncodeRuleFilter(filter RuleFilter) (string, error) {
	filter, err := ValidateRuleFilter(filter)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(filter)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func normalizedRuleFilterValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}
