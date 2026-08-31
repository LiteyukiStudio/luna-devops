package kubeproxy

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	"github.com/LiteyukiStudio/devops/internal/kubepolicy"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestOwnershipSelectorAndsClientAndBindingSelectors(t *testing.T) {
	access := baseAccess()
	access.ApplicationID = "a1"
	rule, _ := kubecatalog.New().Lookup(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"})
	query := url.Values{"labelSelector": []string{"environment=prod"}}
	if err := (OwnershipGuard{}).ConstrainCollection(access, Decision{Rule: rule}, query); err != nil {
		t.Fatal(err)
	}
	selector := query.Get("labelSelector")
	for _, expected := range []string{"environment=prod", "luna.devops/project-id=p1", "luna.devops/application-id=a1", "app.kubernetes.io/managed-by=luna-devops", "luna.devops/management-source"} {
		if !strings.Contains(selector, expected) {
			t.Fatalf("selector %q missing %q", selector, expected)
		}
	}
}

func TestProjectMetricsCollectionDoesNotChangeClientSelector(t *testing.T) {
	access := baseAccess()
	rule, _ := kubecatalog.New().Lookup(schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"})
	query := url.Values{"labelSelector": []string{"environment=prod"}}
	if err := (OwnershipGuard{}).ConstrainCollection(access, Decision{Rule: rule}, query); err != nil {
		t.Fatal(err)
	}
	if query.Get("labelSelector") != "environment=prod" {
		t.Fatalf("project metrics selector was unexpectedly changed: %q", query.Get("labelSelector"))
	}
}

type ownershipReaderFunc func(context.Context, AccessContext, RequestInfo) (metav1.Object, error)

func (reader ownershipReaderFunc) ReadMetadata(ctx context.Context, access AccessContext, info RequestInfo) (metav1.Object, error) {
	return reader(ctx, access, info)
}

func TestOwnershipFailsClosedOnMissingSourceAndPreservesDependencyFailure(t *testing.T) {
	access := baseAccess()
	info := RequestInfo{Verb: "get", APIVersion: "v1", Resource: "pods", Namespace: access.Namespace, Name: "app", IsResourceRequest: true}
	guard := OwnershipGuard{Reader: ownershipReaderFunc(func(context.Context, AccessContext, RequestInfo) (metav1.Object, error) {
		return &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: access.Namespace, Labels: map[string]string{
			kubepolicy.ManagedByLabel: kubepolicy.ManagedByValue, kubepolicy.ProjectIDLabel: access.ProjectID,
		}}}, nil
	})}
	if err := guard.VerifyObject(t.Context(), access, info); AsStatusError(err).HTTPStatus != 404 {
		t.Fatalf("object without management source must be hidden, got %v", err)
	}
	guard.Reader = ownershipReaderFunc(func(context.Context, AccessContext, RequestInfo) (metav1.Object, error) {
		return nil, errors.New("transport unavailable")
	})
	if err := guard.VerifyObject(t.Context(), access, info); AsStatusError(err).HTTPStatus != 503 {
		t.Fatalf("dependency failure must not be disguised as not found, got %v", err)
	}
}

func TestPlatformManagedStatusWriteIsForbidden(t *testing.T) {
	access := baseAccess()
	info := RequestInfo{Verb: "patch", APIGroup: "example.io", APIVersion: "v1", Resource: "widgets", Subresource: "status", Namespace: access.Namespace, Name: "app", IsResourceRequest: true}
	guard := OwnershipGuard{Reader: ownershipReaderFunc(func(context.Context, AccessContext, RequestInfo) (metav1.Object, error) {
		return &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: access.Namespace, Labels: map[string]string{
			kubepolicy.ManagedByLabel: kubepolicy.ManagedByValue, kubepolicy.ProjectIDLabel: access.ProjectID,
			kubepolicy.ManagementSourceLabel: string(kubepolicy.ManagementSourcePlatform),
		}}}, nil
	})}
	if err := guard.VerifyObject(t.Context(), access, info); AsStatusError(err).HTTPStatus != 403 {
		t.Fatalf("platform status write should be forbidden, got %v", err)
	}
}

func TestOwnershipAllowsNotFoundOnlyForServerSideApplyCreate(t *testing.T) {
	access := baseAccess()
	guard := OwnershipGuard{Reader: ownershipReaderFunc(func(context.Context, AccessContext, RequestInfo) (metav1.Object, error) {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "app")
	})}
	base := RequestInfo{Verb: "patch", APIGroup: "apps", APIVersion: "v1", Resource: "deployments", Namespace: access.Namespace, Name: "app", IsResourceRequest: true}
	apply := base
	apply.IsApplyPatch = true
	if err := guard.VerifyObject(t.Context(), access, apply); err != nil {
		t.Fatalf("server-side apply must be allowed to continue to create: %v", err)
	}
	if err := guard.VerifyObject(t.Context(), access, base); AsStatusError(err).HTTPStatus != http.StatusNotFound {
		t.Fatalf("ordinary patch must retain not-found ownership semantics: %v", err)
	}
	applyStatus := apply
	applyStatus.Subresource = "status"
	if err := guard.VerifyObject(t.Context(), access, applyStatus); AsStatusError(err).HTTPStatus != http.StatusNotFound {
		t.Fatalf("apply on a missing subresource must not create its parent: %v", err)
	}
}
