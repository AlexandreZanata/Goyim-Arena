// Package postgres is the PostgreSQL outbound adapter of the moderation
// module (P13-T02). It implements the application RoleRepository against
// the assignment schema (migrations 00022 and 00023): one row per account,
// revoked assignments returned (not hidden) so the caller distinguishes
// "never authorized" from "revoked".
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/Goyim-Arena/internal/moderation/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/moderation/domain"
	platformpg "github.com/AlexandreZanata/Goyim-Arena/internal/platform/postgres"
)

// Repository implements the moderation application ports using PostgreSQL.
type Repository struct {
	pool    *pgxpool.Pool
	queries *platformpg.Queries
}

var _ application.RoleRepository = (*Repository)(nil)

// NewRepository creates a PostgreSQL repository adapter for moderation.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: platformpg.New(pool),
	}
}

// AssignmentFor resolves the administrative assignment of the account, or
// nil when the account holds none. Revocation is carried, not filtered: a
// revoked role in an active session still denies.
func (r *Repository) AssignmentFor(ctx context.Context, accountID domain.AccountID) (*application.RoleAssignment, error) {
	if accountID.IsZero() {
		return nil, domain.ErrEmptyAccountID
	}
	pgUUID, err := pgUUIDFromAccountID(accountID)
	if err != nil {
		return nil, fmt.Errorf("load assignment: %w", err)
	}

	row, err := r.queries.GetAdminRoleByAccount(ctx, pgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load assignment: %w", err)
	}

	role, err := domain.ParseRole(row.Role)
	if err != nil {
		return nil, fmt.Errorf("stored role is invalid: %w", err)
	}

	return &application.RoleAssignment{
		AccountID: accountID,
		Role:      role,
		Revoked:   row.RevokedAt.Valid,
	}, nil
}

func pgUUIDFromAccountID(id domain.AccountID) (pgtype.UUID, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id.String()); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid account id format: %w", err)
	}
	return pgUUID, nil
}
