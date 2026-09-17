package domain

import "strconv"

// Quantity is an immutable pass count. Quantities are positive integers:
// zero-pass grants are meaningless and negative quantities are impossible.
type Quantity struct {
	amount int32
}

// NewQuantity builds a quantity from an int32; values below one are refused.
func NewQuantity(amount int32) (Quantity, error) {
	if amount < 1 {
		return Quantity{}, ErrInvalidQuantity
	}
	return Quantity{amount: amount}, nil
}

// Int32 returns the exact count.
func (q Quantity) Int32() int32 {
	return q.amount
}

// String renders the decimal count.
func (q Quantity) String() string {
	return strconv.FormatInt(int64(q.amount), 10)
}

// IsZero reports whether the quantity is the uninitialized zero value.
func (q Quantity) IsZero() bool {
	return q.amount == 0
}

// Equals reports whether two quantities are exactly equal.
func (q Quantity) Equals(other Quantity) bool {
	return q.amount == other.amount
}
