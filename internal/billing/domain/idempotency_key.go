package domain

import "strings"

// maxIdempotencyKeyLength bounds retry keys. Real keys (a browser operation
// token, a request identifier) stay far below this, and the bound keeps the
// key the adapter derives for the provider inside the provider's own limit.
const maxIdempotencyKeyLength = 200

// IdempotencyKey is the caller-chosen key of one checkout attempt. It is what
// makes a retried attempt harmless: the same key must always describe the same
// logical purchase, so the provider returns the session it already created
// instead of a second one.
//
// The key is opaque to the domain and is never derived from content the caller
// can vary freely: it belongs to the caller (the browser operation token the
// HTTP adapter receives), and it is deliberately independent from the amount,
// the product and the account, which the server resolves on its own.
type IdempotencyKey struct {
	value string
}

// ParseIdempotencyKey validates and trims a retry key. Printable ASCII only,
// so the key travels safely in provider headers and audit lines.
func ParseIdempotencyKey(raw string) (IdempotencyKey, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return IdempotencyKey{}, ErrEmptyIdempotencyKey
	}
	if len(trimmed) > maxIdempotencyKeyLength {
		return IdempotencyKey{}, ErrIdempotencyKeyTooLong
	}
	for index := 0; index < len(trimmed); index++ {
		if trimmed[index] < 0x21 || trimmed[index] > 0x7e {
			return IdempotencyKey{}, ErrInvalidIdempotencyKey
		}
	}
	return IdempotencyKey{value: trimmed}, nil
}

// String returns the stored key.
func (key IdempotencyKey) String() string {
	return key.value
}

// IsZero reports whether the key is the uninitialized zero value.
func (key IdempotencyKey) IsZero() bool {
	return key.value == ""
}

// Equals reports whether two keys are identical.
func (key IdempotencyKey) Equals(other IdempotencyKey) bool {
	return key.value == other.value
}
