// Package application defines the use cases, orchestrations and
// consumer-oriented ports of the arenas module.
package application

import (
	"context"
	"time"
)

// Clock exposes wall-clock time to arenas use cases, keeping them
// deterministic under test (ADR-012).
type Clock interface {
	Now() time.Time
}

// UnitOfWork runs a function inside one database transaction. Publication
// uses it so the Arena row and the consumed Arena Pass commit or roll back
// together. The concrete manager is composed at bootstrap; the module never
// imports another module's adapters.
type UnitOfWork interface {
	// WithinTransaction begins a transaction, makes it available to
	// participants through the context and commits only when fn returns nil.
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// PassConsumption identifies the Arena Pass consumption of one publication.
type PassConsumption struct {
	AccountID string
	ArenaID   string
}

// ConsumedPass reports the lot that provided the pass: the original lot and
// whether the call resolved an earlier consumption instead of consuming
// again.
type ConsumedPass struct {
	LotID     string
	Remaining int32
	Replayed  bool
}

// ArenaPassConsumer is the specific boundary of P07-T05: the arenas module
// consumes an Arena Pass without importing the billing module. The bridge
// adapter is composed at bootstrap and must join the UnitOfWork transaction,
// so a failed publication rolls the consumption back.
type ArenaPassConsumer interface {
	// ConsumeArenaPass consumes one pass of the publication author for the
	// Arena being published. It is only valid inside WithinTransaction.
	ConsumeArenaPass(ctx context.Context, request PassConsumption) (*ConsumedPass, error)
}
