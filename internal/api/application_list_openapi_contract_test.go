package api

import (
	"path/filepath"
	"testing"
)

func TestApplicationListItemOpenAPIDescribesAccessURLAndCurrentRelease(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	components, _ := document["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)

	item, _ := schemas["ApplicationListItem"].(map[string]any)
	properties, _ := item["properties"].(map[string]any)
	accessURL, _ := properties["primaryAccessUrl"].(map[string]any)
	if accessURL["type"] != "string" {
		t.Fatalf("ApplicationListItem.primaryAccessUrl = %#v, want string", accessURL)
	}
	currentRelease, _ := properties["currentRelease"].(map[string]any)
	if currentRelease["$ref"] != "#/components/schemas/ApplicationCurrentRelease" {
		t.Fatalf("ApplicationListItem.currentRelease = %#v", currentRelease)
	}
	required, _ := schemaStringList(item["required"])
	for _, optional := range []string{"primaryAccessUrl", "currentRelease"} {
		if containsString(required, optional) {
			t.Fatalf("ApplicationListItem must keep %s optional because it is only returned with includeRuntime", optional)
		}
	}

	release, _ := schemas["ApplicationCurrentRelease"].(map[string]any)
	releaseRequired, _ := schemaStringList(release["required"])
	for _, field := range []string{"id", "deploymentTargetId", "imageRef", "deployedAt"} {
		if !containsString(releaseRequired, field) {
			t.Fatalf("ApplicationCurrentRelease does not require %s", field)
		}
	}
}

func TestAppTemplateOpenAPIRequiresFeaturedFlag(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	components, _ := document["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	for _, name := range []string{"AppTemplateSummary", "AppTemplate"} {
		schema, _ := schemas[name].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		featured, _ := properties["featured"].(map[string]any)
		if featured["type"] != "boolean" {
			t.Fatalf("%s.featured = %#v, want boolean", name, featured)
		}
		required, _ := schemaStringList(schema["required"])
		if !containsString(required, "featured") {
			t.Fatalf("%s does not require featured", name)
		}
	}
}
