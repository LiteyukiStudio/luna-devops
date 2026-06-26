package gitprovider

import (
	"errors"
	"fmt"
	"strings"
)

type UpstreamError struct {
	StatusCode       int
	Message          string
	DocumentationURL string
	Details          []UpstreamErrorDetail
}

type UpstreamErrorDetail struct {
	Resource string `json:"resource"`
	Code     string `json:"code"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "git api returned an upstream error"
	}
	parts := []string{fmt.Sprintf("git api returned %d", e.StatusCode)}
	if strings.TrimSpace(e.Message) != "" {
		parts = append(parts, strings.TrimSpace(e.Message))
	}
	for _, detail := range e.Details {
		detailParts := []string{}
		if strings.TrimSpace(detail.Resource) != "" {
			detailParts = append(detailParts, strings.TrimSpace(detail.Resource))
		}
		if strings.TrimSpace(detail.Field) != "" {
			detailParts = append(detailParts, strings.TrimSpace(detail.Field))
		}
		if strings.TrimSpace(detail.Code) != "" {
			detailParts = append(detailParts, strings.TrimSpace(detail.Code))
		}
		if len(detailParts) > 0 {
			parts = append(parts, strings.Join(detailParts, "."))
		}
	}
	return strings.Join(parts, ": ")
}

func AsUpstreamError(err error) (*UpstreamError, bool) {
	var upstreamErr *UpstreamError
	if errors.As(err, &upstreamErr) {
		return upstreamErr, true
	}
	return nil, false
}
