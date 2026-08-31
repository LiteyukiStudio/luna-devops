package kubeproxy

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	cborserializer "k8s.io/apimachinery/pkg/runtime/serializer/cbor"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	kubeSchemeOnce sync.Once
	kubeScheme     *runtime.Scheme
	kubeCodecs     serializer.CodecFactory
)

func protocolScheme() (*runtime.Scheme, serializer.CodecFactory) {
	kubeSchemeOnce.Do(func() {
		kubeScheme = runtime.NewScheme()
		_ = metav1.AddMetaToScheme(kubeScheme)
		_ = corev1.AddToScheme(kubeScheme)
		_ = appsv1.AddToScheme(kubeScheme)
		_ = autoscalingv2.AddToScheme(kubeScheme)
		_ = batchv1.AddToScheme(kubeScheme)
		_ = networkingv1.AddToScheme(kubeScheme)
		_ = policyv1.AddToScheme(kubeScheme)
		_ = storagev1.AddToScheme(kubeScheme)
		_ = authorizationv1.AddToScheme(kubeScheme)
		_ = authenticationv1.AddToScheme(kubeScheme)
		_ = metricsv1beta1.AddToScheme(kubeScheme)
		_ = gatewayv1.Install(kubeScheme)
		kubeCodecs = serializer.NewCodecFactory(kubeScheme)
	})
	return kubeScheme, kubeCodecs
}

type Representation struct {
	MediaType  string
	As         string
	Group      string
	Version    string
	Parameters map[string]string
}

func NegotiateRepresentation(accept string) (Representation, error) {
	if strings.TrimSpace(accept) == "" {
		return Representation{MediaType: runtime.ContentTypeJSON}, nil
	}
	bestQuality := -1.0
	best := Representation{}
	for _, raw := range strings.Split(accept, ",") {
		mediaType, parameters, err := mime.ParseMediaType(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		quality := 1.0
		if rawQuality, ok := parameters["q"]; ok {
			quality, err = strconv.ParseFloat(rawQuality, 64)
			if err != nil || quality <= 0 || quality > 1 {
				continue
			}
		}
		delete(parameters, "q")
		mediaType = strings.ToLower(mediaType)
		if mediaType == "*/*" {
			mediaType = runtime.ContentTypeJSON
		}
		switch mediaType {
		case runtime.ContentTypeJSON, runtime.ContentTypeYAML, runtime.ContentTypeProtobuf, runtime.ContentTypeCBOR:
			if quality > bestQuality {
				bestQuality = quality
				best = Representation{MediaType: mediaType, As: parameters["as"], Group: parameters["g"], Version: parameters["v"], Parameters: parameters}
			}
		}
	}
	if bestQuality < 0 {
		return Representation{}, NotAcceptable(fmt.Errorf("no supported media type in Accept"))
	}
	if best.As != "" && (best.Group != "meta.k8s.io" || best.Version != "v1") {
		return Representation{}, NotAcceptable(fmt.Errorf("unsupported Kubernetes meta representation version"))
	}
	return best, nil
}

func WriteNegotiatedObject(writer http.ResponseWriter, request *http.Request, object runtime.Object) error {
	data, contentType, err := EncodeNegotiatedObject(request.Header.Get("Accept"), object)
	if err != nil {
		return err
	}
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Vary", "Accept")
	writer.Header().Del("Content-Encoding")
	writer.Header().Del("ETag")
	writer.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		if _, err := writer.Write(data); err != nil {
			return Unavailable(CodeUnavailable, err)
		}
	}
	return nil
}

func EncodeNegotiatedObject(accept string, object runtime.Object) ([]byte, string, error) {
	representation, err := NegotiateRepresentation(accept)
	if err != nil {
		return nil, "", err
	}
	scheme, codecs := protocolScheme()
	var encoder runtime.Encoder
	if representation.MediaType == runtime.ContentTypeCBOR {
		encoder = cborserializer.NewSerializer(scheme, scheme)
	} else {
		for _, info := range codecs.SupportedMediaTypes() {
			if info.MediaType == representation.MediaType {
				encoder = info.Serializer
				break
			}
		}
	}
	if encoder == nil {
		return nil, "", NotAcceptable(fmt.Errorf("serializer is unavailable"))
	}
	data, err := runtime.Encode(encoder, object)
	if err != nil {
		return nil, "", NotAcceptable(fmt.Errorf("encode Kubernetes representation: %w", err))
	}
	return data, representation.MediaType, nil
}

type LocalResourceSource interface {
	StorageClasses(context.Context, AccessContext) ([]storagev1.StorageClass, error)
}

type LocalResourceHandler struct {
	Source LocalResourceSource
}

func (handler LocalResourceHandler) Handles(info RequestInfo) bool {
	return info.IsResourceRequest && (info.APIGroup == "" && info.APIVersion == "v1" && info.Resource == "namespaces" ||
		info.APIGroup == "storage.k8s.io" && info.APIVersion == "v1" && info.Resource == "storageclasses")
}

