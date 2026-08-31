package kubeproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/kubepolicy"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

const DefaultMaxMetricsResponseBytes int64 = 16 << 20

type PodMetadataReader interface {
	ReadPodMetadata(context.Context, AccessContext, string) (metav1.Object, error)
}

type MetricsProxy struct {
	PodReader    PodMetadataReader
	MaxBodyBytes int64
	Telemetry    *Telemetry
}

func (proxy MetricsProxy) Serve(writer http.ResponseWriter, request *http.Request, access AccessContext, info RequestInfo, upstream Upstream, kubePath string) error {
	if access.ApplicationID == "" {
		return (HTTPProxy{Telemetry: proxy.Telemetry}).Serve(writer, request, upstream, kubePath, info, DiscoveryTransformer{KubePrefix: kubePrefix(access.BindingID), RequestPath: kubePath})
	}
	if info.Name != "" {
		if proxy.PodReader == nil {
			return Unavailable(CodeMetricsSelectorUnavailable, fmt.Errorf("Pod metadata reader is unavailable"))
		}
		pod, err := proxy.PodReader.ReadPodMetadata(request.Context(), access, info.Name)
		if err != nil {
			if apierrors.IsNotFound(err) || AsStatusError(err).HTTPStatus == http.StatusNotFound {
				return NotFound(schema.GroupVersionResource{Version: "v1", Resource: "pods"}, info.Name)
			}
			return Unavailable(CodeMetricsSelectorUnavailable, err)
		}
		if !metadataMatchesAccess(pod, access) {
			return NotFound(schema.GroupVersionResource{Version: "v1", Resource: "pods"}, info.Name)
		}
	}
	upstreamRequest, err := BuildUpstreamRequest(request, upstream, kubePath)
	if err != nil {
		return err
	}
	upstreamRequest.Method = http.MethodGet
	upstreamRequest.Header.Set("Accept", runtime.ContentTypeJSON)
	upstreamRequest.Header.Set("Accept-Encoding", "identity")
	telemetry := proxy.Telemetry
	if telemetry == nil {
		telemetry = NewTelemetry(nil)
	}
	proxyCtx, proxySpan := telemetry.StartInternal(upstreamRequest.Context(), "kubernetes.proxy", trace.SpanKindClient)
	upstreamRequest = upstreamRequest.WithContext(proxyCtx)
	telemetry.InjectUpstream(proxyCtx, upstreamRequest.Header)
	response, err := upstream.Transport.RoundTrip(upstreamRequest)
	if err != nil {
		proxySpan.SetStatus(codes.Error, CodeMetricsSelectorUnavailable)
		proxySpan.End()
		return Unavailable(CodeMetricsSelectorUnavailable, err)
	}
	if response == nil || response.Body == nil {
		proxySpan.SetStatus(codes.Error, CodeMetricsSelectorUnavailable)
		proxySpan.End()
		return Unavailable(CodeMetricsSelectorUnavailable, fmt.Errorf("metrics provider returned an empty response"))
	}
	defer response.Body.Close()
	defer proxySpan.End()
	if response.StatusCode >= http.StatusInternalServerError {
		proxySpan.SetStatus(codes.Error, CodeUnavailable)
	} else {
		proxySpan.SetStatus(codes.Ok, "")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		copyResponseHeaders(writer.Header(), response.Header)
		if location := writer.Header().Get("Location"); location != "" {
			writer.Header().Set("Location", RewriteLocation(location, kubePrefix(access.BindingID)))
		}
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(response.StatusCode)
		if request.Method != http.MethodHead {
			_, _ = io.Copy(writer, response.Body)
		}
		return nil
	}
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return Unavailable(CodeMetricsSelectorUnavailable, fmt.Errorf("metrics provider returned a compressed validation response"))
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != runtime.ContentTypeJSON {
		return Unavailable(CodeMetricsSelectorUnavailable, fmt.Errorf("metrics provider returned an unsupported Content-Type"))
	}
	limit := proxy.MaxBodyBytes
	if limit <= 0 {
		limit = DefaultMaxMetricsResponseBytes
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return Unavailable(CodeMetricsSelectorUnavailable, fmt.Errorf("metrics provider response cannot be validated within the size limit"))
	}
	object, err := decodeAndValidateMetrics(body, access, info)
	if err != nil {
		return Unavailable(CodeMetricsSelectorUnavailable, err)
	}
	representation, err := NegotiateRepresentation(request.Header.Get("Accept"))
	if err != nil {
		return err
	}
	object, err = metricsRepresentation(object, representation)
	if err != nil {
		return err
	}
	copySafeMetricsMetadata(writer.Header(), response.Header)
	return WriteNegotiatedObject(writer, request, object)
}

