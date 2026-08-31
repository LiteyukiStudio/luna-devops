package kubeproxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	cborserializer "k8s.io/apimachinery/pkg/runtime/serializer/cbor"
	"sigs.k8s.io/yaml"
)

const maxDiscoveryRewriteBytes int64 = 64 << 20

type DiscoveryTransformer struct {
	KubePrefix  string
	RequestPath string
}

func (transformer DiscoveryTransformer) Transform(_ context.Context, response *http.Response) error {
	if response == nil {
		return Unavailable(CodeUnavailable, fmt.Errorf("upstream response is missing"))
	}
	if location := response.Header.Get("Location"); location != "" {
		response.Header.Set("Location", RewriteLocation(location, transformer.KubePrefix))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil
	}
	openAPIV3 := strings.HasPrefix(transformer.RequestPath, "/openapi/v3")
	rootDiscovery := transformer.RequestPath == "/api" || transformer.RequestPath == "/apis"
	if !openAPIV3 && !rootDiscovery {
		return nil
	}
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil {
		return Unavailable(CodeUnavailable, fmt.Errorf("invalid discovery Content-Type"))
	}
	if openAPIV3 && mediaType != runtime.ContentTypeJSON {
		return nil
	}
	bodyReader := io.Reader(response.Body)
	encoding := strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding")))
	if encoding == "gzip" {
		reader, err := gzip.NewReader(response.Body)
		if err != nil {
			return Unavailable(CodeUnavailable, fmt.Errorf("decode compressed OpenAPI response: %w", err))
		}
		defer reader.Close()
		bodyReader = reader
	} else if encoding != "" && encoding != "identity" {
		return Unavailable(CodeUnavailable, fmt.Errorf("unsupported OpenAPI Content-Encoding"))
	}
	body, err := io.ReadAll(io.LimitReader(bodyReader, maxDiscoveryRewriteBytes+1))
	if err != nil {
		return Unavailable(CodeUnavailable, err)
	}
	if int64(len(body)) > maxDiscoveryRewriteBytes {
		return Unavailable(CodeUnavailable, fmt.Errorf("OpenAPI response is too large to rewrite safely"))
	}
	var rewritten []byte
	if openAPIV3 {
		rewritten, err = RewriteOpenAPIV3(body, transformer.KubePrefix)
	} else {
		rewritten, err = RewriteRootDiscoveryAddresses(body, mediaType)
	}
	if err != nil {
		return Unavailable(CodeUnavailable, err)
	}
	if encoding == "gzip" {
		compressed := &bytes.Buffer{}
		writer := gzip.NewWriter(compressed)
		if _, err := writer.Write(rewritten); err != nil {
			_ = writer.Close()
			return Unavailable(CodeUnavailable, fmt.Errorf("encode compressed OpenAPI response: %w", err))
		}
		if err := writer.Close(); err != nil {
			return Unavailable(CodeUnavailable, fmt.Errorf("finish compressed OpenAPI response: %w", err))
		}
		rewritten = compressed.Bytes()
		response.Header.Set("Content-Encoding", "gzip")
	} else {
		response.Header.Del("Content-Encoding")
	}
	_ = response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(rewritten))
	response.ContentLength = int64(len(rewritten))
	response.Header.Set("Content-Length", fmt.Sprintf("%d", len(rewritten)))
	response.Header.Del("ETag")
	return nil
}

