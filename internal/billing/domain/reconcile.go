package domain

import "time"

// Reconciliation vocabulary (P12-T10).
//
// Reconciliation never corrects anything silently: it records divergences
// between the local mirrors and the provider as immutable findings for
// human review. Money and entitlements still move only through verified
// webhooks (THR-STRIPE-02).

// ReconciliationKind is the closed vocabulary of divergences. It mirrors
// the CHECK constraint of app.billing_reconciliation_findings exactly.
type ReconciliationKind string

const (
	// ReconciliationMissingLocal means the provider owns an object the
	// local mirrors do not record. It is reserved for list-based
	// comparisons; window reconciliation without a provider list never
	// synthesizes it.
	ReconciliationMissingLocal ReconciliationKind = "missing_local"
	// ReconciliationMissingRemote means a local intent or subscription
	// names a provider object the provider no longer knows.
	ReconciliationMissingRemote ReconciliationKind = "missing_remote"
	// ReconciliationAmountMismatch means the provider charges a different
	// minor-unit total than the server-authoritative intent.
	ReconciliationAmountMismatch ReconciliationKind = "amount_mismatch"
	// ReconciliationCurrencyMismatch means the provider charges a
	// different ISO currency than the intent.
	ReconciliationCurrencyMismatch ReconciliationKind = "currency_mismatch"
	// ReconciliationStatusMismatch means the local lifecycle and the
	// provider lifecycle disagree, including out-of-order webhook
	// deliveries.
	ReconciliationStatusMismatch ReconciliationKind = "status_mismatch"
	// ReconciliationUnprocessedEvent means a verified webhook event in the
	// window never reached a terminal outcome.
	ReconciliationUnprocessedEvent ReconciliationKind = "unprocessed_event"
)

// AllReconciliationKinds returns the closed vocabulary in canonical order.
func AllReconciliationKinds() []ReconciliationKind {
	return []ReconciliationKind{
		ReconciliationMissingLocal,
		ReconciliationMissingRemote,
		ReconciliationAmountMismatch,
		ReconciliationCurrencyMismatch,
		ReconciliationStatusMismatch,
		ReconciliationUnprocessedEvent,
	}
}

// ParseReconciliationKind validates a persisted kind against the exact
// vocabulary.
func ParseReconciliationKind(raw string) (ReconciliationKind, error) {
	kind := ReconciliationKind(raw)
	if !kind.IsValid() {
		return "", ErrInvalidReconciliationKind
	}
	return kind, nil
}

// IsValid reports whether the kind belongs to the closed vocabulary.
func (k ReconciliationKind) IsValid() bool {
	switch k {
	case ReconciliationMissingLocal, ReconciliationMissingRemote,
		ReconciliationAmountMismatch, ReconciliationCurrencyMismatch,
		ReconciliationStatusMismatch, ReconciliationUnprocessedEvent:
		return true
	default:
		return false
	}
}

// String returns the stored kind value.
func (k ReconciliationKind) String() string { return string(k) }

// ReconciliationWindow is the half-open UTC interval already inspected:
// [Start, End). The provider and the local mirrors are compared as of the
// window, never mutated.
type ReconciliationWindow struct {
	start time.Time
	end   time.Time
}

// NewReconciliationWindow builds a window with the end strictly after the
// start. Both instants are normalized to UTC.
func NewReconciliationWindow(start, end time.Time) (ReconciliationWindow, error) {
	if end.IsZero() || start.IsZero() {
		return ReconciliationWindow{}, ErrInvalidReconciliationWindow
	}
	startUTC := start.UTC()
	endUTC := end.UTC()
	if !endUTC.After(startUTC) {
		return ReconciliationWindow{}, ErrInvalidReconciliationWindow
	}
	return ReconciliationWindow{start: startUTC, end: endUTC}, nil
}

// Start returns the inclusive start of the window in UTC.
func (w ReconciliationWindow) Start() time.Time { return w.start }

// End returns the exclusive end of the window in UTC.
func (w ReconciliationWindow) End() time.Time { return w.end }

// IsZero reports whether the window is uninitialized.
func (w ReconciliationWindow) IsZero() bool { return w.start.IsZero() || w.end.IsZero() }
