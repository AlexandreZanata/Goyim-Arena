package domain

import "strings"

// Role is the minimal administrative capability of an account. It mirrors
// the CHECK constraint of app.admin_roles (migrations 00022 and 00023).
// Authorization never keys on email, frontend flags, payment state or
// popularity: only the account identifier resolves to a role, and only the
// policy below decides what the role may do.
type Role string

const (
	// RoleModerator handles reversible content triage.
	RoleModerator Role = "moderator"
	// RoleAdmin decides anything, including account-level sanctions.
	RoleAdmin Role = "admin"
	// RoleSecurity handles safety and legal preservation.
	RoleSecurity Role = "security"
)

// AllRoles returns the closed vocabulary in canonical order.
func AllRoles() []Role {
	return []Role{RoleModerator, RoleAdmin, RoleSecurity}
}

// ParseRole validates a persisted or transported role against the exact
// vocabulary.
func ParseRole(raw string) (Role, error) {
	role := Role(strings.TrimSpace(raw))
	switch role {
	case RoleModerator, RoleAdmin, RoleSecurity:
		return role, nil
	default:
		return "", ErrInvalidRole
	}
}

// IsValid reports whether the role is an authorized enum value.
func (r Role) IsValid() bool {
	switch r {
	case RoleModerator, RoleAdmin, RoleSecurity:
		return true
	default:
		return false
	}
}

// String returns the stored role value.
func (r Role) String() string {
	return string(r)
}

// AccountID identifies an account by its stable identifier. It is an
// opaque string here on purpose: the moderation module never parses emails
// and never trusts frontend claims about who the actor is.
type AccountID string

// String returns the string representation of the account identifier.
func (id AccountID) String() string {
	return string(id)
}

// IsZero reports whether the AccountID is uninitialized.
func (id AccountID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}
