package domain

import "strings"

// Slug is an immutable, validated public arena address. It is a friendly
// address, never an identity: the stable identifier is the arena id and the
// domain derives slugs from the statement at publication.
type Slug struct {
	value string
}

// ParseSlug validates and constructs a Slug: lowercase ASCII letters and
// digits separated by single hyphens, bounded by SlugMinLength and
// SlugMaxLength and never starting or ending with a hyphen.
func ParseSlug(raw string) (Slug, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Slug{}, ErrEmptySlug
	}
	if len(trimmed) < SlugMinLength || len(trimmed) > SlugMaxLength {
		return Slug{}, ErrInvalidSlug
	}

	previousHyphen := false
	for i := 0; i < len(trimmed); i++ {
		b := trimmed[i]
		switch {
		case b >= 'a' && b <= 'z', b >= '0' && b <= '9':
			previousHyphen = false
		case b == '-':
			if previousHyphen || i == 0 || i == len(trimmed)-1 {
				return Slug{}, ErrInvalidSlug
			}
			previousHyphen = true
		default:
			return Slug{}, ErrInvalidSlug
		}
	}

	return Slug{value: trimmed}, nil
}

// String returns the stored slug.
func (s Slug) String() string {
	return s.value
}

// IsZero reports whether the Slug is the uninitialized zero value.
func (s Slug) IsZero() bool {
	return s.value == ""
}

// Equals reports whether two slugs are identical.
func (s Slug) Equals(other Slug) bool {
	return s.value == other.value
}

// slugTransliterations maps common Latin letters used in pt-BR text to
// their ASCII base, so derived slugs stay readable without external Unicode
// normalization dependencies.
var slugTransliterations = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u",
	'ç': "c", 'ñ': "n", 'ý': "y", 'ÿ': "y",
}

// Slugify derives a deterministic slug base from text: lowercasing,
// transliterating common accented letters, collapsing every other character
// into single hyphens and trimming the edges. The result may be shorter than
// SlugMinLength (short or non-Latin statements) or longer than SlugMaxLength
// (long statements); the publication use case composes it with a stable
// suffix and validates the final slug with ParseSlug.
func Slugify(source string) string {
	var builder strings.Builder
	builder.Grow(len(source))

	pendingSeparator := false
	for _, r := range strings.ToLower(source) {
		mapped := ""
		if transliterated, ok := slugTransliterations[r]; ok {
			mapped = transliterated
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			mapped = string(r)
		}

		if mapped == "" {
			if builder.Len() > 0 {
				pendingSeparator = true
			}
			continue
		}
		if pendingSeparator {
			builder.WriteByte('-')
			pendingSeparator = false
		}
		builder.WriteString(mapped)
	}

	return builder.String()
}
