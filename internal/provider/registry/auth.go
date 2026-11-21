package registryprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/security"
)

type bearerChallenge struct {
	Realm   string
	Service string
	Scope   string
}

func normalizeOCIEndpoint(endpoint string) (*url.URL, error) {
	parsed, err := ParseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	trimmedPath := strings.TrimRight(parsed.Path, "/")
	if isDockerHubHost(parsed.Hostname()) {
		parsed.Scheme = "https"
		parsed.Host = "registry-1.docker.io"
		if trimmedPath == "/v1" || trimmedPath == "/v2" {
			trimmedPath = ""
		}
	} else if strings.HasSuffix(trimmedPath, "/v2") {
		trimmedPath = strings.TrimSuffix(trimmedPath, "/v2")
	}
	parsed.Path = trimmedPath
	parsed.RawPath = ""
	return parsed, nil
}

func isDockerHubHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "docker.io", "index.docker.io", "registry-1.docker.io", "hub.docker.com":
		return true
	default:
		return false
	}
}

func sameURLOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(effectiveHostPort(left), effectiveHostPort(right))
}

func effectiveHostPort(u *url.URL) string {
	if u == nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	if port == "" {
		return host
	}
	return host + ":" + port
}

func canUseBearerRealm(registryURL, realmURL *url.URL) bool {
	if registryURL == nil || realmURL == nil {
		return false
	}
	if strings.EqualFold(registryURL.Scheme, "https") && !strings.EqualFold(realmURL.Scheme, "https") {
		return false
	}
	regHost := strings.ToLower(strings.TrimSpace(registryURL.Hostname()))
	realmHost := strings.ToLower(strings.TrimSpace(realmURL.Hostname()))
	if regHost == "" || realmHost == "" {
		return false
	}
	if regHost == realmHost {
		return true
	}
	if !strings.EqualFold(realmURL.Scheme, "https") {
		return false
	}
	if isDockerHubHost(regHost) && realmHost == "auth.docker.io" {
		return true
	}
	if strings.HasPrefix(regHost, "registry.") && strings.HasSuffix(regHost, ".aliyuncs.com") &&
		realmHost == "dockerauth."+strings.TrimPrefix(regHost, "registry.") {
		return true
	}
	if strings.HasPrefix(regHost, "registry.") {
		parentDomain := strings.TrimPrefix(regHost, "registry.")
		if parentDomain == realmHost && strings.Contains(parentDomain, ".") {
			return true
		}
	}
	return false
}

func parseBearerChallenges(headers []string) (bearerChallenge, bool) {
	for _, raw := range headers {
		for _, segment := range splitAuthChallengeSegments(raw) {
			trimmed := strings.TrimSpace(segment)
			if len(trimmed) < 7 || !strings.EqualFold(trimmed[:6], "Bearer") || (trimmed[6] != ' ' && trimmed[6] != '\t') {
				continue
			}
			params := parseAuthParams(strings.TrimSpace(trimmed[7:]))
			if strings.TrimSpace(params["realm"]) == "" {
				continue
			}
			return bearerChallenge{
				Realm:   strings.TrimSpace(params["realm"]),
				Service: strings.TrimSpace(params["service"]),
				Scope:   strings.TrimSpace(params["scope"]),
			}, true
		}
	}
	return bearerChallenge{}, false
}

