package api

import (
	"strings"

	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
)

type gitBranchFilterResult struct {
	items        []gitprovider.Branch
	matchedTotal int
}

func filterGitBranches(branches []gitprovider.Branch, search string, limit int) gitBranchFilterResult {
	search = strings.ToLower(strings.TrimSpace(search))
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	capacity := len(branches)
	if capacity > limit {
		capacity = limit
	}
	filtered := make([]gitprovider.Branch, 0, capacity)
	matchedTotal := 0
	for _, branch := range branches {
		if search != "" && !strings.Contains(strings.ToLower(branch.Name), search) {
			continue
		}
		matchedTotal++
		if len(filtered) < limit {
			filtered = append(filtered, branch)
		}
	}
	return gitBranchFilterResult{items: filtered, matchedTotal: matchedTotal}
}
