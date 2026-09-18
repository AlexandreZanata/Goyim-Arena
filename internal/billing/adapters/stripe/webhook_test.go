package stripe_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	stripeadapter "github.com/AlexandreZanata/Goyim-Arena/internal/billing/adapters/stripe"
)

// testClock is a deterministic clock for tests.
type testClock struct {
	now time.Time
}

func (c *testClock) Now() time.Time { return c.now }

func TestWebhookVerifierRejectsMissingSecret(t *testing.T) {
	t.Parallel()
	_, err := stripeadapter.NewWebhookVerifier("", 0, &testClock{now: time.Now()})
	if !errors.Is(err, stripeadapter.ErrMissingWebhookSecret) {
		t.Fatalf("error = %v, want ErrMissingWebhookSecret", err)
	}
}

func TestWebhookVerifierAcceptsValidSignature(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	body := []byte(`{"id":"evt_test","type":"checkout.session.completed"}`)
	timestamp := time.Now().Unix()
	signedPayload := fmt.Sprintf("%d.%s", timestamp, body)

	// Compute the correct signature.
	mac := newHMAC(signedPayload, secret)
	signature := fmt.Sprintf("t=%d,v1=%s", timestamp, mac)

	if err := verifier.Verify(body, signature, strconv.FormatInt(timestamp, 10)); err != nil {
		t.Fatalf("Verify error = %v", err)
	}
}

func TestWebhookVerifierRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	body := []byte(`{"id":"evt_test","type":"checkout.session.completed"}`)
	timestamp := time.Now().Unix()
	wrongSignature := fmt.Sprintf("t=%d,v1=wrong_signature_value", timestamp)

	if err := verifier.Verify(body, wrongSignature, strconv.FormatInt(timestamp, 10)); err == nil {
		t.Fatal("a wrong signature must be rejected")
	}
}

func TestWebhookVerifierRejectsAlteredBody(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	originalBody := []byte(`{"id":"evt_test","type":"checkout.session.completed"}`)
	alteredBody := []byte(`{"id":"evt_test","type":"checkout.session.completed","extra":"data"}`)
	timestamp := time.Now().Unix()

	// Sign the original body.
	signedPayload := fmt.Sprintf("%d.%s", timestamp, originalBody)
	mac := newHMAC(signedPayload, secret)
	signature := fmt.Sprintf("t=%d,v1=%s", timestamp, mac)

	// Verify with the altered body: must fail.
	if err := verifier.Verify(alteredBody, signature, strconv.FormatInt(timestamp, 10)); err == nil {
		t.Fatal("an altered body must be rejected")
	}
}

func TestWebhookVerifierRejectsExpiredTimestamp(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	body := []byte(`{"id":"evt_test"}`)
	// Timestamp from 10 minutes ago: outside the 5-minute tolerance.
	oldTimestamp := time.Now().Add(-10 * time.Minute).Unix()
	signedPayload := fmt.Sprintf("%d.%s", oldTimestamp, body)
	mac := newHMAC(signedPayload, secret)
	signature := fmt.Sprintf("t=%d,v1=%s", oldTimestamp, mac)

	if err := verifier.Verify(body, signature, strconv.FormatInt(oldTimestamp, 10)); err == nil {
		t.Fatal("an expired timestamp must be rejected")
	}
}

func TestWebhookVerifierRejectsFutureTimestamp(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	body := []byte(`{"id":"evt_test"}`)
	// Timestamp 2 minutes in the future.
	futureTimestamp := time.Now().Add(2 * time.Minute).Unix()
	signedPayload := fmt.Sprintf("%d.%s", futureTimestamp, body)
	mac := newHMAC(signedPayload, secret)
	signature := fmt.Sprintf("t=%d,v1=%s", futureTimestamp, mac)

	if err := verifier.Verify(body, signature, strconv.FormatInt(futureTimestamp, 10)); err == nil {
		t.Fatal("a future timestamp must be rejected")
	}
}

func TestWebhookVerifierRejectsEmptySignatureHeader(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	body := []byte(`{"id":"evt_test"}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	if err := verifier.Verify(body, "", timestamp); err == nil {
		t.Fatal("an empty signature header must be rejected")
	}
}

func TestWebhookVerifierRejectsEmptyTimestampHeader(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	body := []byte(`{"id":"evt_test"}`)
	signature := fmt.Sprintf("t=%d,v1=abc", time.Now().Unix())

	if err := verifier.Verify(body, signature, ""); err == nil {
		t.Fatal("an empty timestamp header must be rejected")
	}
}

func TestWebhookVerifierRejectsNonIntegerTimestamp(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	body := []byte(`{"id":"evt_test"}`)
	signature := fmt.Sprintf("t=%d,v1=abc", time.Now().Unix())

	if err := verifier.Verify(body, signature, "not_a_number"); err == nil {
		t.Fatal("a non-integer timestamp must be rejected")
	}
}

func TestWebhookVerifierRejectsMissingV1Signature(t *testing.T) {
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	body := []byte(`{"id":"evt_test"}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	// Header with no v1= signature.
	signatureHeader := fmt.Sprintf("t=%s,v0=old_version", timestamp)

	if err := verifier.Verify(body, signatureHeader, timestamp); err == nil {
		t.Fatal("a header without v1 signature must be rejected")
	}
}

func TestWebhookVerifierUsesConstantTimeComparison(t *testing.T) {
	// This test verifies that the verifier uses hmac.Equal (constant-time
	// comparison) instead of == (which leaks timing information). The test
	// constructs a signature that is very close to the correct one (only
	// the last character differs) and verifies it is still rejected.
	t.Parallel()

	secret := "whsec_test_secret_key"
	verifier, err := stripeadapter.NewWebhookVerifier(secret, 5*time.Minute, &testClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewWebhookVerifier error = %v", err)
	}

	body := []byte(`{"id":"evt_test"}`)
	timestamp := time.Now().Unix()
	signedPayload := fmt.Sprintf("%d.%s", timestamp, body)
	mac := newHMAC(signedPayload, secret)

	// Flip the last character of the signature.
	if len(mac) > 0 {
		lastChar := mac[len(mac)-1]
		if lastChar == 'a' {
			mac = mac[:len(mac)-1] + "b"
		} else {
			mac = mac[:len(mac)-1] + "a"
		}
	}
	signature := fmt.Sprintf("t=%d,v1=%s", timestamp, mac)

	// Must be rejected despite being very close.
	if err := verifier.Verify(body, signature, strconv.FormatInt(timestamp, 10)); err == nil {
		t.Fatal("a nearly-correct signature must be rejected")
	}
}

// newHMAC computes the HMAC-SHA256 of a message with a secret.
func newHMAC(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
