package httpapi

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestID propagates or generates the X-Request-ID header and stores it in
// the gin context for downstream handlers.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// AccessLog emits one structured log line per request.
func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "http_request",
			slog.String("request_id", requestID(c)),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000.0),
		)
	}
}

type tokenBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

// RateLimiter is a per-key in-memory token bucket.
type RateLimiter struct {
	mu              sync.Mutex
	buckets         map[string]*tokenBucket
	rate            float64
	burst           float64
	now             func() time.Time
	lastCleanup     time.Time
	cleanupInterval time.Duration
	idleTTL         time.Duration
}

// NewRateLimiter builds a limiter that refills rate tokens per second up to
// burst. A nil clock defaults to time.Now.
func NewRateLimiter(rate float64, burst int, now func() time.Time) *RateLimiter {
	if rate <= 0 {
		rate = 1
	}
	if burst < 1 {
		burst = 1
	}
	if now == nil {
		now = time.Now
	}
	nowTime := now()
	return &RateLimiter{
		buckets:         map[string]*tokenBucket{},
		rate:            rate,
		burst:           float64(burst),
		now:             now,
		lastCleanup:     nowTime,
		cleanupInterval: 5 * time.Minute,
		idleTTL:         10 * time.Minute,
	}
}

// Allow consumes one token for key. When rejected it returns a suggested
// retry delay.
func (l *RateLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	if elapsed := now.Sub(b.last); elapsed > 0 {
		b.tokens = min(l.burst, b.tokens+elapsed.Seconds()*l.rate)
		b.last = now
	}
	b.lastSeen = now

	l.cleanupLocked(now)

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1-b.tokens)/l.rate*float64(time.Second)) + time.Millisecond
	return false, wait
}

func (l *RateLimiter) cleanupLocked(now time.Time) {
	if now.Sub(l.lastCleanup) < l.cleanupInterval {
		return
	}
	for key, b := range l.buckets {
		if now.Sub(b.lastSeen) > l.idleTTL {
			delete(l.buckets, key)
		}
	}
	l.lastCleanup = now
}

// RateLimit rejects requests from a client that exceeds its bucket. Health
// checks are exempt so orchestrators never get throttled.
func RateLimit(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.URL.Path {
		case "/healthz", "/readyz":
			c.Next()
			return
		}

		allowed, retryAfter := limiter.Allow(c.ClientIP())
		if !allowed {
			seconds := int(math.Ceil(retryAfter.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			writeError(c, 429, codeRateLimited, "too many requests")
			return
		}
		c.Next()
	}
}
