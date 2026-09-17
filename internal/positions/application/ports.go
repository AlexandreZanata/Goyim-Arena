package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/Goyim-Arena/internal/positions/domain"
)

// Clock reports the current instant. The application never reads the system
// clock directly, so tests and future workers control time.
type Clock interface {
	Now() time.Time
}

// AccountEligibility answers whether an account may confirm positions. The
// positions module never decides account policy: an identity adapter answers
// this consumer-oriented port, mapping its own state into the errors below.
type AccountEligibility interface {
	// EnsureEligible returns ErrAccountNotFound for unknown accounts,
	// ErrAccountNotEligible for accounts that may not participate yet
	// (pending email verification, for example) and ErrAccountSuspended
	// for suspended accounts.
	EnsureEligible(ctx context.Context, accountID domain.AccountID) error
}

// ArenaEligibility answers whether an Arena accepts new positions. The
// arenas module owns the lifecycle: an adapter answers this
// consumer-oriented port, mapping unknown identifiers into ErrArenaNotFound
// and every non-published state (draft, closed, restricted, removed) into
// ErrArenaNotOpen.
type ArenaEligibility interface {
	// EnsureAcceptsPositions returns ErrArenaNotFound or ErrArenaNotOpen.
	EnsureAcceptsPositions(ctx context.Context, arenaID domain.ArenaID) error
}

// PositionRepository persists the private position projection.
type PositionRepository interface {
	// GetByAccountAndArena returns the stored projection of one account in
	// one Arena, or ErrPositionNotFound.
	GetByAccountAndArena(ctx context.Context, arenaID domain.ArenaID, accountID domain.AccountID) (*domain.DebatePosition, error)

	// ConfirmInitialPosition atomically inserts the initial projection.
	// inserted is false when the pair already holds a position: the stored
	// projection is returned so the caller can resolve a replay.
	ConfirmInitialPosition(ctx context.Context, arenaID domain.ArenaID, accountID domain.AccountID, position domain.Position, at time.Time) (*domain.DebatePosition, bool, error)
}
