package kubeproxy

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	"github.com/LiteyukiStudio/devops/internal/kubepolicy"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type TransportClass string

const (
	TransportNormal  TransportClass = "normal"
	TransportWatch   TransportClass = "watch"
	TransportLogs    TransportClass = "logs"
	TransportUpgrade TransportClass = "upgrade"
)

type StreamClass string

const (
	StreamWatch   StreamClass = "watch"
	StreamLogs    StreamClass = "logs"
	StreamUpgrade StreamClass = "upgrade"
)

type AccessContext struct {
	UserID           string
	PlatformRole     string
	ProjectRole      string
	CredentialID     string
	BindingID        string
	ProjectID        string
	ApplicationID    string
	RuntimeClusterID string
	Namespace        string
	Scopes           string
	ExpiresAt        time.Time
}

func (access AccessContext) PolicyContext() kubepolicy.PolicyContext {
	return kubepolicy.PolicyContext{
		Namespace: access.Namespace, ProjectID: access.ProjectID, ApplicationID: access.ApplicationID,
		ManagementSource: kubepolicy.ManagementSourceKubectl,
	}
}

type RequestInfo struct {
	Verb              string
	APIGroup          string
	APIVersion        string
	Resource          string
	Subresource       string
	Namespace         string
	Name              string
	NonResourcePath   string
	IsResourceRequest bool
	IsWatch           bool
	IsUpgrade         bool
	IsApplyPatch      bool
	IsCollection      bool
	IsDiscovery       bool
	Transport         TransportClass
	UpgradeProtocol   string
	Method            string
}

func (info RequestInfo) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: info.APIGroup, Version: info.APIVersion, Resource: info.Resource}
}

type Decision struct {
	Allowed        bool
	Rule           kubecatalog.ResourceRule
	Actions        []authz.Action
	RequiredScopes []string
}

type Upstream struct {
	BaseURL          *url.URL
	Transport        http.RoundTripper
	UpgradeTransport UpgradeRequestRoundTripper
}

type UpgradeRequestRoundTripper interface {
	http.RoundTripper
	WrapRequest(*http.Request) (*http.Request, error)
}

type Authenticator interface {
	Authenticate(context.Context, string, string) (AccessContext, error)
	Revalidate(context.Context, AccessContext) (AccessContext, error)
}

type Authorizer interface {
	Authorize(context.Context, AccessContext, RequestInfo) (Decision, error)
}

type UpstreamFactory interface {
	ForBinding(context.Context, AccessContext) (Upstream, error)
}

type AccessPreflight interface {
	Check(context.Context, AccessContext, RequestInfo, *http.Request) error
}

type MutationPolicyProvider interface {
	MutationContext(context.Context, AccessContext, RequestInfo) (MutationContext, error)
}

type DryRunValidation struct {
	StatusCode    int
	Header        http.Header
	CanonicalJSON []byte
	ClientBody    []byte
}

type DryRunExecutor interface {
	Validate(context.Context, *http.Request, Upstream, string, RequestInfo) (DryRunValidation, error)
}

type ClientKey struct {
	Value string
}

type RequestClass string

const (
	RequestClassAnonymous RequestClass = "anonymous"
	RequestClassNormal    RequestClass = "normal"
)

type Limiter interface {
	AllowPreAuth(context.Context, ClientKey, RequestClass) error
	AllowRequest(context.Context, AccessContext, RequestInfo) error
	AcquireStream(context.Context, AccessContext, StreamClass) (func(), error)
}
