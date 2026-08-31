package kubeproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/kubepolicy"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cborserializer "k8s.io/apimachinery/pkg/runtime/serializer/cbor"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"
)

const DefaultMaxRequestBodyBytes int64 = 16 << 20

type ObjectValidator interface {
	Validate(context.Context, kubepolicy.PolicyContext, *unstructured.Unstructured) field.ErrorList
}

type MutationContext struct {
	Access                 AccessContext
	Info                   RequestInfo
	ExistingLabels         map[string]string
	ReferenceResolver      kubepolicy.ReferenceResolver
	TrustedServiceAccounts map[string]struct{}
	AllowedDomains         []string
	AllowedIngressClasses  map[string]struct{}
	AllowedGatewayParents  map[string]struct{}
	ServiceAccountOrigin   kubepolicy.ServiceAccountOrigin
}

type MutationResult struct {
	Body          []byte
	ContentType   string
	PolicyContext kubepolicy.PolicyContext
}

type Mutator struct {
	MaxBodyBytes int64
	Validator    ObjectValidator
}

func NewMutator() Mutator {
	validator := kubepolicy.NewValidator()
	return Mutator{MaxBodyBytes: DefaultMaxRequestBodyBytes, Validator: validator}
}

func (mutator Mutator) Prepare(ctx context.Context, input MutationContext, contentType string, body io.Reader) (MutationResult, error) {
	_ = ctx
	limit := mutator.MaxBodyBytes
	if limit <= 0 {
		limit = DefaultMaxRequestBodyBytes
	}
	data, err := readLimited(body, limit)
	if err != nil {
		return MutationResult{}, err
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return MutationResult{}, BadRequest(CodeBadRequest, fmt.Errorf("invalid Content-Type"))
	}
	mediaType = strings.ToLower(mediaType)
	policy := input.Access.PolicyContext()
	policy.Resolver = input.ReferenceResolver
	policy.TrustedServiceAccounts = input.TrustedServiceAccounts
	policy.AllowedDomains = append([]string(nil), input.AllowedDomains...)
	policy.AllowedIngressClasses = cloneSet(input.AllowedIngressClasses)
	policy.AllowedGatewayParents = cloneSet(input.AllowedGatewayParents)
	policy.ServiceAccountOrigin = input.ServiceAccountOrigin
	expectedLabels := expectedOwnershipLabels(input)
	policy.ExpectedOwnershipLabels = cloneStringMap(expectedLabels)
	policy.ExpectedLifecycleLabels = expectedLifecycleLabels(input.ExistingLabels)
	policy.ProtectLifecycleLabels = true
	policy.ManagementSource = kubepolicy.ManagementSource(expectedLabels[kubepolicy.ManagementSourceLabel])

	if mediaType == "application/json-patch+json" {
		if err := validateJSONPatch(data); err != nil {
			return MutationResult{}, err
		}
		if policy.ServiceAccountOrigin == "" {
			policy.ServiceAccountOrigin = jsonPatchServiceAccountOrigin(data)
		}
		return MutationResult{Body: data, ContentType: mediaType, PolicyContext: policy}, nil
	}
	if !objectMediaType(mediaType) {
		return MutationResult{}, BadRequest(CodeBadRequest, fmt.Errorf("unsupported Kubernetes object Content-Type"))
	}
	object, err := decodeMutationObject(mediaType, data)
	if err != nil {
		return MutationResult{}, err
	}
	if len(object) == 0 {
		return MutationResult{}, BadRequest(CodeBadRequest, fmt.Errorf("empty Kubernetes object"))
	}
	if mediaType == "application/merge-patch+json" || mediaType == "application/strategic-merge-patch+json" {
		if err := validatePatchOwnership(object, expectedOwnershipLabels(input), expectedLifecycleLabels(input.ExistingLabels)); err != nil {
			return MutationResult{}, err
		}
		if policy.ServiceAccountOrigin == "" {
			policy.ServiceAccountOrigin = mergePatchServiceAccountOrigin(object)
		}
		return MutationResult{Body: data, ContentType: mediaType, PolicyContext: policy}, nil
	}
	if err := mutateObject(input, &policy, object); err != nil {
		return MutationResult{}, err
	}
	encoded, err := encodeMutationObject(mediaType, object)
	if err != nil {
		return MutationResult{}, err
	}
	return MutationResult{Body: encoded, ContentType: mediaType, PolicyContext: policy}, nil
}

