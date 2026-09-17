package dbpool

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type stubClock struct {
	current time.Time
}

func (s *stubClock) Now() time.Time {
	return s.current
}

func (s *stubClock) Advance(d time.Duration) {
	s.current = s.current.Add(d)
}

func TestTracerRecordsDurationAndSanitizesOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	baseTime := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	clock := &stubClock{current: baseTime}

	tr := newTracer(logger, clock)
	ctx := context.Background()

	// 1. Normal query with sensitive SQL and arguments
	const sensitiveSQL = "SELECT * FROM users WHERE password = 'my-secret-password'"
	qCtx := tr.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL:  sensitiveSQL,
		Args: []any{"secret-arg", 123},
	})

	clock.Advance(42 * time.Millisecond)

	tr.TraceQueryEnd(qCtx, nil, pgx.TraceQueryEndData{
		CommandTag: pgconn.NewCommandTag("SELECT 1"),
		Err:        nil,
	})

	output := buf.String()
	if strings.Contains(output, "my-secret-password") {
		t.Errorf("output leaked sensitive SQL: %s", output)
	}
	if strings.Contains(output, "secret-arg") {
		t.Errorf("output leaked argument: %s", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in output, got: %s", output)
	}
	if !strings.Contains(output, `"args_count":2`) {
		t.Errorf("expected args_count: 2 in output, got: %s", output)
	}

	buf.Reset()

	// 2. Query error with sensitive error message
	errCtx := tr.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL:  "SELECT 1",
		Args: nil,
	})
	clock.Advance(10 * time.Millisecond)

	sensitiveErr := errors.New("connection failed: postgres://user:super-secret@host:5432/db")
	tr.TraceQueryEnd(errCtx, nil, pgx.TraceQueryEndData{
		Err: sensitiveErr,
	})

	errOutput := buf.String()
	if strings.Contains(errOutput, "super-secret") {
		t.Errorf("output leaked error secret: %s", errOutput)
	}
	if !strings.Contains(errOutput, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in error output, got: %s", errOutput)
	}
}
