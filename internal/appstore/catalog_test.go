package appstore

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
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
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	fileCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fileCount++
		raw, err := templateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, legacy := range [][]byte{[]byte(`"dataRetentionEnabled"`), []byte(`"dataMountPath"`), []byte(`"dataCapacity"`)} {
			if bytes.Contains(raw, legacy) {
				t.Fatalf("catalog file %s still contains legacy volume property %s", entry.Name(), legacy)
			}
		}
	}
	if fileCount < 30 {
		t.Fatalf("embedded template file count = %d, expected split catalog files", fileCount)
	}

	templates, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != fileCount {
		t.Fatalf("Catalog() len = %d, want %d", len(templates), fileCount)
	}
	persistentTemplates := 0
	for _, template := range templates {
		if template.Source != TemplateSourceBuiltin {
			t.Fatalf("template %s source = %q, want %q", template.ID, template.Source, TemplateSourceBuiltin)
		}
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
		"self reference":      {Components: []Component{component("web", map[string]string{"SELF": "{{components.web.host}}"})}},
		"binding env clash":   {Components: []Component{component("web", map[string]string{"API_HOST": "{{components.api.host}}"}), component("api", nil)}},
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

func TestResolveValuesFailsWhenRandomSecretCannotBeGenerated(t *testing.T) {
	original := randomSource
	randomSource = iotest.ErrReader(errors.New("entropy unavailable"))
	t.Cleanup(func() { randomSource = original })
	template := Template{Values: []ValueDefinition{{Key: "password", Secret: true, AutoGenerate: true}}}
	values, err := ResolveValues(template, nil)
	if !errors.Is(err, ErrRandomSecretUnavailable) || values != nil {
		t.Fatalf("ResolveValues() = %#v, %v; want ErrRandomSecretUnavailable", values, err)
	}
}

func TestResolveValuesReportsMissingRequiredKey(t *testing.T) {
	template := Template{Values: []ValueDefinition{{Key: "baseUrl", Required: true}}}
	_, err := ResolveValues(template, map[string]string{"baseUrl": "  "})
	var required *ValueRequiredError
	if !errors.As(err, &required) || required.Key != "baseUrl" {
		t.Fatalf("ResolveValues() error = %v, want ValueRequiredError for baseUrl", err)
	}
}

func TestNeoBlogComponentDependenciesAreDeclaredOnce(t *testing.T) {
	template, found, err := Find("neo-blog")
	if err != nil || !found {
		t.Fatalf("Find(neo-blog) found=%t err=%v", found, err)
	}
	got := map[string][]string{}
	for _, component := range template.Components {
		got[component.Key] = ComponentDependencies(component.Workload)
	}
	want := map[string][]string{"web": {"api", "redis"}, "api": {"postgres", "redis"}, "postgres": nil, "redis": nil}
	for key, dependencies := range want {
		if !slices.Equal(got[key], dependencies) {
			t.Fatalf("%s dependencies = %v, want %v", key, got[key], dependencies)
		}
	}
	if host, port := ComponentBindingEnvVars("postgres"); host != "POSTGRES_HOST" || port != "POSTGRES_PORT" {
		t.Fatalf("binding env vars = %s/%s", host, port)
	}
}

func TestValidatePlatformTemplateAndRoundTripSpec(t *testing.T) {
	single := Template{
		Slug:        "Internal-Portal",
		Name:        "Internal Portal",
		Description: "Company portal service",
		Category:    "collaboration",
		Version:     "2.1.0",
		Workload: Workload{
			Image:       "ghcr.io/liteyuki/portal:2.1.0",
			ServicePort: 8080,
			Env:         map[string]string{"PUBLIC_URL": "{{baseUrl}}"},
			SecretEnv:   map[string]string{"PORTAL_SECRET": "{{secretKey}}"},
		},
		Values: []ValueDefinition{
			{Key: "baseUrl", Label: "Base URL", Required: true},
			{Key: "secretKey", Label: "Secret Key", Secret: true, AutoGenerate: true},
		},
	}
	record, validated, err := ModelFromPlatformTemplate("ptpl_portal", "usr_admin", true, single)
	if err != nil {
		t.Fatalf("ModelFromPlatformTemplate(single) error = %v", err)
	}
	if record.Slug != "internal-portal" || record.IsMultiComponent || validated.Source != TemplateSourcePlatform {
		t.Fatalf("record = %#v, validated = %#v", record, validated)
	}
	restored, err := TemplateFromPlatformModel(record)
	if err != nil {
		t.Fatalf("TemplateFromPlatformModel error = %v", err)
	}
	if restored.Image != "ghcr.io/liteyuki/portal:2.1.0" || restored.DefaultReplicas != 1 || len(restored.Values) != 2 {
		t.Fatalf("restored = %#v", restored)
	}

	multi := Template{
		Slug:     "suite-app",
		Name:     "Suite App",
		Category: "developerTool",
		Components: []Component{
			{
				Key:  "web",
				Name: "Web",
				Workload: Workload{
					Image:       "ghcr.io/liteyuki/web:1.0.0",
					ServicePort: 3000,
					Env:         map[string]string{"API_ORIGIN": "http://{{components.api.host}}:8080"},
				},
			},
			{
				Key:  "api",
				Name: "API",
				Workload: Workload{
					Image:       "ghcr.io/liteyuki/api:1.0.0",
					ServicePort: 8080,
					SecretEnv:   map[string]string{"API_TOKEN": "{{apiToken}}"},
				},
			},
		},
		Values: []ValueDefinition{{Key: "apiToken", Secret: true, AutoGenerate: true}},
	}
	multiRecord, _, err := ModelFromPlatformTemplate("ptpl_suite", "usr_admin", true, multi)
	if err != nil || !multiRecord.IsMultiComponent {
		t.Fatalf("ModelFromPlatformTemplate(multi) record=%#v err=%v", multiRecord, err)
	}

	// Built-in bundle export strips secret defaults and passes platform validation under a new slug
	mongoExpress, found, err := Find("mongo-express")
	if err != nil || !found {
		t.Fatalf("Find(mongo-express) found=%t err=%v", found, err)
	}
	bundle := BundleFromTemplate(mongoExpress, true)
	bundle.Slug = "custom-mongo-express"
	if _, err := ValidatePlatformTemplate(bundle.ToTemplate("ptpl_mexp")); err != nil {
		t.Fatalf("exported bundle failed platform validation: %v", err)
	}

	invalidCases := map[string]Template{
		"system component kind": {
			Slug: "bad-sys", Name: "Bad", Category: "observability", Kind: "system_component",
			Workload: Workload{Image: "example/probe:1", ServicePort: 8080},
		},
		"secret default": {
			Slug: "bad-secret-default", Name: "Bad", Category: "database",
			Workload: Workload{Image: "example/db:1", ServicePort: 5432, SecretEnv: map[string]string{"PW": "{{pw}}"}},
			Values:   []ValueDefinition{{Key: "pw", Secret: true, Default: "plaintext"}},
		},
		"secret leaked into plain env": {
			Slug: "bad-secret-leak", Name: "Bad", Category: "database",
			Workload: Workload{Image: "example/db:1", ServicePort: 5432, Env: map[string]string{"PW": "{{pw}}"}},
			Values:   []ValueDefinition{{Key: "pw", Secret: true, AutoGenerate: true}},
		},
		"literal secretEnv without placeholder": {
			Slug: "bad-literal-secret", Name: "Bad", Category: "database",
			Workload: Workload{Image: "example/db:1", ServicePort: 5432, SecretEnv: map[string]string{"PW": "hardcoded"}},
		},
		"undeclared placeholder": {
			Slug: "bad-placeholder", Name: "Bad", Category: "database",
			Workload: Workload{Image: "example/db:1", ServicePort: 5432, Env: map[string]string{"FOO": "{{missing}}"}},
		},
	}
	for name, invalid := range invalidCases {
		if _, err := ValidatePlatformTemplate(invalid); err == nil {
			t.Fatalf("expected %s to fail validation", name)
		}
	}
}
