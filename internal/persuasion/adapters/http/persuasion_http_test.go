package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	persuasionhttp "github.com/AlexandreZanata/Goyim-Arena/internal/persuasion/adapters/http"
	persuasionpg "github.com/AlexandreZanata/Goyim-Arena/internal/persuasion/adapters/postgres"
	"github.com/AlexandreZanata/Goyim-Arena/internal/persuasion/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/persuasion/domain"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/clockseed"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/config"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/dbtest"
	platformpg "github.com/AlexandreZanata/Goyim-Arena/internal/platform/postgres"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/security"
)

const (
	persuasionAuthorToken   = "persuasion-author-session-token"
	persuasionSpeakerToken  = "persuasion-speaker-session-token"
	persuasionStrangerToken = "persuasion-stranger-session-token"
	persuasionHash          = "v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

// persuasionHarness wires the persuasion HTTP adapter over one isolated
// database with three accounts: the author, an eligible attributor and a
// stranger who is not part of the Arena.
type persuasionHarness struct {
	mux           http.Handler
	pool          *pgxpool.Pool
	repo          *persuasionpg.Repository
	author        pgtype.UUID
	speaker       pgtype.UUID
	stranger      pgtype.UUID
	arena         pgtype.UUID
	argument      pgtype.UUID
	publishedAt   time.Time
	authorEmail   string
	speakerEmail  string
	strangerEmail string
}

func persuasionUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

func mustPersuasionAccount(t *testing.T, ctx context.Context, q *platformpg.Queries, pool *pgxpool.Pool, email string) pgtype.UUID {
	t.Helper()
	account, err := q.CreateAccount(ctx, platformpg.CreateAccountParams{Email: email, Status: "active"})
	if err != nil {
		t.Fatalf("create account %s: %v", email, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE app.accounts SET email_verified_at = now() WHERE id = $1`, account.ID); err != nil {
		t.Fatalf("verify account %s: %v", email, err)
	}
	return account.ID
}

func mustPersuasionArena(t *testing.T, ctx context.Context, pool *pgxpool.Pool, creator pgtype.UUID, slug string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO app.arenas (creator_id, statement, category, language, status)
		VALUES ($1, 'Afirmação para a API de persuasão', 'technology', 'pt-BR', 'draft')
		RETURNING id`, creator).Scan(&id); err != nil {
		t.Fatalf("insert arena: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE app.arenas SET status = 'published', slug = $2, published_at = now(), version = version + 1
		WHERE id = $1`, id, slug); err != nil {
		t.Fatalf("publish arena: %v", err)
	}
	return id
}

func mustPersuasionArgument(t *testing.T, ctx context.Context, pool *pgxpool.Pool, arenaID, authorID pgtype.UUID, statement, status string, createdAt time.Time) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO app.arguments (arena_id, author_id, relation, content, content_hash, grapheme_cost, status, created_at, updated_at)
		VALUES ($1, $2, 'support', $3, $4, 30, $5, $6, $6)
		RETURNING id`, arenaID, authorID, statement, persuasionHash, status, createdAt).Scan(&id); err != nil {
		t.Fatalf("insert argument %q: %v", statement, err)
	}
	return id
}

// mustChange records one position change of the attributor in the harness
// Arena through the real chain (version advances, the projection follows).
func mustChange(t *testing.T, ctx context.Context, pool *pgxpool.Pool, arenaID, accountID pgtype.UUID, version int32, changedAt time.Time) pgtype.UUID {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO app.debate_positions (arena_id, account_id, initial_position, current_position, version)
		VALUES ($1, $2, 'agree', 'disagree', $3)
		ON CONFLICT (arena_id, account_id) DO UPDATE
		SET current_position = EXCLUDED.current_position,
		    version = EXCLUDED.version,
		    updated_at = now()`, arenaID, accountID, version); err != nil {
		t.Fatalf("upsert position: %v", err)
	}
	var id pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO app.position_changes (arena_id, account_id, from_position, to_position, version, changed_at)
		VALUES ($1, $2, 'agree', 'disagree', $3, $4)
		RETURNING id`, arenaID, accountID, version, changedAt).Scan(&id); err != nil {
		t.Fatalf("insert position change: %v", err)
	}
	return id
}

// attribute inserts one valid attribution row directly, so public reads can be
// seeded without replaying the whole recording journey.
func attribute(t *testing.T, ctx context.Context, pool *pgxpool.Pool, arenaID, changeAccount pgtype.UUID, version int32, changedAt time.Time, argumentID, attributorID pgtype.UUID, status string) {
	t.Helper()
	changeID := mustChange(t, ctx, pool, arenaID, changeAccount, version, changedAt)
	if status == "valid" {
		if _, err := pool.Exec(ctx, `
			INSERT INTO app.persuasion_attributions (position_change_id, attributor_id, argument_id, status)
			VALUES ($1, $2, $3, 'valid')`, changeID, attributorID, argumentID); err != nil {
			t.Fatalf("insert attribution: %v", err)
		}
		return
	}

	// An invalidated attribution always carries its decision record: the
	// schema refuses a bare invalidation (P11-T04). The moderator account is
	// the harness author, who is an account like any other here.
	if _, err := pool.Exec(ctx, `
		INSERT INTO app.persuasion_attributions (
			position_change_id, attributor_id, argument_id, status,
			invalidated_at, moderation_reason, moderated_by, moderated_at)
		VALUES ($1, $2, $3, 'invalid', now(), 'atribuição fraudulenta', $4, now())`,
		changeID, attributorID, argumentID, changeAccount); err != nil {
		t.Fatalf("insert invalidated attribution: %v", err)
	}
}

func mustProfile(t *testing.T, ctx context.Context, pool *pgxpool.Pool, account pgtype.UUID, username string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO app.profiles (account_id, username, username_normalized)
		VALUES ($1, $2, lower($2))`, account, username); err != nil {
		t.Fatalf("insert profile %q: %v", username, err)
	}
}

func setupPersuasionHarness(t *testing.T) *persuasionHarness {
	t.Helper()
	ctx := context.Background()
	db := dbtest.New(t)
	pool := db.Pool.Pool()
	q := platformpg.New(pool)

	harness := &persuasionHarness{
		pool:          pool,
		authorEmail:   "persuasion-http-author@arena.example.com",
		speakerEmail:  "persuasion-http-speaker@arena.example.com",
		strangerEmail: "persuasion-http-stranger@arena.example.com",
		publishedAt:   time.Now().UTC().Add(-2 * time.Hour),
	}
	harness.author = mustPersuasionAccount(t, ctx, q, pool, harness.authorEmail)
	harness.speaker = mustPersuasionAccount(t, ctx, q, pool, harness.speakerEmail)
	harness.stranger = mustPersuasionAccount(t, ctx, q, pool, harness.strangerEmail)

	harness.arena = mustPersuasionArena(t, ctx, pool, harness.author, "persuasion-http-arena")
	harness.argument = mustPersuasionArgument(t, ctx, pool, harness.arena, harness.author, "Argumento do autor na Arena", "published", harness.publishedAt)
	mustProfile(t, ctx, pool, harness.author, "autora_publica")

	harness.repo = persuasionpg.NewRepository(pool)
	clock := clockseed.NewClock()
	handler := persuasionhttp.NewHandler(persuasionhttp.HandlerConfig{
		RecordUseCase: application.NewRecordAttributionsUseCase(
			harness.repo,
			domain.DefaultEligibilityPolicy(),
			platformpg.NewTxManager(pool),
		),
		ArgumentMetricsUseCase: application.NewGetArgumentMetricsUseCase(harness.repo, clock),
		ProfileReputationCase:  application.NewGetProfileReputationUseCase(harness.repo, harness.repo, clock),
		SecurityManager:        mustSecurityManager(t),
	})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	secMgr := mustSecurityManager(t)
	validator := security.SessionValidatorFunc(func(_ context.Context, rawToken string) (security.AuthIdentity, error) {
		switch rawToken {
		case persuasionAuthorToken:
			return security.AuthIdentity{AccountID: persuasionUUID(harness.author), SessionID: "session-author"}, nil
		case persuasionSpeakerToken:
			return security.AuthIdentity{AccountID: persuasionUUID(harness.speaker), SessionID: "session-speaker"}, nil
		case persuasionStrangerToken:
			return security.AuthIdentity{AccountID: persuasionUUID(harness.stranger), SessionID: "session-stranger"}, nil
		default:
			return security.AuthIdentity{}, errors.New("unknown session")
		}
	})

	harness.mux = secMgr.AuthenticateMiddleware(validator)(mux)
	return harness
}

func mustSecurityManager(t *testing.T) *security.Manager {
	t.Helper()
	secMgr, err := security.New(security.Options{
		Env:            config.EnvTest,
		AllowedOrigins: []string{"http://example.com"},
		RequireOrigin:  false,
		Clock:          clockseed.NewClock(),
		Random:         clockseed.NewRandom(),
	})
	if err != nil {
		t.Fatalf("setup security manager: %v", err)
	}
	return secMgr
}

func persuasionRequest(method, path, token, body string) *http.Request {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.AddCookie(&http.Cookie{Name: "arena_session", Value: token})
	}
	return request
}

func persuasionDecode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatalf("decode JSON body: %v (body: %s)", err, string(body))
	}
	return object
}

func persuasionAssertExactKeys(t *testing.T, object map[string]any, want ...string) {
	t.Helper()
	if len(object) != len(want) {
		t.Fatalf("response keys = %v, want exactly %v", object, want)
	}
	for _, key := range want {
		if _, ok := object[key]; !ok {
			t.Fatalf("response is missing key %q (keys: %v)", key, object)
		}
	}
}

func persuasionAssertPrivateHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	cacheControl := recorder.Header().Get("Cache-Control")
	for _, directive := range []string{"private", "no-store", "no-cache", "must-revalidate"} {
		if !strings.Contains(cacheControl, directive) {
			t.Fatalf("Cache-Control = %q, want %q (THR-CACHE-01)", cacheControl, directive)
		}
	}
	if pragma := recorder.Header().Get("Pragma"); pragma != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", pragma)
	}
}

func (h *persuasionHarness) record(t *testing.T, token, changeID, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	h.mux.ServeHTTP(recorder, persuasionRequest(http.MethodPost, "/api/v1/me/position-changes/"+changeID+"/attributions", token, body))
	return recorder
}

func (h *persuasionHarness) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	h.mux.ServeHTTP(recorder, persuasionRequest(http.MethodGet, path, "", ""))
	return recorder
}

func argumentBody(argumentIDs ...pgtype.UUID) string {
	ids := make([]string, 0, len(argumentIDs))
	for _, argumentID := range argumentIDs {
		ids = append(ids, `"`+persuasionUUID(argumentID)+`"`)
	}
	return `{"argument_ids":[` + strings.Join(ids, ",") + `]}`
}

func attributionCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, changeID pgtype.UUID) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM app.persuasion_attributions WHERE position_change_id = $1`, changeID).Scan(&count); err != nil {
		t.Fatalf("count attributions: %v", err)
	}
	return count
}

func TestPersuasionAPIRequiresAuthentication(t *testing.T) {
	harness := setupPersuasionHarness(t)

	probes := []struct {
		name   string
		token  string
		method string
		path   string
		body   string
	}{
		{name: "anonymous", method: http.MethodPost, path: "/api/v1/me/position-changes/018f6b2a-0000-7000-8000-000000000001/attributions", body: `{"argument_ids":[]}`},
		{name: "unknown session", token: "not-a-session", method: http.MethodPost, path: "/api/v1/me/position-changes/018f6b2a-0000-7000-8000-000000000001/attributions", body: `{"argument_ids":[]}`},
	}
	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			recorder := harness.record(t, probe.token, "018f6b2a-0000-7000-8000-000000000001", `{"argument_ids":[]}`)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (body: %s)", recorder.Code, recorder.Body.String())
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/problem+json" {
				t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
			}
			// A cookie the platform middleware rejects is answered before the
			// route wrapper runs, exactly as in the other private adapters; an
			// anonymous caller reaches the route and gets the private cache
			// policy from it.
			if probe.token == "" {
				persuasionAssertPrivateHeaders(t, recorder)
			}
		})
	}
}

