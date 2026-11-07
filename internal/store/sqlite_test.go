package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/turbocalhj/MyGoProject/internal/store"
)

func newRepo(t *testing.T) *store.SQLite {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	repo, err := store.Open(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, repo.Close()) })
	return repo
}

func sampleLink(code string) store.Link {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return store.Link{Code: code, TargetURL: "https://example.com/" + code, Enabled: true, CreatedAt: now, UpdatedAt: now}
}

func TestCreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	link := sampleLink("abc1234")
	require.NoError(t, repo.Create(ctx, link))

	got, err := repo.GetByCode(ctx, "abc1234")
	require.NoError(t, err)
	require.Equal(t, link.TargetURL, got.TargetURL)
	require.True(t, got.Enabled)
	require.Zero(t, got.Hits)
	require.Nil(t, got.ExpiresAt)
	require.True(t, link.CreatedAt.Equal(got.CreatedAt))
}

func TestCreateDuplicateReturnsConflict(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	require.NoError(t, repo.Create(ctx, sampleLink("dup1234")))
	err := repo.Create(ctx, sampleLink("dup1234"))
	require.ErrorIs(t, err, store.ErrConflict)
}

func TestGetMissingReturnsNotFound(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.GetByCode(context.Background(), "missing")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestListPaginationAndSearch(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, code := range []string{"aaa1111", "bbb2222", "ccc3333"} {
		link := sampleLink(code)
		link.CreatedAt = base.Add(time.Duration(i) * time.Minute)
		link.UpdatedAt = link.CreatedAt
		require.NoError(t, repo.Create(ctx, link))
	}

	page, err := repo.List(ctx, store.ListOptions{Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, 3, page.Total)
	require.Len(t, page.Items, 2)
	require.Equal(t, "ccc3333", page.Items[0].Code, "newest first")

	page, err = repo.List(ctx, store.ListOptions{Page: 2, PageSize: 2})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)

	page, err = repo.List(ctx, store.ListOptions{Page: 1, PageSize: 10, Query: "example.com/bbb"})
	require.NoError(t, err)
	require.Equal(t, 1, page.Total)
	require.Equal(t, "bbb2222", page.Items[0].Code)
}

func TestPartialUpdate(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	require.NoError(t, repo.Create(ctx, sampleLink("upl1234")))

	newURL := "https://example.org/updated"
	updatedAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	got, err := repo.Update(ctx, "upl1234", store.Patch{TargetURL: &newURL, UpdatedAt: updatedAt})
	require.NoError(t, err)
	require.Equal(t, newURL, got.TargetURL)
	require.True(t, updatedAt.Equal(got.UpdatedAt))
	require.True(t, got.Enabled, "unspecified fields stay untouched")

	disabled := false
	got, err = repo.Update(ctx, "upl1234", store.Patch{Enabled: &disabled, UpdatedAt: updatedAt})
	require.NoError(t, err)
	require.False(t, got.Enabled)

	expiry := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	got, err = repo.Update(ctx, "upl1234", store.Patch{ExpiresAt: &expiry, UpdatedAt: updatedAt})
	require.NoError(t, err)
	require.NotNil(t, got.ExpiresAt)

	got, err = repo.Update(ctx, "upl1234", store.Patch{ClearExpires: true, UpdatedAt: updatedAt})
	require.NoError(t, err)
	require.Nil(t, got.ExpiresAt)
}

func TestUpdateAndDeleteMissingReturnNotFound(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	_, err := repo.Update(ctx, "missing", store.Patch{UpdatedAt: time.Now()})
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.Delete(ctx, "missing")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestDeleteRemovesLink(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	require.NoError(t, repo.Create(ctx, sampleLink("del1234")))
	require.NoError(t, repo.Delete(ctx, "del1234"))

	_, err := repo.GetByCode(ctx, "del1234")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestIncrementHits(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	require.NoError(t, repo.Create(ctx, sampleLink("hit1234")))

	at := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	require.NoError(t, repo.IncrementHits(ctx, "hit1234", at))

	got, err := repo.GetByCode(ctx, "hit1234")
	require.NoError(t, err)
	require.EqualValues(t, 1, got.Hits)
	require.NotNil(t, got.LastAccessAt)
	require.True(t, at.Equal(*got.LastAccessAt))

	err = repo.IncrementHits(ctx, "missing", at)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestPing(t *testing.T) {
	repo := newRepo(t)
	require.NoError(t, repo.Ping(context.Background()))
}
