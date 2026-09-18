package domain

import "strings"

// Provider refund vocabulary (P12-T09).
//
// Refunds and chargebacks are explicit, audited facts: the provider reports
// money going back, the ledger records compensating entries and a shortfall
// never becomes a negative balance — it becomes a review flag for support
// and fraud handling (docs/MONETIZATION.md §2.3, docs/SECURITY.md §8,
// REQ-BIL-04).

// maxRefundIDBodyLength mirrors the database CHECK exactly: every refund,
// dispute and charge identifier is a prefix followed by one to 194
// alphanumerics.
const maxRefundIDBodyLength = 194

// StripeRefundID is a provider refund object (re_...). It is private: it is
// persisted for correlation and never leaves the billing module.
type StripeRefundID string

// StripeDisputeID is a provider dispute object (dp_...), the chargeback
// anchor. It is private like every provider identifier.
type StripeDisputeID string

// StripeChargeID is a provider charge object (ch_...), kept so a refund or a
// dispute can be correlated with the charge it reverses.
type StripeChargeID string

func parseRefundPrefixedID(raw, prefix string) (string, bool) {
	remainder, found := strings.CutPrefix(raw, prefix)
	if !found || remainder == "" || len(remainder) > maxRefundIDBodyLength {
		return "", false
	}
	for index := 0; index < len(remainder); index++ {
		character := remainder[index]
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		default:
			return "", false
		}
	}
	return remainder, true
}

// ParseStripeRefundID validates a provider refund identifier (re_...).
func ParseStripeRefundID(raw string) (StripeRefundID, error) {
	if _, ok := parseRefundPrefixedID(raw, "re_"); !ok {
		return "", ErrInvalidStripeRefundID
	}
	return StripeRefundID(raw), nil
}

// IsZero reports whether the refund identifier is unset.
func (id StripeRefundID) IsZero() bool { return id == "" }

// String returns the stored identifier.
func (id StripeRefundID) String() string { return string(id) }

// ParseStripeDisputeID validates a provider dispute identifier (dp_...).
func ParseStripeDisputeID(raw string) (StripeDisputeID, error) {
	if _, ok := parseRefundPrefixedID(raw, "dp_"); !ok {
		return "", ErrInvalidStripeDisputeID
	}
	return StripeDisputeID(raw), nil
}

// IsZero reports whether the dispute identifier is unset.
func (id StripeDisputeID) IsZero() bool { return id == "" }

// String returns the stored identifier.
func (id StripeDisputeID) String() string { return string(id) }

// ParseStripeChargeID validates a provider charge identifier (ch_...).
func ParseStripeChargeID(raw string) (StripeChargeID, error) {
	if _, ok := parseRefundPrefixedID(raw, "ch_"); !ok {
		return "", ErrInvalidStripeChargeID
	}
	return StripeChargeID(raw), nil
}

// IsZero reports whether the charge identifier is unset.
func (id StripeChargeID) IsZero() bool { return id == "" }

// String returns the stored identifier.
func (id StripeChargeID) String() string { return string(id) }

// RefundSource names which provider fact triggered the compensation: a
// voluntary refund or a contested chargeback (dispute). A chargeback always
// needs human review, even when the unused benefit could be withdrawn
// automatically.
type RefundSource string

const (
	// RefundSourceRefund is a provider refund (money returned).
	RefundSourceRefund RefundSource = "refund"
	// RefundSourceDispute is a provider dispute/chargeback (money contested).
	RefundSourceDispute RefundSource = "dispute"
)

// ParseRefundSource validates the refund source vocabulary.
func ParseRefundSource(raw string) (RefundSource, error) {
	source := RefundSource(raw)
	if !source.IsValid() {
		return "", ErrInvalidRefundSource
	}
	return source, nil
}

// IsValid reports whether the source belongs to the closed vocabulary.
func (s RefundSource) IsValid() bool {
	switch s {
	case RefundSourceRefund, RefundSourceDispute:
		return true
	default:
		return false
	}
}

// String returns the stored source value.
func (s RefundSource) String() string { return string(s) }

// IsDispute reports whether the source is a contested chargeback.
func (s RefundSource) IsDispute() bool { return s == RefundSourceDispute }

// RefundStatus is the explicit outcome of one refund record. There is no
// silent mutation: either the unused benefit was withdrawn (applied) or a
// human must decide (needs_review).
type RefundStatus string

