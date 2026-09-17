// Package application defines the use cases, orchestrations and
// consumer-oriented ports of the billing module.
package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/Goyim-Arena/internal/billing/domain"
)

// Clock exposes wall-clock time to billing use cases, keeping them
// deterministic under test (ADR-012).
type Clock interface {
	Now() time.Time
}

// GrantPassLotRequest is a validated Arena Pass grant to persist atomically.
type GrantPassLotRequest struct {
	AccountID domain.AccountID
	Origin    domain.PassOrigin
	Quantity  domain.Quantity
	Reference domain.Reference
	// ExpiresAt is nil for grants that never expire (bought passes) and an
	// exact UTC instant for period-bound grants.
	ExpiresAt *time.Time
	GrantedAt time.Time
}

// GrantPassLotResult is the outcome of a grant: the lot the
// (account, origin, reference) key resolves to, and whether it was a replay
// of an earlier attempt.
type GrantPassLotResult struct {
	Lot      domain.PassLot
	Replayed bool
}

// PassLotRepository persists Arena Pass grants with idempotency anchored on
// (account, origin, reference).
type PassLotRepository interface {
	// GrantPassLot inserts the lot exactly once. A duplicate grant resolves
	// the original lot with Replayed set and writes nothing.
	GrantPassLot(ctx context.Context, request GrantPassLotRequest) (*GrantPassLotResult, error)
}
