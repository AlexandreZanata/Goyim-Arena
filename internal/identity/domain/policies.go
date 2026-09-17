package domain

import (
	"time"
)

// VerificationPolicy specifies the rules for issuing and validating email verification tokens.
type VerificationPolicy struct {
	TokenLifetime time.Duration
}

// DefaultVerificationPolicy returns standard 24-hour verification token parameters.
func DefaultVerificationPolicy() VerificationPolicy {
	return VerificationPolicy{
		TokenLifetime: 24 * time.Hour,
	}
}

// IsExpired reports whether a token issued at issuedAt is expired relative to now.
func (p VerificationPolicy) IsExpired(issuedAt, now time.Time) bool {
	return now.After(p.ExpiryInstant(issuedAt))
}

// ExpiryInstant returns the exact timestamp when a token issued at issuedAt expires.
func (p VerificationPolicy) ExpiryInstant(issuedAt time.Time) time.Time {
	return issuedAt.Add(p.TokenLifetime)
}

// CanIssueVerification validates whether the account is in an eligible state to receive a verification token.
func (p VerificationPolicy) CanIssueVerification(account *Account) error {
	if account == nil {
		return ErrEmptyAccountID
	}
	switch account.Status() {
	case AccountStatusDeleted:
		return ErrAccountDeleted
	case AccountStatusSuspended:
		return ErrAccountSuspended
	case AccountStatusActive:
		return ErrAccountAlreadyVerified
	case AccountStatusPending:
		return nil
	default:
		return ErrInvalidAccountStatus
	}
}

// PasswordResetPolicy specifies the rules for password recovery tokens.
// Per THR-AUTH-04 in THREAT_MODEL.md, recovery tokens expire strictly in 15 minutes.
type PasswordResetPolicy struct {
	TokenLifetime time.Duration
}

// DefaultPasswordResetPolicy returns standard 15-minute password reset parameters.
func DefaultPasswordResetPolicy() PasswordResetPolicy {
	return PasswordResetPolicy{
		TokenLifetime: 15 * time.Minute,
	}
}

// IsExpired reports whether a reset token issued at issuedAt is expired relative to now.
func (p PasswordResetPolicy) IsExpired(issuedAt, now time.Time) bool {
	return now.After(p.ExpiryInstant(issuedAt))
}

// ExpiryInstant returns the exact timestamp when a reset token issued at issuedAt expires.
func (p PasswordResetPolicy) ExpiryInstant(issuedAt time.Time) time.Time {
	return issuedAt.Add(p.TokenLifetime)
}

// CanIssueReset validates whether the account is eligible for a password reset.
func (p PasswordResetPolicy) CanIssueReset(account *Account) error {
	if account == nil {
		return ErrEmptyAccountID
	}
	switch account.Status() {
	case AccountStatusDeleted:
		return ErrAccountDeleted
	case AccountStatusSuspended:
		return ErrAccountSuspended
	case AccountStatusActive, AccountStatusPending:
		return nil
	default:
		return ErrInvalidAccountStatus
	}
}

// SessionPolicy defines inactivity and absolute duration limits for authenticated sessions.
// Per THR-AUTH-01 in THREAT_MODEL.md, sessions expire after 24h inactivity or 14 days absolute.
type SessionPolicy struct {
	IdleTimeout      time.Duration
	AbsoluteLifetime time.Duration
}

// DefaultSessionPolicy returns standard session bounds.
func DefaultSessionPolicy() SessionPolicy {
	return SessionPolicy{
		IdleTimeout:      24 * time.Hour,
		AbsoluteLifetime: 14 * 24 * time.Hour,
	}
}

// IsExpired reports whether a session is expired due to either inactivity or absolute lifetime.
func (p SessionPolicy) IsExpired(createdAt, lastSeenAt, now time.Time) bool {
	if now.After(createdAt.Add(p.AbsoluteLifetime)) {
		return true
	}
	if now.After(lastSeenAt.Add(p.IdleTimeout)) {
		return true
	}
	return false
}

// ExpiryInstant calculates the earliest expiration time based on idle timeout and absolute lifetime.
func (p SessionPolicy) ExpiryInstant(createdAt, lastSeenAt time.Time) time.Time {
	idleExpiry := lastSeenAt.Add(p.IdleTimeout)
	absoluteExpiry := createdAt.Add(p.AbsoluteLifetime)
	if idleExpiry.Before(absoluteExpiry) {
		return idleExpiry
	}
	return absoluteExpiry
}
