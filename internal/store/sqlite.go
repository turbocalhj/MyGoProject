package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, keeps CGO disabled
)

const timeLayout = time.RFC3339Nano

const schema = `
CREATE TABLE IF NOT EXISTS links (
	code           TEXT PRIMARY KEY,
	target_url     TEXT NOT NULL,
	enabled        INTEGER NOT NULL DEFAULT 1,
	expires_at     TEXT,
	hits           INTEGER NOT NULL DEFAULT 0,
	created_at     TEXT NOT NULL,
	updated_at     TEXT NOT NULL,
	last_access_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_links_created_at ON links (created_at DESC);
`

// SQLite implements Repository on top of a single SQLite database.
type SQLite struct {
	db *sql.DB
}

// Open opens the database at dsn and ensures the schema exists.
func Open(dsn string) (*SQLite, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: open database: %v", ErrStorage, err)
	}
	// In-memory databases are shared per connection string and need a single
	// connection to stay alive; file-backed databases can serve concurrent reads.
	if strings.Contains(dsn, "mode=memory") {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(10)
	}

	s := &SQLite{db: db}
	if err := s.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Migrate creates the schema. It is idempotent.
func (s *SQLite) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("%w: migrate: %v", ErrStorage, err)
	}
	return nil
}

// Close releases the database handle.
func (s *SQLite) Close() error { return s.db.Close() }

// Ping verifies the database is reachable.
func (s *SQLite) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("%w: ping: %v", ErrStorage, err)
	}
	return nil
}

// Create inserts a link, mapping unique violations to ErrConflict.
func (s *SQLite) Create(ctx context.Context, l Link) error {
	const q = `INSERT INTO links (code, target_url, enabled, expires_at, hits, created_at, updated_at, last_access_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, q,
		l.Code, l.TargetURL, boolToInt(l.Enabled), nullableTime(l.ExpiresAt),
		l.Hits, formatTime(l.CreatedAt), formatTime(l.UpdatedAt), nullableTime(l.LastAccessAt),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("%w: insert link: %v", ErrStorage, err)
	}
	return nil
}

// GetByCode returns a single link.
func (s *SQLite) GetByCode(ctx context.Context, code string) (Link, error) {
	const q = `SELECT code, target_url, enabled, expires_at, hits, created_at, updated_at, last_access_at
FROM links WHERE code = ?`
	link, err := scanLink(s.db.QueryRowContext(ctx, q, code))
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return link, err
}

// List returns a page of links ordered by newest first.
func (s *SQLite) List(ctx context.Context, opts ListOptions) (ListResult, error) {
	where := ""
	args := []any{}
	if opts.Query != "" {
		where = " WHERE code LIKE ? OR target_url LIKE ?"
		like := "%" + opts.Query + "%"
		args = append(args, like, like)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM links"+where, args...).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("%w: count links: %v", ErrStorage, err)
	}

	offset := (opts.Page - 1) * opts.PageSize
	pageArgs := append(append([]any{}, args...), opts.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, "SELECT code, target_url, enabled, expires_at, hits, created_at, updated_at, last_access_at FROM links"+where+
		" ORDER BY created_at DESC, code ASC LIMIT ? OFFSET ?", pageArgs...)
	if err != nil {
		return ListResult{}, fmt.Errorf("%w: list links: %v", ErrStorage, err)
	}
	defer rows.Close()

	items := make([]Link, 0, opts.PageSize)
	for rows.Next() {
		var (
			l          Link
			enabled    int
			expires    sql.NullString
			created    string
			updated    string
			lastAccess sql.NullString
		)
		if err := rows.Scan(&l.Code, &l.TargetURL, &enabled, &expires, &l.Hits, &created, &updated, &lastAccess); err != nil {
			return ListResult{}, fmt.Errorf("%w: scan link row: %v", ErrStorage, err)
		}
		if err := hydrate(&l, enabled, expires, created, updated, lastAccess); err != nil {
			return ListResult{}, err
		}
		items = append(items, l)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("%w: iterate links: %v", ErrStorage, err)
	}
	return ListResult{Items: items, Total: total}, nil
}

// Update applies a partial update and returns the stored link.
func (s *SQLite) Update(ctx context.Context, code string, p Patch) (Link, error) {
	link, err := s.GetByCode(ctx, code)
	if err != nil {
		return Link{}, err
	}

	if p.TargetURL != nil {
		link.TargetURL = *p.TargetURL
	}
	switch {
	case p.ClearExpires:
		link.ExpiresAt = nil
	case p.ExpiresAt != nil:
		link.ExpiresAt = p.ExpiresAt
	}
	if p.Enabled != nil {
		link.Enabled = *p.Enabled
	}
	link.UpdatedAt = p.UpdatedAt

	const q = `UPDATE links SET target_url = ?, enabled = ?, expires_at = ?, updated_at = ? WHERE code = ?`
	res, err := s.db.ExecContext(ctx, q, link.TargetURL, boolToInt(link.Enabled), nullableTime(link.ExpiresAt), formatTime(link.UpdatedAt), code)
	if err != nil {
		return Link{}, fmt.Errorf("%w: update link: %v", ErrStorage, err)
	}
	if affected, err := res.RowsAffected(); err != nil {
		return Link{}, fmt.Errorf("%w: update rows affected: %v", ErrStorage, err)
	} else if affected == 0 {
		return Link{}, ErrNotFound
	}
	return link, nil
}

// Delete removes a link, returning ErrNotFound when it did not exist.
func (s *SQLite) Delete(ctx context.Context, code string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM links WHERE code = ?`, code)
	if err != nil {
		return fmt.Errorf("%w: delete link: %v", ErrStorage, err)
	}
	if affected, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("%w: delete rows affected: %v", ErrStorage, err)
	} else if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// IncrementHits atomically records an access.
