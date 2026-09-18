package application

import "errors"

var (
	// ErrInvalidMetricsConfig indicates the derive use case could not be
	// built from the given dependencies.
	ErrInvalidMetricsConfig = errors.New("application: transparency metrics configuration is invalid")
)