func (mutator Mutator) ValidateFinal(ctx context.Context, policy kubepolicy.PolicyContext, body []byte) error {
	if mutator.Validator == nil {
		return Unavailable(CodeUnavailable, fmt.Errorf("workload policy validator is unavailable"))
	}
	jsonBody := body
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] != '{' {
		var err error
		jsonBody, err = yaml.YAMLToJSON(body)
		if err != nil {
			return BadRequest(CodeBadRequest, fmt.Errorf("dry-run response is not a Kubernetes object"))
		}
	}
	object := &unstructured.Unstructured{}
	if err := json.Unmarshal(jsonBody, &object.Object); err != nil {
		return BadRequest(CodeBadRequest, fmt.Errorf("dry-run response is not valid JSON"))
	}
	errors := kubepolicy.ValidateObjectOwnership(policy, object)
	errors = append(errors, mutator.Validator.Validate(ctx, policy, object)...)
	if len(errors) > 0 {
		return Invalid(errors.ToAggregate())
	}
	return nil
}

func mutateObject(input MutationContext, policy *kubepolicy.PolicyContext, object map[string]any) error {
	kind, _ := object["kind"].(string)
	metadata := ensureMap(object, "metadata")
	if namespace, exists := metadata["namespace"].(string); exists && strings.TrimSpace(namespace) != "" && namespace != input.Access.Namespace {
		return Invalid(fmt.Errorf("metadata.namespace must match the project namespace"))
	}
	if input.Info.Namespace != "" {
		metadata["namespace"] = input.Access.Namespace
	}
	expected := expectedOwnershipLabels(input)
	lifecycle := expectedLifecycleLabels(input.ExistingLabels)
	if err := injectMetadataLabels(metadata, expected, lifecycle); err != nil {
		return err
	}

	switch kind {
	case "Deployment", "ReplicaSet":
		if err := injectTemplate(object, expected, lifecycle, []string{"spec", "template"}); err != nil {
			return err
		}
		if err := injectStringMap(object, []string{"spec", "selector", "matchLabels"}, selectionLabels(input)); err != nil {
			return err
		}
		if err := securePodSpec(object, []string{"spec", "template", "spec"}, policy, input.Info.Verb); err != nil {
			return err
		}
	case "Job":
		if err := injectTemplate(object, expected, lifecycle, []string{"spec", "template"}); err != nil {
			return err
		}
		if err := securePodSpec(object, []string{"spec", "template", "spec"}, policy, input.Info.Verb); err != nil {
			return err
		}
	case "StatefulSet":
		if err := injectTemplate(object, expected, lifecycle, []string{"spec", "template"}); err != nil {
			return err
		}
		if err := securePodSpec(object, []string{"spec", "template", "spec"}, policy, input.Info.Verb); err != nil {
			return err
		}
		if err := injectStringMap(object, []string{"spec", "selector", "matchLabels"}, selectionLabels(input)); err != nil {
			return err
		}
		claims, _, _ := unstructured.NestedSlice(object, "spec", "volumeClaimTemplates")
		for index, raw := range claims {
			claim, ok := raw.(map[string]any)
			if !ok {
				return Invalid(fmt.Errorf("spec.volumeClaimTemplates[%d] must be an object", index))
			}
			if err := injectMetadataLabels(ensureMap(claim, "metadata"), expected, lifecycle); err != nil {
				return err
			}
			claims[index] = claim
		}
		_ = unstructured.SetNestedSlice(object, claims, "spec", "volumeClaimTemplates")
	case "CronJob":
		jobTemplate, found, _ := unstructured.NestedMap(object, "spec", "jobTemplate")
		if !found {
			return Invalid(fmt.Errorf("spec.jobTemplate is required"))
		}
		if err := injectMetadataLabels(ensureMap(jobTemplate, "metadata"), expected, lifecycle); err != nil {
			return err
		}
		if err := injectTemplate(jobTemplate, expected, lifecycle, []string{"spec", "template"}); err != nil {
			return err
		}
		if err := securePodSpec(jobTemplate, []string{"spec", "template", "spec"}, policy, input.Info.Verb); err != nil {
			return err
		}
		_ = unstructured.SetNestedMap(object, jobTemplate, "spec", "jobTemplate")
	case "Pod":
		if err := securePodSpec(object, []string{"spec"}, policy, input.Info.Verb); err != nil {
			return err
		}
	case "Service":
		if err := injectStringMap(object, []string{"spec", "selector"}, selectionLabels(input)); err != nil {
			return err
		}
	case "PodDisruptionBudget":
		if err := injectStringMap(object, []string{"spec", "selector", "matchLabels"}, selectionLabels(input)); err != nil {
			return err
		}
	case "NetworkPolicy":
		if err := injectStringMap(object, []string{"spec", "podSelector", "matchLabels"}, selectionLabels(input)); err != nil {
			return err
		}
	}
	return nil
}

