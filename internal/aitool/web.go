package aitool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/LiteyukiStudio/devops/internal/security"
	"golang.org/x/net/html"
)

const (
	webResponseLimit = 2 << 20
	defaultTextLimit = 20_000
	maxTextLimit     = 50_000
	maxRedirects     = 5
	maxWebProxies    = 16
)

type webSearchItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

func (s *Service) webSearch(ctx context.Context, userID string, arguments map[string]any) (Result, error) {
	query := strings.TrimSpace(stringArgument(arguments, "query"))
	if query == "" || len(query) > 300 {
		return Result{}, ErrInvalidInput
	}
	limit := intArgument(arguments, "limit")
	if limit == 0 {
		limit = 5
	}
	if limit < 1 || limit > 10 {
		return Result{}, ErrInvalidInput
	}

	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	response, finalURL, err := s.getWebResource(ctx, userID, searchURL)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()

	content, err := io.ReadAll(io.LimitReader(response.Body, webResponseLimit+1))
	if err != nil {
		return Result{}, fmt.Errorf("%w: search response could not be read", ErrWebRequestFailed)
	}
	if len(content) > webResponseLimit {
		return Result{}, fmt.Errorf("%w: search response is too large", ErrWebContentRejected)
	}
	items := parseDuckDuckGoResults(content, finalURL, limit)
	return Result{
		Value: map[string]any{
			"query":          query,
			"items":          items,
			"source":         finalURL,
			"contentTrust":   "untrusted_external",
			"resultCount":    len(items),
			"searchProvider": "duckduckgo_html",
		},
		Truncated: len(items) == limit,
	}, nil
}

func (s *Service) fetchWebPage(ctx context.Context, userID string, arguments map[string]any) (Result, error) {
	rawURL := strings.TrimSpace(stringArgument(arguments, "url"))
	if rawURL == "" || len(rawURL) > 2048 {
		return Result{}, ErrInvalidInput
	}
	textLimit := intArgument(arguments, "maxCharacters")
	if textLimit == 0 {
		textLimit = defaultTextLimit
	}
	if textLimit < 1 || textLimit > maxTextLimit {
		return Result{}, ErrInvalidInput
	}

	response, finalURL, err := s.getWebResource(ctx, userID, rawURL)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()

	contentType := response.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if !readableWebMediaType(mediaType) {
		return Result{}, fmt.Errorf("%w: unsupported content type %q", ErrWebContentRejected, mediaType)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, webResponseLimit+1))
	if err != nil {
		return Result{}, fmt.Errorf("%w: response could not be read", ErrWebRequestFailed)
	}
	if len(content) > webResponseLimit {
		return Result{}, fmt.Errorf("%w: response is too large", ErrWebContentRejected)
	}

	title := ""
	links := []map[string]string{}
	text := string(content)
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" || mediaType == "" {
		title, text, links = extractHTMLDocument(content, finalURL)
	}
	text = normalizeExtractedText(text)
	truncated := len([]rune(text)) > textLimit
	if truncated {
		text = string([]rune(text)[:textLimit])
	}

	return Result{
		Value: map[string]any{
			"url":          rawURL,
			"finalUrl":     finalURL,
			"title":        title,
			"contentType":  mediaType,
			"content":      text,
			"links":        links,
			"fetchedAt":    time.Now().UTC(),
			"contentTrust": "untrusted_external",
		},
		Truncated: truncated,
	}, nil
}

