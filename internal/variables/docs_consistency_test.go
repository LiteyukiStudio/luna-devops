package variables_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/variables"
)

// repositoryRoot 是本包目录到仓库根的相对路径；go test 以包目录为工作目录。
const repositoryRoot = "../.."

// 构建时模板变量会出现在镜像 Tag 与 Build Args 两页参考文档中，中英文都必须列出每个变量。
var buildTemplateVariableDocs = []string{
	"docs/docs/zh/reference/image-templates.md",
	"docs/docs/en/reference/image-templates.md",
	"docs/docs/zh/reference/build-variables.md",
	"docs/docs/en/reference/build-variables.md",
}

func TestBuildTemplateVariablesAreDocumented(t *testing.T) {
	names := variables.BuildTemplateVariables()
	if len(names) == 0 {
		t.Fatal("BuildTemplateVariables() returned no variables")
	}
	for _, relativePath := range buildTemplateVariableDocs {
		content, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		for _, name := range names {
			if !strings.Contains(string(content), "`"+name+"`") {
				t.Errorf("%s does not document build template variable %s", relativePath, name)
			}
		}
	}
}

// 构建执行器在检出代码后重新渲染镜像 Tag 与 Build Args；它必须识别 Go 侧定义的每个变量。
func TestBuildTemplateVariablesAreRenderedByExecutor(t *testing.T) {
	script := builder.ExecutorScript()
	for _, name := range variables.BuildTemplateVariables() {
		if !strings.Contains(script, "replace_token '"+name+"'") {
			t.Errorf("builder executor does not render build template variable %s", name)
		}
	}
}
