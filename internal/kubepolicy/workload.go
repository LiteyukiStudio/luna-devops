package kubepolicy

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

type Validator struct{}

func NewValidator() Validator { return Validator{} }

func (Validator) Validate(ctx context.Context, policy PolicyContext, object *unstructured.Unstructured) field.ErrorList {
	if object == nil {
		return field.ErrorList{field.Required(field.NewPath("object"), "object is required")}
	}
	if strings.TrimSpace(policy.Namespace) == "" || strings.TrimSpace(policy.ProjectID) == "" {
		return field.ErrorList{field.InternalError(field.NewPath("metadata"), fmt.Errorf("policy namespace and project are required"))}
	}
	if namespace := strings.TrimSpace(object.GetNamespace()); namespace != "" && namespace != policy.Namespace {
		return field.ErrorList{field.Invalid(field.NewPath("metadata", "namespace"), namespace, "must match the project namespace")}
	}

	switch object.GetKind() {
	case "Pod":
		var value corev1.Pod
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		return validatePodSpec(ctx, policy, &value.Spec, field.NewPath("spec"))
	case "Deployment":
		var value appsv1.Deployment
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		errors := validateLabelSelector(policy, value.Spec.Selector, field.NewPath("spec", "selector"))
		return append(errors, validatePodSpec(ctx, policy, &value.Spec.Template.Spec, field.NewPath("spec", "template", "spec"))...)
	case "StatefulSet":
		var value appsv1.StatefulSet
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		errors := validateLabelSelector(policy, value.Spec.Selector, field.NewPath("spec", "selector"))
		errors = append(errors, validatePodSpec(ctx, policy, &value.Spec.Template.Spec, field.NewPath("spec", "template", "spec"))...)
		errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "services"}, Name: value.Spec.ServiceName}, field.NewPath("spec", "serviceName"))...)
		for index := range value.Spec.VolumeClaimTemplates {
			claim := &value.Spec.VolumeClaimTemplates[index]
			errors = append(errors, validateReferenceMetadata(policy, &claim.ObjectMeta, field.NewPath("spec", "volumeClaimTemplates").Index(index).Child("metadata"))...)
		}
		return errors
	case "ReplicaSet":
		var value appsv1.ReplicaSet
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		errors := validateLabelSelector(policy, value.Spec.Selector, field.NewPath("spec", "selector"))
		return append(errors, validatePodSpec(ctx, policy, &value.Spec.Template.Spec, field.NewPath("spec", "template", "spec"))...)
	case "Job":
		var value batchv1.Job
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		errors := validateManualJobSelector(value.Spec.ManualSelector, field.NewPath("spec", "manualSelector"))
		return append(errors, validatePodSpec(ctx, policy, &value.Spec.Template.Spec, field.NewPath("spec", "template", "spec"))...)
	case "CronJob":
		var value batchv1.CronJob
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		errors := validateManualJobSelector(value.Spec.JobTemplate.Spec.ManualSelector, field.NewPath("spec", "jobTemplate", "spec", "manualSelector"))
		return append(errors, validatePodSpec(ctx, policy, &value.Spec.JobTemplate.Spec.Template.Spec, field.NewPath("spec", "jobTemplate", "spec", "template", "spec"))...)
	case "Service":
		var value corev1.Service
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		return ValidateService(policy, &value)
	case "HorizontalPodAutoscaler":
		var value autoscalingv2.HorizontalPodAutoscaler
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		return validateScaleTarget(ctx, policy, value.Spec.ScaleTargetRef, field.NewPath("spec", "scaleTargetRef"))
	case "PodDisruptionBudget":
		var value policyv1.PodDisruptionBudget
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		return validateLabelSelector(policy, value.Spec.Selector, field.NewPath("spec", "selector"))
	case "NetworkPolicy":
		var value networkingv1.NetworkPolicy
		if err := fromUnstructured(object, &value); err != nil {
			return conversionError(err)
		}
		return validateLabelSelector(policy, &value.Spec.PodSelector, field.NewPath("spec", "podSelector"))
	case "Ingress", "HTTPRoute", "GRPCRoute":
		return ValidateGateway(ctx, policy, object)
	default:
		return nil
	}
}

func fromUnstructured(input *unstructured.Unstructured, output any) error {
	return runtime.DefaultUnstructuredConverter.FromUnstructured(input.Object, output)
}

func conversionError(err error) field.ErrorList {
	return field.ErrorList{field.Invalid(field.NewPath("object"), "", "object does not match its Kubernetes schema: "+err.Error())}
}

