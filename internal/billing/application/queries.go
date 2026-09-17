package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/Goyim-Arena/internal/billing/domain"
)

// PassLotQueryRepository exposes the read projections of Arena Pass lots.
type PassLotQueryRepository interface {
	// ListAccountPassLots returns every lot of the account in canonical
	// order: nearest expiration first, lots that never expire last.
	ListAccountPassLots(ctx context.Context, accountID domain.AccountID) ([]domain.PassLot, error)

	// ListExpiredPassLots returns a bounded page of lots already expired at
	// the instant that still hold passes, ordered by expiration.
	ListExpiredPassLots(ctx context.Context, at time.Time, limit int) ([]domain.PassLot, error)
}

// ArenaPassLotSummary is the private breakdown entry of one lot: expiration
// is derived at read time and expired lots are never counted as available.
type ArenaPassLotSummary struct {
	Lot     domain.PassLot
	Expired bool
}

// ArenaPassSummary is the owner's private pass projection: the total of
// passes available at the checked instant plus the per-lot breakdown.
type ArenaPassSummary struct {
	AccountID      domain.AccountID
	CheckedAt      time.Time
	AvailableTotal int64
	Lots           []ArenaPassLotSummary
}
