package appstore

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestCatalogUsesConsolidatedCategories(t *testing.T) {
	templates, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"collaboration", "database", "developerTool", "middleware", "observability", "security", "storage"}
	categorySet := make(map[string]struct{}, len(want))
	for _, template := range templates {
		categorySet[template.Category] = struct{}{}
	}
	categories := make([]string, 0, len(categorySet))
	for category := range categorySet {
		categories = append(categories, category)
	}
	slices.Sort(categories)
	if !slices.Equal(categories, want) {
		t.Fatalf("catalog categories = %v, want %v", categories, want)
	}
}

func TestCatalogUsesTypedDataVolumeDeclarations(t *testing.T) {
	raw, err := templateFS.ReadFile("templates.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range [][]byte{[]byte(`"dataRetentionEnabled"`), []byte(`"dataMountPath"`), []byte(`"dataCapacity"`)} {
		if bytes.Contains(raw, legacy) {
			t.Fatalf("catalog still contains legacy volume property %s", legacy)
		}
	}

	templates, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	persistentTemplates := 0
	for _, template := range templates {
		for _, dataVolume := range template.DataVolumes {
			if dataVolume.SourceType == "projectVolume" {
				persistentTemplates++
				if dataVolume.LogicalName == "" || dataVolume.MountPath == "" {
					t.Fatalf("template %s has an incomplete projectVolume declaration: %#v", template.ID, dataVolume)
				}
			}
		}
	}
	if persistentTemplates == 0 {
		t.Fatal("catalog has no persistent templates to exercise project volume installation")
	}
}

func TestValidateTemplateDataVolumesRejectsAmbiguousProjectVolumeBindings(t *testing.T) {
	template := Template{ID: "invalid", DataVolumes: []DataVolume{
		{LogicalName: "data", SourceType: "projectVolume", MountPath: "/data"},
		{LogicalName: "backup", SourceType: "projectVolume", MountPath: "/backup"},
	}}
	if err := validateTemplateDataVolumes(template); err == nil {
		t.Fatal("multiple projectVolume declarations were accepted even though install accepts one projectVolumeId")
	}
}

func TestRedisTemplateRendersOptionalPasswordAsSecret(t *testing.T) {
	template, found, err := Find("redis")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("redis template not found")
	}
	if len(template.Values) != 1 {
		t.Fatalf("redis values = %#v", template.Values)
	}
	password := template.Values[0]
	if password.Key != "password" || !password.Secret || password.Required || password.AutoGenerate {
		t.Fatalf("redis password definition = %#v", password)
	}
	if template.ContainerCommand != "/bin/sh\n-ec" || !strings.Contains(template.ContainerArgs, "--requirepass") || strings.Contains(template.ContainerArgs, "test-password") {
		t.Fatalf("redis startup configuration = %q %#v", template.ContainerCommand, template.ContainerArgs)
	}

	withoutPassword, err := Render(template, map[string]string{"password": ""})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := withoutPassword.Values["password"]; exists {
		t.Fatalf("blank optional password remained in rendered values: %#v", withoutPassword.Values)
	}
	if _, exists := withoutPassword.SecretEnv["REDIS_PASSWORD"]; exists {
		t.Fatalf("blank optional password produced a secret environment variable: %#v", withoutPassword.SecretEnv)
	}

	withPassword, err := Render(template, map[string]string{"password": "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	if withPassword.Values["password"] != "test-password" || withPassword.SecretEnv["REDIS_PASSWORD"] != "test-password" {
		t.Fatalf("rendered redis password = values %#v secretEnv %#v", withPassword.Values, withPassword.SecretEnv)
	}
	if _, exists := withPassword.Env["REDIS_PASSWORD"]; exists {
		t.Fatalf("redis password leaked into plain environment variables: %#v", withPassword.Env)
	}
}
