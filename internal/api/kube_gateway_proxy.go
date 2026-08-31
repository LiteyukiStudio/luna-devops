package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/kubeaccess"
	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	"github.com/LiteyukiStudio/devops/internal/kubepolicy"
	"github.com/LiteyukiStudio/devops/internal/kubeproxy"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimeaccess"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	kubeGatewayProxyTokenLifetime = 10 * time.Minute
	kubeGatewayClientCacheTTL     = 8 * time.Minute
	kubeGatewayAuditAction        = "kube_gateway.request"
)

type kubeGatewayClientKeyContextKey struct{}
type kubeGatewayClientCacheContextKey struct{}

// kubeGatewayClientCache is request-scoped. It avoids minting several
// TokenRequest credentials while authorization, metadata checks and the
// upstream proxy are resolving the same request. Long-lived streams refresh
// the metadata client before its short-lived token expires.
type kubeGatewayClientCache struct {
	mu        sync.Mutex
	clusterID string
	createdAt time.Time
	clients   *kubeGatewayClients
}

type kubeGatewayClients struct {
	cluster   model.RuntimeCluster
	baseURL   *url.URL
	transport http.RoundTripper
	typed     kubernetes.Interface
	dynamic   dynamic.Interface
}

func newKubeGateway(handlers *Handlers) *kubeproxy.Gateway {
	metadata := kubeGatewayMetadataReader{handlers: handlers}
	telemetryAdapter := kubeproxy.NewTelemetry(telemetry.Logger())
	return &kubeproxy.Gateway{
		Resolver:       kubeproxy.NewRequestInfoResolver(),
		Upstreams:      kubeGatewayUpstreamFactory{handlers: handlers},
		Preflight:      kubeGatewayAccessPreflight{handlers: handlers},
		MutationPolicy: kubeGatewayMutationPolicy{handlers: handlers},
		Ownership:      kubeproxy.OwnershipGuard{Reader: metadata},
		Mutator:        kubeproxy.NewMutator(),
		DryRunner:      kubeproxy.HTTPDryRunner{},
		Proxy:          kubeproxy.HTTPProxy{},
		Upgrade:        kubeproxy.UpgradeProxy{},
		Metrics:        kubeproxy.MetricsProxy{PodReader: metadata},
		LocalResources: kubeproxy.LocalResourceHandler{Source: kubeGatewayLocalResourceSource{handlers: handlers}},
		Limiter:        kubeproxy.NewLocalLimiter(kubeproxy.DefaultLimiterConfig()),
		Streams:        kubeproxy.DefaultStreamConfig(),
		Audit:          kubeproxy.AuditCoordinator{Recorder: kubeGatewayAuditRecorder{handlers: handlers}, Timeout: 2 * time.Second},
		Telemetry:      telemetryAdapter,
		ClientKey:      kubeGatewayClientKey,
	}
}

// HandleKubeGatewayRequest is the only Gin adapter for the Kubernetes wire
// protocol. It deliberately passes the original net/http request and its raw
// escaped path to kubeproxy; decoded Gin wildcard values are not authorization
// inputs.
func (h *Handlers) HandleKubeGatewayRequest(ctx *gin.Context) {
	if h == nil || h.kubeGateway == nil {
		kubeproxy.WriteStatus(ctx.Writer, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, errors.New("kube gateway is unavailable")))
		ctx.Abort()
		return
	}
	bindingID := strings.TrimSpace(ctx.Param("bindingId"))
	escapedPath, err := kubeproxy.ExtractEscapedKubePath(ctx.Request, bindingID)
	if err != nil {
		kubeproxy.WriteStatus(ctx.Writer, err)
		ctx.Abort()
		return
	}

	requestContext := context.WithValue(ctx.Request.Context(), kubeGatewayClientKeyContextKey{}, strings.TrimSpace(ctx.ClientIP()))
	requestContext = context.WithValue(requestContext, kubeGatewayClientCacheContextKey{}, &kubeGatewayClientCache{})
	request := ctx.Request.WithContext(requestContext)
	ctx.Request = request

	// CatalogAuthorizer must be request-local: successful authentication loads
	// the selected cluster's validated extra rules into this concrete instance,
	// which also keeps SelfSubjectRulesReview on the same catalog.
	authorizer := &kubeproxy.CatalogAuthorizer{Catalog: kubecatalog.New()}
	gateway := *h.kubeGateway
	gateway.Authenticator = &kubeGatewayAuthenticator{handlers: h, authorizer: authorizer}
	gateway.Authorizer = authorizer
	gateway.LocalReviews = kubeproxy.LocalReviewHandler{Authorizer: authorizer}
	gateway.Handle(ctx.Writer, request, bindingID, escapedPath)
	ctx.Abort()
}

