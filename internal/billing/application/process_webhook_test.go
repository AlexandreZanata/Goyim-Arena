package application_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/Regnovum/internal/billing/application"
	"github.com/AlexandreZanata/Regnovum/internal/billing/domain"
)

// fakeWebhookVerifier is a stub that accepts or rejects payloads based on the
// configured result.
type fakeWebhookVerifier struct {
	verifyResult error
}

func (f *fakeWebhookVerifier) Verify(_ []byte, _ string, _ string) error {
	return f.verifyResult
}

// fakeWebhookEventRepository is an in-memory implementation of the webhook
// event repository for unit testing.
type fakeWebhookEventRepository struct {
	mu     sync.Mutex
	events map[string]*application.WebhookEventRecord
}

func newFakeWebhookEventRepository() *fakeWebhookEventRepository {
	return &fakeWebhookEventRepository{
		events: make(map[string]*application.WebhookEventRecord),
	}
}

func (f *fakeWebhookEventRepository) ClaimEvent(_ context.Context, request application.ClaimWebhookEventRequest) (*application.ClaimWebhookEventResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	eventID := request.EventID.String()
	if existing, ok := f.events[eventID]; ok {
		// Replay: resolve the existing row.
		return &application.ClaimWebhookEventResult{
			Record:   *existing,
			Replayed: true,
		}, nil
	}

	// New event: insert it.
	now := time.Now().UTC()
	record := &application.WebhookEventRecord{
		ID:              fmt.Sprintf("local-%d", len(f.events)+1),
		EventID:         request.EventID,
		EventType:       request.EventType,
		Livemode:        request.Livemode,
		StripeCreatedAt: request.StripeCreatedAt,
		PayloadSHA256:   request.PayloadSHA256,
		PayloadBytes:    request.PayloadBytes,
		Status:          application.WebhookEventProcessing,
		Attempts:        1,
		ReceivedAt:      now,
	}
	f.events[eventID] = record
	return &application.ClaimWebhookEventResult{
		Record:   *record,
		Replayed: false,
	}, nil
}

func (f *fakeWebhookEventRepository) MarkProcessed(_ context.Context, eventID domain.WebhookEventID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if record, ok := f.events[eventID.String()]; ok {
		record.Status = application.WebhookEventProcessed
		now := time.Now().UTC()
		record.ProcessedAt = &now
		return nil
	}
	return fmt.Errorf("event %s not found", eventID)
}

func (f *fakeWebhookEventRepository) MarkFailed(_ context.Context, eventID domain.WebhookEventID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if record, ok := f.events[eventID.String()]; ok {
		record.Status = application.WebhookEventFailed
		record.LastError = reason
		return nil
	}
	return fmt.Errorf("event %s not found", eventID)
}

func (f *fakeWebhookEventRepository) MarkIgnored(_ context.Context, eventID domain.WebhookEventID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if record, ok := f.events[eventID.String()]; ok {
		record.Status = application.WebhookEventIgnored
		now := time.Now().UTC()
		record.ProcessedAt = &now
		return nil
	}
	return fmt.Errorf("event %s not found", eventID)
}

// testCheckoutSessionBody builds a minimal checkout.session.completed webhook
// body with the given payment status.
func testCheckoutSessionBody(paymentStatus string) []byte {
	return []byte(fmt.Sprintf(`{
		"id": "evt_test123",
		"type": "checkout.session.completed",
		"livemode": false,
		"created": 1234567890,
		"data": {
			"object": {
				"id": "cs_test_session1",
				"status": "complete",
				"payment_status": "%s"
			}
		}
	}`, paymentStatus))
}

// testCheckoutSessionExpiredBody builds a checkout.session.expired webhook body.
func testCheckoutSessionExpiredBody() []byte {
	return []byte(`{
		"id": "evt_testexpired",
		"type": "checkout.session.expired",
		"livemode": false,
		"created": 1234567891,
		"data": {
			"object": {
				"id": "cs_test_expired1",
				"status": "expired",
				"payment_status": "unpaid"
			}
		}
	}`)
}

// testSubscriptionBody builds a customer.subscription.updated webhook body.
func testSubscriptionBody() []byte {
	return []byte(`{
		"id": "evt_testsub",
		"type": "customer.subscription.updated",
		"livemode": false,
		"created": 1234567892,
		"data": {
			"object": {
				"status": "active"
			}
		}
	}`)
}

