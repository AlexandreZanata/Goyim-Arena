package argon2id

import (
	"fmt"

	"golang.org/x/crypto/argon2"
)

// CurrentVersion represents the Argon2 version implemented by the runtime (0x13 = 19).
const CurrentVersion = argon2.Version

// AlgorithmName identifies the Argon2id variant in the standard PHC format.
const AlgorithmName = "argon2id"

// Params defines the cryptographic cost and sizing parameters for Argon2id hashing.
type Params struct {
	// Memory cost in KiB.
	Memory uint32
	// Iterations (time cost / passes over memory).
	Iterations uint32
	// Parallelism (threads / lanes).
	Parallelism uint8
	// SaltLength in bytes.
	SaltLength uint32
	// KeyLength in bytes.
	KeyLength uint32
}

// DefaultParams returns OWASP-recommended production parameters for Argon2id:
// 64 MiB memory, 3 iterations, 2 threads, 16-byte salt, 32-byte key.
func DefaultParams() Params {
	return Params{
		Memory:      64 * 1024,
		Iterations:  3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// FastParams returns lighter parameters calibrated for fast, non-blocking unit tests:
// 8 MiB memory, 1 iteration, 1 thread, 16-byte salt, 32-byte key.
func FastParams() Params {
	return Params{
		Memory:      8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// Validate checks that the parameters satisfy Argon2id mathematical and security constraints.
func (p Params) Validate() error {
	if p.Parallelism < 1 {
		return fmt.Errorf("%w: parallelism must be at least 1, got %d", ErrInvalidParams, p.Parallelism)
	}
	if p.Iterations < 1 {
		return fmt.Errorf("%w: iterations must be at least 1, got %d", ErrInvalidParams, p.Iterations)
	}
	minMemory := uint32(p.Parallelism) * 8
	if p.Memory < minMemory {
		return fmt.Errorf("%w: memory must be at least %d KiB for parallelism %d, got %d", ErrInvalidParams, minMemory, p.Parallelism, p.Memory)
	}
	if p.SaltLength < 16 {
		return fmt.Errorf("%w: salt length must be at least 16 bytes, got %d", ErrInvalidParams, p.SaltLength)
	}
	if p.KeyLength < 16 {
		return fmt.Errorf("%w: key length must be at least 16 bytes, got %d", ErrInvalidParams, p.KeyLength)
	}
	return nil
}
