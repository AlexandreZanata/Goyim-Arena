package domain

import (
	"time"
)

// ArenaID uniquely identifies an Arena.
type ArenaID string

// String returns the string representation of the arena identifier.
func (id ArenaID) String() string {
	return string(id)
}

// IsZero reports whether the ArenaID is uninitialized.
func (id ArenaID) IsZero() bool {
	return id == ""
}

// CreatorID identifies the account that created the Arena.
type CreatorID string

// String returns the string representation of the creator identifier.
func (id CreatorID) String() string {
	return string(id)
}

// IsZero reports whether the CreatorID is uninitialized.
func (id CreatorID) IsZero() bool {
	return id == ""
}

// ModeratorID identifies the acting moderator of a moderation decision. It
// is a distinct type so a creator id can never be passed as an actor by
// accident.
type ModeratorID string

// String returns the string representation of the moderator identifier.
func (id ModeratorID) String() string {
	return string(id)
}

// IsZero reports whether the ModeratorID is uninitialized.
func (id ModeratorID) IsZero() bool {
	return id == ""
}

// ArenaStatus represents the discrete lifecycle states of an Arena.
type ArenaStatus string

const (
	// ArenaStatusDraft is visible only to its creator: not listed publicly,
	// not consuming a pass.
	ArenaStatusDraft ArenaStatus = "draft"

	// ArenaStatusPublished is open to participation.
	ArenaStatusPublished ArenaStatus = "published"

	// ArenaStatusClosed is read-only: it accepts no new participation.
	ArenaStatusClosed ArenaStatus = "closed"

	// ArenaStatusRestricted stays accessible under a moderation decision
	// with limited interaction.
	ArenaStatusRestricted ArenaStatus = "restricted"

	// ArenaStatusRemoved is unavailable to the public by a recorded
	// decision; it is terminal in the MVP.
	ArenaStatusRemoved ArenaStatus = "removed"
)

// IsValid reports whether the status is an authorized enum value.
func (s ArenaStatus) IsValid() bool {
	switch s {
	case ArenaStatusDraft, ArenaStatusPublished, ArenaStatusClosed, ArenaStatusRestricted, ArenaStatusRemoved:
		return true
	default:
		return false
	}
}

// String returns the stored status value.
func (s ArenaStatus) String() string {
	return string(s)
}

// Arena is the aggregate root of the arenas module. Statement, language and
// provenance become immutable once the Arena leaves the draft state; every
// accepted mutation increments the optimistic version.
type Arena struct {
	id          ArenaID
	creatorID   CreatorID
	statement   Statement
	context     Context
	category    Category
	language    Language
	status      ArenaStatus
	slug        Slug
	version     int32
	createdAt   time.Time
	publishedAt *time.Time
	closesAt    *time.Time
}

// ReconstituteArena rebuilds an Arena from persistent state, validating the
// same invariants the database enforces. Adapters use it to map stored rows.
func ReconstituteArena(
	id ArenaID,
	creatorID CreatorID,
	statement Statement,
	context Context,
	category Category,
	language Language,
	status ArenaStatus,
	slug Slug,
	version int32,
	createdAt time.Time,
	publishedAt *time.Time,
	closesAt *time.Time,
) (*Arena, error) {
	if id.IsZero() {
		return nil, ErrEmptyArenaID
	}
	if creatorID.IsZero() {
		return nil, ErrEmptyCreatorID
	}
	if statement.IsZero() {
		return nil, ErrEmptyStatement
	}
	if category.IsZero() {
		return nil, ErrEmptyCategory
	}
	if language.IsZero() {
		return nil, ErrEmptyLanguage
	}
	if !status.IsValid() {
		return nil, ErrInvalidStatus
	}
	if version < 1 {
		return nil, ErrInvalidVersion
	}
	if status == ArenaStatusDraft {
		if !slug.IsZero() || publishedAt != nil {
			return nil, ErrInvalidStatusChange
		}
	} else if slug.IsZero() || publishedAt == nil {
		return nil, ErrInvalidStatusChange
	}
	if closesAt != nil {
		if publishedAt == nil || !closesAt.After(*publishedAt) {
			return nil, ErrInvalidCloseDate
		}
	}

	return &Arena{
		id:          id,
		creatorID:   creatorID,
		statement:   statement,
		context:     context,
		category:    category,
		language:    language,
		status:      status,
		slug:        slug,
		version:     version,
		createdAt:   createdAt.UTC(),
		publishedAt: copyInstant(publishedAt),
		closesAt:    copyInstant(closesAt),
	}, nil
}

// ID returns the arena identifier.
func (a *Arena) ID() ArenaID {
	return a.id
}

// CreatorID returns the account that created the Arena.
func (a *Arena) CreatorID() CreatorID {
	return a.creatorID
}

// Statement returns the immutable claim.
func (a *Arena) Statement() Statement {
	return a.statement
}

// Context returns the optional context; the zero value means none.
func (a *Arena) Context() Context {
	return a.context
}

// Category returns the editorial category reference.
func (a *Arena) Category() Category {
	return a.category
}

