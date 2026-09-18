package domain

import (
	"strings"
	"time"
)

// Target types admitted by the trail. The set mirrors the CHECK constraint
// of app.audit_events (migration 00025) so a value accepted here is a value
// the database accepts.
var allowedTargetTypes = map[string]bool{
	"account": true, "arena": true, "argument": true,
	"attribution": true, "operation": true, "case": true,
	"action": true, "appeal": true, "intent": true,
	"subscription": true, "run": true,
}

// MetadataAllowlist is the exact set of metadata keys the trail accepts.
// Identifiers, references and transition markers only: payloads,
// credentials, tokens, sessions, emails, addresses, free prose and
// antifraud internals are unrepresentable by construction.
var MetadataAllowlist = map[string]bool{
	"operation_id": true, "reference": true, "idempotency_key": true,
	"arena_id": true, "argument_id": true, "attribution_id": true,
	"target_account_id": true, "case_id": true, "action_id": true,
	"appeal_id": true, "intent_id": true, "subscription_id": true,
	"previous_status": true, "new_status": true, "outcome": true,
	"replayed": true, "rule": true, "reason": true,
}

// MaxMetadataValueLength bounds every metadata value. Keys are allowlisted
// identifiers and codes; the bound stops a value from smuggling an
// unbounded dump.
const MaxMetadataValueLength = 500

// AuditEvent is one immutable administrative fact: who did what to which
// target, why (stable code), with minimal metadata, when, and optionally
// correlated with which sibling events. It carries no payload, no secret
// and no personal data beyond the identifiers the fact is about.
type AuditEvent struct {
	Actor      string
	Action     string
	TargetType string
	TargetID   string
	ReasonCode string
	Metadata   map[string]string
	// IdempotencyKey is empty for unconditional records; set for retried
	// recorders that must resolve the original row.
	IdempotencyKey string
	// CorrelationID groups related events recorded together; empty when
	// the fact stands alone.
	CorrelationID string
	OccurredAt    time.Time
}

// Validate checks the event against the trail invariants without touching
// persistence.
func (e AuditEvent) Validate() error {
	if strings.TrimSpace(e.Actor) == "" {
		return ErrEmptyActorID
	}
	if !validAction(e.Action) {
		return ErrInvalidAction
	}
	if !allowedTargetTypes[e.TargetType] {
		return ErrInvalidTarget
	}
	if strings.TrimSpace(e.TargetID) == "" || len(e.TargetID) > 200 {
		return ErrInvalidTarget
	}
	trimmedReason := strings.TrimSpace(e.ReasonCode)
	if trimmedReason == "" {
		return ErrEmptyReasonCode
	}
	if len(e.ReasonCode) > 200 {
		return ErrReasonCodeTooLong
	}
	for key, value := range e.Metadata {
		if !MetadataAllowlist[key] {
			return ErrInvalidMetadata
		}
		if strings.TrimSpace(value) == "" || len(value) > MaxMetadataValueLength {
			return ErrInvalidMetadata
		}
	}
	if e.IdempotencyKey != "" {
		trimmed := strings.TrimSpace(e.IdempotencyKey)
		if trimmed == "" || len(e.IdempotencyKey) > 200 {
			return ErrInvalidMetadata
		}
	}
	if e.CorrelationID != "" {
		trimmed := strings.TrimSpace(e.CorrelationID)
		if trimmed == "" || len(e.CorrelationID) > 200 {
			return ErrInvalidMetadata
		}
	}
	if e.OccurredAt.IsZero() {
		return ErrEmptyOccurredAt
	}
	return nil
}

func validAction(raw string) bool {
	if len(raw) == 0 || len(raw) > 100 {
		return false
	}
	dot := strings.IndexByte(raw, '.')
	if dot <= 0 || dot == len(raw)-1 || strings.IndexByte(raw[dot+1:], '.') >= 0 {
		return false
	}
	for i := 0; i < len(raw); i++ {
		character := raw[i]
		if character == '.' {
			continue
		}
		switch {
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9' && i > 0:
		case character == '_':
		default:
			return false
		}
	}
	if raw[0] < 'a' || raw[0] > 'z' {
		return false
	}
	afterDot := raw[dot+1]
	return afterDot >= 'a' && afterDot <= 'z'
}
