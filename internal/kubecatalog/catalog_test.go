package kubecatalog

import (
	"context"
	"errors"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type staticScopeResolver struct {
	namespaced bool
	err        error
}

func (resolver staticScopeResolver) IsNamespaced(context.Context, schema.GroupVersionResource) (bool, error) {
	return resolver.namespaced, resolver.err
}

func TestCatalogLocksSharedResourcesToProjectBindings(t *testing.T) {
	catalog := New()
	for _, gvr := range []schema.GroupVersionResource{
		{Version: "v1", Resource: "events"},
		{Version: "v1", Resource: "serviceaccounts"},
		{Version: "v1", Resource: "resourcequotas"},
		{Version: "v1", Resource: "limitranges"},
	} {
		rule, ok := catalog.Lookup(gvr)
		if !ok {
			t.Fatalf("missing %s", gvr.String())
		}
		if rule.BindingScope != BindingScopeProject || rule.CollectionPolicy != CollectionPolicyProjectNamespace {
			t.Fatalf("unexpected shared rule for %s: %#v", gvr.String(), rule)
		}
	}
}

func TestCatalogEphemeralContainersRequiresUpdateAndExec(t *testing.T) {
	rule, ok := New().Lookup(schema.GroupVersionResource{Version: "v1", Resource: "pods"})
	if !ok {
		t.Fatal("pod rule missing")
	}
	permission, ok := rule.PermissionFor("ephemeralcontainers", "patch")
	if !ok || len(permission.Actions) != 2 || permission.Actions[0] != authz.ActionDeploymentUpdate || permission.Actions[1] != authz.ActionDeploymentExec {
		t.Fatalf("unexpected permission: %#v", permission)
	}
}

func TestControllerRevisionsAreReadOnlyForStatefulSetHistory(t *testing.T) {
	rule, ok := New().Lookup(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "controllerrevisions"})
	if !ok {
		t.Fatal("ControllerRevision rule missing")
	}
	if rule.BindingScope != BindingScopeBoth || rule.CollectionPolicy != CollectionPolicyOwnershipSelector {
		t.Fatalf("unexpected ControllerRevision isolation: %#v", rule)
	}
	for _, verb := range []string{"get", "list", "watch"} {
		permission, allowed := rule.PermissionFor("", verb)
		if !allowed || len(permission.Actions) != 1 || permission.Actions[0] != authz.ActionDeploymentRead {
			t.Fatalf("ControllerRevision %s permission is invalid: %#v", verb, permission)
		}
	}
	for _, verb := range []string{"create", "update", "patch", "delete", "deletecollection"} {
		if _, allowed := rule.PermissionFor("", verb); allowed {
			t.Fatalf("ControllerRevision write verb %s must remain denied", verb)
		}
	}
}

func TestExtraRuleFailsClosed(t *testing.T) {
	rule := ExtraResourceRule{APIGroup: "example.io", APIVersion: "v1", Resource: "widgets", Verbs: []string{"get"}, Action: authz.ActionDeploymentRead}
	_, err := NewWithExtra(t.Context(), staticScopeResolver{namespaced: false}, []ExtraResourceRule{rule})
	if !errors.Is(err, ErrExtraRuleClusterScoped) {
		t.Fatalf("expected cluster-scoped denial, got %v", err)
	}
	rule.Resource = "nodes"
	_, err = NewWithExtra(t.Context(), staticScopeResolver{namespaced: true}, []ExtraResourceRule{rule})
	if !errors.Is(err, ErrExtraRuleDenied) {
		t.Fatalf("expected fixed denial, got %v", err)
	}
}

func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	catalog := New()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	rule, _ := catalog.Lookup(gvr)
	delete(rule.Permissions, "get")
	rule.Subresources["exec"]["connect"] = Permission{}
	again, _ := catalog.Lookup(gvr)
	if _, ok := again.Permissions["get"]; !ok {
		t.Fatal("caller mutated catalog permissions")
	}
	permission, _ := again.PermissionFor("exec", "connect")
	if len(permission.Actions) != 1 {
		t.Fatal("caller mutated catalog subresource permissions")
	}
}

func TestExtraRuleMergeIsTransactionalAndValidatesGroup(t *testing.T) {
	catalog := New()
	validGVR := schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "widgets"}
	err := catalog.MergeExtra(t.Context(), staticScopeResolver{namespaced: true}, []ExtraResourceRule{
		{APIGroup: validGVR.Group, APIVersion: validGVR.Version, Resource: validGVR.Resource, Verbs: []string{"get"}, Action: authz.ActionDeploymentRead},
		{APIGroup: "invalid/group", APIVersion: "v1", Resource: "gadgets", Verbs: []string{"get"}, Action: authz.ActionDeploymentRead},
	})
	if !errors.Is(err, ErrExtraRuleInvalid) {
		t.Fatalf("expected malformed group rejection, got %v", err)
	}
	if _, exists := catalog.Lookup(validGVR); exists {
		t.Fatal("failed merge partially modified the catalog")
	}
}
