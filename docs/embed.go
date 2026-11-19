package docs

import (
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"unicode"
)

//go:embed docs/zh docs/en
var embeddedDocsFS embed.FS

const (
	maxDocReadRunes   = 8000
	maxSnippetRunes   = 320
	defaultSearchHits = 8
)

type DocSearchHit struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Heading string `json:"heading,omitempty"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Score   int    `json:"score"`
}

type DocTreeItem struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type DocReadResult struct {
	Path       string        `json:"path,omitempty"`
	Title      string        `json:"title,omitempty"`
	URL        string        `json:"url,omitempty"`
	Content    string        `json:"content,omitempty"`
	Offset     int           `json:"offset,omitempty"`
	NextOffset *int          `json:"nextOffset,omitempty"`
	TotalChars int           `json:"totalChars,omitempty"`
	Directory  []DocTreeItem `json:"directory,omitempty"`
}

type indexedDoc struct {
	RelPath  string // e.g. "use/mcp/index"
	Locale   string // "zh" | "en"
	Title    string
	Body     string
	Sections []indexedSection
}

type indexedSection struct {
	Heading string
	Content string
}

var (
	indexOnce sync.Once
	byLocale  map[string][]indexedDoc
	byKey     map[string]indexedDoc
)

func ensureDocsIndexed() {
	indexOnce.Do(func() {
		byLocale = map[string][]indexedDoc{"zh": {}, "en": {}}
		byKey = make(map[string]indexedDoc, 96)
		for _, locale := range []string{"zh", "en"} {
			root := "docs/" + locale
			_ = fs.WalkDir(embeddedDocsFS, root, func(filePath string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if !strings.HasSuffix(filePath, ".md") && !strings.HasSuffix(filePath, ".mdx") {
					return nil
				}
				raw, readErr := embeddedDocsFS.ReadFile(filePath)
				if readErr != nil {
					return nil
				}
				rel := strings.TrimPrefix(filePath, root+"/")
				rel = strings.TrimSuffix(strings.TrimSuffix(rel, ".mdx"), ".md")
				body := stripFrontmatter(strings.ReplaceAll(string(raw), "\r\n", "\n"))
				title := extractTitle(body, rel)
				sections := splitSections(body, title)
				doc := indexedDoc{
					RelPath:  rel,
					Locale:   locale,
					Title:    title,
					Body:     body,
					Sections: sections,
				}
				byLocale[locale] = append(byLocale[locale], doc)
				byKey[locale+"/"+rel] = doc
				if strings.HasSuffix(rel, "/index") {
					byKey[locale+"/"+strings.TrimSuffix(rel, "/index")] = doc
				} else if rel == "index" {
					byKey[locale+"/"] = doc
				}
				return nil
			})
			sort.Slice(byLocale[locale], func(i, j int) bool {
				return byLocale[locale][i].RelPath < byLocale[locale][j].RelPath
			})
		}
	})
}

func NormalizeLocale(locale string) string {
	lower := strings.ToLower(strings.TrimSpace(locale))
	if strings.HasPrefix(lower, "en") {
		return "en"
	}
	return "zh"
}

func publicDocURL(baseURL, locale, relPath string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://luna-devops.liteyuki.org"
	}
	clean := strings.TrimPrefix(relPath, "/")
	if clean == "index" {
		clean = ""
	} else {
		clean = strings.TrimSuffix(clean, "/index")
	}
	prefix := ""
	if locale == "en" {
		prefix = "/en"
	}
	if clean == "" {
		if prefix == "" {
			return base + "/"
		}
		return base + prefix
	}
	return base + prefix + "/" + clean
}

func Search(query, locale, baseURL string, limit int) []DocSearchHit {
	ensureDocsIndexed()
	loc := NormalizeLocale(locale)
	if limit <= 0 {
		limit = defaultSearchHits
	}
	if limit > 20 {
		limit = 20
	}
	tokens := tokenizeQuery(query)
	if len(tokens) == 0 {
		return []DocSearchHit{}
	}

	var hits []DocSearchHit
	for _, doc := range byLocale[loc] {
		titleLower := strings.ToLower(doc.Title)
		pathLower := strings.ToLower(doc.RelPath)
		for _, sec := range doc.Sections {
			headingLower := strings.ToLower(sec.Heading)
			bodyLower := strings.ToLower(sec.Content)
			score := 0
			for _, tok := range tokens {
				if strings.Contains(titleLower, tok) {
					score += 6
				}
				if strings.Contains(headingLower, tok) {
					score += 5
				}
				if strings.Contains(pathLower, tok) {
					score += 3
				}
				if cnt := strings.Count(bodyLower, tok); cnt > 0 {
					if cnt > 4 {
						cnt = 4
					}
					score += cnt * 2
				}
			}
			if score > 0 {
				hits = append(hits, DocSearchHit{
					Path:    loc + "/" + doc.RelPath,
					Title:   doc.Title,
					Heading: sec.Heading,
					URL:     publicDocURL(baseURL, loc, doc.RelPath),
					Snippet: truncateRunes(strings.TrimSpace(sec.Content), maxSnippetRunes),
					Score:   score,
				})
			}
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Path < hits[j].Path
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func Read(docPath, locale, baseURL string, offset int) (DocReadResult, bool) {
	ensureDocsIndexed()
	loc := NormalizeLocale(locale)
	cleaned := strings.Trim(path.Clean("/"+strings.TrimSpace(docPath)), "/")
	cleaned = strings.TrimSuffix(strings.TrimSuffix(cleaned, ".mdx"), ".md")

	if cleaned == "" || cleaned == "." {
		items := make([]DocTreeItem, 0, len(byLocale[loc]))
		for _, doc := range byLocale[loc] {
			items = append(items, DocTreeItem{
				Path:  loc + "/" + doc.RelPath,
				Title: doc.Title,
				URL:   publicDocURL(baseURL, loc, doc.RelPath),
			})
		}
		return DocReadResult{Directory: items}, true
	}

	lookup := cleaned
	if !strings.HasPrefix(lookup, "zh/") && !strings.HasPrefix(lookup, "en/") {
		lookup = loc + "/" + lookup
	} else {
		loc = lookup[:2]
	}

	doc, ok := byKey[lookup]
	if !ok {
		prefix := lookup + "/"
		var subItems []DocTreeItem
		for _, candidate := range byLocale[loc] {
			fullPath := loc + "/" + candidate.RelPath
			if strings.HasPrefix(fullPath, prefix) {
				subItems = append(subItems, DocTreeItem{
					Path:  fullPath,
					Title: candidate.Title,
					URL:   publicDocURL(baseURL, loc, candidate.RelPath),
				})
			}
		}
		if len(subItems) > 0 {
			return DocReadResult{Directory: subItems}, true
		}
		return DocReadResult{}, false
	}

	runes := []rune(doc.Body)
	if offset < 0 {
		offset = 0
	}
	if offset > len(runes) {
		offset = len(runes)
	}
	end := offset + maxDocReadRunes
	var nextOffset *int
	if end < len(runes) {
		next := end
		nextOffset = &next
	} else {
		end = len(runes)
	}

	return DocReadResult{
		Path:       doc.Locale + "/" + doc.RelPath,
		Title:      doc.Title,
		URL:        publicDocURL(baseURL, doc.Locale, doc.RelPath),
		Content:    string(runes[offset:end]),
		Offset:     offset,
		NextOffset: nextOffset,
		TotalChars: len(runes),
	}, true
}

func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---\n") {
		return strings.TrimSpace(s)
	}
	rest := s[4:]
	if idx := strings.Index(rest, "\n---\n"); idx >= 0 {
		return strings.TrimSpace(rest[idx+5:])
	}
	return strings.TrimSpace(s)
}

func extractTitle(body, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	// Check _meta.json fallback isn't needed if filename is clean
	_ = json.Valid(nil)
	return fallback
}

func splitSections(body, fallbackHeading string) []indexedSection {
	lines := strings.Split(body, "\n")
	var sections []indexedSection
	currentHeading := fallbackHeading
	var buf []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(buf, "\n"))
		if text != "" {
			sections = append(sections, indexedSection{Heading: currentHeading, Content: text})
		}
		buf = buf[:0]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			flush()
			currentHeading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}
		buf = append(buf, line)
	}
	flush()
	if len(sections) == 0 && strings.TrimSpace(body) != "" {
		sections = append(sections, indexedSection{Heading: fallbackHeading, Content: strings.TrimSpace(body)})
	}
	return sections
}

func tokenizeQuery(query string) []string {
	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(tok string) {
		tok = strings.TrimSpace(tok)
		if tok == "" || seen[tok] {
			return
		}
		seen[tok] = true
		out = append(out, tok)
	}

	var asciiBuf []rune
	var cjkRunes []rune
	flushASCII := func() {
		if len(asciiBuf) >= 2 {
			word := string(asciiBuf)
			add(word)
			switch word {
			case "k8s":
				add("kubernetes")
			case "pat":
				add("令牌")
				add("token")
			case "pvc":
				add("数据卷")
				add("volume")
			case "hpa":
				add("自动扩缩容")
				add("autoscaling")
			}
		}
		asciiBuf = asciiBuf[:0]
	}
	flushCJK := func() {
		if len(cjkRunes) == 1 {
			add(string(cjkRunes))
		} else if len(cjkRunes) >= 2 {
			for i := 0; i < len(cjkRunes)-1; i++ {
				add(string(cjkRunes[i : i+2]))
			}
		}
		cjkRunes = cjkRunes[:0]
	}

	for _, r := range lower {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_':
			flushCJK()
			asciiBuf = append(asciiBuf, r)
		case unicode.Is(unicode.Han, r):
			flushASCII()
			cjkRunes = append(cjkRunes, r)
		default:
			flushASCII()
			flushCJK()
		}
	}
	flushASCII()
	flushCJK()
	return out
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
