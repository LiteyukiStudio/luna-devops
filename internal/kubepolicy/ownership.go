package kubepolicy

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateObjectOwnership verifies the labels that form the tenant ownership
// boundary on an admission-resolved object and every controller template that
// creates descendant resources.
func ValidateObjectOwnership(policy PolicyContext, object *unstructured.Unstructured) field.ErrorList {
	if object == nil {
		return field.ErrorList{field.Required(field.NewPath("object"), "object is required")}
	}
	errors := field.ErrorList{}
	expected := RequiredOwnershipLabels(policy)
	_, expectedApplication := expected[ApplicationIDLabel]
	forbidApplication := len(policy.ExpectedOwnershipLabels) > 0 && !expectedApplication
	namespace := strings.TrimSpace(object.GetNamespace())
	if namespace == "" {
		errors = append(errors, field.Required(field.NewPath("metadata", "namespace"), "namespace is required"))
	} else if namespace != policy.Namespace {
		errors = append(errors, field.Invalid(field.NewPath("metadata", "namespace"), namespace, "must match the project namespace"))
	}
	errors = append(errors, validateOwnershipLabels(object.GetLabels(), expected, forbidApplication, field.NewPath("metadata", "labels"))...)
	errors = append(errors, validateLifecycleLabels(object.GetLabels(), policy.ExpectedLifecycleLabels, policy.ProtectLifecycleLabels, field.NewPath("metadata", "labels"))...)

	switch object.GetKind() {
	case "Deployment", "ReplicaSet", "Job", "StatefulSet":
		errors = append(errors, validateNestedOwnership(object.Object, []string{"spec", "template", "metadata", "labels"}, expected, forbidApplication, policy.ExpectedLifecycleLabels, policy.ProtectLifecycleLabels)...)
	case "CronJob":
		errors = append(errors, validateNestedOwnership(object.Object, []string{"spec", "jobTemplate", "metadata", "labels"}, expected, forbidApplication, policy.ExpectedLifecycleLabels, policy.ProtectLifecycleLabels)...)
		errors = append(errors, validateNestedOwnership(object.Object, []string{"spec", "jobTemplate", "spec", "template", "metadata", "labels"}, expected, forbidApplication, policy.ExpectedLifecycleLabels, policy.ProtectLifecycleLabels)...)
	}
	if object.GetKind() == "StatefulSet" {
		claims, found, err := unstructured.NestedSlice(object.Object, "spec", "volumeClaimTemplates")
		if err != nil {
			errors = append(errors, field.Invalid(field.NewPath("spec", "volumeClaimTemplates"), nil, "must be a list"))
		} else if found {
			for index, raw := range claims {
				claim, ok := raw.(map[string]any)
				path := field.NewPath("spec", "volumeClaimTemplates").Index(index).Child("metadata", "labels")
				if !ok {
					errors = append(errors, field.Invalid(path, raw, "claim template must be an object"))
					continue
				}
				labels, _, nestedErr := unstructured.NestedStringMap(claim, "metadata", "labels")
				if nestedErr != nil {
					errors = append(errors, field.Invalid(path, nil, "labels must be a string map"))
					continue
				}
				errors = append(errors, validateOwnershipLabels(labels, expected, forbidApplication, path)...)
				errors = append(errors, validateLifecycleLabels(labels, policy.ExpectedLifecycleLabels, policy.ProtectLifecycleLabels, path)...)
			}
		}
	}
	return errors
}

func validateNestedOwnership(object map[string]any, segments []string, expected map[string]string, forbidApplication bool, expectedLifecycle map[string]string, protectLifecycle bool) field.ErrorList {
	path := field.NewPath(segments[0], segments[1:]...)
	labels, found, err := unstructured.NestedStringMap(object, segments...)
	if err != nil {
		return field.ErrorList{field.Invalid(path, nil, "labels must be a string map")}
	}
	if !found {
		return field.ErrorList{field.Required(path, "ownership labels are required")}
	}
	errors := validateOwnershipLabels(labels, expected, forbidApplication, path)
	return append(errors, validateLifecycleLabels(labels, expectedLifecycle, protectLifecycle, path)...)
}

func validateOwnershipLabels(actual, expected map[string]string, forbidApplication bool, path *field.Path) field.ErrorList {
	errors := field.ErrorList{}
	if forbidApplication {
		if _, exists := actual[ApplicationIDLabel]; exists {
			errors = append(errors, field.Forbidden(path.Key(ApplicationIDLabel), "project bindings cannot assign application ownership"))
		}
	}
	for key, value := range expected {
		if strings.TrimSpace(value) == "" {
			errors = append(errors, field.InternalError(path.Key(key), fmt.Errorf("expected ownership label is empty")))
			continue
		}
		if actual[key] != value {
			errors = append(errors, field.Invalid(path.Key(key), actual[key], "must match the binding ownership label"))
		}
	}
	return errors
}

func validateLifecycleLabels(actual, expected map[string]string, enforce bool, path *field.Path) field.ErrorList {
	if !enforce {
		return nil
	}
	errors := field.ErrorList{}
	for _, key := range protectedLifecycleLabelKeys {
		expectedValue, preserve := expected[key]
		actualValue, exists := actual[key]
		switch {
		case !preserve && exists:
			errors = append(errors, field.Forbidden(path.Key(key), "kubectl cannot assign platform lifecycle ownership"))
		case preserve && (!exists || actualValue != expectedValue):
			errors = append(errors, field.Invalid(path.Key(key), actualValue, "must preserve the platform lifecycle ownership label"))
		}
	}
	return errors
}
