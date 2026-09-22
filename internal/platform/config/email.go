package config

import (
	"fmt"
	"net/mail"
	"strings"
)

// Transactional email delivery configuration (P19-T02A).
//
// The identity flows always send a message — a registration is a link that has
// to arrive — so the environment that serves real accounts always has a
// delivery path: the Resend adapter composed in `internal/bootstrap`. Two
// things that adapter needs are configuration and nothing else is: the endpoint
// is deliberately not configurable (the credential can then only ever be sent
// to the provider), the deadline has a default, and an idempotency key belongs
// to the job that owns the delivery, never to the deployment.
//
//	ARENA_RESEND_API_KEY=re_...
//	ARENA_EMAIL_FROM=Arena <no-reply@example.invalid>
//
// Both are required when ARENA_ENV=production, for the same reason the DSN and
// the payment credential are: production serves registrations, and a
// registration whose confirmation link cannot be delivered is not a journey.
// The composition refuses to build the account journey without them even when
// the environment claims otherwise, so production cannot serve forms whose
// links go nowhere.
//
// The local email sink (ARENA_EMAIL_SINK_DIR) is the other half of this
// decision: it stays a development and test replacement, and production still
// refuses it, because a directory of account codes is not a delivery mechanism.
const (
	// ResendAPIKeyVariable carries the provider credential of transactional
	// email.
	ResendAPIKeyVariable = "ARENA_RESEND_API_KEY"

	// EmailFromVariable carries the verified sender address the provider
	// accepts, optionally with a display name.
	EmailFromVariable = "ARENA_EMAIL_FROM"

	// maxResendAPIKeyLength bounds the credential defensively, so a misfiled
	// blob is refused instead of being handed to the provider.
	maxResendAPIKeyLength = 512

	// maxEmailFromLength is the RFC 5322 address bound, display name included.
	// The provider adapter applies the same bound; it is repeated here so a
	// misfiled value is refused at boot with the variable named.
	maxEmailFromLength = 320
)

// resendAPIKeyPrefix is the shape the provider issues for its sending API.
// Requiring it catches the classic misconfiguration of pasting a value copied
// from another console: without it, the mistake would only surface on the first
// registration, as a rejected delivery.
const resendAPIKeyPrefix = "re_"

// hasResendAPIKeyPrefix reports whether the credential carries the provider
// prefix and an actual key body after it.
func hasResendAPIKeyPrefix(raw string) bool {
	body, found := strings.CutPrefix(raw, resendAPIKeyPrefix)
	return found && body != ""
}

// parseResendAPIKey validates the credential shape without ever echoing the
// value: a problem message names the variable and the reason, never the secret.
func parseResendAPIKey(raw string) (Secret, ValidationErrors) {
	secret := NewSecret(raw)
	if raw == "" {
		return Secret{}, nil
	}

	problem := ""
	switch {
	case len(raw) > maxResendAPIKeyLength:
		problem = fmt.Sprintf("is longer than %d characters (is this really the provider key?)", maxResendAPIKeyLength)
	case strings.TrimSpace(raw) != raw:
		problem = "contains surrounding whitespace (copy the value without spaces or quotes)"
	case !hasResendAPIKeyPrefix(raw):
		problem = fmt.Sprintf("must be a provider sending key (%s...)", resendAPIKeyPrefix)
	case !isPrintableASCII(raw):
		problem = "contains characters outside printable ASCII"
	}
	if problem != "" {
		return Secret{}, ValidationErrors{{Variable: ResendAPIKeyVariable, Problem: problem}}
	}
	return secret, nil
}

// isPrintableText reports whether every byte is printable ASCII or a space.
// It is deliberately not the credential guard: a sender address may carry a
// display name, and a display name contains spaces — "Arena Support" is a
// legitimate value. What has to be refused is a control character, a newline
// or a non-ASCII byte, not the space inside a name.
func isPrintableText(value string) bool {
	for index := 0; index < len(value); index++ {
		if character := value[index]; character < ' ' || character > '~' {
			return false
		}
	}
	return true
}

// parseEmailFrom validates the sender address with the standard parser, the
// same language the provider will read: a bare address or a display name
// followed by an address in angle brackets. A value that no message could carry
// is refused at boot, naming the variable, instead of failing on the first
// delivery.
func parseEmailFrom(raw string) (string, ValidationErrors) {
	if raw == "" {
		return "", nil
	}

	problem := ""
	switch {
	case len(raw) > maxEmailFromLength:
		problem = fmt.Sprintf("is longer than %d characters", maxEmailFromLength)
	case strings.TrimSpace(raw) != raw:
		problem = "contains surrounding whitespace (copy the value without spaces or quotes)"
	case !isPrintableText(raw):
		problem = "contains control characters or non-ASCII bytes"
	default:
		if _, err := mail.ParseAddress(raw); err != nil {
			problem = "is not a valid address (expected, for example, Arena <no-reply@example.invalid>)"
		}
	}
	if problem != "" {
		return "", ValidationErrors{{Variable: EmailFromVariable, Problem: problem}}
	}
	return raw, nil
}

// ResendAPIKey returns the redacted credential of the transactional email
// provider. The composition root passes it to the adapter through Unredacted
// and nothing else reads it.
func (config Config) ResendAPIKey() Secret { return config.resendAPIKey }

// EmailFrom returns the verified sender address of transactional email. It is
// not a secret — it is the address every recipient sees — so it is returned as
// plain configuration.
func (config Config) EmailFrom() string { return config.emailFrom }
