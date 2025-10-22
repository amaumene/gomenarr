package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors
var (
	ErrNotFound          = errors.New("not found")
	ErrDuplicate         = errors.New("duplicate entry")
	ErrInvalidInput      = errors.New("invalid input")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrConflict          = errors.New("conflict")
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrTimeout           = errors.New("timeout")
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field %s: %s", e.Field, e.Message)
}

// NewValidationError creates a new validation error
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// ExternalError represents an error from an external service
type ExternalError struct {
	Service string
	Err     error
}

func (e *ExternalError) Error() string {
	return fmt.Sprintf("external service error (%s): %v", e.Service, e.Err)
}

func (e *ExternalError) Unwrap() error {
	return e.Err
}

// NewExternalError creates a new external error
func NewExternalError(service string, err error) *ExternalError {
	return &ExternalError{
		Service: service,
		Err:     err,
	}
}