// Language returns the fixed content language.
func (a *Arena) Language() Language {
	return a.language
}

// Status returns the lifecycle status.
func (a *Arena) Status() ArenaStatus {
	return a.status
}

// Slug returns the public address; the zero value means the Arena is still a
// draft.
func (a *Arena) Slug() Slug {
	return a.slug
}

// Version returns the optimistic concurrency version.
func (a *Arena) Version() int32 {
	return a.version
}

// CreatedAt returns the creation instant.
func (a *Arena) CreatedAt() time.Time {
	return a.createdAt
}

// PublishedAt returns a copy of the publication instant, or nil for drafts.
func (a *Arena) PublishedAt() *time.Time {
	return copyInstant(a.publishedAt)
}

// ClosesAt returns a copy of the optional close instant.
func (a *Arena) ClosesAt() *time.Time {
	return copyInstant(a.closesAt)
}

// IsDraft reports whether the Arena is still a private draft.
func (a *Arena) IsDraft() bool {
	return a.status == ArenaStatusDraft
}

// AcceptsParticipation reports whether new participation is allowed: only
// published Arenas accept positions, arguments and changes. Closed,
// restricted, removed and draft Arenas reject every participation mutation.
func (a *Arena) AcceptsParticipation() bool {
	return a.status == ArenaStatusPublished
}

// EnsureAcceptsParticipation returns ErrArenaNotOpen when the Arena does not
// accept new participation.
func (a *Arena) EnsureAcceptsParticipation() error {
	if !a.AcceptsParticipation() {
		return ErrArenaNotOpen
	}
	return nil
}

// UpdateDraft applies the provided changes to a draft. Each value object is
// already validated; the transition only guards the draft state. The
// version is incremented once per accepted update.
func (a *Arena) UpdateDraft(statement *Statement, context *Context, category *Category, language *Language) error {
	if a.status != ArenaStatusDraft {
		return ErrArenaNotDraft
	}
	if statement != nil {
		a.statement = *statement
	}
	if context != nil {
		a.context = *context
	}
	if category != nil {
		a.category = *category
	}
	if language != nil {
		a.language = *language
	}
	a.version++
	return nil
}

// Publish transitions a draft to published, assigning the stable slug and
// the publication instant. Only drafts can be published.
func (a *Arena) Publish(slug Slug, now time.Time) error {
	if a.status != ArenaStatusDraft {
		return ErrInvalidStatusChange
	}
	if slug.IsZero() {
		return ErrMissingSlug
	}

	publishedAt := now.UTC()
	a.status = ArenaStatusPublished
	a.slug = slug
	a.publishedAt = &publishedAt
	a.version++
	return nil
}

// Close transitions a published Arena to closed. Reopening does not exist in
// the MVP.
func (a *Arena) Close() error {
	if a.status != ArenaStatusPublished {
		return ErrInvalidStatusChange
	}
	a.status = ArenaStatusClosed
	a.version++
	return nil
}

// Restrict transitions a published or closed Arena to restricted under a
// moderation decision.
func (a *Arena) Restrict() error {
	switch a.status {
	case ArenaStatusPublished, ArenaStatusClosed:
		a.status = ArenaStatusRestricted
		a.version++
		return nil
	default:
		return ErrInvalidStatusChange
	}
}

// Remove transitions a published, closed or restricted Arena to removed.
// Removed is terminal in the MVP.
func (a *Arena) Remove() error {
	switch a.status {
	case ArenaStatusPublished, ArenaStatusClosed, ArenaStatusRestricted:
		a.status = ArenaStatusRemoved
		a.version++
		return nil
	default:
		return ErrInvalidStatusChange
	}
}

// ScheduleClose sets the optional close instant of a published Arena. The
// instant must be after publication; the scheduled closing itself is applied
// by CloseIfDue.
func (a *Arena) ScheduleClose(closesAt time.Time) error {
	if a.status != ArenaStatusPublished || a.publishedAt == nil {
		return ErrInvalidStatusChange
	}
	instant := closesAt.UTC()
	if !instant.After(*a.publishedAt) {
		return ErrInvalidCloseDate
	}
	a.closesAt = &instant
	a.version++
	return nil
}

// ClearCloseSchedule removes the optional close instant of a published
// Arena.
func (a *Arena) ClearCloseSchedule() error {
	if a.status != ArenaStatusPublished {
		return ErrInvalidStatusChange
	}
	a.closesAt = nil
	a.version++
	return nil
}

// CloseIfDue closes a published Arena whose scheduled instant has been
// reached. It reports whether the transition happened; the operation is
// idempotent because a closed Arena is no longer published.
func (a *Arena) CloseIfDue(now time.Time) (bool, error) {
	if a.status != ArenaStatusPublished || a.closesAt == nil {
		return false, nil
	}
	if now.UTC().Before(*a.closesAt) {
		return false, nil
	}
	a.status = ArenaStatusClosed
	a.version++
	return true, nil
}

func copyInstant(instant *time.Time) *time.Time {
	if instant == nil {
		return nil
	}
	copied := instant.UTC()
	return &copied
}
