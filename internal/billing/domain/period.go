package domain

import "time"

// BillingPeriod is one billed period of a subscription: the exact interval the
// provider charges for. It is the anchor of the per-period Member franchise
// (P12-T08), so the interval is validated once here: a period that does not
// advance in time could never be billed twice, and a reversed or incomplete
// period would make the franchise ambiguous.
//
// The mirrored database constraint (migration 00020) requires the pair to be
// either both absent or both present with the end strictly after the start.
type BillingPeriod struct {
	start time.Time
	end   time.Time
}

// NewBillingPeriod builds a period from the provider's instants. Values are
// normalized to UTC, because provider timestamps are seconds since the epoch
// and must be comparable with the instants recorded elsewhere.
func NewBillingPeriod(start, end time.Time) (BillingPeriod, error) {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return BillingPeriod{}, ErrInvalidBillingPeriod
	}
	return BillingPeriod{start: start.UTC(), end: end.UTC()}, nil
}

// Start returns the inclusive start instant of the period.
func (p BillingPeriod) Start() time.Time { return p.start }

// End returns the exclusive end instant of the period.
func (p BillingPeriod) End() time.Time { return p.end }

// IsZero reports whether the period is unset, which is the valid state of a
// subscription the provider has not billed yet.
func (p BillingPeriod) IsZero() bool { return p.start.IsZero() && p.end.IsZero() }

// String renders the diagnostic form of the interval.
func (p BillingPeriod) String() string {
	return p.start.Format(time.RFC3339) + ".." + p.end.Format(time.RFC3339)
}
