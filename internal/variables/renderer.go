package variables

import "strings"

type Context struct {
	SourceBranch string
	SourceCommit string
	SourceTag    string
}

func Render(value string, ctx Context) string {
	output := strings.TrimSpace(value)
	if output == "" {
		return ""
	}
	refName := fallback(strings.TrimSpace(ctx.SourceTag), strings.TrimSpace(ctx.SourceBranch))
	shortSHA := shortCommit(ctx.SourceCommit)
	replacements := map[string]string{
		"${{ github.sha }}":      strings.TrimSpace(ctx.SourceCommit),
		"${{ github.ref_name }}": refName,
		"${{ github.ref_type }}": refType(ctx),
		"${{ github.ref }}":      githubRef(ctx),
		"${{ github.head_ref }}": strings.TrimSpace(ctx.SourceBranch),
		"${{ github.base_ref }}": "",
		"{sha}":                  strings.TrimSpace(ctx.SourceCommit),
		"{commit}":               strings.TrimSpace(ctx.SourceCommit),
		"{short_sha}":            shortSHA,
		"{commit_short}":         shortSHA,
		"{branch}":               strings.TrimSpace(ctx.SourceBranch),
		"{tag}":                  strings.TrimSpace(ctx.SourceTag),
		"{ref_name}":             refName,
	}
	for key, replacement := range replacements {
		output = strings.ReplaceAll(output, key, replacement)
	}
	return output
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}

func refType(ctx Context) string {
	if strings.TrimSpace(ctx.SourceTag) != "" {
		return "tag"
	}
	return "branch"
}

func githubRef(ctx Context) string {
	if tag := strings.TrimSpace(ctx.SourceTag); tag != "" {
		return "refs/tags/" + tag
	}
	if branch := strings.TrimSpace(ctx.SourceBranch); branch != "" {
		return "refs/heads/" + branch
	}
	return ""
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