func (h *Handlers) HandleKubeGatewayNoMethod(ctx *gin.Context) {
	ctx.Header("Allow", strings.Join(kubeGatewayHTTPMethods, ", "))
	h.writeKubeGatewayBoundaryStatus(ctx, kubeproxy.MethodNotAllowed())
}

func (h *Handlers) HandleKubeGatewayNoRoute(ctx *gin.Context) {
	h.writeKubeGatewayBoundaryStatus(ctx, kubeGatewayRouteNotFound())
}

// writeKubeGatewayBoundaryStatus keeps router-level protocol failures inside
// the same redacted telemetry boundary as routed Kubernetes requests. The
// generic Gin middleware deliberately excludes every /kube/ path.
func (h *Handlers) writeKubeGatewayBoundaryStatus(ctx *gin.Context, responseErr error) {
	if ctx == nil || ctx.Request == nil {
		return
	}
	telemetryAdapter := (*kubeproxy.Telemetry)(nil)
	if h != nil && h.kubeGateway != nil {
		telemetryAdapter = h.kubeGateway.Telemetry
	}
	if telemetryAdapter == nil {
		telemetryAdapter = kubeproxy.NewTelemetry(telemetry.Logger())
	}
	info := kubeproxy.RequestInfo{Method: ctx.Request.Method, Transport: kubeproxy.TransportNormal}
	request, boundary := telemetryAdapter.StartRequest(ctx.Request, info, "discovery")
	ctx.Request = request
	status := kubeproxy.AsStatusError(responseErr)
	kubeproxy.WriteStatus(ctx.Writer, status)
	boundary.End(status.HTTPStatus, status.Code, status)
	ctx.Abort()
}

func kubeGatewayRouteNotFound() error {
	return kubeproxy.NotFound(schema.GroupVersionResource{}, "")
}

type kubeGatewayAuthenticator struct {
	handlers   *Handlers
	authorizer *kubeproxy.CatalogAuthorizer
}

func (authenticator *kubeGatewayAuthenticator) Authenticate(ctx context.Context, credential, bindingID string) (kubeproxy.AccessContext, error) {
	if authenticator == nil || authenticator.handlers == nil || authenticator.handlers.kubeAccess == nil || authenticator.authorizer == nil {
		return kubeproxy.AccessContext{}, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, errors.New("kube authentication is unavailable"))
	}
	authentication, err := authenticator.handlers.kubeAccess.Authenticate(ctx, credential, bindingID)
	if err != nil {
		return kubeproxy.AccessContext{}, kubeGatewayAuthenticationError(err)
	}
	access := kubeGatewayAccessContext(authentication)
	catalog, err := kubeGatewayCatalog(ctx, authentication.Cluster)
	if err != nil {
		return kubeproxy.AccessContext{}, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	authenticator.authorizer.Catalog = catalog
	return access, nil
}

func (authenticator *kubeGatewayAuthenticator) Revalidate(ctx context.Context, previous kubeproxy.AccessContext) (kubeproxy.AccessContext, error) {
	if authenticator == nil || authenticator.handlers == nil || authenticator.handlers.kubeAccess == nil || authenticator.authorizer == nil {
		return kubeproxy.AccessContext{}, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, errors.New("kube authentication is unavailable"))
	}
	authentication, err := authenticator.handlers.kubeAccess.Revalidate(ctx, kubeaccess.Authentication{
		Token: model.AccessToken{ID: previous.CredentialID}, Binding: model.KubeAccessBinding{ID: previous.BindingID},
	})
	if err != nil {
		return kubeproxy.AccessContext{}, kubeGatewayAuthenticationError(err)
	}
	access := kubeGatewayAccessContext(authentication)
	catalog, err := kubeGatewayCatalog(ctx, authentication.Cluster)
	if err != nil {
		return kubeproxy.AccessContext{}, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	authenticator.authorizer.Catalog = catalog
	return access, nil
}

