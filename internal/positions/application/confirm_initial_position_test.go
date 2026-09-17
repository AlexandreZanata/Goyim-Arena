package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/Goyim-Arena/internal/positions/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/positions/domain"
)

const (
	testArenaRaw   = "018f6b2a-0000-7000-8000-000000000001"
	testAccountRaw = "018f6b2a-0000-7000-8000-000000000002"
)

var testInstant = time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fakeAccountEligibility struct {
	mu    sync.Mutex
	err   error
	calls []domain.AccountID
}

func (f *fakeAccountEligibility) EnsureEligible(_ context.Context, accountID domain.AccountID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, accountID)
	return f.err
}

func (f *fakeAccountEligibility) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fakeArenaEligibility struct {
	mu    sync.Mutex
	err   error
	calls []domain.ArenaID
}

func (f *fakeArenaEligibility) EnsureAcceptsPositions(_ context.Context, arenaID domain.ArenaID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, arenaID)
	return f.err
}

func (f *fakeArenaEligibility) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fakePositionRepo struct {
	mu sync.Mutex

	stored  map[string]*domain.DebatePosition
	inserts int

	getErr             error
	firstGetErr        error
	confirmErr         error
	confirmNotInserted bool
}

func newFakePositionRepo() *fakePositionRepo {
	return &fakePositionRepo{stored: map[string]*domain.DebatePosition{}}
}

func positionKey(arenaID domain.ArenaID, accountID domain.AccountID) string {
	return arenaID.String() + "|" + accountID.String()
}

func (r *fakePositionRepo) GetByAccountAndArena(_ context.Context, arenaID domain.ArenaID, accountID domain.AccountID) (*domain.DebatePosition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.firstGetErr != nil {
		err := r.firstGetErr
		r.firstGetErr = nil
		return nil, err
	}
	if r.getErr != nil {
		return nil, r.getErr
	}
	stored, ok := r.stored[positionKey(arenaID, accountID)]
	if !ok {
		return nil, application.ErrPositionNotFound
	}
	return stored, nil
}

func (r *fakePositionRepo) ConfirmInitialPosition(_ context.Context, arenaID domain.ArenaID, accountID domain.AccountID, position domain.Position, at time.Time) (*domain.DebatePosition, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.confirmErr != nil {
		return nil, false, r.confirmErr
	}
	key := positionKey(arenaID, accountID)
	if stored, ok := r.stored[key]; ok {
		return stored, false, nil
	}
	if r.confirmNotInserted {
		return nil, false, nil
	}
	created, err := domain.ConfirmInitialPosition(arenaID, accountID, position, at)
	if err != nil {
		return nil, false, err
	}
	r.stored[key] = created
	r.inserts++
	return created, true, nil
}

func (r *fakePositionRepo) insertCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inserts
}

func (r *fakePositionRepo) storedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.stored)
}

func (r *fakePositionRepo) storedPosition(t *testing.T, arenaID domain.ArenaID, accountID domain.AccountID) *domain.DebatePosition {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.stored[positionKey(arenaID, accountID)]
	if !ok {
		t.Fatal("no stored position")
	}
	return stored
}

func newConfirmUseCase(repo *fakePositionRepo, accounts *fakeAccountEligibility, arenas *fakeArenaEligibility) *application.ConfirmInitialPositionUseCase {
	return application.NewConfirmInitialPositionUseCase(repo, accounts, arenas, fixedClock{now: testInstant})
}

func confirmCommand(position string) application.ConfirmInitialPositionCommand {
	return application.ConfirmInitialPositionCommand{
		AccountID: testAccountRaw,
		ArenaID:   testArenaRaw,
		Position:  position,
	}
}

func TestConfirmInitialPositionRecordsTheFirstChoice(t *testing.T) {
	repo := newFakePositionRepo()
	accounts := &fakeAccountEligibility{}
	arenas := &fakeArenaEligibility{}
	useCase := newConfirmUseCase(repo, accounts, arenas)

	result, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Replayed {
		t.Fatal("first confirmation must not be a replay")
	}
	if !result.Position.InitialPosition().Equals(mustPosition(t, domain.PositionAgree)) {
		t.Fatal("initial position was not recorded")
	}
	if !result.Position.CurrentPosition().Equals(result.Position.InitialPosition()) || result.Position.Version() != 1 {
		t.Fatal("confirmation must start current equal to initial at version 1")
	}
	if !result.Position.CreatedAt().Equal(testInstant) {
		t.Fatal("confirmation must use the injected clock")
	}
	if repo.insertCount() != 1 || repo.storedCount() != 1 {
		t.Fatalf("inserts = %d, stored = %d, want exactly one", repo.insertCount(), repo.storedCount())
	}
	if accounts.callCount() != 1 || arenas.callCount() != 1 {
		t.Fatal("first confirmation must check account and arena eligibility exactly once")
	}
}

