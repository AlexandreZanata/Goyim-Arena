package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/dbtest"
	platformpg "github.com/AlexandreZanata/Goyim-Arena/internal/platform/postgres"
	walletpg "github.com/AlexandreZanata/Goyim-Arena/internal/wallet/adapters/postgres"
	"github.com/AlexandreZanata/Goyim-Arena/internal/wallet/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/wallet/domain"
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

func mustWalletAccount(t *testing.T, ctx context.Context, q *platformpg.Queries, email string) platformpg.AppAccount {
	t.Helper()
	acc, err := q.CreateAccount(ctx, platformpg.CreateAccountParams{Email: email, Status: "active"})
	if err != nil {
		t.Fatalf("create account %s: %v", email, err)
	}
	return acc
}

func mustCreditRequest(
	t *testing.T,
	accountID domain.AccountID,
	bucket domain.Bucket,
	operationType domain.OperationType,
	amount int64,
	reference, idempotencyKey string,
) application.CreditRequest {
	t.Helper()
	ink, err := domain.NewInk(amount)
	if err != nil {
		t.Fatalf("NewInk(%d): %v", amount, err)
	}
	ref, err := domain.ParseReference(reference)
	if err != nil {
		t.Fatalf("ParseReference(%q): %v", reference, err)
	}
	key, err := domain.ParseIdempotencyKey(idempotencyKey)
	if err != nil {
		t.Fatalf("ParseIdempotencyKey(%q): %v", idempotencyKey, err)
	}
	direction, err := operationType.Direction()
	if err != nil {
		t.Fatalf("Direction(%q): %v", operationType, err)
	}
	delta, err := direction.Apply(ink)
	if err != nil {
		t.Fatalf("Apply(%d): %v", amount, err)
	}
	return application.CreditRequest{
		AccountID:      accountID,
		Bucket:         bucket,
		OperationType:  operationType,
		IdempotencyKey: key,
		Reference:      ref,
		Delta:          delta,
		ChangedAt:      time.Now().UTC(),
	}
}

func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestRepository_ApplyCreditPersistsAtomically(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "credit-atomic@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	result, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketFree, domain.OperationCreditFree, 5000, "free:2026-09", "free:2026-09:atomic",
	))
	if err != nil {
		t.Fatalf("ApplyCredit() error = %v", err)
	}
	if result.Replayed {
		t.Fatal("first credit must not be a replay")
	}
	if result.Operation.ID().IsZero() {
		t.Fatal("operation id is empty")
	}
	if result.Operation.Type() != domain.OperationCreditFree || !result.Operation.IsCredit() {
		t.Errorf("operation type = %q", result.Operation.Type())
	}
	if result.Operation.AccountID() != accountID {
		t.Errorf("operation account = %q", result.Operation.AccountID())
	}
	if result.Operation.IdempotencyKey().String() != "free:2026-09:atomic" {
		t.Errorf("operation key = %q", result.Operation.IdempotencyKey())
	}
	if result.Operation.Reference().String() != "free:2026-09" {
		t.Errorf("operation reference = %q", result.Operation.Reference())
	}
	if result.Operation.CreatedAt().IsZero() {
		t.Error("operation createdAt is zero")
	}

	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 5000 || wallet.BalancePurchased != 0 {
		t.Fatalf("balances = %d/%d, want 5000/0", wallet.BalanceFree, wallet.BalancePurchased)
	}

	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID); got != 1 {
		t.Fatalf("operations = %d, want 1", got)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_transactions"); got != 1 {
		t.Fatalf("transactions = %d, want 1", got)
	}

	entries, err := q.ListWalletTransactionsByAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(entries) != 1 || entries[0].Amount != 5000 || entries[0].Bucket != "FREE_INK" {
		t.Fatalf("entries = %+v, want one +5000 FREE_INK transaction", entries)
	}
}

func TestRepository_ApplyCreditIsIdempotentSequential(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "credit-sequential@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))
	request := mustCreditRequest(t, accountID, domain.BucketFree, domain.OperationCreditFree, 5000, "free:2026-09", "free:2026-09:sequential")

	first, err := repo.ApplyCredit(ctx, request)
	if err != nil {
		t.Fatalf("first ApplyCredit() error = %v", err)
	}
	if first.Replayed {
		t.Fatal("first attempt must not be a replay")
	}

	second, err := repo.ApplyCredit(ctx, request)
	if err != nil {
		t.Fatalf("second ApplyCredit() error = %v", err)
	}
	if !second.Replayed {
		t.Fatal("second attempt with the same key must be a replay")
	}
	if second.Operation.ID() != first.Operation.ID() {
		t.Fatalf("replay returned operation %q, want original %q", second.Operation.ID(), first.Operation.ID())
	}
	if second.Operation.CreatedAt() != first.Operation.CreatedAt() {
		t.Errorf("replay createdAt = %v, want original %v", second.Operation.CreatedAt(), first.Operation.CreatedAt())
	}

	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID); got != 1 {
		t.Fatalf("operations = %d, want 1", got)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_transactions"); got != 1 {
		t.Fatalf("transactions = %d, want 1", got)
	}
	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 5000 {
		t.Fatalf("balance_free = %d, want 5000 (credited exactly once)", wallet.BalanceFree)
	}
}

