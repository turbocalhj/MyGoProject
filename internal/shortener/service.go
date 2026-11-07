package shortener

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/turbocalhj/MyGoProject/internal/store"
)

const (
	maxURLLength   = 2048
	maxCodeRetries = 5
	maxPageSize    = 100
)

// ErrCodeExhausted indicates repeated short code collisions.
var ErrCodeExhausted = errors.New("short code generation exhausted")

var aliasPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

// Options configures a Service. All fields are optional.
type Options struct {
	// BaseURL is the public origin used to build short_url values.
	BaseURL string
	// Now overrides the clock, mainly for tests.
	Now func() time.Time
	// Generate overrides the code generator, mainly for tests.
	Generate func() (string, error)
}

// Service implements the short link use cases on top of a Repository.
type Service struct {
	repo     store.Repository
	baseURL  string
	now      func() time.Time
	generate func() (string, error)
}

// NewService builds a Service.
func NewService(repo store.Repository, opts Options) *Service {
	s := &Service{
		repo:     repo,
		baseURL:  strings.TrimRight(opts.BaseURL, "/"),
		now:      opts.Now,
		generate: opts.Generate,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.generate == nil {
		s.generate = GenerateCode
	}
	return s
}

// CreateInput carries the fields accepted by Create.
type CreateInput struct {
	TargetURL string
	Alias     string
	ExpiresAt *time.Time
}

// UpdateInput carries the optional fields accepted by Update.
type UpdateInput struct {
	TargetURL    *string
	ExpiresAt    *time.Time
	ClearExpires bool
	Enabled      *bool
}

// Create validates the input and persists a new link. When Alias is empty a
// random code is generated, retrying on collision up to maxCodeRetries times.
func (s *Service) Create(ctx context.Context, in CreateInput) (store.Link, error) {
	if err := ValidateURL(in.TargetURL); err != nil {
		return store.Link{}, err
	}
	now := s.now().UTC()
	if in.ExpiresAt != nil && !in.ExpiresAt.After(now) {
		return store.Link{}, invalid(CodeInvalidExpires, "expires_at must be in the future")
	}

	custom := in.Alias != ""
	if custom {
		if err := ValidateAlias(in.Alias); err != nil {
			return store.Link{}, err
		}
	}

	code := in.Alias
	for attempt := 1; attempt <= maxCodeRetries; attempt++ {
		if !custom {
			generated, err := s.generate()
			if err != nil {
				return store.Link{}, fmt.Errorf("%w: %v", ErrStorage, err)
			}
			code = generated
		}

		link := store.Link{
			Code:      code,
			TargetURL: strings.TrimSpace(in.TargetURL),
			Enabled:   true,
			ExpiresAt: utcOrNil(in.ExpiresAt),
			CreatedAt: now,
			UpdatedAt: now,
		}
		err := s.repo.Create(ctx, link)
		switch {
		case err == nil:
			return link, nil
		case errors.Is(err, store.ErrConflict):
			if custom {
				return store.Link{}, ErrConflict
			}
		default:
			return store.Link{}, err
		}
	}
	return store.Link{}, ErrCodeExhausted
}

// Resolve looks up a code and enforces the enabled/expiry rules. It records an
// access only when the link is usable; analytics failures never block the
// redirect.
func (s *Service) Resolve(ctx context.Context, code string) (store.Link, error) {
	link, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return store.Link{}, err
	}
	if !link.Enabled {
		return store.Link{}, ErrDisabled
	}
	now := s.now().UTC()
	if link.Expired(now) {
		return store.Link{}, ErrExpired
	}

	// Best effort: a failed counter update must not fail the redirect.
	if err := s.repo.IncrementHits(ctx, code, now); err == nil {
		link.Hits++
		link.LastAccessAt = &now
	}
	return link, nil
}

// Get returns a link by code.
func (s *Service) Get(ctx context.Context, code string) (store.Link, error) {
	return s.repo.GetByCode(ctx, code)
}

// List validates pagination and returns a page of links.
func (s *Service) List(ctx context.Context, page, pageSize int, query string) (store.ListResult, error) {
	if page < 1 || pageSize < 1 || pageSize > maxPageSize {
		return store.ListResult{}, invalid(CodeInvalidPagination, "page must be >= 1 and page_size must be between 1 and 100")
	}
	return s.repo.List(ctx, store.ListOptions{Page: page, PageSize: pageSize, Query: strings.TrimSpace(query)})
}

// Update applies a partial update after validating the new values.
func (s *Service) Update(ctx context.Context, code string, in UpdateInput) (store.Link, error) {
	now := s.now().UTC()
	patch := store.Patch{
		ClearExpires: in.ClearExpires,
		Enabled:      in.Enabled,
		UpdatedAt:    now,
	}
	if in.TargetURL != nil {
		if err := ValidateURL(*in.TargetURL); err != nil {
			return store.Link{}, err
		}
		trimmed := strings.TrimSpace(*in.TargetURL)
		patch.TargetURL = &trimmed
	}
	if in.ExpiresAt != nil {
		if !in.ExpiresAt.After(now) {
			return store.Link{}, invalid(CodeInvalidExpires, "expires_at must be in the future")
		}
		patch.ExpiresAt = utcOrNil(in.ExpiresAt)
	}
	return s.repo.Update(ctx, code, patch)
}

// Delete removes a link.
func (s *Service) Delete(ctx context.Context, code string) error {
	return s.repo.Delete(ctx, code)
}

// ShortURL renders the public short URL for a code.
func (s *Service) ShortURL(code string) string {
	if s.baseURL == "" {
		return "/" + code
	}
	return s.baseURL + "/" + code
}

// ValidateURL checks that raw is a usable absolute http(s) URL.
func ValidateURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > maxURLLength {
		return invalid(CodeInvalidURL, fmt.Sprintf("url must be between 1 and %d characters", maxURLLength))
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return invalid(CodeInvalidURL, "url must be an absolute URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return invalid(CodeInvalidURL, "url scheme must be http or https")
	}
}

// ValidateAlias checks that alias matches the allowed character set and length.
func ValidateAlias(alias string) error {
	if !aliasPattern.MatchString(alias) {
		return invalid(CodeInvalidAlias, "alias must be 3-32 characters matching [a-zA-Z0-9_-]")
	}
	return nil
}

func utcOrNil(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
