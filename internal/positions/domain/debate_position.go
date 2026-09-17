package domain

import "time"

// DebatePosition is the private projection of one account's position in one
// Arena. The initial position is immutable history: it is only ever set at
// confirmation. The current position starts equal to the initial one and
// moves only through accepted changes, each incrementing the optimistic
// version.
type DebatePosition struct {
	arenaID   ArenaID
	accountID AccountID
	initial   Position
	current   Position
	version   int32
	createdAt time.Time
	updatedAt time.Time
}

// ConfirmInitialPosition creates the projection at confirmation time:
// version 1, current equal to the initial position.
func ConfirmInitialPosition(arenaID ArenaID, accountID AccountID, position Position, at time.Time) (*DebatePosition, error) {
	if err := validateIdentity(arenaID, accountID); err != nil {
		return nil, err
	}
	if err := validatePosition(position); err != nil {
		return nil, err
	}
	if at.IsZero() {
		return nil, ErrInvalidInstant
	}
	return &DebatePosition{
		arenaID:   arenaID,
		accountID: accountID,
		initial:   position,
		current:   position,
		version:   1,
		createdAt: at,
		updatedAt: at,
	}, nil
}

// ReconstituteDebatePosition rebuilds the projection from stored state,
// validating the same invariants the database enforces. Adapters use it to
// map stored rows.
func ReconstituteDebatePosition(arenaID ArenaID, accountID AccountID, initial, current Position, version int32, createdAt, updatedAt time.Time) (*DebatePosition, error) {
	if err := validateIdentity(arenaID, accountID); err != nil {
		return nil, err
	}
	if err := validatePosition(initial); err != nil {
		return nil, err
	}
	if err := validatePosition(current); err != nil {
		return nil, err
	}
	if version < 1 {
		return nil, ErrInvalidVersion
	}
	if createdAt.IsZero() || updatedAt.IsZero() || updatedAt.Before(createdAt) {
		return nil, ErrInvalidInstant
	}
	// The chain starts at the initial position: before any change the
	// projection must still be the initial choice.
	if version == 1 && !current.Equals(initial) {
		return nil, ErrBrokenChain
	}
	return &DebatePosition{
		arenaID:   arenaID,
		accountID: accountID,
		initial:   initial,
		current:   current,
		version:   version,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}, nil
}

// ChangeTo applies one accepted change: the target must differ from the
// current position, the instant never moves backwards and the projection
// advances by exactly one version. The returned change is the history entry
// to append; the initial position is never touched.
func (p *DebatePosition) ChangeTo(next Position, at time.Time) (PositionChange, error) {
	if err := validatePosition(next); err != nil {
		return PositionChange{}, err
	}
	if next.Equals(p.current) {
		return PositionChange{}, ErrSamePosition
	}
	if at.IsZero() || at.Before(p.updatedAt) {
		return PositionChange{}, ErrInvalidInstant
	}

	change, err := NewPositionChange(p.arenaID, p.accountID, p.current, next, p.version+1, at)
	if err != nil {
		return PositionChange{}, err
	}

	p.current = next
	p.version = change.version
	p.updatedAt = at
	return change, nil
}

// ArenaID returns the Arena the projection belongs to.
func (p *DebatePosition) ArenaID() ArenaID { return p.arenaID }

// AccountID returns the account that holds the projection.
func (p *DebatePosition) AccountID() AccountID { return p.accountID }

// InitialPosition returns the immutable first confirmed position.
func (p *DebatePosition) InitialPosition() Position { return p.initial }

// CurrentPosition returns the projection of the chain tip.
func (p *DebatePosition) CurrentPosition() Position { return p.current }

// Version returns the optimistic version: 1 at confirmation, one more per
// accepted change.
func (p *DebatePosition) Version() int32 { return p.version }

// CreatedAt returns the confirmation instant.
func (p *DebatePosition) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt returns the instant of the last accepted change (or the
// confirmation instant when there is none).
func (p *DebatePosition) UpdatedAt() time.Time { return p.updatedAt }

// validateIdentity rejects uninitialized identifiers.
func validateIdentity(arenaID ArenaID, accountID AccountID) error {
	if arenaID.IsZero() {
		return ErrEmptyArenaID
	}
	if accountID.IsZero() {
		return ErrEmptyAccountID
	}
	return nil
}

// validatePosition rejects missing or unsupported position values.
func validatePosition(position Position) error {
	if position.IsZero() {
		return ErrEmptyPosition
	}
	if !position.IsSupported() {
		return ErrInvalidPosition
	}
	return nil
}
