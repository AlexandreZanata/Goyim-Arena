// Package domain defines the value objects, entities, policies and errors of
// the billing module. It uses only the Go standard library and must not
// import any adapter, HTTP, database or provider package.
//
// Webhook vocabulary (P12-T05): the verified event identifier and the
// provider-specific event type, independent of transport and persistence.
package domain

import (
	"regexp"
	"strings"
)

// stripeEventIDPattern matches the Stripe event identifier: the prefix evt_
// followed by 1 to 194 alphanumeric characters. This mirrors the CHECK
// constraint of migration 00020 on app.stripe_events.stripe_event_id.
var stripeEventIDPattern = regexp.MustCompile(`^evt_[A-Za-z0-9]{1,194}$`)

// maxStripeEventTypeLength is the schema bound for the event type string.
const maxStripeEventTypeLength = 120

// WebhookEventID is the opaque, unique provider event identifier (evt_...).
// It is the idempotency anchor of the webhook: two deliveries of the same
// event resolve the same database row instead of creating two.
type WebhookEventID struct {
	value string
}

// ParseWebhookEventID validates and stores a provider event identifier.
func ParseWebhookEventID(raw string) (WebhookEventID, error) {
	if raw == "" {
		return WebhookEventID{}, ErrEmptyWebhookEventID
	}
	if !stripeEventIDPattern.MatchString(raw) {
		return WebhookEventID{}, ErrInvalidWebhookEventID
	}
	return WebhookEventID{value: raw}, nil
}

// String returns the stored event identifier.
func (id WebhookEventID) String() string { return id.value }

// IsZero reports whether the identifier is the uninitialized zero value.
func (id WebhookEventID) IsZero() bool { return id.value == "" }

// WebhookEventType is the Stripe event type string (e.g.,
// "checkout.session.completed"). It is a plain string validated against the
// schema CHECK: lowercase letters, digits and underscores, dot-separated
// segments, at most 120 characters.
type WebhookEventType struct {
	value string
}

// ParseWebhookEventType validates and stores a provider event type.
func ParseWebhookEventType(raw string) (WebhookEventType, error) {
	if raw == "" {
		return WebhookEventType{}, ErrEmptyWebhookEventType
	}
	if len(raw) > maxStripeEventTypeLength {
		return WebhookEventType{}, ErrInvalidWebhookEventType
	}
	// Validate the shape: lowercase letter start, segments of lowercase
	// letters/digits/underscores separated by dots. No consecutive dots,
	// no trailing dot, no leading dot.
	if raw[0] == '.' || raw[len(raw)-1] == '.' || strings.Contains(raw, "..") {
		return WebhookEventType{}, ErrInvalidWebhookEventType
	}
	segmentPattern := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	segments := splitEventType(raw)
	if len(segments) == 0 {
		return WebhookEventType{}, ErrInvalidWebhookEventType
	}
	for _, seg := range segments {
		if !segmentPattern.MatchString(seg) {
			return WebhookEventType{}, ErrInvalidWebhookEventType
		}
	}
	return WebhookEventType{value: raw}, nil
}

// splitEventType splits a dot-separated event type into segments. Empty
// segments (from consecutive dots or trailing dots) are not included.
func splitEventType(raw string) []string {
	var segments []string
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '.' {
			if i > start {
				segments = append(segments, raw[start:i])
			}
			start = i + 1
		}
	}
	if start < len(raw) {
		segments = append(segments, raw[start:])
	}
	return segments
}

// String returns the stored event type.
func (t WebhookEventType) String() string { return t.value }

// IsZero reports whether the type is the uninitialized zero value.
func (t WebhookEventType) IsZero() bool { return t.value == "" }

// IsCheckoutSessionCompleted reports whether this event type represents a
// checkout session that was completed by the provider. This is the only
// event type that may settle a checkout intent.
func (t WebhookEventType) IsCheckoutSessionCompleted() bool {
	return t.value == "checkout.session.completed"
}

// IsCheckoutSessionExpired reports whether this event type represents a
// checkout session that expired without payment.
func (t WebhookEventType) IsCheckoutSessionExpired() bool {
	return t.value == "checkout.session.expired"
}

// IsSubscriptionEvent reports whether this event type concerns a subscription
// lifecycle change.
func (t WebhookEventType) IsSubscriptionEvent() bool {
	return len(t.value) > 22 && t.value[:22] == "customer.subscription."
}