func TestRecordAttributionsJourney(t *testing.T) {
	ctx := context.Background()
	harness := setupPersuasionHarness(t)
	second := mustPersuasionArgument(t, ctx, harness.pool, harness.arena, harness.author, "Segundo argumento do autor", "published", harness.publishedAt)
	changeID := mustChange(t, ctx, harness.pool, harness.arena, harness.speaker, 2, time.Now().UTC())

	// The selection is recorded and echoed back.
	recorder := harness.record(t, persuasionSpeakerToken, persuasionUUID(changeID), argumentBody(harness.argument, second))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", recorder.Code, recorder.Body.String())
	}
	persuasionAssertPrivateHeaders(t, recorder)
	body := persuasionDecode(t, recorder.Body.Bytes())
	persuasionAssertExactKeys(t, body, "argument_ids", "replayed")
	if replayed, _ := body["replayed"].(bool); replayed {
		t.Fatal("the first recording must not be reported as a replay")
	}
	if recorder.Header().Get("Idempotency-Replayed") != "" {
		t.Fatal("the first recording must not carry the replay header")
	}
	if got := attributionCount(t, ctx, harness.pool, changeID); got != 2 {
		t.Fatalf("stored attributions = %d, want 2", got)
	}

	// Retrying the same selection resolves it instead of writing again.
	retry := harness.record(t, persuasionSpeakerToken, persuasionUUID(changeID), argumentBody(harness.argument, second))
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200 (body: %s)", retry.Code, retry.Body.String())
	}
	if retry.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("Idempotency-Replayed = %q, want true", retry.Header().Get("Idempotency-Replayed"))
	}
	if replayed, _ := persuasionDecode(t, retry.Body.Bytes())["replayed"].(bool); !replayed {
		t.Fatal("the retry must be reported as a replay")
	}
	if got := attributionCount(t, ctx, harness.pool, changeID); got != 2 {
		t.Fatalf("stored attributions after retry = %d, want the same 2", got)
	}

	// An empty selection is a valid skip and never creates a fake entry; the
	// response still carries an array, never null.
	emptyChange := mustChange(t, ctx, harness.pool, harness.arena, harness.speaker, 4, time.Now().UTC())
	skip := harness.record(t, persuasionSpeakerToken, persuasionUUID(emptyChange), `{"argument_ids":[]}`)
	if skip.Code != http.StatusCreated {
		t.Fatalf("skip status = %d, want 201 (body: %s)", skip.Code, skip.Body.String())
	}
	if !strings.Contains(skip.Body.String(), `"argument_ids":[]`) {
		t.Fatalf("skip body = %s, want an empty array", skip.Body.String())
	}
	if got := attributionCount(t, ctx, harness.pool, emptyChange); got != 0 {
		t.Fatalf("stored attributions after skip = %d, want none", got)
	}
	if got := attributionCount(t, ctx, harness.pool, changeID); got != 2 {
		t.Fatalf("stored attributions after skip = %d, want the earlier 2", got)
	}

	// A fresh change credits a third argument: the response carries the whole
	// recorded set of that change.
	third := mustPersuasionArgument(t, ctx, harness.pool, harness.arena, harness.author, "Terceiro argumento do autor", "published", harness.publishedAt)
	nextChange := mustChange(t, ctx, harness.pool, harness.arena, harness.speaker, 5, time.Now().UTC())
	grown := harness.record(t, persuasionSpeakerToken, persuasionUUID(nextChange), argumentBody(third))
	if grown.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", grown.Code, grown.Body.String())
	}
	ids, _ := persuasionDecode(t, grown.Body.Bytes())["argument_ids"].([]any)
	if len(ids) != 1 || ids[0] != persuasionUUID(third) {
		t.Fatalf("argument_ids = %v, want exactly the credited argument", ids)
	}
}