func (s *Service) getWebResource(ctx context.Context, userID, rawURL string) (*http.Response, string, error) {
	if s.webPolicyProvider == nil {
		return nil, "", fmt.Errorf("%w: web access is not configured", ErrWebRequestFailed)
	}
	policy, err := s.webPolicyProvider(ctx, userID)
	if err != nil {
		return nil, "", fmt.Errorf("%w: policy unavailable", ErrWebRequestFailed)
	}
	parsed, err := policy.ValidateURL(rawURL)
	if err != nil {
		if errors.Is(err, security.ErrBlockedByPolicy) {
			return nil, "", fmt.Errorf("%w: %v", ErrWebTargetBlocked, err)
		}
		return nil, "", ErrInvalidInput
	}
	if webURLContainsCredentials(parsed) {
		return nil, "", ErrInvalidInput
	}

	client := security.NewHTTPClient(policy, 20*time.Second)
	proxyURL, err := s.nextWebProxy(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if proxyURL != nil {
		if err := policy.ValidateProxyTarget(ctx, parsed); err != nil {
			if errors.Is(err, security.ErrBlockedByPolicy) {
				return nil, "", fmt.Errorf("%w: target denied by network policy", ErrWebTargetBlocked)
			}
			return nil, "", ErrInvalidInput
		}
		// 目标地址已在请求与重定向阶段按用户出网策略校验。代理本身是平台管理员
		// 配置的受信传输节点，因此拨号策略只精确放行当前代理主机与端口。
		proxyPolicy := security.AdminEgressPolicy()
		proxyPolicy.AllowedPorts = []int{webURLPort(proxyURL)}
		if net.ParseIP(proxyURL.Hostname()) == nil {
			proxyPolicy.DomainAllowList = []string{proxyURL.Hostname()}
		}
		client = security.NewHTTPClientWithProxy(proxyPolicy, 20*time.Second, proxyURL)
	}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("%w: too many redirects", ErrWebRequestFailed)
		}
		if webURLContainsCredentials(request.URL) {
			return ErrInvalidInput
		}
		if _, err := policy.ValidateURL(request.URL.String()); err != nil {
			if errors.Is(err, security.ErrBlockedByPolicy) {
				return fmt.Errorf("%w: redirect target is blocked", ErrWebTargetBlocked)
			}
			return ErrInvalidInput
		}
		if proxyURL != nil {
			if err := policy.ValidateProxyTarget(request.Context(), request.URL); err != nil {
				if errors.Is(err, security.ErrBlockedByPolicy) {
					return fmt.Errorf("%w: redirect target is blocked", ErrWebTargetBlocked)
				}
				return ErrInvalidInput
			}
		}
		return nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", ErrInvalidInput
	}
	request.Header.Set("Accept", "text/html, text/plain, text/markdown, application/json, application/xml;q=0.9, */*;q=0.1")
	request.Header.Set("User-Agent", "Luna-DevOps-Agent/1.0 (+https://luna-devops.liteyuki.org)")
	response, err := client.Do(request)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebTargetBlocked), errors.Is(err, security.ErrBlockedByPolicy):
			return nil, "", fmt.Errorf("%w: target denied by network policy", ErrWebTargetBlocked)
		case errors.Is(err, ErrInvalidInput):
			return nil, "", ErrInvalidInput
		default:
			return nil, "", fmt.Errorf("%w: remote request failed", ErrWebRequestFailed)
		}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		return nil, "", fmt.Errorf("%w: remote server returned HTTP %d", ErrWebRequestFailed, response.StatusCode)
	}
	return response, response.Request.URL.String(), nil
}

func (s *Service) nextWebProxy(ctx context.Context, userID string) (*url.URL, error) {
	if s.webProxyProvider == nil {
		return nil, nil
	}
	rawPool, err := s.webProxyProvider(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: proxy configuration unavailable", ErrWebRequestFailed)
	}
	proxies, err := ParseWebProxyPool(rawPool)
	if err != nil {
		return nil, fmt.Errorf("%w: proxy configuration is invalid", ErrWebRequestFailed)
	}
	if len(proxies) == 0 {
		return nil, nil
	}
	index := (s.webProxyCursor.Add(1) - 1) % uint64(len(proxies))
	return proxies[index], nil
}

func ParseWebProxyPool(rawPool []string) ([]*url.URL, error) {
	if len(rawPool) > maxWebProxies {
		return nil, fmt.Errorf("proxy pool exceeds %d entries", maxWebProxies)
	}
	proxies := make([]*url.URL, 0, len(rawPool))
	seen := make(map[string]struct{}, len(rawPool))
	for index, raw := range rawPool {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if len(raw) > 2048 || strings.ContainsAny(raw, "\r\n\t") {
			return nil, fmt.Errorf("proxy entry %d is invalid", index+1)
		}
		proxyURL, err := url.Parse(raw)
		if err != nil || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") ||
			proxyURL.Hostname() == "" || proxyURL.Fragment != "" || proxyURL.RawQuery != "" ||
			(proxyURL.Path != "" && proxyURL.Path != "/") || webURLPort(proxyURL) == 0 {
			return nil, fmt.Errorf("proxy entry %d is invalid", index+1)
		}
		normalized := proxyURL.String()
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		proxies = append(proxies, proxyURL)
	}
	return proxies, nil
}

