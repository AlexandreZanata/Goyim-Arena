package application

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"
)

// historyCursorVersion prefixes every cursor payload so a future layout can
// be introduced without silently misreading old cursors.
const historyCursorVersion = "v1"

// minHistoryCursorSecretLength is the minimum HMAC key size accepted for
// history cursor signing (256 bits).
const minHistoryCursorSecretLength = 32

// HistoryCursorCodec encodes and verifies opaque, server-signed pass history
// cursors: clients may pass them back verbatim, but a forged or corrupted
// cursor is rejected instead of being interpreted.
type HistoryCursorCodec struct {
	secret []byte
}

// NewHistoryCursorCodec builds the codec from the configured signing secret.
// Secrets shorter than 256 bits are refused.
func NewHistoryCursorCodec(secret []byte) (*HistoryCursorCodec, error) {
	if len(secret) < minHistoryCursorSecretLength {
		return nil, ErrWeakHistoryCursorSecret
	}
	copied := make([]byte, len(secret))
	copy(copied, secret)
	return &HistoryCursorCodec{secret: copied}, nil
}

// Encode renders the signed cursor of the last delivered entry.
func (c *HistoryCursorCodec) Encode(entry PassConsumptionRecord) string {
	payload := strings.Join([]string{
		historyCursorVersion,
		entry.ConsumedAt.UTC().Format(time.RFC3339Nano),
		entry.ConsumptionID,
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
func (c *HistoryCursorCodec) Decode(raw string) (*ConsumptionPosition, error) {
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
	if len(fields) != 3 || fields[0] != historyCursorVersion {
		return nil, ErrInvalidCursor
	}

	consumedAt, err := time.Parse(time.RFC3339Nano, fields[1])
	if err != nil {
		return nil, ErrInvalidCursor
	}

	consumptionID := fields[2]
	if consumptionID == "" {
		return nil, ErrInvalidCursor
	}
	for i := 0; i < len(consumptionID); i++ {
		if consumptionID[i] < 0x21 || consumptionID[i] > 0x7e {
			return nil, ErrInvalidCursor
		}
	}

	return &ConsumptionPosition{ConsumedAt: consumedAt.UTC(), ConsumptionID: consumptionID}, nil
}
