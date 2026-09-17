package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Statement is an immutable, validated Arena claim. Structure only:
// trimming, newline canonicalization, printable-text checks and versioned
// length bounds. The domain never judges the content itself.
type Statement struct {
	value string
}

// ParseStatement validates, normalizes and constructs a Statement. The
// input is trimmed, CRLF/CR are canonicalized to LF, control characters and
// bidirectional overrides are rejected (multi-line text is supported) and
// the length is bounded by the versioned policy in Unicode runes.
func ParseStatement(raw string, policy StatementPolicy) (Statement, error) {
	if !policy.IsValid() {
		return Statement{}, ErrInvalidPolicy
	}
	if !utf8.ValidString(raw) {
		return Statement{}, ErrInvalidStatement
	}

	normalized := normalizeText(raw)
	if normalized == "" {
		return Statement{}, ErrEmptyStatement
	}

	length := utf8.RuneCountInString(normalized)
	if length < policy.MinLength {
		return Statement{}, ErrStatementTooShort
	}
	if length > policy.MaxLength {
		return Statement{}, ErrStatementTooLong
	}
	if containsUnsupportedRunes(normalized) {
		return Statement{}, ErrInvalidStatement
	}

	return Statement{value: normalized}, nil
}

// String returns the normalized statement.
func (s Statement) String() string {
	return s.value
}

// RuneCount returns the statement length in Unicode runes.
func (s Statement) RuneCount() int {
	return utf8.RuneCountInString(s.value)
}

// IsZero reports whether the Statement is the uninitialized zero value.
func (s Statement) IsZero() bool {
	return s.value == ""
}

// Equals reports whether two statements are identical.
func (s Statement) Equals(other Statement) bool {
	return s.value == other.value
}

// Context is an immutable, optional Arena context. The zero value means the
// creator provided none.
type Context struct {
	value string
}

// ParseContext validates and normalizes an optional context. An empty or
// whitespace-only input yields the zero Context; anything else follows the
// same structural rules as the statement with the context length bound.
func ParseContext(raw string, policy StatementPolicy) (Context, error) {
	if !policy.IsValid() {
		return Context{}, ErrInvalidPolicy
	}
	if !utf8.ValidString(raw) {
		return Context{}, ErrInvalidContext
	}

	normalized := normalizeText(raw)
	if normalized == "" {
		return Context{}, nil
	}
	if utf8.RuneCountInString(normalized) > policy.ContextMaxLength {
		return Context{}, ErrContextTooLong
	}
	if containsUnsupportedRunes(normalized) {
		return Context{}, ErrInvalidContext
	}

	return Context{value: normalized}, nil
}

// String returns the normalized context, or an empty string when unset.
func (c Context) String() string {
	return c.value
}

// IsZero reports whether the context is unset.
func (c Context) IsZero() bool {
	return c.value == ""
}

// Equals reports whether two contexts are identical.
func (c Context) Equals(other Context) bool {
	return c.value == other.value
}

// normalizeText trims surrounding whitespace and canonicalizes line endings
// to LF so equal multi-line text always has one representation.
func normalizeText(raw string) string {
	replaced := strings.ReplaceAll(raw, "\r\n", "\n")
	replaced = strings.ReplaceAll(replaced, "\r", "\n")
	return strings.TrimSpace(replaced)
}

// containsUnsupportedRunes reports whether the text carries control
// characters (newline excepted) or bidirectional overrides that could
// reorder displayed content.
func containsUnsupportedRunes(value string) bool {
	for _, r := range value {
		if isBidiOverride(r) {
			return true
		}
		if r == '\n' {
			continue
		}
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// isBidiOverride reports whether the rune is a bidirectional control.
func isBidiOverride(r rune) bool {
	switch r {
	case '\u200E', '\u200F', '\u061C',
		'\u202A', '\u202B', '\u202C', '\u202D', '\u202E',
		'\u2066', '\u2067', '\u2068', '\u2069':
		return true
	default:
		return false
	}
}
