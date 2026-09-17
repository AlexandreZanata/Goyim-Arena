package domain

import "strings"

// AccountID identifies the wallet owner. It is the wallet module view of the
// identity account identifier and never carries profile, email or payment
// data.
type AccountID string

// String returns the string representation of the account identifier.
func (id AccountID) String() string {
	return string(id)
}

// IsZero reports whether the AccountID is uninitialized.
func (id AccountID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}
