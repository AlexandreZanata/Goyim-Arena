package domain

import "strings"

// Bounds of the domain. Every limit is a compile-time constant so a caller
// cannot widen it at runtime; adapters re-check what they receive.
const (
	// maxRecipientLength is the RFC 5321 forward-path limit (254 octets).
	maxRecipientLength = 254
	// maxLocalPartLength is the RFC 5321 local-part limit (64 octets).
	maxLocalPartLength = 64

	// maxSubjectLength is generous for a localized subject line and far
	// below any header limit per line.
	maxSubjectLength = 200

	// maxBodyLength bounds one rendered representation. A provider accepts
	// more, but a message larger than this is a bug upstream, not a
	// legitimate email.
	maxBodyLength = 64 * 1024

	// maxDisplayNameLength bounds the user-controlled display name that the
	// greeting may carry.
	maxDisplayNameLength = 80

	// maxCodeLength bounds a one-time code or token. Codes are short by
	// construction; the cap keeps hostile input cheap to reject.
	maxCodeLength = 512

	// maxIdempotencyKeyLength bounds the delivery key (RFC-shaped, so a
	// UUID or a prefixed identifier fits with room to spare).
	maxIdempotencyKeyLength = 200
)

// TemplateID is a transactional email template of the closed product set.
type TemplateID string

// The templates the product sends. There is no free-form template: an
// unknown identifier is a programming error, and the localized content of
// each one lives in the catalog (locales/<locale>/email.json).
const (
	// TemplateVerification confirms ownership of an email address.
	TemplateVerification TemplateID = "verification"
	// TemplatePasswordReset delivers a password reset code.
	TemplatePasswordReset TemplateID = "password_reset"
)

// TemplateIDs returns the closed set, sorted, for allowlists and tests.
func TemplateIDs() []TemplateID {
	return []TemplateID{TemplatePasswordReset, TemplateVerification}
}

// Valid reports whether the identifier belongs to the closed set.
func (t TemplateID) Valid() bool {
	switch t {
	case TemplateVerification, TemplatePasswordReset:
		return true
	default:
		return false
	}
}

// String returns the identifier as the wire value used by the catalog and
// by durable job parameters.
func (t TemplateID) String() string { return string(t) }

// Locale is a locale of the interface catalog. The set is closed on
// purpose: an email is composed in a locale we actually ship, and the
// worker freezes it when the job is enqueued, so a later preference change
// cannot retarget a message already in the outbox.
type Locale string

// The locales with a catalog present (I18N_STANDARD.md).
const (
	LocaleBrazilianPortuguese Locale = "pt-BR"
	LocaleAmericanEnglish     Locale = "en-US"
)

// LocaleDefault is the product default, per I18N_STANDARD.md.
const LocaleDefault = LocaleBrazilianPortuguese

// Locales returns the closed set, sorted.
func Locales() []Locale { return []Locale{LocaleAmericanEnglish, LocaleBrazilianPortuguese} }

// ParseLocale canonicalizes a locale tag and checks it against the closed
// set. Only the exact BCP 47 shape of a shipped locale is accepted: a
// variant such as "pt-br" or "en_US" is rejected instead of silently
// corrected, so a producer that stores the wrong tag fails loudly.
func ParseLocale(raw string) (Locale, error) {
	switch Locale(strings.TrimSpace(raw)) {
	case LocaleBrazilianPortuguese:
		return LocaleBrazilianPortuguese, nil
	case LocaleAmericanEnglish:
		return LocaleAmericanEnglish, nil
	default:
		return "", ErrUnsupportedLocale
	}
}

// Valid reports whether the locale belongs to the closed set.
func (l Locale) Valid() bool {
	switch l {
	case LocaleBrazilianPortuguese, LocaleAmericanEnglish:
		return true
	default:
		return false
	}
}

// String returns the BCP 47 tag.
func (l Locale) String() string { return string(l) }

// TemplateValues are the runtime values merged into a template. They are
// deliberately two bounded scalars instead of a map: the closing of the
// data the caller may inject into a message is the whole point, and a map
// would let a caller smuggle an arbitrary field past the catalog.
type TemplateValues struct {
	// Name is the user's display name. Empty means "greeting without a
	// name", which is a legitimate state (the account may not have one).
	Name string
	// Code is the one-time code the email delivers. It is never logged.
	Code string
}

// NewTemplateValues validates the values structurally.
func NewTemplateValues(name, code string) (TemplateValues, error) {
	values := TemplateValues{Name: strings.TrimSpace(name), Code: strings.TrimSpace(code)}
	if len([]rune(values.Name)) > maxDisplayNameLength || hasControl(values.Name) {
		return TemplateValues{}, ErrInvalidTemplateValue
	}
	if values.Code == "" || len(values.Code) > maxCodeLength || hasControl(values.Code) {
		return TemplateValues{}, ErrInvalidTemplateValue
	}
	// A code is a single token: whitespace inside it would change how a
	// provider folds a header and how a client renders the value.
	if strings.ContainsAny(values.Code, " \t\r\n") {
		return TemplateValues{}, ErrInvalidTemplateValue
	}
	return values, nil
}

// Body is one rendered representation of a template. Subject and Text are
// plain text; HTML is markup produced by an adapter with contextual
// escaping, never by concatenation.
type Body struct {
	Subject string
	Text    string
	HTML    string
}

