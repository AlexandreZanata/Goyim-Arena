package stripe

import "errors"

// Construction errors of the adapter. They describe a gateway that could not
// be built at all, which is a boot-time configuration problem: the composition
// root fails fast instead of serving checkout requests with a half-configured
// provider.
var (
	// ErrMissingSecretKey indicates no provider credential was configured.
	// Without it there is nothing to authenticate with, and the adapter
	// refuses to build rather than discovering the problem on the first
	// purchase.
	ErrMissingSecretKey = errors.New("stripe: a secret key is required")

	// ErrInvalidTimeout indicates the configured timeout is not positive: an
	// unbounded provider call could hold a server goroutine open forever.
	ErrInvalidTimeout = errors.New("stripe: the timeout must be positive")

	// ErrInvalidBaseURL indicates the configured endpoint is not an absolute
	// HTTP(S) URL.
	ErrInvalidBaseURL = errors.New("stripe: the base URL must be an absolute HTTP(S) URL")
)
