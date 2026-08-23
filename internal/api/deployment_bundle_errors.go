package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
)

type deploymentBundleErrorSpec struct {
	Status  int
	Message string
}

// deploymentBundleErrorCatalog is the public contract for deployment bundle
// failures. Messages are deliberately static: dependency diagnostics, bundle
// contents, mapped IDs, paths, and secret metadata must never cross this API
// boundary.
var deploymentBundleErrorCatalog = map[string]deploymentBundleErrorSpec{
	"deployment_bundle.candidate_query_invalid":           {Status: http.StatusBadRequest, Message: "deployment bundle candidate query is invalid"},
	"deployment_bundle.digest_mismatch":                   {Status: http.StatusConflict, Message: "deployment bundle changed after preview"},
	"deployment_bundle.export_failed":                     {Status: http.StatusInternalServerError, Message: "deployment bundle export failed"},
	"deployment_bundle.internal_error":                    {Status: http.StatusInternalServerError, Message: "deployment bundle operation failed"},
	"deployment_bundle.invalid_json":                      {Status: http.StatusBadRequest, Message: "deployment bundle JSON is invalid"},
	"deployment_bundle.not_ready":                         {Status: http.StatusConflict, Message: "deployment bundle is not ready to import"},
	"deployment_bundle.reference_ambiguous":               {Status: http.StatusConflict, Message: "deployment bundle reference is ambiguous"},
	"deployment_bundle.reference_forbidden":               {Status: http.StatusForbidden, Message: "deployment bundle reference is unavailable"},
	"deployment_bundle.reference_incompatible":            {Status: http.StatusConflict, Message: "deployment bundle reference is incompatible"},
	"deployment_bundle.reference_missing":                 {Status: http.StatusConflict, Message: "deployment bundle reference is missing"},
	"deployment_bundle.registry_push_credential_required": {Status: http.StatusConflict, Message: "deployment bundle requires a registry push credential"},
	"deployment_bundle.repository_binding_missing":        {Status: http.StatusConflict, Message: "deployment bundle requires a repository binding"},
	"deployment_bundle.runtime_path_conflict":             {Status: http.StatusBadRequest, Message: "deployment bundle runtime paths conflict"},
	"deployment_bundle.secret_encrypt_failed":             {Status: http.StatusInternalServerError, Message: "deployment bundle secret could not be stored"},
	"deployment_bundle.secret_required":                   {Status: http.StatusBadRequest, Message: "deployment bundle secrets must be provided again"},
	"deployment_bundle.secret_requirement_invalid":        {Status: http.StatusBadRequest, Message: "deployment bundle secret requirement is invalid"},
	"deployment_bundle.stage_conflict":                    {Status: http.StatusConflict, Message: "deployment bundle stage conflicts with an existing deployment"},
	"deployment_bundle.too_large":                         {Status: http.StatusRequestEntityTooLarge, Message: "deployment bundle exceeds the 1 MiB limit"},
	"deployment_bundle.unsupported_kind":                  {Status: http.StatusBadRequest, Message: "deployment bundle kind is unsupported"},
	"deployment_bundle.unsupported_version":               {Status: http.StatusBadRequest, Message: "deployment bundle version is unsupported"},
	"deployment.stage_invalid":                            {Status: http.StatusBadRequest, Message: "deployment stage must be dev, test, staging, or prod"},
	"deployment.build_args_invalid":                       {Status: http.StatusBadRequest, Message: "deployment build arguments are invalid"},
	"deployment.resource_quantity_invalid":                {Status: http.StatusBadRequest, Message: "deployment resource quantity is invalid"},
	"deployment.runtime_config_unavailable":               {Status: http.StatusInternalServerError, Message: "deployment runtime configuration is unavailable"},
	"deployment.runtime_config_files_invalid":             {Status: http.StatusBadRequest, Message: "deployment runtime configuration files are invalid"},
	"deployment.runtime_config_path_invalid":              {Status: http.StatusBadRequest, Message: "deployment runtime configuration path is invalid"},
	"deployment.runtime_path_invalid":                     {Status: http.StatusBadRequest, Message: "deployment runtime paths conflict"},
	"deployment.runtime_secret_files_invalid":             {Status: http.StatusInternalServerError, Message: "deployment runtime secret files could not be encoded"},
	"deployment_target.service_account_invalid":           {Status: http.StatusBadRequest, Message: "deployment service account configuration is invalid"},
	"deployment_target.image_ref_required":                {Status: http.StatusBadRequest, Message: "deployment bundle image reference is required"},
	"build_template.invalid":                              {Status: http.StatusBadRequest, Message: "deployment bundle build template configuration is invalid"},
	"build_template.not_found":                            {Status: http.StatusBadRequest, Message: "deployment bundle build template is unavailable"},
}

func deploymentBundleVolumeErrorSpec(code string) (deploymentBundleErrorSpec, bool) {
	conflict := deploymentBundleErrorSpec{Status: http.StatusConflict, Message: "deployment bundle volume mapping conflicts with the destination"}
	invalid := deploymentBundleErrorSpec{Status: http.StatusBadRequest, Message: "deployment bundle volume mapping is invalid"}
	unavailable := deploymentBundleErrorSpec{Status: http.StatusServiceUnavailable, Message: "deployment bundle volume dependency is unavailable"}
	switch code {
	case volume.CodeInvalidInput:
		return invalid, true
	case volume.CodeClusterUnavailable, volume.CodeQuotaUnavailable:
		return unavailable, true
	case volume.CodeNotFound, volume.CodeClaimNotFound, volume.CodeOwnershipConflict,
		volume.CodeIncompatibleCluster, volume.CodeBindingConflict, volume.CodeInUse,
		volume.CodeRevisionConflict, volume.CodeStateConflict, volume.CodeIdempotencyConflict,
		volume.CodeNameConflict, volume.CodeClaimConflict, volume.CodeClaimSpecConflict,
		volume.CodeQuotaExceeded:
		return conflict, true
	default:
		return deploymentBundleErrorSpec{}, false
	}
}

func deploymentBundleErrorSpecFor(code string) (deploymentBundleErrorSpec, bool) {
	spec, ok := deploymentBundleErrorCatalog[strings.TrimSpace(code)]
	if ok {
		return spec, true
	}
	return deploymentBundleVolumeErrorSpec(strings.TrimSpace(code))
}

func deploymentBundleErrorCode(err error) string {
	var bundleErr *deploymentBundleError
	if errors.As(err, &bundleErr) {
		code := strings.TrimSpace(bundleErr.Code)
		if _, ok := deploymentBundleErrorSpecFor(code); ok {
			return code
		}
	}
	if code := volume.ErrorCode(err); code != "" {
		if _, ok := deploymentBundleVolumeErrorSpec(code); ok {
			return code
		}
	}
	return "deployment_bundle.internal_error"
}

func deploymentBundleOperationError(err error) error {
	if err == nil {
		return nil
	}
	// Operation telemetry must remain useful without copying database messages,
	// bundle contents, mapped identifiers, or secret metadata into span events.
	return errors.New(deploymentBundleErrorCode(err))
}

func writeDeploymentBundleCode(ctx *gin.Context, code string) {
	spec, ok := deploymentBundleErrorSpecFor(code)
	if !ok {
		code = "deployment_bundle.internal_error"
		spec = deploymentBundleErrorCatalog[code]
	}
	writeErrorCode(ctx, spec.Status, code, spec.Message)
}

func writeDeploymentBundleError(ctx *gin.Context, err error) {
	writeDeploymentBundleCode(ctx, deploymentBundleErrorCode(err))
}
