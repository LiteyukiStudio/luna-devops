package variables

import (
	"maps"
	"sort"
	"testing"
)

func TestExpandEnvRefsBasic(t *testing.T) {
	input := map[string]string{
		"USER":         "blog",
		"PASSWORD":     "s3cret",
		"HOST":         "localhost",
		"PORT":         "5432",
		"DB":           "blogdb",
		"DATABASE_URL": "postgresql://${USER}:${PASSWORD}@${HOST}:${PORT}/${DB}",
	}
	got := ExpandEnvRefs(input)
	want := "postgresql://blog:s3cret@localhost:5432/blogdb"
	if got["DATABASE_URL"] != want {
		t.Fatalf("DATABASE_URL = %q, want %q", got["DATABASE_URL"], want)
	}
}

func TestExpandEnvRefsChain(t *testing.T) {
	input := map[string]string{
		"A": "${B}",
		"B": "${C}",
		"C": "final",
	}
	got := ExpandEnvRefs(input)
	if got["A"] != "final" {
		t.Fatalf("chain A = %q, want %q", got["A"], "final")
	}
}

func TestExpandEnvRefsSelfReference(t *testing.T) {
	input := map[string]string{
		"A": "${A}",
		"B": "pre_${B}_post",
	}
	got := ExpandEnvRefs(input)
	if got["A"] != "${A}" {
		t.Fatalf("self-ref A = %q, should remain literal", got["A"])
	}
	if got["B"] != "pre_${B}_post" {
		t.Fatalf("self-ref B = %q, should remain literal", got["B"])
	}
}

func TestExpandEnvRefsMissingKey(t *testing.T) {
	input := map[string]string{
		"URL":  "http://${HOST}:${PORT}",
		"HOST": "localhost",
	}
	got := ExpandEnvRefs(input)
	if got["URL"] != "http://localhost:${PORT}" {
		t.Fatalf("missing key URL = %q, want %q", got["URL"], "http://localhost:${PORT}")
	}
}

func TestExpandEnvRefsEmptyValue(t *testing.T) {
	input := map[string]string{
		"HOST": "",
		"URL":  "http://${HOST}:8080",
	}
	got := ExpandEnvRefs(input)
	if got["URL"] != "http://:8080" {
		t.Fatalf("empty value URL = %q, want %q", got["URL"], "http://:8080")
	}
}

func TestExpandEnvRefsNoRefs(t *testing.T) {
	input := map[string]string{
		"HOST": "localhost",
		"PORT": "5432",
		"URL":  "http://localhost:5432",
	}
	got := ExpandEnvRefs(input)
	if !maps.Equal(got, input) {
		t.Fatalf("no-refs: got %v, want unchanged", sortedKeys(got))
	}
}

func sortedKeys(m map[string]string) map[string]string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(m))
	for _, k := range keys {
		out[k] = m[k]
	}
	return out
}

func TestRenderGitHubStyleVariables(t *testing.T) {
	got := Render(
		"app:${{ github.ref_name }}-{short_sha}-${{ github.ref_type }}-${{ github.ref }}",
		Context{SourceBranch: "main", SourceCommit: "1234567890abcdef", SourceTag: ""},
	)
	want := "app:main-1234567890ab-branch-refs/heads/main"
	if got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestRenderPrefersTagRefName(t *testing.T) {
	got := Render(
		"app:${{ github.ref_name }}-${{ github.ref_type }}",
		Context{SourceBranch: "main", SourceCommit: "1234567890abcdef", SourceTag: "v1.0.0"},
	)
	want := "app:v1.0.0-tag"
	if got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestRenderDoesNotSupportDuplicateAliasVariables(t *testing.T) {
	got := Render(
		"app:{sha}-{commit}-{commit_short}-{branch}-{tag}-{ref_name}",
		Context{SourceBranch: "main", SourceCommit: "1234567890abcdef", SourceTag: "v1.0.0"},
	)
	want := "app:{sha}-{commit}-{commit_short}-{branch}-{tag}-{ref_name}"
	if got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}
