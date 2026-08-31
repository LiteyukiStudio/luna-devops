package kubecatalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubevalidation "k8s.io/apimachinery/pkg/util/validation"
)

type BindingScope string

const (
	BindingScopeProject     BindingScope = "project"
	BindingScopeApplication BindingScope = "application"
	BindingScopeBoth        BindingScope = "both"
)

type CollectionPolicy string

const (
	CollectionPolicyOwnershipSelector CollectionPolicy = "ownership_selector"
	CollectionPolicyProjectNamespace  CollectionPolicy = "project_namespace"
	CollectionPolicyMetricsVerified   CollectionPolicy = "metrics_verified"
	CollectionPolicyLocalSynthetic    CollectionPolicy = "local_synthetic"
)

type Category string

const (
	CategoryWorkload    Category = "workload"
	CategoryNetwork     Category = "network"
	CategoryConfig      Category = "config"
	CategoryStorage     Category = "storage"
	CategoryGateway     Category = "gateway"
	CategoryObservation Category = "observation"
	CategoryNamespace   Category = "namespace"
	CategoryPlatform    Category = "platform_config"
	CategoryReview      Category = "review"
	CategoryExtra       Category = "extra"
)

type Permission struct {
	Actions []authz.Action
}

func (permission Permission) clone() Permission {
	permission.Actions = append([]authz.Action(nil), permission.Actions...)
	return permission
}

type ResourceRule struct {
	GVR              schema.GroupVersionResource
	Category         Category
	Namespaced       bool
	BindingScope     BindingScope
	CollectionPolicy CollectionPolicy
	Permissions      map[string]Permission
	Subresources     map[string]map[string]Permission
	Local            bool
}

func (rule ResourceRule) PermissionFor(subresource, verb string) (Permission, bool) {
	verb = strings.ToLower(strings.TrimSpace(verb))
	if subresource == "" {
		permission, ok := rule.Permissions[verb]
		return permission.clone(), ok
	}
	permissions, ok := rule.Subresources[strings.ToLower(strings.TrimSpace(subresource))]
	if !ok {
		return Permission{}, false
	}
	permission, ok := permissions[verb]
	return permission.clone(), ok
}

func (rule ResourceRule) clone() ResourceRule {
	clone := rule
	clone.Permissions = clonePermissions(rule.Permissions)
	clone.Subresources = make(map[string]map[string]Permission, len(rule.Subresources))
	for name, permissions := range rule.Subresources {
		clone.Subresources[name] = clonePermissions(permissions)
	}
	return clone
}

func clonePermissions(input map[string]Permission) map[string]Permission {
	output := make(map[string]Permission, len(input))
	for verb, permission := range input {
		output[verb] = permission.clone()
	}
	return output
}

type Catalog struct {
	rules map[schema.GroupVersionResource]ResourceRule
}

func New() *Catalog {
	catalog := &Catalog{rules: make(map[schema.GroupVersionResource]ResourceRule)}
	for _, rule := range builtinRules() {
		catalog.rules[rule.GVR] = rule
	}
	return catalog
}

func (catalog *Catalog) Lookup(gvr schema.GroupVersionResource) (ResourceRule, bool) {
	if catalog == nil {
		return ResourceRule{}, false
	}
	rule, ok := catalog.rules[gvr]
	return rule.clone(), ok
}

func (catalog *Catalog) Rules() []ResourceRule {
	if catalog == nil {
		return nil
	}
	rules := make([]ResourceRule, 0, len(catalog.rules))
	for _, rule := range catalog.rules {
		rules = append(rules, rule.clone())
	}
	sort.Slice(rules, func(i, j int) bool {
		left, right := rules[i].GVR, rules[j].GVR
		if left.Group != right.Group {
			return left.Group < right.Group
		}
		if left.Version != right.Version {
			return left.Version < right.Version
		}
		return left.Resource < right.Resource
	})
	return rules
}

type ExtraResourceRule struct {
	APIGroup     string
	APIVersion   string
	Resource     string
	Subresources []string
	Verbs        []string
	Action       authz.Action
}

type ScopeResolver interface {
	IsNamespaced(context.Context, schema.GroupVersionResource) (bool, error)
}

var (
	ErrExtraRuleInvalid       = errors.New("kubectl extra resource rule is invalid")
	ErrExtraRuleClusterScoped = errors.New("kubectl extra resource must be namespaced")
	ErrExtraRuleDenied        = errors.New("kubectl extra resource is fixed-deny")
)

