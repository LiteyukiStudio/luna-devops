package kubeproxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type ClientKeyFunc func(*http.Request) (ClientKey, error)

type Gateway struct {
	Resolver       RequestInfoResolver
	Authenticator  Authenticator
	Authorizer     Authorizer
	Upstreams      UpstreamFactory
	Preflight      AccessPreflight
	MutationPolicy MutationPolicyProvider
	Ownership      OwnershipGuard
	Mutator        Mutator
	DryRunner      DryRunExecutor
	Proxy          HTTPProxy
	Upgrade        UpgradeProxy
	Metrics        MetricsProxy
	LocalResources LocalResourceHandler
	LocalReviews   LocalReviewHandler
	Limiter        Limiter
	Streams        StreamConfig
	Audit          AuditCoordinator
	Telemetry      *Telemetry
	ClientKey      ClientKeyFunc
}

func (gateway *Gateway) Handle(writer http.ResponseWriter, request *http.Request, bindingID, escapedKubePath string) {
	tracked := &statusResponseWriter{ResponseWriter: writer, status: http.StatusOK}
	if gateway == nil || request == nil {
		WriteStatus(tracked, Unavailable(CodeUnavailable, fmt.Errorf("kube gateway request is unavailable")))
		return
	}
	configured := *gateway
	telemetry := configured.Telemetry
	if telemetry == nil {
		telemetry = NewTelemetry(nil)
	}
	configured.Telemetry = telemetry
	request, boundary := telemetry.StartRequest(request, RequestInfo{Method: request.Method}, "unknown")
	configured.Proxy.Telemetry = telemetry
	configured.Metrics.Telemetry = telemetry
	configured.Upgrade.Telemetry = telemetry
	switch dryRunner := configured.DryRunner.(type) {
	case HTTPDryRunner:
		dryRunner.Telemetry = telemetry
		configured.DryRunner = dryRunner
	case *HTTPDryRunner:
		if dryRunner != nil {
			copy := *dryRunner
			copy.Telemetry = telemetry
			configured.DryRunner = &copy
		}
	}
	var finalErr error
	defer func() {
		if recovered := recover(); recovered != nil {
			finalErr = Unavailable(CodeUnavailable, fmt.Errorf("kube gateway panic"))
			if !tracked.wroteHeader {
				WriteStatus(tracked, finalErr)
			}
		}
		code := ""
		if finalErr != nil {
			code = AsStatusError(finalErr).Code
		}
		boundary.End(tracked.status, code, finalErr)
	}()
	finalErr = configured.serve(tracked, request, bindingID, escapedKubePath, boundary)
	if finalErr != nil && !tracked.wroteHeader {
		WriteStatus(tracked, finalErr)
	}
}

