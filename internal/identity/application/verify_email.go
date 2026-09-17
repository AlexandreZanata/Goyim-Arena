package application

import (
	"context"
	"crypto/sha256"
	"strings"
)

// VerifyEmailCommand holds the raw verification token provided by the user.
type VerifyEmailCommand struct {
	Token string
}

// VerifyEmailUseCase validates single-use tokens and promotes the account to Active.
type VerifyEmailUseCase struct {
	accounts AccountRepository
	tokens   VerificationTokenRepository
	clock    Clock
}

// NewVerifyEmailUseCase constructs a VerifyEmailUseCase.
func NewVerifyEmailUseCase(
	accounts AccountRepository,
	tokens VerificationTokenRepository,
	clock Clock,
) *VerifyEmailUseCase {
	return &VerifyEmailUseCase{
		accounts: accounts,
		tokens:   tokens,
		clock:    clock,
	}
}

// Execute validates the token, verifies the account email, and transitions account status.
func (uc *VerifyEmailUseCase) Execute(ctx context.Context, cmd VerifyEmailCommand) error {
	trimmedToken := strings.TrimSpace(cmd.Token)
	if trimmedToken == "" {
		return ErrInvalidToken
	}

	tokenHash := sha256.Sum256([]byte(trimmedToken))
	record, err := uc.tokens.GetVerificationToken(ctx, tokenHash[:])
	if err != nil {
		return err
	}

	now := uc.clock.Now()

	// Replay protection: token has already been consumed or superseded
	if record.UsedAt != nil {
		return ErrTokenAlreadyUsed
	}

	// Expiration check
	if now.After(record.ExpiresAt) {
		return ErrTokenExpired
	}

	account, err := uc.accounts.GetAccountByID(ctx, record.AccountID)
	if err != nil {
		return err
	}

	// Execute domain state mutation (enforces invariants)
	if err := account.VerifyEmail(now); err != nil {
		return err
	}

	// Consume token single-use
	if err := uc.tokens.MarkTokenUsed(ctx, record.ID, now); err != nil {
		return err
	}

	// Update account status in store
	return uc.accounts.SetEmailVerified(ctx, account.ID(), now)
}