func kubeGatewayAccessContext(authentication kubeaccess.Authentication) kubeproxy.AccessContext {
	applicationID := ""
	if authentication.Binding.ApplicationID != nil {
		applicationID = strings.TrimSpace(*authentication.Binding.ApplicationID)
	}
	expiresAt := time.Time{}
	if authentication.Token.ExpiresAt != nil {
		expiresAt = authentication.Token.ExpiresAt.UTC()
	}
	return kubeproxy.AccessContext{
		UserID: authentication.User.ID, PlatformRole: authentication.User.Role, ProjectRole: authentication.Access.Role,
		CredentialID: authentication.Token.ID, BindingID: authentication.Binding.ID,
		ProjectID: authentication.Project.ID, ApplicationID: applicationID,
		RuntimeClusterID: authentication.Cluster.ID, Namespace: authentication.Project.KubernetesNamespace,
		Scopes: authentication.Token.Scope, ExpiresAt: expiresAt,
	}
}

func kubeGatewayAuthenticationError(err error) error {
	switch {
	case errors.Is(err, kubeaccess.ErrCredentialInvalid),
		errors.Is(err, kubeaccess.ErrCredentialNotFound),
		errors.Is(err, kubeaccess.ErrPermissionDenied),
		errors.Is(err, kubeaccess.ErrContextInvalid),
		errors.Is(err, kubeaccess.ErrGatewayDisabled),
		errors.Is(err, gorm.ErrRecordNotFound):
		return kubeproxy.Unauthorized(err)
	case errors.Is(err, kubeaccess.ErrGatewayReconciling):
		return kubeproxy.Unavailable("kube_gateway.reconciling", err)
	default:
		return kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
}

type namespacedKubeScopeResolver struct{}

func (namespacedKubeScopeResolver) IsNamespaced(context.Context, schema.GroupVersionResource) (bool, error) {
	// Extra rules are validated against live discovery before persistence.
	// The request path only reconstructs that immutable validated catalog; the
	// namespace RoleBinding remains the final upstream containment boundary.
	return true, nil
}

func kubeGatewayCatalog(ctx context.Context, cluster model.RuntimeCluster) (*kubecatalog.Catalog, error) {
	rules, err := decodeRuntimeClusterKubeGatewayRules(cluster.KubeGatewayExtraResourceRules)
	if err != nil {
		return nil, err
	}
	extra := make([]kubecatalog.ExtraResourceRule, 0, len(rules))
	for _, rule := range rules {
		extra = append(extra, kubecatalog.ExtraResourceRule{
			APIGroup: rule.APIGroup, APIVersion: rule.APIVersion, Resource: rule.Resource,
			Subresources: append([]string(nil), rule.Subresources...), Verbs: append([]string(nil), rule.Verbs...),
			Action: authz.Action(rule.Action),
		})
	}
	if len(extra) == 0 {
		return kubecatalog.New(), nil
	}
	return kubecatalog.NewWithExtra(ctx, namespacedKubeScopeResolver{}, extra)
}

type kubeGatewayUpstreamFactory struct{ handlers *Handlers }

func (factory kubeGatewayUpstreamFactory) ForBinding(ctx context.Context, access kubeproxy.AccessContext) (kubeproxy.Upstream, error) {
	clients, err := factory.handlers.kubeGatewayClients(ctx, access)
	if err != nil {
		return kubeproxy.Upstream{}, err
	}
	return kubeproxy.Upstream{BaseURL: clients.baseURL, Transport: clients.transport}, nil
}

func (h *Handlers) kubeGatewayClients(ctx context.Context, access kubeproxy.AccessContext) (*kubeGatewayClients, error) {
	if h == nil || h.dbWithContext(ctx) == nil {
		return nil, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, errors.New("kube gateway dependencies are unavailable"))
	}
	if strings.TrimSpace(access.RuntimeClusterID) == "" {
		return nil, kubeproxy.Unauthorized(errors.New("runtime cluster binding is missing"))
	}
	if cache, _ := ctx.Value(kubeGatewayClientCacheContextKey{}).(*kubeGatewayClientCache); cache != nil {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		if cache.clients != nil && cache.clusterID == access.RuntimeClusterID && time.Since(cache.createdAt) < kubeGatewayClientCacheTTL {
			return cache.clients, nil
		}
		clients, err := h.newKubeGatewayClients(ctx, access)
		if err != nil {
			return nil, err
		}
		cache.clusterID, cache.createdAt, cache.clients = access.RuntimeClusterID, time.Now(), clients
		return clients, nil
	}
	return h.newKubeGatewayClients(ctx, access)
}

