package domain

import "time"

// OperationID uniquely identifies a logical ledger operation.
type OperationID string

// String returns the string representation of the operation identifier.
func (id OperationID) String() string {
	return string(id)
}

// IsZero reports whether the OperationID is uninitialized.
func (id OperationID) IsZero() bool {
	return id == ""
}

// Operation is the immutable, reconstituted record of a logical INK
// operation: its type, idempotency key, cause reference and creation
// instant. It is the entity returned when a retry replays an existing key.
type Operation struct {
	id             OperationID
	accountID      AccountID
	operationType  OperationType
	idempotencyKey IdempotencyKey
	reference      Reference
	createdAt      time.Time
}

// ReconstituteOperation rebuilds an Operation from persistent state,
// validating the same invariants without re-running creation logic.
func ReconstituteOperation(
	id OperationID,
	accountID AccountID,
	operationType OperationType,
	idempotencyKey IdempotencyKey,
	reference Reference,
	createdAt time.Time,
) (*Operation, error) {
	if id.IsZero() {
		return nil, ErrEmptyOperationID
	}
	if accountID.IsZero() {
		return nil, ErrEmptyAccountID
	}
	if !operationType.IsValid() {
		return nil, ErrInvalidOperationType
	}
	if idempotencyKey.IsZero() {
		return nil, ErrEmptyIdempotencyKey
	}
	if reference.IsZero() {
		return nil, ErrEmptyReference
	}

	return &Operation{
		id:             id,
		accountID:      accountID,
		operationType:  operationType,
		idempotencyKey: idempotencyKey,
		reference:      reference,
		createdAt:      createdAt,
	}, nil
}

// ID returns the operation identifier.
func (o *Operation) ID() OperationID {
	return o.id
}

// AccountID returns the wallet owner.
func (o *Operation) AccountID() AccountID {
	return o.accountID
}

// Type returns the operation type.
func (o *Operation) Type() OperationType {
	return o.operationType
}

// IdempotencyKey returns the retry key of the operation.
func (o *Operation) IdempotencyKey() IdempotencyKey {
	return o.idempotencyKey
}

// Reference returns the stable cause reference of the operation.
func (o *Operation) Reference() Reference {
	return o.reference
}

// CreatedAt returns the operation creation instant.
func (o *Operation) CreatedAt() time.Time {
	return o.createdAt
}

// IsCredit reports whether the operation credits a bucket.
func (o *Operation) IsCredit() bool {
	return o.operationType.IsCredit()
}

// IsDebit reports whether the operation debits a bucket.
func (o *Operation) IsDebit() bool {
	return o.operationType.IsDebit()
}
