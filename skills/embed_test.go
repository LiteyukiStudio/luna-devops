package skills

import (
	"strings"
	"testing"
)

func TestEmbeddedSkillsLoadAndStayUniversal(t *testing.T) {
	entry, err := EntrySkill()
	if err != nil {
		t.Fatalf("EntrySkill() error = %v", err)
	}
	if entry.ID != "luna-devops" || !strings.Contains(entry.Instructions, "/api/v1/mcp") {
		t.Fatalf("unexpected entry skill: %+v", entry)
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
		"luna deployment exec",
		"luna-cli",
	}
	for _, skill := range domains {
		got, ok, err := GetDomainSkill(skill.ID)
		if err != nil || !ok || got.Name != skill.ID {
			t.Fatalf("GetDomainSkill(%q) = (%+v, %v, %v)", skill.ID, got, ok, err)
		}
		for _, forbidden := range webOnlyTokens {
			if strings.Contains(skill.Instructions, forbidden) {
				t.Errorf("domain skill %q contains host-coupled or deprecated token %q", skill.ID, forbidden)
			}
		}
	}
}
