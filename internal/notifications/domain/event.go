package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// EventKeyPrefix namespaces every delivery key, so a queue row says what kind
// of work it is without reading the payload.
const EventKeyPrefix = "email:"

// EventKey derives the stable enqueue key of one notification event.
//
// The key identifies the *event* that asks for an email, not the email: the
// one-time code is part of the digest, so issuing a fresh code (the user asked
// for another link) is a new event with a new key, while a retried request
// that carries the same code resolves the existing job instead of queueing a
// second message. The code itself never enters the key in the clear: only a
// truncated digest of the triple does, which is not invertible.
func EventKey(templateID TemplateID, recipient, code string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		templateID.String(),
		strings.ToLower(strings.TrimSpace(recipient)),
		code,
	}, "\x00")))
	return EventKeyPrefix + templateID.String() + ":" + hex.EncodeToString(sum[:16])
}

// ValidateEventKey reports whether a value is usable as an enqueue key: it is
// stored in the same column the queue validates, so it is checked here as
// well, and the alphabet keeps it readable in an operational query.
func ValidateEventKey(key string) error {
	if !strings.HasPrefix(key, EventKeyPrefix) || len(key) > maxIdempotencyKeyLength {
		return ErrInvalidIdempotencyKey
	}
	for _, char := range key {
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9':
			continue
		case strings.ContainsRune("._:-@", char):
			continue
		default:
			return ErrInvalidIdempotencyKey
		}
	}
	return nil
}

// DeliveryKey derives the provider-side idempotency key of one *attempt* to
// deliver a queued job.
//
// It is anchored on the job identifier and not on the message content,
// because the property that matters is different: a retry of the same job must
// reach the provider as the same logical send, while two distinct events that
// happen to render the same body (two codes, two recipients asking at the same
// time) must stay two sends. The job row is exactly that identity, and it is
// stable across every retry of it.
func DeliveryKey(jobID string) (string, error) {
	trimmed := strings.TrimSpace(jobID)
	if trimmed == "" || len(trimmed) > maxIdempotencyKeyLength {
		return "", ErrInvalidIdempotencyKey
	}
	key := "job:" + trimmed
	for _, char := range key {
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9':
			continue
		case strings.ContainsRune("._:-", char):
			continue
		default:
			// A queue identifier outside this alphabet would be rejected
			// by the message it keys, which would surface as a delivery
			// failure instead of a composition error.
			return "", ErrInvalidIdempotencyKey
		}
	}
	return key, nil
}
