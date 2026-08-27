package security

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
)

func TestPublicEgressPolicyBlocksPrivateAndSpecialIPs(t *testing.T) {
	policy := PublicEgressPolicy()
	blocked := []string{
		"http://0.0.0.0:8080",
		"http://127.0.0.1:8080",
		"http://10.0.0.1",
		"http://169.254.169.254",
		"http://[::1]:8080",
		"http://[fc00::1]",
	}

	for _, target := range blocked {
		if _, err := policy.ValidateURL(target); !errors.Is(err, ErrBlockedByPolicy) {
			t.Fatalf("%s should be blocked, got %v", target, err)
		}
	}
}

func TestAdminEgressPolicyAllowsPrivateIPs(t *testing.T) {
	policy := AdminEgressPolicy()

	if _, err := policy.ValidateURL("http://127.0.0.1:8080"); err != nil {
		t.Fatalf("admin policy should allow loopback: %v", err)
	}
}

func TestEgressPolicySupportsDomainAndPortControls(t *testing.T) {
	policy := PublicEgressPolicy()
	policy.ApplyIPFilterToNames = false
	policy.DomainBlockList = []string{"evil.test"}
	policy.AllowedPorts = []int{443}

	if _, err := policy.ValidateURL("https://api.example.com/v1"); err != nil {
		t.Fatalf("expected allowed domain: %v", err)
	}
	if _, err := policy.ValidateURL("https://evil.test/v1"); !errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("expected domain to be blocked, got %v", err)
	}
	if _, err := policy.ValidateURL("http://api.example.com/v1"); !errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("expected port to be blocked, got %v", err)
	}
}

func TestDomainAllowListBypassesReservedIPBlock(t *testing.T) {
	policy := PublicEgressPolicy()
	policy.DomainAllowList = []string{"localhost"}
	policy.IPBlockList = []string{"127.0.0.0/8", "::1/128"}

	if _, err := PublicEgressPolicy().ValidateURL("http://localhost:8080"); err != nil {
		t.Fatalf("URL preflight must defer DNS policy to dial time: %v", err)
	}
	lookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	dial := func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("test dial stopped") }
	if _, err := dialContextWithPolicy(context.Background(), "tcp", "localhost:8080", PublicEgressPolicy(), lookup, dial); !errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("localhost should be blocked at dial time, got %v", err)
	}
	if _, err := dialContextWithPolicy(context.Background(), "tcp", "localhost:8080", policy, lookup, dial); errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("domain allowlist should permit reserved resolved ip: %v", err)
	}
}

func TestDomainAllowListMatchesSubdomains(t *testing.T) {
	policy := PublicEgressPolicy()
	policy.DomainAllowList = []string{"github.com"}
	policy.IPBlockList = []string{"198.18.0.0/15"}

	if err := policy.ValidateHostPort("api.github.com", 443); err != nil {
		t.Fatalf("domain allowlist should permit subdomain and skip fake-ip blocklist: %v", err)
	}
}

func TestIPAllowAndBlockLists(t *testing.T) {
	allowPolicy := PublicEgressPolicy()
	allowPolicy.IPAllowList = []string{"127.0.0.1"}

	if _, err := allowPolicy.ValidateURL("http://127.0.0.1:8080"); err != nil {
		t.Fatalf("ip allowlist should permit reserved direct ip: %v", err)
	}

	blockPolicy := AdminEgressPolicy()
	blockPolicy.IPBlockList = []string{"127.0.0.1"}
	if _, err := blockPolicy.ValidateURL("http://127.0.0.1:8080"); !errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("explicit ip blocklist should win, got %v", err)
	}
}

func TestIPBlockErrorIncludesMatchedRule(t *testing.T) {
	policy := PublicEgressPolicy()
	policy.IPBlockList = []string{"198.18.0.0/15"}

	_, err := policy.ValidateURL("http://198.18.0.67")
	if !errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("expected blocked fake ip, got %v", err)
	}
	if !strings.Contains(err.Error(), "198.18.0.0/15") {
		t.Fatalf("expected matched cidr in error, got %v", err)
	}
}

func TestIPv4DoesNotMatchIPv4MappedIPv6CIDR(t *testing.T) {
	policy := AdminEgressPolicy()
	policy.IPBlockList = []string{"::ffff:0:0/96"}

	if err := policy.ValidateHostPort("20.205.243.166", 443); err != nil {
		t.Fatalf("plain IPv4 should not match IPv4-mapped IPv6 CIDR: %v", err)
	}
}

func TestCIDRBlockListMatchesSameIPFamily(t *testing.T) {
	policy := AdminEgressPolicy()
	policy.IPBlockList = []string{"20.205.243.0/24"}

	if err := policy.ValidateHostPort("20.205.243.166", 443); !errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("IPv4 CIDR should block IPv4 target, got %v", err)
	}

	policy.IPBlockList = []string{"2001:db8::/32"}
	if err := policy.ValidateHostPort("2001:db8::1", 443); !errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("IPv6 CIDR should block IPv6 target, got %v", err)
	}
}

func TestEgressPolicyForRole(t *testing.T) {
	if EgressPolicyForRole("user").AllowPrivateNetwork {
		t.Fatal("normal user should not be allowed to access private network")
	}
	if !EgressPolicyForRole(authz.PlatformRoleAdmin).AllowPrivateNetwork {
		t.Fatal("platform admin should be allowed to access private network")
	}
}

func TestDialContextPinsValidatedDNSAddress(t *testing.T) {
	policy := PublicEgressPolicy()
	var dialed string
	lookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	dial := func(_ context.Context, _ string, address string) (net.Conn, error) {
		dialed = address
		return nil, errors.New("test dial stopped")
	}

	_, err := dialContextWithPolicy(context.Background(), "tcp", "example.com:443", policy, lookup, dial)
	if err == nil {
		t.Fatal("test dial should stop with an error")
	}
	if dialed != "93.184.216.34:443" {
		t.Fatalf("dialed address = %q, want validated IP", dialed)
	}
}

func TestDialContextRejectsPrivateDNSResultBeforeDial(t *testing.T) {
	policy := PublicEgressPolicy()
	dialCalled := false
	lookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
	}
	dial := func(context.Context, string, string) (net.Conn, error) {
		dialCalled = true
		return nil, nil
	}

	_, err := dialContextWithPolicy(context.Background(), "tcp", "attacker.example:80", policy, lookup, dial)
	if !errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("private DNS result should be blocked, got %v", err)
	}
	if dialCalled {
		t.Fatal("dial must not run for a blocked DNS result")
	}
}

func TestProxyTargetRetainsPrivateDNSValidation(t *testing.T) {
	target, err := PublicEgressPolicy().ValidateURL("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	if err := PublicEgressPolicy().ValidateProxyTarget(context.Background(), target); !errors.Is(err, ErrBlockedByPolicy) {
		t.Fatalf("proxy target resolving to loopback should be blocked, got %v", err)
	}
}
