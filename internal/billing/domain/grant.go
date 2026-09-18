package domain

import "strconv"

// GrantKind is the closed vocabulary of what a purchased product confers.
//
// The kinds map onto the entitlement vocabulary of the other billing use
// cases and of the wallet ledger (P12-T06/T07/T08): INK credits the
// PURCHASED_INK bucket, ARENA_PASS grants a PURCHASE pass lot, and MEMBER is
// the recurring franchise applied once per subscription period.
type GrantKind string

const (
	// GrantKindINK is a one-off purchased INK quantity.
	GrantKindINK GrantKind = "INK"

	// GrantKindArenaPass is a one-off purchased Arena Pass quantity.
	GrantKindArenaPass GrantKind = "ARENA_PASS"

	// GrantKindMember is the recurring Member entitlement; its quantities
	// belong to the subscription period, not to a fixed number.
	GrantKindMember GrantKind = "MEMBER"
)

// AllGrantKinds returns the closed vocabulary in canonical order.
func AllGrantKinds() []GrantKind {
	return []GrantKind{GrantKindINK, GrantKindArenaPass, GrantKindMember}
}

// ParseGrantKind validates a configured or persisted grant kind against the
// exact vocabulary.
func ParseGrantKind(raw string) (GrantKind, error) {
	kind := GrantKind(raw)
	switch kind {
	case GrantKindINK, GrantKindArenaPass, GrantKindMember:
		return kind, nil
	default:
		return "", ErrInvalidGrant
	}
}

// Grant is the entitlement a catalog product confers. Exactly one shape is
// valid per kind, so a product can never promise an amount its kind does not
// describe: INK and ARENA_PASS carry a positive quantity, MEMBER carries none
// (its franchise is fixed by the subscription period).
type Grant struct {
	kind     GrantKind
	quantity int64
}

// NewINKGrant builds a purchased INK grant; zero or negative quantities are
// meaningless.
func NewINKGrant(quantity int64) (Grant, error) {
	if quantity < 1 {
		return Grant{}, ErrInvalidGrant
	}
	return Grant{kind: GrantKindINK, quantity: quantity}, nil
}

// NewArenaPassGrant builds a purchased Arena Pass grant.
func NewArenaPassGrant(quantity int32) (Grant, error) {
	if quantity < 1 {
		return Grant{}, ErrInvalidGrant
	}
	return Grant{kind: GrantKindArenaPass, quantity: int64(quantity)}, nil
}

// NewMemberGrant builds the recurring Member entitlement.
func NewMemberGrant() Grant {
	return Grant{kind: GrantKindMember}
}

// Kind returns the grant kind.
func (g Grant) Kind() GrantKind {
	return g.kind
}

// Quantity returns the INK units or the pass count; MEMBER grants have none.
func (g Grant) Quantity() int64 {
	return g.quantity
}

// IsZero reports whether the grant is the uninitialized zero value.
func (g Grant) IsZero() bool {
	return g.kind == ""
}

// Validate reports whether the grant carries exactly the quantities its kind
// describes.
func (g Grant) Validate() error {
	switch g.kind {
	case GrantKindINK, GrantKindArenaPass:
		if g.quantity < 1 {
			return ErrInvalidGrant
		}
	case GrantKindMember:
		if g.quantity != 0 {
			return ErrInvalidGrant
		}
	default:
		return ErrInvalidGrant
	}
	return nil
}

// Equals reports whether two grants are exactly the same entitlement.
func (g Grant) Equals(other Grant) bool {
	return g.kind == other.kind && g.quantity == other.quantity
}

// String renders the diagnostic form "INK 10000" or "MEMBER".
func (g Grant) String() string {
	if g.kind == GrantKindMember {
		return GrantKindMember.String()
	}
	return g.kind.String() + " " + strconv.FormatInt(g.quantity, 10)
}

// String returns the stored grant kind value.
func (k GrantKind) String() string {
	return string(k)
}
