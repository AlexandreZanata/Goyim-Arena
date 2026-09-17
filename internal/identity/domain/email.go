package domain

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Email is an immutable value object representing a normalized, validated email address.
type Email struct {
	address string
}

// ParseEmail validates, normalizes, and constructs an Email value object.
// The raw input is trimmed, checked for invalid Unicode/control sequences,
// validated against RFC 5321/5322 boundaries, and normalized to lowercase.
func ParseEmail(raw string) (Email, error) {
	if !utf8.ValidString(raw) {
		return Email{}, ErrInvalidEmail
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Email{}, ErrEmptyEmail
	}

	// Maximum total length per RFC 5321 (and RFC 3696 errata 1690).
	if len(trimmed) > 254 {
		return Email{}, ErrEmailTooLong
	}

	// Reject any internal whitespace, control characters, or bidirectional overrides.
	for _, r := range trimmed {
		if unicode.IsSpace(r) || unicode.IsControl(r) || isBidiOverride(r) {
			return Email{}, ErrInvalidEmail
		}
	}

	parts := strings.Split(trimmed, "@")
	if len(parts) != 2 {
		return Email{}, ErrInvalidEmail
	}

	local := parts[0]
	domain := parts[1]

	if err := validateLocalPart(local); err != nil {
		return Email{}, err
	}

	if err := validateDomainPart(domain); err != nil {
		return Email{}, err
	}

	normalized := strings.ToLower(local) + "@" + strings.ToLower(domain)
	return Email{address: normalized}, nil
}

func isBidiOverride(r rune) bool {
	switch r {
	case '\u200E', '\u200F', '\u061C', // Directional marks
		'\u202A', '\u202B', '\u202C', '\u202D', '\u202E', // Embeddings and overrides
		'\u2066', '\u2067', '\u2068', '\u2069': // Isolates
		return true
	default:
		return false
	}
}

func validateLocalPart(local string) error {
	if len(local) == 0 {
		return ErrInvalidEmail
	}
	// RFC 5321 limit: max 64 octets for local part.
	if len(local) > 64 {
		return ErrInvalidEmail
	}
	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") {
		return ErrInvalidEmail
	}
	if strings.Contains(local, "..") {
		return ErrInvalidEmail
	}

	for i := 0; i < len(local); i++ {
		b := local[i]
		if !isValidLocalChar(b) {
			return ErrInvalidEmail
		}
	}
	return nil
}

func isValidLocalChar(b byte) bool {
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
		return true
	}
	switch b {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '/', '=', '?', '^', '_', '`', '{', '|', '}', '~', '.':
		return true
	default:
		return false
	}
}

func validateDomainPart(domain string) error {
	if len(domain) == 0 || len(domain) > 255 {
		return ErrInvalidEmail
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return ErrInvalidEmail
	}
	if strings.Contains(domain, "..") {
		return ErrInvalidEmail
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return ErrInvalidEmail
	}

	for i, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return ErrInvalidEmail
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return ErrInvalidEmail
		}
		allNumeric := true
		for j := 0; j < len(label); j++ {
			b := label[j]
			if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
				allNumeric = false
			} else if b >= '0' && b <= '9' || b == '-' {
				// allowed
			} else {
				return ErrInvalidEmail
			}
		}
		// TLD (last label) must be at least 2 characters and cannot be all numeric
		if i == len(labels)-1 {
			if len(label) < 2 || allNumeric {
				return ErrInvalidEmail
			}
		}
	}
	return nil
}

// String returns the normalized email address string.
func (e Email) String() string {
	return e.address
}

// Local returns the local part of the normalized email address.
func (e Email) Local() string {
	if e.address == "" {
		return ""
	}
	parts := strings.Split(e.address, "@")
	return parts[0]
}

// Domain returns the domain part of the normalized email address.
func (e Email) Domain() string {
	if e.address == "" {
		return ""
	}
	parts := strings.Split(e.address, "@")
	return parts[1]
}

// Equals reports whether two Email value objects are identical.
func (e Email) Equals(other Email) bool {
	return e.address == other.address
}

// IsZero reports whether the Email is the uninitialized zero value.
func (e Email) IsZero() bool {
	return e.address == ""
}

// Masked returns an obfuscated representation suitable for safe logs and UI display.
// Example: "johndoe@example.com" -> "j***e@example.com".
func (e Email) Masked() string {
	if e.IsZero() {
		return ""
	}
	local := e.Local()
	domain := e.Domain()

	var maskedLocal string
	switch len(local) {
	case 1:
		maskedLocal = local + "***"
	case 2:
		maskedLocal = string(local[0]) + "***" + string(local[1])
	default:
		maskedLocal = string(local[0]) + "***" + string(local[len(local)-1])
	}
	return fmt.Sprintf("%s@%s", maskedLocal, domain)
}
