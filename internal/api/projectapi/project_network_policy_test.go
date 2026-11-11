package projectapi

import "testing"

func TestNormalizeEgressAllowlist(t *testing.T) {
	got, err := normalizeEgressAllowlist("1.1.1.9/24  443,8443\n\n2606:4700::/32\n")
	if err != nil || got != "1.1.1.0/24 443,8443\n2606:4700::/32" {
		t.Fatalf("normalized = %q, %v", got, err)
	}
	for _, invalid := range []string{
		"0.0.0.0/0",
		"10.1.0.0/16",
		"169.254.169.254/32",
		"1.1.1.0/24 0",
		"1.1.1.0/24 443 extra",
		"not-a-cidr",
		// IPv4 映射的 IPv6 会被 Kubernetes 当作 IPv4 网段，必须拒绝。
		"::ffff:0.0.0.0/96",
		"::ffff:169.254.169.254/128",
		"::ffff:1.1.1.1/128 443",
	} {
		if _, err := normalizeEgressAllowlist(invalid); err == nil {
			t.Fatalf("allowlist %q must be rejected", invalid)
		}
	}
	// 包含私网段的公网大网段允许保存，私网部分由渲染器排除。
	if _, err := normalizeEgressAllowlist("0.0.0.0/1"); err != nil {
		t.Fatalf("wide public range rejected: %v", err)
	}
}