func (h *Handlers) newKubeGatewayClients(ctx context.Context, access kubeproxy.AccessContext) (*kubeGatewayClients, error) {
	var cluster model.RuntimeCluster
	if err := runtimecluster.ActiveScope(h.dbWithContext(ctx)).First(&cluster, "id = ? and type in ?", access.RuntimeClusterID, []string{"kubernetes", "k3s"}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, kubeproxy.Unauthorized(err)
		}
		return nil, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	if !cluster.KubeGatewayEnabled {
		return nil, kubeproxy.Unauthorized(kubeaccess.ErrGatewayDisabled)
	}
	manager, _, err := h.runtimeClusterKubeGatewayManagerAndSpec(ctx, cluster)
	if err != nil {
		return nil, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	config, err := manager.BuildGatewayProxyRESTConfig(ctx, kubeprovider.GatewayTokenRequestOptions{Expiration: kubeGatewayProxyTokenLifetime})
	if err != nil {
		return nil, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	baseURL, err := url.Parse(strings.TrimSpace(config.Host))
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.User != nil {
		return nil, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, errors.New("kube API server URL is invalid"))
	}
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	typedClient, err := kubernetes.NewForConfigAndClient(config, httpClient)
	if err != nil {
		return nil, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	dynamicClient, err := dynamic.NewForConfigAndClient(config, httpClient)
	if err != nil {
		return nil, kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &kubeGatewayClients{cluster: cluster, baseURL: baseURL, transport: transport, typed: typedClient, dynamic: dynamicClient}, nil
}

type kubeGatewayMetadataReader struct{ handlers *Handlers }

func (reader kubeGatewayMetadataReader) ReadMetadata(ctx context.Context, access kubeproxy.AccessContext, info kubeproxy.RequestInfo) (metav1.Object, error) {
	clients, err := reader.handlers.kubeGatewayClients(ctx, access)
	if err != nil {
		return nil, err
	}
	return kubeGatewayDynamicResource(clients.dynamic, access, info.GVR()).Get(ctx, info.Name, metav1.GetOptions{})
}

func (reader kubeGatewayMetadataReader) ReadPodMetadata(ctx context.Context, access kubeproxy.AccessContext, name string) (metav1.Object, error) {
	clients, err := reader.handlers.kubeGatewayClients(ctx, access)
	if err != nil {
		return nil, err
	}
	return clients.typed.CoreV1().Pods(access.Namespace).Get(ctx, name, metav1.GetOptions{})
}

func kubeGatewayDynamicResource(client dynamic.Interface, access kubeproxy.AccessContext, gvr schema.GroupVersionResource) dynamic.ResourceInterface {
	return client.Resource(gvr).Namespace(access.Namespace)
}

type kubeGatewayReferenceResolver struct {
	handlers *Handlers
	access   kubeproxy.AccessContext
}

func (resolver kubeGatewayReferenceResolver) ResolveMetadata(ctx context.Context, reference kubepolicy.ObjectReference) (metav1.Object, error) {
	if reference.Namespace != "" && reference.Namespace != resolver.access.Namespace {
		return nil, apierrors.NewNotFound(reference.GVR.GroupResource(), reference.Name)
	}
	clients, err := resolver.handlers.kubeGatewayClients(ctx, resolver.access)
	if err != nil {
		return nil, err
	}
	return kubeGatewayDynamicResource(clients.dynamic, resolver.access, reference.GVR).Get(ctx, reference.Name, metav1.GetOptions{})
}

type kubeGatewayMutationPolicy struct{ handlers *Handlers }

func (provider kubeGatewayMutationPolicy) MutationContext(ctx context.Context, access kubeproxy.AccessContext, info kubeproxy.RequestInfo) (kubeproxy.MutationContext, error) {
	clients, err := provider.handlers.kubeGatewayClients(ctx, access)
	if err != nil {
		return kubeproxy.MutationContext{}, err
	}
	existingLabels := map[string]string(nil)
	if info.Name != "" {
		object, err := kubeGatewayDynamicResource(clients.dynamic, access, info.GVR()).Get(ctx, info.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) && kubeGatewayApplyMayCreate(info) {
				// Server-side apply is an upsert. A missing object has no
				// ownership labels to preserve, so validation continues with the
				// same creation policy used for POST. Other PATCH types remain
				// object-only and fail closed below.
				existingLabels = nil
			} else if apierrors.IsNotFound(err) {
				return kubeproxy.MutationContext{}, kubeproxy.NotFound(info.GVR(), info.Name)
			} else {
				return kubeproxy.MutationContext{}, err
			}
		} else {
			existingLabels = object.GetLabels()
		}
	}

	allowedDomains := make([]string, 0, 4)
	for _, suffix := range decodeGatewayDomainSuffixes(clients.cluster.GatewayDomainSuffixesRaw, clients.cluster.GatewayRootDomain, "") {
		suffix = strings.Trim(strings.ToLower(strings.TrimSpace(suffix)), ".")
		if suffix != "" {
			allowedDomains = append(allowedDomains, suffix, "*."+suffix)
		}
	}
	ingressClass := strings.TrimSpace(clients.cluster.GatewayClassName)
	if ingressClass == "" {
		ingressClass = "traefik"
	}
	return kubeproxy.MutationContext{
		ExistingLabels:    existingLabels,
		ReferenceResolver: kubeGatewayReferenceResolver{handlers: provider.handlers, access: access},
		// kubectl input can never nominate an internally trusted ServiceAccount.
		TrustedServiceAccounts: map[string]struct{}{},
		AllowedDomains:         allowedDomains,
		AllowedIngressClasses:  map[string]struct{}{ingressClass: {}},
		AllowedGatewayParents:  kubeGatewayAllowedGatewayParents(clients.cluster),
	}, nil
}

func kubeGatewayApplyMayCreate(info kubeproxy.RequestInfo) bool {
	return info.IsApplyPatch && info.Verb == "patch" && info.Name != "" && info.Subresource == ""
}

func kubeGatewayAllowedGatewayParents(cluster model.RuntimeCluster) map[string]struct{} {
	namespace := strings.TrimSpace(cluster.GatewayNamespace)
	if namespace == "" {
		namespace = "kube-system"
	}
	name := strings.TrimSpace(cluster.GatewayName)
	if name == "" {
		name = "luna-gateway"
	}
	return map[string]struct{}{kubepolicy.GatewayParentKey(namespace, name): {}}
}

type kubeGatewayAccessPreflight struct{ handlers *Handlers }

func (preflight kubeGatewayAccessPreflight) Check(ctx context.Context, access kubeproxy.AccessContext, info kubeproxy.RequestInfo, request *http.Request) error {
	connect := info.Verb == "connect" && (info.Subresource == "exec" || info.Subresource == "attach" || info.Subresource == "portforward")
	ephemeralContainers := info.Resource == "pods" && info.Subresource == "ephemeralcontainers" && (info.Verb == "update" || info.Verb == "patch")
	if !connect && !ephemeralContainers {
		return nil
	}
	clients, err := preflight.handlers.kubeGatewayClients(ctx, access)
	if err != nil {
		return err
	}
	pod, err := clients.typed.CoreV1().Pods(access.Namespace).Get(ctx, info.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return kubeproxy.NotFound(info.GVR(), info.Name)
		}
		return kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	if !kubeGatewayMetadataOwnedByAccess(&pod.ObjectMeta, access) {
		return kubeproxy.NotFound(info.GVR(), info.Name)
	}
	if err := preflight.ensureWebConsoleAllowed(ctx, access, pod); err != nil {
		return err
	}
	if info.Subresource != "portforward" {
		return nil
	}
	requested, err := kubeproxy.RequestedPortForwardPorts(request)
	if err != nil {
		return err
	}
	services, err := clients.typed.CoreV1().Services(access.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	allowed := kubeGatewayPortForwardPorts(pod, services.Items)
	for _, port := range requested {
		if _, ok := allowed[port]; !ok {
			return kubeproxy.Forbidden("kube_gateway.port_forward_denied", errors.New("port is not declared by the pod or a matching Service"))
		}
	}
	return nil
}

func (preflight kubeGatewayAccessPreflight) ensureWebConsoleAllowed(ctx context.Context, access kubeproxy.AccessContext, pod *corev1.Pod) error {
	var project model.Project
	if err := preflight.handlers.dbWithContext(ctx).First(&project, "id = ? and delete_status = ?", access.ProjectID, "active").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kubeproxy.Forbidden("kube_gateway.web_console_disabled", err)
		}
		return kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	labels := pod.GetLabels()
	if labels[kubepolicy.ManagementSourceLabel] == string(kubepolicy.ManagementSourceKubectl) {
		if runtimeaccess.Enabled(project.WebConsoleEnabled, nil) {
			return nil
		}
		return kubeproxy.Forbidden("kube_gateway.web_console_disabled", errors.New("project web console is disabled"))
	}
	targetID := strings.TrimSpace(labels[kubeprovider.DeploymentTargetIDLabel])
	if targetID == "" {
		return kubeproxy.Forbidden("kube_gateway.web_console_disabled", errors.New("platform workload has no deployment target"))
	}
	var target model.DeploymentTarget
	query := preflight.handlers.dbWithContext(ctx).Where("id = ? and project_id = ?", targetID, access.ProjectID)
	if access.ApplicationID != "" {
		query = query.Where("application_id = ?", access.ApplicationID)
	}
	if err := query.First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kubeproxy.Forbidden("kube_gateway.web_console_disabled", err)
		}
		return kubeproxy.Unavailable(kubeproxy.CodeUnavailable, err)
	}
	if !resourceCanMutateDuringDelete(target.DeleteStatus) || !runtimeaccess.Enabled(project.WebConsoleEnabled, target.WebConsoleEnabled) {
		return kubeproxy.Forbidden("kube_gateway.web_console_disabled", errors.New("deployment web console is disabled"))
	}
	return nil
}