func (handler LocalResourceHandler) Serve(ctx context.Context, writer http.ResponseWriter, request *http.Request, access AccessContext, info RequestInfo) error {
	if !handler.Handles(info) {
		return NotFound(info.GVR(), info.Name)
	}
	representation, err := NegotiateRepresentation(request.Header.Get("Accept"))
	if err != nil {
		return err
	}
	var object runtime.Object
	if info.Resource == "namespaces" {
		object, err = localNamespaceObject(access, info)
	} else {
		object, err = handler.localStorageClassObject(ctx, access, info)
	}
	if err != nil {
		return err
	}
	object, err = projectRepresentation(object, representation)
	if err != nil {
		return err
	}
	return WriteNegotiatedObject(writer, request, object)
}

func localNamespaceObject(access AccessContext, info RequestInfo) (runtime.Object, error) {
	namespace := corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: access.Namespace, Labels: map[string]string{
			"app.kubernetes.io/managed-by": "luna-devops", "luna.devops/project-id": access.ProjectID,
		}},
	}
	if info.Name != "" {
		if info.Name != access.Namespace {
			return nil, NotFound(info.GVR(), info.Name)
		}
		return &namespace, nil
	}
	return &corev1.NamespaceList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "NamespaceList"}, Items: []corev1.Namespace{namespace}}, nil
}

func (handler LocalResourceHandler) localStorageClassObject(ctx context.Context, access AccessContext, info RequestInfo) (runtime.Object, error) {
	if handler.Source == nil {
		return nil, Unavailable(CodeUnavailable, fmt.Errorf("storage class source is unavailable"))
	}
	items, err := handler.Source.StorageClasses(ctx, access)
	if err != nil {
		return nil, Unavailable(CodeUnavailable, err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	if info.Name != "" {
		for index := range items {
			if items[index].Name == info.Name {
				items[index].TypeMeta = metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClass"}
				return &items[index], nil
			}
		}
		return nil, NotFound(info.GVR(), info.Name)
	}
	return &storagev1.StorageClassList{TypeMeta: metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClassList"}, Items: items}, nil
}

func projectRepresentation(object runtime.Object, representation Representation) (runtime.Object, error) {
	switch representation.As {
	case "":
		return object, nil
	case "Table":
		return objectTable(object)
	case "PartialObjectMetadata", "PartialObjectMetadataList":
		return partialMetadataObject(object)
	default:
		return nil, NotAcceptable(fmt.Errorf("unsupported Kubernetes meta representation %q", representation.As))
	}
}

func objectTable(object runtime.Object) (runtime.Object, error) {
	table := &metav1.Table{
		TypeMeta:          metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "Table"},
		ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name", Type: "string", Format: "name", Priority: 0}, {Name: "Age", Type: "string", Priority: 0}},
	}
	appendRow := func(metadata metav1.Object, raw runtime.RawExtension) {
		table.Rows = append(table.Rows, metav1.TableRow{Cells: []any{metadata.GetName(), "<unknown>"}, Object: raw})
	}
	switch value := object.(type) {
	case *corev1.Namespace:
		appendRow(&value.ObjectMeta, runtime.RawExtension{Object: value})
	case *corev1.NamespaceList:
		for index := range value.Items {
			appendRow(&value.Items[index].ObjectMeta, runtime.RawExtension{Object: &value.Items[index]})
		}
	case *storagev1.StorageClass:
		appendRow(&value.ObjectMeta, runtime.RawExtension{Object: value})
	case *storagev1.StorageClassList:
		for index := range value.Items {
			appendRow(&value.Items[index].ObjectMeta, runtime.RawExtension{Object: &value.Items[index]})
		}
	default:
		return nil, NotAcceptable(fmt.Errorf("Table conversion is unavailable"))
	}
	return table, nil
}

func partialMetadataObject(object runtime.Object) (runtime.Object, error) {
	partial := func(metadata metav1.Object) metav1.PartialObjectMetadata {
		return metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "PartialObjectMetadata"}, ObjectMeta: *metadata.(*metav1.ObjectMeta).DeepCopy()}
	}
	switch value := object.(type) {
	case *corev1.Namespace:
		result := partial(&value.ObjectMeta)
		return &result, nil
	case *storagev1.StorageClass:
		result := partial(&value.ObjectMeta)
		return &result, nil
	case *corev1.NamespaceList:
		result := &metav1.PartialObjectMetadataList{TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "PartialObjectMetadataList"}}
		for index := range value.Items {
			result.Items = append(result.Items, partial(&value.Items[index].ObjectMeta))
		}
		return result, nil
	case *storagev1.StorageClassList:
		result := &metav1.PartialObjectMetadataList{TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "PartialObjectMetadataList"}}
		for index := range value.Items {
			result.Items = append(result.Items, partial(&value.Items[index].ObjectMeta))
		}
		return result, nil
	default:
		return nil, NotAcceptable(fmt.Errorf("PartialObjectMetadata conversion is unavailable"))
	}
}

func ResourceGroupVersion(info RequestInfo) schema.GroupVersion {
	return schema.GroupVersion{Group: info.APIGroup, Version: info.APIVersion}
}
