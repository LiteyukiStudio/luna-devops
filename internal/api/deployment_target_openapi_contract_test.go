package api

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
)

func TestDeploymentTargetAgentSchemaCoversHandlerInput(t *testing.T) {
	for _, operationID := range []string{"createDeploymentTarget"} {
		t.Run(operationID, func(t *testing.T) {
			operation, ok := aitool.PlatformOperation(operationID)
			if !ok {
				t.Fatalf("%s is missing from the Agent platform catalog", operationID)
			}
			rootProperties := schemaProperties(t, operation.InputSchema)
			body, ok := rootProperties["body"].(map[string]any)
			if !ok {
				t.Fatalf("%s Agent schema is missing its body object", operationID)
			}
			properties := schemaProperties(t, body)

			want := make([]string, 0, reflect.TypeOf(deploymentTargetInput{}).NumField())
			inputType := reflect.TypeOf(deploymentTargetInput{})
			for index := 0; index < inputType.NumField(); index++ {
				field := inputType.Field(index)
				name := strings.Split(field.Tag.Get("json"), ",")[0]
				if name != "" && name != "-" {
					want = append(want, name)
					assertSchemaMatchesGoType(t, name, properties[name], field.Type)
				}
			}
			got := make([]string, 0, len(properties))
			for name := range properties {
				got = append(got, name)
			}
			sort.Strings(want)
			sort.Strings(got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("DeploymentTargetInput and Agent body schema differ\n got: %v\nwant: %v", got, want)
			}
		})
	}
}

func TestDeploymentTargetAgentSchemaRequiresRouteAndBodyArguments(t *testing.T) {
	tests := map[string][]string{
		"createDeploymentTarget": {"applicationId", "body", "projectId"},
	}
	for operationID, want := range tests {
		t.Run(operationID, func(t *testing.T) {
			operation, ok := aitool.PlatformOperation(operationID)
			if !ok {
				t.Fatalf("%s is missing from the Agent platform catalog", operationID)
			}
			got, ok := schemaStringList(operation.InputSchema["required"])
			if !ok {
				t.Fatalf("%s has no required argument list: %#v", operationID, operation.InputSchema)
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s required arguments = %v, want %v", operationID, got, want)
			}
		})
	}
}

func schemaStringList(raw any) ([]string, bool) {
	switch values := raw.(type) {
	case []string:
		return append([]string(nil), values...), true
	case []any:
		output := make([]string, 0, len(values))
		for _, value := range values {
			item, ok := value.(string)
			if !ok {
				return nil, false
			}
			output = append(output, item)
		}
		return output, true
	default:
		return nil, false
	}
}

func assertSchemaMatchesGoType(t *testing.T, name string, raw any, goType reflect.Type) {
	t.Helper()
	schema, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("schema property %q is missing", name)
	}
	for goType.Kind() == reflect.Pointer {
		goType = goType.Elem()
	}
	want := map[reflect.Kind]string{
		reflect.String: "string",
		reflect.Bool:   "boolean",
		reflect.Int:    "integer",
		reflect.Slice:  "array",
		reflect.Map:    "object",
	}[goType.Kind()]
	if want == "" {
		t.Fatalf("unsupported Go field type for %q: %s", name, goType)
	}
	if schemaTypeMatches(schema["type"], want) {
		return
	}
	t.Fatalf("schema property %q type = %#v, want %q for Go type %s", name, schema["type"], want, goType)
}

func schemaTypeMatches(raw any, want string) bool {
	if raw == want {
		return true
	}
	values, ok := raw.([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestDeploymentTargetAgentSchemaDescribesSourceAndStructuredFields(t *testing.T) {
	operation, ok := aitool.PlatformOperation("createDeploymentTarget")
	if !ok {
		t.Fatal("createDeploymentTarget is missing from the Agent platform catalog")
	}
	body := schemaProperties(t, operation.InputSchema)["body"].(map[string]any)
	properties := schemaProperties(t, body)

	assertSchemaEnum(t, properties, "sourceType", "repository", "image")
	stage, ok := properties["stage"].(map[string]any)
	if !ok || stage["pattern"] != "^(dev|test|staging|prod|sys-[a-z0-9-]+)$" || stage["default"] != "dev" {
		t.Fatalf("stage must accept public stages and persisted system stages: %#v", stage)
	}
	autoScalingMin, ok := properties["autoScalingMinReplicas"].(map[string]any)
	if !ok || autoScalingMin["minimum"] != float64(0) {
		t.Fatalf("autoScalingMinReplicas must allow scale-to-zero: %#v", autoScalingMin)
	}
	assertSchemaEnum(t, properties, "workloadType", "Deployment", "StatefulSet")

	servicePorts, ok := properties["servicePorts"].(map[string]any)
	if !ok || servicePorts["type"] != "array" {
		t.Fatal("servicePorts must be an array in the Agent schema")
	}
	items, ok := servicePorts["items"].(map[string]any)
	if !ok {
		t.Fatal("servicePorts must expose its item schema")
	}
	port, ok := schemaProperties(t, items)["port"].(map[string]any)
	if !ok || port["minimum"] != float64(1) || port["maximum"] != float64(65535) {
		t.Fatalf("servicePorts.port bounds are invalid: %#v", port)
	}

	buildVariables, ok := properties["buildVariables"].(map[string]any)
	if !ok {
		t.Fatal("buildVariables must be an object in the Agent schema")
	}
	additional, ok := buildVariables["additionalProperties"].(map[string]any)
	if !ok || additional["type"] != "string" {
		t.Fatalf("buildVariables must accept arbitrary string values: %#v", buildVariables)
	}
}

func schemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no object properties: %#v", schema)
	}
	return properties
}

func assertSchemaEnum(t *testing.T, properties map[string]any, name string, values ...string) {
	t.Helper()
	schema, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q is missing", name)
	}
	raw, ok := schema["enum"].([]any)
	if !ok {
		t.Fatalf("schema property %q has no enum: %#v", name, schema)
	}
	got := make([]string, 0, len(raw))
	for _, value := range raw {
		got = append(got, value.(string))
	}
	if !reflect.DeepEqual(got, values) {
		t.Fatalf("schema property %q enum = %v, want %v", name, got, values)
	}
}
