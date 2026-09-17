package domain

import "strings"

// maxIdempotencyKeyLength bounds retry keys. Real keys (request ids,
// UUIDs) stay far below it, and the bound keeps the wallet reference built
// from the key within its own limit.
const maxIdempotencyKeyLength = 128

// IdempotencyKey is the client attempt key of one publication. The same key
// is accepted exactly once per author: a retry resolves the recorded
// argument instead of debiting INK again.
type IdempotencyKey struct {
	value string
}

// ParseIdempotencyKey validates and trims an idempotency key. It accepts
// printable ASCII only so stored keys stay stable identifiers.
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

// WalletReference builds the INK operation reference of this attempt. It
// stays within the wallet reference bounds because the key is bounded.
func (k IdempotencyKey) WalletReference() string {
	return "argument:" + k.value
}
