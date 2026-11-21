package skills

import (
	"fmt"
	"strings"
	"testing"
)

func TestEmbeddedSkillsLoadAndStayUniversal(t *testing.T) {
	if _, err := embeddedFS.ReadFile("README.md"); err == nil {
		t.Fatal("skills/README.md maintainer guide must not be embedded into production binaries")
	}

	entry, err := EntrySkill()
	if err != nil {
		t.Fatalf("EntrySkill() error = %v", err)
	}
	if entry.ID != "luna-devops" || !strings.Contains(entry.Instructions, "/api/v1/mcp") {
		t.Fatalf("unexpected entry skill: %+v", entry)
	}
	if lineCount := len(strings.Split(entry.RawMarkdown, "\n")); lineCount >= 500 {
		t.Fatalf("entry SKILL.md line count = %d, want < 500", lineCount)
	}
	if strings.Contains(entry.Instructions, "../domains/") {
		t.Fatal("entry SKILL.md must use self-contained 1-level-deep references/<skillId>.md links")
	}

	domains, err := ListDomainSkills()
	if err != nil {
		t.Fatalf("ListDomainSkills() error = %v", err)
	}
	if len(domains) != 7 {
		t.Fatalf("len(domains) = %d, want 7", len(domains))
	}

	webOnlyTokens := []string{
		"present_card",
		"request_input",
		"request_choice",
		"navigate_to_route",
		"wait_for_task",
		"luna deployment exec",
		"luna-cli",
	}
	for _, forbidden := range webOnlyTokens {
		if strings.Contains(entry.Instructions, forbidden) {
			t.Errorf("entry SKILL.md contains host-coupled or deprecated token %q", forbidden)
		}
	}
	for _, skill := range domains {
		got, ok, err := GetDomainSkill(skill.ID)
		if err != nil || !ok || got.Name != skill.ID {
			t.Fatalf("GetDomainSkill(%q) = (%+v, %v, %v)", skill.ID, got, ok, err)
		}
		refLink := fmt.Sprintf("references/%s.md", skill.ID)
		if !strings.Contains(entry.Instructions, refLink) {
			t.Errorf("entry SKILL.md missing direct 1-level link %q", refLink)
		}
		if lines := len(strings.Split(skill.RawMarkdown, "\n")); lines > 100 && !strings.Contains(skill.Instructions, "## 目录") {
			t.Errorf("reference %q has %d lines (> 100) without a top-level '## 目录' TOC", skill.ID, lines)
		}
		for _, forbidden := range webOnlyTokens {
			if strings.Contains(skill.Instructions, forbidden) {
				t.Errorf("domain skill %q contains host-coupled or deprecated token %q", skill.ID, forbidden)
			}
		}
	}
}
