package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Source structural bounds; they mirror the CHECK constraints of
// app.argument_sources.
const (
	SourceURLMinLength         = 8
	SourceURLMaxLength         = 2048
	SourceDescriptionMaxLength = 500
)

// Source is one structured source supporting an argument: an absolute
// http(s) URL plus an optional short description. A source supports a claim
// without certifying it (BUSINESS_RULES §4.2).
type Source struct {
	url         string
	description string
}

// ParseSource validates and constructs a Source. The scheme is canonicalized
// to lowercase; the description is trimmed and empty means absent.
func ParseSource(rawURL, rawDescription string) (Source, error) {
	url, err := parseSourceURL(rawURL)
	if err != nil {
		return Source{}, err
	}
	description, err := parseSourceDescription(rawDescription)
	if err != nil {
		return Source{}, err
	}
	return Source{url: url, description: description}, nil
}

// ReconstituteSource rebuilds a stored source, validating the same
// structural bounds without re-canonicalizing.
func ReconstituteSource(url, description string) (Source, error) {
	if url == "" {
		return Source{}, ErrEmptySourceURL
	}
	if len(url) < SourceURLMinLength || len(url) > SourceURLMaxLength {
		return Source{}, ErrInvalidSourceURL
	}
	if description != "" {
		if utf8.RuneCountInString(description) > SourceDescriptionMaxLength {
			return Source{}, ErrSourceDescriptionTooLong
		}
		if containsUnsupportedRunes(description) {
			return Source{}, ErrInvalidContent
		}
	}
	return Source{url: url, description: description}, nil
}

// URL returns the canonical absolute URL.
func (s Source) URL() string {
	return s.url
}

// Description returns the short description, or an empty string when none
// was declared.
func (s Source) Description() string {
	return s.description
}

// HasDescription reports whether a description was declared.
func (s Source) HasDescription() bool {
	return s.description != ""
}

// IsZero reports whether the Source is the uninitialized zero value.
func (s Source) IsZero() bool {
	return s.url == ""
}

// Equals reports whether two sources are the same URL and description.
func (s Source) Equals(other Source) bool {
	return s.url == other.url && s.description == other.description
}

// parseSourceURL validates an absolute http(s) address within the
// structural bounds. The domain never parses URLs as transport data
// (net/url is forbidden here): it enforces scheme, host presence and the
// absence of whitespace, mirroring the database check.
func parseSourceURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrEmptySourceURL
	}

	separator := strings.Index(trimmed, "://")
	if separator <= 0 {
		return "", ErrInvalidSourceURL
	}
	scheme := strings.ToLower(trimmed[:separator])
	if scheme != "http" && scheme != "https" {
		return "", ErrInvalidSourceURL
	}
	canonical := scheme + trimmed[separator:]

	if len(canonical) < SourceURLMinLength || len(canonical) > SourceURLMaxLength {
		return "", ErrInvalidSourceURL
	}
	host := canonical[separator+3:]
	if host == "" || strings.HasPrefix(host, "/") {
		return "", ErrInvalidSourceURL
	}
	for _, r := range canonical {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", ErrInvalidSourceURL
		}
	}
	return canonical, nil
}

// parseSourceDescription trims the optional description and enforces its
// length and character rules.
func parseSourceDescription(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if utf8.RuneCountInString(trimmed) > SourceDescriptionMaxLength {
		return "", ErrSourceDescriptionTooLong
	}
	if containsUnsupportedRunes(trimmed) {
		return "", ErrInvalidContent
	}
	return trimmed, nil
}
