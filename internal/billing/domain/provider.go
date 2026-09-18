package domain

import "strings"

// Provider vocabulary (P12-T03).
//
// The gateway port speaks about provider objects without ever carrying a
// provider type: identifiers become opaque value objects and statuses become
// closed vocabularies. The shapes below mirror migration 00020 line by line,
// so a value accepted here is a value the database accepts and vice versa;
// a provider answer outside these shapes is a contract violation and is
// refused instead of being stored.
//
// These identifiers are private: they name objects in the payment provider,
// never appear in a public projection, and are never logged.

// maxProviderIDBodyLength mirrors the database CHECK constraints exactly:
// every provider identifier is a prefix followed by one to 194 alphanumerics.
const maxProviderIDBodyLength = 194

// StripeCustomerID is the provider customer that carries one account.
type StripeCustomerID string

// StripeCheckoutSessionID is a hosted checkout session. Its prefix encodes
// the provider mode (cs_test_ / cs_live_), so it can never be confused
// between test and live: test and live objects are never mixed.
type StripeCheckoutSessionID string

// StripePaymentIntentID is the payment attempt behind a settled or open
// checkout, kept for refund and dispute correlation. The empty value is
// valid and means the provider has not created one yet.
type StripePaymentIntentID string

// StripeSubscriptionID is a provider subscription, the anchor of the Member
// entitlement of one billed period.
type StripeSubscriptionID string

