// Package postgres is the outbound PostgreSQL adapter of the persuasion
// module (P11-T03). It joins the caller transaction when the context
// carries one, so the change lock, the set-level checks and the attribution
// inserts commit or roll back together.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/Goyim-Arena/internal/persuasion/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/persuasion/domain"
	platformpg "github.com/AlexandreZanata/Goyim-Arena/internal/platform/postgres"
)

// Repository implements the persuasion application ports using PostgreSQL.
type Repository struct {
	pool    *pgxpool.Pool
	queries *platformpg.Queries
}

var _ application.AttributionRepository = (*Repository)(nil)

// NewRepository creates a PostgreSQL repository adapter for persuasion.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: platformpg.New(pool),
	}
}

// queriesFor binds the queries to the caller transaction when one is
// carried by the context; the FOR UPDATE lock only means anything inside
// the shared transaction.
func (r *Repository) queriesFor(ctx context.Context) *platformpg.Queries {
	if tx, ok := platformpg.TxFromContext(ctx); ok {
		return r.queries.WithTx(tx)
	}
	return r.queries
}

// LockChangeForAttributor loads the position change scoped to its account
// and locks it FOR UPDATE.
func (r *Repository) LockChangeForAttributor(ctx context.Context, changeID domain.ChangeID, attributorID domain.AttributorID) (*domain.Change, error) {
	changeParam, ok := uuidParam(changeID.String())
	if !ok {
		return nil, application.ErrChangeNotFound
	}
	attributorParam, ok := uuidParam(attributorID.String())
	if !ok {
		return nil, application.ErrChangeNotFound
	}

	row, err := r.queriesFor(ctx).GetPositionChangeForAttributor(ctx, platformpg.GetPositionChangeForAttributorParams{
		ChangeID:     changeParam,
		AttributorID: attributorParam,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrChangeNotFound
		}
		return nil, fmt.Errorf("lock position change: %w", err)
	}

	arenaID, err := domain.ParseArenaID(uuidToString(row.ArenaID))
	if err != nil {
		return nil, fmt.Errorf("stored arena id is invalid: %w", err)
	}
	accountID, err := domain.ParseAttributorID(uuidToString(row.AccountID))
	if err != nil {
		return nil, fmt.Errorf("stored attributor id is invalid: %w", err)
	}
	return &domain.Change{
		ID:           changeID,
		ArenaID:      arenaID,
		AttributorID: accountID,
		ChangedAt:    row.ChangedAt.Time,
	}, nil
}

// ListAttributedArgumentIDs returns the arguments already credited by the
// change.
func (r *Repository) ListAttributedArgumentIDs(ctx context.Context, changeID domain.ChangeID) ([]domain.ArgumentID, error) {
	changeParam, ok := uuidParam(changeID.String())
	if !ok {
		return nil, application.ErrChangeNotFound
	}

	rows, err := r.queriesFor(ctx).ListAttributionArgumentIDs(ctx, changeParam)
	if err != nil {
		return nil, fmt.Errorf("list attributed arguments: %w", err)
	}

	ids := make([]domain.ArgumentID, 0, len(rows))
	for _, row := range rows {
		argumentID, err := domain.ParseArgumentID(uuidToString(row))
		if err != nil {
			return nil, fmt.Errorf("stored argument id is invalid: %w", err)
		}
		ids = append(ids, argumentID)
	}
	return ids, nil
}

// ListCandidates loads the eligibility inputs of the proposed arguments.
func (r *Repository) ListCandidates(ctx context.Context, argumentIDs []domain.ArgumentID) ([]domain.Candidate, error) {
	if len(argumentIDs) == 0 {
		return nil, nil
	}
	params := make([]pgtype.UUID, 0, len(argumentIDs))
	for _, argumentID := range argumentIDs {
		param, ok := uuidParam(argumentID.String())
		if !ok {
			return nil, application.ErrArgumentNotFound
		}
		params = append(params, param)
	}

	rows, err := r.queriesFor(ctx).ListAttributionCandidates(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list attribution candidates: %w", err)
	}
	if len(rows) != len(argumentIDs) {
		// At least one proposed identifier does not exist.
		return nil, application.ErrArgumentNotFound
	}

	candidates := make([]domain.Candidate, 0, len(rows))
	for _, row := range rows {
		argumentID, err := domain.ParseArgumentID(uuidToString(row.ID))
		if err != nil {
			return nil, fmt.Errorf("stored argument id is invalid: %w", err)
		}
		arenaID, err := domain.ParseArenaID(uuidToString(row.ArenaID))
		if err != nil {
			return nil, fmt.Errorf("stored arena id is invalid: %w", err)
		}
		authorID, err := domain.ParseAuthorID(uuidToString(row.AuthorID))
		if err != nil {
			return nil, fmt.Errorf("stored author id is invalid: %w", err)
		}
		status := domain.ArgumentStatus(row.Status)
		if !status.IsValid() {
			return nil, fmt.Errorf("stored argument status is invalid: %q", row.Status)
		}
		candidates = append(candidates, domain.Candidate{
			ID:        argumentID,
			ArenaID:   arenaID,
			AuthorID:  authorID,
			CreatedAt: row.CreatedAt.Time,
			Status:    status,
		})
	}
	return candidates, nil
}

// CreateAttributions records the accepted candidates under the unique
// (change, argument) pair.
func (r *Repository) CreateAttributions(ctx context.Context, changeID domain.ChangeID, attributorID domain.AttributorID, candidates []domain.Candidate) error {
	changeParam, ok := uuidParam(changeID.String())
	if !ok {
		return application.ErrChangeNotFound
	}
	attributorParam, ok := uuidParam(attributorID.String())
	if !ok {
		return application.ErrChangeNotFound
	}

	for _, candidate := range candidates {
		argumentParam, ok := uuidParam(candidate.ID.String())
		if !ok {
			return application.ErrArgumentNotFound
		}
		_, err := r.queriesFor(ctx).CreateAttribution(ctx, platformpg.CreateAttributionParams{
			ChangeID:     changeParam,
			AttributorID: attributorParam,
			ArgumentID:   argumentParam,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Already attributed by a concurrent replay: nothing to do.
				continue
			}
			return fmt.Errorf("create attribution: %w", err)
		}
	}
	return nil
}

// uuidParam parses a canonical UUID string into its database parameter.
func uuidParam(raw string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		return pgtype.UUID{}, false
	}
	return id, id.Valid
}

// uuidToString renders a database UUID in canonical form.
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}