func kubeGatewayMetadataOwnedByAccess(object metav1.Object, access kubeproxy.AccessContext) bool {
	if object == nil || object.GetNamespace() != access.Namespace {
		return false
	}
	values := object.GetLabels()
	source := values[kubepolicy.ManagementSourceLabel]
	if values[kubepolicy.ManagedByLabel] != kubepolicy.ManagedByValue || values[kubepolicy.ProjectIDLabel] != access.ProjectID ||
		(source != string(kubepolicy.ManagementSourcePlatform) && source != string(kubepolicy.ManagementSourceKubectl)) {
		return false
	}
	return access.ApplicationID == "" || values[kubepolicy.ApplicationIDLabel] == access.ApplicationID
}

func kubeGatewayPortForwardPorts(pod *corev1.Pod, services []corev1.Service) map[int32]struct{} {
	allowed := map[int32]struct{}{}
	named := map[string]int32{}
	if pod == nil {
		return allowed
	}
	containers := append([]corev1.Container(nil), pod.Spec.InitContainers...)
	containers = append(containers, pod.Spec.Containers...)
	for _, container := range containers {
		for _, port := range container.Ports {
			if port.ContainerPort > 0 {
				allowed[port.ContainerPort] = struct{}{}
				if port.Name != "" {
					named[port.Name] = port.ContainerPort
				}
			}
		}
	}
	podLabels := labels.Set(pod.Labels)
	for _, service := range services {
		if len(service.Spec.Selector) == 0 || !labels.SelectorFromSet(service.Spec.Selector).Matches(podLabels) {
			continue
		}
		for _, port := range service.Spec.Ports {
			switch {
			case port.TargetPort.StrVal != "":
				if value := named[port.TargetPort.StrVal]; value > 0 {
					allowed[value] = struct{}{}
				}
			case port.TargetPort.IntVal > 0:
				allowed[port.TargetPort.IntVal] = struct{}{}
			case port.Port > 0:
				allowed[port.Port] = struct{}{}
			}
		}
	}
	return allowed
}