func expectedOwnershipLabels(input MutationContext) map[string]string {
	source := string(kubepolicy.ManagementSourceKubectl)
	if value := input.ExistingLabels[kubepolicy.ManagementSourceLabel]; value == string(kubepolicy.ManagementSourcePlatform) || value == string(kubepolicy.ManagementSourceKubectl) {
		source = value
	}
	labels := map[string]string{
		kubepolicy.ManagedByLabel: kubepolicy.ManagedByValue, kubepolicy.ProjectIDLabel: input.Access.ProjectID,
		kubepolicy.ManagementSourceLabel: source,
	}
	if input.Access.ApplicationID != "" {
		labels[kubepolicy.ApplicationIDLabel] = input.Access.ApplicationID
	} else if value := input.ExistingLabels[kubepolicy.ApplicationIDLabel]; value != "" {
		labels[kubepolicy.ApplicationIDLabel] = value
	}
	return labels
}

func expectedLifecycleLabels(existing map[string]string) map[string]string {
	expected := map[string]string{}
	for _, key := range kubepolicy.ProtectedLifecycleLabelKeys() {
		if value, exists := existing[key]; exists {
			expected[key] = value
		}
	}
	return expected
}

func injectMetadataLabels(metadata map[string]any, expected, lifecycle map[string]string) error {
	if value, exists := metadata["labels"]; exists && value == nil {
		return Invalid(fmt.Errorf("ownership labels cannot be removed"))
	}
	labels := ensureMap(metadata, "labels")
	if _, expectedApplication := expected[kubepolicy.ApplicationIDLabel]; !expectedApplication {
		if _, exists := labels[kubepolicy.ApplicationIDLabel]; exists {
			return Invalid(fmt.Errorf("reserved ownership label %s cannot be set by a project binding", kubepolicy.ApplicationIDLabel))
		}
	}
	for key, expectedValue := range expected {
		if actual, exists := labels[key]; exists && actual != nil && fmt.Sprint(actual) != expectedValue {
			return Invalid(fmt.Errorf("reserved ownership label %s conflicts with the binding", key))
		}
		labels[key] = expectedValue
	}
	for _, key := range kubepolicy.ProtectedLifecycleLabelKeys() {
		expectedValue, preserve := lifecycle[key]
		actual, exists := labels[key]
		if !preserve {
			if exists {
				return Invalid(fmt.Errorf("reserved lifecycle label %s cannot be assigned by kubectl", key))
			}
			continue
		}
		if exists && (actual == nil || fmt.Sprint(actual) != expectedValue) {
			return Invalid(fmt.Errorf("reserved lifecycle label %s cannot be changed", key))
		}
		labels[key] = expectedValue
	}
	return nil
}

func injectTemplate(object map[string]any, expected, lifecycle map[string]string, path []string) error {
	template, found, err := unstructured.NestedMap(object, path...)
	if err != nil || !found {
		return Invalid(fmt.Errorf("%s is required", strings.Join(path, ".")))
	}
	if err := injectMetadataLabels(ensureMap(template, "metadata"), expected, lifecycle); err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(object, template, path...); err != nil {
		return Invalid(err)
	}
	return nil
}

