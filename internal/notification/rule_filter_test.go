package notification

import (
	"errors"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestRuleFilterRequiresExplicitScopeAndRejectsUnknownFields(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "missing scope", raw: `{}`},
		{name: "projects without projects", raw: `{"scope":"projects"}`},
		{name: "all with projects", raw: `{"scope":"all","projectIds":["prj_1"]}`},
		{name: "unknown scope", raw: `{"scope":"mine"}`},
		{name: "unknown field", raw: `{"scope":"all","visibility":"all"}`},
		{name: "invalid severity", raw: `{"scope":"all","severities":["critical"]}`},
		{name: "multiple values", raw: `{"scope":"all"} {"scope":"all"}`},
		{name: "malformed", raw: `{"scope":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeRuleFilter([]byte(tt.raw)); !errors.Is(err, ErrInvalidRuleFilter) {
				t.Fatalf("DecodeRuleFilter(%s) error = %v", tt.raw, err)
			}
		})
	}
}

func TestRuleMatchesEventUsesExplicitProjectScope(t *testing.T) {
	event := Event{
		Type:             "build.failed",
		Severity:         SeverityError,
		Project:          EntityRef{ID: "prj_related"},
		Application:      EntityRef{ID: "app_related"},
		DeploymentTarget: EntityRef{ID: "dep_related"},
	}
	rule := model.NotificationRule{EventTypesJSON: EncodeStringList([]string{event.Type})}

	projectsFilter, err := EncodeRuleFilter(RuleFilter{Scope: RuleScopeProjects, ProjectIDs: []string{" prj_related ", "prj_related"}})
	if err != nil {
		t.Fatalf("encode projects filter: %v", err)
	}
	rule.FilterJSON = projectsFilter
	if !ruleMatchesEvent(rule, event) {
		t.Fatal("explicit related project did not match")
	}
	event.Project.ID = "prj_other"
	if ruleMatchesEvent(rule, event) {
		t.Fatal("unselected project unexpectedly matched")
	}
	event.Project.ID = ""
	if ruleMatchesEvent(rule, event) {
		t.Fatal("projectless event unexpectedly matched projects scope")
	}

	allFilter, err := EncodeRuleFilter(RuleFilter{Scope: RuleScopeAll})
	if err != nil {
		t.Fatalf("encode all filter: %v", err)
	}
	rule.FilterJSON = allFilter
	if !ruleMatchesEvent(rule, event) {
		t.Fatal("explicit all scope did not match projectless event")
	}
}

func TestRuleMatchesEventFailsClosedForInvalidOrEmptyRule(t *testing.T) {
	event := Event{Type: "build.failed", Severity: SeverityError, Project: EntityRef{ID: "prj_1"}}
	tests := []model.NotificationRule{
		{EventTypesJSON: EncodeStringList([]string{event.Type}), FilterJSON: `{}`},
		{EventTypesJSON: EncodeStringList([]string{event.Type}), FilterJSON: `{"scope":"all","unknown":true}`},
		{EventTypesJSON: EncodeStringList([]string{event.Type}), FilterJSON: `{"scope":`},
		{EventTypesJSON: `[]`, FilterJSON: `{"scope":"all"}`},
	}
	for index, rule := range tests {
		if ruleMatchesEvent(rule, event) {
			t.Fatalf("invalid rule %d unexpectedly matched", index)
		}
	}
}