func TestRecordAttributionsRejections(t *testing.T) {
	ctx := context.Background()
	harness := setupPersuasionHarness(t)
	otherArena := mustPersuasionArena(t, ctx, harness.pool, harness.author, "persuasion-http-other-arena")
	foreign := mustPersuasionArgument(t, ctx, harness.pool, otherArena, harness.author, "Argumento de outra Arena", "published", harness.publishedAt)
	late := mustPersuasionArgument(t, ctx, harness.pool, harness.arena, harness.author, "Argumento publicado depois da mudança", "published", time.Now().UTC().Add(2*time.Hour))
	own := mustPersuasionArgument(t, ctx, harness.pool, harness.arena, harness.speaker, "Argumento do próprio atribuidor", "published", harness.publishedAt)
	changeID := mustChange(t, ctx, harness.pool, harness.arena, harness.speaker, 2, time.Now().UTC())

	tests := []struct {
		name     string
		token    string
		changeID string
		body     string
		status   int
		code     string
	}{
		{
			name: "more than three arguments", token: persuasionSpeakerToken, changeID: persuasionUUID(changeID),
			body:   argumentBody(harness.argument, foreign, late, own),
			status: http.StatusBadRequest, code: "persuasion_too_many_attributions",
		},
		{
			name: "own argument", token: persuasionSpeakerToken, changeID: persuasionUUID(changeID),
			body:   argumentBody(own),
			status: http.StatusBadRequest, code: "persuasion_self_attribution",
		},
		{
			name: "argument from another arena", token: persuasionSpeakerToken, changeID: persuasionUUID(changeID),
			body:   argumentBody(foreign),
			status: http.StatusBadRequest, code: "persuasion_cross_arena_argument",
		},
		{
			name: "argument published after the change", token: persuasionSpeakerToken, changeID: persuasionUUID(changeID),
			body:   argumentBody(late),
			status: http.StatusBadRequest, code: "persuasion_argument_not_before_change",
		},
		{
			name: "foreign change", token: persuasionStrangerToken, changeID: persuasionUUID(changeID),
			body:   argumentBody(harness.argument),
			status: http.StatusNotFound, code: "change_not_found",
		},
		{
			name: "unknown change", token: persuasionSpeakerToken, changeID: "018f6b2a-0000-7000-8000-0000000000ff",
			body:   argumentBody(harness.argument),
			status: http.StatusNotFound, code: "change_not_found",
		},
		{
			name: "unknown argument", token: persuasionSpeakerToken, changeID: persuasionUUID(changeID),
			body:   `{"argument_ids":["018f6b2a-0000-7000-8000-0000000000ee"]}`,
			status: http.StatusNotFound, code: "argument_not_found",
		},
		{
			name: "invalid JSON", token: persuasionSpeakerToken, changeID: persuasionUUID(changeID),
			body:   `{"argument_ids":`,
			status: http.StatusBadRequest, code: "invalid_json",
		},
		{
			name: "repeated argument inside the selection", token: persuasionSpeakerToken, changeID: persuasionUUID(changeID),
			body:   argumentBody(harness.argument, harness.argument),
			status: http.StatusBadRequest, code: "persuasion_duplicate_attribution",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := harness.record(t, test.token, test.changeID, test.body)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, test.status, recorder.Body.String())
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/problem+json" {
				t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
			}
			persuasionAssertPrivateHeaders(t, recorder)
			problem := persuasionDecode(t, recorder.Body.Bytes())
			for _, key := range []string{"type", "title", "status", "code"} {
				if _, ok := problem[key]; !ok {
					t.Fatalf("problem document is missing %q: %v", key, problem)
				}
			}
			if code, _ := problem["code"].(string); code != test.code {
				t.Fatalf("code = %q, want %q (body: %s)", code, test.code, recorder.Body.String())
			}
		})
	}

	// Nothing was written by any rejected attempt.
	if got := attributionCount(t, ctx, harness.pool, changeID); got != 0 {
		t.Fatalf("stored attributions = %d, want none after rejections", got)
	}
}

