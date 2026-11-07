// Package config loads and validates runtime configuration from environment
// variables. It intentionally has no config-file support so that deployments
// stay declarative and container friendly.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds every runtime setting the server needs.
type Config struct {
	HTTPAddr       string
	DatabaseDSN    string
	BaseURL        string
	RateLimitRPS   float64
	RateLimitBurst int
	LogLevel       string
}

// Load reads configuration from the process environment, applying defaults and
// validating values. It returns an error instead of silently accepting invalid
// input.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:       envOrDefault("HTTP_ADDR", ":8080"),
		DatabaseDSN:    envOrDefault("DATABASE_DSN", "file:shortlink.db"),
		BaseURL:        strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		RateLimitRPS:   20,
		RateLimitBurst: 40,
		LogLevel:       strings.ToLower(envOrDefault("LOG_LEVEL", "info")),
	}

	if raw := os.Getenv("RATE_LIMIT_RPS"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return Config{}, fmt.Errorf("config: RATE_LIMIT_RPS must be a number: %w", err)
		}
		cfg.RateLimitRPS = v
	}
	if raw := os.Getenv("RATE_LIMIT_BURST"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("config: RATE_LIMIT_BURST must be an integer: %w", err)
		}
		cfg.RateLimitBurst = v
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks that every field holds a usable value.
func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("config: HTTP_ADDR must not be empty")
	}
	if c.DatabaseDSN == "" {
		return fmt.Errorf("config: DATABASE_DSN must not be empty")
	}
	if c.RateLimitRPS <= 0 {
		return fmt.Errorf("config: RATE_LIMIT_RPS must be greater than 0, got %v", c.RateLimitRPS)
	}
	if c.RateLimitBurst < 1 {
		return fmt.Errorf("config: RATE_LIMIT_BURST must be at least 1, got %d", c.RateLimitBurst)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: LOG_LEVEL must be one of debug/info/warn/error, got %q", c.LogLevel)
	}
	if c.BaseURL != "" && !strings.HasPrefix(c.BaseURL, "http://") && !strings.HasPrefix(c.BaseURL, "https://") {
		return fmt.Errorf("config: BASE_URL must start with http:// or https://, got %q", c.BaseURL)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
