package domain

import "strings"

// Position is the immutable opinion of one account about one Arena. The
// vocabulary is closed: agree, disagree and undecided. A position carries no
// plan, payment or reputation weight: such data never reaches this domain.
type Position struct {
	value string
}

const (
	// PositionAgree means the participant agrees with the Arena statement.
	PositionAgree = "agree"

	// PositionDisagree means the participant disagrees with the statement.
	PositionDisagree = "disagree"

	// PositionUndecided means the participant has not taken a side.
	PositionUndecided = "undecided"
)

// ParsePosition validates and constructs a Position from its canonical
// lowercase value.
func ParsePosition(raw string) (Position, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Position{}, ErrEmptyPosition
	}
	switch trimmed {
	case PositionAgree, PositionDisagree, PositionUndecided:
		return Position{value: trimmed}, nil
	default:
		return Position{}, ErrInvalidPosition
	}
}

// SupportedPositions returns every position value in canonical order.
func SupportedPositions() []Position {
	return []Position{
		{value: PositionAgree},
		{value: PositionDisagree},
		{value: PositionUndecided},
	}
}

// String returns the stored position value.
func (p Position) String() string {
	return p.value
}

// IsZero reports whether the Position is the uninitialized zero value.
func (p Position) IsZero() bool {
	return p.value == ""
}

// IsSupported reports whether the Position carries an authorized value.
func (p Position) IsSupported() bool {
	switch p.value {
	case PositionAgree, PositionDisagree, PositionUndecided:
		return true
	default:
		return false
	}
}

// Equals reports whether two positions are the same opinion.
func (p Position) Equals(other Position) bool {
	return p.value == other.value
}