func TestConfirmInitialPositionIsIdempotentOnRetry(t *testing.T) {
	repo := newFakePositionRepo()
	accounts := &fakeAccountEligibility{}
	arenas := &fakeArenaEligibility{}
	useCase := newConfirmUseCase(repo, accounts, arenas)

	first, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionDisagree))
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	retry, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionDisagree))
	if err != nil {
		t.Fatalf("retry Execute() error = %v", err)
	}
	if !retry.Replayed {
		t.Fatal("retry must resolve a replay")
	}
	if retry.Position.Version() != first.Position.Version() || !retry.Position.InitialPosition().Equals(first.Position.InitialPosition()) {
		t.Fatal("replay must return the recorded projection")
	}
	if repo.insertCount() != 1 || repo.storedCount() != 1 {
		t.Fatal("retry must not write again")
	}
	if accounts.callCount() != 1 || arenas.callCount() != 1 {
		t.Fatal("replay must not re-check eligibility")
	}
}

func TestConfirmInitialPositionRefusesDifferentInitialValue(t *testing.T) {
	repo := newFakePositionRepo()
	accounts := &fakeAccountEligibility{}
	arenas := &fakeArenaEligibility{}
	useCase := newConfirmUseCase(repo, accounts, arenas)

	if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionUndecided)); !errors.Is(err, application.ErrInitialPositionAlreadySet) {
		t.Fatalf("different value error = %v, want ErrInitialPositionAlreadySet", err)
	}

	stored := repo.storedPosition(t, mustArenaID(t), mustAccountID(t))
	if !stored.InitialPosition().Equals(mustPosition(t, domain.PositionAgree)) || stored.Version() != 1 {
		t.Fatal("refused confirmation mutated the stored projection")
	}
	if repo.insertCount() != 1 {
		t.Fatal("refused confirmation must not write")
	}
}

func TestConfirmInitialPositionChecksEligibilityOnlyForTheFirstWrite(t *testing.T) {
	repo := newFakePositionRepo()
	accounts := &fakeAccountEligibility{err: application.ErrAccountSuspended}
	arenas := &fakeArenaEligibility{}
	useCase := newConfirmUseCase(repo, accounts, arenas)

	if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); !errors.Is(err, application.ErrAccountSuspended) {
		t.Fatalf("suspended account error = %v, want ErrAccountSuspended", err)
	}
	if repo.storedCount() != 0 || repo.insertCount() != 0 {
		t.Fatal("suspended account must not write")
	}
	if arenas.callCount() != 0 {
		t.Fatal("arena eligibility must not be checked after an account refusal")
	}

	accounts.err = nil
	arenas.err = application.ErrArenaNotOpen
	if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); !errors.Is(err, application.ErrArenaNotOpen) {
		t.Fatalf("closed arena error = %v, want ErrArenaNotOpen", err)
	}
	if repo.storedCount() != 0 || repo.insertCount() != 0 {
		t.Fatal("closed arena must not write")
	}

	// Missing accounts and Arenas propagate their own errors.
	accounts.err = application.ErrAccountNotFound
	arenas.err = nil
	if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); !errors.Is(err, application.ErrAccountNotFound) {
		t.Fatalf("missing account error = %v, want ErrAccountNotFound", err)
	}
	accounts.err = nil
	arenas.err = application.ErrArenaNotFound
	if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); !errors.Is(err, application.ErrArenaNotFound) {
		t.Fatalf("missing arena error = %v, want ErrArenaNotFound", err)
	}
	if repo.storedCount() != 0 || repo.insertCount() != 0 {
		t.Fatal("ineligible pairs must not write")
	}

	// A recorded confirmation replays even after the Arena closes.
	arenas.err = nil
	if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); err != nil {
		t.Fatalf("confirmation after eligibility returned: %v", err)
	}
	arenas.err = application.ErrArenaNotOpen
	accounts.err = application.ErrAccountSuspended
	replay, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree))
	if err != nil {
		t.Fatalf("replay after closure error = %v", err)
	}
	if !replay.Replayed {
		t.Fatal("replay after closure must return the recorded result")
	}
}

