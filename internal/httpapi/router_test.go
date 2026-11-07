package httpapi_test

import (
	"bytes"
	"encoding/json"
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

type testEnv struct {
	server *httptest.Server
	repo   *store.SQLite
	svc    *shortener.Service
	client *http.Client
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	repo, err := store.Open(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := shortener.NewService(repo, shortener.Options{BaseURL: "http://short.test"})
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{Service: svc, Repository: repo, Logger: logger}))
	t.Cleanup(server.Close)

	return &testEnv{
		server: server,
		repo:   repo,
		svc:    svc,
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

func (e *testEnv) request(t *testing.T, method, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.server.URL+path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	require.NoError(t, err)
	return resp
}

func decode(t *testing.T, resp *http.Response, target any) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(target))
}

func createLink(t *testing.T, env *testEnv, body map[string]any) map[string]any {
	t.Helper()
	resp := env.request(t, http.MethodPost, "/api/v1/links", body, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var out map[string]any
	decode(t, resp, &out)
	return out
}

func TestCreateAndRedirect(t *testing.T) {
	env := newEnv(t)

	created := createLink(t, env, map[string]any{"url": "https://example.com/target"})
	code, ok := created["code"].(string)
	require.True(t, ok)
	require.Len(t, code, shortener.CodeLength)
	require.Equal(t, "http://short.test/"+code, created["short_url"])
	require.Equal(t, "https://example.com/target", created["target_url"])

	resp := env.request(t, http.MethodGet, "/"+code, nil, nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "https://example.com/target", resp.Header.Get("Location"))
	require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))

	var stats map[string]any
	decode(t, env.request(t, http.MethodGet, "/api/v1/links/"+code+"/stats", nil, nil), &stats)
	require.EqualValues(t, 1, stats["hits"])
	require.NotNil(t, stats["last_access_at"])
}

func TestCreateCustomAliasAndConflict(t *testing.T) {
	env := newEnv(t)

	created := createLink(t, env, map[string]any{"url": "https://example.com", "alias": "my-link"})
	require.Equal(t, "my-link", created["code"])

	resp := env.request(t, http.MethodPost, "/api/v1/links", map[string]any{"url": "https://example.org", "alias": "my-link"}, nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	decode(t, resp, &body)
	require.Equal(t, "alias_taken", body.Error.Code)
	require.NotEmpty(t, body.Error.RequestID)
}

func TestCreateValidationErrors(t *testing.T) {
	env := newEnv(t)

	for name, payload := range map[string]map[string]any{
		"invalid_url":   {"url": "ftp://example.com"},
		"invalid_alias": {"url": "https://example.com", "alias": "a b!"},
	} {
		t.Run(name, func(t *testing.T) {
			resp := env.request(t, http.MethodPost, "/api/v1/links", payload, nil)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			decode(t, resp, &body)
			require.Equal(t, name, body.Error.Code, "error code")
			require.Equal(t, name, name)
		})
	}

	resp := env.request(t, http.MethodPost, "/api/v1/links", map[string]any{}, nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestExpiredLinkReturnsGoneAndSkipsCounting(t *testing.T) {
	env := newEnv(t)
	past := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, env.repo.Create(t.Context(), store.Link{
		Code:      "exp1234",
		TargetURL: "https://example.com",
		Enabled:   true,
		ExpiresAt: &past,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}))

	resp := env.request(t, http.MethodGet, "/exp1234", nil, nil)
	require.Equal(t, http.StatusGone, resp.StatusCode)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decode(t, resp, &body)
	require.Equal(t, "link_expired", body.Error.Code)

	link, err := env.repo.GetByCode(t.Context(), "exp1234")
	require.NoError(t, err)
	require.Zero(t, link.Hits)
}

func TestDisableAndDeleteLink(t *testing.T) {
	env := newEnv(t)
	created := createLink(t, env, map[string]any{"url": "https://example.com"})
	code := created["code"].(string)

	disabled := false
	resp := env.request(t, http.MethodPatch, "/api/v1/links/"+code, map[string]any{"enabled": disabled}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = env.request(t, http.MethodGet, "/"+code, nil, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()

	resp = env.request(t, http.MethodDelete, "/api/v1/links/"+code, nil, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	resp = env.request(t, http.MethodGet, "/"+code, nil, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	resp = env.request(t, http.MethodDelete, "/api/v1/links/"+code, nil, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestUpdateOnlyTargetKeepsHits(t *testing.T) {
	env := newEnv(t)
	created := createLink(t, env, map[string]any{"url": "https://example.com"})
	code := created["code"].(string)
	require.Equal(t, http.StatusFound, env.request(t, http.MethodGet, "/"+code, nil, nil).StatusCode)

	resp := env.request(t, http.MethodPatch, "/api/v1/links/"+code, map[string]any{"target_url": "https://example.org/new"}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated map[string]any
	decode(t, resp, &updated)
	require.Equal(t, "https://example.org/new", updated["target_url"])
	require.EqualValues(t, 1, updated["hits"])

	resp = env.request(t, http.MethodGet, "/"+code, nil, nil)
	require.Equal(t, "https://example.org/new", resp.Header.Get("Location"))
	resp.Body.Close()
}

func TestClearExpires(t *testing.T) {
	env := newEnv(t)
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	created := createLink(t, env, map[string]any{"url": "https://example.com", "expires_at": future})
	code := created["code"].(string)
	require.NotNil(t, created["expires_at"])

	resp := env.request(t, http.MethodPatch, "/api/v1/links/"+code, map[string]any{"expires_at": nil}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated map[string]any
	decode(t, resp, &updated)
	require.Nil(t, updated["expires_at"])
}

func TestListAndPaginationValidation(t *testing.T) {
	env := newEnv(t)
	createLink(t, env, map[string]any{"url": "https://example.com/one"})
	createLink(t, env, map[string]any{"url": "https://example.com/two"})

	resp := env.request(t, http.MethodGet, "/api/v1/links?page=1&page_size=2", nil, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	decode(t, resp, &list)
	require.Equal(t, 2, list.Total)
	require.Len(t, list.Items, 2)

	resp = env.request(t, http.MethodGet, "/api/v1/links?page=0", nil, nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var bad struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decode(t, resp, &bad)
	require.Equal(t, "invalid_pagination", bad.Error.Code)
}

func TestUnknownCodeReturnsNotFound(t *testing.T) {
	env := newEnv(t)
	resp := env.request(t, http.MethodGet, "/nope1234", nil, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestHealthEndpoints(t *testing.T) {
	env := newEnv(t)

	for _, path := range []string{"/healthz", "/readyz"} {
		resp := env.request(t, http.MethodGet, path, nil, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		decode(t, resp, &body)
		require.Equal(t, "ok", body["status"])
	}
}

func TestReadyZReportsStorageFailure(t *testing.T) {
	env := newEnv(t)
	require.NoError(t, env.repo.Close())

	resp := env.request(t, http.MethodGet, "/readyz", nil, nil)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	var body map[string]any
	decode(t, resp, &body)
	require.Equal(t, "unavailable", body["status"])
}

func TestRequestIDPropagation(t *testing.T) {
	env := newEnv(t)

	resp := env.request(t, http.MethodGet, "/healthz", nil, map[string]string{"X-Request-ID": "trace-123"})
	require.Equal(t, "trace-123", resp.Header.Get("X-Request-ID"))
	resp.Body.Close()

	resp = env.request(t, http.MethodGet, "/healthz", nil, nil)
	require.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, resp.Header.Get("X-Request-ID"))
	resp.Body.Close()
}
