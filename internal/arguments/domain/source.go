package domain

import (
	"strconv"
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

// parseSourceURL validates and canonicalizes an absolute HTTP(S) address.
// Parsing is structural only: the domain never fetches or dereferences the
// URL. Userinfo is refused so credentials cannot be persisted or rendered,
// and non-ASCII hostnames are refused rather than being silently transformed
// without an approved IDNA policy. The parser deliberately stays textual so
// the domain does not import a transport URL package.
func parseSourceURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrEmptySourceURL
	}
	if len(trimmed) > SourceURLMaxLength {
		return "", ErrInvalidSourceURL
	}
	for _, r := range trimmed {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", ErrInvalidSourceURL
		}
	}

	separator := strings.Index(trimmed, "://")
	if separator <= 0 {
		return "", ErrInvalidSourceURL
	}
	scheme := strings.ToLower(trimmed[:separator])
	if scheme != "http" && scheme != "https" {
		return "", ErrInvalidSourceURL
	}
	rest := trimmed[separator+3:]
	authorityEnd := len(rest)
	if index := strings.IndexAny(rest, "/?#"); index >= 0 {
		authorityEnd = index
	}
	authority := rest[:authorityEnd]
	if !validSourceAuthority(authority) {
		return "", ErrInvalidSourceURL
	}

	canonical := scheme + "://" + rest
	if len(canonical) < SourceURLMinLength || len(canonical) > SourceURLMaxLength {
		return "", ErrInvalidSourceURL
	}
	return canonical, nil
}

// validSourceAuthority checks the host and optional numeric port. It rejects
// userinfo, malformed ports, empty labels and non-ASCII hostnames. This is a
// URL allowlist, not a DNS lookup: hostnames are not resolved by the server.
func validSourceAuthority(authority string) bool {
	if authority == "" || strings.Contains(authority, "@") {
		return false
	}

	host := authority
	if strings.HasPrefix(host, "[") {
		closing := strings.IndexByte(host, ']')
		if closing < 2 {
			return false
		}
		for _, r := range host[1:closing] {
			if r > unicode.MaxASCII || !(r == ':' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
		host = host[closing+1:]
		if host != "" {
			if !strings.HasPrefix(host, ":") || !validSourcePort(host[1:]) {
				return false
			}
		}
		return true
	}

	if colon := strings.LastIndexByte(host, ':'); colon >= 0 {
		if strings.Contains(host[:colon], ":") || !validSourcePort(host[colon+1:]) {
			return false
		}
		host = host[:colon]
	}
	if host == "" || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if r > unicode.MaxASCII || !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
				return false
			}
		}
	}
	return true
}

func validSourcePort(raw string) bool {
	if raw == "" || len(raw) > 5 {
		return false
	}
	port, err := strconv.Atoi(raw)
	return err == nil && port >= 1 && port <= 65535
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
