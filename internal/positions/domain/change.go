package domain

import "time"

// PositionChange is one accepted transition of the append-only history: the
// previous position, the new position, the resulting projection version and
// the instant. It is a value object: the persistence layer assigns the row
// identifier, and later phases may attribute the change to arguments.
type PositionChange struct {
	arenaID   ArenaID
	accountID AccountID
	from      Position
	to        Position
	version   int32
	changedAt time.Time
}

// NewPositionChange validates and constructs one history entry. A change
// always moves to a different position and carries the resulting version,
// which starts at 2 (version 1 is the initial confirmation).
func NewPositionChange(arenaID ArenaID, accountID AccountID, from, to Position, version int32, changedAt time.Time) (PositionChange, error) {
	if arenaID.IsZero() {
		return PositionChange{}, ErrEmptyArenaID
	}
	if accountID.IsZero() {
		return PositionChange{}, ErrEmptyAccountID
	}
	if from.IsZero() || to.IsZero() {
		return PositionChange{}, ErrEmptyPosition
	}
	if !from.IsSupported() || !to.IsSupported() {
		return PositionChange{}, ErrInvalidPosition
	}
	if from.Equals(to) {
		return PositionChange{}, ErrSamePosition
	}
	if version < 2 {
		return PositionChange{}, ErrInvalidVersion
	}
	if changedAt.IsZero() {
		return PositionChange{}, ErrInvalidInstant
	}
	return PositionChange{
		arenaID:   arenaID,
		accountID: accountID,
		from:      from,
		to:        to,
		version:   version,
		changedAt: changedAt,
	}, nil
}

// ArenaID returns the Arena the change belongs to.
func (c PositionChange) ArenaID() ArenaID { return c.arenaID }

// AccountID returns the account that changed its position.
func (c PositionChange) AccountID() AccountID { return c.accountID }

// From returns the position before the change.
func (c PositionChange) From() Position { return c.from }

// To returns the position after the change.
func (c PositionChange) To() Position { return c.to }

// Version returns the resulting projection version.
func (c PositionChange) Version() int32 { return c.version }

// ChangedAt returns the instant of the accepted change.
func (c PositionChange) ChangedAt() time.Time { return c.changedAt }

// DeriveCurrentPosition replays an append-only chain over the initial
// position. The chain must be contiguous: the first change carries version 2
// and every change starts from the running position. The caller scopes the
// chain to one (Arena, account) pair; this function only proves continuity,
// so the projection is always reconstructible from history.
func DeriveCurrentPosition(initial Position, changes []PositionChange) (Position, int32, error) {
	if initial.IsZero() {
		return Position{}, 0, ErrEmptyPosition
	}
	if !initial.IsSupported() {
		return Position{}, 0, ErrInvalidPosition
	}

	current := initial
	version := int32(1)
	for _, change := range changes {
		if change.version != version+1 || !change.from.Equals(current) {
			return Position{}, 0, ErrBrokenChain
		}
		current = change.to
		version = change.version
	}
	return current, version, nil
}
