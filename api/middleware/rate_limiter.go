package middleware

import (
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// IPTracker stores rate limiters for each IP.
type IPTracker struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	b        int
}

// NewIPTracker initializes a new tracker.
func NewIPTracker(r rate.Limit, b int) *IPTracker {
	return &IPTracker{
		limiters: make(map[string]*rate.Limiter),
		r:        r,
		b:        b,
	}
}

// getLimiter returns the rate limiter for the given IP address.
func (i *IPTracker) getLimiter(ip string) *rate.Limiter {
	i.mu.RLock()
	limiter, exists := i.limiters[ip]
	i.mu.RUnlock()

	if !exists {
		i.mu.Lock()
		limiter, exists = i.limiters[ip]
		if !exists {
			limiter = rate.NewLimiter(i.r, i.b)
			i.limiters[ip] = limiter
		}
		i.mu.Unlock()
	}

	return limiter
}

// RateLimit middleware enforces a rate limit per client IP.
func (i *IPTracker) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		// Basic naive IP extraction if RemoteAddr includes port
		if strings.Index(ip, ":") != -1 {
			ip = strings.Split(ip, ":")[0]
		}
		limiter := i.getLimiter(ip)

		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