// parsePrefixedProviderID validates the common shape of provider identifiers:
// an exact prefix followed by one to 194 alphanumerics.
func parsePrefixedProviderID(raw, prefix string) (string, bool) {
	remainder, found := strings.CutPrefix(raw, prefix)
	if !found || remainder == "" || len(remainder) > maxProviderIDBodyLength {
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

// ParseStripeCustomerID validates a provider customer identifier (cus_...).
func ParseStripeCustomerID(raw string) (StripeCustomerID, error) {
	if _, ok := parsePrefixedProviderID(raw, "cus_"); !ok {
		return "", ErrInvalidStripeCustomerID
	}
	return StripeCustomerID(raw), nil
}

// IsZero reports whether the customer identifier is unset.
func (id StripeCustomerID) IsZero() bool { return id == "" }

// String returns the stored identifier.
func (id StripeCustomerID) String() string { return string(id) }

// ParseStripeCheckoutSessionID validates a checkout session identifier
// (cs_test_... or cs_live_...) against the mode the caller believes it is
// operating in. A session from the other mode is refused, so a test session
// can never be recorded as a live payment or the reverse.
func ParseStripeCheckoutSessionID(raw string, livemode bool) (StripeCheckoutSessionID, error) {
	prefix := "cs_test_"
	if livemode {
		prefix = "cs_live_"
	}
	if _, ok := parsePrefixedProviderID(raw, prefix); !ok {
		// Distinguish a malformed identifier from one that is well formed
		// but belongs to the other provider mode: the first is a contract
		// violation, the second a mode mix-up between our records and the
		// provider.
		if _, otherMode := parsePrefixedProviderID(raw, otherSessionPrefix(livemode)); otherMode {
			return "", ErrStripeCheckoutSessionModeMismatch
		}
		return "", ErrInvalidStripeCheckoutSessionID
	}
	return StripeCheckoutSessionID(raw), nil
}

// otherSessionPrefix returns the session prefix of the opposite provider mode.
func otherSessionPrefix(livemode bool) string {
	if livemode {
		return "cs_test_"
	}
	return "cs_live_"
}

// IsZero reports whether the session identifier is unset.
func (id StripeCheckoutSessionID) IsZero() bool { return id == "" }

// String returns the stored identifier.
func (id StripeCheckoutSessionID) String() string { return string(id) }

// ParseStripePaymentIntentID validates a payment intent identifier (pi_...).
// The empty value is valid and means the provider has not created one: a
// subscription checkout never produces a payment intent.
func ParseStripePaymentIntentID(raw string) (StripePaymentIntentID, error) {
	if raw == "" {
		return "", nil
	}
	if _, ok := parsePrefixedProviderID(raw, "pi_"); !ok {
		return "", ErrInvalidStripePaymentIntentID
	}
	return StripePaymentIntentID(raw), nil
}

// IsZero reports whether the payment intent identifier is unset.
func (id StripePaymentIntentID) IsZero() bool { return id == "" }

// String returns the stored identifier.
func (id StripePaymentIntentID) String() string { return string(id) }

// ParseStripeSubscriptionID validates a provider subscription identifier
// (sub_...).
func ParseStripeSubscriptionID(raw string) (StripeSubscriptionID, error) {
	if _, ok := parsePrefixedProviderID(raw, "sub_"); !ok {
		return "", ErrInvalidStripeSubscriptionID
	}
	return StripeSubscriptionID(raw), nil
}

// IsZero reports whether the subscription identifier is unset.
func (id StripeSubscriptionID) IsZero() bool { return id == "" }

// String returns the stored identifier.
func (id StripeSubscriptionID) String() string { return string(id) }

// CheckoutSessionStatus is the provider status of a hosted checkout session:
// open while the buyer can pay, complete once the session was submitted,
// expired once it can no longer be paid.
type CheckoutSessionStatus string

const (
	// CheckoutStatusOpen is a session the buyer can still pay.
	CheckoutStatusOpen CheckoutSessionStatus = "open"

	// CheckoutStatusComplete is a session the buyer submitted. It does not
	// mean the payment settled: settlement is reported by the payment intent
	// and the webhook, never by the browser returning to the success page.
	CheckoutStatusComplete CheckoutSessionStatus = "complete"

	// CheckoutStatusExpired is a session that can no longer be paid.
	CheckoutStatusExpired CheckoutSessionStatus = "expired"
)

// AllCheckoutSessionStatuses returns the closed vocabulary in canonical order.
func AllCheckoutSessionStatuses() []CheckoutSessionStatus {
	return []CheckoutSessionStatus{CheckoutStatusOpen, CheckoutStatusComplete, CheckoutStatusExpired}
}

// ParseCheckoutSessionStatus validates a provider session status.
func ParseCheckoutSessionStatus(raw string) (CheckoutSessionStatus, error) {
	status := CheckoutSessionStatus(raw)
	if !status.IsValid() {
		return "", ErrInvalidCheckoutSessionStatus
	}
	return status, nil
}

// IsValid reports whether the status belongs to the closed vocabulary.
func (status CheckoutSessionStatus) IsValid() bool {
	switch status {
	case CheckoutStatusOpen, CheckoutStatusComplete, CheckoutStatusExpired:
		return true
	default:
		return false
	}
}

// String returns the provider vocabulary value.
func (status CheckoutSessionStatus) String() string { return string(status) }

// CheckoutPaymentStatus reports whether a checkout session was actually paid.
// It exists separately from CheckoutSessionStatus because a completed session
// is not a settled payment: the success page, the browser and the session
// status can never grant a benefit on their own (THR-STRIPE-02).
type CheckoutPaymentStatus string

const (
	// CheckoutPaymentPaid reports a settled payment.
	CheckoutPaymentPaid CheckoutPaymentStatus = "paid"

	// CheckoutPaymentUnpaid reports a session that was not paid.
	CheckoutPaymentUnpaid CheckoutPaymentStatus = "unpaid"

	// CheckoutPaymentNotRequired reports a session whose total is zero, so no
	// payment was ever collected.
	CheckoutPaymentNotRequired CheckoutPaymentStatus = "no_payment_required"
)

// AllCheckoutPaymentStatuses returns the closed vocabulary in canonical order.
func AllCheckoutPaymentStatuses() []CheckoutPaymentStatus {
	return []CheckoutPaymentStatus{CheckoutPaymentPaid, CheckoutPaymentUnpaid, CheckoutPaymentNotRequired}
}

// ParseCheckoutPaymentStatus validates a provider payment status.
func ParseCheckoutPaymentStatus(raw string) (CheckoutPaymentStatus, error) {
	status := CheckoutPaymentStatus(raw)
	if !status.IsValid() {
		return "", ErrInvalidCheckoutPaymentStatus
	}
	return status, nil
}

// IsValid reports whether the status belongs to the closed vocabulary.
func (status CheckoutPaymentStatus) IsValid() bool {
	switch status {
	case CheckoutPaymentPaid, CheckoutPaymentUnpaid, CheckoutPaymentNotRequired:
		return true
	default:
		return false
	}
}

// IsSettled reports whether the provider collected (or was not owed) the
// money. Only a settled session may ever produce an entitlement.
func (status CheckoutPaymentStatus) IsSettled() bool {
	return status == CheckoutPaymentPaid || status == CheckoutPaymentNotRequired
}

// String returns the provider vocabulary value.
func (status CheckoutPaymentStatus) String() string { return string(status) }

// SubscriptionStatus is the provider status of a subscription. The vocabulary
// is closed and mirrors migration 00020 exactly; the entitlement rules are
// applied by the use cases, never by this type.
type SubscriptionStatus string

const (
	// SubscriptionIncomplete awaits the first successful payment.
	SubscriptionIncomplete SubscriptionStatus = "incomplete"

	// SubscriptionIncompleteExpired never started and can never revive.
	SubscriptionIncompleteExpired SubscriptionStatus = "incomplete_expired"

	// SubscriptionTrialing is in a trial period.
	SubscriptionTrialing SubscriptionStatus = "trialing"

	// SubscriptionActive is paid and current.
	SubscriptionActive SubscriptionStatus = "active"

	// SubscriptionPastDue failed to collect the current period.
	SubscriptionPastDue SubscriptionStatus = "past_due"

	// SubscriptionCanceled ended and can never revive.
	SubscriptionCanceled SubscriptionStatus = "canceled"

	// SubscriptionUnpaid exhausted collection retries.
	SubscriptionUnpaid SubscriptionStatus = "unpaid"

	// SubscriptionPaused is temporarily not collecting.
	SubscriptionPaused SubscriptionStatus = "paused"
)

// AllSubscriptionStatuses returns the closed vocabulary in canonical order.
func AllSubscriptionStatuses() []SubscriptionStatus {
	return []SubscriptionStatus{
		SubscriptionIncomplete, SubscriptionIncompleteExpired, SubscriptionTrialing,
		SubscriptionActive, SubscriptionPastDue, SubscriptionCanceled,
		SubscriptionUnpaid, SubscriptionPaused,
	}
}

// ParseSubscriptionStatus validates a provider subscription status.
func ParseSubscriptionStatus(raw string) (SubscriptionStatus, error) {
	status := SubscriptionStatus(raw)
	if !status.IsValid() {
		return "", ErrInvalidSubscriptionStatus
	}
	return status, nil
}

// IsValid reports whether the status belongs to the closed vocabulary.
func (status SubscriptionStatus) IsValid() bool {
	switch status {
	case SubscriptionIncomplete, SubscriptionIncompleteExpired, SubscriptionTrialing,
		SubscriptionActive, SubscriptionPastDue, SubscriptionCanceled,
		SubscriptionUnpaid, SubscriptionPaused:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the status can never lead back to a live
// subscription. Terminal statuses are recorded, never overwritten by an
// out-of-order event that reports an earlier state.
func (status SubscriptionStatus) IsTerminal() bool {
	return status == SubscriptionCanceled || status == SubscriptionIncompleteExpired
}

// String returns the provider vocabulary value.
func (status SubscriptionStatus) String() string { return string(status) }

// CheckoutMode is the kind of purchase a hosted checkout collects: a one-off
// payment (INK packs and Arena Passes) or the first period of a subscription
// (Member).
type CheckoutMode string

const (
	// CheckoutModePayment collects a single payment.
	CheckoutModePayment CheckoutMode = "payment"

	// CheckoutModeSubscription starts a subscription.
	CheckoutModeSubscription CheckoutMode = "subscription"
)

// AllCheckoutModes returns the closed vocabulary in canonical order.
func AllCheckoutModes() []CheckoutMode {
	return []CheckoutMode{CheckoutModePayment, CheckoutModeSubscription}
}

// ParseCheckoutMode validates a checkout mode.
func ParseCheckoutMode(raw string) (CheckoutMode, error) {
	mode := CheckoutMode(raw)
	if !mode.IsValid() {
		return "", ErrInvalidCheckoutMode
	}
	return mode, nil
}

// IsValid reports whether the mode belongs to the closed vocabulary.
func (mode CheckoutMode) IsValid() bool {
	switch mode {
	case CheckoutModePayment, CheckoutModeSubscription:
		return true
	default:
		return false
	}
}

// String returns the provider vocabulary value.
func (mode CheckoutMode) String() string { return string(mode) }
