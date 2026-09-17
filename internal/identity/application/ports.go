// Package application defines the use cases, orchestrations, and consumer-oriented
// ports for the identity and authentication module.
package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/Goyim-Arena/internal/identity/domain"
)

// Clock exposes wall-clock time to identity application use cases.
type Clock interface {
	Now() time.Time
}

// Random exposes cryptographically secure random bytes to identity application use cases.
type Random interface {
	Read(buffer []byte) (int, error)
}

// PasswordHasher abstracts secure password hashing, constant-time verification,
// rehash detection for evolving security parameters, and dummy hashes for
// uniform login timing (mitigating user enumeration timing attacks per THR-AUTH-02).
type PasswordHasher interface {
	// HashPassword derives a secure Argon2id hash from a plain-text password using
	// a fresh cryptographically random salt and configured cost parameters.
	HashPassword(password string) (string, error)

	// VerifyPassword checks a plain-text password against an encoded Argon2id hash
	// using constant-time comparison to prevent timing side-channels.
	VerifyPassword(password, encodedHash string) (bool, error)

	// NeedsRehash reports whether an encoded hash was produced with parameters weaker
	// or different from the current hasher configuration, indicating the credential
	// should be re-hashed upon successful authentication.
	NeedsRehash(encodedHash string) bool

	// DummyHash returns a pre-computed, validly formatted Argon2id hash matching
	// current parameters to be used when an account does not exist, ensuring
	// identical CPU and memory consumption to prevent user enumeration (THR-AUTH-02).
	DummyHash() string
}

// AccountRepository defines persistent storage operations for accounts and credentials.
type AccountRepository interface {
	// CreateAccountWithPassword atomically creates an account in Pending status alongside its password credential.
	CreateAccountWithPassword(ctx context.Context, email domain.Email, passwordHash string) (*domain.Account, error)

	// GetAccountByEmail looks up an account by its case-insensitive email address.
	GetAccountByEmail(ctx context.Context, email domain.Email) (*domain.Account, error)

	// GetAccountByID looks up an account by its unique identifier.
	GetAccountByID(ctx context.Context, id domain.AccountID) (*domain.Account, error)

	// SetEmailVerified marks an account as Active and records the email verification timestamp.
	SetEmailVerified(ctx context.Context, id domain.AccountID, verifiedAt time.Time) error
}

// VerificationTokenRecord represents a stored single-use email verification token.
type VerificationTokenRecord struct {
	ID        string
	AccountID domain.AccountID
	TokenHash []byte
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// VerificationTokenRepository manages single-use email verification tokens.
type VerificationTokenRepository interface {
	// CreateVerificationToken stores a new cryptographic token hash for an account.
	CreateVerificationToken(ctx context.Context, accountID domain.AccountID, tokenHash []byte, expiresAt time.Time) error

	// GetVerificationToken retrieves a verification token record by its binary hash.
	GetVerificationToken(ctx context.Context, tokenHash []byte) (*VerificationTokenRecord, error)

	// MarkTokenUsed records that the token has been consumed, preventing replay.
	MarkTokenUsed(ctx context.Context, tokenID string, usedAt time.Time) error

	// InvalidateActiveTokens marks all existing unconsumed verification tokens for the account as used.
	InvalidateActiveTokens(ctx context.Context, accountID domain.AccountID) error
}

// EmailSender delivers or enqueues transactional verification and password recovery emails.
type EmailSender interface {
	// SendVerificationEmail delivers or enqueues an email containing the unhashed verification token.
	SendVerificationEmail(ctx context.Context, email domain.Email, token string) error

	// SendPasswordResetEmail delivers or enqueues an email containing the unhashed password reset token.
	SendPasswordResetEmail(ctx context.Context, email domain.Email, token string) error
}

// PasswordResetTokenRecord represents a stored single-use password recovery token.
type PasswordResetTokenRecord struct {
	ID        string
	AccountID domain.AccountID
	TokenHash []byte
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// PasswordResetTokenRepository manages single-use password recovery tokens.
type PasswordResetTokenRepository interface {
	// CreatePasswordResetToken stores a new cryptographic reset token hash for an account.
	CreatePasswordResetToken(ctx context.Context, accountID domain.AccountID, tokenHash []byte, expiresAt time.Time) error

	// GetPasswordResetToken retrieves a password reset token record by its binary hash.
	GetPasswordResetToken(ctx context.Context, tokenHash []byte) (*PasswordResetTokenRecord, error)

	// MarkPasswordResetTokenUsed records that the token has been consumed, preventing replay.
	MarkPasswordResetTokenUsed(ctx context.Context, tokenID string, usedAt time.Time) error

	// InvalidateActivePasswordResetTokens marks all existing unconsumed reset tokens for the account as used.
	InvalidateActivePasswordResetTokens(ctx context.Context, accountID domain.AccountID) error
}

// PasswordCredentialRecord represents a stored password credential.
type PasswordCredentialRecord struct {
	AccountID    domain.AccountID
	PasswordHash string
	Algorithm    string
	Version      int32
}

// PasswordCredentialRepository manages storage and updates of password credentials.
type PasswordCredentialRepository interface {
	// GetPasswordCredential retrieves the password credential record for an account.
	GetPasswordCredential(ctx context.Context, accountID domain.AccountID) (*PasswordCredentialRecord, error)

	// UpdatePasswordCredential updates the stored hash (e.g. during transparent rehash).
	UpdatePasswordCredential(ctx context.Context, accountID domain.AccountID, passwordHash string, algorithm string, version int32) error
}

// SessionRepository manages persistence, retrieval, and revocation of opaque user sessions.
type SessionRepository interface {
	// CreateSession stores a newly created session record and returns the reconstituted domain Session.
	CreateSession(ctx context.Context, accountID domain.AccountID, tokenHash []byte, expiresAt time.Time, ipAddress, userAgent string) (*domain.Session, error)

	// GetSessionByTokenHash retrieves a session by its SHA-256 token hash (including expired or revoked).
	GetSessionByTokenHash(ctx context.Context, tokenHash []byte) (*domain.Session, error)

	// TouchSession updates the last_seen_at and expires_at timestamps of an active session.
	TouchSession(ctx context.Context, id domain.SessionID, lastSeenAt, expiresAt time.Time) error

	// RevokeSession marks a single session as revoked by its token hash.
	RevokeSession(ctx context.Context, tokenHash []byte) error

	// RevokeAllAccountSessions revokes all active sessions for a given account.
	RevokeAllAccountSessions(ctx context.Context, accountID domain.AccountID) error
}
