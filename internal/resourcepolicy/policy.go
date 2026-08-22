package resourcepolicy

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	DefaultCPURequestPercent    = 10
	DefaultMemoryRequestPercent = 25
	DefaultCPULimitPercent      = 100
	DefaultMemoryLimitPercent   = 100
)

var (
	ErrInvalidPolicy   = errors.New("runtime.resource_policy_invalid")
	ErrInvalidQuantity = errors.New("runtime.resource_quantity_invalid")
)

type Policy struct {
	CPURequestPercent    int `json:"cpuRequestPercent"`
	MemoryRequestPercent int `json:"memoryRequestPercent"`
	CPULimitPercent      int `json:"cpuLimitPercent"`
	MemoryLimitPercent   int `json:"memoryLimitPercent"`
}

type EffectiveResources struct {
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
}

func Default() Policy {
	return Policy{
		CPURequestPercent: DefaultCPURequestPercent, MemoryRequestPercent: DefaultMemoryRequestPercent,
		CPULimitPercent: DefaultCPULimitPercent, MemoryLimitPercent: DefaultMemoryLimitPercent,
	}
}

func (p Policy) Validate() error {
	for name, value := range map[string]int{
		"cpuRequestPercent": p.CPURequestPercent, "memoryRequestPercent": p.MemoryRequestPercent,
		"cpuLimitPercent": p.CPULimitPercent, "memoryLimitPercent": p.MemoryLimitPercent,
	} {
		if value < 0 || value > 100 {
			return fmt.Errorf("%w: %s must be between 0 and 100", ErrInvalidPolicy, name)
		}
	}
	if p.CPULimitPercent > 0 && p.CPURequestPercent > p.CPULimitPercent {
		return fmt.Errorf("%w: cpu request percent cannot exceed enabled cpu limit percent", ErrInvalidPolicy)
	}
	if p.MemoryLimitPercent > 0 && p.MemoryRequestPercent > p.MemoryLimitPercent {
		return fmt.Errorf("%w: memory request percent cannot exceed enabled memory limit percent", ErrInvalidPolicy)
	}
	return nil
}

func Calculate(cpuQuota, memoryQuota string, policy Policy) (EffectiveResources, error) {
	if err := policy.Validate(); err != nil {
		return EffectiveResources{}, err
	}
	cpu, err := positiveQuantity(cpuQuota, "cpu quota")
	if err != nil {
		return EffectiveResources{}, err
	}
	memory, err := positiveQuantity(memoryQuota, "memory quota")
	if err != nil {
		return EffectiveResources{}, err
	}
	return EffectiveResources{
		CPURequest:    scaleCPU(cpu, policy.CPURequestPercent),
		MemoryRequest: scaleMemory(memory, policy.MemoryRequestPercent),
		CPULimit:      scaleCPU(cpu, policy.CPULimitPercent),
		MemoryLimit:   scaleMemory(memory, policy.MemoryLimitPercent),
	}, nil
}

func positiveQuantity(value, field string) (resource.Quantity, error) {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(value))
	if err != nil || quantity.Sign() <= 0 {
		if err == nil {
			err = errors.New("quantity must be positive")
		}
		return resource.Quantity{}, fmt.Errorf("%w: invalid %s: %v", ErrInvalidQuantity, field, err)
	}
	return quantity, nil
}

func scaleCPU(quantity resource.Quantity, percent int) string {
	if percent == 0 {
		return ""
	}
	milli := ceilRatio(big.NewInt(quantity.MilliValue()), int64(percent), 100)
	if milli.Sign() <= 0 {
		milli.SetInt64(1)
	}
	return resource.NewMilliQuantity(milli.Int64(), resource.DecimalSI).String()
}

func scaleMemory(quantity resource.Quantity, percent int) string {
	if percent == 0 {
		return ""
	}
	bytes := ceilRatio(big.NewInt(quantity.Value()), int64(percent), 100)
	if bytes.Sign() <= 0 {
		bytes.SetInt64(1)
	}
	return resource.NewQuantity(bytes.Int64(), resource.BinarySI).String()
}

func ceilRatio(value *big.Int, numerator, denominator int64) *big.Int {
	product := new(big.Int).Mul(value, big.NewInt(numerator))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(product, big.NewInt(denominator), remainder)
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}
