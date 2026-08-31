package kubeproxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSelfSubjectAccessReviewUsesCatalogResourceScope(t *testing.T) {
	handler := LocalReviewHandler{Authorizer: CatalogAuthorizer{Catalog: kubecatalog.New()}}
	access := baseAccess()
	for _, attributes := range []authorizationv1.ResourceAttributes{
		{Namespace: access.Namespace, Verb: "get", Resource: "namespaces", Name: access.Namespace},
		{Namespace: access.Namespace, Verb: "get", Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses", Name: "fast"},
	} {
		t.Run(attributes.Resource, func(t *testing.T) {
			review := authorizationv1.SelfSubjectAccessReview{
				TypeMeta: metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectAccessReview"},
				Spec:     authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &attributes},
			}
			body, err := json.Marshal(review)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "https://gateway.example/apis/authorization.k8s.io/v1/selfsubjectaccessreviews", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			writer := httptest.NewRecorder()
			info := RequestInfo{Verb: "create", APIGroup: "authorization.k8s.io", APIVersion: "v1", Resource: "selfsubjectaccessreviews", IsResourceRequest: true, IsCollection: true}
			if err := handler.Serve(writer, request, access, info); err != nil {
				t.Fatal(err)
			}
			var result authorizationv1.SelfSubjectAccessReview
			if err := json.Unmarshal(writer.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if !result.Status.Allowed || result.Status.Denied {
				t.Fatalf("cluster-scoped local resource was not reflected as allowed: %#v", result.Status)
			}
		})
	}
}

func TestSelfSubjectRulesReviewIncludesClusterScopedLocalResources(t *testing.T) {
	handler := LocalReviewHandler{Authorizer: CatalogAuthorizer{Catalog: kubecatalog.New()}}
	access := baseAccess()
	review := authorizationv1.SelfSubjectRulesReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectRulesReview"},
		Spec:     authorizationv1.SelfSubjectRulesReviewSpec{Namespace: access.Namespace},
	}
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://gateway.example/apis/authorization.k8s.io/v1/selfsubjectrulesreviews", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	info := RequestInfo{Verb: "create", APIGroup: "authorization.k8s.io", APIVersion: "v1", Resource: "selfsubjectrulesreviews", IsResourceRequest: true, IsCollection: true}
	if err := handler.Serve(writer, request, access, info); err != nil {
		t.Fatal(err)
	}
	var result authorizationv1.SelfSubjectRulesReview
	if err := json.Unmarshal(writer.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, rule := range result.Status.ResourceRules {
		for _, resource := range rule.Resources {
			if resource == "namespaces" || resource == "storageclasses" {
				for _, verb := range rule.Verbs {
					if verb == "get" {
						found[resource] = true
					}
				}
			}
		}
	}
	for _, resource := range []string{"namespaces", "storageclasses"} {
		if !found[resource] {
			t.Fatalf("SelfSubjectRulesReview omitted cluster-scoped local resource %s: %#v", resource, result.Status.ResourceRules)
		}
	}
}
