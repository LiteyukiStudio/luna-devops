package runtimecluster

import (
	"reflect"
	"testing"
)

func TestGatewayDomainSuffixesNormalizeAndDefault(t *testing.T) {
	got := NormalizeGatewayDomainSuffixes([]string{" Apps.Example.Com. ", "internal.example.com", "apps.example.com"})
	want := []string{"apps.example.com", "internal.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeGatewayDomainSuffixes() = %#v, want %#v", got, want)
	}
	if got := DecodeGatewayDomainSuffixes(""); !reflect.DeepEqual(got, []string{DefaultGatewayDomainSuffix}) {
		t.Fatalf("DecodeGatewayDomainSuffixes(empty) = %#v", got)
	}
}
