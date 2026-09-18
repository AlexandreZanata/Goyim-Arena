package domain

// CheckoutIntentStatus is the local lifecycle of a checkout intent, the
// server-side record of one commercial decision (migration 00020).
//
// It is deliberately distinct from CheckoutSessionStatus, which is the
// provider's own view of a hosted session: the provider says whether a session
// can still be paid, while this vocabulary says what happened to *our*
// intention, and only a verified webhook settles one (THR-STRIPE-02).
type CheckoutIntentStatus string

const (
	// CheckoutIntentCreated is the decision recorded before any provider
	// session exists. A created intent never has a session identifier.
	CheckoutIntentCreated CheckoutIntentStatus = "created"

	// CheckoutIntentOpen is an intent with a provider session the buyer can
	// still pay. The session identifier is always present.
	CheckoutIntentOpen CheckoutIntentStatus = "open"

	// CheckoutIntentPaid is a settled intent: set exclusively from a verified
	// provider webhook, never from a browser visit, and dated.
	CheckoutIntentPaid CheckoutIntentStatus = "paid"

	// CheckoutIntentExpired is an intent whose session can no longer be paid,
	// closed at a known instant.
	CheckoutIntentExpired CheckoutIntentStatus = "expired"

	// CheckoutIntentFailed is an intent the provider refused or that was
	// abandoned, closed at a known instant and possibly without a session.
	CheckoutIntentFailed CheckoutIntentStatus = "failed"
)

// AllCheckoutIntentStatuses returns the closed vocabulary in canonical order.
func AllCheckoutIntentStatuses() []CheckoutIntentStatus {
	return []CheckoutIntentStatus{
		CheckoutIntentCreated, CheckoutIntentOpen, CheckoutIntentPaid,
		CheckoutIntentExpired, CheckoutIntentFailed,
	}
}

// ParseCheckoutIntentStatus validates a persisted or requested status.
func ParseCheckoutIntentStatus(raw string) (CheckoutIntentStatus, error) {
	status := CheckoutIntentStatus(raw)
	if !status.IsValid() {
		return "", ErrInvalidCheckoutIntentStatus
	}
	return status, nil
}

// IsValid reports whether the status belongs to the closed vocabulary.
func (status CheckoutIntentStatus) IsValid() bool {
	switch status {
	case CheckoutIntentCreated, CheckoutIntentOpen, CheckoutIntentPaid,
		CheckoutIntentExpired, CheckoutIntentFailed:
		return true
	default:
		return false
	}
}

// IsSettled reports whether the provider confirmed the payment. Only a settled
// intent may ever produce an entitlement.
func (status CheckoutIntentStatus) IsSettled() bool {
	return status == CheckoutIntentPaid
}

// IsTerminal reports whether the status can never change again. It mirrors the
// transition table enforced by the database trigger: settled, expired and
// failed intents never revive, even under a delayed provider event.
func (status CheckoutIntentStatus) IsTerminal() bool {
	return status == CheckoutIntentPaid || status == CheckoutIntentExpired || status == CheckoutIntentFailed
}

// String returns the stored vocabulary value.
func (status CheckoutIntentStatus) String() string { return string(status) }