func securePodSpec(object map[string]any, path []string, policy *kubepolicy.PolicyContext, verb string) error {
	spec, found, err := unstructured.NestedMap(object, path...)
	if err != nil || !found {
		return Invalid(fmt.Errorf("%s is required", strings.Join(path, ".")))
	}
	if value, exists := spec["automountServiceAccountToken"]; exists {
		boolean, valid := value.(bool)
		if !valid || boolean {
			return Invalid(fmt.Errorf("%s.automountServiceAccountToken must be false", strings.Join(path, ".")))
		}
	} else {
		spec["automountServiceAccountToken"] = false
	}
	for _, fieldName := range []string{"initContainers", "containers", "ephemeralContainers"} {
		if err := defaultContainerSecurity(spec, fieldName, append(append([]string(nil), path...), fieldName)); err != nil {
			return err
		}
	}
	if policy.ServiceAccountOrigin == "" {
		name, present := spec["serviceAccountName"].(string)
		switch {
		case verb == "create" && (!present || strings.TrimSpace(name) == ""):
			policy.ServiceAccountOrigin = kubepolicy.ServiceAccountAbsent
		case verb == "create":
			policy.ServiceAccountOrigin = kubepolicy.ServiceAccountExplicit
		default:
			policy.ServiceAccountOrigin = kubepolicy.ServiceAccountUnchanged
		}
	}
	if err := unstructured.SetNestedMap(object, spec, path...); err != nil {
		return Invalid(err)
	}
	return nil
}

func defaultContainerSecurity(spec map[string]any, fieldName string, path []string) error {
	raw, exists := spec[fieldName]
	if !exists {
		return nil
	}
	containers, ok := raw.([]any)
	if !ok {
		return Invalid(fmt.Errorf("%s must be a list", strings.Join(path, ".")))
	}
	for index, rawContainer := range containers {
		container, ok := rawContainer.(map[string]any)
		if !ok {
			return Invalid(fmt.Errorf("%s[%d] must be an object", strings.Join(path, "."), index))
		}
		securityContext := map[string]any{}
		if rawSecurity, found := container["securityContext"]; found && rawSecurity != nil {
			var valid bool
			securityContext, valid = rawSecurity.(map[string]any)
			if !valid {
				return Invalid(fmt.Errorf("%s[%d].securityContext must be an object", strings.Join(path, "."), index))
			}
		}
		if value, found := securityContext["allowPrivilegeEscalation"]; !found || value == nil {
			securityContext["allowPrivilegeEscalation"] = false
		}
		container["securityContext"] = securityContext
		containers[index] = container
	}
	spec[fieldName] = containers
	return nil
}

func injectStringMap(object map[string]any, path []string, expected map[string]string) error {
	current, found, err := unstructured.NestedStringMap(object, path...)
	if err != nil {
		return Invalid(err)
	}
	if !found {
		current = map[string]string{}
	}
	for key, value := range expected {
		if actual, exists := current[key]; exists && actual != value {
			return Invalid(fmt.Errorf("selector label %s conflicts with the binding", key))
		}
		current[key] = value
	}
	if err := unstructured.SetNestedStringMap(object, current, path...); err != nil {
		return Invalid(err)
	}
	return nil
}

func selectionLabels(input MutationContext) map[string]string {
	labels := map[string]string{kubepolicy.ProjectIDLabel: input.Access.ProjectID}
	if input.Access.ApplicationID != "" {
		labels[kubepolicy.ApplicationIDLabel] = input.Access.ApplicationID
	} else if existing := strings.TrimSpace(input.ExistingLabels[kubepolicy.ApplicationIDLabel]); existing != "" {
		labels[kubepolicy.ApplicationIDLabel] = existing
	}
	return labels
}

func validateJSONPatch(data []byte) error {
	var operations []struct {
		Op   string `json:"op"`
		Path string `json:"path"`
		From string `json:"from"`
	}
	if err := json.Unmarshal(data, &operations); err != nil || len(operations) == 0 {
		return BadRequest(CodeBadRequest, fmt.Errorf("invalid JSON Patch"))
	}
	for _, operation := range operations {
		for _, path := range []string{operation.Path, operation.From} {
			decoded := strings.ReplaceAll(strings.ReplaceAll(path, "~1", "/"), "~0", "~")
			if reservedMutationPath(decoded) {
				return Invalid(fmt.Errorf("JSON Patch cannot modify ownership labels"))
			}
		}
	}
	return nil
}