func validatePodSpec(ctx context.Context, policy PolicyContext, spec *corev1.PodSpec, path *field.Path) field.ErrorList {
	if spec == nil {
		return field.ErrorList{field.Required(path, "pod spec is required")}
	}
	errors := field.ErrorList{}
	if spec.HostNetwork {
		errors = append(errors, field.Forbidden(path.Child("hostNetwork"), "host networking is not allowed"))
	}
	if spec.HostPID {
		errors = append(errors, field.Forbidden(path.Child("hostPID"), "host PID namespace is not allowed"))
	}
	if spec.HostIPC {
		errors = append(errors, field.Forbidden(path.Child("hostIPC"), "host IPC namespace is not allowed"))
	}
	if spec.NodeName != "" {
		errors = append(errors, field.Forbidden(path.Child("nodeName"), "direct node placement is not allowed"))
	}
	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		errors = append(errors, field.Invalid(path.Child("automountServiceAccountToken"), spec.AutomountServiceAccountToken, "must be explicitly false"))
	}
	errors = append(errors, validateServiceAccount(ctx, policy, spec.ServiceAccountName, path.Child("serviceAccountName"))...)

	for index := range spec.InitContainers {
		errors = append(errors, validateContainer(&spec.InitContainers[index], path.Child("initContainers").Index(index))...)
		errors = append(errors, validateContainerReferences(ctx, policy, &spec.InitContainers[index], path.Child("initContainers").Index(index))...)
	}
	for index := range spec.Containers {
		errors = append(errors, validateContainer(&spec.Containers[index], path.Child("containers").Index(index))...)
		errors = append(errors, validateContainerReferences(ctx, policy, &spec.Containers[index], path.Child("containers").Index(index))...)
	}
	for index := range spec.EphemeralContainers {
		container := corev1.Container{
			Name: spec.EphemeralContainers[index].Name, SecurityContext: spec.EphemeralContainers[index].SecurityContext,
			Ports: spec.EphemeralContainers[index].Ports, Env: spec.EphemeralContainers[index].Env, EnvFrom: spec.EphemeralContainers[index].EnvFrom,
		}
		errors = append(errors, validateContainer(&container, path.Child("ephemeralContainers").Index(index))...)
		errors = append(errors, validateContainerReferences(ctx, policy, &container, path.Child("ephemeralContainers").Index(index))...)
	}
	for index := range spec.Volumes {
		errors = append(errors, validateVolume(ctx, policy, &spec.Volumes[index], path.Child("volumes").Index(index))...)
	}
	for index, reference := range spec.ImagePullSecrets {
		errors = append(errors, ValidateReference(ctx, policy, ObjectReference{
			GVR: schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, Name: reference.Name,
		}, path.Child("imagePullSecrets").Index(index).Child("name"))...)
	}
	return errors
}

func validateContainer(container *corev1.Container, path *field.Path) field.ErrorList {
	errors := field.ErrorList{}
	if container.SecurityContext != nil {
		security := container.SecurityContext
		if security.Privileged != nil && *security.Privileged {
			errors = append(errors, field.Forbidden(path.Child("securityContext", "privileged"), "privileged containers are not allowed"))
		}
		if security.AllowPrivilegeEscalation == nil || *security.AllowPrivilegeEscalation {
			errors = append(errors, field.Invalid(path.Child("securityContext", "allowPrivilegeEscalation"), security.AllowPrivilegeEscalation, "must be explicitly false"))
		}
		if security.Capabilities != nil && len(security.Capabilities.Add) > 0 {
			errors = append(errors, field.Forbidden(path.Child("securityContext", "capabilities", "add"), "adding Linux capabilities is not allowed"))
		}
	} else {
		errors = append(errors, field.Required(path.Child("securityContext", "allowPrivilegeEscalation"), "must be explicitly false"))
	}
	for index, port := range container.Ports {
		if port.HostPort != 0 {
			errors = append(errors, field.Forbidden(path.Child("ports").Index(index).Child("hostPort"), "host ports are not allowed"))
		}
	}
	return errors
}

func validateContainerReferences(ctx context.Context, policy PolicyContext, container *corev1.Container, path *field.Path) field.ErrorList {
	errors := field.ErrorList{}
	for index, source := range container.EnvFrom {
		if source.SecretRef != nil {
			errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, Name: source.SecretRef.Name}, path.Child("envFrom").Index(index).Child("secretRef", "name"))...)
		}
		if source.ConfigMapRef != nil {
			errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, Name: source.ConfigMapRef.Name}, path.Child("envFrom").Index(index).Child("configMapRef", "name"))...)
		}
	}
	for index, env := range container.Env {
		if env.ValueFrom == nil {
			continue
		}
		if env.ValueFrom.SecretKeyRef != nil {
			errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, Name: env.ValueFrom.SecretKeyRef.Name}, path.Child("env").Index(index).Child("valueFrom", "secretKeyRef", "name"))...)
		}
		if env.ValueFrom.ConfigMapKeyRef != nil {
			errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, Name: env.ValueFrom.ConfigMapKeyRef.Name}, path.Child("env").Index(index).Child("valueFrom", "configMapKeyRef", "name"))...)
		}
	}
	return errors
}