// splitAuthChallengeSegments splits comma-delimited RFC 9110 WWW-Authenticate
// challenges while keeping comma-separated auth-param pairs attached to their scheme.
func splitAuthChallengeSegments(header string) []string {
	var segments []string
	inQuotes := false
	start := 0
	for i := 0; i < len(header); i++ {
		if header[i] == '\\' && inQuotes && i+1 < len(header) {
			i++
			continue
		}
		if header[i] == '"' {
			inQuotes = !inQuotes
			continue
		}
		if !inQuotes && header[i] == ',' {
			tail := strings.TrimLeft(header[i+1:], " \t")
			if looksLikeNewAuthScheme(tail) {
				segments = append(segments, strings.TrimSpace(header[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(header[start:]); tail != "" {
		segments = append(segments, tail)
	}
	return segments
}

func looksLikeNewAuthScheme(s string) bool {
	if s == "" {
		return false
	}
	spaceIdx := strings.IndexAny(s, " \t")
	if spaceIdx <= 0 {
		return false
	}
	token := s[:spaceIdx]
	if strings.Contains(token, "=") || strings.Contains(token, ",") || strings.Contains(token, "\"") {
		return false
	}
	rest := strings.TrimLeft(s[spaceIdx+1:], " \t")
	return rest != "" && !strings.HasPrefix(rest, "=")
}

func parseAuthParams(input string) map[string]string {
	params := make(map[string]string)
	rest := input
	for len(rest) > 0 {
		rest = strings.TrimLeft(rest, " \t,")
		if rest == "" || looksLikeNewAuthScheme(rest) {
			break
		}
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(rest[:eq]))
		rest = strings.TrimLeft(rest[eq+1:], " \t")
		if rest == "" {
			break
		}
		var val string
		if rest[0] == '"' {
			rest = rest[1:]
			var sb strings.Builder
			closed := false
			for i := 0; i < len(rest); i++ {
				if rest[i] == '\\' && i+1 < len(rest) {
					i++
					sb.WriteByte(rest[i])
					continue
				}
				if rest[i] == '"' {
					rest = rest[i+1:]
					closed = true
					break
				}
				sb.WriteByte(rest[i])
			}
			if !closed {
				rest = ""
			}
			val = sb.String()
		} else {
			comma := strings.IndexByte(rest, ',')
			if comma < 0 {
				val = strings.TrimSpace(rest)
				rest = ""
			} else {
				val = strings.TrimSpace(rest[:comma])
				rest = rest[comma+1:]
			}
		}
		if key != "" {
			params[key] = val
		}
	}
	return params
}

func basicAuthHeader(credential Credential) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(credential.Username+":"+credential.Secret))
}

type publicRedirectContextKey struct{}

type originScopedTransport struct {
	primary http.RoundTripper
	public  http.RoundTripper
}

func (t originScopedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.public != nil && req.Context().Value(publicRedirectContextKey{}) == true {
		return t.public.RoundTrip(req)
	}
	return t.primary.RoundTrip(req)
}

func (t originScopedTransport) CloseIdleConnections() {
	type idleCloser interface{ CloseIdleConnections() }
	if closer, ok := t.primary.(idleCloser); ok {
		closer.CloseIdleConnections()
	}
	if closer, ok := t.public.(idleCloser); ok {
		closer.CloseIdleConnections()
	}
}

func newRegistryHTTPClient(policy security.EgressPolicy, timeout time.Duration, disallowRedirects bool) *http.Client {
	client := security.NewHTTPClient(policy, timeout)
	if disallowRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		return client
	}
	if policy.AllowPrivateNetwork {
		publicPolicy := policy
		publicPolicy.AllowPrivateNetwork = false
		publicPolicy.ApplyIPFilterToNames = true
		publicClient := security.NewHTTPClient(publicPolicy, timeout)
		client.Transport = originScopedTransport{primary: client.Transport, public: publicClient.Transport}
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("stopped after 5 redirects")
		}
		if req.URL.User != nil {
			return security.ErrInvalidURL
		}
		initial := via[0].URL
		sawHTTPS := false
		crossedOrigin := !sameURLOrigin(initial, req.URL)
		for _, prev := range via {
			if strings.EqualFold(prev.URL.Scheme, "https") {
				sawHTTPS = true
			}
			if !sameURLOrigin(initial, prev.URL) {
				crossedOrigin = true
			}
		}
		if sawHTTPS && !strings.EqualFold(req.URL.Scheme, "https") {
			return errors.New("refusing https to http redirect")
		}
		if _, err := policy.ValidateURL(req.URL.String()); err != nil {
			return err
		}
		if crossedOrigin {
			req.Header.Del("Authorization")
			if policy.AllowPrivateNetwork {
				publicPolicy := policy
				publicPolicy.AllowPrivateNetwork = false
				publicPolicy.ApplyIPFilterToNames = true
				if err := publicPolicy.ValidateProxyTarget(req.Context(), req.URL); err != nil {
					return err
				}
				*req = *req.WithContext(context.WithValue(req.Context(), publicRedirectContextKey{}, true))
			}
		}
		return nil
	}
	return client
}

func drainAndCloseBody(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4096))
	_ = body.Close()
}

func doRegistryGET(parent context.Context, requestURL string, accept string, policy security.EgressPolicy, credential Credential, timeout time.Duration) (*http.Response, error) {
	targetURL, err := policy.ValidateURL(requestURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	client := newRegistryHTTPClient(policy, timeout, false)
	cleanup := func() {
		cancel()
		client.CloseIdleConnections()
	}

	send := func(authHeader string) (*http.Response, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if reqErr != nil {
			return nil, reqErr
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		return client.Do(req)
	}

	initialAuth := ""
	if credential.Secret != "" {
		initialAuth = basicAuthHeader(credential)
	}
	resp, err := send(initialAuth)
	if err != nil {
		cleanup()
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return wrapResponseCancel(resp, cleanup), nil
	}

	challenge, ok := parseBearerChallenges(resp.Header.Values("Www-Authenticate"))
	if !ok {
		return wrapResponseCancel(resp, cleanup), nil
	}
	realmURL, err := policy.ValidateURL(challenge.Realm)
	if err != nil || !canUseBearerRealm(targetURL, realmURL) {
		return wrapResponseCancel(resp, cleanup), nil
	}

	tokenClient := newRegistryHTTPClient(policy, timeout, true)
	token, ok := exchangeBearerToken(ctx, tokenClient, realmURL, challenge, credential)
	tokenClient.CloseIdleConnections()
	if !ok {
		return wrapResponseCancel(resp, cleanup), nil
	}
	drainAndCloseBody(resp.Body)

	retryResp, retryErr := send("Bearer " + token)
	if retryErr != nil {
		cleanup()
		return nil, retryErr
	}
	return wrapResponseCancel(retryResp, cleanup), nil
}

func exchangeBearerToken(ctx context.Context, client *http.Client, realmURL *url.URL, challenge bearerChallenge, credential Credential) (string, bool) {
	tokenEndpoint := *realmURL
	query := tokenEndpoint.Query()
	if challenge.Service != "" {
		query.Set("service", challenge.Service)
	}
	if challenge.Scope != "" {
		query.Del("scope")
		for _, scopeItem := range strings.Fields(challenge.Scope) {
			query.Add("scope", scopeItem)
		}
	}
	tokenEndpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenEndpoint.String(), nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Accept", "application/json")
	if credential.Secret != "" {
		req.Header.Set("Authorization", basicAuthHeader(credential))
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer drainAndCloseBody(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", false
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	token := strings.TrimSpace(payload.Token)
	if token == "" {
		token = strings.TrimSpace(payload.AccessToken)
	}
	return token, token != ""
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func wrapResponseCancel(resp *http.Response, cancel context.CancelFunc) *http.Response {
	if resp == nil || resp.Body == nil {
		cancel()
		return resp
	}
	resp.Body = &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}
	return resp
}
