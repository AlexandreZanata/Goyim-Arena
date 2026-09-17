// Package application defines the use cases, orchestrations, and consumer-oriented
// ports for the identity and authentication module.
package application

// PasswordHasher abstracts secure password hashing, constant-time verification,
// rehash detection for evolving security parameters, and dummy hashes for
// uniform login timing (mitigating user enumeration timing attacks per THR-AUTH-02).
type PasswordHasher interface {
	// HashPassword derives a secure Argon2id hash from a plain-text password using
	// a fresh cryptographically random salt and configured cost parameters.
	HashPassword(password string) (string, error)

	// VerifyPassword checks a plain-text password against an encoded Argon2id hash
	// using constant-time comparison to prevent timing side-channels.
	VerifyPassword(password, encodedHash string) (bool, error)

	// NeedsRehash reports whether an encoded hash was produced with parameters weaker
	// or different from the current hasher configuration, indicating the credential
	// should be re-hashed upon successful authentication.
	NeedsRehash(encodedHash string) bool

	// DummyHash returns a pre-computed, validly formatted Argon2id hash matching
	// current parameters to be used when an account does not exist, ensuring
	// identical CPU and memory consumption to prevent user enumeration (THR-AUTH-02).
	DummyHash() string
}
