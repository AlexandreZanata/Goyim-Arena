package domain

import (
	"fmt"
	"time"
)

// DefaultUsernameCooldown is the default minimum interval between two
// username changes of the same account. It is configurable at bootstrap.
const DefaultUsernameCooldown = 30 * 24 * time.Hour

// defaultReservedUsernames lists handles that must not be claimed by users:
// routes, product vocabulary, operational roles and brand terms. The list is
// normalized (lowercase ASCII) and matched against Username.Normalized.
var defaultReservedUsernames = []string{
	"admin", "administrator", "api", "app", "arena", "arenas", "assets",
	"billing", "goyim", "health", "help", "ink", "jobs", "login", "logout",
	"metrics", "moderation", "moderator", "official", "privacy",
	"profiles", "root", "security", "settings", "signup", "staff", "static",
	"support", "system", "terms", "transparency", "wallet", "www",
}

// DefaultReservedUsernames returns a copy of the standard reserved handles.
func DefaultReservedUsernames() []string {
	out := make([]string, len(defaultReservedUsernames))
	copy(out, defaultReservedUsernames)
	return out
}

// UsernameChange is the auditable record of a username assignment or change.
// Previous is zero when the account is claiming its first username.
type UsernameChange struct {
	Previous  Username
	Current   Username
	ChangedAt time.Time
}

// IsFirstAssignment reports whether the change establishes the first
// username of an account.
func (c UsernameChange) IsFirstAssignment() bool {
	return c.Previous.IsZero()
}

// UsernamePolicy owns the rules of username assignment and change: the
// reserved list and the configurable cooldown between changes. The audit
// record it produces (UsernameChange) is what the application persists in
// app.username_history.
type UsernamePolicy struct {
	cooldown time.Duration
	reserved map[string]struct{}
}

// NewUsernamePolicy builds a policy from an explicit cooldown and reserved
// list. Negative cooldowns are rejected and every reserved entry must be a
// valid username; entries are stored normalized.
func NewUsernamePolicy(cooldown time.Duration, reserved []string) (UsernamePolicy, error) {
	if cooldown < 0 {
		return UsernamePolicy{}, ErrInvalidCooldown
	}

	normalized := make(map[string]struct{}, len(reserved))
	for _, entry := range reserved {
		username, err := ParseUsername(entry)
		if err != nil {
			return UsernamePolicy{}, fmt.Errorf("reserved username %q: %w", entry, ErrInvalidReservedUsername)
		}
		normalized[username.Normalized()] = struct{}{}
	}

	return UsernamePolicy{cooldown: cooldown, reserved: normalized}, nil
}

// DefaultUsernamePolicy returns the standard policy: a 30-day cooldown and
// the default reserved handles.
func DefaultUsernamePolicy() UsernamePolicy {
	reserved := make(map[string]struct{}, len(defaultReservedUsernames))
	for _, entry := range defaultReservedUsernames {
		reserved[entry] = struct{}{}
	}
	return UsernamePolicy{cooldown: DefaultUsernameCooldown, reserved: reserved}
}

// Cooldown returns the configured minimum interval between changes.
func (p UsernamePolicy) Cooldown() time.Duration {
	return p.cooldown
}

// IsReserved reports whether the normalized username is reserved. The zero
// policy reserves nothing.
func (p UsernamePolicy) IsReserved(username Username) bool {
	if username.IsZero() {
		return false
	}
	_, reserved := p.reserved[username.Normalized()]
	return reserved
}

// ValidateReserved returns ErrUsernameReserved when the username may not be
// claimed.
func (p UsernamePolicy) ValidateReserved(username Username) error {
	if p.IsReserved(username) {
		return ErrUsernameReserved
	}
	return nil
}

// CheckCooldown returns ErrUsernameCooldown when less than the configured
// cooldown elapsed since lastChangedAt, when lastChangedAt is unknown (zero),
// or when the clock moved backwards. A non-positive cooldown disables the
// rule. Callers only reach this check when a previous username exists.
func (p UsernamePolicy) CheckCooldown(lastChangedAt, now time.Time) error {
	if p.cooldown <= 0 {
		return nil
	}
	if lastChangedAt.IsZero() {
		return ErrUsernameCooldown
	}
	if now.Sub(lastChangedAt) < p.cooldown {
		return ErrUsernameCooldown
	}
	return nil
}

// NextEligibleAt returns the first instant a new change becomes eligible.
// It returns the zero time when lastChangedAt is unknown.
func (p UsernamePolicy) NextEligibleAt(lastChangedAt time.Time) time.Time {
	if lastChangedAt.IsZero() {
		return time.Time{}
	}
	return lastChangedAt.Add(p.cooldown)
}

// PlanChange validates a username assignment or change and returns the audit
// record to persist. First assignments bypass the cooldown but still respect
// the reserved list; changes must differ (by normalized form) from the
// current username and respect the cooldown.
func (p UsernamePolicy) PlanChange(previous, proposed Username, lastChangedAt, now time.Time) (UsernameChange, error) {
	if proposed.IsZero() {
		return UsernameChange{}, ErrEmptyUsername
	}
	if err := p.ValidateReserved(proposed); err != nil {
		return UsernameChange{}, err
	}

	if !previous.IsZero() {
		if previous.Equals(proposed) {
			return UsernameChange{}, ErrUsernameUnchanged
		}
		if err := p.CheckCooldown(lastChangedAt, now); err != nil {
			return UsernameChange{}, err
		}
	}

	return UsernameChange{
		Previous:  previous,
		Current:   proposed,
		ChangedAt: now.UTC(),
	}, nil
}
