package httpapi

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type rateLimitedVisitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type inMemoryRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*rateLimitedVisitor
	limit    rate.Limit
	burst    int
	ttl      time.Duration
}

func NewRateLimitMiddleware(limit rate.Limit, burst int, ttl time.Duration) gin.HandlerFunc {
	store := &inMemoryRateLimiter{
		visitors: map[string]*rateLimitedVisitor{},
		limit:    limit,
		burst:    burst,
		ttl:      ttl,
	}
	return func(c *gin.Context) {
		key := c.Request.URL.Path + ":" + c.ClientIP()
		if !store.allow(key, time.Now()) {
			LogWarnRequest(c, "rate_limit_exceeded",
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"client_ip", c.ClientIP(),
			)
			Error(c, http.StatusTooManyRequests, "rate limit exceeded")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *inMemoryRateLimiter) allow(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupExpired(now)

	visitor, ok := s.visitors[key]
	if !ok {
		visitor = &rateLimitedVisitor{
			limiter:  rate.NewLimiter(s.limit, s.burst),
			lastSeen: now,
		}
		s.visitors[key] = visitor
	}
	visitor.lastSeen = now
	return visitor.limiter.Allow()
}

func (s *inMemoryRateLimiter) cleanupExpired(now time.Time) {
	for key, visitor := range s.visitors {
		if now.Sub(visitor.lastSeen) > s.ttl {
			delete(s.visitors, key)
		}
	}
}