// testUnknownEventBody builds a webhook body with an unknown event type.
func testUnknownEventBody() []byte {
	return []byte(`{
		"id": "evt_testunknown",
		"type": "invoice.payment_failed",
		"livemode": false,
		"created": 1234567893,
		"data": {
			"object": {}
		}
	}`)
}

func TestProcessWebhookRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	repo := newFakeWebhookEventRepository()
	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier: &fakeWebhookVerifier{verifyResult: nil},
		Events:   repo,
		Clock:    &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	// Build a body larger than 1 MiB.
	oversized := make([]byte, 1<<20+1)
	for i := range oversized {
		oversized[i] = 'a'
	}

	err = useCase.Execute(context.Background(), application.ProcessWebhookCommand{
		RawBody:         oversized,
		SignatureHeader: "t=123,v1=abc",
		TimestampHeader: "123",
	})
	if !errors.Is(err, application.ErrWebhookPayloadTooLarge) {
		t.Fatalf("error = %v, want ErrWebhookPayloadTooLarge", err)
	}
	if len(repo.events) != 0 {
		t.Fatal("an oversized body must not be persisted")
	}
}

func TestProcessWebhookRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	repo := newFakeWebhookEventRepository()
	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier: &fakeWebhookVerifier{
			verifyResult: fmt.Errorf("%w: test reason", application.ErrWebhookSignatureInvalid),
		},
		Events: repo,
		Clock:  &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	body := testCheckoutSessionBody("paid")
	err = useCase.Execute(context.Background(), application.ProcessWebhookCommand{
		RawBody:         body,
		SignatureHeader: "t=123,v1=invalid",
		TimestampHeader: "123",
	})
	if !errors.Is(err, application.ErrWebhookSignatureInvalid) {
		t.Fatalf("error = %v, want ErrWebhookSignatureInvalid", err)
	}
	if len(repo.events) != 0 {
		t.Fatal("an invalid signature must not be persisted")
	}
}

func TestProcessWebhookIsIdempotentOnReplay(t *testing.T) {
	t.Parallel()

	repo := newFakeWebhookEventRepository()
	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier: &fakeWebhookVerifier{verifyResult: nil},
		Events:   repo,
		Clock:    &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	body := testCheckoutSessionBody("paid")
	cmd := application.ProcessWebhookCommand{
		RawBody:         body,
		SignatureHeader: "t=123,v1=valid",
		TimestampHeader: "123",
	}

	// First delivery: processes the event.
	if err := useCase.Execute(context.Background(), cmd); err != nil {
		t.Fatalf("first Execute error = %v", err)
	}
	if len(repo.events) != 1 {
		t.Fatalf("events = %d, want 1", len(repo.events))
	}

	// Second delivery: replay, no second insert.
	if err := useCase.Execute(context.Background(), cmd); err != nil {
		t.Fatalf("second Execute error = %v", err)
	}
	if len(repo.events) != 1 {
		t.Fatalf("events after replay = %d, want 1", len(repo.events))
	}

	// The event was processed exactly once.
	record := repo.events["evt_test123"]
	if record.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", record.Attempts)
	}
	if record.Status != application.WebhookEventProcessed {
		t.Errorf("status = %s, want processed", record.Status)
	}
}

