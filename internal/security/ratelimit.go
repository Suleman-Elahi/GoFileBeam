package security

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter implements token bucket rate limiting per IP.
//
// The bucket holds up to `burst` tokens and refills continuously at
// `rate` tokens per window. Two properties matter and were both wrong in
// the previous implementation:
//
//  1. Tokens are a float and refill is computed from the time elapsed
//     since the LAST REFILL (not since the last request). Measuring from
//     the last request meant that during a burst of rapid requests —
//     exactly what a parallel chunked upload produces — each call saw a
//     tiny elapsed interval, truncated it to an integer number of tokens
//     (often 0), and reset the clock, so the bucket never refilled while
//     it was being drained. That turned a legitimate upload into a 429
//     partway through.
//  2. `burst` is configurable independently of `rate`, so a client can
//     be allowed a bounded burst (e.g. all chunks of one large file)
//     without raising the sustained per-minute budget that actually
//     protects against abuse.
type RateLimiter struct {
	visitors map[string]*Visitor
	mu       sync.RWMutex
	rate     int           // sustained requests per window
	burst    int           // bucket capacity (max tokens)
	window   time.Duration
	// violationsBeforeBlock is how many denied requests in a row (while
	// the bucket is empty) trigger a temporary block. 0 disables blocking
	// for this limiter — used for the chunk-upload limiter, where sessions
	// already gate access and a block would lock out a legitimate large
	// upload that is merely going fast.
	violationsBeforeBlock int
}

// Visitor tracks rate limit info for an IP
type Visitor struct {
	tokens       float64
	lastRefill   time.Time
	lastSeen     time.Time
	violations   int // track repeated violations
	blocked      bool
	blockedUntil time.Time
}

// NewRateLimiter creates a new rate limiter with burst == rate and
// violation blocking enabled (the general-purpose configuration used for
// non-upload routes).
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	return NewRateLimiterWithBurst(requestsPerMinute, requestsPerMinute, 5)
}

// NewRateLimiterWithBurst creates a rate limiter with an explicit bucket
// capacity and violation policy.
//
// blockAfterViolations: 0 = never block on violations (the bucket alone
// throttles), N = block the IP for 15 minutes after N denials.
func NewRateLimiterWithBurst(requestsPerMinute, burst, blockAfterViolations int) *RateLimiter {
	if burst < 1 {
		burst = 1
	}
	rl := &RateLimiter{
		visitors:              make(map[string]*Visitor),
		rate:                  requestsPerMinute,
		burst:                 burst,
		window:                time.Minute,
		violationsBeforeBlock: blockAfterViolations,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request from an IP should be allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	visitor, exists := rl.visitors[ip]
	if !exists {
		visitor = &Visitor{
			tokens:     float64(rl.burst),
			lastRefill: now,
			lastSeen:   now,
		}
		rl.visitors[ip] = visitor
	}

	// Check if IP is blocked
	if visitor.blocked {
		if now.Before(visitor.blockedUntil) {
			return false
		}
		// Unblock if time has passed; the elapsed-time refill below
		// restores the bucket to full automatically.
		visitor.blocked = false
		visitor.violations = 0
	}

	// Refill continuously based on time since the last refill, keeping
	// fractional tokens so short intervals accumulate instead of being
	// truncated away.
	if elapsed := now.Sub(visitor.lastRefill).Seconds(); elapsed > 0 {
		visitor.tokens += elapsed / rl.window.Seconds() * float64(rl.rate)
		if visitor.tokens > float64(rl.burst) {
			visitor.tokens = float64(rl.burst)
		}
		visitor.lastRefill = now
	}

	visitor.lastSeen = now

	// Check if tokens available
	if visitor.tokens >= 1 {
		visitor.tokens--
		return true
	}

	// Rate limit exceeded
	visitor.violations++

	// Block IP after repeated violations (only if this limiter blocks)
	if rl.violationsBeforeBlock > 0 && visitor.violations >= rl.violationsBeforeBlock {
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

// GetClientIP extracts the real client IP from request.
//
// Order of precedence:
//  1. CF-Connecting-IP - set by Cloudflare to the true original client IP.
//     Most reliable when the deployment sits behind Cloudflare, since
//     Cloudflare overwrites this header itself and it cannot be spoofed by
//     the client as long as direct (non-proxied) access to the origin is
//     blocked.
//  2. X-Forwarded-For - standard proxy header, a comma-separated list where
//     the first entry is (by convention) the original client. Only trusted
//     if it parses as a valid IP; each entry is trimmed of whitespace
//     before parsing. Note this header CAN be spoofed by the client unless
//     the reverse proxy in front of this app overwrites/sanitizes it.
//  3. X-Real-IP - set by some reverse proxies (e.g. nginx) to a single IP.
//  4. r.RemoteAddr - the actual TCP peer address. This includes a port
//     (e.g. "1.2.3.4:51000"), which must be stripped, otherwise every new
//     connection from the same client (a new ephemeral port) is treated as
//     a distinct identity, silently disabling both rate limiting and
//     brute-force protection. This was confirmed live: with no trusted
//     proxy headers present, distinct curl invocations against the same
//     client never triggered brute-force blocking because each one got a
//     unique source port.
func GetClientIP(r *http.Request) string {
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		if ip := validIP(cfIP); ip != "" {
			return ip
		}
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		for _, part := range strings.Split(xff, ",") {
			candidate := strings.TrimSpace(part)
			if ip := validIP(candidate); ip != "" {
				return ip
			}
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := validIP(strings.TrimSpace(xri)); ip != "" {
			return ip
		}
	}

	// Fall back to the TCP peer address, stripping the port.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// validIP returns s if it parses as a valid IP address (v4 or v6, with or
// without a zone/port already stripped), otherwise returns "". This guards
// against malformed or garbage header values being used as a rate-limit or
// brute-force tracking key.
func validIP(s string) string {
	if s == "" {
		return ""
	}
	// Headers occasionally include a port (nonstandard but seen in the
	// wild); strip it if present before validating.
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	if net.ParseIP(s) != nil {
		return s
	}
	return ""
}