func (s *SQLite) IncrementHits(ctx context.Context, code string, at time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE links SET hits = hits + 1, last_access_at = ? WHERE code = ?`, formatTime(at), code)
	if err != nil {
		return fmt.Errorf("%w: increment hits: %v", ErrStorage, err)
	}
	if affected, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("%w: increment rows affected: %v", ErrStorage, err)
	} else if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func scanLink(row *sql.Row) (Link, error) {
	var (
		l          Link
		enabled    int
		expires    sql.NullString
		created    string
		updated    string
		lastAccess sql.NullString
	)
	if err := row.Scan(&l.Code, &l.TargetURL, &enabled, &expires, &l.Hits, &created, &updated, &lastAccess); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Link{}, err
		}
		return Link{}, fmt.Errorf("%w: scan link: %v", ErrStorage, err)
	}
	if err := hydrate(&l, enabled, expires, created, updated, lastAccess); err != nil {
		return Link{}, err
	}
	return l, nil
}

func hydrate(l *Link, enabled int, expires sql.NullString, created, updated string, lastAccess sql.NullString) error {
	l.Enabled = enabled != 0

	createdAt, err := parseTime(created)
	if err != nil {
		return err
	}
	l.CreatedAt = createdAt

	updatedAt, err := parseTime(updated)
	if err != nil {
		return err
	}
	l.UpdatedAt = updatedAt

	if expires.Valid {
		expiresAt, err := parseTime(expires.String)
		if err != nil {
			return err
		}
		l.ExpiresAt = &expiresAt
	}
	if lastAccess.Valid {
		at, err := parseTime(lastAccess.String)
		if err != nil {
			return err
		}
		l.LastAccessAt = &at
	}
	return nil
}

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(raw string) (time.Time, error) {
	t, err := time.Parse(timeLayout, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: parse timestamp %q: %v", ErrStorage, raw, err)
	}
	return t, nil
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func isUniqueViolation(err error) bool {
	return strings.Contains(strings.ToUpper(err.Error()), "UNIQUE CONSTRAINT")
}