// RewriteRootDiscoveryAddresses removes deprecated upstream address hints from
// /api and /apis. Those hints are not needed by modern clients and would expose
// the private kube-apiserver authority behind the gateway.
func RewriteRootDiscoveryAddresses(input []byte, mediaType string) ([]byte, error) {
	switch mediaType {
	case runtime.ContentTypeJSON:
		var document any
		if err := json.Unmarshal(input, &document); err != nil {
			return nil, fmt.Errorf("decode root discovery response: %w", err)
		}
		clearDiscoveryAddresses(document)
		return json.Marshal(document)
	case runtime.ContentTypeYAML:
		jsonInput, err := yaml.YAMLToJSON(input)
		if err != nil {
			return nil, fmt.Errorf("decode YAML root discovery response: %w", err)
		}
		rewritten, err := RewriteRootDiscoveryAddresses(jsonInput, runtime.ContentTypeJSON)
		if err != nil {
			return nil, err
		}
		return yaml.JSONToYAML(rewritten)
	case runtime.ContentTypeProtobuf, runtime.ContentTypeCBOR:
		object, err := decodeRootDiscoveryObject(input, mediaType)
		if err != nil {
			return nil, err
		}
		switch value := object.(type) {
		case *metav1.APIVersions:
			value.ServerAddressByClientCIDRs = nil
		case *metav1.APIGroupList:
			for index := range value.Groups {
				value.Groups[index].ServerAddressByClientCIDRs = nil
			}
		default:
			return nil, fmt.Errorf("unexpected root discovery object %T", object)
		}
		return encodeRootDiscoveryObject(object, mediaType)
	default:
		return nil, fmt.Errorf("unsupported root discovery Content-Type %q", mediaType)
	}
}

func clearDiscoveryAddresses(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "serverAddressByClientCIDRs" {
				typed[key] = []any{}
				continue
			}
			clearDiscoveryAddresses(child)
		}
	case []any:
		for _, child := range typed {
			clearDiscoveryAddresses(child)
		}
	}
}

func decodeRootDiscoveryObject(data []byte, mediaType string) (runtime.Object, error) {
	scheme, codecs := protocolScheme()
	if mediaType == runtime.ContentTypeCBOR {
		object, _, err := cborserializer.NewSerializer(scheme, scheme).Decode(data, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("decode CBOR root discovery response: %w", err)
		}
		return object, nil
	}
	object, _, err := codecs.UniversalDeserializer().Decode(data, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decode Protobuf root discovery response: %w", err)
	}
	return object, nil
}

func encodeRootDiscoveryObject(object runtime.Object, mediaType string) ([]byte, error) {
	scheme, codecs := protocolScheme()
	var encoder runtime.Encoder
	if mediaType == runtime.ContentTypeCBOR {
		encoder = cborserializer.NewSerializer(scheme, scheme)
	} else {
		for _, info := range codecs.SupportedMediaTypes() {
			if info.MediaType == mediaType {
				encoder = info.Serializer
				break
			}
		}
	}
	if encoder == nil {
		return nil, fmt.Errorf("root discovery serializer is unavailable")
	}
	encoded, err := runtime.Encode(encoder, object)
	if err != nil {
		return nil, fmt.Errorf("encode root discovery response: %w", err)
	}
	return encoded, nil
}

func RewriteOpenAPIV3(input []byte, kubePrefix string) ([]byte, error) {
	var document any
	if err := json.Unmarshal(input, &document); err != nil {
		return nil, fmt.Errorf("decode OpenAPI v3 response: %w", err)
	}
	prefix := normalizeKubePrefix(kubePrefix)
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if key == "serverRelativeURL" {
					if path, ok := child.(string); ok {
						typed[key] = RewriteLocation(path, prefix)
					}
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(document)
	return json.Marshal(document)
}

func RewriteLocation(location, kubePrefix string) string {
	parsed, err := url.Parse(strings.TrimSpace(location))
	if err != nil || parsed.Path == "" {
		return normalizeKubePrefix(kubePrefix) + "/"
	}
	prefix := normalizeKubePrefix(kubePrefix)
	if !strings.HasPrefix(parsed.Path, prefix+"/") && parsed.Path != prefix {
		parsed.Path = prefix + "/" + strings.TrimPrefix(parsed.Path, "/")
		parsed.RawPath = ""
	}
	parsed.Scheme, parsed.Host, parsed.User = "", "", nil
	return parsed.String()
}

func normalizeKubePrefix(value string) string {
	value = "/" + strings.Trim(strings.TrimSpace(value), "/")
	if value == "/" {
		return ""
	}
	return value
}
