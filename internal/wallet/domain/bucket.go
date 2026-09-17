package domain

// Bucket is the INK bucket of a ledger transaction. Buckets follow the
// product rule of docs/MONETIZATION.md §2.2: the plan franchise (FREE_INK)
// is consumed before any purchased INK (PURCHASED_INK).
type Bucket string

const (
	// BucketFree is the plan franchise; it does not accumulate between
	// periods and is consumed first.
	BucketFree Bucket = "FREE_INK"

	// BucketPurchased is bought INK; it does not expire initially and is
	// consumed only after the franchise.
	BucketPurchased Bucket = "PURCHASED_INK"
)

// ParseBucket validates a persisted or transport bucket value against the
// exact vocabulary enforced by the ledger CHECK constraint.
func ParseBucket(raw string) (Bucket, error) {
	bucket := Bucket(raw)
	if !bucket.IsValid() {
		return "", ErrInvalidBucket
	}
	return bucket, nil
}

// IsValid reports whether the bucket is an authorized enum value.
func (b Bucket) IsValid() bool {
	switch b {
	case BucketFree, BucketPurchased:
		return true
	default:
		return false
	}
}

// String returns the stored bucket value.
func (b Bucket) String() string {
	return string(b)
}

// Priority fixes the mandatory consumption order. Invalid buckets report 0
// so they can never be ordered as if they were spendable.
func (b Bucket) Priority() int {
	switch b {
	case BucketFree:
		return 1
	case BucketPurchased:
		return 2
	default:
		return 0
	}
}