func jsonPatchServiceAccountOrigin(data []byte) kubepolicy.ServiceAccountOrigin {
	var operations []struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(data, &operations) != nil {
		return kubepolicy.ServiceAccountUnchanged
	}
	for _, operation := range operations {
		if strings.HasSuffix(operation.Path, "/serviceAccountName") || strings.HasSuffix(operation.Path, "/serviceAccount") {
			return kubepolicy.ServiceAccountExplicit
		}
	}
	return kubepolicy.ServiceAccountUnchanged
}

func mergePatchServiceAccountOrigin(object map[string]any) kubepolicy.ServiceAccountOrigin {
	found := false
	var walk func(map[string]any)
	walk = func(value map[string]any) {
		if _, ok := value["serviceAccountName"]; ok {
			found = true
		}
		for _, child := range value {
			if nested, ok := child.(map[string]any); ok {
				walk(nested)
			}
		}
	}
	walk(object)
	if found {
		return kubepolicy.ServiceAccountExplicit
	}
	return kubepolicy.ServiceAccountUnchanged
}

func validatePatchOwnership(object map[string]any, expected, lifecycle map[string]string) error {
	var walk func(map[string]any) error
	walk = func(value map[string]any) error {
		if metadata, ok := value["metadata"].(map[string]any); ok {
			if raw, exists := metadata["labels"]; exists {
				labels, valid := raw.(map[string]any)
				if !valid {
					return Invalid(fmt.Errorf("ownership labels cannot be removed"))
				}
				if _, expectedApplication := expected[kubepolicy.ApplicationIDLabel]; !expectedApplication {
					if _, exists := labels[kubepolicy.ApplicationIDLabel]; exists {
						return Invalid(fmt.Errorf("reserved ownership label %s cannot be set by a project binding", kubepolicy.ApplicationIDLabel))
					}
				}
				for key, expectedValue := range expected {
					if actual, exists := labels[key]; exists && (actual == nil || fmt.Sprint(actual) != expectedValue) {
						return Invalid(fmt.Errorf("reserved ownership label %s cannot be changed", key))
					}
				}
				for _, key := range kubepolicy.ProtectedLifecycleLabelKeys() {
					expectedValue, preserve := lifecycle[key]
					actual, touched := labels[key]
					if !touched {
						continue
					}
					if !preserve || actual == nil || fmt.Sprint(actual) != expectedValue {
						return Invalid(fmt.Errorf("reserved lifecycle label %s cannot be changed", key))
					}
				}
			}
		}
		for _, child := range value {
			switch nested := child.(type) {
			case map[string]any:
				if err := walk(nested); err != nil {
					return err
				}
			case []any:
				for _, item := range nested {
					if itemMap, ok := item.(map[string]any); ok {
						if err := walk(itemMap); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	}
	return walk(object)
}

func reservedMutationPath(path string) bool {
	if strings.HasSuffix(path, "/metadata/labels") || path == "/metadata/labels" {
		return true
	}
	keys := []string{kubepolicy.ManagedByLabel, kubepolicy.ProjectIDLabel, kubepolicy.ApplicationIDLabel, kubepolicy.ManagementSourceLabel}
	keys = append(keys, kubepolicy.ProtectedLifecycleLabelKeys()...)
	for _, key := range keys {
		if strings.Contains(path, "/metadata/labels/"+key) {
			return true
		}
	}
	return false
}

func objectMediaType(mediaType string) bool {
	switch mediaType {
	case runtime.ContentTypeJSON, runtime.ContentTypeYAML, runtime.ContentTypeProtobuf, runtime.ContentTypeCBOR,
		"application/apply-patch+yaml", "application/apply-patch+json", "application/merge-patch+json", "application/strategic-merge-patch+json":
		return true
	default:
		return false
	}
}

func decodeMutationObject(mediaType string, data []byte) (map[string]any, error) {
	jsonData := data
	switch mediaType {
	case runtime.ContentTypeYAML, "application/apply-patch+yaml":
		converted, err := yaml.YAMLToJSON(data)
		if err != nil {
			return nil, BadRequest(CodeBadRequest, fmt.Errorf("invalid YAML object"))
		}
		jsonData = converted
	case runtime.ContentTypeProtobuf:
		_, codecs := protocolScheme()
		object, _, err := codecs.UniversalDeserializer().Decode(data, nil, nil)
		if err != nil {
			return nil, BadRequest(CodeBadRequest, fmt.Errorf("invalid Kubernetes Protobuf object"))
		}
		value, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if err != nil {
			return nil, BadRequest(CodeBadRequest, fmt.Errorf("decode Kubernetes Protobuf object"))
		}
		return value, nil
	case runtime.ContentTypeCBOR:
		scheme, _ := protocolScheme()
		decoder := cborserializer.NewSerializer(scheme, scheme)
		object, _, err := decoder.Decode(data, nil, nil)
		if err != nil {
			return nil, BadRequest(CodeBadRequest, fmt.Errorf("invalid Kubernetes CBOR object"))
		}
		value, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if err != nil {
			return nil, BadRequest(CodeBadRequest, fmt.Errorf("decode Kubernetes CBOR object"))
		}
		return value, nil
	}
	var object map[string]any
	if err := json.Unmarshal(jsonData, &object); err != nil {
		return nil, BadRequest(CodeBadRequest, fmt.Errorf("invalid JSON object"))
	}
	return object, nil
}

func encodeMutationObject(mediaType string, object map[string]any) ([]byte, error) {
	jsonData, err := json.Marshal(object)
	if err != nil {
		return nil, Unavailable(CodeUnavailable, err)
	}
	switch mediaType {
	case runtime.ContentTypeYAML, "application/apply-patch+yaml":
		encoded, err := yaml.JSONToYAML(jsonData)
		if err != nil {
			return nil, Unavailable(CodeUnavailable, err)
		}
		return encoded, nil
	case runtime.ContentTypeProtobuf, runtime.ContentTypeCBOR:
		typed, err := typedMutationObject(object)
		if err != nil {
			return nil, BadRequest(CodeBadRequest, err)
		}
		scheme, codecs := protocolScheme()
		var encoder runtime.Encoder
		if mediaType == runtime.ContentTypeCBOR {
			encoder = cborserializer.NewSerializer(scheme, scheme)
		} else {
			for _, info := range codecs.SupportedMediaTypes() {
				if info.MediaType == runtime.ContentTypeProtobuf {
					encoder = info.Serializer
					break
				}
			}
		}
		if encoder == nil {
			return nil, Unavailable(CodeUnavailable, fmt.Errorf("Kubernetes mutation serializer is unavailable"))
		}
		encoded, err := runtime.Encode(encoder, typed)
		if err != nil {
			return nil, BadRequest(CodeBadRequest, fmt.Errorf("encode Kubernetes mutation object: %w", err))
		}
		return encoded, nil
	default:
		return jsonData, nil
	}
}

func typedMutationObject(value map[string]any) (runtime.Object, error) {
	apiVersion, _ := value["apiVersion"].(string)
	kind, _ := value["kind"].(string)
	groupVersion, err := schema.ParseGroupVersion(apiVersion)
	if err != nil || kind == "" {
		return nil, fmt.Errorf("Kubernetes object apiVersion and kind are required")
	}
	scheme, _ := protocolScheme()
	object, err := scheme.New(groupVersion.WithKind(kind))
	if err != nil {
		return nil, fmt.Errorf("Kubernetes Protobuf/CBOR write is unavailable for %s %s", apiVersion, kind)
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(value, object); err != nil {
		return nil, fmt.Errorf("convert Kubernetes mutation object: %w", err)
	}
	return object, nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, BadRequest(CodeBadRequest, fmt.Errorf("request body is required"))
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, BadRequest(CodeBadRequest, err)
	}
	if int64(len(data)) > limit {
		return nil, TooLarge(fmt.Errorf("request body exceeds %d bytes", limit))
	}
	return data, nil
}

func ensureMap(parent map[string]any, key string) map[string]any {
	if value, ok := parent[key].(map[string]any); ok {
		return value
	}
	value := map[string]any{}
	parent[key] = value
	return value
}

func cloneSet(input map[string]struct{}) map[string]struct{} {
	if input == nil {
		return nil
	}
	output := make(map[string]struct{}, len(input))
	for value := range input {
		output[value] = struct{}{}
	}
	return output
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
