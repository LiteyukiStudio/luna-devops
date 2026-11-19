package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
)

func TestDefaultAIToolCatalogCompletenessAndSecurityInvariants(t *testing.T) {
	reg := aitool.NewRegistry()
	h := &Handlers{aiToolRegistry: reg}
	_ = newDomainHandlers(h)

	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}

	catalog := reg.Catalog()
	if len(catalog) != 166 {
		t.Fatalf("DefaultAIToolCatalog count = %d, want exact 166", len(catalog))
	}

	var (
		tenantCount, adminCount           int
		tenantL0, tenantL1, tenantL2      int
		adminOnlyL0, adminOnlyL1, adminL2 int
	)
	domainCounts := map[string]int{}

	for _, op := range catalog {
		domainCounts[op.Domain]++
		if op.AdminOnly {
			adminCount++
			switch op.ApprovalTier {
			case aitool.ApprovalTierL0:
				adminOnlyL0++
			case aitool.ApprovalTierL1:
				adminOnlyL1++
			case aitool.ApprovalTierL2:
				adminL2++
			default:
				t.Fatalf("tool %s has unexpected approvalTier %q", op.OperationID, op.ApprovalTier)
			}
			if !strings.HasPrefix(op.OperationID, "admin_") {
				t.Fatalf("adminOnly tool %s must use admin_ prefix", op.OperationID)
			}
		} else {
			tenantCount++
			switch op.ApprovalTier {
			case aitool.ApprovalTierL0:
				tenantL0++
			case aitool.ApprovalTierL1:
				tenantL1++
			case aitool.ApprovalTierL2:
				tenantL2++
			default:
				t.Fatalf("tool %s has unexpected approvalTier %q", op.OperationID, op.ApprovalTier)
			}
			if strings.HasPrefix(op.OperationID, "admin_") {
				t.Fatalf("non-adminOnly tool %s must not use admin_ prefix", op.OperationID)
			}
		}

		// Every tool must declare valid OAuth scopes (or web:read).
		if len(op.RequiredScopes) == 0 {
			t.Fatalf("tool %s has empty requiredScopes", op.OperationID)
		}
		for _, scope := range op.RequiredScopes {
			if scope == "web:read" {
				continue
			}
			normalized := authz.NormalizeOAuthScope(scope)
			if normalized == "" || normalized != scope || !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleAdmin, normalized) {
				t.Fatalf("tool %s declares invalid scope %q", op.OperationID, scope)
			}
			if !op.AdminOnly && !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleUser, normalized) {
				t.Fatalf("tenant-visible tool %s declares admin-only scope %q without AdminOnly=true", op.OperationID, scope)
			}
		}

		// Every L2 tool must have a server-side Preview handler.
		registered, ok := reg.Get(op.OperationID)
		if !ok {
			t.Fatalf("tool %s missing from registry.Get", op.OperationID)
		}
		if op.ApprovalTier == aitool.ApprovalTierL2 && !registered.HasPreview {
			t.Fatalf("L2 tool %s is missing mandatory Preview handler", op.OperationID)
		}

		// Every write or async mutation must declare post-mutation authoritative L0 verification.
		if (op.Effect == aitool.EffectWrite || op.Effect == aitool.EffectAsync) && len(op.Verification) == 0 {
			t.Fatalf("mutating tool %s (effect=%s) is missing verification rules", op.OperationID, op.Effect)
		}

		// InputSchema must be a self-contained strict JSON Schema object (additionalProperties: false, no $ref).
		if op.InputSchema["type"] != "object" || op.InputSchema["additionalProperties"] != false {
			t.Fatalf("tool %s InputSchema must be strict object with additionalProperties=false: %#v", op.OperationID, op.InputSchema)
		}
		encoded, err := json.Marshal(op)
		if err != nil {
			t.Fatalf("marshal %s: %v", op.OperationID, err)
		}
		if strings.Contains(string(encoded), `"$ref"`) {
			t.Fatalf("tool %s contains unresolved $ref in catalog JSON", op.OperationID)
		}
	}

	if tenantCount != 133 || adminCount != 33 {
		t.Fatalf("visibility split = (%d tenant, %d adminOnly), want (133, 33)", tenantCount, adminCount)
	}
	if tenantL0 != 48 || tenantL1 != 10 || tenantL2 != 75 {
		t.Fatalf("tenant tier split = (L0=%d, L1=%d, L2=%d), want (48, 10, 75)", tenantL0, tenantL1, tenantL2)
	}
	if adminOnlyL0 != 11 || adminOnlyL1 != 0 || adminL2 != 22 {
		t.Fatalf("adminOnly tier split = (L0=%d, L1=%d, L2=%d), want (11, 0, 22)", adminOnlyL0, adminOnlyL1, adminL2)
	}

	wantDomains := map[string]int{
		aitool.DomainDelivery:  26,
		aitool.DomainBuild:     29,
		aitool.DomainRuntime:   25,
		aitool.DomainNetwork:   11,
		aitool.DomainWorkspace: 17,
		aitool.DomainOps:       25,
		aitool.DomainAdmin:     33,
	}
	for domain, want := range wantDomains {
		if got := domainCounts[domain]; got != want {
			t.Fatalf("domain %q tool count = %d, want %d", domain, got, want)
		}
	}
}
