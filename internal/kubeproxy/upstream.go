package kubeproxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var strippedRequestHeaders = []string{
	"Authorization", "Proxy-Authorization", "Cookie", "Audit-ID",
	"Traceparent", "Tracestate", "Baggage", "Forwarded",
	"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Port",
	"X-Remote-User", "X-Remote-Group", "X-Remote-Extra",
}

func BuildUpstreamRequest(request *http.Request, upstream Upstream, escapedKubePath string) (*http.Request, error) {
	if request == nil || upstream.BaseURL == nil || upstream.Transport == nil {
		return nil, Unavailable(CodeUnavailable, fmt.Errorf("upstream is incomplete"))
	}
	if upstream.BaseURL.User != nil {
		return nil, Unavailable(CodeUnavailable, fmt.Errorf("upstream URL userinfo is not allowed"))
	}
	if _, err := validateEscapedPath(escapedKubePath); err != nil {
		return nil, err
	}
	target := *upstream.BaseURL
	basePath := strings.TrimSuffix(target.EscapedPath(), "/")
	if basePath == "" {
		basePath = strings.TrimSuffix(target.Path, "/")
	}
	if escapedKubePath == "" {
		escapedKubePath = "/"
	}
	target.RawPath = basePath + escapedKubePath
	decoded, err := url.PathUnescape(target.RawPath)
	if err != nil {
		return nil, BadRequest(CodeBadRequest, fmt.Errorf("upstream path cannot be encoded"))
	}
	target.Path = decoded
	target.RawQuery = request.URL.RawQuery
	target.Fragment = ""

	clone := request.Clone(request.Context())
	clone.URL = &target
	clone.Host = target.Host
	clone.RequestURI = ""
	clone.Header = request.Header.Clone()
	clone.TransferEncoding = nil
	clone.Trailer = nil
	clone.Close = false
	SanitizeUpstreamHeaders(clone.Header, false)
	return clone, nil
}

func SanitizeUpstreamHeaders(header http.Header, preserveUpgrade bool) {
	for _, name := range strippedRequestHeaders {
		header.Del(name)
	}
	for name := range header {
		if strings.HasPrefix(strings.ToLower(name), "impersonate-") || strings.HasPrefix(strings.ToLower(name), "x-remote-extra-") {
			header.Del(name)
		}
	}
	connectionTokens := append([]string(nil), header.Values("Connection")...)
	for _, value := range connectionTokens {
		for _, token := range strings.Split(value, ",") {
			name := strings.TrimSpace(token)
			if name != "" && (!preserveUpgrade || !strings.EqualFold(name, "upgrade")) {
				header.Del(name)
			}
		}
	}
	if !preserveUpgrade {
		for _, name := range []string{"Connection", "Upgrade", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding"} {
			header.Del(name)
		}
	} else {
		header.Set("Connection", "Upgrade")
	}
}
