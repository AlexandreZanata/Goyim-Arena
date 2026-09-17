package domain

// StatementPolicy carries the versioned text limits of Arena statements and
// contexts. The values are configuration (injected at bootstrap) and never
// content judgments: the domain validates structure and size only.
type StatementPolicy struct {
	// Version identifies the configuration revision the limits came from.
	Version string
	// MinLength and MaxLength bound the statement length in Unicode runes.
	MinLength int
	MaxLength int
	// ContextMaxLength bounds the optional context in Unicode runes.
	ContextMaxLength int
}

// DefaultStatementPolicy returns the initial versioned limits of the MVP.
func DefaultStatementPolicy() StatementPolicy {
	return StatementPolicy{
		Version:          "2026-09",
		MinLength:        10,
		MaxLength:        280,
		ContextMaxLength: 2000,
	}
}

// IsValid reports whether the policy is internally coherent.
func (p StatementPolicy) IsValid() bool {
	return p.MinLength > 0 && p.MaxLength >= p.MinLength && p.ContextMaxLength > 0
}

// Slug format bounds; they mirror the CHECK constraint of app.arenas.
const (
	SlugMinLength = 3
	SlugMaxLength = 80
)

// Category format bounds; they mirror the CHECK constraint of
// app.categories.
const (
	CategoryMinLength = 2
	CategoryMaxLength = 40
)
