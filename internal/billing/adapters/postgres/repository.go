// Package postgres is the PostgreSQL outbound adapter of the billing module.
// It implements the Arena Pass grant port against the entitlement schema
// (migration 00011): the grant is inserted with ON CONFLICT DO NOTHING on
// (account_id, origin, reference), so retries resolve the original lot
// untouched.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/Goyim-Arena/internal/billing/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/billing/domain"
	platformpg "github.com/AlexandreZanata/Goyim-Arena/internal/platform/postgres"
)

// Repository implements the billing application ports using PostgreSQL.
type Repository struct {
	pool    *pgxpool.Pool
	queries *platformpg.Queries
}

var _ application.PassLotRepository = (*Repository)(nil)

// NewRepository creates a PostgreSQL repository adapter for billing.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: platformpg.New(pool),
	}
}

// GrantPassLot inserts the lot exactly once per (account, origin,
// reference). A duplicate grant writes nothing and resolves the original
// lot, including its immutable expiration.
func (r *Repository) GrantPassLot(ctx context.Context, request application.GrantPassLotRequest) (*application.GrantPassLotResult, error) {
	pgUUID, err := pgUUIDFromAccountID(request.AccountID)
	if err != nil {
		return nil, fmt.Errorf("grant pass lot: %w", err)
	}

	row, err := r.queries.CreateArenaPassLotIfAbsent(ctx, platformpg.CreateArenaPassLotIfAbsentParams{
		AccountID:         pgUUID,
		Origin:            request.Origin.String(),
		Quantity:          request.Quantity.Int32(),
		RemainingQuantity: request.Quantity.Int32(),
		ExpiresAt:         timestamptz(request.ExpiresAt),
		Reference:         request.Reference.String(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := r.queries.GetArenaPassLotByGrant(ctx, platformpg.GetArenaPassLotByGrantParams{
			AccountID: pgUUID,
			Origin:    request.Origin.String(),
			Reference: request.Reference.String(),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errors.New("grant conflict without a stored lot")
			}
			return nil, fmt.Errorf("load replayed pass lot: %w", err)
		}
		lot, err := mapPassLotRow(existing)
		if err != nil {
			return nil, err
		}
		return &application.GrantPassLotResult{Lot: *lot, Replayed: true}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("create pass lot: %w", err)
	}

	lot, err := mapPassLotRow(row)
	if err != nil {
		return nil, err
	}
	return &application.GrantPassLotResult{Lot: *lot}, nil
}

func mapPassLotRow(row platformpg.AppArenaPassLot) (*domain.PassLot, error) {
	origin, err := domain.ParsePassOrigin(row.Origin)
	if err != nil {
		return nil, fmt.Errorf("stored pass origin is invalid: %w", err)
	}
	quantity, err := domain.NewQuantity(row.Quantity)
	if err != nil {
		return nil, fmt.Errorf("stored pass quantity is invalid: %w", err)
	}
	reference, err := domain.ParseReference(row.Reference)
	if err != nil {
		return nil, fmt.Errorf("stored pass reference is invalid: %w", err)
	}

	var expiresAt *time.Time
	if row.ExpiresAt.Valid {
		instant := row.ExpiresAt.Time.UTC()
		expiresAt = &instant
	}

	return domain.ReconstitutePassLot(
		domain.LotID(uuidToString(row.ID)),
		domain.AccountID(uuidToString(row.AccountID)),
		origin,
		quantity,
		row.RemainingQuantity,
		expiresAt,
		reference,
		row.CreatedAt.Time,
	)
}

func pgUUIDFromAccountID(id domain.AccountID) (pgtype.UUID, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id.String()); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid account id format: %w", err)
	}
	return pgUUID, nil
}

func timestamptz(instant *time.Time) pgtype.Timestamptz {
	if instant == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: instant.UTC(), Valid: true}
}

func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}
