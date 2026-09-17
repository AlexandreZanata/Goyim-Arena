package application

import (
	"encoding/base64"
	"strings"
	"time"
)

// statementCursorVersion prefixes every cursor payload so a future layout
// can be introduced without silently misreading old cursors.
const statementCursorVersion = "v1"

// encodeStatementCursor renders the opaque keyset cursor of the last
// delivered entry. The cursor is an encoding, not a capability: every query
// still filters by the authenticated account.
func encodeStatementCursor(entry StatementEntry) string {
	parts := []string{
		statementCursorVersion,
		entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		entry.TransactionID,
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, "|")))
}

// parseStatementCursor decodes a cursor into its keyset position. An empty
// cursor yields a nil position (first page); malformed, foreign or
// version-mismatched cursors fail with ErrInvalidCursor instead of being
// reflected back.
func parseStatementCursor(raw string) (*StatementPosition, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, ErrInvalidCursor
	}

	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 || parts[0] != statementCursorVersion {
		return nil, ErrInvalidCursor
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return nil, ErrInvalidCursor
	}

	transactionID := parts[2]
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
