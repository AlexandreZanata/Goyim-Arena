// Package postgres is the PostgreSQL outbound adapter of the arenas module.
// It implements the private draft repository against the immutable Arena
// schema (migration 00012): every statement is scoped by the creator, and
// writes use the optimistic version check. Drafts never touch the Arena
// Pass ledger.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/Goyim-Arena/internal/arenas/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/arenas/domain"
	platformpg "github.com/AlexandreZanata/Goyim-Arena/internal/platform/postgres"
)

// Repository implements the arenas application ports using PostgreSQL.
type Repository struct {
	pool    *pgxpool.Pool
	queries *platformpg.Queries
}

var _ application.ArenaRepository = (*Repository)(nil)

// NewRepository creates a PostgreSQL repository adapter for arenas.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: platformpg.New(pool),
	}
}

// CreateArena stores a new private draft.
func (r *Repository) CreateArena(ctx context.Context, request application.CreateArenaRequest) (*domain.Arena, error) {
	creatorUUID, err := pgUUIDFromCreatorID(request.CreatorID)
	if err != nil {
		return nil, fmt.Errorf("create arena: %w", err)
	}

	row, err := r.queries.CreateArena(ctx, platformpg.CreateArenaParams{
		CreatorID: creatorUUID,
		Statement: request.Statement.String(),
		Context:   textFromContext(request.Context),
		Category:  request.Category.String(),
		Language:  request.Language.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("create arena: %w", err)
	}
	return mapArenaRow(row)
}

// GetArenaForCreator returns the Arena owned by the creator; foreign and
// missing ids are indistinguishable.
func (r *Repository) GetArenaForCreator(ctx context.Context, arenaID domain.ArenaID, creatorID domain.CreatorID) (*domain.Arena, error) {
	arenaUUID, creatorUUID, err := arenaScope(arenaID, creatorID)
	if err != nil {
		return nil, err
	}

	row, err := r.queries.GetArenaForCreator(ctx, platformpg.GetArenaForCreatorParams{
		ID:        arenaUUID,
		CreatorID: creatorUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrArenaNotFound
		}
		return nil, fmt.Errorf("get arena for creator: %w", err)
	}
	return mapArenaRow(row)
}

// ListArenaDraftsForCreator returns the creator's drafts, newest first.
func (r *Repository) ListArenaDraftsForCreator(ctx context.Context, creatorID domain.CreatorID) ([]domain.Arena, error) {
	creatorUUID, err := pgUUIDFromCreatorID(creatorID)
	if err != nil {
		return nil, fmt.Errorf("list arena drafts: %w", err)
	}

	rows, err := r.queries.ListArenaDraftsForCreator(ctx, creatorUUID)
	if err != nil {
		return nil, fmt.Errorf("list arena drafts: %w", err)
	}

	drafts := make([]domain.Arena, 0, len(rows))
	for _, row := range rows {
		arena, err := mapArenaRow(row)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, *arena)
	}
	return drafts, nil
}

// UpdateArenaDraft replaces the draft fields under the optimistic version
// check; a stale version affects no row.
func (r *Repository) UpdateArenaDraft(ctx context.Context, arenaID domain.ArenaID, creatorID domain.CreatorID, update application.DraftUpdate) (*domain.Arena, error) {
	arenaUUID, creatorUUID, err := arenaScope(arenaID, creatorID)
	if err != nil {
		return nil, err
	}

	row, err := r.queries.UpdateArenaDraft(ctx, platformpg.UpdateArenaDraftParams{
		ID:        arenaUUID,
		CreatorID: creatorUUID,
		Statement: update.Statement.String(),
		Context:   textFromContext(update.Context),
		Category:  update.Category.String(),
		Language:  update.Language.String(),
		Version:   update.ExpectedVersion,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, r.diagnoseDraftMiss(ctx, arenaUUID, creatorUUID, update.ExpectedVersion)
		}
		return nil, fmt.Errorf("update arena draft: %w", err)
	}
	return mapArenaRow(row)
}

