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

func mustDebitRequest(
	t *testing.T,
	accountID domain.AccountID,
	operationType domain.OperationType,
	amount int64,
	reference, idempotencyKey string,
) application.DebitRequest {
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
	return application.DebitRequest{
		AccountID:      accountID,
		OperationType:  operationType,
		IdempotencyKey: key,
		Reference:      ref,
		Amount:         ink,
		ChangedAt:      time.Now().UTC(),
	}
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatalf("scan uuid %q: %v", raw, err)
	}
	return id
}

func assertAllocation(t *testing.T, allocation domain.Allocation, wantFree, wantPurchased int64) {
	t.Helper()
	if allocation.FromFree().Int64() != wantFree || allocation.FromPurchased().Int64() != wantPurchased {
		t.Fatalf("allocation = %d/%d, want %d/%d",
			allocation.FromFree().Int64(), allocation.FromPurchased().Int64(), wantFree, wantPurchased)
	}
}

func TestRepository_ApplyDebitFreeOnly(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "debit-free@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	if _, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketFree, domain.OperationCreditFree, 5000, "free:2026-09", "debit-free:credit",
	)); err != nil {
		t.Fatalf("seed credit error = %v", err)
	}

	result, err := repo.ApplyDebit(ctx, mustDebitRequest(
		t, accountID, domain.OperationDebitArgument, 3000, "argument:1", "debit-free:debit",
	))
	if err != nil {
		t.Fatalf("ApplyDebit() error = %v", err)
	}
	if result.Replayed {
		t.Fatal("fresh debit must not be a replay")
	}
	assertAllocation(t, result.Allocation, 3000, 0)

	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 2000 || wallet.BalancePurchased != 0 {
		t.Fatalf("balances = %d/%d, want 2000/0", wallet.BalanceFree, wallet.BalancePurchased)
	}

	entries, err := q.ListWalletTransactionsByAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("transactions = %d, want 2 (credit + debit)", len(entries))
	}
	if entries[0].Amount != -3000 || entries[0].Bucket != "FREE_INK" || entries[0].OperationType != "debit_argument" {
		t.Fatalf("newest entry = %+v, want a -3000 FREE_INK debit", entries[0])
	}
	if entries[1].Amount != 5000 {
		t.Fatalf("oldest entry = %+v, want the +5000 credit", entries[1])
	}
}

func TestRepository_ApplyDebitSplitsAcrossBuckets(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "debit-split@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	if _, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketFree, domain.OperationCreditFree, 5000, "free:2026-09", "debit-split:free",
	)); err != nil {
		t.Fatalf("free credit error = %v", err)
	}
	if _, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketPurchased, domain.OperationCreditPurchase, 10000, "stripe:evt_split", "debit-split:purchased",
	)); err != nil {
		t.Fatalf("purchased credit error = %v", err)
	}

	result, err := repo.ApplyDebit(ctx, mustDebitRequest(
		t, accountID, domain.OperationDebitArgument, 12000, "argument:2", "debit-split:debit",
	))
	if err != nil {
		t.Fatalf("ApplyDebit() error = %v", err)
	}
	assertAllocation(t, result.Allocation, 5000, 7000)

	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 0 || wallet.BalancePurchased != 3000 {
		t.Fatalf("balances = %d/%d, want 0/3000", wallet.BalanceFree, wallet.BalancePurchased)
	}

	lines, err := q.ListWalletTransactionsByOperationID(ctx, mustUUID(t, result.Operation.ID().String()))
	if err != nil {
		t.Fatalf("list debit lines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("debit lines = %d, want 2 (split)", len(lines))
	}
	if lines[0].Bucket != "FREE_INK" || lines[0].Amount != -5000 {
		t.Errorf("line 0 = %s/%d, want FREE_INK/-5000", lines[0].Bucket, lines[0].Amount)
	}
	if lines[1].Bucket != "PURCHASED_INK" || lines[1].Amount != -7000 {
		t.Errorf("line 1 = %s/%d, want PURCHASED_INK/-7000", lines[1].Bucket, lines[1].Amount)
	}
}

