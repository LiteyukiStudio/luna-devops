package resourcepolicy

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCalculateResourcePolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy Policy
		want   EffectiveResources
	}{
		{name: "defaults", policy: Default(), want: EffectiveResources{CPURequest: "100m", MemoryRequest: "256Mi", CPULimit: "1", MemoryLimit: "1Gi"}},
		{name: "all disabled", policy: Policy{}, want: EffectiveResources{}},
		{name: "request only", policy: Policy{CPURequestPercent: 20, MemoryRequestPercent: 30}, want: EffectiveResources{CPURequest: "200m", MemoryRequest: "322122548"}},
		{name: "limit only", policy: Policy{CPULimitPercent: 50, MemoryLimitPercent: 50}, want: EffectiveResources{CPULimit: "500m", MemoryLimit: "512Mi"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Calculate("1", "1Gi", test.policy)
			if err != nil || got != test.want {
				t.Fatalf("Calculate() = %#v, %v; want %#v", got, err, test.want)
			}
			for _, value := range []string{got.CPURequest, got.MemoryRequest, got.CPULimit, got.MemoryLimit} {
				if value != "" {
					if _, err := resource.ParseQuantity(value); err != nil {
						t.Fatalf("quantity %q is invalid: %v", value, err)
					}
				}
			}
		})
	}
}

func TestCalculateRoundsSmallPositiveValuesUp(t *testing.T) {
	got, err := Calculate("0.0001", "1", Policy{CPURequestPercent: 1, MemoryRequestPercent: 1})
	if err != nil || got.CPURequest != "1m" || got.MemoryRequest != "1" {
		t.Fatalf("Calculate() = %#v, %v", got, err)
	}
}

func TestPolicyValidation(t *testing.T) {
	if err := (Policy{CPURequestPercent: 51, CPULimitPercent: 50}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := (Policy{CPURequestPercent: 50, CPULimitPercent: 0}).Validate(); err != nil {
		t.Fatalf("request-only policy rejected: %v", err)
	}
	if _, err := Calculate("invalid", "1Gi", Default()); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("Calculate() error = %v", err)
	}
}