type kubeGatewayLocalResourceSource struct{ handlers *Handlers }

func (source kubeGatewayLocalResourceSource) StorageClasses(ctx context.Context, access kubeproxy.AccessContext) ([]storagev1.StorageClass, error) {
	if source.handlers == nil || source.handlers.volumeClusters == nil {
		return nil, errors.New("storage class source is unavailable")
	}
	items, err := source.handlers.volumeClusters.ListStorageClasses(ctx, access.RuntimeClusterID)
	if err != nil {
		return nil, err
	}
	result := make([]storagev1.StorageClass, 0, len(items))
	for _, item := range items {
		class := storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: item.Name}, Provisioner: item.Provisioner}
		allowExpansion := item.AllowVolumeExpansion
		class.AllowVolumeExpansion = &allowExpansion
		if item.IsDefault {
			class.Annotations = map[string]string{
				"storageclass.kubernetes.io/is-default-class":      "true",
				"storageclass.beta.kubernetes.io/is-default-class": "true",
			}
		}
		if value := strings.TrimSpace(item.VolumeBindingMode); value != "" {
			mode := storagev1.VolumeBindingMode(value)
			class.VolumeBindingMode = &mode
		}
		if value := strings.TrimSpace(item.ReclaimPolicy); value != "" {
			policy := corev1.PersistentVolumeReclaimPolicy(value)
			class.ReclaimPolicy = &policy
		}
		result = append(result, class)
	}
	return result, nil
}

