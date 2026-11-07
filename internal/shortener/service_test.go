package shortener_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/turbocalhj/MyGoProject/internal/shortener"
	"github.com/turbocalhj/MyGoProject/internal/store"
)

type fakeRepo struct {
	links          map[string]store.Link
	createErr      error
	incrementErr   error
	incrementCalls int
	listOptions    store.ListOptions
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{links: map[string]store.Link{}}
}

func (f *fakeRepo) Create(_ context.Context, link store.Link) error {
	if f.createErr != nil {
		return f.createErr
	}
	if _, ok := f.links[link.Code]; ok {
		return store.ErrConflict
	}
	f.links[link.Code] = link
	return nil
}

func (f *fakeRepo) GetByCode(_ context.Context, code string) (store.Link, error) {
	l, ok := f.links[code]
	if !ok {
		return store.Link{}, store.ErrNotFound
	}
	return l, nil
}

func (f *fakeRepo) List(_ context.Context, opts store.ListOptions) (store.ListResult, error) {
	f.listOptions = opts
	return store.ListResult{Items: []store.Link{}, Total: 0}, nil
}

func (f *fakeRepo) Update(_ context.Context, code string, patch store.Patch) (store.Link, error) {
	l, ok := f.links[code]
	if !ok {
		return store.Link{}, store.ErrNotFound
	}
	if patch.TargetURL != nil {
		l.TargetURL = *patch.TargetURL
	}
	if patch.ClearExpires {
		l.ExpiresAt = nil
	} else if patch.ExpiresAt != nil {
		l.ExpiresAt = patch.ExpiresAt
	}
	if patch.Enabled != nil {
		l.Enabled = *patch.Enabled
	}
	f.links[code] = l
	return l, nil
}

func (f *fakeRepo) Delete(_ context.Context, code string) error {
	if _, ok := f.links[code]; !ok {
		return store.ErrNotFound
	}
	delete(f.links, code)
	return nil
}

func (f *fakeRepo) IncrementHits(_ context.Context, code string, at time.Time) error {
	f.incrementCalls++
	if f.incrementErr != nil {
		return f.incrementErr
	}
	l, ok := f.links[code]
	if !ok {
		return store.ErrNotFound
	}
	l.Hits++
	l.LastAccessAt = &at
	f.links[code] = l
	return nil
}

func (f *fakeRepo) Ping(context.Context) error { return nil }

var fixedNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func newService(t *testing.T, repo store.Repository, codes ...string) *shortener.Service {
	t.Helper()
	return shortener.NewService(repo, shortener.Options{
		BaseURL: "https://sho.rt",
		Now:     func() time.Time { return fixedNow },
		Generate: func() (string, error) {
			if len(codes) == 0 {
				return "abc1234", nil
			}
			code := codes[0]
			codes = codes[1:]
			return code, nil
		},
	})
}

func TestCreateGeneratesCode(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo)

	link, err := svc.Create(context.Background(), shortener.CreateInput{TargetURL: "https://example.com/long"})
	require.NoError(t, err)
	require.Equal(t, "abc1234", link.Code)
	require.True(t, link.Enabled)
	require.True(t, fixedNow.Equal(link.CreatedAt))
	require.Equal(t, "https://sho.rt/abc1234", svc.ShortURL(link.Code))
}

func TestCreateCustomAlias(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo)

	link, err := svc.Create(context.Background(), shortener.CreateInput{TargetURL: "https://example.com", Alias: "my-link"})
	require.NoError(t, err)
	require.Equal(t, "my-link", link.Code)
}

func TestCreateAliasConflict(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo)
	_, err := svc.Create(context.Background(), shortener.CreateInput{TargetURL: "https://example.com", Alias: "taken"})
	require.NoError(t, err)

	_, err = svc.Create(context.Background(), shortener.CreateInput{TargetURL: "https://example.org", Alias: "taken"})
	require.ErrorIs(t, err, shortener.ErrConflict)
}

func TestCreateRetriesOnGeneratedCollision(t *testing.T) {
	repo := newFakeRepo()
	repo.links["dup1111"] = store.Link{Code: "dup1111"}
	svc := newService(t, repo, "dup1111", "uni2222")

	link, err := svc.Create(context.Background(), shortener.CreateInput{TargetURL: "https://example.com"})
	require.NoError(t, err)
	require.Equal(t, "uni2222", link.Code)
}

func TestCreateExhaustsRetries(t *testing.T) {
	repo := newFakeRepo()
	repo.links["dup1111"] = store.Link{Code: "dup1111"}
	svc := newService(t, repo, "dup1111", "dup1111", "dup1111", "dup1111", "dup1111", "dup1111")

	_, err := svc.Create(context.Background(), shortener.CreateInput{TargetURL: "https://example.com"})
	require.ErrorIs(t, err, shortener.ErrCodeExhausted)
}