func decodeAndValidateMetrics(body []byte, access AccessContext, info RequestInfo) (runtime.Object, error) {
	if info.Name == "" {
		var list metricsv1beta1.PodMetricsList
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, fmt.Errorf("decode PodMetricsList: %w", err)
		}
		if list.Kind != "PodMetricsList" || list.APIVersion != "metrics.k8s.io/v1beta1" {
			return nil, fmt.Errorf("metrics provider returned the wrong kind")
		}
		for index := range list.Items {
			if !metadataMatchesAccess(&list.Items[index].ObjectMeta, access) {
				return nil, fmt.Errorf("PodMetricsList entry is outside the application binding")
			}
		}
		return &list, nil
	}
	var metric metricsv1beta1.PodMetrics
	if err := json.Unmarshal(body, &metric); err != nil {
		return nil, fmt.Errorf("decode PodMetrics: %w", err)
	}
	if metric.Kind != "PodMetrics" || metric.APIVersion != "metrics.k8s.io/v1beta1" || metric.Name != info.Name || !metadataMatchesAccess(&metric.ObjectMeta, access) {
		return nil, fmt.Errorf("PodMetrics entry is outside the application binding")
	}
	return &metric, nil
}

func metadataMatchesAccess(object metav1.Object, access AccessContext) bool {
	if object == nil || object.GetNamespace() != access.Namespace {
		return false
	}
	labels := object.GetLabels()
	source := labels[kubepolicy.ManagementSourceLabel]
	return labels[kubepolicy.ManagedByLabel] == kubepolicy.ManagedByValue && labels[kubepolicy.ProjectIDLabel] == access.ProjectID && labels[kubepolicy.ApplicationIDLabel] == access.ApplicationID &&
		(source == string(kubepolicy.ManagementSourcePlatform) || source == string(kubepolicy.ManagementSourceKubectl))
}

func metricsRepresentation(object runtime.Object, representation Representation) (runtime.Object, error) {
	switch representation.As {
	case "":
		return object, nil
	case "Table":
		return metricsTable(object)
	case "PartialObjectMetadata", "PartialObjectMetadataList":
		return metricsPartialMetadata(object)
	default:
		return nil, NotAcceptable(fmt.Errorf("unsupported metrics representation %q", representation.As))
	}
}

func metricsTable(object runtime.Object) (runtime.Object, error) {
	table := &metav1.Table{
		TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "Table"},
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name", Priority: 0},
			{Name: "CPU(cores)", Type: "string", Priority: 0},
			{Name: "Memory(bytes)", Type: "string", Priority: 0},
		},
	}
	appendMetric := func(metric *metricsv1beta1.PodMetrics) {
		cpu, memory := aggregateMetric(metric)
		table.Rows = append(table.Rows, metav1.TableRow{Cells: []any{metric.Name, cpu, memory}, Object: runtime.RawExtension{Object: metric}})
	}
	switch value := object.(type) {
	case *metricsv1beta1.PodMetrics:
		appendMetric(value)
	case *metricsv1beta1.PodMetricsList:
		for index := range value.Items {
			appendMetric(&value.Items[index])
		}
	default:
		return nil, NotAcceptable(fmt.Errorf("PodMetrics Table conversion is unavailable"))
	}
	return table, nil
}

func aggregateMetric(metric *metricsv1beta1.PodMetrics) (string, string) {
	cpu, memory := int64(0), int64(0)
	for _, container := range metric.Containers {
		cpu += container.Usage.Cpu().MilliValue()
		memory += container.Usage.Memory().Value()
	}
	return fmt.Sprintf("%dm", cpu), fmt.Sprintf("%d", memory)
}

func metricsPartialMetadata(object runtime.Object) (runtime.Object, error) {
	partial := func(metadata metav1.ObjectMeta) metav1.PartialObjectMetadata {
		return metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "PartialObjectMetadata"}, ObjectMeta: *metadata.DeepCopy()}
	}
	switch value := object.(type) {
	case *metricsv1beta1.PodMetrics:
		result := partial(value.ObjectMeta)
		return &result, nil
	case *metricsv1beta1.PodMetricsList:
		result := &metav1.PartialObjectMetadataList{TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "PartialObjectMetadataList"}, ListMeta: value.ListMeta}
		for index := range value.Items {
			result.Items = append(result.Items, partial(value.Items[index].ObjectMeta))
		}
		return result, nil
	default:
		return nil, NotAcceptable(fmt.Errorf("PodMetrics PartialObjectMetadata conversion is unavailable"))
	}
}

func copySafeMetricsMetadata(target, source http.Header) {
	for _, name := range []string{"Warning", "Retry-After", "Audit-ID"} {
		for _, value := range source.Values(name) {
			target.Add(name, value)
		}
	}
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Encoding", "ETag", "Vary"} {
		target.Del(name)
	}
}