func NewWithExtra(ctx context.Context, resolver ScopeResolver, extra []ExtraResourceRule) (*Catalog, error) {
	catalog := New()
	if err := catalog.MergeExtra(ctx, resolver, extra); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (catalog *Catalog) MergeExtra(ctx context.Context, resolver ScopeResolver, extra []ExtraResourceRule) error {
	if catalog == nil || resolver == nil {
		return fmt.Errorf("%w: missing catalog or scope resolver", ErrExtraRuleInvalid)
	}
	next := &Catalog{rules: make(map[schema.GroupVersionResource]ResourceRule, len(catalog.rules)+len(extra))}
	for gvr, rule := range catalog.rules {
		next.rules[gvr] = rule.clone()
	}
	for _, input := range extra {
		gvr := schema.GroupVersionResource{
			Group: strings.TrimSpace(input.APIGroup), Version: strings.TrimSpace(input.APIVersion), Resource: strings.ToLower(strings.TrimSpace(input.Resource)),
		}
		if !validGroup(gvr.Group) || !validSegment(gvr.Version) || len(kubevalidation.IsDNS1035Label(gvr.Resource)) > 0 {
			return fmt.Errorf("%w: malformed GVR", ErrExtraRuleInvalid)
		}
		if _, exists := next.rules[gvr]; exists {
			return fmt.Errorf("%w: duplicate or built-in GVR cannot be replaced: %s", ErrExtraRuleInvalid, gvr.String())
		}
		if IsFixedDenied(gvr, "") {
			return fmt.Errorf("%w: %s", ErrExtraRuleDenied, gvr.String())
		}
		if _, ok := authz.ProjectPolicyForAction(input.Action); !ok {
			return fmt.Errorf("%w: unknown action %q", ErrExtraRuleInvalid, input.Action)
		}
		namespaced, err := resolver.IsNamespaced(ctx, gvr)
		if err != nil {
			return fmt.Errorf("resolve %s scope: %w", gvr.String(), err)
		}
		if !namespaced {
			return fmt.Errorf("%w: %s", ErrExtraRuleClusterScoped, gvr.String())
		}
		permissions, err := extraPermissions(input.Verbs, input.Action)
		if err != nil {
			return err
		}
		subresources := make(map[string]map[string]Permission, len(input.Subresources))
		for _, raw := range input.Subresources {
			name := strings.ToLower(strings.TrimSpace(raw))
			if !validSegment(name) || IsFixedDenied(gvr, name) {
				return fmt.Errorf("%w: invalid subresource %q", ErrExtraRuleInvalid, raw)
			}
			subresources[name] = clonePermissions(permissions)
		}
		next.rules[gvr] = ResourceRule{
			GVR: gvr, Category: CategoryExtra, Namespaced: true, BindingScope: BindingScopeBoth,
			CollectionPolicy: CollectionPolicyOwnershipSelector, Permissions: permissions, Subresources: subresources,
		}
	}
	catalog.rules = next.rules
	return nil
}

func validGroup(value string) bool {
	return value != "" && len(kubevalidation.IsDNS1123Subdomain(value)) == 0
}

func extraPermissions(verbs []string, action authz.Action) (map[string]Permission, error) {
	permissions := make(map[string]Permission, len(verbs))
	for _, raw := range verbs {
		verb := strings.ToLower(strings.TrimSpace(raw))
		switch verb {
		case "get", "list", "watch", "create", "update", "patch", "delete", "deletecollection":
			permissions[verb] = Permission{Actions: []authz.Action{action}}
		default:
			return nil, fmt.Errorf("%w: unsupported verb %q", ErrExtraRuleInvalid, raw)
		}
	}
	if len(permissions) == 0 {
		return nil, fmt.Errorf("%w: verbs are required", ErrExtraRuleInvalid)
	}
	return permissions, nil
}

func validSegment(value string) bool {
	if value == "" || strings.ContainsAny(value, "*/\\\x00") || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func IsFixedDenied(gvr schema.GroupVersionResource, subresource string) bool {
	resource := strings.ToLower(gvr.Resource)
	subresource = strings.ToLower(strings.TrimSpace(subresource))
	if subresource == "proxy" || subresource == "finalize" || subresource == "binding" || subresource == "token" {
		return true
	}
	switch resource {
	case "nodes", "persistentvolumes", "roles", "rolebindings", "clusterroles", "clusterrolebindings",
		"customresourcedefinitions", "apiservices", "mutatingwebhookconfigurations", "validatingwebhookconfigurations",
		"certificatesigningrequests", "tokenreviews", "subjectaccessreviews", "localsubjectaccessreviews", "daemonsets",
		"referencegrants", "gateways":
		return true
	default:
		return false
	}
}

func builtinRules() []ResourceRule {
	read := func(action authz.Action) map[string]Permission {
		return permissions(action, "get", "list", "watch")
	}
	readNoWatch := func(action authz.Action) map[string]Permission {
		return permissions(action, "get", "list")
	}
	crud := func(readAction, writeAction, deleteAction authz.Action) map[string]Permission {
		result := read(readAction)
		addPermissions(result, writeAction, "create", "update", "patch")
		addPermissions(result, deleteAction, "delete", "deletecollection")
		return result
	}
	rule := func(group, version, resource string, category Category, scope BindingScope, collection CollectionPolicy, values map[string]Permission) ResourceRule {
		return ResourceRule{GVR: schema.GroupVersionResource{Group: group, Version: version, Resource: resource}, Category: category, Namespaced: true, BindingScope: scope, CollectionPolicy: collection, Permissions: values, Subresources: map[string]map[string]Permission{}}
	}

	pods := rule("", "v1", "pods", CategoryWorkload, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionDeploymentRead, authz.ActionDeploymentUpdate, authz.ActionDeploymentRestart))
	pods.Subresources["log"] = permissions(authz.ActionDeploymentRead, "get")
	pods.Subresources["exec"] = connectPermissions(authz.ActionDeploymentExec)
	pods.Subresources["attach"] = connectPermissions(authz.ActionDeploymentExec)
	pods.Subresources["portforward"] = connectPermissions(authz.ActionDeploymentExec)
	pods.Subresources["eviction"] = permissions(authz.ActionDeploymentRestart, "create")
	pods.Subresources["ephemeralcontainers"] = map[string]Permission{
		"update": {Actions: []authz.Action{authz.ActionDeploymentUpdate, authz.ActionDeploymentExec}},
		"patch":  {Actions: []authz.Action{authz.ActionDeploymentUpdate, authz.ActionDeploymentExec}},
	}

	deployments := rule("apps", "v1", "deployments", CategoryWorkload, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionDeploymentRead, authz.ActionDeploymentUpdate, authz.ActionDeploymentDelete))
	deployments.Subresources["scale"] = permissions(authz.ActionDeploymentUpdate, "get", "update", "patch")
	deployments.Subresources["status"] = permissions(authz.ActionDeploymentRead, "get")
	statefulsets := rule("apps", "v1", "statefulsets", CategoryWorkload, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionDeploymentRead, authz.ActionDeploymentUpdate, authz.ActionDeploymentDelete))
	statefulsets.Subresources["scale"] = permissions(authz.ActionDeploymentUpdate, "get", "update", "patch")
	statefulsets.Subresources["status"] = permissions(authz.ActionDeploymentRead, "get")
	replicasets := rule("apps", "v1", "replicasets", CategoryWorkload, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionDeploymentRead, authz.ActionDeploymentUpdate, authz.ActionDeploymentRestart))
	replicasets.Subresources["scale"] = permissions(authz.ActionDeploymentUpdate, "get", "update", "patch")
	replicasets.Subresources["status"] = permissions(authz.ActionDeploymentRead, "get")
	controllerRevisions := rule("apps", "v1", "controllerrevisions", CategoryWorkload, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		read(authz.ActionDeploymentRead))

	jobs := rule("batch", "v1", "jobs", CategoryWorkload, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionDeploymentRead, authz.ActionDeploymentUpdate, authz.ActionDeploymentDelete))
	jobs.Subresources["status"] = permissions(authz.ActionDeploymentRead, "get")
	cronjobs := rule("batch", "v1", "cronjobs", CategoryWorkload, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionDeploymentRead, authz.ActionDeploymentUpdate, authz.ActionDeploymentDelete))
	cronjobs.Subresources["status"] = permissions(authz.ActionDeploymentRead, "get")
	hpa := rule("autoscaling", "v2", "horizontalpodautoscalers", CategoryWorkload, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionDeploymentRead, authz.ActionDeploymentUpdate, authz.ActionDeploymentDelete))
	hpa.Subresources["status"] = permissions(authz.ActionDeploymentRead, "get")
	pdb := rule("policy", "v1", "poddisruptionbudgets", CategoryWorkload, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionDeploymentRead, authz.ActionDeploymentUpdate, authz.ActionDeploymentDelete))
	pdb.Subresources["status"] = permissions(authz.ActionDeploymentRead, "get")

	services := rule("", "v1", "services", CategoryNetwork, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionDeploymentRead, authz.ActionDeploymentUpdate, authz.ActionDeploymentDelete))
	endpoints := rule("", "v1", "endpoints", CategoryNetwork, BindingScopeBoth, CollectionPolicyOwnershipSelector, read(authz.ActionDeploymentRead))
	endpointSlices := rule("discovery.k8s.io", "v1", "endpointslices", CategoryNetwork, BindingScopeBoth, CollectionPolicyOwnershipSelector, read(authz.ActionDeploymentRead))
	networkPolicies := rule("networking.k8s.io", "v1", "networkpolicies", CategoryNetwork, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionClusterRead, authz.ActionClusterManage, authz.ActionClusterManage))
	ingresses := rule("networking.k8s.io", "v1", "ingresses", CategoryGateway, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionGatewayRead, authz.ActionGatewayManage, authz.ActionGatewayDelete))
	ingresses.Subresources["status"] = permissions(authz.ActionGatewayRead, "get")
	httpRoutes := rule("gateway.networking.k8s.io", "v1", "httproutes", CategoryGateway, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionGatewayRead, authz.ActionGatewayManage, authz.ActionGatewayDelete))
	httpRoutes.Subresources["status"] = permissions(authz.ActionGatewayRead, "get")
	grpcRoutes := rule("gateway.networking.k8s.io", "v1", "grpcroutes", CategoryGateway, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionGatewayRead, authz.ActionGatewayManage, authz.ActionGatewayDelete))
	grpcRoutes.Subresources["status"] = permissions(authz.ActionGatewayRead, "get")

	configMaps := rule("", "v1", "configmaps", CategoryConfig, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionSecretReadSummary, authz.ActionSecretUpdate, authz.ActionSecretUpdate))
	secrets := rule("", "v1", "secrets", CategoryConfig, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionSecretViewValue, authz.ActionSecretUpdate, authz.ActionSecretUpdate))
	pvcs := rule("", "v1", "persistentvolumeclaims", CategoryStorage, BindingScopeBoth, CollectionPolicyOwnershipSelector,
		crud(authz.ActionVolumeRead, authz.ActionVolumeWrite, authz.ActionVolumeDelete))
	pvcs.Subresources["status"] = permissions(authz.ActionVolumeRead, "get")

	events := rule("", "v1", "events", CategoryObservation, BindingScopeProject, CollectionPolicyProjectNamespace, read(authz.ActionClusterRead))
	eventsV1 := rule("events.k8s.io", "v1", "events", CategoryObservation, BindingScopeProject, CollectionPolicyProjectNamespace, read(authz.ActionClusterRead))
	podMetrics := rule("metrics.k8s.io", "v1beta1", "pods", CategoryObservation, BindingScopeBoth, CollectionPolicyMetricsVerified, readNoWatch(authz.ActionDeploymentRead))
	quotas := rule("", "v1", "resourcequotas", CategoryPlatform, BindingScopeProject, CollectionPolicyProjectNamespace, read(authz.ActionClusterRead))
	limits := rule("", "v1", "limitranges", CategoryPlatform, BindingScopeProject, CollectionPolicyProjectNamespace, read(authz.ActionClusterRead))
	serviceAccounts := rule("", "v1", "serviceaccounts", CategoryPlatform, BindingScopeProject, CollectionPolicyProjectNamespace, read(authz.ActionClusterRead))
	namespaces := rule("", "v1", "namespaces", CategoryNamespace, BindingScopeBoth, CollectionPolicyLocalSynthetic, readNoWatch(authz.ActionProjectRead))
	namespaces.Local = true
	namespaces.Namespaced = false
	storageClasses := rule("storage.k8s.io", "v1", "storageclasses", CategoryStorage, BindingScopeBoth, CollectionPolicyLocalSynthetic, readNoWatch(authz.ActionVolumeRead))
	storageClasses.Local = true
	storageClasses.Namespaced = false

	ssar := rule("authorization.k8s.io", "v1", "selfsubjectaccessreviews", CategoryReview, BindingScopeBoth, CollectionPolicyLocalSynthetic, permissions(authz.ActionProjectRead, "create"))
	ssar.Local = true
	ssar.Namespaced = false
	ssrr := rule("authorization.k8s.io", "v1", "selfsubjectrulesreviews", CategoryReview, BindingScopeBoth, CollectionPolicyLocalSynthetic, permissions(authz.ActionProjectRead, "create"))
	ssrr.Local = true
	ssrr.Namespaced = false
	selfReview := rule("authentication.k8s.io", "v1", "selfsubjectreviews", CategoryReview, BindingScopeBoth, CollectionPolicyLocalSynthetic, permissions(authz.ActionProjectRead, "create"))
	selfReview.Local = true
	selfReview.Namespaced = false

	return []ResourceRule{
		pods, deployments, statefulsets, replicasets, controllerRevisions, jobs, cronjobs, hpa, pdb,
		services, endpoints, endpointSlices, networkPolicies, ingresses, httpRoutes, grpcRoutes,
		configMaps, secrets, pvcs, events, eventsV1, podMetrics, quotas, limits, serviceAccounts,
		namespaces, storageClasses, ssar, ssrr, selfReview,
	}
}

func permissions(action authz.Action, verbs ...string) map[string]Permission {
	result := make(map[string]Permission, len(verbs))
	addPermissions(result, action, verbs...)
	return result
}

func connectPermissions(action authz.Action) map[string]Permission {
	return map[string]Permission{"connect": {Actions: []authz.Action{action}}}
}

func addPermissions(target map[string]Permission, action authz.Action, verbs ...string) {
	for _, verb := range verbs {
		target[verb] = Permission{Actions: []authz.Action{action}}
	}
}
