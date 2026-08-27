package security

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

type ListMode string

const (
	ListModeAllow ListMode = "allow"
	ListModeBlock ListMode = "block"
)

type EgressPolicy struct {
	AllowPrivateNetwork  bool
	DomainAllowList      []string
	DomainBlockList      []string
	IPAllowList          []string
	IPBlockList          []string
	AllowedPorts         []int
	ApplyIPFilterToNames bool
}

var (
	ErrInvalidURL      = errors.New("egress url is invalid")
	ErrBlockedByPolicy = errors.New("egress target is blocked by policy")
)

func PublicEgressPolicy() EgressPolicy {
	return EgressPolicy{
		AllowPrivateNetwork:  false,
		ApplyIPFilterToNames: true,
	}
}

func AdminEgressPolicy() EgressPolicy {
	policy := PublicEgressPolicy()
	policy.AllowPrivateNetwork = true
	return policy
}

func EgressPolicyForRole(role string) EgressPolicy {
	if role == authz.PlatformRoleAdmin {
		return AdminEgressPolicy()
	}
	return PublicEgressPolicy()
}

func ReservedIPBlockList() []string {
	return []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
		"255.255.255.255/32",
		"::/128",
		"::1/128",
		"::ffff:0:0/96",
		"64:ff9b::/96",
		"100::/64",
		"2001::/23",
		"2001:db8::/32",
		"fc00::/7",
		"fe80::/10",
		"ff00::/8",
	}
}

func ReservedIPBlockListText() string {
	return strings.Join(ReservedIPBlockList(), "\n")
}

func NewHTTPClient(policy EgressPolicy, timeout time.Duration) *http.Client {
	return NewHTTPClientWithProxy(policy, timeout, nil)
}

func NewHTTPClientWithProxy(policy EgressPolicy, timeout time.Duration, proxyURL *url.URL) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	dialer := &net.Dialer{Timeout: timeout}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialContextWithPolicy(ctx, network, address, policy, net.DefaultResolver.LookupIPAddr, dialer.DialContext)
	}
	return telemetry.InstrumentHTTPClient(&http.Client{Timeout: timeout, Transport: transport})
}

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)
type dialContextFunc func(context.Context, string, string) (net.Conn, error)

func dialContextWithPolicy(ctx context.Context, network, address string, policy EgressPolicy, lookup lookupIPAddrFunc, dial dialContextFunc) (net.Conn, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid port", ErrInvalidURL)
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if err := policy.validateHostAndPortRules(host, port); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := policy.validateIP(ip); err != nil {
			return nil, err
		}
		return dial(ctx, network, net.JoinHostPort(ip.String(), portText))
	}

	addresses, err := lookup(ctx, normalizeDomain(host))
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("%w: dns lookup failed", ErrBlockedByPolicy)
	}
	for _, resolved := range addresses {
		if err := policy.validateResolvedIP(host, resolved.IP); err != nil {
			return nil, err
		}
	}
	var lastErr error
	for _, resolved := range addresses {
		connection, err := dial(ctx, network, net.JoinHostPort(resolved.IP.String(), portText))
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (p EgressPolicy) ValidateURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: malformed url", ErrInvalidURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: unsupported scheme", ErrInvalidURL)
	}
	port := defaultPort(parsed)
	if err := p.ValidateHostPort(parsed.Hostname(), port); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (p EgressPolicy) ValidateHostPort(host string, port int) error {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if err := p.validateHostAndPortRules(host, port); err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip != nil {
		return p.validateIP(ip)
	}
	// DNS is intentionally resolved only by dialContextWithPolicy, which
	// validates and dials the same resolved address to prevent rebinding.
	return nil
}

// ValidateProxyTarget resolves a target before handing it to an HTTP proxy.
// Direct connections use dialContextWithPolicy instead, where validation and
// dialing share the same resolved address.
func (p EgressPolicy) ValidateProxyTarget(ctx context.Context, target *url.URL) error {
	if target == nil {
		return fmt.Errorf("%w: malformed url", ErrInvalidURL)
	}
	host := strings.TrimSpace(target.Hostname())
	if err := p.ValidateHostPort(host, defaultPort(target)); err != nil {
		return err
	}
	if net.ParseIP(host) != nil || domainListed(host, p.DomainAllowList) || !p.ApplyIPFilterToNames {
		return nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, normalizeDomain(host))
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("%w: dns lookup failed", ErrBlockedByPolicy)
	}
	for _, address := range addresses {
		if err := p.validateResolvedIP(host, address.IP); err != nil {
			return err
		}
	}
	return nil
}