func TestProcessWebhookHandlesCheckoutSessionCompleted(t *testing.T) {
	t.Parallel()

	repo := newFakeWebhookEventRepository()
	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier: &fakeWebhookVerifier{verifyResult: nil},
		Events:   repo,
		Clock:    &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	body := testCheckoutSessionBody("paid")
	if err := useCase.Execute(context.Background(), application.ProcessWebhookCommand{
		RawBody:         body,
		SignatureHeader: "t=123,v1=valid",
		TimestampHeader: "123",
	}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	record := repo.events["evt_test123"]
	if record == nil {
		t.Fatal("event not found")
	}
	if record.Status != application.WebhookEventProcessed {
		t.Errorf("status = %s, want processed", record.Status)
	}
	if record.EventType.String() != "checkout.session.completed" {
		t.Errorf("event type = %s", record.EventType)
	}
}

func TestProcessWebhookHandlesCheckoutSessionExpired(t *testing.T) {
	t.Parallel()

	repo := newFakeWebhookEventRepository()
	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier: &fakeWebhookVerifier{verifyResult: nil},
		Events:   repo,
		Clock:    &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	body := testCheckoutSessionExpiredBody()
	if err := useCase.Execute(context.Background(), application.ProcessWebhookCommand{
		RawBody:         body,
		SignatureHeader: "t=123,v1=valid",
		TimestampHeader: "123",
	}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	record := repo.events["evt_testexpired"]
	if record == nil {
		t.Fatal("event not found")
	}
	if record.Status != application.WebhookEventProcessed {
		t.Errorf("status = %s, want processed", record.Status)
	}
}

func TestProcessWebhookHandlesSubscriptionEvent(t *testing.T) {
	t.Parallel()

	repo := newFakeWebhookEventRepository()
	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier: &fakeWebhookVerifier{verifyResult: nil},
		Events:   repo,
		Clock:    &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	body := testSubscriptionBody()
	if err := useCase.Execute(context.Background(), application.ProcessWebhookCommand{
		RawBody:         body,
		SignatureHeader: "t=123,v1=valid",
		TimestampHeader: "123",
	}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	record := repo.events["evt_testsub"]
	if record == nil {
		t.Fatal("event not found")
	}
	if record.Status != application.WebhookEventProcessed {
		t.Errorf("status = %s, want processed", record.Status)
	}
}

func TestProcessWebhookIgnoresUnknownEventType(t *testing.T) {
	t.Parallel()

	repo := newFakeWebhookEventRepository()
	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier: &fakeWebhookVerifier{verifyResult: nil},
		Events:   repo,
		Clock:    &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	body := testUnknownEventBody()
	if err := useCase.Execute(context.Background(), application.ProcessWebhookCommand{
		RawBody:         body,
		SignatureHeader: "t=123,v1=valid",
		TimestampHeader: "123",
	}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	record := repo.events["evt_testunknown"]
	if record == nil {
		t.Fatal("event not found")
	}
	if record.Status != application.WebhookEventProcessed {
		t.Errorf("status = %s, want processed", record.Status)
	}
}

func TestProcessWebhookRejectsUnpaidCheckoutSession(t *testing.T) {
	t.Parallel()

	repo := newFakeWebhookEventRepository()
	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier: &fakeWebhookVerifier{verifyResult: nil},
		Events:   repo,
		Clock:    &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	// A checkout.session.completed with unpaid payment status is malformed.
	body := testCheckoutSessionBody("unpaid")
	err = useCase.Execute(context.Background(), application.ProcessWebhookCommand{
		RawBody:         body,
		SignatureHeader: "t=123,v1=valid",
		TimestampHeader: "123",
	})
	if err == nil {
		t.Fatal("an unpaid checkout session must be rejected")
	}
	// The event should be marked as failed.
	record := repo.events["evt_test123"]
	if record == nil {
		t.Fatal("event not found")
	}
	if record.Status != application.WebhookEventFailed {
		t.Errorf("status = %s, want failed", record.Status)
	}
}

func TestProcessWebhookRequiresCoherentConfiguration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*application.ProcessWebhookDependencies)
	}{
		{name: "no verifier", mutate: func(d *application.ProcessWebhookDependencies) { d.Verifier = nil }},
		{name: "no events", mutate: func(d *application.ProcessWebhookDependencies) { d.Events = nil }},
		{name: "no clock", mutate: func(d *application.ProcessWebhookDependencies) { d.Clock = nil }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			deps := application.ProcessWebhookDependencies{
				Verifier: &fakeWebhookVerifier{verifyResult: nil},
				Events:   newFakeWebhookEventRepository(),
				Clock:    &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
			}
			testCase.mutate(&deps)
			_, err := application.NewProcessWebhookUseCase(deps)
			if err == nil {
				t.Fatal("a refused configuration must not build a use case")
			}
		})
	}
}

