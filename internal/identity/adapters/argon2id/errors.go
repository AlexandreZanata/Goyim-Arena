package argon2id

import "errors"

var (
	// ErrEmptyPassword indicates that the password to hash or verify is empty.
	ErrEmptyPassword = errors.New("argon2id: password cannot be empty")

	// ErrInvalidHash indicates that the encoded hash string is malformed or unrecognized.
	ErrInvalidHash = errors.New("argon2id: encoded hash format is invalid")

	// ErrIncompatibleVersion indicates that the hash version is not supported.
	ErrIncompatibleVersion = errors.New("argon2id: incompatible argon2 version")

	// ErrInvalidParams indicates that the provided cost or sizing parameters violate constraints.
	ErrInvalidParams = errors.New("argon2id: invalid parameters")

	// ErrEntropyFailed indicates that the random reader failed to provide cryptographic bytes.
	ErrEntropyFailed = errors.New("argon2id: failed to read cryptographic entropy")
)
