package domain

import "strings"

// maxIdempotencyKeyLength bounds retry keys; real keys (request ids, Stripe
// event ids, billing period references) stay far below this.
const maxIdempotencyKeyLength = 200

// IdempotencyKey is an immutable retry key of a logical ledger operation.
// The same key is accepted exactly once: retries resolve the original
// operation instead of duplicating it, so the key is a stable identifier
// (never prose) with printable ASCII characters only.
type IdempotencyKey struct {
	value string
}

// ParseIdempotencyKey validates and trims an idempotency key.
func ParseIdempotencyKey(raw string) (IdempotencyKey, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return IdempotencyKey{}, ErrEmptyIdempotencyKey
	}
	if len(trimmed) > maxIdempotencyKeyLength {
		return IdempotencyKey{}, ErrIdempotencyKeyTooLong
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] < 0x21 || trimmed[i] > 0x7e {
			return IdempotencyKey{}, ErrInvalidIdempotencyKey
		}
	}
	return IdempotencyKey{value: trimmed}, nil
}

// String returns the stored key.
func (k IdempotencyKey) String() string {
	return k.value
}

// IsZero reports whether the IdempotencyKey is the uninitialized zero value.
func (k IdempotencyKey) IsZero() bool {
	return k.value == ""
}

// Equals reports whether two keys are identical.
func (k IdempotencyKey) Equals(other IdempotencyKey) bool {
	return k.value == other.value
}