const (
	// RefundStatusApplied means the compensating entries covered the whole
	// reversible benefit and no review is pending.
	RefundStatusApplied RefundStatus = "applied"
	// RefundStatusNeedsReview means support/fraud must decide: part of the
	// benefit was already consumed, the refund was partial over passes, the
	// source was a chargeback, or nothing reversible remained.
	RefundStatusNeedsReview RefundStatus = "needs_review"
)

// ParseRefundStatus validates the refund status vocabulary.
func ParseRefundStatus(raw string) (RefundStatus, error) {
	status := RefundStatus(raw)
	if !status.IsValid() {
		return "", ErrInvalidRefundStatus
	}
	return status, nil
}

// IsValid reports whether the status belongs to the closed vocabulary.
func (s RefundStatus) IsValid() bool {
	switch s {
	case RefundStatusApplied, RefundStatusNeedsReview:
		return true
	default:
		return false
	}
}

// String returns the stored status value.
func (s RefundStatus) String() string { return string(s) }

// NeedsReview reports whether the status waits for a human decision.
func (s RefundStatus) NeedsReview() bool { return s == RefundStatusNeedsReview }

// AssessINK decides the compensating debit for a purchased INK grant.
//
//   - granted is the INK quantity the catalog granted for the intent.
//   - paidMinor is the price the server charged (minor units, >0).
//   - refundedMinor is the money the provider returned (minor units, >0).
//   - available is the current PURCHASED_INK balance that can be withdrawn.
//   - source distinguishes a refund from a chargeback.
//
// The reversible INK is the granted quantity prorated by the refunded share
// of the price, using integer arithmetic only (never float): a full refund
// reverses the whole grant, a partial refund reverses the proportional share
// rounded down (at least one unit when anything was refunded). The debit is
// capped at the available balance so good-faith consumption never produces a
// negative balance: a shortfall, a dispute source or an empty debit becomes
// needsReview instead of a silent overdraft.
func AssessINK(granted, paidMinor, refundedMinor, available int64, source RefundSource) (debit int64, needsReview bool, err error) {
	if granted < 1 {
		return 0, false, ErrInvalidGrant
	}
	if paidMinor < 1 {
		return 0, false, ErrInvalidMoney
	}
	if refundedMinor < 1 {
		return 0, false, ErrInvalidRefundAmount
	}
	if refundedMinor > paidMinor {
		return 0, false, ErrInvalidRefundAmount
	}
	if available < 0 {
		return 0, false, ErrInvalidRefundAssessment
	}
	if !source.IsValid() {
		return 0, false, ErrInvalidRefundSource
	}

	var reversible int64
	if refundedMinor >= paidMinor {
		reversible = granted
	} else {
		reversible = granted * refundedMinor / paidMinor
		if reversible < 1 {
			reversible = 1
		}
	}

	if reversible > available {
		return available, true, nil
	}
	if source.IsDispute() {
		return reversible, true, nil
	}
	if reversible == 0 {
		return 0, true, nil
	}
	return reversible, false, nil
}

// AssessPasses decides the compensating revocation for a purchased Arena Pass
// lot.
//
//   - granted is the pass quantity the catalog granted.
//   - remaining is what the lot still holds (0 when fully consumed).
//   - partial reports whether the money refund covered only part of the price:
//     passes are discrete, so a partial money refund never revokes passes
//     automatically — it always waits for a human.
//   - source distinguishes a refund from a chargeback.
//
// A full refund revokes every remaining pass; any consumed pass, any partial
// refund and any dispute becomes needsReview instead of a silent rewrite of
// history. Consumption rows are never deleted: revocation only zeroes the
// remaining projection.
func AssessPasses(granted, remaining int32, partial bool, source RefundSource) (revoke int32, needsReview bool, err error) {
	if granted < 1 {
		return 0, false, ErrInvalidQuantity
	}
	if remaining < 0 || remaining > granted {
		return 0, false, ErrInvalidRemaining
	}
	if !source.IsValid() {
		return 0, false, ErrInvalidRefundSource
	}

	if partial {
		return 0, true, nil
	}
	if source.IsDispute() {
		return remaining, true, nil
	}
	if remaining < granted {
		return remaining, true, nil
	}
	return remaining, false, nil
}
