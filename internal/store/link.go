// Package store defines the persistence contract for short links and its
// SQLite implementation.
package store

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by every Repository implementation.
var (
	// ErrNotFound means no link exists for the requested code.
	ErrNotFound = errors.New("link not found")
	// ErrConflict means the code is already taken.
	ErrConflict = errors.New("link code already exists")
	// ErrStorage means the underlying storage failed.
	ErrStorage = errors.New("storage unavailable")
)

// Link is a persisted short link.
type Link struct {
	Code         string
	TargetURL    string
	Enabled      bool
	ExpiresAt    *time.Time
	Hits         int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastAccessAt *time.Time
}

// Expired reports whether the link is past its expiry at the given instant.
func (l Link) Expired(at time.Time) bool {
	return l.ExpiresAt != nil && !at.Before(*l.ExpiresAt)
}

// Patch describes a partial update. Only non-nil fields are applied.
// ClearExpires removes an existing expiry and takes precedence over ExpiresAt.
type Patch struct {
	TargetURL    *string
	ExpiresAt    *time.Time
	ClearExpires bool
	Enabled      *bool
	UpdatedAt    time.Time
}

// ListOptions controls pagination and filtering for List.
type ListOptions struct {
	Page     int
	PageSize int
	Query    string
}

// ListResult is a page of links plus the total count.
type ListResult struct {
	Items []Link
	Total int
}

// Repository abstracts link persistence so the service layer stays storage
// agnostic.
type Repository interface {
	Create(ctx context.Context, link Link) error
	GetByCode(ctx context.Context, code string) (Link, error)
	List(ctx context.Context, opts ListOptions) (ListResult, error)
	Update(ctx context.Context, code string, patch Patch) (Link, error)
	Delete(ctx context.Context, code string) error
	IncrementHits(ctx context.Context, code string, at time.Time) error
	Ping(ctx context.Context) error
}