func validateVolume(ctx context.Context, policy PolicyContext, volume *corev1.Volume, path *field.Path) field.ErrorList {
	errors := field.ErrorList{}
	source := &volume.VolumeSource
	if source.HostPath != nil {
		errors = append(errors, field.Forbidden(path.Child("hostPath"), "hostPath volumes are not allowed"))
	}
	if source.Secret != nil {
		errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, Name: source.Secret.SecretName}, path.Child("secret", "secretName"))...)
	}
	if source.ConfigMap != nil {
		errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, Name: source.ConfigMap.Name}, path.Child("configMap", "name"))...)
	}
	if source.PersistentVolumeClaim != nil {
		errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}, Name: source.PersistentVolumeClaim.ClaimName}, path.Child("persistentVolumeClaim", "claimName"))...)
	}
	if source.Projected != nil {
		for index, projection := range source.Projected.Sources {
			projectionPath := path.Child("projected", "sources").Index(index)
			if projection.ServiceAccountToken != nil {
				errors = append(errors, field.Forbidden(projectionPath.Child("serviceAccountToken"), "projected service account tokens are not allowed"))
			}
			if projection.Secret != nil {
				errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, Name: projection.Secret.Name}, projectionPath.Child("secret", "name"))...)
			}
			if projection.ConfigMap != nil {
				errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, Name: projection.ConfigMap.Name}, projectionPath.Child("configMap", "name"))...)
			}
		}
	}
	if source.CSI != nil {
		_, secretProviderClass := source.CSI.VolumeAttributes["secretProviderClass"]
		if source.CSI.NodePublishSecretRef != nil || secretProviderClass || strings.EqualFold(source.CSI.Driver, "secrets-store.csi.k8s.io") {
			errors = append(errors, field.Forbidden(path.Child("csi"), "CSI secret providers are not allowed"))
		}
	}
	return errors
}

func validateServiceAccount(ctx context.Context, policy PolicyContext, name string, path *field.Path) field.ErrorList {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if name == "default" && (policy.ServiceAccountOrigin == ServiceAccountAbsent || policy.ServiceAccountOrigin == ServiceAccountUnchanged) {
		return nil
	}
	if _, trusted := policy.TrustedServiceAccounts[name]; !trusted || policy.ServiceAccountOrigin != ServiceAccountTrusted {
		return field.ErrorList{field.Forbidden(path, "service account is not approved by a trusted platform plan")}
	}
	return ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}, Name: name}, path)
}

func validateScaleTarget(ctx context.Context, policy PolicyContext, reference autoscalingv2.CrossVersionObjectReference, path *field.Path) field.ErrorList {
	resource := ""
	switch reference.Kind {
	case "Deployment":
		resource = "deployments"
	case "StatefulSet":
		resource = "statefulsets"
	case "ReplicaSet":
		resource = "replicasets"
	default:
		return field.ErrorList{field.NotSupported(path.Child("kind"), reference.Kind, []string{"Deployment", "StatefulSet", "ReplicaSet"})}
	}
	group := strings.TrimSuffix(reference.APIVersion, "/v1")
	if reference.APIVersion == "v1" {
		group = ""
	}
	return ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Group: group, Version: "v1", Resource: resource}, Name: reference.Name}, path.Child("name"))
}

func validateLabelSelector(policy PolicyContext, selector *metav1.LabelSelector, path *field.Path) field.ErrorList {
	if selector == nil {
		return field.ErrorList{field.Required(path, "selector is required")}
	}
	expected := RequiredSelectionLabels(policy)
	for key, value := range expected {
		if selector.MatchLabels[key] != value {
			return field.ErrorList{field.Invalid(path.Child("matchLabels").Key(key), selector.MatchLabels[key], "selector must include the binding ownership label")}
		}
	}
	return nil
}

func validateManualJobSelector(manual *bool, path *field.Path) field.ErrorList {
	if manual != nil && *manual {
		return field.ErrorList{field.Forbidden(path, "manual Job selectors are not allowed")}
	}
	return nil
}

func validateReferenceMetadata(policy PolicyContext, metadata *metav1.ObjectMeta, path *field.Path) field.ErrorList {
	if metadata == nil {
		return field.ErrorList{field.Required(path, "metadata is required")}
	}
	for key, expected := range RequiredOwnershipLabels(policy) {
		if metadata.Labels[key] != expected {
			return field.ErrorList{field.Invalid(path.Child("labels").Key(key), metadata.Labels[key], "must match the binding ownership label")}
		}
	}
	return nil
}
