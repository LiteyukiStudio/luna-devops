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
	template := Template{ID: "invalid", Workload: Workload{DataVolumes: []DataVolume{
		{LogicalName: "data", SourceType: "projectVolume", MountPath: "/data"},
		{LogicalName: "backup", SourceType: "projectVolume", MountPath: "/backup"},
	}}}
	if err := validateTemplateWorkloads(template); err == nil {
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

	valuesWithoutPassword, err := ResolveValues(template, map[string]string{"password": ""})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := valuesWithoutPassword["password"]; exists {
		t.Fatalf("blank optional password remained in rendered values: %#v", valuesWithoutPassword)
	}
	withoutPassword := RenderWorkload(template.Workload, valuesWithoutPassword)
	if _, exists := withoutPassword.SecretEnv["REDIS_PASSWORD"]; exists {
		t.Fatalf("blank optional password produced a secret environment variable: %#v", withoutPassword.SecretEnv)
	}

	valuesWithPassword, err := ResolveValues(template, map[string]string{"password": "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	withPassword := RenderWorkload(template.Workload, valuesWithPassword)
	if valuesWithPassword["password"] != "test-password" || withPassword.SecretEnv["REDIS_PASSWORD"] != "test-password" {
		t.Fatalf("rendered redis password = values %#v secretEnv %#v", valuesWithPassword, withPassword.SecretEnv)
	}
	if _, exists := withPassword.Env["REDIS_PASSWORD"]; exists {
		t.Fatalf("redis password leaked into plain environment variables: %#v", withPassword.Env)
	}
}

func TestNeoBlogTemplateSharesGeneratedSecretsAcrossComponents(t *testing.T) {
	template, found, err := Find("neo-blog")
	if err != nil {
		t.Fatal(err)
	}
	if !found || !template.IsMultiComponent() || template.Image != "" {
		t.Fatalf("neo-blog template = %#v", template)
	}
	keys := make([]string, 0, len(template.Components))
	for _, component := range template.Components {
		keys = append(keys, component.Key)
	}
	if !slices.Equal(keys, []string{"web", "api", "postgres", "redis"}) {
		t.Fatalf("neo-blog component keys = %v", keys)
	}

	values, err := ResolveValues(template, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"databasePassword", "redisPassword", "passwordSalt", "jwtSecret"} {
		if len(values[key]) < 32 {
			t.Fatalf("generated %s is too short for neo-blog production checks: %d", key, len(values[key]))
		}
	}
	for _, component := range template.Components {
		values[ComponentHostValueKey(component.Key)] = "luna-blog-" + component.Key + "-prod"
	}
	rendered := map[string]RenderedWorkload{}
	for _, component := range template.Components {
		rendered[component.Key] = RenderWorkload(component.Workload, values)
	}
	if rendered["postgres"].SecretEnv["POSTGRES_PASSWORD"] != values["databasePassword"] ||
		!strings.Contains(rendered["api"].SecretEnv["DB_URL"], ":"+values["databasePassword"]+"@luna-blog-postgres-prod:5432/") {
		t.Fatalf("database password is not shared: postgres=%#v api=%#v", rendered["postgres"].SecretEnv, rendered["api"].SecretEnv)
	}
	redisURL := "redis://:" + values["redisPassword"] + "@luna-blog-redis-prod:6379/0"
	if rendered["redis"].SecretEnv["REDIS_PASSWORD"] != values["redisPassword"] || rendered["api"].SecretEnv["REDIS_URL"] != redisURL || rendered["web"].SecretEnv["REDIS_URL"] != redisURL {
		t.Fatalf("redis password is not shared across redis, api, and web")
	}
	if rendered["web"].Env["BACKEND_URL"] != "http://luna-blog-api-prod:8888" {
		t.Fatalf("frontend backend url = %q", rendered["web"].Env["BACKEND_URL"])
	}
	for key, workload := range rendered {
		for name, value := range workload.Env {
			for _, secretKey := range []string{"databasePassword", "redisPassword", "passwordSalt", "jwtSecret"} {
				if strings.Contains(value, values[secretKey]) {
					t.Fatalf("component %s leaked %s into plain env %s", key, secretKey, name)
				}
			}
		}
	}
}

func TestTemplatePlaceholderEscapesURLUserinfo(t *testing.T) {
	got := renderTemplateString("redis://:{{ password | urlUserinfo }}@redis:6379/0", map[string]string{"password": "p@ss:w/rd?#"})
	if got != "redis://:p%40ss%3Aw%2Frd%3F%23@redis:6379/0" {
		t.Fatalf("escaped url = %q", got)
	}
}

func TestValidateTemplateWorkloadsRejectsInvalidComponents(t *testing.T) {
	component := func(key string, env map[string]string) Component {
		return Component{Key: key, Name: key, Workload: Workload{Image: "example/" + key, ServicePort: 80, Env: env}}
	}
	for name, template := range map[string]Template{
		"duplicate key":       {Components: []Component{component("web", nil), component("web", nil)}},
		"invalid key":         {Components: []Component{component("Web-1", nil)}},
		"unknown reference":   {Components: []Component{component("web", map[string]string{"API": "{{components.api.host}}"})}},
		"template workload":   {Workload: Workload{Image: "example/app"}, Components: []Component{component("web", nil)}},
		"missing image":       {Components: []Component{{Key: "web", Name: "Web", Workload: Workload{ServicePort: 80}}}},
		"two project volumes": {Components: []Component{{Key: "web", Name: "Web", Workload: Workload{Image: "example/web", ServicePort: 80, DataVolumes: []DataVolume{{LogicalName: "a", SourceType: "projectVolume", MountPath: "/a"}, {LogicalName: "b", SourceType: "projectVolume", MountPath: "/b"}}}}}},
	} {
		if err := validateTemplateWorkloads(template); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	valid := Template{Components: []Component{component("web", map[string]string{"API": "http://{{components.api.host}}:80"}), component("api", nil)}}
	if err := validateTemplateWorkloads(valid); err != nil {
		t.Fatalf("valid components rejected: %v", err)
	}
}

func TestCatalogFeaturesOnlyCuratedApplicationTemplates(t *testing.T) {
	templates, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}

	featured := make([]string, 0)
	for _, template := range templates {
		if !template.Featured {
			continue
		}
		if template.Kind == "system_component" {
			t.Fatalf("system component template %q must not be featured", template.ID)
		}
		featured = append(featured, template.ID)
	}
	slices.Sort(featured)
	want := []string{"gitea", "neo-blog", "postgresql", "redis"}
	if !slices.Equal(featured, want) {
		t.Fatalf("featured templates = %v, want %v", featured, want)
	}
}
