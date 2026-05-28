package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type visitor struct {
	tokens   float64
	lastSeen time.Time
}

// RateLimiter implements a simple per-IP token bucket.
func RateLimiter(rate float64, burst int) gin.HandlerFunc {
	var (
		mu       sync.Mutex
		visitors = make(map[string]*visitor)
	)

	// Background cleanup
	go func() {
		for range time.Tick(time.Minute) {
			mu.Lock()
			for ip, v := range visitors {
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.Lock()
		v, exists := visitors[ip]
		if !exists {
			v = &visitor{tokens: float64(burst), lastSeen: time.Now()}
			visitors[ip] = v
		}

		elapsed := time.Since(v.lastSeen).Seconds()
		v.tokens += elapsed * rate
		if v.tokens > float64(burst) {
			v.tokens = float64(burst)
		}
		v.lastSeen = time.Now()

		if v.tokens < 1 {
			mu.Unlock()
			response.Error(c, http.StatusTooManyRequests, "rate limit exceeded")
			c.Abort()
			return
		}

		v.tokens--
		mu.Unlock()

		c.Next()
	}
}