func webURLPort(parsed *url.URL) int {
	if parsed == nil {
		return 0
	}
	if parsed.Port() == "" {
		if parsed.Scheme == "https" {
			return 443
		}
		return 80
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return 0
	}
	return port
}

func readableWebMediaType(mediaType string) bool {
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "", "application/json", "application/ld+json", "application/xml", "application/xhtml+xml",
		"application/yaml", "application/x-yaml", "application/toml", "application/javascript":
		return true
	default:
		return false
	}
}

func parseDuckDuckGoResults(content []byte, baseURL string, limit int) []webSearchItem {
	document, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return nil
	}
	items := make([]webSearchItem, 0, limit)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if len(items) >= limit {
			return
		}
		if node.Type == html.ElementNode && node.Data == "a" && hasHTMLClass(node, "result__a") {
			href := htmlAttribute(node, "href")
			resolved := resolveWebLink(baseURL, href)
			if decoded := decodeDuckDuckGoRedirect(resolved); decoded != "" {
				resolved = decoded
			}
			title := normalizeExtractedText(nodeText(node))
			if title != "" && resolved != "" {
				items = append(items, webSearchItem{Title: title, URL: resolved})
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
			if len(items) >= limit {
				return
			}
		}
	}
	walk(document)
	return items
}

func extractHTMLDocument(content []byte, baseURL string) (string, string, []map[string]string) {
	document, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return "", "", nil
	}
	title := ""
	var textBuilder strings.Builder
	links := make([]map[string]string, 0, 30)
	var walk func(*html.Node, bool)
	walk = func(node *html.Node, ignored bool) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "noscript", "svg", "canvas":
				ignored = true
			case "title":
				title = normalizeExtractedText(nodeText(node))
			case "a":
				if len(links) < 30 {
					href := resolveWebLink(baseURL, htmlAttribute(node, "href"))
					label := normalizeExtractedText(nodeText(node))
					if href != "" && label != "" {
						links = append(links, map[string]string{"label": label, "url": href})
					}
				}
			}
		}
		if node.Type == html.TextNode && !ignored {
			textBuilder.WriteString(node.Data)
			textBuilder.WriteByte('\n')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, ignored)
		}
	}
	walk(document, false)
	return title, textBuilder.String(), links
}

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func hasHTMLClass(node *html.Node, expected string) bool {
	for _, className := range strings.Fields(htmlAttribute(node, "class")) {
		if className == expected {
			return true
		}
	}
	return false
}

func htmlAttribute(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func resolveWebLink(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	target, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(target)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	if webURLContainsCredentials(resolved) {
		return ""
	}
	resolved.Fragment = ""
	return resolved.String()
}

func webURLContainsCredentials(parsed *url.URL) bool {
	if parsed == nil || parsed.User != nil {
		return true
	}
	for key := range parsed.Query() {
		normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", ""), "_", ""))
		switch normalized {
		case "authorization", "accesskey", "apikey", "key", "password", "secret", "signature",
			"sig", "token", "accesstoken", "xamzcredential", "xamzsignature", "xgoogcredential", "xgoogsignature":
			return true
		}
	}
	return false
}

func decodeDuckDuckGoRedirect(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	value := parsed.Query().Get("uddg")
	if value == "" {
		return raw
	}
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return raw
	}
	return decoded
}

func normalizeExtractedText(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	space := false
	newlines := 0
	for _, current := range strings.TrimSpace(value) {
		switch {
		case current == '\n' || current == '\r':
			if builder.Len() > 0 && newlines < 2 {
				builder.WriteByte('\n')
			}
			newlines++
			space = false
		case unicode.IsSpace(current):
			if builder.Len() > 0 && !space && newlines == 0 {
				builder.WriteByte(' ')
			}
			space = true
		default:
			builder.WriteRune(current)
			space = false
			newlines = 0
		}
	}
	return strings.TrimSpace(builder.String())
}
