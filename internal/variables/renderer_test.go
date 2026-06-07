package variables

import "testing"

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
		"app:${{ github.ref_name }}-{tag}-${{ github.ref_type }}",
		Context{SourceBranch: "main", SourceCommit: "1234567890abcdef", SourceTag: "v1.0.0"},
	)
	want := "app:v1.0.0-v1.0.0-tag"
	if got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}