func (gateway *Gateway) serve(writer *statusResponseWriter, request *http.Request, bindingID, escapedKubePath string, boundary *RequestTelemetry) error {
	if gateway.Authenticator == nil || gateway.Authorizer == nil || gateway.Upstreams == nil || gateway.Preflight == nil || gateway.Limiter == nil || gateway.ClientKey == nil {
		return Unavailable(CodeUnavailable, fmt.Errorf("kube gateway dependencies are incomplete"))
	}
	clientKey, err := gateway.ClientKey(request)
	if err != nil {
		return RateLimited(err)
	}
	if err := gateway.Limiter.AllowPreAuth(request.Context(), clientKey, RequestClassAnonymous); err != nil {
		return err
	}
	credential, err := bearerCredential(request.Header)
	if err != nil {
		return err
	}
	authCtx, authSpan := gateway.TelemetryOrDefault().StartInternal(request.Context(), "kube.gateway.authenticate", trace.SpanKindInternal)
	access, err := gateway.Authenticator.Authenticate(authCtx, credential, bindingID)
	if err != nil {
		authErr := authenticationError(err)
		authSpan.SetStatus(codes.Error, AsStatusError(authErr).Code)
		authSpan.End()
		return authErr
	}
	authSpan.End()
	if access.BindingID != bindingID {
		return Unauthorized(fmt.Errorf("binding does not match credential"))
	}
	info, err := gateway.Resolver.Resolve(request, escapedKubePath)
	if err != nil {
		return gateway.recordDenial(request.Context(), access, RequestInfo{Method: request.Method}, err)
	}
	forceSafeProtocolQuery(request, info)
	category := "discovery"
	if info.IsResourceRequest {
		if catalogAuthorizer, ok := catalogFromAuthorizer(gateway.Authorizer); ok {
			if rule, exists := catalogAuthorizer.Lookup(info.GVR()); exists {
				category = string(rule.Category)
			}
		}
	}
	boundary.Classify(info, category)
	if err := gateway.Limiter.AllowRequest(request.Context(), access, info); err != nil {
		return gateway.recordDenial(request.Context(), access, info, err)
	}
	authorizeCtx, authorizeSpan := gateway.TelemetryOrDefault().StartInternal(request.Context(), "kube.gateway.authorize", trace.SpanKindInternal)
	decision, err := gateway.Authorizer.Authorize(authorizeCtx, access, info)
	if err != nil {
		authorizeSpan.SetStatus(codes.Error, AsStatusError(err).Code)
	}
	authorizeSpan.End()
	if err != nil || !decision.Allowed {
		if err == nil {
			err = Forbidden(CodeForbidden, fmt.Errorf("authorization decision denied"))
		}
		return gateway.recordDenial(request.Context(), access, info, err)
	}
	if err := gateway.Preflight.Check(request.Context(), access, info, request); err != nil {
		return gateway.recordDenial(request.Context(), access, info, err)
	}
	if err := boundNonUpgradeRequestBody(request, info); err != nil {
		return gateway.recordDenial(request.Context(), access, info, err)
	}
	if info.IsCollection && (info.Verb == "list" || info.Verb == "watch" || info.Verb == "deletecollection") {
		query := request.URL.Query()
		if err := gateway.Ownership.ConstrainCollection(access, decision, query); err != nil {
			return gateway.recordDenial(request.Context(), access, info, err)
		}
		request.URL.RawQuery = query.Encode()
	}
	if info.Name != "" && !decision.Rule.Local && !(info.APIGroup == "metrics.k8s.io" && info.Resource == "pods") {
		if err := gateway.Ownership.VerifyObject(request.Context(), access, info); err != nil {
			return gateway.recordDenial(request.Context(), access, info, err)
		}
	}

	event := auditEvent(request.Context(), access, info)
	attempt, err := gateway.Audit.Begin(request.Context(), event, ShouldPersistAudit(info, decision))
	if err != nil {
		return err
	}
	streamClass, streaming := streamClassFor(info)
	finishAudit := func(operationErr error) {
		status := writer.status
		finishedAt := time.Now()
		result := AuditResult{Allowed: true, StatusCode: status, Outcome: "succeeded", FinishedAt: finishedAt, Duration: finishedAt.Sub(event.StartedAt)}
		if operationErr != nil {
			statusError := AsStatusError(operationErr)
			if !writer.wroteHeader {
				result.StatusCode = statusError.HTTPStatus
			}
			result.Outcome, result.ErrorCode = "failed", statusError.Code
		} else if status >= http.StatusBadRequest {
			result.Outcome = "failed"
		}
		if streaming {
			result.StreamTerminal = streamTerminal(request.Context(), operationErr)
		}
		if err := gateway.Audit.Finish(request.Context(), attempt, result); err != nil {
			gateway.TelemetryOrDefault().RecordAuditFailure(request.Context())
		}
	}

	if streaming {
		streamController := StreamController{
			Limiter: gateway.Limiter, Config: gateway.Streams,
			Revalidate: func(ctx context.Context, previous AccessContext) (AccessContext, error) {
				updated, err := gateway.Authenticator.Revalidate(ctx, previous)
				if err != nil {
					return AccessContext{}, err
				}
				decision, err := gateway.Authorizer.Authorize(ctx, updated, info)
				if err != nil {
					return AccessContext{}, err
				}
				if !decision.Allowed {
					return AccessContext{}, Forbidden(CodeForbidden, fmt.Errorf("stream authorization decision denied"))
				}
				if err := gateway.Preflight.Check(ctx, updated, info, request.WithContext(ctx)); err != nil {
					return AccessContext{}, err
				}
				if info.Name != "" && !(info.APIGroup == "metrics.k8s.io" && info.Resource == "pods") {
					if err := gateway.Ownership.VerifyObject(ctx, updated, info); err != nil {
						return AccessContext{}, err
					}
				}
				return updated, nil
			},
		}
		streamCtx, closeStream, err := streamController.Open(request.Context(), access, streamClass)
		if err != nil {
			finishAudit(err)
			return err
		}
		defer closeStream()
		request = request.WithContext(streamCtx)
	}

	if gateway.LocalReviews.Handles(info) {
		localReviews := gateway.LocalReviews
		if localReviews.Authorizer == nil {
			localReviews.Authorizer = gateway.Authorizer
		}
		err := localReviews.Serve(writer, request, access, info)
		finishAudit(err)
		return err
	}
	if gateway.LocalResources.Handles(info) {
		err := gateway.LocalResources.Serve(request.Context(), writer, request, access, info)
		finishAudit(err)
		return err
	}
	upstream, err := gateway.Upstreams.ForBinding(request.Context(), access)
	if err != nil {
		err = dependencyError(err, CodeUnavailable)
		finishAudit(err)
		return err
	}
	if requiresMutation(info) {
		if gateway.DryRunner == nil || gateway.MutationPolicy == nil {
			err := Unavailable(CodeUnavailable, fmt.Errorf("dry-run policy executor is unavailable"))
			finishAudit(err)
			return err
		}
		mutationContext, err := gateway.MutationPolicy.MutationContext(request.Context(), access, info)
		if err != nil {
			err = dependencyError(err, CodeUnavailable)
			finishAudit(err)
			return err
		}
		mutationContext.Access, mutationContext.Info = access, info
		mutator := gateway.Mutator
		if mutator.Validator == nil {
			mutator = NewMutator()
		}
		result, err := mutator.Prepare(request.Context(), mutationContext, request.Header.Get("Content-Type"), request.Body)
		if err != nil {
			finishAudit(err)
			return err
		}
		replaceRequestBody(request, result.Body)
		request.Header.Set("Content-Type", result.ContentType)
		validation, err := gateway.DryRunner.Validate(request.Context(), request, upstream, escapedKubePath, info)
		if err != nil {
			finishAudit(err)
			return err
		}
		if validation.StatusCode < 200 || validation.StatusCode >= 300 {
			err = writeDryRunResult(writer, request, validation)
			finishAudit(err)
			return err
		}
		if err := mutator.ValidateFinal(request.Context(), result.PolicyContext, validation.CanonicalJSON); err != nil {
			finishAudit(err)
			return err
		}
		if clientDryRun(request.URL.Query()) {
			err = writeDryRunResult(writer, request, validation)
			finishAudit(err)
			return err
		}
		replaceRequestBody(request, result.Body)
	}

	var serveErr error
	switch {
	case info.IsUpgrade:
		serveErr = gateway.Upgrade.Serve(writer, request, upstream, escapedKubePath)
	case info.APIGroup == "metrics.k8s.io" && info.Resource == "pods":
		serveErr = gateway.Metrics.Serve(writer, request, access, info, upstream, escapedKubePath)
	case info.IsDiscovery:
		serveErr = gateway.Proxy.Serve(writer, request, upstream, escapedKubePath, info, DiscoveryTransformer{KubePrefix: kubePrefix(bindingID), RequestPath: escapedKubePath})
	default:
		serveErr = gateway.Proxy.Serve(writer, request, upstream, escapedKubePath, info, DiscoveryTransformer{KubePrefix: kubePrefix(bindingID), RequestPath: escapedKubePath})
	}
	finishAudit(serveErr)
	return serveErr
}

