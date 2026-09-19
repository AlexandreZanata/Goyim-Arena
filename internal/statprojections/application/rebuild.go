// Package application coordinates rebuilds of derived public statistics.
// The normalized source tables remain authoritative; this package only defines
// the consumer-facing rebuild port and its bounded, resumable batches.
package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
)

const (
	ProjectionVersion              = 1
	DefaultBatchSize               = 100
	MaxBatchSize                   = 1000
	TransparencyMethodologyVersion = 1
)

var ErrInvalidRebuildConfig = errors.New("stat projections: invalid rebuild configuration")

var canonicalUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ArenaRebuildBatch is one short unit of work. NextCursor is empty when the
// source was exhausted; persisting it between invocations makes interruption
// safe without holding a transaction across the whole table.
type ArenaRebuildBatch struct {
	Processed  int
	NextCursor string
	Watermarks []time.Time
}

// Repository is the adapter port for projection rebuilds.
type Repository interface {
	RebuildArenaBatch(ctx context.Context, cursor string, limit int) (ArenaRebuildBatch, error)
	RebuildTransparency(ctx context.Context, start, end time.Time) error
}

// RebuildUseCase performs bounded rebuilds. Each call to RebuildArenaBatch is
// independently committed by the adapter, so writes are not blocked for the
// duration of a complete rebuild.
type RebuildUseCase struct {
	repository Repository
	batchSize  int
}

func NewRebuildUseCase(repository Repository, batchSize int) (*RebuildUseCase, error) {
	if repository == nil {
		return nil, ErrInvalidRebuildConfig
	}
	if batchSize == 0 {
		batchSize = DefaultBatchSize
	}
	if batchSize < 1 || batchSize > MaxBatchSize {
		return nil, fmt.Errorf("%w: batch size must be between 1 and %d", ErrInvalidRebuildConfig, MaxBatchSize)
	}
	return &RebuildUseCase{repository: repository, batchSize: batchSize}, nil
}

func (uc *RebuildUseCase) BatchSize() int { return uc.batchSize }

func (uc *RebuildUseCase) RebuildArenaBatch(ctx context.Context, cursor string) (ArenaRebuildBatch, error) {
	if uc == nil || uc.repository == nil {
		return ArenaRebuildBatch{}, ErrInvalidRebuildConfig
	}
	if cursor != "" && !canonicalUUID.MatchString(cursor) {
		return ArenaRebuildBatch{}, fmt.Errorf("%w: cursor must be a canonical UUID", ErrInvalidRebuildConfig)
	}
	return uc.repository.RebuildArenaBatch(ctx, cursor, uc.batchSize)
}

func (uc *RebuildUseCase) RebuildTransparency(ctx context.Context, start, end time.Time) error {
	if uc == nil || uc.repository == nil || start.IsZero() || end.IsZero() || !start.Before(end) {
		return ErrInvalidRebuildConfig
	}
	return uc.repository.RebuildTransparency(ctx, start.UTC(), end.UTC())
}