// NewBody validates a rendered body. The subject must be header-safe: a
// CR or LF inside it would let content start a new header.
func NewBody(subject, text, html string) (Body, error) {
	if subject == "" || len(subject) > maxSubjectLength || strings.ContainsAny(subject, "\r\n") || hasControl(subject) {
		return Body{}, ErrInvalidBody
	}
	if !validBodyPart(text) || !validBodyPart(html) {
		return Body{}, ErrInvalidBody
	}
	return Body{Subject: subject, Text: text, HTML: html}, nil
}

func validBodyPart(part string) bool {
	return part != "" && len(part) <= maxBodyLength
}

// Message is a fully composed delivery request: the recipient, the frozen
// locale and template, the rendered body and the idempotency key that lets
// a retry reach the provider as the same logical send.
type Message struct {
	recipient      string
	locale         Locale
	template       TemplateID
	body           Body
	idempotencyKey string
}

// NewMessage validates and freezes a delivery request.
func NewMessage(recipient string, locale Locale, templateID TemplateID, body Body, idempotencyKey string) (Message, error) {
	normalized, err := normalizeRecipient(recipient)
	if err != nil {
		return Message{}, err
	}
	if !locale.Valid() {
		return Message{}, ErrUnsupportedLocale
	}
	if !templateID.Valid() {
		return Message{}, ErrUnsupportedTemplate
	}
	if _, err := NewBody(body.Subject, body.Text, body.HTML); err != nil {
		return Message{}, err
	}
	if !validIdempotencyKey(idempotencyKey) {
		return Message{}, ErrInvalidIdempotencyKey
	}
	return Message{
		recipient:      normalized,
		locale:         locale,
		template:       templateID,
		body:           body,
		idempotencyKey: idempotencyKey,
	}, nil
}

// Validate re-checks a message that may have been built by another path,
// such as a deserialized job payload. The zero Message is invalid, and an
// adapter can therefore refuse an unwired delivery instead of sending an
// empty request to a provider.
func (m Message) Validate() error {
	_, err := NewMessage(m.recipient, m.locale, m.template, m.body, m.idempotencyKey)
	return err
}

// Recipient returns the normalized address the adapter sends to.
func (m Message) Recipient() string { return m.recipient }

// Locale returns the locale the message was composed in.
func (m Message) Locale() Locale { return m.locale }

// Template returns the template the message came from.
func (m Message) Template() TemplateID { return m.template }

// Body returns the rendered representations.
func (m Message) Body() Body { return m.body }

// IdempotencyKey returns the delivery key. It is a correlation identifier,
// not a secret, but it is never logged: replaying a job must not be
// observable through the log stream.
func (m Message) IdempotencyKey() string { return m.idempotencyKey }

// normalizeRecipient validates the mailbox and returns the address as it
// will be sent. The address is not rewritten beyond trimming: providers
// treat the local part of an address as case-sensitive in principle, and
// silently lowercasing it would change the recipient.
func normalizeRecipient(address string) (string, error) {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" || len(trimmed) > maxRecipientLength || hasControl(trimmed) || strings.ContainsAny(trimmed, " \t") {
		return "", ErrInvalidRecipient
	}
	at := strings.LastIndex(trimmed, "@")
	if at <= 0 || at == len(trimmed)-1 {
		return "", ErrInvalidRecipient
	}
	local, host := trimmed[:at], trimmed[at+1:]
	if strings.Contains(local, "@") || len(local) > maxLocalPartLength {
		return "", ErrInvalidRecipient
	}
	if !validLocalPart(local) || !validHost(host) {
		return "", ErrInvalidRecipient
	}
	return trimmed, nil
}

// validLocalPart accepts the dot-atom shape with a conservative character
// set: quoted forms and comments are legal SMTP but never reachable from
// our own signup and account flows, so they are refused rather than
// guessed at.
func validLocalPart(local string) bool {
	if local == "" || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return false
	}
	for _, char := range local {
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9':
			continue
		case strings.ContainsRune("._%+-", char):
			continue
		default:
			return false
		}
	}
	return true
}

// validHost accepts a dotted hostname: at least two non-empty labels of
// letters, digits and hyphen, each starting and ending with an
// alphanumeric character.
func validHost(host string) bool {
	if host == "" || len(host) > maxRecipientLength || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return false
		}
		if !isAlphaNumeric(rune(label[0])) || !isAlphaNumeric(rune(label[len(label)-1])) {
			return false
		}
		for _, char := range label {
			if isAlphaNumeric(char) || char == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func isAlphaNumeric(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9')
}

// validIdempotencyKey accepts the printable, URL-safe shape of a key we
// generate. The provider echoes it back for correlation, so the key may not
// carry whitespace or control characters.
func validIdempotencyKey(key string) bool {
	if key == "" || len(key) > maxIdempotencyKeyLength {
		return false
	}
	for _, char := range key {
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9':
			continue
		case strings.ContainsRune("._:-", char):
			continue
		default:
			return false
		}
	}
	return true
}

// hasControl reports whether the value carries a control character
// (including DEL). Such values are refused everywhere: inside a subject
// they would inject headers, and inside a log field they would forge
// records.
func hasControl(value string) bool {
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}
