package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type arenaSearchFake struct {
	query string
	limit int
	items []ArenaResult
}

func (f *arenaSearchFake) SearchArenas(_ context.Context, query, _ string, _ *Cursor, limit int) ([]ArenaResult, error) {
	f.query, f.limit = query, limit
	return f.items, nil
}

type argumentSearchFake struct {
	query string
	limit int
}

func (f *argumentSearchFake) SearchArguments(_ context.Context, query, _ string, _ *Cursor, limit int) ([]ArgumentResult, error) {
	f.query, f.limit = query, limit
	return []ArgumentResult{{ID: "arg-1", CreatedAt: time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)}}, nil
}

func TestCursorCodecRoundTripAndScope(t *testing.T) {
	codec, err := NewCursorCodec([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	original := Cursor{Score: 0.875, At: time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC), ID: "00000000-0000-0000-0000-000000000001", Kind: "arena"}
	raw := codec.Encode(original)
	decoded, err := codec.Decode(raw, "arena")
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded.Score != original.Score || !decoded.At.Equal(original.At) || decoded.ID != original.ID {
		t.Fatalf("decoded = %+v, want %+v", decoded, original)
	}
	if _, err := codec.Decode(raw, "argument"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("wrong cursor kind error = %v, want ErrInvalidCursor", err)
	}
	forged := raw[:len(raw)-1] + "A"
	if _, err := codec.Decode(forged, "arena"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("forged cursor error = %v, want ErrInvalidCursor", err)
	}
}

func TestSearchUseCasesValidateAndPaginate(t *testing.T) {
	codec, err := NewCursorCodec([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	arenaFake := &arenaSearchFake{items: []ArenaResult{
		{ID: "first", PublishedAt: time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC), Score: 1},
		{ID: "second", PublishedAt: time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC), Score: .5},
	}}
	arenas, err := NewArenaSearchUseCase(arenaFake, codec)
	if err != nil {
		t.Fatal(err)
	}
	page, err := arenas.Execute(context.Background(), "  fusão OR energia ", "pt-BR", "", 1)
	if err != nil {
		t.Fatalf("arena Execute() error = %v", err)
	}
	if len(page.Items) != 1 || page.NextCursor == "" || arenaFake.query != "fusão OR energia" || arenaFake.limit != 2 {
		t.Fatalf("page = %+v, fake = %+v, want trimmed page, cursor, and lookahead", page, arenaFake)
	}
	if _, err := arenas.Execute(context.Background(), "websearch_to_tsquery('x')", "de-DE", "", 1); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid language error = %v, want ErrInvalidQuery", err)
	}
	if _, err := arenas.Execute(context.Background(), "", "pt-BR", "", 1); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("empty query error = %v, want ErrInvalidQuery", err)
	}

	argumentFake := &argumentSearchFake{}
	arguments, err := NewArgumentSearchUseCase(argumentFake, codec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arguments.Execute(context.Background(), "termo", "en-US", "", 0); err != nil {
		t.Fatalf("argument Execute() error = %v", err)
	}
	if argumentFake.limit != DefaultLimit+1 {
		t.Fatalf("argument limit = %d, want %d", argumentFake.limit, DefaultLimit+1)
	}
}

func TestCursorCodecRequires256BitSecret(t *testing.T) {
	if _, err := NewCursorCodec([]byte("short")); !errors.Is(err, ErrWeakCursorSecret) {
		t.Fatalf("error = %v, want ErrWeakCursorSecret", err)
	}
}
