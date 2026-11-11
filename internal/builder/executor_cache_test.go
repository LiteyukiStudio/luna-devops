package builder

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func executorSection(t *testing.T, name string) string {
	t.Helper()
	script := ExecutorScript()
	begin := "# --- " + name + ":begin ---"
	end := "# --- " + name + ":end ---"
	start := strings.Index(script, begin)
	stop := strings.Index(script, end)
	if start < 0 || stop <= start {
		t.Fatalf("executor section %q not found; keep its markers in run.sh", name)
	}
	return script[start:stop]
}

func runExecutorSection(t *testing.T, section string, env ...string) (string, error) {
	t.Helper()
	command := exec.Command("sh", "-c", "set -eu\nset --\n"+section+"\nprintf '%s\\n' \"$@\"")
	command.Env = append([]string{"PATH=/usr/bin:/bin"}, env...)
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestExecutorRegistryCacheArgs(t *testing.T) {
	section := executorSection(t, "registry-cache-args")
	base := []string{"CACHE_TAG=" + RegistryCacheTag, "IMAGE_REF=harbor.example.com:5000/team/api:main-1234"}
	const cacheRef = "type=registry,ref=harbor.example.com:5000/team/api:" + RegistryCacheTag
	const exportOptions = ",image-manifest=true,oci-mediatypes=true,ignore-error=true"

	for _, test := range []struct {
		name string
		env  []string
		want string
	}{
		{name: "disabled", env: []string{"CACHE_ENABLED=false"}, want: ""},
		{name: "without secrets exports intermediate stages", env: []string{"CACHE_ENABLED=true"}, want: "--import-cache\n" + cacheRef + "\n--export-cache\n" + cacheRef + ",mode=max" + exportOptions + "\n"},
		{name: "with secrets exports final layers only", env: []string{"CACHE_ENABLED=true", "BUILD_CONTAINS_SECRETS=true"}, want: "--import-cache\n" + cacheRef + "\n--export-cache\n" + cacheRef + ",mode=min" + exportOptions + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := runExecutorSection(t, section, append(base, test.env...)...)
			if err != nil {
				t.Fatalf("section failed: %v\n%s", err, output)
			}
			if strings.TrimLeft(output, "\n") != test.want {
				t.Fatalf("cache args = %q, want %q", output, test.want)
			}
		})
	}
}

func TestExecutorRejectsReservedCacheTag(t *testing.T) {
	section := executorSection(t, "reserved-cache-tag")
	output, err := runExecutorSection(t, section, "CACHE_TAG="+RegistryCacheTag, "IMAGE_REF=registry.example.com/team/api:"+RegistryCacheTag)
	if err == nil || !strings.Contains(output, "reserved for the build cache") {
		t.Fatalf("reserved cache tag should fail the build, err=%v output=%q", err, output)
	}
	if output, err := runExecutorSection(t, section, "CACHE_TAG="+RegistryCacheTag, "IMAGE_REF=registry.example.com/team/api:"+RegistryCacheTag+"-v2"); err != nil {
		t.Fatalf("similar tag should be accepted: %v\n%s", err, output)
	}
}

func TestExecutorUserVarsStayInsideHookSubshell(t *testing.T) {
	varsDir := t.TempDir()
	for name, value := range map[string]string{"IMAGE_REF": "registry.example.com/other/api:main", "NODE_ENV": "production"} {
		if err := os.WriteFile(filepath.Join(varsDir, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	section := strings.ReplaceAll(executorSection(t, "user-vars"), "/executor/vars", varsDir)
	section += `
hook_view="$(export_user_vars; printf '%s|%s' "$IMAGE_REF" "$NODE_ENV")"
printf '%s\n%s|%s\n%s\n' "$hook_view" "$IMAGE_REF" "${NODE_ENV:-}" "$(user_var_value IMAGE_REF)"
`
	output, err := runExecutorSection(t, section, "IMAGE_REF=registry.example.com/team/api:main", "BUILD_ENV_KEYS=NODE_ENV", "BUILD_ARG_KEYS=IMAGE_REF")
	if err != nil {
		t.Fatalf("section failed: %v\n%s", err, output)
	}
	want := "registry.example.com/other/api:main|production\nregistry.example.com/team/api:main|\nregistry.example.com/other/api:main\n"
	if strings.TrimRight(output, "\n")+"\n" != want {
		t.Fatalf("user vars output = %q, want %q", output, want)
	}
}
