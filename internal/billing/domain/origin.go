package domain

// PassOrigin is the stable origin of an Arena Pass grant. The values are
// mirrored exactly by the CHECK constraint of app.arena_pass_lots
// (migration 00011).
type PassOrigin string

const (
	// OriginPurchase is a bought pass; it does not expire initially.
	OriginPurchase PassOrigin = "PURCHASE"

	// OriginMember is the monthly Member franchise; it expires at the end of
	// its subscription period and does not accumulate.
	OriginMember PassOrigin = "MEMBER"

	// OriginAdmin is a discretionary administrative grant; its expiration is
	// optional.
	OriginAdmin PassOrigin = "ADMIN"
)

// AllPassOrigins returns the vocabulary in canonical order.
func AllPassOrigins() []PassOrigin {
	return []PassOrigin{OriginPurchase, OriginMember, OriginAdmin}
}

// ParsePassOrigin validates a persisted or transport origin against the
// exact ledger vocabulary.
func ParsePassOrigin(raw string) (PassOrigin, error) {
	origin := PassOrigin(raw)
	if !origin.IsValid() {
		return "", ErrInvalidPassOrigin
	}
	return origin, nil
}

// IsValid reports whether the origin is an authorized enum value.
func (o PassOrigin) IsValid() bool {
	switch o {
	case OriginPurchase, OriginMember, OriginAdmin:
		return true
	default:
		return false
	}
}

// String returns the stored origin value.
func (o PassOrigin) String() string {
	return string(o)
}
