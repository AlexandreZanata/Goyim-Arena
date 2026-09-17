package domain

// AllocationLine is one bucket consumption inside an allocation plan.
type AllocationLine struct {
	Bucket Bucket
	Amount Ink
}

// Allocation is the immutable consumption plan of a debit across the two
// buckets. It fixes the mandatory priority of docs/MONETIZATION.md §2.2: the
// plan franchise (FREE_INK) is consumed before any purchased INK.
type Allocation struct {
	fromFree      Ink
	fromPurchased Ink
}

// NewAllocation builds an allocation from explicit bucket amounts. It is
// used to reconstruct the plan of an already stored debit.
func NewAllocation(fromFree, fromPurchased Ink) Allocation {
	return Allocation{fromFree: fromFree, fromPurchased: fromPurchased}
}

// AllocateDebit computes the consumption plan of a debit of amount given the
// available balances. FREE_INK is drained first; PURCHASED_INK covers the
// remainder. An amount above the total balance fails with ErrInsufficientInk
// so no partial debit is ever planned.
func AllocateDebit(amount, availableFree, availablePurchased Ink) (Allocation, error) {
	if amount.IsZero() {
		return Allocation{}, ErrZeroAmount
	}

	// Fast path: the franchise alone covers the debit. It is checked before
	// summing so the total can never overflow here.
	if amount.Int64() <= availableFree.Int64() {
		return Allocation{fromFree: amount}, nil
	}

	total, err := availableFree.Add(availablePurchased)
	if err != nil {
		return Allocation{}, err
	}
	if amount.Int64() > total.Int64() {
		return Allocation{}, ErrInsufficientInk
	}

	remaining, err := amount.Sub(availableFree)
	if err != nil {
		return Allocation{}, err
	}
	return Allocation{fromFree: availableFree, fromPurchased: remaining}, nil
}

// FromFree returns how much the plan consumes from the FREE_INK bucket.
func (a Allocation) FromFree() Ink {
	return a.fromFree
}

// FromPurchased returns how much the plan consumes from the PURCHASED_INK
// bucket.
func (a Allocation) FromPurchased() Ink {
	return a.fromPurchased
}

// Total returns the exact debited total.
func (a Allocation) Total() (Ink, error) {
	return a.fromFree.Add(a.fromPurchased)
}

// Lines returns the non-empty consumption lines in priority order (FREE_INK
// before PURCHASED_INK), ready for the ledger.
func (a Allocation) Lines() []AllocationLine {
	lines := make([]AllocationLine, 0, 2)
	if !a.fromFree.IsZero() {
		lines = append(lines, AllocationLine{Bucket: BucketFree, Amount: a.fromFree})
	}
	if !a.fromPurchased.IsZero() {
		lines = append(lines, AllocationLine{Bucket: BucketPurchased, Amount: a.fromPurchased})
	}
	return lines
}

// IsZero reports whether the plan consumes nothing.
func (a Allocation) IsZero() bool {
	return a.fromFree.IsZero() && a.fromPurchased.IsZero()
}

// Equals reports whether two plans consume the same amounts.
func (a Allocation) Equals(other Allocation) bool {
	return a.fromFree.Equals(other.fromFree) && a.fromPurchased.Equals(other.fromPurchased)
}
