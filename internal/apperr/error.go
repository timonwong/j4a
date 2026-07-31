package apperr

import (
	"errors"
	"fmt"
)

// Kind is a stable, machine-readable error category.
type Kind string

const (
	KindUnexpected     Kind = "unexpected"
	KindInvalidInput   Kind = "invalid_input"
	KindConfig         Kind = "config"
	KindAuth           Kind = "auth"
	KindNotFound       Kind = "not_found"
	KindAPI            Kind = "api_error"
	KindRateLimit      Kind = "rate_limit"
	KindPartialFailure Kind = "partial_failure"
)

// Error carries a stable error category while preserving the underlying cause.
type Error struct {
	Kind       Kind
	Message    string
	StatusCode int
	Cause      error
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error { return e.Cause }

// New constructs a categorized application error.
func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

// Wrap categorizes an existing error with additional context.
func Wrap(kind Kind, err error, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...), Cause: err}
}

// As returns an application error, mapping unknown errors to KindUnexpected.
func As(err error) *Error {
	if err == nil {
		return nil
	}
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	return &Error{Kind: KindUnexpected, Message: err.Error(), Cause: err}
}

// ExitCode maps stable error categories to process exit codes.
func ExitCode(err error) int {
	switch As(err).Kind {
	case KindInvalidInput, KindConfig:
		return 2
	case KindAuth:
		return 3
	case KindNotFound:
		return 4
	case KindAPI:
		return 5
	case KindRateLimit:
		return 6
	case KindPartialFailure:
		return 7
	default:
		return 1
	}
}