func kubeGatewayClientKey(request *http.Request) (kubeproxy.ClientKey, error) {
	if request == nil {
		return kubeproxy.ClientKey{}, errors.New("request is unavailable")
	}
	value, _ := request.Context().Value(kubeGatewayClientKeyContextKey{}).(string)
	if value == "" {
		value = strings.TrimSpace(request.RemoteAddr)
		if host, _, err := net.SplitHostPort(value); err == nil {
			value = host
		}
	}
	if value == "" {
		return kubeproxy.ClientKey{}, errors.New("client address is unavailable")
	}
	digest := sha256.Sum256([]byte(value))
	return kubeproxy.ClientKey{Value: hex.EncodeToString(digest[:16])}, nil
}

type kubeGatewayAuditRecorder struct{ handlers *Handlers }

type kubeProtocolAuditMetadata struct {
	CredentialID     string `json:"credentialId,omitempty"`
	BindingID        string `json:"bindingId,omitempty"`
	ProjectID        string `json:"projectId,omitempty"`
	ApplicationID    string `json:"applicationId,omitempty"`
	RuntimeClusterID string `json:"runtimeClusterId,omitempty"`
	Namespace        string `json:"namespace,omitempty"`
	APIGroup         string `json:"apiGroup,omitempty"`
	APIVersion       string `json:"apiVersion,omitempty"`
	Resource         string `json:"resource,omitempty"`
	Subresource      string `json:"subresource,omitempty"`
	Verb             string `json:"verb,omitempty"`
	ObjectName       string `json:"objectName,omitempty"`
	Transport        string `json:"transport,omitempty"`
	RequestID        string `json:"requestId,omitempty"`
	TraceID          string `json:"traceId,omitempty"`
	Allowed          *bool  `json:"allowed,omitempty"`
	StatusCode       int    `json:"statusCode,omitempty"`
	Outcome          string `json:"outcome,omitempty"`
	ErrorCode        string `json:"errorCode,omitempty"`
	StreamTerminal   string `json:"streamTerminal,omitempty"`
	DurationMS       int64  `json:"durationMs,omitempty"`
	FinishedAt       string `json:"finishedAt,omitempty"`
}

func (recorder kubeGatewayAuditRecorder) Begin(ctx context.Context, event kubeproxy.AuditEvent) (kubeproxy.AuditAttempt, error) {
	if recorder.handlers == nil || recorder.handlers.dbWithContext(ctx) == nil {
		return kubeproxy.AuditAttempt{}, errors.New("audit database is unavailable")
	}
	metadata := kubeProtocolAuditMetadataForEvent(ctx, event)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return kubeproxy.AuditAttempt{}, err
	}
	startedAt := event.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	idValue := id.New("aud")
	metadataValue := string(encoded)
	entry := model.AuditLog{
		ID: idValue, UserID: event.ActorID, Action: kubeGatewayAuditAction, Resource: kubeGatewayAuditResource(event),
		Success: false, Message: "started", Metadata: &metadataValue, CreatedAt: startedAt,
	}
	if err := recorder.handlers.dbWithContext(ctx).Create(&entry).Error; err != nil {
		return kubeproxy.AuditAttempt{}, errors.New("audit write failed")
	}
	return kubeproxy.AuditAttempt{ID: idValue}, nil
}

func (recorder kubeGatewayAuditRecorder) Finish(ctx context.Context, attempt kubeproxy.AuditAttempt, result kubeproxy.AuditResult) error {
	if recorder.handlers == nil || recorder.handlers.dbWithContext(ctx) == nil || strings.TrimSpace(attempt.ID) == "" {
		return errors.New("audit finalization is unavailable")
	}
	var entry model.AuditLog
	if err := recorder.handlers.dbWithContext(ctx).Select("id", "metadata").First(&entry, "id = ?", attempt.ID).Error; err != nil {
		return errors.New("audit read failed")
	}
	metadata := kubeProtocolAuditMetadata{}
	if entry.Metadata != nil && strings.TrimSpace(*entry.Metadata) != "" {
		if err := json.Unmarshal([]byte(*entry.Metadata), &metadata); err != nil {
			return errors.New("audit metadata is invalid")
		}
	}
	applyKubeGatewayAuditResult(&metadata, result)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	metadataValue := string(encoded)
	success := result.Allowed && result.StatusCode < http.StatusBadRequest && result.Outcome == "succeeded"
	update := recorder.handlers.dbWithContext(ctx).Model(&model.AuditLog{}).Where("id = ?", attempt.ID).Updates(map[string]any{
		"success": success, "message": metadata.Outcome, "metadata": &metadataValue,
	})
	if update.Error != nil || update.RowsAffected != 1 {
		return errors.New("audit finalization failed")
	}
	return nil
}

func (recorder kubeGatewayAuditRecorder) RecordDenial(ctx context.Context, event kubeproxy.AuditEvent, result kubeproxy.AuditResult) error {
	if recorder.handlers == nil || recorder.handlers.dbWithContext(ctx) == nil {
		return errors.New("audit database is unavailable")
	}
	metadata := kubeProtocolAuditMetadataForEvent(ctx, event)
	applyKubeGatewayAuditResult(&metadata, result)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	createdAt := event.StartedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	metadataValue := string(encoded)
	entry := model.AuditLog{
		ID: id.New("aud"), UserID: event.ActorID, Action: kubeGatewayAuditAction,
		Resource: kubeGatewayAuditResource(event), Success: false, Message: metadata.Outcome,
		Metadata: &metadataValue, CreatedAt: createdAt,
	}
	if err := recorder.handlers.dbWithContext(ctx).Create(&entry).Error; err != nil {
		return errors.New("audit write failed")
	}
	return nil
}

func kubeProtocolAuditMetadataForEvent(ctx context.Context, event kubeproxy.AuditEvent) kubeProtocolAuditMetadata {
	return kubeProtocolAuditMetadata{
		CredentialID: event.CredentialID, BindingID: event.BindingID, ProjectID: event.ProjectID,
		ApplicationID: event.ApplicationID, RuntimeClusterID: event.RuntimeClusterID, Namespace: event.Namespace,
		APIGroup: event.APIGroup, APIVersion: event.APIVersion, Resource: event.Resource,
		Subresource: event.Subresource, Verb: event.Verb, ObjectName: event.ObjectName,
		Transport: string(event.Transport), RequestID: telemetry.RequestIDFromContext(ctx), TraceID: event.TraceID,
	}
}

func applyKubeGatewayAuditResult(metadata *kubeProtocolAuditMetadata, result kubeproxy.AuditResult) {
	if metadata == nil {
		return
	}
	allowed := result.Allowed
	metadata.Allowed = &allowed
	metadata.StatusCode = result.StatusCode
	metadata.Outcome = strings.TrimSpace(result.Outcome)
	metadata.ErrorCode = strings.TrimSpace(result.ErrorCode)
	metadata.StreamTerminal = strings.TrimSpace(result.StreamTerminal)
	if result.Duration > 0 {
		metadata.DurationMS = result.Duration.Milliseconds()
	}
	if !result.FinishedAt.IsZero() {
		metadata.FinishedAt = result.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
}

func kubeGatewayAuditResource(event kubeproxy.AuditEvent) string {
	group := strings.TrimSpace(event.APIGroup)
	if group == "" {
		group = "core"
	}
	resource := strings.Trim(strings.Join([]string{group, event.APIVersion, event.Resource}, "/"), "/")
	if resource == "" || resource == "core" {
		return "kubernetes.discovery"
	}
	return "kubernetes." + resource
}

// Compile-time interface checks keep the API integration synchronized with
// kubeproxy's intentionally narrow security boundaries.
var (
	_ kubeproxy.Authenticator          = (*kubeGatewayAuthenticator)(nil)
	_ kubeproxy.UpstreamFactory        = kubeGatewayUpstreamFactory{}
	_ kubeproxy.MetadataReader         = kubeGatewayMetadataReader{}
	_ kubeproxy.PodMetadataReader      = kubeGatewayMetadataReader{}
	_ kubeproxy.MutationPolicyProvider = kubeGatewayMutationPolicy{}
	_ kubeproxy.AccessPreflight        = kubeGatewayAccessPreflight{}
	_ kubeproxy.LocalResourceSource    = kubeGatewayLocalResourceSource{}
	_ kubeproxy.AuditRecorder          = kubeGatewayAuditRecorder{}
	_ kubepolicy.ReferenceResolver     = kubeGatewayReferenceResolver{}
	_ kubecatalog.ScopeResolver        = namespacedKubeScopeResolver{}
)
