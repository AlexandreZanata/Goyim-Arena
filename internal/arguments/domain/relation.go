package domain

import "strings"

// Relation is the declared stance of an argument towards the Arena
// statement. The vocabulary is closed and mirrors the schema check:
// support, oppose or context (a favor, contra ou contextual).
type Relation struct {
	value string
}

const (
	// RelationSupport agrees with the statement.
	RelationSupport = "support"

	// RelationOppose disagrees with the statement.
	RelationOppose = "oppose"

	// RelationContext adds context without taking a side.
	RelationContext = "context"
)

// ParseRelation validates and constructs a Relation from its canonical
// lowercase value.
func ParseRelation(raw string) (Relation, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Relation{}, ErrEmptyRelation
	}
	switch trimmed {
	case RelationSupport, RelationOppose, RelationContext:
		return Relation{value: trimmed}, nil
	default:
		return Relation{}, ErrInvalidRelation
	}
}

// SupportedRelations returns every relation in canonical order.
func SupportedRelations() []Relation {
	return []Relation{
		{value: RelationSupport},
		{value: RelationOppose},
		{value: RelationContext},
	}
}

// String returns the stored relation value.
func (r Relation) String() string {
	return r.value
}

// IsZero reports whether the Relation is the uninitialized zero value.
func (r Relation) IsZero() bool {
	return r.value == ""
}

// IsSupported reports whether the Relation carries an authorized value.
func (r Relation) IsSupported() bool {
	switch r.value {
	case RelationSupport, RelationOppose, RelationContext:
		return true
	default:
		return false
	}
}

// Equals reports whether two relations are the same stance.
func (r Relation) Equals(other Relation) bool {
	return r.value == other.value
}