func TestConfirmInitialPositionResolvesLostInsertRaces(t *testing.T) {
	arenaID := mustArenaID(t)
	accountID := mustAccountID(t)

	t.Run("same value replays", func(t *testing.T) {
		repo := newFakePositionRepo()
		winner, err := domain.ConfirmInitialPosition(arenaID, accountID, mustPosition(t, domain.PositionAgree), testInstant)
		if err != nil {
			t.Fatalf("seed projection: %v", err)
		}
		repo.stored[positionKey(arenaID, accountID)] = winner
		repo.firstGetErr = application.ErrPositionNotFound
		repo.confirmNotInserted = true

		useCase := newConfirmUseCase(repo, &fakeAccountEligibility{}, &fakeArenaEligibility{})
		result, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree))
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if !result.Replayed || result.Position.Version() != 1 {
			t.Fatal("lost race with the same value must resolve the replay")
		}
	})

	t.Run("different value conflicts", func(t *testing.T) {
		repo := newFakePositionRepo()
		winner, err := domain.ConfirmInitialPosition(arenaID, accountID, mustPosition(t, domain.PositionDisagree), testInstant)
		if err != nil {
			t.Fatalf("seed projection: %v", err)
		}
		repo.stored[positionKey(arenaID, accountID)] = winner
		repo.firstGetErr = application.ErrPositionNotFound
		repo.confirmNotInserted = true

		useCase := newConfirmUseCase(repo, &fakeAccountEligibility{}, &fakeArenaEligibility{})
		if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); !errors.Is(err, application.ErrInitialPositionAlreadySet) {
			t.Fatalf("lost race with a different value error = %v, want ErrInitialPositionAlreadySet", err)
		}
	})
}

func TestConfirmInitialPositionValidatesInputsWithoutTouchingPorts(t *testing.T) {
	probes := []struct {
		name    string
		command application.ConfirmInitialPositionCommand
		want    error
	}{
		{name: "empty account", command: application.ConfirmInitialPositionCommand{ArenaID: testArenaRaw, Position: domain.PositionAgree}, want: domain.ErrEmptyAccountID},
		{name: "empty arena", command: application.ConfirmInitialPositionCommand{AccountID: testAccountRaw, Position: domain.PositionAgree}, want: domain.ErrEmptyArenaID},
		{name: "empty position", command: application.ConfirmInitialPositionCommand{AccountID: testAccountRaw, ArenaID: testArenaRaw}, want: domain.ErrEmptyPosition},
		{name: "unknown position", command: application.ConfirmInitialPositionCommand{AccountID: testAccountRaw, ArenaID: testArenaRaw, Position: "maybe"}, want: domain.ErrInvalidPosition},
	}
	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			repo := newFakePositionRepo()
			accounts := &fakeAccountEligibility{}
			arenas := &fakeArenaEligibility{}
			useCase := newConfirmUseCase(repo, accounts, arenas)

			if _, err := useCase.Execute(context.Background(), probe.command); !errors.Is(err, probe.want) {
				t.Fatalf("error = %v, want %v", err, probe.want)
			}
			if repo.storedCount() != 0 || repo.insertCount() != 0 || accounts.callCount() != 0 || arenas.callCount() != 0 {
				t.Fatal("invalid input must not touch any port")
			}
		})
	}
}

func TestConfirmInitialPositionPropagatesStorageFailures(t *testing.T) {
	storageErr := errors.New("storage down")

	t.Run("read failure", func(t *testing.T) {
		repo := newFakePositionRepo()
		repo.getErr = storageErr
		useCase := newConfirmUseCase(repo, &fakeAccountEligibility{}, &fakeArenaEligibility{})
		if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); !errors.Is(err, storageErr) {
			t.Fatalf("error = %v, want the storage failure", err)
		}
	})

	t.Run("insert failure", func(t *testing.T) {
		repo := newFakePositionRepo()
		repo.confirmErr = storageErr
		useCase := newConfirmUseCase(repo, &fakeAccountEligibility{}, &fakeArenaEligibility{})
		if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); !errors.Is(err, storageErr) {
			t.Fatalf("error = %v, want the storage failure", err)
		}
	})

	t.Run("re-read failure after a lost race", func(t *testing.T) {
		repo := newFakePositionRepo()
		repo.firstGetErr = application.ErrPositionNotFound
		repo.confirmNotInserted = true
		useCase := newConfirmUseCase(repo, &fakeAccountEligibility{}, &fakeArenaEligibility{})
		if _, err := useCase.Execute(context.Background(), confirmCommand(domain.PositionAgree)); !errors.Is(err, application.ErrPositionNotFound) {
			t.Fatalf("error = %v, want ErrPositionNotFound", err)
		}
	})
}

