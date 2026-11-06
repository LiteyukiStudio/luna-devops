package imageref_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/imageref"
	"github.com/LiteyukiStudio/devops/internal/variables"
)

// 镜像 Tag 由构建执行器在检出代码后实际渲染，Go 侧渲染用于预估镜像地址与通知；两侧必须逐字一致。
// 这里抽取 run.sh 的渲染函数实际执行，避免两侧变量方言再次漂移（曾出现 {branch} 只在 Go 侧生效）。
func TestExecutorRendersImageTagsLikeGo(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not available")
	}
	script := builder.ExecutorScript()
	block := regexp.MustCompile(`(?s)# --- build-template-render:begin ---.*# --- build-template-render:end ---`).FindString(script)
	shortCommit := regexp.MustCompile(`(?s)\nshort_commit\(\) \{.*?\n\}\n`).FindString(script)
	if block == "" || shortCommit == "" {
		t.Fatal("executor render functions not found; keep the build-template-render markers in run.sh")
	}
	library := filepath.Join(t.TempDir(), "render.sh")
	if err := os.WriteFile(library, []byte(shortCommit+"\n"+block+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	templates := []string{"latest"}
	for _, variable := range variables.BuildTemplateVariables() {
		templates = append(templates, variable, "build-"+variable+"-x")
	}
	templates = append(templates, "{branchSlug}-{shortSha}", "${{ github.ref_name }}-{short_sha}", "{tag}", "{unknown}")
	contexts := []variables.Context{
		{SourceBranch: "Feature/Login_v2", SourceCommit: "0123456789abcdef0123456789abcdef01234567"},
		{SourceTag: "v1.2.0", SourceBranch: "main", SourceCommit: "fedcba9876543210fedcba9876543210fedcba98"},
		{SourceBranch: "--Release.2026--", SourceCommit: "abc"},
		{},
	}
	for _, context := range contexts {
		for _, template := range templates {
			command := exec.Command(shell, "-c", `. "$1"; render_image_tag`, "render", library)
			command.Env = append(os.Environ(),
				"IMAGE_TAG_TEMPLATE="+template,
				"CHECKED_OUT_COMMIT="+context.SourceCommit,
				"SOURCE_BRANCH="+context.SourceBranch,
				"SOURCE_TAG="+context.SourceTag,
			)
			output, err := command.Output()
			if err != nil {
				t.Fatalf("executor render %q failed: %v", template, err)
			}
			if got, want := strings.TrimSpace(string(output)), imageref.RenderBuildTagTemplate(template, context); got != want {
				t.Errorf("template %q with %+v: executor = %q, go = %q", template, context, got, want)
			}
		}
	}
}
