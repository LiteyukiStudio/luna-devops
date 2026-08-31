package kubepolicy

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	ManagedByLabel          = "app.kubernetes.io/managed-by"
	ManagedByValue          = "luna-devops"
	ProjectIDLabel          = "luna.devops/project-id"
	ApplicationIDLabel      = "luna.devops/application-id"
	ManagementSourceLabel   = "luna.devops/management-source"
	EnvironmentIDLabel      = "luna.devops/environment-id"
	DeploymentTargetIDLabel = "luna.devops/deployment-target-id"
	ReleaseIDLabel          = "luna.devops/release-id"
	GatewayRouteIDLabel     = "luna.devops/gateway-route-id"
	ProjectVolumeIDLabel    = "luna.devops/project-volume-id"
)

var protectedLifecycleLabelKeys = []string{
	DeploymentTargetIDLabel,
	ReleaseIDLabel,
	EnvironmentIDLabel,
	GatewayRouteIDLabel,
	ProjectVolumeIDLabel,
}

// ProtectedLifecycleLabelKeys returns the platform lifecycle labels that a
// kubectl request may preserve but must never assign or change.
func ProtectedLifecycleLabelKeys() []string {
	return append([]string(nil), protectedLifecycleLabelKeys...)
}

type ManagementSource string

const (
	ManagementSourcePlatform ManagementSource = "platform"
	ManagementSourceKubectl  ManagementSource = "kubectl"
)

type ServiceAccountOrigin string

const (
	ServiceAccountAbsent    ServiceAccountOrigin = "absent"
	ServiceAccountExplicit  ServiceAccountOrigin = "explicit"
	ServiceAccountUnchanged ServiceAccountOrigin = "unchanged"
	ServiceAccountTrusted   ServiceAccountOrigin = "trusted"
)

type PolicyContext struct {
	Namespace               string
	ProjectID               string
	ApplicationID           string
	ManagementSource        ManagementSource
	ExpectedOwnershipLabels map[string]string
	ExpectedLifecycleLabels map[string]string
	ProtectLifecycleLabels  bool
	ServiceAccountOrigin    ServiceAccountOrigin
	TrustedServiceAccounts  map[string]struct{}
	Resolver                ReferenceResolver
	AllowedDomains          []string
	AllowedIngressClasses   map[string]struct{}
	AllowedGatewayParents   map[string]struct{}
}

func (policy PolicyContext) ApplicationBinding() bool {
	return strings.TrimSpace(policy.ApplicationID) != ""
}

type ObjectReference struct {
	GVR       schema.GroupVersionResource
	Namespace string
	Name      string
}

type ReferenceResolver interface {
	ResolveMetadata(context.Context, ObjectReference) (metav1.Object, error)
}

func ValidateReference(ctx context.Context, policy PolicyContext, reference ObjectReference, path *field.Path) field.ErrorList {
	if path == nil {
		path = field.NewPath("reference")
	}
	reference.Namespace = strings.TrimSpace(reference.Namespace)
	reference.Name = strings.TrimSpace(reference.Name)
	if reference.Namespace != "" && reference.Namespace != policy.Namespace {
		return field.ErrorList{field.Forbidden(path, "cross-namespace references are not allowed")}
	}
	if reference.Name == "" {
		return field.ErrorList{field.Required(path, "reference name is required")}
	}
	if policy.Resolver == nil {
		return field.ErrorList{field.InternalError(path, fmt.Errorf("reference resolver is unavailable"))}
	}
	reference.Namespace = policy.Namespace
	object, err := policy.Resolver.ResolveMetadata(ctx, reference)
	if err != nil {
		return field.ErrorList{field.Invalid(path, reference.Name, "referenced object is unavailable or outside the binding")}
	}
	return ValidateReferenceOwnership(policy, object, path)
}

func ValidateReferenceOwnership(policy PolicyContext, object metav1.Object, path *field.Path) field.ErrorList {
	if object == nil {
		return field.ErrorList{field.Invalid(path, "", "referenced object has no metadata")}
	}
	if object.GetNamespace() != "" && object.GetNamespace() != policy.Namespace {
		return field.ErrorList{field.Forbidden(path, "referenced object is outside the project namespace")}
	}
	labels := object.GetLabels()
	source := labels[ManagementSourceLabel]
	if labels[ManagedByLabel] != ManagedByValue || labels[ProjectIDLabel] != policy.ProjectID || source != string(ManagementSourcePlatform) && source != string(ManagementSourceKubectl) {
		return field.ErrorList{field.Forbidden(path, "referenced object is not owned by this project")}
	}
	if policy.ApplicationBinding() && labels[ApplicationIDLabel] != policy.ApplicationID {
		return field.ErrorList{field.Forbidden(path, "referenced object is not owned by this application")}
	}
	return nil
}

func RequiredOwnershipLabels(policy PolicyContext) map[string]string {
	if len(policy.ExpectedOwnershipLabels) > 0 {
		labels := make(map[string]string, len(policy.ExpectedOwnershipLabels))
		for key, value := range policy.ExpectedOwnershipLabels {
			labels[key] = strings.TrimSpace(value)
		}
		return labels
	}
	labels := map[string]string{
		ManagedByLabel:        ManagedByValue,
		ProjectIDLabel:        strings.TrimSpace(policy.ProjectID),
		ManagementSourceLabel: string(policy.ManagementSource),
	}
	if policy.ApplicationBinding() {
		labels[ApplicationIDLabel] = strings.TrimSpace(policy.ApplicationID)
	}
	return labels
}

func RequiredSelectionLabels(policy PolicyContext) map[string]string {
	labels := map[string]string{ProjectIDLabel: strings.TrimSpace(policy.ProjectID)}
	if applicationID := strings.TrimSpace(policy.ExpectedOwnershipLabels[ApplicationIDLabel]); applicationID != "" {
		labels[ApplicationIDLabel] = applicationID
	} else if policy.ApplicationBinding() {
		labels[ApplicationIDLabel] = strings.TrimSpace(policy.ApplicationID)
	}
	return labels
}
