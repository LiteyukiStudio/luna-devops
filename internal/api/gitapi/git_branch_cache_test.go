package gitapi

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
)

func TestFilterGitBranchesCountsMatchesBeforeLimit(t *testing.T) {
	result := filterGitBranches([]gitprovider.Branch{
		{Name: "main"},
		{Name: "codex/complete-blog-builder"},
		{Name: "codex/music-player-visualizer"},
		{Name: "release/stable"},
	}, "codex", 1)

	if len(result.items) != 1 {
		t.Fatalf("expected one limited item, got %d", len(result.items))
	}
	if result.items[0].Name != "codex/complete-blog-builder" {
		t.Fatalf("expected first matching branch, got %q", result.items[0].Name)
	}
	if result.matchedTotal != 2 {
		t.Fatalf("expected matched total to count all matches, got %d", result.matchedTotal)
	}
}

func TestGitAccountNeedsRefresh(t *testing.T) {
	if gitAccountNeedsRefresh(model.GitAccount{}) {
		t.Fatal("account without expiry should not need refresh")
	}

	future := time.Now().Add(10 * time.Minute)
	if gitAccountNeedsRefresh(model.GitAccount{ExpiresAt: &future}) {
		t.Fatal("account expiring after refresh window should not need refresh")
	}

	soon := time.Now().Add(time.Minute)
	if !gitAccountNeedsRefresh(model.GitAccount{ExpiresAt: &soon}) {
		t.Fatal("account expiring inside refresh window should need refresh")
	}
}
