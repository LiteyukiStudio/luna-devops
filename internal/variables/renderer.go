package variables

import (
	"regexp"
	"sort"
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

// BuildTemplateVariables 返回构建时可在镜像 tag 与 Build Args 值中使用的全部变量写法，按字典序排列。
// 它与 Render 共用 buildTemplateValues，是这组变量的唯一事实源；公开文档一致性测试也读取它。
func BuildTemplateVariables() []string {
	values := buildTemplateValues(Context{})
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func Render(value string, ctx Context) string {
	output := strings.TrimSpace(value)
	if output == "" {
		return ""
	}
	for key, replacement := range buildTemplateValues(ctx) {
		output = strings.ReplaceAll(output, key, replacement)
	}
	return output
}

// buildTemplateValues 是构建时变量的唯一定义；构建执行器 run.sh 的 render_template_value 必须逐一支持，
// 由 executor 一致性测试实际执行脚本比对两侧渲染结果。
func buildTemplateValues(ctx Context) map[string]string {
	commit := strings.TrimSpace(ctx.SourceCommit)
	branch := strings.TrimSpace(ctx.SourceBranch)
	tag := strings.TrimSpace(ctx.SourceTag)
	return map[string]string{
		"${{ github.sha }}":      commit,
		"${{ github.ref_name }}": fallback(tag, branch),
		"${{ github.ref_type }}": refType(ctx),
		"${{ github.ref }}":      githubRef(ctx),
		"{short_sha}":            shortCommit(commit),
		"{commit}":               commit,
		"{shortSha}":             shortCommit(commit),
		"{branch}":               branch,
		"{branchSlug}":           Slug(branch),
		"{tag}":                  tag,
		"{tagSlug}":              Slug(tag),
	}
}

// Slug 把分支或标签名转成小写字母、数字和连字符；空值保持为空，交由镜像 tag 规范化处理。
func Slug(value string) string {
	var builder strings.Builder
	previousDash := false
	for _, char := range strings.ToLower(strings.TrimSpace(value)) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			builder.WriteRune(char)
			previousDash = false
			continue
		}
		if !previousDash {
			builder.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
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
