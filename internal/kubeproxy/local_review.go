package kubeproxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const maxLocalReviewBodyBytes int64 = 1 << 20

type LocalReviewHandler struct {
	Authorizer Authorizer
}

func (handler LocalReviewHandler) Handles(info RequestInfo) bool {
	if !info.IsResourceRequest || info.Verb != "create" || info.Name != "" || info.Subresource != "" {
		return false
	}
	return info.APIGroup == "authorization.k8s.io" && info.APIVersion == "v1" && (info.Resource == "selfsubjectaccessreviews" || info.Resource == "selfsubjectrulesreviews") ||
		info.APIGroup == "authentication.k8s.io" && info.APIVersion == "v1" && info.Resource == "selfsubjectreviews"
}

func (handler LocalReviewHandler) Serve(writer http.ResponseWriter, request *http.Request, access AccessContext, info RequestInfo) error {
	if handler.Authorizer == nil {
		return Unavailable(CodeUnavailable, fmt.Errorf("local review authorizer is unavailable"))
	}
	switch info.Resource {
	case "selfsubjectaccessreviews":
		return handler.serveAccessReview(writer, request, access)
	case "selfsubjectrulesreviews":
		return handler.serveRulesReview(writer, request, access)
	case "selfsubjectreviews":
		return handler.serveIdentityReview(writer, request, access)
	default:
		return NotFound(info.GVR(), info.Name)
	}
}

func (handler LocalReviewHandler) serveAccessReview(writer http.ResponseWriter, request *http.Request, access AccessContext) error {
	var review authorizationv1.SelfSubjectAccessReview
	if err := decodeLocalReview(request, &review); err != nil {
		return err
	}
	attributes := review.Spec.ResourceAttributes
	if attributes == nil || review.Spec.NonResourceAttributes != nil {
		return BadRequest(CodeBadRequest, fmt.Errorf("only resource self-access reviews are supported"))
	}
	catalog, ok := catalogFromAuthorizer(handler.Authorizer)
	if !ok {
		return Unavailable(CodeUnavailable, fmt.Errorf("access review requires the central catalog authorizer"))
	}
	matched := false
	for _, rule := range catalog.Rules() {
		if rule.GVR.Group != strings.TrimSpace(attributes.Group) || rule.GVR.Resource != strings.ToLower(strings.TrimSpace(attributes.Resource)) ||
			(strings.TrimSpace(attributes.Version) != "" && rule.GVR.Version != strings.TrimSpace(attributes.Version)) {
			continue
		}
		matched = true
		namespace := ""
		if rule.Namespaced {
			if attributes.Namespace != "" && attributes.Namespace != access.Namespace {
				review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: false, Denied: true, Reason: "outside the binding namespace"}
				return WriteNegotiatedObject(writer, request, &review)
			}
			namespace = access.Namespace
		}
		requestInfo := RequestInfo{
			Verb:     reviewAuthorizationVerb(rule, attributes.Subresource, attributes.Verb),
			APIGroup: rule.GVR.Group, APIVersion: rule.GVR.Version, Resource: rule.GVR.Resource,
			Subresource: strings.ToLower(strings.TrimSpace(attributes.Subresource)), Namespace: namespace, Name: attributes.Name,
			IsResourceRequest: true, IsCollection: attributes.Name == "",
		}
		decision, err := handler.Authorizer.Authorize(request.Context(), access, requestInfo)
		if err == nil && decision.Allowed {
			review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true, Reason: "allowed by Luna project policy"}
			review.TypeMeta = metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectAccessReview"}
			return WriteNegotiatedObject(writer, request, &review)
		}
	}
	if !matched {
		review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: false, Denied: true, Reason: "resource is outside the Luna kubectl catalog"}
	} else {
		review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: false, Denied: true, Reason: "denied by Luna project policy"}
	}
	review.TypeMeta = metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectAccessReview"}
	return WriteNegotiatedObject(writer, request, &review)
}

