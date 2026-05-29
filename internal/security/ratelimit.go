package security

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements token bucket rate limiting per IP
type RateLimiter struct {
	visitors map[string]*Visitor
	mu       sync.RWMutex
	rate     int           // requests per window
	window   time.Duration // time window
}

// Visitor tracks rate limit info for an IP
type Visitor struct {
	tokens     int
	lastSeen   time.Time
	violations int // track repeated violations
	blocked    bool
	blockedUntil time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*Visitor),
		rate:     requestsPerMinute,
		window:   time.Minute,
	}
	
	// Start cleanup goroutine
	go rl.cleanup()
	
	return rl
}

// Allow checks if a request from an IP should be allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	visitor, exists := rl.visitors[ip]
	if !exists {
		visitor = &Visitor{
			tokens:   rl.rate,
			lastSeen: time.Now(),
		}
		rl.visitors[ip] = visitor
	}
	
	// Check if IP is blocked
	if visitor.blocked {
		if time.Now().Before(visitor.blockedUntil) {
			return false
		}
		// Unblock if time has passed
		visitor.blocked = false
		visitor.violations = 0
	}
	
	// Refill tokens based on time passed
	now := time.Now()
	elapsed := now.Sub(visitor.lastSeen)
	
	if elapsed > rl.window {
		visitor.tokens = rl.rate
	} else {
		// Gradual refill
		tokensToAdd := int(elapsed.Seconds() / rl.window.Seconds() * float64(rl.rate))
		visitor.tokens += tokensToAdd
		if visitor.tokens > rl.rate {
			visitor.tokens = rl.rate
		}
	}
	
	visitor.lastSeen = now
	
	// Check if tokens available
	if visitor.tokens > 0 {
		visitor.tokens--
		return true
	}
	
	// Rate limit exceeded
	visitor.violations++
	
	// Block IP after repeated violations
	if visitor.violations >= 5 {
		visitor.blocked = true
		visitor.blockedUntil = now.Add(15 * time.Minute)
	}
	
	return false
}

// cleanup removes old visitors
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, visitor := range rl.visitors {
			// Remove visitors not seen in 10 minutes (unless blocked)
			if !visitor.blocked && now.Sub(visitor.lastSeen) > 10*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// GetClientIP extracts the real client IP from request
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies/load balancers)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		for idx := 0; idx < len(xff); idx++ {
			if xff[idx] == ',' {
				return xff[:idx]
			}
		}
		return xff
	}
	
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// Fall back to RemoteAddr
	return r.RemoteAddr
}
