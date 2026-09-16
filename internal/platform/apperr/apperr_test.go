package apperr

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewCarriesKindCodeAndDetail(t *testing.T) {
	err := New(KindConflict, "ARENA-USERNAME-TAKEN", "username already taken")

	if err.Kind() != KindConflict || err.Code() != "ARENA-USERNAME-TAKEN" || err.Detail() != "username already taken" {
		t.Fatalf("unexpected error: kind=%s code=%s detail=%q", err.Kind(), err.Code(), err.Detail())
	}
	if err.IsRetryable() {
		t.Fatal("errors are not retryable by default")
	}
}

func TestNewfFormatsPublicDetail(t *testing.T) {
	err := Newf(KindValidation, "ARENA-FIELD", "field %q has %d chars", "username", 3)
	if err.Detail() != `field "username" has 3 chars` {
		t.Fatalf("detail = %q", err.Detail())
	}
}

func TestWithCauseKeepsUnwrapAndSafeString(t *testing.T) {
	cause := fmt.Errorf("row lock timeout")
	err := New(KindInternal, "ARENA-DB", "database unavailable").WithCause(cause)

	if !errors.Is(err, cause) {
		t.Fatal("WithCause must support errors.Is unwrapping")
	}

	// Error() renders code + public detail only; the cause must stay out of
	// the default rendering so accidental logging stays safe.
	if strings.Contains(err.Error(), "row lock timeout") {
		t.Fatalf("Error() leaked the internal cause: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "ARENA-DB") {
		t.Fatalf("Error() must carry the stable code: %q", err.Error())
	}
}

func TestMarkRetryable(t *testing.T) {
	err := New(KindRateLimited, "ARENA-RL", "slow down").MarkRetryable()
	if !err.IsRetryable() {
		t.Fatal("MarkRetryable must set the retryable flag")
	}
}

func TestKindOfDefaultsToInternalForForeignErrors(t *testing.T) {
	if got := KindOf(errors.New("boom")); got != KindInternal {
		t.Fatalf("foreign error kind = %s, want internal", got)
	}

	wrapped := fmt.Errorf("handling: %w", New(KindNotFound, "ARENA-404", "arena not found"))
	if got := KindOf(wrapped); got != KindNotFound {
		t.Fatalf("wrapped vocabulary kind = %s, want not_found", got)
	}
}
