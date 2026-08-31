package kubeproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type HTTPDryRunner struct {
	MaxBodyBytes int64
	Telemetry    *Telemetry
}

func (runner HTTPDryRunner) Validate(ctx context.Context, request *http.Request, upstream Upstream, kubePath string, _ RequestInfo) (DryRunValidation, error) {
	upstreamRequest, err := BuildUpstreamRequest(request.WithContext(ctx), upstream, kubePath)
	if err != nil {
		return DryRunValidation{}, err
	}
	query := upstreamRequest.URL.Query()
	query.Set("dryRun", "All")
	upstreamRequest.URL.RawQuery = query.Encode()
	upstreamRequest.Header.Set("Accept", runtime.ContentTypeJSON)
	upstreamRequest.Header.Set("Accept-Encoding", "identity")
	telemetry := runner.Telemetry
	if telemetry == nil {
		telemetry = NewTelemetry(nil)
	}
	proxyCtx, proxySpan := telemetry.StartInternal(upstreamRequest.Context(), "kubernetes.proxy", trace.SpanKindClient)
	upstreamRequest = upstreamRequest.WithContext(proxyCtx)
	telemetry.InjectUpstream(proxyCtx, upstreamRequest.Header)
	response, err := upstream.Transport.RoundTrip(upstreamRequest)
	if err != nil {
		proxySpan.SetStatus(codes.Error, CodeUnavailable)
		proxySpan.End()
		return DryRunValidation{}, Unavailable(CodeUnavailable, err)
	}
	if response == nil || response.Body == nil {
		proxySpan.SetStatus(codes.Error, CodeUnavailable)
		proxySpan.End()
		return DryRunValidation{}, Unavailable(CodeUnavailable, fmt.Errorf("dry-run response is empty"))
	}
	defer response.Body.Close()
	defer proxySpan.End()
	if response.StatusCode >= http.StatusInternalServerError {
		proxySpan.SetStatus(codes.Error, CodeUnavailable)
	} else {
		proxySpan.SetStatus(codes.Ok, "")
	}
	limit := runner.MaxBodyBytes
	if limit <= 0 {
		limit = DefaultMaxRequestBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return DryRunValidation{}, Unavailable(CodeUnavailable, fmt.Errorf("dry-run response exceeds the validation limit"))
	}
	result := DryRunValidation{StatusCode: response.StatusCode, Header: safeDryRunHeaders(response.Header), ClientBody: bytes.Clone(body)}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, nil
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != runtime.ContentTypeJSON || response.Header.Get("Content-Encoding") != "" {
		return DryRunValidation{}, Unavailable(CodeUnavailable, fmt.Errorf("dry-run response is not uncompressed Kubernetes JSON"))
	}
	canonical := &bytes.Buffer{}
	if err := json.Compact(canonical, body); err != nil {
		return DryRunValidation{}, Unavailable(CodeUnavailable, fmt.Errorf("dry-run response is invalid JSON"))
	}
	result.CanonicalJSON = canonical.Bytes()
	object, err := decodeProtocolObject(result.CanonicalJSON)
	if err != nil {
		return DryRunValidation{}, Unavailable(CodeUnavailable, err)
	}
	clientBody, contentType, err := EncodeNegotiatedObject(request.Header.Get("Accept"), object)
	if err != nil {
		return DryRunValidation{}, err
	}
	result.ClientBody = clientBody
	result.Header.Set("Content-Type", contentType)
	result.Header.Set("Content-Length", fmt.Sprintf("%d", len(clientBody)))
	result.Header.Set("Vary", "Accept")
	result.Header.Del("Content-Encoding")
	result.Header.Del("ETag")
	return result, nil
}

func decodeProtocolObject(data []byte) (runtime.Object, error) {
	scheme, codecs := protocolScheme()
	object, _, err := codecs.UniversalDeserializer().Decode(data, nil, nil)
	if err == nil {
		return object, nil
	}
	var typeMeta metav1.TypeMeta
	if json.Unmarshal(data, &typeMeta) != nil || typeMeta.APIVersion == "" || typeMeta.Kind == "" {
		return nil, fmt.Errorf("dry-run object has no valid apiVersion/kind")
	}
	gv, parseErr := schema.ParseGroupVersion(typeMeta.APIVersion)
	if parseErr != nil {
		return nil, fmt.Errorf("dry-run object apiVersion is invalid")
	}
	giv := gv.WithKind(typeMeta.Kind)
	if _, newErr := scheme.New(giv); newErr == nil {
		return nil, fmt.Errorf("decode registered dry-run object: %w", err)
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return nil, fmt.Errorf("decode unstructured dry-run object")
	}
	return &unstructured.Unstructured{Object: value}, nil
}

func safeDryRunHeaders(source http.Header) http.Header {
	header := make(http.Header)
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Encoding", "ETag", "Warning", "Retry-After", "Audit-ID", "Vary"} {
		for _, value := range source.Values(name) {
			header.Add(name, value)
		}
	}
	header.Set("Cache-Control", "no-store")
	header.Del("Set-Cookie")
	return header
}
