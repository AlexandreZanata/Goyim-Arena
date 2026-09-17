package domain

import "strings"

// Category is an immutable, validated reference to an editorial Arena
// category. Existence is enforced by the database foreign key; the value
// object validates the stable slug format only.
type Category struct {
	slug string
}

// ParseCategory validates and constructs a Category from its slug.
func ParseCategory(raw string) (Category, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Category{}, ErrEmptyCategory
	}
	if len(trimmed) < CategoryMinLength || len(trimmed) > CategoryMaxLength {
		return Category{}, ErrInvalidCategory
	}

	for i := 0; i < len(trimmed); i++ {
		b := trimmed[i]
		switch {
		case (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9'):
			// allowed
		case b == '-':
			if i == 0 || i == len(trimmed)-1 {
				return Category{}, ErrInvalidCategory
			}
		default:
			return Category{}, ErrInvalidCategory
		}
	}

	return Category{slug: trimmed}, nil
}

// String returns the category slug.
func (c Category) String() string {
	return c.slug
}

// IsZero reports whether the Category is the uninitialized zero value.
func (c Category) IsZero() bool {
	return c.slug == ""
}

// Equals reports whether two categories are identical.
func (c Category) Equals(other Category) bool {
	return c.slug == other.slug
}
