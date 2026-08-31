package kubeproxy

import (
	"context"
	"fmt"
	"net/url"

	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	"github.com/LiteyukiStudio/devops/internal/kubepolicy"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

type MetadataReader interface {
	ReadMetadata(context.Context, AccessContext, RequestInfo) (metav1.Object, error)
}

type OwnershipGuard struct {
	Reader MetadataReader
}

func (guard OwnershipGuard) ConstrainCollection(access AccessContext, decision Decision, query url.Values) error {
	if query == nil {
		return BadRequest(CodeBadRequest, fmt.Errorf("query is required"))
	}
	switch decision.Rule.CollectionPolicy {
	case kubecatalog.CollectionPolicyMetricsVerified:
		if access.ApplicationID == "" {
			return nil
		}
		fallthrough
	case kubecatalog.CollectionPolicyOwnershipSelector:
		forced, err := ownershipSelector(access)
		if err != nil {
			return Unavailable(CodeUnavailable, err)
		}
		client, err := labels.Parse(query.Get("labelSelector"))
		if err != nil {
			return BadRequest(CodeBadRequest, fmt.Errorf("invalid labelSelector: %w", err))
		}
		requirements, selectable := forced.Requirements()
		if !selectable {
			return Forbidden(CodeForbidden, fmt.Errorf("binding selector is not selectable"))
		}
		query.Set("labelSelector", client.Add(requirements...).String())
	case kubecatalog.CollectionPolicyProjectNamespace, kubecatalog.CollectionPolicyLocalSynthetic:
		return nil
	default:
		return Forbidden(CodeForbidden, fmt.Errorf("resource has no collection isolation policy"))
	}
	return nil
}

func ownershipSelector(access AccessContext) (labels.Selector, error) {
	values := map[string]string{
		kubepolicy.ManagedByLabel: ManagedByValue,
		kubepolicy.ProjectIDLabel: access.ProjectID,
	}
	if access.ApplicationID != "" {
		values[kubepolicy.ApplicationIDLabel] = access.ApplicationID
	}
	selector := labels.SelectorFromSet(values)
	source, err := labels.NewRequirement(kubepolicy.ManagementSourceLabel, selection.In, []string{string(kubepolicy.ManagementSourcePlatform), string(kubepolicy.ManagementSourceKubectl)})
	if err != nil {
		return nil, fmt.Errorf("build ownership selector: %w", err)
	}
	return selector.Add(*source), nil
}

const ManagedByValue = kubepolicy.ManagedByValue

func (guard OwnershipGuard) VerifyObject(ctx context.Context, access AccessContext, info RequestInfo) error {
	if info.Name == "" || info.IsDiscovery {
		return nil
	}
	if guard.Reader == nil {
		return Unavailable(CodeUnavailable, fmt.Errorf("metadata reader is unavailable"))
	}
	object, err := guard.Reader.ReadMetadata(ctx, access, info)
	if err != nil {
		if apierrors.IsNotFound(err) || AsStatusError(err).HTTPStatus == 404 {
			if allowsApplyCreate(info) {
				return nil
			}
			return NotFound(info.GVR(), info.Name)
		}
		return Unavailable(CodeUnavailable, err)
	}
	if object == nil {
		return NotFound(info.GVR(), info.Name)
	}
	objectLabels := object.GetLabels()
	source := objectLabels[kubepolicy.ManagementSourceLabel]
	if object.GetNamespace() != access.Namespace || objectLabels[kubepolicy.ManagedByLabel] != kubepolicy.ManagedByValue || objectLabels[kubepolicy.ProjectIDLabel] != access.ProjectID || source != string(kubepolicy.ManagementSourcePlatform) && source != string(kubepolicy.ManagementSourceKubectl) {
		return NotFound(info.GVR(), info.Name)
	}
	if access.ApplicationID != "" && objectLabels[kubepolicy.ApplicationIDLabel] != access.ApplicationID {
		return NotFound(info.GVR(), info.Name)
	}
	if info.Subresource == "status" && info.Verb != "get" && source != string(kubepolicy.ManagementSourceKubectl) {
		return Forbidden(CodeForbidden, fmt.Errorf("status writes are limited to kubectl-managed resources"))
	}
	return nil
}

func allowsApplyCreate(info RequestInfo) bool {
	return info.Name != "" && info.Subresource == "" && info.Verb == "patch" && info.IsApplyPatch
}