func (handler LocalReviewHandler) serveRulesReview(writer http.ResponseWriter, request *http.Request, access AccessContext) error {
	var review authorizationv1.SelfSubjectRulesReview
	if err := decodeLocalReview(request, &review); err != nil {
		return err
	}
	if review.Spec.Namespace != "" && review.Spec.Namespace != access.Namespace {
		return Forbidden(CodeForbidden, fmt.Errorf("rules review namespace is outside the binding"))
	}
	catalog, ok := catalogFromAuthorizer(handler.Authorizer)
	if !ok {
		return Unavailable(CodeUnavailable, fmt.Errorf("rules review requires the central catalog authorizer"))
	}
	for _, rule := range catalog.Rules() {
		namespace := ""
		if rule.Namespaced {
			namespace = access.Namespace
		}
		verbSet := map[string]struct{}{}
		for verb := range rule.Permissions {
			info := RequestInfo{Verb: verb, APIGroup: rule.GVR.Group, APIVersion: rule.GVR.Version, Resource: rule.GVR.Resource, Namespace: namespace, IsResourceRequest: true, IsCollection: true}
			decision, err := handler.Authorizer.Authorize(request.Context(), access, info)
			if err == nil && decision.Allowed {
				for _, reported := range reportedReviewVerbs(verb) {
					verbSet[reported] = struct{}{}
				}
			}
		}
		for subresource, permissions := range rule.Subresources {
			subresourceVerbs := map[string]struct{}{}
			for verb := range permissions {
				info := RequestInfo{Verb: verb, APIGroup: rule.GVR.Group, APIVersion: rule.GVR.Version, Resource: rule.GVR.Resource, Subresource: subresource, Namespace: namespace, IsResourceRequest: true, IsCollection: false, Name: "*"}
				decision, err := handler.Authorizer.Authorize(request.Context(), access, info)
				if err == nil && decision.Allowed {
					for _, reported := range reportedReviewVerbs(verb) {
						subresourceVerbs[reported] = struct{}{}
					}
				}
			}
			verbs := sortedReviewVerbs(subresourceVerbs)
			if len(verbs) > 0 {
				review.Status.ResourceRules = append(review.Status.ResourceRules, authorizationv1.ResourceRule{Verbs: verbs, APIGroups: []string{rule.GVR.Group}, Resources: []string{rule.GVR.Resource + "/" + subresource}})
			}
		}
		verbs := sortedReviewVerbs(verbSet)
		if len(verbs) > 0 {
			review.Status.ResourceRules = append(review.Status.ResourceRules, authorizationv1.ResourceRule{Verbs: verbs, APIGroups: []string{rule.GVR.Group}, Resources: []string{rule.GVR.Resource}})
		}
	}
	review.TypeMeta = metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectRulesReview"}
	return WriteNegotiatedObject(writer, request, &review)
}

func reviewAuthorizationVerb(rule kubecatalog.ResourceRule, subresource, verb string) string {
	verb = strings.ToLower(strings.TrimSpace(verb))
	subresource = strings.ToLower(strings.TrimSpace(subresource))
	if _, connect := rule.PermissionFor(subresource, "connect"); connect && (verb == "get" || verb == "create" || verb == "connect") {
		return "connect"
	}
	return verb
}

func reportedReviewVerbs(verb string) []string {
	if verb == "connect" {
		return []string{"get", "create"}
	}
	return []string{verb}
}

func sortedReviewVerbs(values map[string]struct{}) []string {
	verbs := make([]string, 0, len(values))
	for verb := range values {
		verbs = append(verbs, verb)
	}
	sort.Strings(verbs)
	return verbs
}

func (handler LocalReviewHandler) serveIdentityReview(writer http.ResponseWriter, request *http.Request, access AccessContext) error {
	var review authenticationv1.SelfSubjectReview
	if err := decodeLocalReview(request, &review); err != nil {
		return err
	}
	groups := []string{"luna:project:" + access.ProjectID, "luna:project-role:" + access.ProjectRole}
	if access.PlatformRole != "" {
		groups = append(groups, "luna:platform-role:"+access.PlatformRole)
	}
	review.TypeMeta = metav1.TypeMeta{APIVersion: "authentication.k8s.io/v1", Kind: "SelfSubjectReview"}
	review.Status.UserInfo = authenticationv1.UserInfo{Username: "luna:" + access.UserID, UID: access.UserID, Groups: groups}
	return WriteNegotiatedObject(writer, request, &review)
}

func decodeLocalReview(request *http.Request, output any) error {
	if request == nil || request.Body == nil {
		return BadRequest(CodeBadRequest, fmt.Errorf("review body is required"))
	}
	data, err := readLimited(request.Body, maxLocalReviewBodyBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, output); err != nil {
		return BadRequest(CodeBadRequest, fmt.Errorf("invalid review object"))
	}
	return nil
}
