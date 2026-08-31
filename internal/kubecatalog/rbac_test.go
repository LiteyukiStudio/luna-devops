package kubecatalog

import (
	"reflect"
	"testing"
)

func TestRBACContainsNoWildcardsOrLocalClusterResources(t *testing.T) {
	for _, rule := range New().ProjectClusterRoleRules() {
		for _, group := range rule.APIGroups {
			if group == "*" {
				t.Fatal("wildcard API group is forbidden")
			}
		}
		for _, resource := range rule.Resources {
			if resource == "*" || resource == "namespaces" || resource == "storageclasses" {
				t.Fatalf("unexpected cluster role resource %q", resource)
			}
		}
		for _, verb := range rule.Verbs {
			if verb == "*" {
				t.Fatal("wildcard verb is forbidden")
			}
		}
	}
}

func TestDiscoveryRBACIsFixedReadOnly(t *testing.T) {
	rules := DiscoveryClusterRoleRules()
	if len(rules) != 1 || len(rules[0].Verbs) != 1 || rules[0].Verbs[0] != "get" {
		t.Fatalf("unexpected discovery rules: %#v", rules)
	}
}

func TestControllerRevisionRBACIsReadOnly(t *testing.T) {
	for _, rule := range New().ProjectClusterRoleRules() {
		if len(rule.APIGroups) == 1 && rule.APIGroups[0] == "apps" && len(rule.Resources) == 1 && rule.Resources[0] == "controllerrevisions" {
			if !reflect.DeepEqual(rule.Verbs, []string{"get", "list", "watch"}) {
				t.Fatalf("ControllerRevision RBAC must remain read-only: %#v", rule.Verbs)
			}
			return
		}
	}
	t.Fatal("ControllerRevision RBAC rule missing")
}
