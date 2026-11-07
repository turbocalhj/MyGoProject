package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/turbocalhj/MyGoProject/internal/httpapi"
	"github.com/turbocalhj/MyGoProject/internal/shortener"
	"github.com/turbocalhj/MyGoProject/internal/store"
)

func TestRateLimiterTokenBucket(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := httpapi.NewRateLimiter(20, 2, func() time.Time { return now })

	allowed, _ := limiter.Allow("1.2.3.4")
	require.True(t, allowed)
	allowed, _ = limiter.Allow("1.2.3.4")
	require.True(t, allowed)

	allowed, retry := limiter.Allow("1.2.3.4")
	require.False(t, allowed)
	require.Greater(t, retry, time.Duration(0))

	allowed, _ = limiter.Allow("5.6.7.8")
	require.True(t, allowed, "buckets are per client")

	now = now.Add(100 * time.Millisecond) // 20 rps refills 2 tokens
	allowed, _ = limiter.Allow("1.2.3.4")
	require.True(t, allowed)
}

func TestRateLimitMiddleware(t *testing.T) {
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	repo, err := store.Open(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := shortener.NewService(repo, shortener.Options{})
	router := httpapi.NewRouter(httpapi.Deps{
		Service:     svc,
		Repository:  repo,
		RateLimiter: httpapi.NewRateLimiter(1, 2, nil),
		Logger:      logger,
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	for i := 0; i < 2; i++ {
		resp, err := http.Get(server.URL + "/api/v1/links")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	}

	resp, err := http.Get(server.URL + "/api/v1/links")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("Retry-After"))

	health, err := http.Get(server.URL + "/healthz")
	require.NoError(t, err)
	defer health.Body.Close()
	require.Equal(t, http.StatusOK, health.StatusCode, "health checks bypass throttling")
}