// TestRepository_ApplyCreditConcurrent runs the P06-T03 acceptance probe:
// twenty identical attempts produce exactly one operation, one transaction
// and one balance change, and every caller resolves the same operation.
func TestRepository_ApplyCreditConcurrent(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t, dbtest.WithPoolLimits(20, 1))
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "credit-concurrent@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))
	request := mustCreditRequest(t, accountID, domain.BucketPurchased, domain.OperationCreditPurchase, 10000, "stripe:evt_concurrent", "purchase:concurrent")

	const attempts = 20
	var (
		successes atomic.Int32
		replays   atomic.Int32
		wg        sync.WaitGroup
	)
	operationIDs := make([]string, attempts)
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := repo.ApplyCredit(ctx, request)
			if err != nil {
				errs[index] = err
				return
			}
			successes.Add(1)
			if result.Replayed {
				replays.Add(1)
			}
			operationIDs[index] = result.Operation.ID().String()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("attempt %d error = %v", i, err)
		}
	}
	if successes.Load() != attempts {
		t.Fatalf("successes = %d, want %d", successes.Load(), attempts)
	}
	if replays.Load() != attempts-1 {
		t.Fatalf("replays = %d, want %d", replays.Load(), attempts-1)
	}
	for i, operationID := range operationIDs {
		if operationID != operationIDs[0] {
			t.Fatalf("attempt %d resolved operation %q, want %q", i, operationID, operationIDs[0])
		}
	}

	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID); got != 1 {
		t.Fatalf("operations = %d, want exactly 1", got)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_transactions"); got != 1 {
		t.Fatalf("transactions = %d, want exactly 1", got)
	}
	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalancePurchased != 10000 || wallet.BalanceFree != 0 {
		t.Fatalf("balances = %d/%d, want 0/10000 (credited exactly once)", wallet.BalanceFree, wallet.BalancePurchased)
	}
}

func TestRepository_ApplyCreditAccumulatesByBucket(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "credit-buckets@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	credits := []application.CreditRequest{
		mustCreditRequest(t, accountID, domain.BucketFree, domain.OperationCreditFree, 5000, "free:2026-09", "bucket-free"),
		mustCreditRequest(t, accountID, domain.BucketPurchased, domain.OperationCreditPurchase, 10000, "stripe:evt_purchase", "bucket-purchase"),
		mustCreditRequest(t, accountID, domain.BucketFree, domain.OperationCreditMember, 30000, "member:2026-09", "bucket-member"),
		mustCreditRequest(t, accountID, domain.BucketPurchased, domain.OperationCreditRefund, 2000, "moderation:case-7", "bucket-refund"),
		mustCreditRequest(t, accountID, domain.BucketFree, domain.OperationCreditAdmin, 100, "admin:ticket-9", "bucket-admin"),
	}
	for _, request := range credits {
		if _, err := repo.ApplyCredit(ctx, request); err != nil {
			t.Fatalf("ApplyCredit(%s) error = %v", request.OperationType, err)
		}
	}

	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 35100 || wallet.BalancePurchased != 12000 {
		t.Fatalf("balances = %d/%d, want 35100/12000", wallet.BalanceFree, wallet.BalancePurchased)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID); got != len(credits) {
		t.Fatalf("operations = %d, want %d", got, len(credits))
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_transactions"); got != len(credits) {
		t.Fatalf("transactions = %d, want %d", got, len(credits))
	}
}

func TestRepository_ApplyCreditRefusesCrossAccountReplay(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	owner := mustWalletAccount(t, ctx, q, "credit-owner@arena.example.com")
	intruder := mustWalletAccount(t, ctx, q, "credit-intruder@arena.example.com")
	ownerID := domain.AccountID(uuidString(owner.ID))
	intruderID := domain.AccountID(uuidString(intruder.ID))

	if _, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, ownerID, domain.BucketFree, domain.OperationCreditFree, 5000, "free:2026-09", "shared-key",
	)); err != nil {
		t.Fatalf("owner credit error = %v", err)
	}

	_, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, intruderID, domain.BucketFree, domain.OperationCreditFree, 5000, "free:2026-09", "shared-key",
	))
	if !errors.Is(err, application.ErrIdempotencyMismatch) {
		t.Fatalf("cross-account replay error = %v, want ErrIdempotencyMismatch", err)
	}

	// The intruder's wallet row was rolled back with the refused attempt.
	if _, err := q.GetWalletAccount(ctx, intruder.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("intruder wallet error = %v, want ErrNoRows (no partial state)", err)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", owner.ID); got != 1 {
		t.Fatalf("owner operations = %d, want 1", got)
	}
	ownerWallet, err := q.GetWalletAccount(ctx, owner.ID)
	if err != nil {
		t.Fatalf("get owner wallet: %v", err)
	}
	if ownerWallet.BalanceFree != 5000 {
		t.Fatalf("owner balance_free = %d, want 5000", ownerWallet.BalanceFree)
	}
}

// TestRepository_ApplyCreditRollsBackOnBalanceFailure proves the operation,
// transaction and balance change are one atomic unit: a balance overflow
// leaves no partial ledger rows.
func TestRepository_ApplyCreditRollsBackOnBalanceFailure(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "credit-overflow@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	// Seed the wallet and push the free balance to the 64-bit ceiling.
	if _, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketFree, domain.OperationCreditFree, 1, "free:seed", "overflow-seed",
	)); err != nil {
		t.Fatalf("seed credit error = %v", err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE app.wallet_accounts SET balance_free = 9223372036854775807 WHERE account_id = $1", acc.ID,
	); err != nil {
		t.Fatalf("seed max balance: %v", err)
	}
	baseline := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID)

	_, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketFree, domain.OperationCreditFree, 1, "free:overflow", "overflow-credit",
	))
	if err == nil {
		t.Fatal("credit beyond the bigint ceiling must fail")
	}

	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID); got != baseline {
		t.Fatalf("operations after failure = %d, want %d (no partial state)", got, baseline)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_transactions"); got != baseline {
		t.Fatalf("transactions after failure = %d, want %d (no partial state)", got, baseline)
	}
	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 9223372036854775807 {
		t.Fatalf("balance_free = %d, want the untouched ceiling", wallet.BalanceFree)
	}
}
