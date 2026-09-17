package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxGraphemeCost is the strict product limit: an argument carries at most
// 3.000 grapheme clusters (BUSINESS_RULES §4). The cost is also the billing
// unit: 1 INK per cluster, spaces, punctuation and line breaks included.
const MaxGraphemeCost = 3000

// contentHashVersion prefixes the canonical hash so the algorithm can
// evolve without invalidating stored rows.
const contentHashVersion = "v1"

// contentHashDomain separates argument content hashes from any other hash
// computed with the same digest.
const contentHashDomain = "goyim-arena/arguments/content/v1"

// GraphemeCounter counts extended grapheme clusters (Unicode UAX #29) of a
// string. The domain stays standard-library only (AGENTS.md): the concrete
// counter lives in internal/platform/text and is injected by the
// application, per ADR-013.
type GraphemeCounter func(value string) int

// ContentHash is the versioned canonical hash of argument content. It is
// stable across executions: the same normalized content always produces the
// same value.
type ContentHash struct {
	value string
}

// HashContent returns the versioned canonical hash of already normalized
// content: SHA-256 over a domain-separated payload, rendered as
// "v1:<64 hex chars>".
func HashContent(normalized string) ContentHash {
	sum := sha256.Sum256([]byte(contentHashDomain + "\x00" + normalized))
	return ContentHash{value: contentHashVersion + ":" + hex.EncodeToString(sum[:])}
}

// ParseContentHash validates and constructs a ContentHash from its stored
// canonical form.
func ParseContentHash(raw string) (ContentHash, error) {
	trimmed := strings.TrimSpace(raw)
	prefix := contentHashVersion + ":"
	if !strings.HasPrefix(trimmed, prefix) {
		return ContentHash{}, ErrInvalidContentHash
	}
	digest := trimmed[len(prefix):]
	if len(digest) != sha256.Size*2 {
		return ContentHash{}, ErrInvalidContentHash
	}
	for i := 0; i < len(digest); i++ {
		c := digest[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ContentHash{}, ErrInvalidContentHash
		}
	}
	return ContentHash{value: trimmed}, nil
}

// String returns the canonical hash value.
func (h ContentHash) String() string {
	return h.value
}

// IsZero reports whether the ContentHash is the uninitialized zero value.
func (h ContentHash) IsZero() bool {
	return h.value == ""
}

// Equals reports whether two hashes are identical.
func (h ContentHash) Equals(other ContentHash) bool {
	return h.value == other.value
}

// Content is the immutable plaintext of an argument together with its
// grapheme cost and canonical hash. It is normalized once, at construction:
// newlines become "\n", surrounding whitespace is trimmed, and the cost is
// counted over the normalized text by the injected UAX #29 counter.
type Content struct {
	value string
	cost  int
	hash  ContentHash
}

// ParseContent validates, normalizes and measures argument content.
func ParseContent(raw string, counter GraphemeCounter) (Content, error) {
	if counter == nil {
		return Content{}, ErrMissingGraphemeCounter
	}
	if !utf8.ValidString(raw) {
		return Content{}, ErrInvalidContent
	}

	normalized := normalizeContent(raw)
	if normalized == "" {
		return Content{}, ErrEmptyContent
	}
	if containsUnsupportedRunes(normalized) {
		return Content{}, ErrInvalidContent
	}

	cost := counter(normalized)
	if cost < 1 {
		return Content{}, ErrEmptyContent
	}
	if cost > MaxGraphemeCost {
		return Content{}, ErrContentTooLong
	}

	return Content{value: normalized, cost: cost, hash: HashContent(normalized)}, nil
}

// ReconstituteContent rebuilds stored content, validating the stored cost
// range and the recorded hash against a fresh canonical computation.
// Adapters use it to map rows; the grapheme counter is not needed because
// the cost was measured at publication.
func ReconstituteContent(value string, cost int, hash ContentHash) (Content, error) {
	if value == "" || strings.TrimSpace(value) == "" {
		return Content{}, ErrEmptyContent
	}
	if cost < 1 || cost > MaxGraphemeCost {
		return Content{}, ErrContentTooLong
	}
	if hash.IsZero() {
		return Content{}, ErrInvalidContentHash
	}
	if !HashContent(value).Equals(hash) {
		return Content{}, ErrContentHashMismatch
	}
	return Content{value: value, cost: cost, hash: hash}, nil
}

// String returns the normalized plaintext.
func (c Content) String() string {
	return c.value
}

// GraphemeCost returns the UAX #29 cluster count of the normalized text:
// the 1 INK per cluster billing unit.
func (c Content) GraphemeCost() int {
	return c.cost
}

// Hash returns the versioned canonical hash.
func (c Content) Hash() ContentHash {
	return c.hash
}

// IsZero reports whether the Content is the uninitialized zero value.
func (c Content) IsZero() bool {
	return c.value == ""
}

// Equals reports whether two contents are the same normalized text.
func (c Content) Equals(other Content) bool {
	return c.value == other.value
}

// normalizeContent canonicalizes line breaks and trims surrounding
// whitespace; inner text is preserved exactly.
func normalizeContent(raw string) string {
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