func TestCreateValidation(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo)
	ctx := context.Background()

	past := fixedNow.Add(-time.Hour)
	_, err := svc.Create(ctx, shortener.CreateInput{TargetURL: "https://example.com", ExpiresAt: &past})
	var vErr *shortener.ValidationError
	require.ErrorAs(t, err, &vErr)
	require.Equal(t, shortener.CodeInvalidExpires, vErr.Code)

	_, err = svc.Create(ctx, shortener.CreateInput{TargetURL: "https://example.com", Alias: "a b!"})
	require.ErrorAs(t, err, &vErr)
	require.Equal(t, shortener.CodeInvalidAlias, vErr.Code)
}

func TestValidateURL(t *testing.T) {
	valid := []string{"https://example.com", "http://example.com/a?b=c#d", "  https://example.com  "}
	for _, raw := range valid {
		require.NoError(t, shortener.ValidateURL(raw), raw)
	}

	invalid := []string{"", "   ", "example.com", "ftp://example.com", "https://", "javascript:alert(1)"}
	for _, raw := range invalid {
		err := shortener.ValidateURL(raw)
		var vErr *shortener.ValidationError
		require.ErrorAs(t, err, &vErr, raw)
		require.Equal(t, shortener.CodeInvalidURL, vErr.Code, raw)
	}

	tooLong := "https://example.com/" + string(make([]byte, 2100))
	require.Error(t, shortener.ValidateURL(tooLong))
}

func TestResolve(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo)
	ctx := context.Background()

	link, err := svc.Create(ctx, shortener.CreateInput{TargetURL: "https://example.com"})
	require.NoError(t, err)

	resolved, err := svc.Resolve(ctx, link.Code)
	require.NoError(t, err)
	require.Equal(t, "https://example.com", resolved.TargetURL)
	require.EqualValues(t, 1, resolved.Hits)
	require.Equal(t, 1, repo.incrementCalls)
}

func TestResolveExpiredDoesNotCount(t *testing.T) {
	repo := newFakeRepo()
	expires := fixedNow.Add(-time.Minute)
	repo.links["exp1234"] = store.Link{Code: "exp1234", TargetURL: "https://example.com", Enabled: true, ExpiresAt: &expires}
	svc := newService(t, repo)

	_, err := svc.Resolve(context.Background(), "exp1234")
	require.ErrorIs(t, err, shortener.ErrExpired)
	require.Zero(t, repo.incrementCalls)
}

func TestResolveDisabledDoesNotCount(t *testing.T) {
	repo := newFakeRepo()
	repo.links["off1234"] = store.Link{Code: "off1234", TargetURL: "https://example.com", Enabled: false}
	svc := newService(t, repo)

	_, err := svc.Resolve(context.Background(), "off1234")
	require.ErrorIs(t, err, shortener.ErrDisabled)
	require.Zero(t, repo.incrementCalls)
}

func TestResolveToleratesCounterFailure(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo)
	ctx := context.Background()
	link, err := svc.Create(ctx, shortener.CreateInput{TargetURL: "https://example.com"})
	require.NoError(t, err)

	repo.incrementErr = errors.New("boom")
	resolved, err := svc.Resolve(ctx, link.Code)
	require.NoError(t, err, "analytics failures must not break redirects")
	require.Zero(t, resolved.Hits)
}

func TestListRejectsInvalidPagination(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo)
	ctx := context.Background()

	for _, tc := range []struct{ page, size int }{{0, 20}, {-1, 20}, {1, 0}, {1, 101}} {
		_, err := svc.List(ctx, tc.page, tc.size, "")
		var vErr *shortener.ValidationError
		require.ErrorAs(t, err, &vErr)
		require.Equal(t, shortener.CodeInvalidPagination, vErr.Code)
	}

	_, err := svc.List(ctx, 2, 10, " example ")
	require.NoError(t, err)
	require.Equal(t, 2, repo.listOptions.Page)
	require.Equal(t, "example", repo.listOptions.Query)
}

func TestUpdateValidatesAndApplies(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo)
	ctx := context.Background()
	link, err := svc.Create(ctx, shortener.CreateInput{TargetURL: "https://example.com"})
	require.NoError(t, err)

	past := fixedNow.Add(-time.Minute)
	_, err = svc.Update(ctx, link.Code, shortener.UpdateInput{ExpiresAt: &past})
	var vErr *shortener.ValidationError
	require.ErrorAs(t, err, &vErr)
	require.Equal(t, shortener.CodeInvalidExpires, vErr.Code)

	newURL := "ftp://example.com"
	_, err = svc.Update(ctx, link.Code, shortener.UpdateInput{TargetURL: &newURL})
	require.ErrorAs(t, err, &vErr)
	require.Equal(t, shortener.CodeInvalidURL, vErr.Code)

	validURL := "https://example.org/next"
	updated, err := svc.Update(ctx, link.Code, shortener.UpdateInput{TargetURL: &validURL})
	require.NoError(t, err)
	require.Equal(t, validURL, updated.TargetURL)
	require.True(t, fixedNow.Equal(updated.UpdatedAt))
}

func TestShortURLWithoutBase(t *testing.T) {
	svc := shortener.NewService(newFakeRepo(), shortener.Options{})
	require.Equal(t, "/abc", svc.ShortURL("abc"))
}