// forbiddenMarkers are the tokens and values a public persuasion document must
// never carry: the attributor identity, internal identifiers and emails
// (BR §5.1, REQ-PERS-05).
func forbiddenMarkers(t *testing.T, harness *persuasionHarness) []string {
	t.Helper()
	return []string{
		"account_id", "attributor", "email", "session", "password",
		harness.authorEmail, harness.speakerEmail, harness.strangerEmail,
		persuasionUUID(harness.speaker), persuasionUUID(harness.stranger),
	}
}

func assertNoForbiddenMarkers(t *testing.T, harness *persuasionHarness, body string) {
	t.Helper()
	lowercased := strings.ToLower(body)
	for _, marker := range forbiddenMarkers(t, harness) {
		if strings.Contains(lowercased, strings.ToLower(marker)) {
			t.Fatalf("SECURITY VIOLATION: public response leaks %q: %s", marker, body)
		}
	}
}

func TestPublicArgumentAttributionCount(t *testing.T) {
	ctx := context.Background()
	harness := setupPersuasionHarness(t)
	second := mustPersuasionArgument(t, ctx, harness.pool, harness.arena, harness.author, "Segundo argumento público", "published", harness.publishedAt)

	// One eligible person credits the argument twice; the stranger credits it
	// once. Both are eligible, so the count is two people and three events.
	attribute(t, ctx, harness.pool, harness.arena, harness.speaker, 2, time.Now().UTC(), harness.argument, harness.speaker, "valid")
	attribute(t, ctx, harness.pool, harness.arena, harness.speaker, 3, time.Now().UTC(), harness.argument, harness.speaker, "valid")
	attribute(t, ctx, harness.pool, harness.arena, harness.stranger, 2, time.Now().UTC(), harness.argument, harness.stranger, "valid")
	// An invalidated attribution never integrates the valid totals.
	attribute(t, ctx, harness.pool, harness.arena, harness.author, 2, time.Now().UTC(), harness.argument, harness.author, "invalid")

	// The public read needs no session at all.
	recorder := harness.get(t, "/api/v1/arguments/"+persuasionUUID(harness.argument)+"/attributions")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", recorder.Code, recorder.Body.String())
	}
	persuasionAssertExactKeys(t, persuasionDecode(t, recorder.Body.Bytes()), "valid_attributions", "distinct_people", "checked_at")

	body := recorder.Body.String()
	if !strings.Contains(body, `"distinct_people":2`) || !strings.Contains(body, `"valid_attributions":3`) {
		t.Fatalf("counts = %s, want two people and three events", body)
	}
	assertNoForbiddenMarkers(t, harness, body)

	// Public counts are short-cache reads with a strong validator.
	cacheControl := recorder.Header().Get("Cache-Control")
	if cacheControl != "public, max-age=60" {
		t.Fatalf("Cache-Control = %q, want public, max-age=60", cacheControl)
	}
	etag := recorder.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag is missing on a public count read")
	}

	revalidation := httptest.NewRequest(http.MethodGet, "/api/v1/arguments/"+persuasionUUID(harness.argument)+"/attributions", nil)
	revalidation.Header.Set("If-None-Match", etag)
	notModified := httptest.NewRecorder()
	harness.mux.ServeHTTP(notModified, revalidation)
	if notModified.Code != http.StatusNotModified {
		t.Fatalf("revalidation status = %d, want 304", notModified.Code)
	}
	if notModified.Body.Len() != 0 {
		t.Fatalf("304 body = %q, want empty", notModified.Body.String())
	}

	// An argument without eligible attributions answers zeros, not an error.
	empty := harness.get(t, "/api/v1/arguments/"+persuasionUUID(second)+"/attributions")
	if empty.Code != http.StatusOK {
		t.Fatalf("empty status = %d, want 200", empty.Code)
	}
	emptyBody := empty.Body.String()
	if !strings.Contains(emptyBody, `"distinct_people":0`) || !strings.Contains(emptyBody, `"valid_attributions":0`) {
		t.Fatalf("empty counts = %s, want zeros", emptyBody)
	}

	// An unknown argument is a plain problem document.
	missing := harness.get(t, "/api/v1/arguments/018f6b2a-0000-7000-8000-0000000000aa/attributions")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", missing.Code)
	}
	if code, _ := persuasionDecode(t, missing.Body.Bytes())["code"].(string); code != "argument_not_found" {
		t.Fatalf("missing code = %q, want argument_not_found", code)
	}
}

