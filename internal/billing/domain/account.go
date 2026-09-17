package domain

import "strings"

// AccountID identifies the entitlement owner. It is the billing module view
// of the identity account identifier.
type AccountID string

// String returns the string representation of the account identifier.
func (id AccountID) String() string {
	return string(id)
}

// IsZero reports whether the AccountID is uninitialized.
func (id AccountID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

// LotID uniquely identifies an Arena Pass lot.
type LotID string

// String returns the string representation of the lot identifier.
func (id LotID) String() string {
	return string(id)
}

// IsZero reports whether the LotID is uninitialized.
func (id LotID) IsZero() bool {
	return id == ""
}
