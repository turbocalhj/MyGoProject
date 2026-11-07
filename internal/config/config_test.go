package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/turbocalhj/MyGoProject/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"HTTP_ADDR", "DATABASE_DSN", "BASE_URL", "RATE_LIMIT_RPS", "RATE_LIMIT_BURST", "LOG_LEVEL"} {
		t.Setenv(key, "")
	}

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, "file:shortlink.db", cfg.DatabaseDSN)
	require.Empty(t, cfg.BaseURL)
	require.InDelta(t, 20, cfg.RateLimitRPS, 0.001)
	require.Equal(t, 40, cfg.RateLimitBurst)
	require.Equal(t, "info", cfg.LogLevel)
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("DATABASE_DSN", "file:test.db")
	t.Setenv("BASE_URL", "https://sho.rt/")
	t.Setenv("RATE_LIMIT_RPS", "5.5")
	t.Setenv("RATE_LIMIT_BURST", "10")
	t.Setenv("LOG_LEVEL", "DEBUG")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, ":9090", cfg.HTTPAddr)
	require.Equal(t, "https://sho.rt", cfg.BaseURL, "trailing slash must be trimmed")
	require.InDelta(t, 5.5, cfg.RateLimitRPS, 0.001)
	require.Equal(t, 10, cfg.RateLimitBurst)
	require.Equal(t, "debug", cfg.LogLevel)
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"non numeric rps":   {"RATE_LIMIT_RPS": "fast"},
		"non integer burst": {"RATE_LIMIT_BURST": "1.5"},
		"zero rps":          {"RATE_LIMIT_RPS": "0"},
		"zero burst":        {"RATE_LIMIT_BURST": "0"},
		"bad log level":     {"LOG_LEVEL": "verbose"},
		"bad base url":      {"BASE_URL": "sho.rt"},
		"empty addr":        {"HTTP_ADDR": ""},
		"empty dsn":         {"DATABASE_DSN": ""},
	}

	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			for key, value := range env {
				t.Setenv(key, value)
			}
			_, err := config.Load()
			if name == "empty addr" || name == "empty dsn" {
				// Empty env vars fall back to defaults by design.
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}
