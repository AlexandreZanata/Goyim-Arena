package domain

import "time"

// PassLot is the immutable, reconstituted record of an Arena Pass grant:
// origin, granted quantity, remaining consumable projection, optional
// expiration and the stable reference of the cause.
type PassLot struct {
	id        LotID
	accountID AccountID
	origin    PassOrigin
	quantity  Quantity
	remaining int32
	expiresAt *time.Time
	reference Reference
	createdAt time.Time
}

// ReconstitutePassLot rebuilds a PassLot from persistent state, validating
// the same invariants without re-running grant rules.
func ReconstitutePassLot(
	id LotID,
	accountID AccountID,
	origin PassOrigin,
	quantity Quantity,
	remaining int32,
	expiresAt *time.Time,
	reference Reference,
	createdAt time.Time,
) (*PassLot, error) {
	if id.IsZero() {
		return nil, ErrEmptyLotID
	}
	if accountID.IsZero() {
		return nil, ErrEmptyAccountID
	}
	if !origin.IsValid() {
		return nil, ErrInvalidPassOrigin
	}
	if quantity.IsZero() {
		return nil, ErrInvalidQuantity
	}
	if remaining < 0 || remaining > quantity.Int32() {
		return nil, ErrInvalidRemaining
	}
	if reference.IsZero() {
		return nil, ErrEmptyReference
	}

	var expiryCopy *time.Time
	if expiresAt != nil {
		instant := expiresAt.UTC()
		expiryCopy = &instant
	}

	return &PassLot{
		id:        id,
		accountID: accountID,
		origin:    origin,
		quantity:  quantity,
		remaining: remaining,
		expiresAt: expiryCopy,
		reference: reference,
		createdAt: createdAt.UTC(),
	}, nil
}

// ID returns the lot identifier.
func (l *PassLot) ID() LotID {
	return l.id
}

// AccountID returns the entitlement owner.
func (l *PassLot) AccountID() AccountID {
	return l.accountID
}

// Origin returns the grant origin.
func (l *PassLot) Origin() PassOrigin {
	return l.origin
}

// Quantity returns the granted quantity.
func (l *PassLot) Quantity() Quantity {
	return l.quantity
}

// Remaining returns the consumable projection of the lot.
func (l *PassLot) Remaining() int32 {
	return l.remaining
}

// ExpiresAt returns a copy of the expiration instant, or nil when the lot
// never expires (bought passes do not expire initially).
func (l *PassLot) ExpiresAt() *time.Time {
	if l.expiresAt == nil {
		return nil
	}
	instant := *l.expiresAt
	return &instant
}

// Reference returns the stable cause reference of the grant.
func (l *PassLot) Reference() Reference {
	return l.reference
}

// CreatedAt returns the grant instant.
func (l *PassLot) CreatedAt() time.Time {
	return l.createdAt
}

// IsExpired reports whether the lot is expired at the instant: an expiration
// instant marks the end of validity, so the lot is expired from it onwards.
func (l *PassLot) IsExpired(at time.Time) bool {
	if l.expiresAt == nil {
		return false
	}
	return !at.UTC().Before(*l.expiresAt)
}

// IsAvailable reports whether the lot can be consumed at the instant.
func (l *PassLot) IsAvailable(at time.Time) bool {
	return l.remaining > 0 && !l.IsExpired(at)
}
