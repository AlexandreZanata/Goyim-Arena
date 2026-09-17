package domain

import (
	"strings"
	"unicode/utf8"
)

// The content languages supported by the MVP, in canonical BCP 47 form. The
// allowlist mirrors the CHECK constraint of app.arenas; it is independent
// from the interface locale and never changed by it.
const (
	LanguagePortuguese = "pt-BR"
	LanguageEnglish    = "en-US"
)

var supportedLanguages = map[string]struct{}{
	LanguagePortuguese: {},
	LanguageEnglish:    {},
}

// Language is an immutable value object for the Arena content language.
// Arena content is never translated automatically: the language is fixed at
// publication (docs/MVP.md §3).
type Language struct {
	tag string
}

// ParseLanguage canonicalizes and validates an Arena language. The input is
// trimmed and canonicalized as a language tag with an optional region, then
// checked against the supported allowlist; unknown values are rejected
// instead of being reflected back.
func ParseLanguage(raw string) (Language, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Language{}, ErrEmptyLanguage
	}

	canonical, err := canonicalizeLanguageTag(trimmed)
	if err != nil {
		return Language{}, err
	}
	if _, ok := supportedLanguages[canonical]; !ok {
		return Language{}, ErrUnsupportedLanguage
	}
	return Language{tag: canonical}, nil
}

// SupportedLanguages returns every content language the product accepts.
func SupportedLanguages() []Language {
	return []Language{{tag: LanguagePortuguese}, {tag: LanguageEnglish}}
}

// canonicalizeLanguageTag accepts language plus optional region and rejects
// scripts, variants, extensions, private use and non-ASCII input.
func canonicalizeLanguageTag(value string) (string, error) {
	for i := 0; i < len(value); i++ {
		if value[i] >= utf8.RuneSelf {
			return "", ErrInvalidLanguage
		}
	}
	if value == "*" {
		return "", ErrInvalidLanguage
	}

	subtags := strings.Split(value, "-")
	if len(subtags) > 2 {
		return "", ErrInvalidLanguage
	}

	language := strings.ToLower(subtags[0])
	if len(language) < 2 || len(language) > 3 || !isASCIIAlpha(language) {
		return "", ErrInvalidLanguage
	}
	canonical := language

	if len(subtags) == 2 {
		region := subtags[1]
		switch {
		case len(region) == 2 && isASCIIAlpha(region):
			canonical += "-" + strings.ToUpper(region)
		case len(region) == 3 && isASCIIDigit(region):
			canonical += "-" + region
		default:
			return "", ErrInvalidLanguage
		}
	}

	return canonical, nil
}

func isASCIIAlpha(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		b := value[i]
		if (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') {
			return false
		}
	}
	return true
}

func isASCIIDigit(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

// String returns the canonical language tag.
func (l Language) String() string {
	return l.tag
}

// IsZero reports whether the Language is the uninitialized zero value.
func (l Language) IsZero() bool {
	return l.tag == ""
}

// Equals reports whether two languages are the same canonical tag.
func (l Language) Equals(other Language) bool {
	return l.tag == other.tag
}
