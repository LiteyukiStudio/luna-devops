package security

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
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
	if role == "platform_admin" {
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
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, portText, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid port", ErrInvalidURL)
		}
		if err := policy.ValidateHostPort(host, port); err != nil {
			return nil, err
		}
		return (&net.Dialer{Timeout: timeout}).DialContext(ctx, network, address)
	}
	return &http.Client{Timeout: timeout, Transport: transport}
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
	originalHost := host
	egressDebug("validate host=%s port=%d allowPrivate=%t applyIPFilterToNames=%t allowedPorts=%s domainAllow=%s domainBlock=%s ipAllow=%s ipBlock=%s", originalHost, port, p.AllowPrivateNetwork, p.ApplyIPFilterToNames, debugPorts(p.AllowedPorts), debugList(p.DomainAllowList), debugList(p.DomainBlockList), debugList(p.IPAllowList), debugList(p.IPBlockList))
	if host == "" || port < 1 || port > 65535 {
		egressDebug("blocked host=%s port=%d reason=invalid-host-or-port", originalHost, port)
		return fmt.Errorf("%w: invalid host or port", ErrInvalidURL)
	}
	if len(p.AllowedPorts) > 0 && !containsPort(p.AllowedPorts, port) {
		egressDebug("blocked host=%s port=%d reason=port-not-allowed allowedPorts=%s", originalHost, port, debugPorts(p.AllowedPorts))
		return fmt.Errorf("%w: port is not allowed", ErrBlockedByPolicy)
	}

	if ip := net.ParseIP(host); ip != nil {
		egressDebug("host is direct ip host=%s ip=%s", originalHost, ip.String())
		if err := p.validateIP(ip); err != nil {
			egressDebug("blocked host=%s ip=%s reason=%v", originalHost, ip.String(), err)
			return err
		}
		egressDebug("allowed host=%s ip=%s reason=direct-ip-policy-pass", originalHost, ip.String())
		return nil
	}
	host = normalizeDomain(host)
	if listed, item := domainListedBy(host, p.DomainBlockList); listed {
		egressDebug("blocked host=%s normalized=%s reason=domain-blocklist matched=%s", originalHost, host, item)
		return fmt.Errorf("%w: domain is in blocklist", ErrBlockedByPolicy)
	}
	if listed, item := domainListedBy(host, p.DomainAllowList); listed {
		egressDebug("allowed host=%s normalized=%s reason=domain-allowlist matched=%s skipIPFilter=true", originalHost, host, item)
		return nil
	}
	if !p.ApplyIPFilterToNames {
		egressDebug("allowed host=%s normalized=%s reason=ip-filter-disabled", originalHost, host)
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		egressDebug("blocked host=%s normalized=%s reason=dns-lookup-failed err=%v", originalHost, host, err)
		return fmt.Errorf("%w: dns lookup failed", ErrBlockedByPolicy)
	}
	egressDebug("resolved host=%s normalized=%s ips=%s", originalHost, host, debugIPs(ips))
	for _, ip := range ips {
		if err := p.validateIP(ip); err != nil {
			egressDebug("blocked host=%s normalized=%s ip=%s reason=%v", originalHost, host, ip.String(), err)
			return err
		}
		egressDebug("ip allowed host=%s normalized=%s ip=%s", originalHost, host, ip.String())
	}
	egressDebug("allowed host=%s normalized=%s reason=all-resolved-ips-pass", originalHost, host)
	return nil
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
		if err == nil && cidr.Contains(ip) {
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
		if err == nil && cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func egressDebug(format string, args ...any) {
	if !egressDebugEnabled() {
		return
	}
	log.Printf("[DEBUG] egress."+format, args...)
}

func egressDebugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug", "trace":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "development", "dev", "local":
		return true
	}
	return false
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