func TestRepository_ApplyDebitPurchasedOnly(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "debit-purchased@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	if _, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketPurchased, domain.OperationCreditPurchase, 10000, "stripe:evt_only", "debit-purchased:credit",
	)); err != nil {
		t.Fatalf("credit error = %v", err)
	}

	result, err := repo.ApplyDebit(ctx, mustDebitRequest(
		t, accountID, domain.OperationDebitAdmin, 1000, "admin:ticket-1", "debit-purchased:debit",
	))
	if err != nil {
		t.Fatalf("ApplyDebit() error = %v", err)
	}
	assertAllocation(t, result.Allocation, 0, 1000)

	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 0 || wallet.BalancePurchased != 9000 {
		t.Fatalf("balances = %d/%d, want 0/9000", wallet.BalanceFree, wallet.BalancePurchased)
	}
}

func TestRepository_ApplyDebitInsufficientLeavesNoPartialState(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "debit-insufficient@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	if _, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketFree, domain.OperationCreditFree, 100, "free:seed", "insufficient:seed",
	)); err != nil {
		t.Fatalf("seed credit error = %v", err)
	}
	baselineOperations := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID)
	baselineTransactions := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_transactions")

	request := mustDebitRequest(t, accountID, domain.OperationDebitArgument, 101, "argument:3", "insufficient:debit")
	if _, err := repo.ApplyDebit(ctx, request); !errors.Is(err, domain.ErrInsufficientInk) {
		t.Fatalf("insufficient debit error = %v, want ErrInsufficientInk", err)
	}

	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID); got != baselineOperations {
		t.Fatalf("operations after failure = %d, want %d", got, baselineOperations)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_transactions"); got != baselineTransactions {
		t.Fatalf("transactions after failure = %d, want %d", got, baselineTransactions)
	}
	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 100 {
		t.Fatalf("balance_free = %d, want 100 untouched", wallet.BalanceFree)
	}

	// The failed attempt did not burn the idempotency key: after funding,
	// the same request succeeds with the same key.
	if _, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketFree, domain.OperationCreditFree, 1000, "free:refill", "insufficient:refill",
	)); err != nil {
		t.Fatalf("refill credit error = %v", err)
	}
	result, err := repo.ApplyDebit(ctx, request)
	if err != nil {
		t.Fatalf("retry after funding error = %v", err)
	}
	if result.Replayed || result.Operation.Reference().String() != "argument:3" {
		t.Fatalf("retry result = %+v, want a fresh debit of argument:3", result)
	}
}

func TestRepository_ApplyDebitWithoutWalletIsInsufficient(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "debit-no-wallet@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	if _, err := repo.ApplyDebit(ctx, mustDebitRequest(
		t, accountID, domain.OperationDebitArgument, 1, "argument:4", "no-wallet:debit",
	)); !errors.Is(err, domain.ErrInsufficientInk) {
		t.Fatalf("debit without wallet error = %v, want ErrInsufficientInk", err)
	}

	if _, err := q.GetWalletAccount(ctx, acc.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("wallet error = %v, want ErrNoRows (no wallet created)", err)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID); got != 0 {
		t.Fatalf("operations = %d, want 0", got)
	}
}

func TestRepository_ApplyDebitIsIdempotent(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t)
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "debit-idempotent@arena.example.com")
	other := mustWalletAccount(t, ctx, q, "debit-idempotent-other@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))
	otherID := domain.AccountID(uuidString(other.ID))

	for _, credit := range []application.CreditRequest{
		mustCreditRequest(t, accountID, domain.BucketFree, domain.OperationCreditFree, 5000, "free:2026-09", "debit-idempotent:free"),
		mustCreditRequest(t, accountID, domain.BucketPurchased, domain.OperationCreditPurchase, 10000, "stripe:evt_idem", "debit-idempotent:purchased"),
	} {
		if _, err := repo.ApplyCredit(ctx, credit); err != nil {
			t.Fatalf("seed credit error = %v", err)
		}
	}

	request := mustDebitRequest(t, accountID, domain.OperationDebitArgument, 12000, "argument:5", "debit-idempotent:debit")
	first, err := repo.ApplyDebit(ctx, request)
	if err != nil {
		t.Fatalf("first ApplyDebit() error = %v", err)
	}
	assertAllocation(t, first.Allocation, 5000, 7000)

	second, err := repo.ApplyDebit(ctx, request)
	if err != nil {
		t.Fatalf("second ApplyDebit() error = %v", err)
	}
	if !second.Replayed {
		t.Fatal("second attempt with the same key must be a replay")
	}
	if second.Operation.ID() != first.Operation.ID() {
		t.Fatalf("replay operation = %q, want %q", second.Operation.ID(), first.Operation.ID())
	}
	if !second.Allocation.Equals(first.Allocation) {
		t.Fatalf("replay allocation = %d/%d, want %d/%d",
			second.Allocation.FromFree().Int64(), second.Allocation.FromPurchased().Int64(),
			first.Allocation.FromFree().Int64(), first.Allocation.FromPurchased().Int64())
	}

	// The key is global: another account cannot replay it.
	if _, err := repo.ApplyDebit(ctx, mustDebitRequest(
		t, otherID, domain.OperationDebitArgument, 12000, "argument:5", "debit-idempotent:debit",
	)); !errors.Is(err, application.ErrIdempotencyMismatch) {
		t.Fatalf("cross-account replay error = %v, want ErrIdempotencyMismatch", err)
	}

	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 0 || wallet.BalancePurchased != 3000 {
		t.Fatalf("balances = %d/%d, want 0/3000 (debited exactly once)", wallet.BalanceFree, wallet.BalancePurchased)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID); got != 3 {
		t.Fatalf("operations = %d, want 3 (2 credits + 1 debit)", got)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_transactions"); got != 4 {
		t.Fatalf("transactions = %d, want 4 (2 credits + 1 split debit)", got)
	}
}

