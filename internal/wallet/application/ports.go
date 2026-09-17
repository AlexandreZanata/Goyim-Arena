package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/Goyim-Arena/internal/wallet/domain"
)

// Clock exposes wall-clock time to wallet use cases, keeping them
// deterministic under test (ADR-012).
type Clock interface {
	Now() time.Time
}

// CreditRequest is a validated INK credit to persist atomically.
type CreditRequest struct {
	AccountID      domain.AccountID
	Bucket         domain.Bucket
	OperationType  domain.OperationType
	IdempotencyKey domain.IdempotencyKey
	Reference      domain.Reference
	// Delta is the signed ledger amount produced by the operation direction
	// (positive for credits).
	Delta     int64
	ChangedAt time.Time
}

// CreditResult is the outcome of a credit: the operation that the
// idempotency key resolves to, and whether it was a replay of an earlier
// attempt (no new transaction or balance change happened).
type CreditResult struct {
	Operation domain.Operation
	Replayed  bool
}

// CreditRepository persists credits and their idempotency registry.
type CreditRepository interface {
	// ApplyCredit atomically ensures the wallet, stores the operation under
	// its idempotency key, records the bucket transaction and updates the
	// balance. The same key resolves to the original operation untouched.
	ApplyCredit(ctx context.Context, request CreditRequest) (*CreditResult, error)
}
