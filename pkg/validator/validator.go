package validator

import (
	"errors"
	"strings"
)

var (
	ErrEmptyField    = errors.New("field cannot be empty")
	ErrInvalidFormat = errors.New("invalid format")
)

// ValidateNonEmpty validates that a string is not empty
func ValidateNonEmpty(value, fieldName string) error {
	if strings.TrimSpace(value) == "" {
		return ErrEmptyField
	}
	return nil
}

// ValidatePositive validates that a number is positive
func ValidatePositive(value int64, fieldName string) error {
	if value <= 0 {
		return ErrInvalidFormat
	}
	return nil
}
