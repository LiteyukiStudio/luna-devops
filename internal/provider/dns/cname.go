package dns

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

type Resolver interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
}

type NetResolver struct {
	resolver *net.Resolver
}

func NewNetResolver() NetResolver {
	return NetResolver{resolver: net.DefaultResolver}
}

func (r NetResolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	resolver := r.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	operationCtx, end := telemetry.StartOperation(ctx, "dns", "lookup_cname",
		attribute.String("dns.question.name", strings.TrimSpace(host)),
	)
	result, err := resolver.LookupCNAME(operationCtx, host)
	end(err)
	return result, err
}

func CheckCNAME(ctx context.Context, resolver Resolver, host string, target string) error {
	host = strings.TrimSpace(host)
	target = normalizeDNSName(target)
	if host == "" || target == "" {
		return fmt.Errorf("host and cname target are required")
	}
	if resolver == nil {
		resolver = NewNetResolver()
	}
	got, err := resolver.LookupCNAME(ctx, host)
	if err != nil {
		return err
	}
	if normalizeDNSName(got) != target {
		return fmt.Errorf("cname target mismatch: got %s, want %s", normalizeDNSName(got), target)
	}
	return nil
}

func normalizeDNSName(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}
