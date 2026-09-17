package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/Goyim-Arena/internal/billing/adapters/postgres"
	"github.com/AlexandreZanata/Goyim-Arena/internal/billing/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/billing/domain"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/dbtest"
	platformpg "github.com/AlexandreZanata/Goyim-Arena/internal/platform/postgres"
)

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

func mustBillingAccount(t *testing.T, ctx context.Context, q *platformpg.Queries, email string) platformpg.AppAccount {
	t.Helper()
	acc, err := q.CreateAccount(ctx, platformpg.CreateAccountParams{Email: email, Status: "active"})
	if err != nil {
		t.Fatalf("create account %s: %v", email, err)
	}
	return acc
}

func mustGrantRequest(
	t *testing.T,
	accountID domain.AccountID,
	origin domain.PassOrigin,
	quantity int32,
	reference string,
	expiresAt *time.Time,
) application.GrantPassLotRequest {
	t.Helper()
	parsedQuantity, err := domain.NewQuantity(quantity)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", quantity, err)
	}
	parsedReference, err := domain.ParseReference(reference)
	if err != nil {
		t.Fatalf("ParseReference(%q): %v", reference, err)
	}
	return application.GrantPassLotRequest{
		AccountID: accountID,
		Origin:    origin,
		Quantity:  parsedQuantity,
		Reference: parsedReference,
		ExpiresAt: expiresAt,
		GrantedAt: time.Now().UTC(),
	}
}

func countPassLots(t *testing.T, ctx context.Context, pool *pgxpool.Pool, accountID pgtype.UUID) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM app.arena_pass_lots WHERE account_id = $1", accountID).Scan(&count); err != nil {
		t.Fatalf("count pass lots: %v", err)
	}
	return count
}

func TestRepository_GrantPassLotPurchasesAndMember(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := postgres.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustBillingAccount(t, ctx, q, "grant-purchase@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	purchase, err := repo.GrantPassLot(ctx, mustGrantRequest(t, accountID, domain.OriginPurchase, 5, "stripe:evt_purchase_1", nil))
	if err != nil {
		t.Fatalf("purchase grant error = %v", err)
	}
	if purchase.Replayed {
		t.Fatal("first purchase grant must not be a replay")
	}
	if purchase.Lot.Origin() != domain.OriginPurchase || purchase.Lot.Quantity().Int32() != 5 || purchase.Lot.Remaining() != 5 {
		t.Fatalf("purchase lot = %+v", purchase.Lot)
	}
	if purchase.Lot.ExpiresAt() != nil {
		t.Fatalf("bought passes do not expire, got %v", purchase.Lot.ExpiresAt())
	}

	periodEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.FixedZone("BRT", -3*3600))
	member, err := repo.GrantPassLot(ctx, mustGrantRequest(t, accountID, domain.OriginMember, 1, "member:2026-09", &periodEnd))
	if err != nil {
		t.Fatalf("member grant error = %v", err)
	}
	if member.Lot.ExpiresAt() == nil {
		t.Fatal("member lot must expire at the end of its period")
	}
	if member.Lot.ExpiresAt().Location() != time.UTC || !member.Lot.ExpiresAt().Equal(periodEnd) {
		t.Fatalf("member expiration = %v, want %v in UTC", member.Lot.ExpiresAt(), periodEnd)
	}

	stored, err := q.GetArenaPassLotByGrant(ctx, platformpg.GetArenaPassLotByGrantParams{
		AccountID: acc.ID,
		Origin:    "PURCHASE",
		Reference: "stripe:evt_purchase_1",
	})
	if err != nil {
		t.Fatalf("load stored lot: %v", err)
	}
	if uuidString(stored.ID) != purchase.Lot.ID().String() {
		t.Fatalf("stored lot id = %q, want %q", uuidString(stored.ID), purchase.Lot.ID())
	}
	if countPassLots(t, ctx, pool, acc.ID) != 2 {
		t.Fatalf("lots = %d, want 2", countPassLots(t, ctx, pool, acc.ID))
	}
}

