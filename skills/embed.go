package skills

import (
	"embed"
	"fmt"
	"strings"
	"sync"
)

//go:embed luna-devops/SKILL.md luna-devops/references/*.md
var embeddedFS embed.FS

// DomainSkillIDs defines the canonical order of progressive domain skills
// shared between internal luna-agent, the /api/v1/mcp server, and external Skill packages.
var DomainSkillIDs = []string{
	"workspace-iam",
	"source-registry-build",
	"application-deployment",
	"runtime-volumes",
	"gateway-topology",
	"diagnostics-governance",
	"platform-admin",
}

// SkillEntry represents a parsed SKILL.md document.
type SkillEntry struct {
	ID           string `json:"skillId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	RawMarkdown  string `json:"-"`
}

var (
	loadOnce     sync.Once
	loadErr      error
	entrySkill   SkillEntry
	domainByID   map[string]SkillEntry
	domainInList []SkillEntry
)

func ensureLoaded() error {
	loadOnce.Do(func() {
		rawEntry, err := embeddedFS.ReadFile("luna-devops/SKILL.md")
		if err != nil {
			loadErr = fmt.Errorf("read luna-devops entry skill: %w", err)
			return
		}
		parsedEntry, err := parseSkillMarkdown("luna-devops", string(rawEntry))
		if err != nil {
			loadErr = err
			return
		}
		entrySkill = parsedEntry

		domainByID = make(map[string]SkillEntry, len(DomainSkillIDs))
		domainInList = make([]SkillEntry, 0, len(DomainSkillIDs))
		for _, id := range DomainSkillIDs {
			raw, readErr := embeddedFS.ReadFile(fmt.Sprintf("luna-devops/references/%s.md", id))
			if readErr != nil {
				loadErr = fmt.Errorf("read domain skill %s: %w", id, readErr)
				return
			}
			parsed, parseErr := parseSkillMarkdown(id, string(raw))
			if parseErr != nil {
				loadErr = parseErr
				return
			}
			domainByID[id] = parsed
			domainInList = append(domainInList, parsed)
		}
	})
	return loadErr
}

// EntrySkill returns the root luna-devops SKILL.md entry.
func EntrySkill() (SkillEntry, error) {
	if err := ensureLoaded(); err != nil {
		return SkillEntry{}, err
	}
	return entrySkill, nil
}

// ListDomainSkills returns all domain skills in canonical order.
func ListDomainSkills() ([]SkillEntry, error) {
	if err := ensureLoaded(); err != nil {
		return nil, err
	}
	out := make([]SkillEntry, len(domainInList))
	copy(out, domainInList)
	return out, nil
}

// GetDomainSkill returns a single domain skill by ID.
func GetDomainSkill(skillID string) (SkillEntry, bool, error) {
	if err := ensureLoaded(); err != nil {
		return SkillEntry{}, false, err
	}
	entry, ok := domainByID[strings.TrimSpace(skillID)]
	return entry, ok, nil
}

func parseSkillMarkdown(expectedID, raw string) (SkillEntry, error) {
	trimmed := strings.TrimSpace(raw)
	normalized := strings.ReplaceAll(trimmed, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return SkillEntry{}, fmt.Errorf("missing YAML frontmatter in skill %s", expectedID)
	}
	rest := normalized[len("---\n"):]
	endIdx := strings.Index(rest, "\n---\n")
	if endIdx < 0 {
		return SkillEntry{}, fmt.Errorf("unterminated YAML frontmatter in skill %s", expectedID)
	}
	frontmatter := rest[:endIdx]
	body := strings.TrimSpace(rest[endIdx+len("\n---\n"):])

	var name, description string
	for _, line := range strings.Split(frontmatter, "\n") {
		sep := strings.IndexByte(line, ':')
		if sep <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:sep])
		val := strings.TrimSpace(line[sep+1:])
		switch key {
		case "name":
			name = val
		case "description":
			description = val
		}
	}
	if name != expectedID || description == "" || body == "" {
		return SkillEntry{}, fmt.Errorf("malformed metadata in skill %s (name=%q)", expectedID, name)
	}
	return SkillEntry{
		ID:           expectedID,
		Name:         name,
		Description:  description,
		Instructions: body,
		RawMarkdown:  trimmed,
	}, nil
}
