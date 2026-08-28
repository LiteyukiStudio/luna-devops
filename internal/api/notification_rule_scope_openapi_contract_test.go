package api

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNotificationRuleScopeOpenAPIContract(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	input := schemas["NotificationRuleInput"].(map[string]any)
	inputRequired, _ := schemaStringList(input["required"])
	for _, field := range []string{"name", "eventTypes", "filter", "channelIds"} {
		if !containsString(inputRequired, field) {
			t.Errorf("NotificationRuleInput does not require %s", field)
		}
	}
	inputProperties := input["properties"].(map[string]any)
	for _, field := range []string{"eventTypes", "channelIds"} {
		property := inputProperties[field].(map[string]any)
		if property["minItems"] != float64(1) || property["uniqueItems"] != true {
			t.Errorf("NotificationRuleInput.%s = %#v, want non-empty unique array", field, property)
		}
	}
	filterReference := inputProperties["filter"].(map[string]any)
	if filterReference["$ref"] != "#/components/schemas/NotificationRuleFilter" {
		t.Fatalf("NotificationRuleInput.filter = %#v", filterReference)
	}

	filter := schemas["NotificationRuleFilter"].(map[string]any)
	if filter["additionalProperties"] != false {
		t.Fatalf("NotificationRuleFilter must reject unknown fields: %#v", filter)
	}
	filterRequired, _ := schemaStringList(filter["required"])
	if !containsString(filterRequired, "scope") {
		t.Fatal("NotificationRuleFilter does not require scope")
	}
	filterProperties := filter["properties"].(map[string]any)
	scope := filterProperties["scope"].(map[string]any)
	if !reflect.DeepEqual(scope["enum"], []any{"projects", "all"}) {
		t.Fatalf("NotificationRuleFilter.scope = %#v", scope)
	}

	conditions := filter["allOf"].([]any)
	seen := map[string]bool{}
	for _, rawCondition := range conditions {
		condition := rawCondition.(map[string]any)
		ifSchema := condition["if"].(map[string]any)
		ifProperties := ifSchema["properties"].(map[string]any)
		ifScope := ifProperties["scope"].(map[string]any)
		scopeValue := ifScope["const"].(string)
		thenSchema := condition["then"].(map[string]any)
		thenProperties := thenSchema["properties"].(map[string]any)
		projectIDs := thenProperties["projectIds"].(map[string]any)
		switch scopeValue {
		case "projects":
			required, _ := schemaStringList(thenSchema["required"])
			if !containsString(required, "projectIds") || projectIDs["minItems"] != float64(1) {
				t.Errorf("projects scope condition = %#v", thenSchema)
			}
		case "all":
			if projectIDs["maxItems"] != float64(0) {
				t.Errorf("all scope condition = %#v", thenSchema)
			}
		default:
			t.Fatalf("unexpected notification scope condition %q", scopeValue)
		}
		seen[scopeValue] = true
	}
	if !seen["projects"] || !seen["all"] {
		t.Fatalf("notification scope conditions = %#v", seen)
	}
}

func TestPlatformEventOpenAPIRequiresResourceOwner(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	event := schemas["PlatformEvent"].(map[string]any)
	required, _ := schemaStringList(event["required"])
	if !containsString(required, "resourceOwnerUserId") {
		t.Fatal("PlatformEvent does not require resourceOwnerUserId")
	}
	properties := event["properties"].(map[string]any)
	resourceOwner := properties["resourceOwnerUserId"].(map[string]any)
	if resourceOwner["type"] != "string" {
		t.Fatalf("PlatformEvent.resourceOwnerUserId = %#v", resourceOwner)
	}
}
