package domain

import (
	"bytes"
	"testing"
	"time"
)

func TestNewSessionSuccess(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	policy := DefaultSessionPolicy() // 24h idle, 14d absolute
	tokenHash := []byte("01234567890123456789012345678901")

	sess, err := NewSession(
		SessionID("sess-123"),
		AccountID("acc-456"),
		tokenHash,
		"192.0.2.1",
		"Mozilla/5.0 TestBrowser",
		now,
		policy,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sess.ID() != "sess-123" {
		t.Errorf("expected ID sess-123, got %s", sess.ID())
	}
	if sess.AccountID() != "acc-456" {
		t.Errorf("expected AccountID acc-456, got %s", sess.AccountID())
	}
	if !sess.HasTokenHash(tokenHash) {
		t.Errorf("token hash mismatch")
	}
	if sess.CreatedAt() != now {
		t.Errorf("expected createdAt %v, got %v", now, sess.CreatedAt())
	}
	if sess.LastSeenAt() != now {
		t.Errorf("expected lastSeenAt %v, got %v", now, sess.LastSeenAt())
	}
	expectedExpiry := now.Add(24 * time.Hour)
	if sess.ExpiresAt() != expectedExpiry {
		t.Errorf("expected expiresAt %v, got %v", expectedExpiry, sess.ExpiresAt())
	}
	if sess.IsRevoked() {
		t.Errorf("new session should not be revoked")
	}
	if sess.IPAddress() != "192.0.2.1" {
		t.Errorf("expected IP 192.0.2.1, got %s", sess.IPAddress())
	}
	if sess.UserAgent() != "Mozilla/5.0 TestBrowser" {
		t.Errorf("expected user agent Mozilla/5.0 TestBrowser, got %s", sess.UserAgent())
	}
}

func TestNewSessionValidation(t *testing.T) {
	now := time.Now()
	policy := DefaultSessionPolicy()
	validHash := []byte("01234567890123456789012345678901")

	tests := []struct {
		name      string
		id        SessionID
		accountID AccountID
		hash      []byte
		wantErr   error
	}{
		{
			name:      "empty session id",
			id:        "",
			accountID: "acc-1",
			hash:      validHash,
			wantErr:   ErrEmptySessionID,
		},
		{
			name:      "whitespace session id",
			id:        "   ",
			accountID: "acc-1",
			hash:      validHash,
			wantErr:   ErrEmptySessionID,
		},
		{
			name:      "empty account id",
			id:        "sess-1",
			accountID: "",
			hash:      validHash,
			wantErr:   ErrEmptyAccountID,
		},
		{
			name:      "empty token hash",
			id:        "sess-1",
			accountID: "acc-1",
			hash:      nil,
			wantErr:   ErrEmptySessionToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSession(tt.id, tt.accountID, tt.hash, "", "", now, policy)
			if err == nil {
				t.Fatalf("expected error %v, got nil", tt.wantErr)
			}
			if err != tt.wantErr {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestSessionRevocation(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	policy := DefaultSessionPolicy()
	sess, err := NewSession("sess-1", "acc-1", []byte("12345678901234567890123456789012"), "", "", now, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sess.IsRevoked() {
		t.Fatalf("expected session not revoked")
	}

	revokedAt := now.Add(10 * time.Minute)
	if err := sess.Revoke(revokedAt); err != nil {
		t.Fatalf("failed to revoke session: %v", err)
	}

	if !sess.IsRevoked() {
		t.Fatalf("expected session to be revoked")
	}
	if sess.RevokedAt() == nil || *sess.RevokedAt() != revokedAt {
		t.Fatalf("expected revokedAt %v, got %v", revokedAt, sess.RevokedAt())
	}

	// Re-revoking returns error
	if err := sess.Revoke(revokedAt.Add(time.Minute)); err != ErrSessionRevoked {
		t.Fatalf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestSessionExpirationRules(t *testing.T) {
	createdAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	policy := SessionPolicy{
		IdleTimeout:      24 * time.Hour,
		AbsoluteLifetime: 14 * 24 * time.Hour,
	}

	sess, err := NewSession("sess-1", "acc-1", []byte("12345678901234567890123456789012"), "", "", createdAt, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Active after 1 hour
	if sess.IsExpired(createdAt.Add(1*time.Hour), policy) {
		t.Errorf("session should be active after 1 hour")
	}

	// Inactive after 24h + 1s without touch
	if !sess.IsExpired(createdAt.Add(24*time.Hour+time.Second), policy) {
		t.Errorf("session should be expired after idle timeout")
	}

	// Touch at day 13
	day13 := createdAt.Add(13 * 24 * time.Hour)
	sess.Touch(day13, policy)
	if sess.IsExpired(day13.Add(1*time.Hour), policy) {
		t.Errorf("session should be active 1 hour after touch at day 13")
	}

	// Absolute lifetime expires at day 14 + 1s, even if touched at day 13
	day14Past := createdAt.Add(14*24*time.Hour + time.Second)
	if !sess.IsExpired(day14Past, policy) {
		t.Errorf("session should be expired past absolute lifetime")
	}
}

func TestSessionShouldTouch(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	policy := DefaultSessionPolicy()
	sess, err := NewSession("sess-1", "acc-1", []byte("12345678901234567890123456789012"), "", "", now, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	threshold := 5 * time.Minute

	// Immediately after creation
	if sess.ShouldTouch(now, threshold) {
		t.Errorf("should not touch immediately after creation")
	}

	// 2 minutes later
	if sess.ShouldTouch(now.Add(2*time.Minute), threshold) {
		t.Errorf("should not touch before threshold")
	}

	// 5 minutes later
	if !sess.ShouldTouch(now.Add(5*time.Minute), threshold) {
		t.Errorf("should touch at threshold")
	}

	// 10 minutes later
	if !sess.ShouldTouch(now.Add(10*time.Minute), threshold) {
		t.Errorf("should touch after threshold")
	}

	// Revoked session never touches
	_ = sess.Revoke(now)
	if sess.ShouldTouch(now.Add(10*time.Minute), threshold) {
		t.Errorf("revoked session should not touch")
	}
}

func TestSessionDefensiveCopies(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	policy := DefaultSessionPolicy()
	originalHash := []byte{1, 2, 3, 4}
	sess, err := NewSession("sess-1", "acc-1", originalHash, "", "", now, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mutating original slice does not affect session
	originalHash[0] = 99
	if bytes.Equal(sess.TokenHash(), originalHash) {
		t.Errorf("session token hash was mutated via original slice")
	}

	// Mutating returned slice does not affect session
	returned := sess.TokenHash()
	returned[0] = 88
	if bytes.Equal(sess.TokenHash(), returned) {
		t.Errorf("session token hash was mutated via getter slice")
	}
}
