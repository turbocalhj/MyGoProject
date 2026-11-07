// Package shortener contains the domain logic for creating, resolving and
// managing short links.
package shortener

import (
	"errors"
	"fmt"

	"github.com/turbocalhj/MyGoProject/internal/store"
)

// Sentinel errors surfaced by the service layer.
var (
	// ErrNotFound is returned when a code does not exist.
	ErrNotFound = store.ErrNotFound
	// ErrConflict is returned when a custom alias is already taken.
	ErrConflict = store.ErrConflict
	// ErrStorage is returned when persistence fails.
	ErrStorage = store.ErrStorage
	// ErrExpired is returned when a link is past its expiry.
	ErrExpired = errors.New("link expired")
	// ErrDisabled is returned when a link has been disabled.
	ErrDisabled = errors.New("link disabled")
)

// Error codes exposed to API clients.
const (
	CodeInvalidURL        = "invalid_url"
	CodeInvalidAlias      = "invalid_alias"
	CodeInvalidExpires    = "invalid_expires_at"
	CodeInvalidPagination = "invalid_pagination"
)

// ValidationError marks a client-side input problem.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func invalid(code, message string) *ValidationError {
	return &ValidationError{Code: code, Message: message}
}
