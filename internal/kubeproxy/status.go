package kubeproxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	CodeBadRequest                 = "kube_gateway.bad_request"
	CodeUnauthorized               = "kube_gateway.unauthorized"
	CodeForbidden                  = "kube_gateway.forbidden"
	CodeNotFound                   = "kube_gateway.not_found"
	CodeMethodNotAllowed           = "kube_gateway.method_not_allowed"
	CodeNotAcceptable              = "kube_gateway.not_acceptable"
	CodeConflict                   = "kube_gateway.conflict"
	CodeRequestTooLarge            = "kube_gateway.request_too_large"
	CodeInvalid                    = "kube_gateway.invalid"
	CodeRateLimited                = "kube_gateway.rate_limited"
	CodeUnavailable                = "kube_gateway.unavailable"
	CodeUpstreamTimeout            = "kube_gateway.upstream_timeout"
	CodeMetricsSelectorUnavailable = "kube_gateway.metrics_selector_unavailable"
	CodeAuditUnavailable           = "kube_gateway.audit_unavailable"
)

type StatusError struct {
	Code       string
	HTTPStatus int
	Reason     metav1.StatusReason
	Message    string
	RetryAfter int32
	Cause      error
	Details    *metav1.StatusDetails
}

func (err *StatusError) Error() string {
	if err == nil {
		return ""
	}
	return err.Code
}

func (err *StatusError) Unwrap() error { return err.Cause }
func (err *StatusError) ErrorCode() string {
	if err == nil {
		return ""
	}
	return err.Code
}

func NewStatusError(status int, reason metav1.StatusReason, code, message string, cause error) *StatusError {
	return &StatusError{Code: strings.TrimSpace(code), HTTPStatus: status, Reason: reason, Message: strings.TrimSpace(message), Cause: cause}
}

func BadRequest(code string, cause error) *StatusError {
	if code == "" {
		code = CodeBadRequest
	}
	return NewStatusError(http.StatusBadRequest, metav1.StatusReasonBadRequest, code, "the Kubernetes request is invalid", cause)
}

func Unauthorized(cause error) *StatusError {
	return NewStatusError(http.StatusUnauthorized, metav1.StatusReasonUnauthorized, CodeUnauthorized, "the Kubernetes credential is invalid or expired", cause)
}

func Forbidden(code string, cause error) *StatusError {
	if code == "" {
		code = CodeForbidden
	}
	return NewStatusError(http.StatusForbidden, metav1.StatusReasonForbidden, code, "the requested operation is not allowed", cause)
}

func NotFound(gvr schema.GroupVersionResource, name string) *StatusError {
	return &StatusError{Code: CodeNotFound, HTTPStatus: http.StatusNotFound, Reason: metav1.StatusReasonNotFound, Message: "the requested resource was not found", Details: &metav1.StatusDetails{Name: name, Group: gvr.Group, Kind: gvr.Resource}}
}

func MethodNotAllowed() *StatusError {
	return NewStatusError(http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed, CodeMethodNotAllowed, "the HTTP method is not supported", nil)
}

func NotAcceptable(cause error) *StatusError {
	return NewStatusError(http.StatusNotAcceptable, metav1.StatusReasonNotAcceptable, CodeNotAcceptable, "none of the requested response representations are supported", cause)
}

func TooLarge(cause error) *StatusError {
	return NewStatusError(http.StatusRequestEntityTooLarge, metav1.StatusReasonRequestEntityTooLarge, CodeRequestTooLarge, "the request body is too large", cause)
}

func Conflict(cause error) *StatusError {
	return NewStatusError(http.StatusConflict, metav1.StatusReasonConflict, CodeConflict, "the Kubernetes operation conflicts with the current object state", cause)
}

func Invalid(cause error) *StatusError {
	return NewStatusError(http.StatusUnprocessableEntity, metav1.StatusReasonInvalid, CodeInvalid, "the Kubernetes object did not pass policy validation", cause)
}

func RateLimited(cause error) *StatusError {
	return &StatusError{Code: CodeRateLimited, HTTPStatus: http.StatusTooManyRequests, Reason: metav1.StatusReasonTooManyRequests, Message: "the request limit was exceeded", RetryAfter: 1, Cause: cause}
}

func Unavailable(code string, cause error) *StatusError {
	if code == "" {
		code = CodeUnavailable
	}
	return NewStatusError(http.StatusServiceUnavailable, metav1.StatusReasonServiceUnavailable, code, "the Kubernetes upstream is unavailable", cause)
}

func GatewayTimeout(cause error) *StatusError {
	return NewStatusError(http.StatusGatewayTimeout, metav1.StatusReasonTimeout, CodeUpstreamTimeout, "the Kubernetes upstream timed out", cause)
}

func AsStatusError(err error) *StatusError {
	var status *StatusError
	if errors.As(err, &status) {
		return status
	}
	return Unavailable(CodeUnavailable, err)
}

func WriteStatus(writer http.ResponseWriter, err error) {
	statusErr := AsStatusError(err)
	message := statusErr.Message
	if message == "" {
		message = statusErr.Code
	}
	details := statusErr.Details.DeepCopy()
	if details == nil {
		details = &metav1.StatusDetails{}
	}
	details.Causes = append(details.Causes, metav1.StatusCause{Type: metav1.CauseTypeUnexpectedServerResponse, Message: statusErr.Code})
	status := metav1.Status{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   metav1.StatusFailure, Message: message, Reason: statusErr.Reason, Code: int32(statusErr.HTTPStatus),
		Details: details,
	}
	if statusErr.RetryAfter > 0 {
		status.Details.RetryAfterSeconds = statusErr.RetryAfter
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if statusErr.HTTPStatus == http.StatusUnauthorized {
		writer.Header().Set("WWW-Authenticate", "Bearer")
	}
	if statusErr.RetryAfter > 0 {
		writer.Header().Set("Retry-After", fmt.Sprintf("%d", statusErr.RetryAfter))
	}
	writer.WriteHeader(statusErr.HTTPStatus)
	_ = json.NewEncoder(writer).Encode(status)
}
