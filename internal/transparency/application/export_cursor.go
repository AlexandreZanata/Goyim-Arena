package application

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"
)

// exportCursorVersion prefixes every cursor payload so a future layout can
// be introduced without silently misreading old cursors.
const exportCursorVersion = "v1"

// minExportCursorSecretLength is the minimum HMAC key size accepted for
// export cursor signing (256 bits).
const minExportCursorSecretLength = 32

// ExportPosition is the decoded keyset position of the export page: the
// last argument already delivered to the caller.
type ExportPosition struct {
	CreatedAt  time.Time
	ArgumentID string
}

// ExportCursorCodec encodes and verifies opaque, server-signed keyset
// cursors: clients may pass them back verbatim, but a forged or corrupted
// cursor is rejected instead of being interpreted. The payload carries no
// account identifier, only the public ordering position.
type ExportCursorCodec struct {
	secret []byte
}

// NewExportCursorCodec builds the codec from the configured signing secret.
// Secrets shorter than 256 bits are refused.
func NewExportCursorCodec(secret []byte) (*ExportCursorCodec, error) {
	if len(secret) < minExportCursorSecretLength {
		return nil, ErrWeakExportCursorSecret
	}
	copied := make([]byte, len(secret))
	copy(copied, secret)
	return &ExportCursorCodec{secret: copied}, nil
}

// Encode renders the signed cursor of the last delivered argument.
func (c *ExportCursorCodec) Encode(argument ExportArgument) string {
	payload := strings.Join([]string{
		exportCursorVersion,
		argument.CreatedAt.UTC().Format(time.RFC3339Nano),
		argument.ID,
	}, "|")

	signature := hmac.New(sha256.New, c.secret)
	signature.Write([]byte(payload))

	return base64.RawURLEncoding.EncodeToString([]byte(payload)) +
		"." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil))
}

// Decode verifies the signature and decodes the keyset position. An empty
// cursor yields a nil position (first page); malformed, forged or
// version-mismatched cursors fail with ErrInvalidCursor instead of being
// reflected back.
func (c *ExportCursorCodec) Decode(raw string) (*ExportPosition, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidCursor
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidCursor
	}
	signatureBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidCursor
	}

	expected := hmac.New(sha256.New, c.secret)
	expected.Write(payloadBytes)
	if !hmac.Equal(signatureBytes, expected.Sum(nil)) {
		return nil, ErrInvalidCursor
	}

	fields := strings.Split(string(payloadBytes), "|")
	if len(fields) != 3 || fields[0] != exportCursorVersion {
		return nil, ErrInvalidCursor
	}

	createdAt, err := time.Parse(time.RFC3339Nano, fields[1])
	if err != nil {
		return nil, ErrInvalidCursor
	}

	argumentID := fields[2]
	if argumentID == "" {
		return nil, ErrInvalidCursor
	}
	for i := 0; i < len(argumentID); i++ {
		if argumentID[i] < 0x21 || argumentID[i] > 0x7e {
			return nil, ErrInvalidCursor
		}
	}

	return &ExportPosition{CreatedAt: createdAt.UTC(), ArgumentID: argumentID}, nil
}
