package domain

import "strings"

// maxReferenceLength bounds operation references; every real reference
// (argument UUID, Stripe event, moderation case, billing period) stays far
// below this.
const maxReferenceLength = 200

// Reference is an immutable, non-empty identifier of the cause of an INK
// operation: the argument being paid for, the Stripe event that credited a
// purchase, the moderation case behind a refund or the billing period of a
// grant. It is opaque to the domain and never carries user-facing prose.
type Reference struct {
	value string
}

// ParseReference validates and trims an operation reference. It accepts
// printable ASCII only: control characters, newlines, spacing and non-ASCII
// payloads are rejected so stored references stay stable identifiers.
func ParseReference(raw string) (Reference, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Reference{}, ErrEmptyReference
	}
	if len(trimmed) > maxReferenceLength {
		return Reference{}, ErrReferenceTooLong
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] < 0x21 || trimmed[i] > 0x7e {
			return Reference{}, ErrInvalidReference
		}
	}
	return Reference{value: trimmed}, nil
}

// String returns the stored reference.
func (r Reference) String() string {
	return r.value
}

// IsZero reports whether the Reference is the uninitialized zero value.
func (r Reference) IsZero() bool {
	return r.value == ""
}

// Equals reports whether two references are identical.
func (r Reference) Equals(other Reference) bool {
	return r.value == other.value
}