// TestRepository_GrantPassLotIsIdempotentAndImmutableExpiry proves retries
// never duplicate a grant and that a replayed payload can never rewrite the
// original expiration.
func TestRepository_GrantPassLotIsIdempotentAndImmutableExpiry(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := postgres.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustBillingAccount(t, ctx, q, "grant-replay@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	originalEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	first, err := repo.GrantPassLot(ctx, mustGrantRequest(t, accountID, domain.OriginMember, 1, "member:2026-09", &originalEnd))
	if err != nil {
		t.Fatalf("first grant error = %v", err)
	}

	// Retry with a drifted payload: different quantity and expiration under
	// the same key must resolve the original lot untouched.
	driftedEnd := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	retry, err := repo.GrantPassLot(ctx, mustGrantRequest(t, accountID, domain.OriginMember, 3, "member:2026-09", &driftedEnd))
	if err != nil {
		t.Fatalf("retry grant error = %v", err)
	}
	if !retry.Replayed {
		t.Fatal("retry must be a replay")
	}
	if retry.Lot.ID() != first.Lot.ID() {
		t.Fatalf("retry lot = %q, want original %q", retry.Lot.ID(), first.Lot.ID())
	}
	if retry.Lot.Quantity().Int32() != 1 || retry.Lot.Remaining() != 1 {
		t.Fatalf("retry rewrote the quantity: %+v", retry.Lot)
	}
	if retry.Lot.ExpiresAt() == nil || !retry.Lot.ExpiresAt().Equal(originalEnd) {
		t.Fatalf("retry rewrote the expiration: %v, want %v", retry.Lot.ExpiresAt(), originalEnd)
	}

	if countPassLots(t, ctx, pool, acc.ID) != 1 {
		t.Fatalf("lots after retry = %d, want exactly 1", countPassLots(t, ctx, pool, acc.ID))
	}
}

func TestRepository_GrantPassLotConcurrent(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t, dbtest.WithPoolLimits(20, 1))
	pool := testDB.Pool.Pool()
	repo := postgres.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustBillingAccount(t, ctx, q, "grant-concurrent@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))
	request := mustGrantRequest(t, accountID, domain.OriginPurchase, 2, "stripe:evt_concurrent", nil)

	const attempts = 20
	var (
		fresh    atomic.Int32
		replayed atomic.Int32
		wg       sync.WaitGroup
	)
	lotIDs := make([]string, attempts)
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := repo.GrantPassLot(ctx, request)
			if err != nil {
				errs[index] = err
				return
			}
			if result.Replayed {
				replayed.Add(1)
			} else {
				fresh.Add(1)
			}
			lotIDs[index] = result.Lot.ID().String()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("attempt %d error = %v", i, err)
		}
	}
	if fresh.Load() != 1 || replayed.Load() != attempts-1 {
		t.Fatalf("fresh=%d replayed=%d, want 1/%d", fresh.Load(), replayed.Load(), attempts-1)
	}
	for i, lotID := range lotIDs {
		if lotID != lotIDs[0] {
			t.Fatalf("attempt %d resolved lot %q, want %q", i, lotID, lotIDs[0])
		}
	}
	if got := countPassLots(t, ctx, pool, acc.ID); got != 1 {
		t.Fatalf("lots = %d, want exactly 1", got)
	}
}

func TestRepository_GrantPassLotDistinctGrants(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := postgres.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustBillingAccount(t, ctx, q, "grant-distinct@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	grants := []application.GrantPassLotRequest{
		mustGrantRequest(t, accountID, domain.OriginPurchase, 1, "stripe:evt_a", nil),
		mustGrantRequest(t, accountID, domain.OriginPurchase, 1, "stripe:evt_b", nil),
		mustGrantRequest(t, accountID, domain.OriginAdmin, 1, "stripe:evt_a", nil),
	}
	seen := map[string]bool{}
	for i, grant := range grants {
		result, err := repo.GrantPassLot(ctx, grant)
		if err != nil {
			t.Fatalf("grant %d error = %v", i, err)
		}
		if result.Replayed {
			t.Fatalf("grant %d unexpectedly replayed", i)
		}
		if seen[result.Lot.ID().String()] {
			t.Fatalf("grant %d reused a lot id", i)
		}
		seen[result.Lot.ID().String()] = true
	}
	if got := countPassLots(t, ctx, pool, acc.ID); got != len(grants) {
		t.Fatalf("lots = %d, want %d", got, len(grants))
	}
}

func TestRepository_GrantArenaPassesUseCaseEndToEnd(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := postgres.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustBillingAccount(t, ctx, q, "grant-use-case@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	fixedNow := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	useCase := application.NewGrantArenaPassesUseCase(repo, fixedClock{now: fixedNow})

	command := application.GrantArenaPassesCommand{
		AccountID: accountID.String(),
		Origin:    "PURCHASE",
		Quantity:  3,
		Reference: "stripe:evt_use_case",
	}
	first, err := useCase.Execute(ctx, command)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if first.Replayed || first.Lot.Quantity().Int32() != 3 {
		t.Fatalf("first grant = %+v", first)
	}

	retry, err := useCase.Execute(ctx, command)
	if err != nil {
		t.Fatalf("retry Execute() error = %v", err)
	}
	if !retry.Replayed || retry.Lot.ID() != first.Lot.ID() {
		t.Fatalf("retry = %+v, want the original lot", retry)
	}
	if got := countPassLots(t, ctx, pool, acc.ID); got != 1 {
		t.Fatalf("lots = %d, want 1", got)
	}

	// Member grants require the period end.
	_, err = useCase.Execute(ctx, application.GrantArenaPassesCommand{
		AccountID: accountID.String(),
		Origin:    "MEMBER",
		Quantity:  1,
		Reference: "member:2026-09",
	})
	if !errors.Is(err, domain.ErrExpirationRequired) {
		t.Fatalf("member without expiration error = %v, want ErrExpirationRequired", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
