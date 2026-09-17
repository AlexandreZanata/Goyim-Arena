package application

import (
	"context"

	"github.com/AlexandreZanata/Goyim-Arena/internal/arenas/domain"
)

// CreateArenaRequest is a validated draft to persist.
type CreateArenaRequest struct {
	CreatorID domain.CreatorID
	Statement domain.Statement
	Context   domain.Context
	Category  domain.Category
	Language  domain.Language
}

// DraftUpdate replaces the mutable fields of a draft under an optimistic
// version check.
type DraftUpdate struct {
	Statement       domain.Statement
	Context         domain.Context
	Category        domain.Category
	Language        domain.Language
	ExpectedVersion int32
}

// ArenaRepository persists Arenas and their private drafts. Every mutation
// is scoped by the creator; the repository never exposes a foreign draft.
type ArenaRepository interface {
	// CreateArena stores a new draft and returns the reconstituted entity.
	CreateArena(ctx context.Context, request CreateArenaRequest) (*domain.Arena, error)

	// GetArenaForCreator returns the Arena owned by the creator, or
	// ErrArenaNotFound.
	GetArenaForCreator(ctx context.Context, arenaID domain.ArenaID, creatorID domain.CreatorID) (*domain.Arena, error)

	// ListArenaDraftsForCreator returns the creator's drafts, newest first.
	ListArenaDraftsForCreator(ctx context.Context, creatorID domain.CreatorID) ([]domain.Arena, error)

	// UpdateArenaDraft replaces the draft fields under the optimistic
	// version check. It returns ErrVersionConflict when the stored version
	// moved, ErrArenaNotFound when the draft is missing or foreign and
	// domain.ErrArenaNotDraft when the Arena already left the draft state.
	UpdateArenaDraft(ctx context.Context, arenaID domain.ArenaID, creatorID domain.CreatorID, update DraftUpdate) (*domain.Arena, error)

	// DeleteArenaDraft removes a draft owned by the creator. It returns
	// ErrArenaNotFound when the draft is missing or foreign and
	// domain.ErrArenaNotDraft when the Arena already left the draft state.
	DeleteArenaDraft(ctx context.Context, arenaID domain.ArenaID, creatorID domain.CreatorID) error
}