func TestProcessWebhookSuccessPageNeverGrantsBenefit(t *testing.T) {
	t.Parallel()

	// This test proves the core invariant: a checkout.session.completed
	// event with an unpaid payment status is rejected. The success page of
	// a checkout session never grants benefit; only a verified webhook with
	// a settled payment status can settle an intent (THR-STRIPE-02).
	repo := newFakeWebhookEventRepository()
	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier: &fakeWebhookVerifier{verifyResult: nil},
		Events:   repo,
		Clock:    &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	// Unpaid checkout session completed: rejected.
	body := testCheckoutSessionBody("unpaid")
	err = useCase.Execute(context.Background(), application.ProcessWebhookCommand{
		RawBody:         body,
		SignatureHeader: "t=123,v1=valid",
		TimestampHeader: strconv.FormatInt(time.Now().Unix(), 10),
	})
	if err == nil {
		t.Fatal("an unpaid checkout.session.completed must be rejected")
	}

	// No payment was granted.
	record := repo.events["evt_test123"]
	if record == nil {
		t.Fatal("event not found")
	}
	if record.Status != application.WebhookEventFailed {
		t.Errorf("status = %s, want failed (no benefit granted)", record.Status)
	}
}

func TestProcessWebhookAppliesMemberEntitlements(t *testing.T) {
	t.Parallel()

	repo := newFakeWebhookEventRepository()
	catalog := testMemberCatalog(t)
	subs := newFakeSubscriptionRepository()
	customers := newFakeCustomerDirectory()
	customers.mapping["cus_user99"] = domain.AccountID("usr_account_99")
	passLots := &fakeMemberPassLots{}
	inker := &fakeMemberInker{}
	clock := &fakeClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)}

	memberSettler, err := application.NewApplyMemberEntitlementsUseCase(application.MemberEntitlementsDependencies{
		Catalog:       catalog,
		Subscriptions: subs,
		Customers:     customers,
		PassLots:      passLots,
		Inker:         inker,
		Clock:         clock,
	})
	if err != nil {
		t.Fatalf("NewApplyMemberEntitlementsUseCase: %v", err)
	}

	useCase, err := application.NewProcessWebhookUseCase(application.ProcessWebhookDependencies{
		Verifier:      &fakeWebhookVerifier{verifyResult: nil},
		Events:        repo,
		MemberSettler: memberSettler,
		Clock:         clock,
	})
	if err != nil {
		t.Fatalf("NewProcessWebhookUseCase error = %v", err)
	}

	subBody := []byte(`{
		"id": "evt_subcreated1",
		"type": "customer.subscription.created",
		"livemode": false,
		"created": 1789740000,
		"data": {
			"object": {
				"id": "sub_member99",
				"customer": "cus_user99",
				"status": "active",
				"current_period_start": 1789740000,
				"current_period_end": 1792418400,
				"cancel_at_period_end": false,
				"items": {
					"data": [
						{
							"id": "si_123",
							"price": {
								"id": "price_1QbrMember"
							}
						}
					]
				}
			}
		}
	}`)

	if err := useCase.Execute(context.Background(), application.ProcessWebhookCommand{
		RawBody:         subBody,
		SignatureHeader: "t=123,v1=valid",
		TimestampHeader: strconv.FormatInt(time.Now().Unix(), 10),
	}); err != nil {
		t.Fatalf("Execute subscription webhook error = %v", err)
	}

	eventRec := repo.events["evt_subcreated1"]
	if eventRec == nil || eventRec.Status != application.WebhookEventProcessed {
		t.Fatalf("expected event processed, got %+v", eventRec)
	}

	// Verify subscription recorded
	subRec := subs.subs["sub_member99"]
	if subRec == nil {
		t.Fatal("expected subscription recorded in repository")
	}
	if subRec.Status != domain.SubscriptionActive {
		t.Errorf("subscription status = %v, want active", subRec.Status)
	}

	// Verify pass granted
	if len(passLots.grants) != 1 {
		t.Fatalf("expected 1 pass granted, got %d", len(passLots.grants))
	}
	if passLots.grants[0].Origin != domain.OriginMember || passLots.grants[0].Quantity.Int32() != 1 {
		t.Errorf("pass grant = %+v, want 1 member pass", passLots.grants[0])
	}

	// Verify INK granted: exactly 30k
	if len(inker.credits) != 1 {
		t.Fatalf("expected 1 ink credit, got %d", len(inker.credits))
	}
	if inker.credits[0].Amount != 30000 {
		t.Errorf("ink credit amount = %d, want 30000", inker.credits[0].Amount)
	}
}