// TestRepository_ApplyDebitConcurrentNoDoubleSpend is the THR-WAL-01 probe:
// fifty concurrent debits compete for a balance that funds exactly one
// operation. Exactly one succeeds, forty-nine fail with insufficient ink,
// and the balance never goes negative.
func TestRepository_ApplyDebitConcurrentNoDoubleSpend(t *testing.T) {
	ctx := context.Background()
	testDB := dbtest.New(t, dbtest.WithPoolLimits(50, 1))
	pool := testDB.Pool.Pool()
	repo := walletpg.NewRepository(pool)
	q := platformpg.New(pool)

	acc := mustWalletAccount(t, ctx, q, "debit-race@arena.example.com")
	accountID := domain.AccountID(uuidString(acc.ID))

	if _, err := repo.ApplyCredit(ctx, mustCreditRequest(
		t, accountID, domain.BucketFree, domain.OperationCreditFree, 10000, "free:2026-09", "debit-race:seed",
	)); err != nil {
		t.Fatalf("seed credit error = %v", err)
	}

	// Build every request before spawning goroutines: testing.T must not
	// fail from a non-test goroutine.
	const attempts = 50
	requests := make([]application.DebitRequest, attempts)
	for i := range requests {
		requests[i] = mustDebitRequest(
			t, accountID, domain.OperationDebitArgument, 10000,
			fmt.Sprintf("argument:%d", i), fmt.Sprintf("debit-race:%d", i),
		)
	}

	var (
		successes    atomic.Int32
		insufficient atomic.Int32
		wg           sync.WaitGroup
	)
	unexpected := make([]error, attempts)
	results := make([]*application.DebitResult, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := repo.ApplyDebit(ctx, requests[index])
			if err != nil {
				if errors.Is(err, domain.ErrInsufficientInk) {
					insufficient.Add(1)
				} else {
					unexpected[index] = err
				}
				return
			}
			successes.Add(1)
			results[index] = result
		}(i)
	}
	wg.Wait()

	for i, err := range unexpected {
		if err != nil {
			t.Fatalf("attempt %d unexpected error = %v", i, err)
		}
	}
	if successes.Load() != 1 {
		t.Fatalf("successes = %d, want exactly 1", successes.Load())
	}
	if insufficient.Load() != attempts-1 {
		t.Fatalf("insufficient = %d, want %d", insufficient.Load(), attempts-1)
	}

	wallet, err := q.GetWalletAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get wallet: %v", err)
	}
	if wallet.BalanceFree != 0 || wallet.BalancePurchased != 0 {
		t.Fatalf("balances = %d/%d, want 0/0 (no double spend, never negative)", wallet.BalanceFree, wallet.BalancePurchased)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_operations WHERE account_id = $1", acc.ID); got != 2 {
		t.Fatalf("operations = %d, want 2 (1 credit + 1 debit)", got)
	}
	if got := countRows(t, ctx, pool, "SELECT count(*) FROM app.wallet_transactions"); got != 2 {
		t.Fatalf("transactions = %d, want 2 (1 credit + 1 debit)", got)
	}

	var winner *application.DebitResult
	for _, result := range results {
		if result != nil {
			winner = result
		}
	}
	if winner == nil {
		t.Fatal("no successful debit produced an operation")
	}
	assertAllocation(t, winner.Allocation, 10000, 0)
}
