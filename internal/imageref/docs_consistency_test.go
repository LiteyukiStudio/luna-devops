package imageref_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/imageref"
)

// repositoryRoot 是本包目录到仓库根的相对路径；go test 以包目录为工作目录。
const repositoryRoot = "../.."

var repositoryTemplateVariableDocs = []string{
	"docs/docs/zh/reference/image-templates.md",
	"docs/docs/en/reference/image-templates.md",
}

func TestRepositoryTemplateVariablesAreDocumented(t *testing.T) {
	names := imageref.RepositoryTemplateVariables()
	if len(names) == 0 {
		t.Fatal("RepositoryTemplateVariables() returned no variables")
	}
	for _, relativePath := range repositoryTemplateVariableDocs {
		content, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		for _, name := range names {
			if !strings.Contains(string(content), "`{"+name+"}`") {
				t.Errorf("%s does not document repository template variable {%s}", relativePath, name)
			}
		}
	}
}
