package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule violations.
type ErrorCode string

const (
	CodeInvalidPeriod ErrorCode = "TRANSPARENCY_INVALID_PERIOD"
)

// DomainError represents an invariant or rule failure in the transparency domain.
type DomainError struct {
	Code    ErrorCode
	Message string
}

func (e DomainError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e DomainError) Is(target error) bool {
	t, ok := target.(DomainError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

var (
	ErrInvalidPeriod = DomainError{Code: CodeInvalidPeriod, Message: "metrics period must be a complete interval with the end after the start"}
)
