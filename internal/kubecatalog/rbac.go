package kubecatalog

import (
	"sort"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
)

var DiscoveryNonResourceURLs = []string{
	"/version",
	"/api", "/api/*",
	"/apis", "/apis/*",
	"/openapi/v2", "/openapi/v3", "/openapi/v3/*",
}

type rbacRuleKey struct {
	group    string
	resource string
}

func (catalog *Catalog) ProjectClusterRoleRules() []rbacv1.PolicyRule {
	if catalog == nil {
		return nil
	}
	verbsByResource := map[rbacRuleKey]map[string]struct{}{}
	for _, rule := range catalog.Rules() {
		if rule.Local {
			continue
		}
		addRBACVerbs(verbsByResource, rbacRuleKey{group: rule.GVR.Group, resource: rule.GVR.Resource}, rule.Permissions)
		for subresource, permissions := range rule.Subresources {
			addRBACVerbs(verbsByResource, rbacRuleKey{group: rule.GVR.Group, resource: rule.GVR.Resource + "/" + subresource}, permissions)
		}
	}
	keys := make([]rbacRuleKey, 0, len(verbsByResource))
	for value := range verbsByResource {
		keys = append(keys, value)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].group != keys[j].group {
			return keys[i].group < keys[j].group
		}
		return keys[i].resource < keys[j].resource
	})
	rules := make([]rbacv1.PolicyRule, 0, len(keys))
	for _, value := range keys {
		verbs := make([]string, 0, len(verbsByResource[value]))
		for verb := range verbsByResource[value] {
			verbs = append(verbs, verb)
		}
		sort.Strings(verbs)
		rules = append(rules, rbacv1.PolicyRule{APIGroups: []string{value.group}, Resources: []string{value.resource}, Verbs: verbs})
	}
	return rules
}

func DiscoveryClusterRoleRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{{NonResourceURLs: append([]string(nil), DiscoveryNonResourceURLs...), Verbs: []string{"get"}}}
}

func addRBACVerbs(target map[rbacRuleKey]map[string]struct{}, resource rbacRuleKey, permissions map[string]Permission) {
	verbs := target[resource]
	if verbs == nil {
		verbs = map[string]struct{}{}
		target[resource] = verbs
	}
	for kubeVerb := range permissions {
		for _, upstreamVerb := range upstreamRBACVerbs(kubeVerb) {
			if upstreamVerb != "" && upstreamVerb != "*" {
				verbs[upstreamVerb] = struct{}{}
			}
		}
	}
}

func upstreamRBACVerbs(verb string) []string {
	switch strings.ToLower(verb) {
	case "connect":
		return []string{"get", "create"}
	case "get", "list", "watch", "create", "update", "patch", "delete", "deletecollection":
		return []string{strings.ToLower(verb)}
	default:
		return nil
	}
}
