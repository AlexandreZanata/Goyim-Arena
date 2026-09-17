package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/Goyim-Arena/internal/identity/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/identity/domain"
	platformpg "github.com/AlexandreZanata/Goyim-Arena/internal/platform/postgres"
)

// Repository implements application.AccountRepository and application.VerificationTokenRepository.
type Repository struct {
	pool    *pgxpool.Pool
	queries *platformpg.Queries
}

var (
	_ application.AccountRepository           = (*Repository)(nil)
	_ application.VerificationTokenRepository = (*Repository)(nil)
)

// NewRepository creates a PostgreSQL repository adapter for identity.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: platformpg.New(pool),
	}
}

// CreateAccountWithPassword atomically creates an account in Pending status alongside its password credential.
func (r *Repository) CreateAccountWithPassword(ctx context.Context, email domain.Email, passwordHash string) (*domain.Account, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	accRow, err := qtx.CreateAccount(ctx, platformpg.CreateAccountParams{
		Email:  email.String(),
		Status: string(domain.AccountStatusPending),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, application.ErrDuplicateEmail
		}
		return nil, fmt.Errorf("create account: %w", err)
	}

	err = qtx.CreatePasswordCredential(ctx, platformpg.CreatePasswordCredentialParams{
		AccountID:    accRow.ID,
		PasswordHash: passwordHash,
		Algorithm:    "argon2id",
		Version:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("create password credential: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return mapAccountRow(accRow)
}

// GetAccountByEmail looks up an account by its case-insensitive email address.
func (r *Repository) GetAccountByEmail(ctx context.Context, email domain.Email) (*domain.Account, error) {
	row, err := r.queries.GetAccountByEmail(ctx, email.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrAccountNotFound
		}
		return nil, fmt.Errorf("get account by email: %w", err)
	}
	return mapAccountRow(row)
}

// GetAccountByID looks up an account by its unique identifier.
func (r *Repository) GetAccountByID(ctx context.Context, id domain.AccountID) (*domain.Account, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id.String()); err != nil {
		return nil, fmt.Errorf("invalid account id format: %w", err)
	}

	row, err := r.queries.GetAccountByID(ctx, pgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrAccountNotFound
		}
		return nil, fmt.Errorf("get account by id: %w", err)
	}
	return mapAccountRow(row)
}

// SetEmailVerified marks an account as Active and records the email verification timestamp.
func (r *Repository) SetEmailVerified(ctx context.Context, id domain.AccountID, verifiedAt time.Time) error {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id.String()); err != nil {
		return fmt.Errorf("invalid account id format: %w", err)
	}

	_, err := r.queries.SetEmailVerified(ctx, pgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrAccountNotFound
		}
		return fmt.Errorf("set email verified: %w", err)
	}
	return nil
}

// CreateVerificationToken stores a new cryptographic token hash for an account.
func (r *Repository) CreateVerificationToken(ctx context.Context, accountID domain.AccountID, tokenHash []byte, expiresAt time.Time) error {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(accountID.String()); err != nil {
		return fmt.Errorf("invalid account id format: %w", err)
	}

	_, err := r.queries.CreateEmailVerificationToken(ctx, platformpg.CreateEmailVerificationTokenParams{
		AccountID: pgUUID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}
	return nil
}

// GetVerificationToken retrieves a verification token record by its binary hash.
func (r *Repository) GetVerificationToken(ctx context.Context, tokenHash []byte) (*application.VerificationTokenRecord, error) {
	row, err := r.queries.GetEmailVerificationTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrInvalidToken
		}
		return nil, fmt.Errorf("get verification token: %w", err)
	}

	var usedAt *time.Time
	if row.UsedAt.Valid {
		t := row.UsedAt.Time
		usedAt = &t
	}

	return &application.VerificationTokenRecord{
		ID:        uuidToString(row.ID),
		AccountID: domain.AccountID(uuidToString(row.AccountID)),
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt.Time,
		UsedAt:    usedAt,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

// MarkTokenUsed records that the token has been consumed, preventing replay.
func (r *Repository) MarkTokenUsed(ctx context.Context, tokenID string, usedAt time.Time) error {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(tokenID); err != nil {
		return fmt.Errorf("invalid token id format: %w", err)
	}
	if err := r.queries.MarkEmailVerificationTokenUsed(ctx, pgUUID); err != nil {
		return fmt.Errorf("mark token used: %w", err)
	}
	return nil
}

// InvalidateActiveTokens marks all existing unconsumed verification tokens for the account as used.
func (r *Repository) InvalidateActiveTokens(ctx context.Context, accountID domain.AccountID) error {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(accountID.String()); err != nil {
		return fmt.Errorf("invalid account id format: %w", err)
	}
	if err := r.queries.InvalidateActiveEmailVerificationTokens(ctx, pgUUID); err != nil {
		return fmt.Errorf("invalidate active tokens: %w", err)
	}
	return nil
}

func mapAccountRow(row platformpg.AppAccount) (*domain.Account, error) {
	email, err := domain.ParseEmail(row.Email)
	if err != nil {
		return nil, fmt.Errorf("parse account email: %w", err)
	}

	var verifiedAt *time.Time
	if row.EmailVerifiedAt.Valid {
		t := row.EmailVerifiedAt.Time
		verifiedAt = &t
	}

	return domain.ReconstituteAccount(
		domain.AccountID(uuidToString(row.ID)),
		email,
		domain.AccountStatus(row.Status),
		verifiedAt,
		row.CreatedAt.Time,
		row.UpdatedAt.Time,
	)
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
