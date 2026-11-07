package httpapi

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/turbocalhj/MyGoProject/internal/shortener"
	"github.com/turbocalhj/MyGoProject/internal/store"
)

// Deps are the collaborators required by the HTTP layer.
type Deps struct {
	Service     *shortener.Service
	Repository  store.Repository
	RateLimiter *RateLimiter
	Logger      *slog.Logger
}

// NewRouter wires middleware and routes. When RateLimiter is nil, throttling
// is disabled (useful for tests that focus on other behavior).
func NewRouter(d Deps) *gin.Engine {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(RequestID())
	r.Use(AccessLog(logger))
	if d.RateLimiter != nil {
		r.Use(RateLimit(d.RateLimiter))
	}

	h := &handlers{service: d.Service, repo: d.Repository, logger: logger}

	r.GET("/healthz", h.health)
	r.GET("/readyz", h.ready)

	api := r.Group("/api/v1")
	api.POST("/links", h.createLink)
	api.GET("/links", h.listLinks)
	api.GET("/links/:code", h.getLink)
	api.PATCH("/links/:code", h.updateLink)
	api.DELETE("/links/:code", h.deleteLink)
	api.GET("/links/:code/stats", h.linkStats)

	r.GET("/:code", h.redirect)

	return r
}

// DefaultRateLimiter builds the production limiter settings.
func DefaultRateLimiter(rate float64, burst int) *RateLimiter {
	return NewRateLimiter(rate, burst, time.Now)
}