func (p EgressPolicy) validateHostAndPortRules(host string, port int) error {
	if host == "" || port < 1 || port > 65535 {
		return fmt.Errorf("%w: invalid host or port", ErrInvalidURL)
	}
	if len(p.AllowedPorts) > 0 && !containsPort(p.AllowedPorts, port) {
		return fmt.Errorf("%w: port is not allowed", ErrBlockedByPolicy)
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	if listed, _ := domainListedBy(host, p.DomainBlockList); listed {
		return fmt.Errorf("%w: domain is in blocklist", ErrBlockedByPolicy)
	}
	return nil
}

func (p EgressPolicy) validateResolvedIP(host string, ip net.IP) error {
	if domainListed(host, p.DomainAllowList) || !p.ApplyIPFilterToNames {
		return nil
	}
	return p.validateIP(ip)
}

func (p EgressPolicy) validateIP(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("%w: empty ip", ErrBlockedByPolicy)
	}
	if listed, item := ipListedBy(ip, p.IPBlockList); listed {
		return fmt.Errorf("%w: ip is in blocklist (%s)", ErrBlockedByPolicy, item)
	}
	if listed, item := ipListedBy(ip, p.IPAllowList); listed {
		egressDebug("ip allowlist matched ip=%s matched=%s", ip.String(), item)
		return nil
	}
	if p.AllowPrivateNetwork {
		egressDebug("ip allowed by private-network policy ip=%s", ip.String())
		return nil
	}
	if isPrivateOrSpecialIP(ip) {
		return fmt.Errorf("%w: private or special ip is not allowed", ErrBlockedByPolicy)
	}
	return nil
}

func defaultPort(parsed *url.URL) int {
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err == nil {
			return port
		}
		return 0
	}
	if parsed.Scheme == "https" {
		return 443
	}
	return 80
}

func containsPort(ports []int, target int) bool {
	for _, port := range ports {
		if port == target {
			return true
		}
	}
	return false
}

func domainListed(host string, list []string) bool {
	listed, _ := domainListedBy(host, list)
	return listed
}

func domainListedBy(host string, list []string) (bool, string) {
	host = normalizeDomain(host)
	for _, item := range list {
		item = normalizeDomain(item)
		if item == "" {
			continue
		}
		if item == host {
			return true, item
		}
		if strings.HasSuffix(host, "."+item) {
			return true, item
		}
		if strings.HasPrefix(item, "*.") {
			suffix := strings.TrimPrefix(item, "*.")
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				return true, item
			}
		}
	}
	return false, ""
}

func normalizeDomain(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func ipListed(ip net.IP, list []string) bool {
	listed, _ := ipListedBy(ip, list)
	return listed
}

func ipListedBy(ip net.IP, list []string) (bool, string) {
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if listed := net.ParseIP(item); listed != nil && listed.Equal(ip) {
			return true, item
		}
		_, cidr, err := net.ParseCIDR(item)
		if err == nil && cidrContainsSameFamily(cidr, ip) {
			return true, item
		}
	}
	return false, ""
}

func isPrivateOrSpecialIP(ip net.IP) bool {
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsPrivate() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		return inCIDRs(v4, ReservedIPBlockList())
	}
	return inCIDRs(ip, ReservedIPBlockList())
}

func inCIDRs(ip net.IP, cidrs []string) bool {
	for _, item := range cidrs {
		_, cidr, err := net.ParseCIDR(item)
		if err == nil && cidrContainsSameFamily(cidr, ip) {
			return true
		}
	}
	return false
}

func cidrContainsSameFamily(cidr *net.IPNet, ip net.IP) bool {
	normalizedIP := normalizeIPForCIDR(cidr, ip)
	if normalizedIP == nil {
		return false
	}
	return cidr.Contains(normalizedIP)
}

func normalizeIPForCIDR(cidr *net.IPNet, ip net.IP) net.IP {
	if cidr == nil || ip == nil {
		return nil
	}
	_, bits := cidr.Mask.Size()
	if bits == 32 {
		return ip.To4()
	}
	if bits != 128 {
		return nil
	}
	if ip.To4() != nil {
		return nil
	}
	return ip.To16()
}

func egressDebug(format string, args ...any) {
	if !egressDebugEnabled() {
		return
	}
	telemetry.Logger().Debug("egress policy decision",
		slog.String("event.name", "security.egress.decision"),
		slog.String("decision.detail", fmt.Sprintf(format, args...)),
	)
}

func egressDebugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug", "trace":
		return true
	}
	return config.RuntimeMode() == "development"
}

func debugList(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, "|")
}

func debugPorts(values []int) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, "|")
}

func debugIPs(values []net.IP) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, value.String())
	}
	return strings.Join(parts, "|")
}
