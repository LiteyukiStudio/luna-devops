package platformapi

import (
	"slices"
	"testing"
)

func TestParseSearchQueryAndCandidateRouting(t *testing.T) {
	t.Run("inline prefix narrows candidate types and preserves image colon", func(t *testing.T) {
		parsed := parseSearchQuery("deploy: ghcr.io/liteyuki/probe:1.2.0")
		if parsed.emptyTerm {
			t.Fatal("expected non-empty term")
		}
		if parsed.term != "ghcr.io/liteyuki/probe:1.2.0" {
			t.Fatalf("term = %q", parsed.term)
		}
		candidates := resolveCandidateSearchTypes(parsed, nil)
		if !slices.Equal(candidates, []string{SearchTypeDeploymentTarget}) {
			t.Fatalf("candidates = %v", candidates)
		}
	})

	t.Run("prefix without remainder marks emptyTerm", func(t *testing.T) {
		parsed := parseSearchQuery("app:   ")
		if !parsed.emptyTerm {
			t.Fatal("expected emptyTerm = true")
		}
	})

	t.Run("https URL is not misparsed as inline prefix and extracts hostname", func(t *testing.T) {
		parsed := parseSearchQuery("https://Blog.Liteyuki.org:8443/posts/1?draft=1")
		if len(parsed.inlineTypes) != 0 {
			t.Fatalf("unexpected inlineTypes = %v", parsed.inlineTypes)
		}
		if parsed.urlHostLower != "blog.liteyuki.org" {
			t.Fatalf("urlHostLower = %q, want blog.liteyuki.org", parsed.urlHostLower)
		}
	})

	t.Run("registry host with port is not misparsed as inline prefix", func(t *testing.T) {
		parsed := parseSearchQuery("harbor.internal:5000/devops/api:v2")
		if len(parsed.inlineTypes) != 0 {
			t.Fatalf("unexpected inlineTypes = %v", parsed.inlineTypes)
		}
	})

	t.Run("resource ID prefixes route to single table fast path", func(t *testing.T) {
		cases := map[string]string{
			"prj_abc123":  SearchTypeProject,
			"app_abc123":  SearchTypeApplication,
			"dplt_abc123": SearchTypeDeploymentTarget,
			"gwr_abc123":  SearchTypeGatewayRoute,
			"rpb_abc123":  SearchTypeRepositoryBinding,
			"pvol_abc123": SearchTypeProjectVolume,
			"prcs_abc123": SearchTypeRuntimeConfigSet,
			"bvs_abc123":  SearchTypeBuildVariableSet,
			"nch_abc123":  SearchTypeNotificationChannel,
			"clu_abc123":  SearchTypeRuntimeCluster,
			"reg_abc123":  SearchTypeArtifactRegistry,
			"bldr_abc123": SearchTypeBuildRun,
			"rel_abc123":  SearchTypeRelease,
			"usr_abc123":  SearchTypeUser,
		}
		for query, wantType := range cases {
			parsed := parseSearchQuery(query)
			candidates := resolveCandidateSearchTypes(parsed, nil)
			if !slices.Equal(candidates, []string{wantType}) {
				t.Fatalf("query %q candidates = %v, want [%s]", query, candidates, wantType)
			}
		}
	})

	t.Run("commit SHA candidate adds build_run while ordinary keywords exclude build_run and release", func(t *testing.T) {
		ordinary := parseSearchQuery("blog-service")
		ordinaryCandidates := resolveCandidateSearchTypes(ordinary, nil)
		if slices.Contains(ordinaryCandidates, SearchTypeBuildRun) || slices.Contains(ordinaryCandidates, SearchTypeRelease) {
			t.Fatalf("ordinary keyword unexpectedly included history tables: %v", ordinaryCandidates)
		}

		sha := parseSearchQuery("0d7b808e")
		if !sha.commitSHA {
			t.Fatal("expected 0d7b808e to be detected as commit SHA")
		}
		shaCandidates := resolveCandidateSearchTypes(sha, nil)
		if !slices.Contains(shaCandidates, SearchTypeBuildRun) {
			t.Fatalf("commit SHA candidates missing build_run: %v", shaCandidates)
		}
		if slices.Contains(shaCandidates, SearchTypeRelease) {
			t.Fatalf("commit SHA candidates should not scan release: %v", shaCandidates)
		}
	})

	t.Run("escapes SQL LIKE wildcards", func(t *testing.T) {
		parsed := parseSearchQuery(`100%_literal\check`)
		want := `100\%\_literal\\check`
		if parsed.escapedLower != want {
			t.Fatalf("escapedLower = %q, want %q", parsed.escapedLower, want)
		}
	})
}
