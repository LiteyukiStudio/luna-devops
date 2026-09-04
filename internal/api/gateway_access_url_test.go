package api

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/api/gatewayapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
)

func TestGatewayRouteAccessURLUsesPublicScheme(t *testing.T) {
	route := model.GatewayRoute{Host: "app.example.com", Path: "/admin", TLSMode: "http-only"}

	if got := gatewayapi.GatewayRouteAccessURL(route, "https", 443); got != "https://app.example.com/admin" {
		t.Fatalf("access url = %q", got)
	}
}

func TestGatewayRouteAccessURLNormalizesPathAndScheme(t *testing.T) {
	route := model.GatewayRoute{Host: "app.example.com", Path: "admin"}

	if got := gatewayapi.GatewayRouteAccessURL(route, "ftp", 80); got != "http://app.example.com/admin" {
		t.Fatalf("access url = %q", got)
	}
}

func TestGatewayRouteAccessURLOmitsRootPath(t *testing.T) {
	route := model.GatewayRoute{Host: "app.example.com", Path: "/"}

	if got := gatewayapi.GatewayRouteAccessURL(route, "https", 443); got != "https://app.example.com" {
		t.Fatalf("access url = %q", got)
	}
}

func TestGatewayRouteAccessURLShowsNonStandardPublicPort(t *testing.T) {
	route := model.GatewayRoute{Host: "app.example.com", Path: "/"}

	if got := gatewayapi.GatewayRouteAccessURL(route, "https", 9443); got != "https://app.example.com:9443" {
		t.Fatalf("access url = %q", got)
	}
	if got := gatewayapi.GatewayRouteAccessURL(route, "http", 8080); got != "http://app.example.com:8080" {
		t.Fatalf("access url = %q", got)
	}
}

func TestNormalizeGatewayHostUsesFirstClusterDomainSuffix(t *testing.T) {
	h := &Handlers{}
	handler := gatewayapi.New(gatewayHost{domainHost: domainHost{handlers: h}})
	cluster := model.RuntimeCluster{GatewayDomainSuffixesRaw: "Apps.Example.Com."}

	if got := handler.NormalizeGatewayHost("demo", cluster, ""); got != "demo.apps.example.com" {
		t.Fatalf("host = %q", got)
	}
}

func TestNormalizeGatewayHostUsesSelectedDomainSuffix(t *testing.T) {
	h := &Handlers{}
	handler := gatewayapi.New(gatewayHost{domainHost: domainHost{handlers: h}})
	cluster := model.RuntimeCluster{GatewayDomainSuffixesRaw: "apps.example.com\ninternal.example.com"}

	if got := handler.NormalizeGatewayHost("demo", cluster, "internal.example.com"); got != "demo.internal.example.com" {
		t.Fatalf("host = %q", got)
	}
}

func TestNormalizeGatewayDomainSuffixesUsesExplicitValuesOnly(t *testing.T) {
	got := runtimecluster.DecodeGatewayDomainSuffixes("Apps.Example.Com.\ninternal.example.com\napps.example.com")
	want := []string{"apps.example.com", "internal.example.com"}
	if len(got) != len(want) {
		t.Fatalf("suffixes = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("suffixes = %#v", got)
		}
	}
}
