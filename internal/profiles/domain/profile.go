package domain

import (
	"strings"
	"time"
)

// AccountID identifies the account that owns a profile. It is the profiles
// module view of the identity account identifier and never carries email or
// credential data.
type AccountID string

// String returns the string representation of the account identifier.
func (id AccountID) String() string {
	return string(id)
}

// IsZero reports whether the AccountID is uninitialized.
func (id AccountID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

// Profile is the aggregate root of the profiles module: the public username
// and the interface locale preference of one account. It deliberately holds
// no email, credential or payment-provider data (docs/PRIVACY.md).
type Profile struct {
	accountID AccountID
	username  Username
	locale    Locale
	createdAt time.Time
	updatedAt time.Time
}

// NewProfile creates a validated profile for an account.
func NewProfile(accountID AccountID, username Username, locale Locale, now time.Time) (*Profile, error) {
	return ReconstituteProfile(accountID, username, locale, now, now)
}

// ReconstituteProfile rebuilds a Profile from persistent state, validating
// the same invariants without re-running creation rules. Adapters use it to
// map stored rows.
func ReconstituteProfile(accountID AccountID, username Username, locale Locale, createdAt, updatedAt time.Time) (*Profile, error) {
	if accountID.IsZero() {
		return nil, ErrEmptyAccountID
	}
	if username.IsZero() {
		return nil, ErrEmptyUsername
	}
	if locale.IsZero() {
		return nil, ErrEmptyLocale
	}
	if !locale.IsSupported() {
		return nil, ErrUnsupportedLocale
	}

	return &Profile{
		accountID: accountID,
		username:  username,
		locale:    locale,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}, nil
}

// ID returns the identifier of the owning account.
func (p *Profile) ID() AccountID {
	return p.accountID
}

// Username returns the public handle.
func (p *Profile) Username() Username {
	return p.username
}

// Locale returns the interface locale preference.
func (p *Profile) Locale() Locale {
	return p.locale
}

// CreatedAt returns the profile creation timestamp.
func (p *Profile) CreatedAt() time.Time {
	return p.createdAt
}

// UpdatedAt returns the timestamp of the latest profile mutation.
func (p *Profile) UpdatedAt() time.Time {
	return p.updatedAt
}

// ChangeUsername applies a previously planned and audited username change.
func (p *Profile) ChangeUsername(change UsernameChange) error {
	if change.Current.IsZero() {
		return ErrEmptyUsername
	}
	p.username = change.Current
	p.updatedAt = change.ChangedAt
	return nil
}

// ChangeLocale replaces the interface locale preference. It never touches
// content_language, which belongs to Arena content and is immutable.
func (p *Profile) ChangeLocale(locale Locale, now time.Time) error {
	if locale.IsZero() {
		return ErrEmptyLocale
	}
	if !locale.IsSupported() {
		return ErrUnsupportedLocale
	}
	p.locale = locale
	p.updatedAt = now
	return nil
}