func TestPublicProfileReputation(t *testing.T) {
	ctx := context.Background()
	harness := setupPersuasionHarness(t)
	otherArena := mustPersuasionArena(t, ctx, harness.pool, harness.author, "persuasion-http-reputation-arena")
	otherArgument := mustPersuasionArgument(t, ctx, harness.pool, otherArena, harness.author, "Argumento em outra Arena do autor", "published", harness.publishedAt)

	attribute(t, ctx, harness.pool, harness.arena, harness.speaker, 2, time.Now().UTC(), harness.argument, harness.speaker, "valid")
	attribute(t, ctx, harness.pool, harness.arena, harness.speaker, 3, time.Now().UTC(), harness.argument, harness.speaker, "valid")
	attribute(t, ctx, harness.pool, harness.arena, harness.stranger, 2, time.Now().UTC(), harness.argument, harness.stranger, "valid")
	attribute(t, ctx, harness.pool, otherArena, harness.speaker, 2, time.Now().UTC(), otherArgument, harness.speaker, "valid")
	// A fraudulent attribution of the same person elsewhere must not count.
	attribute(t, ctx, harness.pool, otherArena, harness.stranger, 2, time.Now().UTC(), otherArgument, harness.stranger, "invalid")

	// The username is matched case-insensitively, as every profile lookup.
	recorder := harness.get(t, "/api/v1/profiles/AUTORA_PUBLICA/reputation")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	reputation := persuasionDecode(t, recorder.Body.Bytes())
	persuasionAssertExactKeys(t, reputation, "username", "influenced_people", "valid_attributions", "arenas", "by_category", "by_language", "checked_at")

	if username, _ := reputation["username"].(string); username != "autora_publica" {
		t.Fatalf("username = %q, want the canonical handle", username)
	}
	// The same person influenced two Arenas and the stranger only one: the
	// headline is the sum of the per-Arena distinct counts (BR §6), which is
	// larger than the global distinct count of people.
	if got, _ := reputation["influenced_people"].(float64); got != 3 {
		t.Fatalf("influenced_people = %v, want 3", reputation["influenced_people"])
	}
	if got, _ := reputation["valid_attributions"].(float64); got != 4 {
		t.Fatalf("valid_attributions = %v, want 4 valid events", reputation["valid_attributions"])
	}

	// The document is self-consistent: the distributions fold the same facts.
	arenas, _ := reputation["arenas"].([]any)
	var people, events float64
	for _, raw := range arenas {
		arena, _ := raw.(map[string]any)
		people += arena["distinct_people"].(float64)
		events += arena["valid_attributions"].(float64)
	}
	if people != 3 || events != 4 {
		t.Fatalf("arena slices fold to %v/%v, want the headline 3/4", people, events)
	}
	byCategory, _ := reputation["by_category"].([]any)
	if len(byCategory) == 0 {
		t.Fatalf("by_category is empty: %s", body)
	}
	var categoryPeople float64
	for _, raw := range byCategory {
		bucket, _ := raw.(map[string]any)
		categoryPeople += bucket["distinct_people"].(float64)
	}
	if categoryPeople != 3 {
		t.Fatalf("by_category folds to %v people, want 3", categoryPeople)
	}
	assertNoForbiddenMarkers(t, harness, body)

	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "public, max-age=60" {
		t.Fatalf("Cache-Control = %q, want public, max-age=60", cacheControl)
	}
	etag := recorder.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag is missing on the public reputation read")
	}
	revalidation := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/autora_publica/reputation", nil)
	revalidation.Header.Set("If-None-Match", etag)
	notModified := httptest.NewRecorder()
	harness.mux.ServeHTTP(notModified, revalidation)
	if notModified.Code != http.StatusNotModified {
		t.Fatalf("revalidation status = %d, want 304", notModified.Code)
	}

	// A profile whose author received nothing answers zeroed facts with
	// explicit empty arrays, never nulls.
	mustProfile(t, ctx, harness.pool, harness.stranger, "sem_fatos")
	withoutFacts := harness.get(t, "/api/v1/profiles/sem_fatos/reputation")
	if withoutFacts.Code != http.StatusOK {
		t.Fatalf("without-facts status = %d, want 200 (body: %s)", withoutFacts.Code, withoutFacts.Body.String())
	}
	emptyBody := withoutFacts.Body.String()
	for _, marker := range []string{`"influenced_people":0`, `"valid_attributions":0`, `"arenas":[]`, `"by_category":[]`, `"by_language":[]`} {
		if !strings.Contains(emptyBody, marker) {
			t.Fatalf("without-facts body = %s, want %s", emptyBody, marker)
		}
	}

	// A username without a profile is absent, never empty numbers.
	absent := harness.get(t, "/api/v1/profiles/nao_existe/reputation")
	if absent.Code != http.StatusNotFound {
		t.Fatalf("absent status = %d, want 404 (body: %s)", absent.Code, absent.Body.String())
	}
	if code, _ := persuasionDecode(t, absent.Body.Bytes())["code"].(string); code != "profile_not_found" {
		t.Fatalf("absent code = %q, want profile_not_found", code)
	}
	if contentType := absent.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
	}
}
