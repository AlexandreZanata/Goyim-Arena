package domain

import (
	"bytes"
	"strings"
	"time"
)

// SessionID uniquely identifies a session record in the identity domain.
type SessionID string

// String returns the string representation of the session identifier.
func (id SessionID) String() string {
	return string(id)
}

// IsZero reports whether the SessionID is empty or uninitialized.
func (id SessionID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

// Session represents an authenticated user session backed by an opaque token hash.
type Session struct {
	id         SessionID
	accountID  AccountID
	tokenHash  []byte
	createdAt  time.Time
	expiresAt  time.Time
	lastSeenAt time.Time
	revokedAt  *time.Time
	ipAddress  string
	userAgent  string
}

// NewSession initializes a new Session entity with initial expiration computed from the provided policy.
func NewSession(
	id SessionID,
	accountID AccountID,
	tokenHash []byte,
	ipAddress, userAgent string,
	now time.Time,
	policy SessionPolicy,
) (*Session, error) {
	if id.IsZero() {
		return nil, ErrEmptySessionID
	}
	if accountID.IsZero() {
		return nil, ErrEmptyAccountID
	}
	if len(tokenHash) == 0 {
		return nil, ErrEmptySessionToken
	}

	hashCopy := make([]byte, len(tokenHash))
	copy(hashCopy, tokenHash)

	expiresAt := policy.ExpiryInstant(now, now)

	return &Session{
		id:         id,
		accountID:  accountID,
		tokenHash:  hashCopy,
		createdAt:  now,
		expiresAt:  expiresAt,
		lastSeenAt: now,
		revokedAt:  nil,
		ipAddress:  strings.TrimSpace(ipAddress),
		userAgent:  strings.TrimSpace(userAgent),
	}, nil
}

// ReconstituteSession constructs a Session from persistent storage without re-evaluating creation invariants.
func ReconstituteSession(
	id SessionID,
	accountID AccountID,
	tokenHash []byte,
	createdAt, expiresAt, lastSeenAt time.Time,
	revokedAt *time.Time,
	ipAddress, userAgent string,
) (*Session, error) {
	if id.IsZero() {
		return nil, ErrEmptySessionID
	}
	if accountID.IsZero() {
		return nil, ErrEmptyAccountID
	}
	if len(tokenHash) == 0 {
		return nil, ErrEmptySessionToken
	}

	hashCopy := make([]byte, len(tokenHash))
	copy(hashCopy, tokenHash)

	var revokedCopy *time.Time
	if revokedAt != nil {
		t := *revokedAt
		revokedCopy = &t
	}

	return &Session{
		id:         id,
		accountID:  accountID,
		tokenHash:  hashCopy,
		createdAt:  createdAt,
		expiresAt:  expiresAt,
		lastSeenAt: lastSeenAt,
		revokedAt:  revokedCopy,
		ipAddress:  strings.TrimSpace(ipAddress),
		userAgent:  strings.TrimSpace(userAgent),
	}, nil
}

// ID returns the unique session identifier.
func (s *Session) ID() SessionID {
	return s.id
}

// AccountID returns the account identifier to which the session belongs.
func (s *Session) AccountID() AccountID {
	return s.accountID
}

// TokenHash returns a defensive copy of the SHA-256 binary hash of the session token.
func (s *Session) TokenHash() []byte {
	copyBuf := make([]byte, len(s.tokenHash))
	copy(copyBuf, s.tokenHash)
	return copyBuf
}

// HasTokenHash reports whether the session's token hash matches the provided hash.
func (s *Session) HasTokenHash(hash []byte) bool {
	return bytes.Equal(s.tokenHash, hash)
}

// CreatedAt returns the session establishment instant.
func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

// ExpiresAt returns the current expiration deadline.
func (s *Session) ExpiresAt() time.Time {
	return s.expiresAt
}

// LastSeenAt returns the instant the session was last observed active.
func (s *Session) LastSeenAt() time.Time {
	return s.lastSeenAt
}

// RevokedAt returns a copy of the revocation instant, or nil if the session is not revoked.
func (s *Session) RevokedAt() *time.Time {
	if s.revokedAt == nil {
		return nil
	}
	t := *s.revokedAt
	return &t
}

// IPAddress returns the client IP address recorded when the session was created.
func (s *Session) IPAddress() string {
	return s.ipAddress
}

// UserAgent returns the client user-agent recorded when the session was created.
func (s *Session) UserAgent() string {
	return s.userAgent
}

// IsRevoked reports whether the session has been explicitly revoked.
func (s *Session) IsRevoked() bool {
	return s.revokedAt != nil
}

// IsExpired reports whether the session has expired per the given policy or reached its stored deadline.
func (s *Session) IsExpired(now time.Time, policy SessionPolicy) bool {
	if policy.IsExpired(s.createdAt, s.lastSeenAt, now) {
		return true
	}
	return now.After(s.expiresAt)
}

// Revoke transitions an active session to the revoked state.
func (s *Session) Revoke(now time.Time) error {
	if s.IsRevoked() {
		return ErrSessionRevoked
	}
	t := now
	s.revokedAt = &t
	return nil
}

// ShouldTouch reports whether sufficient time has elapsed since lastSeenAt to warrant writing an updated timestamp.
func (s *Session) ShouldTouch(now time.Time, threshold time.Duration) bool {
	if s.IsRevoked() {
		return false
	}
	return !now.Before(s.lastSeenAt.Add(threshold))
}

// Touch advances lastSeenAt and recomputes the expiration deadline according to the policy.
func (s *Session) Touch(now time.Time, policy SessionPolicy) {
	if s.IsRevoked() {
		return
	}
	s.lastSeenAt = now
	s.expiresAt = policy.ExpiryInstant(s.createdAt, now)
}
