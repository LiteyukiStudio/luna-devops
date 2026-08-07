package variables

import (
	"regexp"
	"strings"
)

type Context struct {
	SourceBranch string
	SourceCommit string
	SourceTag    string
}

var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ExpandEnvRefs 展开 values 中的 ${VAR_NAME} 引用，从 data map 内查找对应 key 的值。
// 不支持的引用和循环引用保留原样；缺失的 key 也保留原样。
func ExpandEnvRefs(data map[string]string) map[string]string {
	expanded := make(map[string]string, len(data))
	// 先拷贝所有 key
	for key, value := range data {
		expanded[key] = value
	}
	// 对每个 value 展开引用，最多展开 len(data) 轮以处理链式引用
	for range len(data) {
		changed := false
		for key, value := range expanded {
			next := envRefPattern.ReplaceAllStringFunc(value, func(match string) string {
				name := match[2 : len(match)-1]
				if resolved, ok := expanded[name]; ok && name != key {
					return resolved
				}
				return match // 缺失或自引用则保持原样
			})
			if next != value {
				expanded[key] = next
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	// 清理未解析的 ${...}（属于不存在的引用），用 os.Expand 展开为当前环境变量
	// 实际上我们只希望解析来自 data 的引用，所以不做 os.Expand。
	// 但是考虑到向后兼容和使用者预期，我们只对 data 内的引用做展开。
	return expanded
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
		"{short_sha}":            shortSHA,
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
