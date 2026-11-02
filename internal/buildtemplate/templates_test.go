package buildtemplate

import (
	"strings"
	"testing"
)

func TestBuiltInTemplatesRenderWithDefaults(t *testing.T) {
	for _, definition := range List() {
		t.Run(definition.ID, func(t *testing.T) {
			preview, err := Render(definition.ID, definition.Version, nil)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if !strings.HasPrefix(preview.Dockerfile, "FROM ") {
				t.Fatalf("Dockerfile = %q", preview.Dockerfile)
			}
			if strings.Contains(preview.Dockerfile, "{{") {
				t.Fatalf("Dockerfile contains an unresolved template expression: %q", preview.Dockerfile)
			}
			if len(preview.Checksum) != 64 {
				t.Fatalf("checksum length = %d", len(preview.Checksum))
			}
		})
	}
}

func TestRenderEscapesRuntimeCommandAsJSON(t *testing.T) {
	preview, err := Render("node-service", "", map[string]string{
		"startCommand": `node -e "console.log('ready')"`,
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(preview.Dockerfile, `CMD ["sh", "-c", "node -e \"console.log('ready')\""]`) {
		t.Fatalf("runtime command was not JSON escaped: %s", preview.Dockerfile)
	}
}

func TestNormalizeValuesRejectsUnsafeOrUnknownValues(t *testing.T) {
	definition, ok := Find("static-site", "")
	if !ok {
		t.Fatal("static-site template not found")
	}
	for name, raw := range map[string]string{
		"parent path":   `{"sourceDirectory":"../private"}`,
		"absolute path": `{"sourceDirectory":"/private"}`,
		"unknown key":   `{"extra":"value"}`,
		"newline":       `{"sourceDirectory":"public\\nRUN whoami"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeValues(definition, raw); err == nil {
				t.Fatal("NormalizeValues() error = nil")
			}
		})
	}
}

func TestRenderRejectsPortWithTrailingCharacters(t *testing.T) {
	if _, err := Render("go-service", "", map[string]string{"port": "8080abc"}); err == nil {
		t.Fatal("Render() accepted a port with trailing characters")
	}
}

func TestRecommendPrefersMoreSpecificTemplate(t *testing.T) {
	got := Recommend([]string{"package.json", "vite.config.ts", "src/main.ts"})
	if len(got) < 2 || got[0] != "node-static" || got[1] != "node-service" {
		t.Fatalf("Recommend() = %#v", got)
	}
}

func TestRecommendPrefersNextJSService(t *testing.T) {
	got := Recommend([]string{"package.json", "pnpm-lock.yaml", "next.config.ts", "app/page.tsx"})
	if len(got) < 2 || got[0] != "nextjs-service" {
		t.Fatalf("Recommend() = %#v", got)
	}
}

func TestNextJSServiceUsesOfficialStandaloneContainerPattern(t *testing.T) {
	preview, err := Render("nextjs-service", "", nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	for _, wanted := range []string{
		"FROM node:24-slim AS dependencies",
		"pnpm install --frozen-lockfile",
		"/app/.next/standalone",
		"/app/.next/static",
		"USER nextjs",
		`CMD ["node", "server.js"]`,
	} {
		if !strings.Contains(preview.Dockerfile, wanted) {
			t.Fatalf("Dockerfile does not contain %q:\n%s", wanted, preview.Dockerfile)
		}
	}
}

func TestRecommendRecognizesAdditionalRuntimes(t *testing.T) {
	for name, files := range map[string][]string{
		"bun-service":    {"package.json", "bun.lock"},
		"dotnet-service": {"src/WebApp/WebApp.csproj"},
		"java-gradle":    {"build.gradle.kts", "gradlew"},
		"java-maven":     {"pom.xml"},
		"ruby-service":   {"Gemfile", "Gemfile.lock"},
	} {
		t.Run(name, func(t *testing.T) {
			got := Recommend(files)
			if len(got) == 0 || got[0] != name {
				t.Fatalf("Recommend() = %#v, want %q first", got, name)
			}
		})
	}
}
