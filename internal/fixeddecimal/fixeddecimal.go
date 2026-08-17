package fixeddecimal

import (
	"errors"
	"regexp"

	"github.com/shopspring/decimal"
)

var pattern = regexp.MustCompile(`^\d+(?:\.\d{1,8})?$`)

var Numeric24Scale8Max = decimal.RequireFromString("9999999999999999.99999999")

// Parse validates the canonical API/storage form used by numeric(24,8).
// Exponents, signs, whitespace, and more than eight fractional digits are rejected.
func Parse(raw string, allowZero bool, maximum decimal.Decimal) (decimal.Decimal, error) {
	if !pattern.MatchString(raw) {
		return decimal.Zero, errors.New("invalid fixed decimal")
	}
	value, err := decimal.NewFromString(raw)
	if err != nil || value.IsNegative() || (!allowZero && !value.IsPositive()) || value.GreaterThan(maximum) {
		return decimal.Zero, errors.New("fixed decimal is outside the allowed range")
	}
	return value, nil
}
