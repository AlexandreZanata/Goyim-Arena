package domain

import (
	"errors"
	"math"
	"strconv"
)

// Ink is an immutable, non-negative INK quantity. INK is an internal
// consumption unit (docs/MONETIZATION.md §2), never money and never a float:
// every quantity is an exact signed 64-bit integer, matching the bigint
// columns of the append-only ledger (migration 00008).
type Ink struct {
	amount int64
}

// NewInk builds an INK quantity from an int64. Negative values are invalid:
// debits are expressed by the operation direction, not by a negative
// quantity.
func NewInk(amount int64) (Ink, error) {
	if amount < 0 {
		return Ink{}, ErrNegativeInk
	}
	return Ink{amount: amount}, nil
}

// ParseInk parses a decimal INK quantity. Only ASCII digits are accepted:
// signs, whitespace, decimal separators, exponents and every other float
// notation are rejected, so a floating-point value can never enter the
// ledger. Values beyond the 64-bit range fail with ErrInkOverflow.
func ParseInk(raw string) (Ink, error) {
	if raw == "" {
		return Ink{}, ErrInvalidInk
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return Ink{}, ErrInvalidInk
		}
	}

	amount, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return Ink{}, ErrInkOverflow
		}
		return Ink{}, ErrInvalidInk
	}
	return Ink{amount: amount}, nil
}

// Add returns the exact sum of two quantities; crossing the 64-bit ceiling
// fails instead of wrapping.
func (i Ink) Add(other Ink) (Ink, error) {
	if other.amount > math.MaxInt64-i.amount {
		return Ink{}, ErrInkOverflow
	}
	return Ink{amount: i.amount + other.amount}, nil
}

// Sub returns the exact difference; it fails instead of producing a
// negative quantity (no overdraft exists in the ledger).
func (i Ink) Sub(other Ink) (Ink, error) {
	if other.amount > i.amount {
		return Ink{}, ErrInsufficientInk
	}
	return Ink{amount: i.amount - other.amount}, nil
}

// Int64 returns the exact quantity. INK never passes through float64.
func (i Ink) Int64() int64 {
	return i.amount
}

// String renders the canonical decimal form.
func (i Ink) String() string {
	return strconv.FormatInt(i.amount, 10)
}

// IsZero reports whether the quantity is exactly zero INK.
func (i Ink) IsZero() bool {
	return i.amount == 0
}

// Equals reports whether two quantities are exactly equal.
func (i Ink) Equals(other Ink) bool {
	return i.amount == other.amount
}