// DeleteArenaDraft removes a draft owned by the creator.
func (r *Repository) DeleteArenaDraft(ctx context.Context, arenaID domain.ArenaID, creatorID domain.CreatorID) error {
	arenaUUID, creatorUUID, err := arenaScope(arenaID, creatorID)
	if err != nil {
		return err
	}

	rows, err := r.queries.DeleteArenaDraft(ctx, platformpg.DeleteArenaDraftParams{
		ID:        arenaUUID,
		CreatorID: creatorUUID,
	})
	if err != nil {
		return fmt.Errorf("delete arena draft: %w", err)
	}
	if rows == 1 {
		return nil
	}
	return r.diagnoseDraftMiss(ctx, arenaUUID, creatorUUID, 0)
}

// diagnoseDraftMiss explains why a scoped draft write affected no row:
// missing or foreign Arena, a non-draft state or a concurrent change.
func (r *Repository) diagnoseDraftMiss(ctx context.Context, arenaUUID, creatorUUID pgtype.UUID, expectedVersion int32) error {
	state, err := r.queries.GetArenaStateForCreator(ctx, platformpg.GetArenaStateForCreatorParams{
		ID:        arenaUUID,
		CreatorID: creatorUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrArenaNotFound
		}
		return fmt.Errorf("diagnose arena draft write: %w", err)
	}

	if state.Status != string(domain.ArenaStatusDraft) {
		return domain.ErrArenaNotDraft
	}
	if expectedVersion > 0 && state.Version != expectedVersion {
		return application.ErrVersionConflict
	}
	// A draft with the expected version rejected the write: a concurrent
	// transaction changed it between our statements.
	return application.ErrVersionConflict
}

func mapArenaRow(row platformpg.AppArena) (*domain.Arena, error) {
	policy := domain.ReconstitutionPolicy()

	statement, err := domain.ParseStatement(row.Statement, policy)
	if err != nil {
		return nil, fmt.Errorf("stored arena statement is invalid: %w", err)
	}

	context := domain.Context{}
	if row.Context.Valid {
		context, err = domain.ParseContext(row.Context.String, policy)
		if err != nil {
			return nil, fmt.Errorf("stored arena context is invalid: %w", err)
		}
	}

	category, err := domain.ParseCategory(row.Category)
	if err != nil {
		return nil, fmt.Errorf("stored arena category is invalid: %w", err)
	}
	language, err := domain.ParseLanguage(row.Language)
	if err != nil {
		return nil, fmt.Errorf("stored arena language is invalid: %w", err)
	}

	slug := domain.Slug{}
	if row.Slug.Valid {
		slug, err = domain.ParseSlug(row.Slug.String)
		if err != nil {
			return nil, fmt.Errorf("stored arena slug is invalid: %w", err)
		}
	}

	return domain.ReconstituteArena(
		domain.ArenaID(uuidToString(row.ID)),
		domain.CreatorID(uuidToString(row.CreatorID)),
		statement,
		context,
		category,
		language,
		domain.ArenaStatus(row.Status),
		slug,
		row.Version,
		row.CreatedAt.Time,
		timePtr(row.PublishedAt),
		timePtr(row.ClosesAt),
	)
}

func arenaScope(arenaID domain.ArenaID, creatorID domain.CreatorID) (pgtype.UUID, pgtype.UUID, error) {
	arenaUUID, err := pgUUIDFromArenaID(arenaID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, application.ErrArenaNotFound
	}
	creatorUUID, err := pgUUIDFromCreatorID(creatorID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, application.ErrArenaNotFound
	}
	return arenaUUID, creatorUUID, nil
}

func pgUUIDFromArenaID(id domain.ArenaID) (pgtype.UUID, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id.String()); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid arena id format: %w", err)
	}
	return pgUUID, nil
}

func pgUUIDFromCreatorID(id domain.CreatorID) (pgtype.UUID, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id.String()); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid creator id format: %w", err)
	}
	return pgUUID, nil
}

func textFromContext(context domain.Context) pgtype.Text {
	if context.IsZero() {
		return pgtype.Text{}
	}
	return pgtype.Text{String: context.String(), Valid: true}
}

func timePtr(instant pgtype.Timestamptz) *time.Time {
	if !instant.Valid {
		return nil
	}
	copied := instant.Time.UTC()
	return &copied
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
