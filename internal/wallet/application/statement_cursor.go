package application

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"
)

// statementCursorVersion prefixes every cursor payload so a future layout
// can be introduced without silently misreading old cursors.
const statementCursorVersion = "v1"

// minStatementCursorSecretLength is the minimum HMAC key size accepted for
// cursor signing (256 bits).
const minStatementCursorSecretLength = 32

// StatementCursorCodec encodes and verifies opaque, server-signed statement
// cursors: clients may pass them back verbatim, but a forged or corrupted
// cursor is rejected instead of being interpreted.
type StatementCursorCodec struct {
	secret []byte
}

// NewStatementCursorCodec builds the codec from the configured signing
// secret. Secrets shorter than 256 bits are refused.
func NewStatementCursorCodec(secret []byte) (*StatementCursorCodec, error) {
	if len(secret) < minStatementCursorSecretLength {
		return nil, ErrWeakStatementCursorSecret
	}
	copied := make([]byte, len(secret))
	copy(copied, secret)
	return &StatementCursorCodec{secret: copied}, nil
}

// Encode renders the signed cursor of the last delivered entry.
func (c *StatementCursorCodec) Encode(entry StatementEntry) string {
	payload := strings.Join([]string{
		statementCursorVersion,
		entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		entry.TransactionID,
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
func (c *StatementCursorCodec) Decode(raw string) (*StatementPosition, error) {
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
	if len(fields) != 3 || fields[0] != statementCursorVersion {
		return nil, ErrInvalidCursor
	}

	createdAt, err := time.Parse(time.RFC3339Nano, fields[1])
	if err != nil {
		return nil, ErrInvalidCursor
	}

	transactionID := fields[2]
	if transactionID == "" {
		return nil, ErrInvalidCursor
	}
	for i := 0; i < len(transactionID); i++ {
		if transactionID[i] < 0x21 || transactionID[i] > 0x7e {
			return nil, ErrInvalidCursor
		}
	}

	return &StatementPosition{CreatedAt: createdAt.UTC(), TransactionID: transactionID}, nil
}
