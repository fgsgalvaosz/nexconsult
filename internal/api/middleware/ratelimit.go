package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nexconsult/cnpj-api/internal/config"
	"golang.org/x/time/rate"
)

// RateLimiter implements rate limiting using token bucket algorithm
type RateLimiter struct {
	config   config.RateLimitConfig
	clients  map[string]*rate.Limiter
	mu       sync.RWMutex
	lastSeen map[string]time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config config.RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config:   config,
		clients:  make(map[string]*rate.Limiter),
		lastSeen: make(map[string]time.Time),
	}

	// Start cleanup goroutine
	go rl.cleanupClients()

	return rl
}

// Middleware returns the rate limiting middleware
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get client identifier (IP address)
		clientID := c.ClientIP()

		// Get or create limiter for this client
		limiter := rl.getLimiter(clientID)

		// Check if request is allowed
		if !limiter.Allow() {
			// Get retry after duration
			retryAfter := rl.getRetryAfter(limiter)

			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", rl.config.RequestsPerMinute))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(retryAfter).Unix()))
			c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"message":     fmt.Sprintf("Too many requests. Try again in %v", retryAfter),
				"retry_after": retryAfter.Seconds(),
				"timestamp":   time.Now(),
			})
			c.Abort()
			return
		}

		// Add rate limit headers
		remaining := rl.getRemainingTokens(limiter)
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", rl.config.RequestsPerMinute))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Minute).Unix()))

		c.Next()
	}
}

// getLimiter gets or creates a rate limiter for a client
func (rl *RateLimiter) getLimiter(clientID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Update last seen time
	rl.lastSeen[clientID] = time.Now()

	// Get existing limiter
	if limiter, exists := rl.clients[clientID]; exists {
		return limiter
	}

	// Create new limiter
	// Convert requests per minute to requests per second
	rps := rate.Limit(float64(rl.config.RequestsPerMinute) / 60.0)
	limiter := rate.NewLimiter(rps, rl.config.BurstSize)
	rl.clients[clientID] = limiter

	return limiter
}

// getRetryAfter calculates when the client can make the next request
func (rl *RateLimiter) getRetryAfter(_ *rate.Limiter) time.Duration {
	// Since we can't access the internal state directly,
	// we'll estimate based on the rate limit configuration
	tokensPerSecond := float64(rl.config.RequestsPerMinute) / 60.0
	if tokensPerSecond <= 0 {
		return time.Minute
	}

	// Estimate time for one token to become available
	tokenInterval := time.Duration(float64(time.Second) / tokensPerSecond)

	// Add some buffer time
	return tokenInterval + time.Second
}

// getRemainingTokens estimates remaining tokens (approximate)
func (rl *RateLimiter) getRemainingTokens(limiter *rate.Limiter) int {
	// Since we can't access internal state, we'll make a simple estimation
	// Try to make a reservation to see if tokens are available
	reservation := limiter.Reserve()
	if !reservation.OK() {
		return 0
	}

	// Cancel the reservation immediately since we're just testing
	reservation.Cancel()

	// If we could make a reservation, assume we have some tokens available
	// This is a rough estimate - in production you might want more sophisticated tracking
	return rl.config.BurstSize / 2 // Conservative estimate
}

// cleanupClients removes old client limiters to prevent memory leaks
func (rl *RateLimiter) cleanupClients() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()

		cutoff := time.Now().Add(-rl.config.CleanupInterval * 2)

		for clientID, lastSeen := range rl.lastSeen {
			if lastSeen.Before(cutoff) {
				delete(rl.clients, clientID)
				delete(rl.lastSeen, clientID)
			}
		}

		rl.mu.Unlock()
	}
}

// GetStats returns rate limiter statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return map[string]interface{}{
		"active_clients":      len(rl.clients),
		"requests_per_minute": rl.config.RequestsPerMinute,
		"burst_size":          rl.config.BurstSize,
		"cleanup_interval":    rl.config.CleanupInterval,
	}
}