// TestConfirmInitialPositionConcurrentDifferentValues is the P09-T03
// concurrency proof: many simultaneous confirmations with different values
// produce exactly one initial position; calls matching the winner replay and
// calls with the losing value conflict.
func TestConfirmInitialPositionConcurrentDifferentValues(t *testing.T) {
	repo := newFakePositionRepo()
	accounts := &fakeAccountEligibility{}
	arenas := &fakeArenaEligibility{}
	useCase := newConfirmUseCase(repo, accounts, arenas)

	const perValue = 6
	values := []string{domain.PositionAgree, domain.PositionDisagree}

	type outcome struct {
		value    string
		replayed bool
		err      error
	}

	start := make(chan struct{})
	outcomes := make(chan outcome, perValue*len(values))
	var waitGroup sync.WaitGroup
	for _, value := range values {
		for i := 0; i < perValue; i++ {
			waitGroup.Add(1)
			go func(value string) {
				defer waitGroup.Done()
				<-start
				result, err := useCase.Execute(context.Background(), confirmCommand(value))
				if err != nil {
					outcomes <- outcome{value: value, err: err}
					return
				}
				outcomes <- outcome{value: value, replayed: result.Replayed}
			}(value)
		}
	}
	close(start)
	waitGroup.Wait()
	close(outcomes)

	fresh := 0
	replayed := map[string]int{}
	conflicts := map[string]int{}
	winner := ""
	for result := range outcomes {
		switch {
		case result.err == nil && !result.replayed:
			fresh++
			winner = result.value
		case result.err == nil && result.replayed:
			replayed[result.value]++
		case errors.Is(result.err, application.ErrInitialPositionAlreadySet):
			conflicts[result.value]++
		default:
			t.Fatalf("unexpected outcome: value %q, error %v", result.value, result.err)
		}
	}

	if fresh != 1 {
		t.Fatalf("fresh confirmations = %d, want exactly 1", fresh)
	}
	if winner == "" {
		t.Fatal("no winning value recorded")
	}
	if replayed[winner] != perValue-1 {
		t.Fatalf("replays for the winner = %d, want %d", replayed[winner], perValue-1)
	}
	loser := domain.PositionAgree
	if winner == domain.PositionAgree {
		loser = domain.PositionDisagree
	}
	if replayed[loser] != 0 || conflicts[loser] != perValue {
		t.Fatalf("loser outcomes = %d replays / %d conflicts, want 0/%d", replayed[loser], conflicts[loser], perValue)
	}

	if repo.insertCount() != 1 || repo.storedCount() != 1 {
		t.Fatalf("inserts = %d, stored = %d, want exactly one", repo.insertCount(), repo.storedCount())
	}
	stored := repo.storedPosition(t, mustArenaID(t), mustAccountID(t))
	if stored.InitialPosition().String() != winner || stored.Version() != 1 {
		t.Fatalf("stored winner = %q v%d, want %q v1", stored.InitialPosition().String(), stored.Version(), winner)
	}
}

func mustArenaID(t *testing.T) domain.ArenaID {
	t.Helper()
	arenaID, err := domain.ParseArenaID(testArenaRaw)
	if err != nil {
		t.Fatalf("ParseArenaID: %v", err)
	}
	return arenaID
}

func mustAccountID(t *testing.T) domain.AccountID {
	t.Helper()
	accountID, err := domain.ParseAccountID(testAccountRaw)
	if err != nil {
		t.Fatalf("ParseAccountID: %v", err)
	}
	return accountID
}

func mustPosition(t *testing.T, raw string) domain.Position {
	t.Helper()
	position, err := domain.ParsePosition(raw)
	if err != nil {
		t.Fatalf("ParsePosition(%q): %v", raw, err)
	}
	return position
}