func (gateway *Gateway) TelemetryOrDefault() *Telemetry {
	if gateway.Telemetry == nil {
		return NewTelemetry(nil)
	}
	return gateway.Telemetry
}

func bearerCredential(header http.Header) (string, error) {
	values := header.Values("Authorization")
	if len(values) != 1 {
		return "", Unauthorized(fmt.Errorf("exactly one Bearer Authorization header is required"))
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", Unauthorized(fmt.Errorf("Bearer credential is malformed"))
	}
	return parts[1], nil
}

func catalogFromAuthorizer(authorizer Authorizer) (*kubecatalog.Catalog, bool) {
	switch value := authorizer.(type) {
	case CatalogAuthorizer:
		return value.Catalog, value.Catalog != nil
	case *CatalogAuthorizer:
		return value.Catalog, value != nil && value.Catalog != nil
	default:
		return nil, false
	}
}

func (gateway *Gateway) recordDenial(ctx context.Context, access AccessContext, info RequestInfo, denial error) error {
	if gateway.Audit.Recorder == nil {
		return Unavailable(CodeAuditUnavailable, fmt.Errorf("audit recorder is unavailable for authenticated denial"))
	}
	status := AsStatusError(denial)
	result := AuditResult{Allowed: false, StatusCode: status.HTTPStatus, Outcome: "rejected", ErrorCode: status.Code, FinishedAt: time.Now()}
	if err := gateway.Audit.Recorder.RecordDenial(ctx, auditEvent(ctx, access, info), result); err != nil {
		return Unavailable(CodeAuditUnavailable, err)
	}
	return denial
}

func auditEvent(ctx context.Context, access AccessContext, info RequestInfo) AuditEvent {
	traceID := trace.SpanContextFromContext(ctx).TraceID().String()
	if traceID == "00000000000000000000000000000000" {
		traceID = ""
	}
	return AuditEvent{
		ActorID: access.UserID, CredentialID: access.CredentialID, BindingID: access.BindingID,
		ProjectID: access.ProjectID, ApplicationID: access.ApplicationID, RuntimeClusterID: access.RuntimeClusterID,
		Namespace: access.Namespace, APIGroup: info.APIGroup, APIVersion: info.APIVersion, Resource: info.Resource,
		Subresource: info.Subresource, Verb: info.Verb, ObjectName: info.Name, Transport: info.Transport, TraceID: traceID, StartedAt: time.Now(),
	}
}

func streamClassFor(info RequestInfo) (StreamClass, bool) {
	switch info.Transport {
	case TransportWatch:
		return StreamWatch, true
	case TransportLogs:
		return StreamLogs, true
	case TransportUpgrade:
		return StreamUpgrade, true
	default:
		return "", false
	}
}

func streamTerminal(ctx context.Context, operationErr error) string {
	cause := context.Cause(ctx)
	switch {
	case errors.Is(cause, ErrStreamAuthorizationLost):
		return "authorization_revoked"
	case errors.Is(cause, ErrStreamCredentialExpired):
		return "credential_expired"
	case errors.Is(cause, ErrStreamLifetimeExpired):
		return "max_duration"
	case errors.Is(cause, ErrStreamIdle):
		return "idle_timeout"
	case errors.Is(cause, context.Canceled):
		return "client_canceled"
	case operationErr != nil:
		return "failed"
	default:
		return "completed"
	}
}

func requiresMutation(info RequestInfo) bool {
	switch info.Verb {
	case "create", "update", "patch":
		return info.Subresource != "scale" && info.Subresource != "eviction"
	default:
		return false
	}
}

func clientDryRun(query url.Values) bool {
	for _, value := range query["dryRun"] {
		if strings.EqualFold(strings.TrimSpace(value), "All") {
			return true
		}
	}
	return false
}

func writeDryRunResult(writer http.ResponseWriter, request *http.Request, result DryRunValidation) error {
	copyResponseHeaders(writer.Header(), result.Header)
	writer.Header().Set("Cache-Control", "no-store")
	status := result.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	writer.WriteHeader(status)
	if request.Method != http.MethodHead {
		_, err := writer.Write(result.ClientBody)
		return err
	}
	return nil
}

func replaceRequestBody(request *http.Request, body []byte) {
	if request.Body != nil {
		_ = request.Body.Close()
	}
	copy := bytes.Clone(body)
	request.Body = io.NopCloser(bytes.NewReader(copy))
	request.ContentLength = int64(len(copy))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(copy)), nil }
}

func boundNonUpgradeRequestBody(request *http.Request, info RequestInfo) error {
	if request == nil || info.IsUpgrade || request.Body == nil || request.Body == http.NoBody {
		return nil
	}
	body, err := readLimited(request.Body, DefaultMaxRequestBodyBytes)
	if err != nil {
		return err
	}
	replaceRequestBody(request, body)
	return nil
}

func authenticationError(err error) error {
	var status *StatusError
	if errors.As(err, &status) {
		return status
	}
	return Unauthorized(err)
}

func dependencyError(err error, fallbackCode string) error {
	var status *StatusError
	if errors.As(err, &status) {
		return status
	}
	return Unavailable(fallbackCode, err)
}

func forceSafeProtocolQuery(request *http.Request, info RequestInfo) {
	if request == nil || info.Subresource != "log" {
		return
	}
	query := request.URL.Query()
	query.Set("insecureSkipTLSVerifyBackend", "false")
	request.URL.RawQuery = query.Encode()
}

func kubePrefix(bindingID string) string {
	return BindingPathPrefix(bindingID)
}

func BindingPathPrefix(bindingID string) string {
	return "/kube/v1/bindings/" + url.PathEscape(strings.TrimSpace(bindingID))
}

// ExtractEscapedKubePath keeps the raw escaped path intact for RequestInfo
// validation. Router wildcard parameters are commonly decoded and must not be
// used as the authorization input.
func ExtractEscapedKubePath(request *http.Request, bindingID string) (string, error) {
	if request == nil || strings.TrimSpace(bindingID) == "" {
		return "", BadRequest(CodeBadRequest, fmt.Errorf("binding route is invalid"))
	}
	escaped := request.URL.EscapedPath()
	prefix := BindingPathPrefix(bindingID)
	if escaped == prefix || escaped == prefix+"/" {
		return "/", nil
	}
	if !strings.HasPrefix(escaped, prefix+"/") {
		return "", BadRequest(CodeBadRequest, fmt.Errorf("request path is outside the binding route"))
	}
	return strings.TrimPrefix(escaped, prefix), nil
}

type statusResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (writer *statusResponseWriter) WriteHeader(status int) {
	if writer.wroteHeader {
		return
	}
	writer.status, writer.wroteHeader = status, true
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *statusResponseWriter) Write(data []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(data)
}

func (writer *statusResponseWriter) Flush() {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (writer *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := writer.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	writer.status, writer.wroteHeader = http.StatusSwitchingProtocols, true
	return hijacker.Hijack()
}

func (writer *statusResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	if readerFrom, ok := writer.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(reader)
	}
	return io.Copy(writer.ResponseWriter, reader)
}

func (writer *statusResponseWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }
